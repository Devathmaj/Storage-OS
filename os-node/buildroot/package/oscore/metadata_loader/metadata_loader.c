#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <stdarg.h>
#include <limits.h>
#include <sys/sysinfo.h>
#include <sys/stat.h>
#include <sys/mman.h>
#include <fcntl.h>
#include <errno.h>
#include <dirent.h>
#include <time.h>
#include <signal.h>
#include "../query/query.h"

#define METADATA_CACHE_FILE "/var/cache/metadata_index.bin"
#define STORAGE_ROOT "/data"
#define PID_FILE "/var/run/metadata_loader.pid"
#define LOG_FILE "/var/log/metadata_loader.log"

static FileIndex global_index;
static volatile sig_atomic_t running = 1;
static FILE *log_fp = NULL;

// Logging function
static void log_message(const char *level, const char *format, ...) {
    if (!log_fp) {
        log_fp = fopen(LOG_FILE, "a");
        if (!log_fp) {
            return;
        }
    }
    
    time_t now = time(NULL);
    char time_buf[64];
    strftime(time_buf, sizeof(time_buf), "%Y-%m-%d %H:%M:%S", localtime(&now));
    
    fprintf(log_fp, "[%s] [%s] ", time_buf, level);
    
    va_list args;
    va_start(args, format);
    vfprintf(log_fp, format, args);
    va_end(args);
    
    fprintf(log_fp, "\n");
    fflush(log_fp);
}

// Signal handler for graceful shutdown
static void signal_handler(int sig) {
    running = 0;
    log_message("INFO", "Received signal %d, shutting down", sig);
}

// Determine optimal memory limit based on system RAM
static size_t get_optimal_memory_limit() {
    struct sysinfo info;
    if (sysinfo(&info) != 0) {
        log_message("WARN", "Failed to get system info, using default 128MB");
        return 128 * 1024 * 1024;
    }
    
    unsigned long total_ram = info.totalram * info.mem_unit;
    size_t memory_limit;
    
    // Use adaptive sizing based on total RAM
    if (total_ram < 512UL * 1024 * 1024) {
        // Less than 512MB: use 16MB
        memory_limit = 16 * 1024 * 1024;
    } else if (total_ram < 1024UL * 1024 * 1024) {
        // 512MB - 1GB: use 64MB
        memory_limit = 64 * 1024 * 1024;
    } else if (total_ram < 2048UL * 1024 * 1024) {
        // 1GB - 2GB: use 128MB
        memory_limit = 128 * 1024 * 1024;
    } else if (total_ram < 4096UL * 1024 * 1024) {
        // 2GB - 4GB: use 256MB
        memory_limit = 256 * 1024 * 1024;
    } else {
        // 4GB+: use 512MB (max)
        memory_limit = 512 * 1024 * 1024;
    }
    
    log_message("INFO", "System RAM: %lu MB, allocating %zu MB for metadata cache",
                total_ram / (1024 * 1024), memory_limit / (1024 * 1024));
    
    return memory_limit;
}

// Get file type from extension
static const char* get_file_type(const char *name) {
    const char *dot = strrchr(name, '.');
    if (!dot || dot == name) {
        return "unknown";
    }
    return dot + 1;
}

// Recursively scan directory and add files to index
static int scan_directory(FileIndex *index, const char *base_path, int *files_scanned, int *files_skipped) {
    DIR *dir = opendir(base_path);
    if (!dir) {
        log_message("WARN", "Failed to open directory: %s", base_path);
        return -1;
    }
    
    struct dirent *entry;
    while ((entry = readdir(dir)) != NULL) {
        if (strcmp(entry->d_name, ".") == 0 || strcmp(entry->d_name, "..") == 0) {
            continue;
        }
        
        char full_path[PATH_MAX];
        snprintf(full_path, sizeof(full_path), "%s/%s", base_path, entry->d_name);
        
        struct stat st;
        if (stat(full_path, &st) != 0) {
            log_message("WARN", "Failed to stat: %s", full_path);
            continue;
        }
        
        if (S_ISDIR(st.st_mode)) {
            // Recursively scan subdirectory
            scan_directory(index, full_path, files_scanned, files_skipped);
        } else if (S_ISREG(st.st_mode)) {
            // Add file to index
            const char *file_type = get_file_type(entry->d_name);
            
            // Check if we can add before attempting
            if (!query_can_add_file(index, entry->d_name, full_path, file_type)) {
                (*files_skipped)++;
                if (*files_skipped == 1) {
                    log_message("WARN", "Memory limit reached, skipping remaining files");
                }
                continue;
            }
            
            int result = query_add_file(index, entry->d_name, full_path, 
                                       st.st_size, st.st_ctime, st.st_mtime, file_type);
            
            if (result == 0) {
                (*files_scanned)++;
                if (*files_scanned % 10000 == 0) {
                    log_message("INFO", "Scanned %d files, memory usage: %zu MB",
                               *files_scanned, query_get_memory_usage(index) / (1024 * 1024));
                }
            } else if (result == -2) {
                (*files_skipped)++;
                if (*files_skipped == 1) {
                    log_message("WARN", "Memory limit reached at %d files", *files_scanned);
                }
            }
        }
    }
    
    closedir(dir);
    return 0;
}

// Save index to persistent cache
static int save_index_cache(FileIndex *index) {
    log_message("INFO", "Saving metadata cache to %s", METADATA_CACHE_FILE);
    
    FILE *fp = fopen(METADATA_CACHE_FILE, "wb");
    if (!fp) {
        log_message("ERROR", "Failed to create cache file: %s", strerror(errno));
        return -1;
    }
    
    // Write header
    uint32_t magic = 0x4D455441; // "META"
    uint32_t version = 1;
    fwrite(&magic, sizeof(magic), 1, fp);
    fwrite(&version, sizeof(version), 1, fp);
    fwrite(&index->count, sizeof(index->count), 1, fp);
    
    // Write file metadata
    for (size_t i = 0; i < index->count; i++) {
        FileMetadata *file = &index->files[i];
        
        uint32_t name_len = strlen(file->name) + 1;
        uint32_t path_len = strlen(file->path) + 1;
        uint32_t type_len = strlen(file->type) + 1;
        
        fwrite(&name_len, sizeof(name_len), 1, fp);
        fwrite(file->name, name_len, 1, fp);
        
        fwrite(&path_len, sizeof(path_len), 1, fp);
        fwrite(file->path, path_len, 1, fp);
        
        fwrite(&type_len, sizeof(type_len), 1, fp);
        fwrite(file->type, type_len, 1, fp);
        
        fwrite(&file->size, sizeof(file->size), 1, fp);
        fwrite(&file->created, sizeof(file->created), 1, fp);
        fwrite(&file->modified, sizeof(file->modified), 1, fp);
    }
    
    fclose(fp);
    log_message("INFO", "Cache saved successfully (%zu files)", index->count);
    return 0;
}

// Load index from persistent cache
static int load_index_cache(FileIndex *index) {
    struct stat st;
    if (stat(METADATA_CACHE_FILE, &st) != 0) {
        log_message("INFO", "No cache file found, will perform full scan");
        return -1;
    }
    
    log_message("INFO", "Loading metadata cache from %s", METADATA_CACHE_FILE);
    
    FILE *fp = fopen(METADATA_CACHE_FILE, "rb");
    if (!fp) {
        log_message("ERROR", "Failed to open cache file: %s", strerror(errno));
        return -1;
    }
    
    uint32_t magic, version;
    size_t count;
    
    if (fread(&magic, sizeof(magic), 1, fp) != 1 || magic != 0x4D455441) {
        log_message("ERROR", "Invalid cache file magic");
        fclose(fp);
        return -1;
    }
    
    fread(&version, sizeof(version), 1, fp);
    fread(&count, sizeof(count), 1, fp);
    
    log_message("INFO", "Cache contains %zu files", count);
    
    int loaded = 0;
    int skipped = 0;
    
    for (size_t i = 0; i < count; i++) {
        uint32_t name_len, path_len, type_len;
        char name[256], path[PATH_MAX], type[32];
        size_t size;
        time_t created, modified;
        
        if (fread(&name_len, sizeof(name_len), 1, fp) != 1) break;
        if (name_len > sizeof(name)) break;
        fread(name, name_len, 1, fp);
        
        if (fread(&path_len, sizeof(path_len), 1, fp) != 1) break;
        if (path_len > sizeof(path)) break;
        fread(path, path_len, 1, fp);
        
        if (fread(&type_len, sizeof(type_len), 1, fp) != 1) break;
        if (type_len > sizeof(type)) break;
        fread(type, type_len, 1, fp);
        
        fread(&size, sizeof(size), 1, fp);
        fread(&created, sizeof(created), 1, fp);
        fread(&modified, sizeof(modified), 1, fp);
        
        // Verify file still exists
        struct stat file_st;
        if (stat(path, &file_st) == 0) {
            if (query_can_add_file(index, name, path, type)) {
                query_add_file(index, name, path, size, created, modified, type);
                loaded++;
            } else {
                skipped++;
                break;
            }
        }
    }
    
    fclose(fp);
    log_message("INFO", "Loaded %d files from cache (%d skipped)", loaded, skipped);
    return 0;
}

// Create shared memory segment for IPC
static int create_shared_memory(FileIndex *index) {
    // Create shared memory that other processes can access
    int shm_fd = shm_open("/metadata_index", O_CREAT | O_RDWR, 0666);
    if (shm_fd < 0) {
        log_message("ERROR", "Failed to create shared memory: %s", strerror(errno));
        return -1;
    }
    
    size_t shm_size = sizeof(FileIndex) + (index->count * sizeof(FileMetadata));
    if (ftruncate(shm_fd, shm_size) < 0) {
        log_message("ERROR", "Failed to set shared memory size: %s", strerror(errno));
        close(shm_fd);
        return -1;
    }
    
    close(shm_fd);
    log_message("INFO", "Created shared memory segment (%zu bytes)", shm_size);
    return 0;
}

int main(int argc, char *argv[]) {
    // Setup signal handlers
    signal(SIGTERM, signal_handler);
    signal(SIGINT, signal_handler);
    
    // Open log file
    log_fp = fopen(LOG_FILE, "a");
    
    log_message("INFO", "Starting metadata loader service");
    
    // Create PID file
    FILE *pid_fp = fopen(PID_FILE, "w");
    if (pid_fp) {
        fprintf(pid_fp, "%d\n", getpid());
        fclose(pid_fp);
    }
    
    // Determine optimal memory limit
    size_t memory_limit = get_optimal_memory_limit();
    
    // Initialize index with memory limit
    if (query_init_index_with_limit(&global_index, memory_limit) != 0) {
        log_message("ERROR", "Failed to initialize index");
        return 1;
    }
    
    // Try to load from cache first
    int cache_loaded = load_index_cache(&global_index);
    
    // If cache load failed or incomplete, perform full scan
    if (cache_loaded != 0 || global_index.count == 0) {
        log_message("INFO", "Starting full filesystem scan from %s", STORAGE_ROOT);
        
        int files_scanned = 0;
        int files_skipped = 0;
        
        if (scan_directory(&global_index, STORAGE_ROOT, &files_scanned, &files_skipped) == 0) {
            log_message("INFO", "Filesystem scan complete: %d files indexed, %d skipped",
                       files_scanned, files_skipped);
            
            // Save cache for next boot
            save_index_cache(&global_index);
        } else {
            log_message("ERROR", "Filesystem scan failed");
        }
    }
    
    log_message("INFO", "Index ready: %zu files, %zu MB memory used",
               global_index.count, query_get_memory_usage(&global_index) / (1024 * 1024));
    
    // Create shared memory for other processes
    create_shared_memory(&global_index);
    
    // Keep running to maintain memory
    log_message("INFO", "Service running, keeping metadata in memory");
    
    while (running) {
        sleep(60);  // Wake up every minute to check
    }
    
    // Cleanup
    log_message("INFO", "Shutting down");
    save_index_cache(&global_index);
    query_free_index(&global_index);
    shm_unlink("/metadata_index");
    unlink(PID_FILE);
    
    if (log_fp) {
        fclose(log_fp);
    }
    
    return 0;
}
