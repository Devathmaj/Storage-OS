#include "oscore_integrated.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <sys/stat.h>
#include <time.h>
#include <errno.h>

// Initialize the integrated storage system
int oscore_init(OSCoreStorage *storage, const char *storage_root, size_t max_handles) {
    if (!storage || !storage_root) {
        return -1;
    }
    
    memset(storage, 0, sizeof(OSCoreStorage));
    
    // Initialize query index
    if (query_init_index(&storage->query_index) != 0) {
        return -1;
    }
    
    // Set storage root
    storage->storage_root = strdup(storage_root);
    if (!storage->storage_root) {
        query_free_index(&storage->query_index);
        return -1;
    }
    
    // Create storage directory if it doesn't exist
    mkdir(storage_root, 0755);
    
    // Initialize handle pool
    storage->max_handles = max_handles > 0 ? max_handles : 64;
    storage->active_handles = calloc(storage->max_handles, sizeof(FSHandle));
    if (!storage->active_handles) {
        free(storage->storage_root);
        query_free_index(&storage->query_index);
        return -1;
    }
    
    storage->handle_count = 0;
    
    return 0;
}

// Helper to construct full path
static char* oscore_build_path(OSCoreStorage *storage, const char *name) {
    size_t root_len = strlen(storage->storage_root);
    size_t name_len = strlen(name);
    char *full_path = malloc(root_len + name_len + 2);
    
    if (!full_path) {
        return NULL;
    }
    
    snprintf(full_path, root_len + name_len + 2, "%s/%s", storage->storage_root, name);
    return full_path;
}

// Store a file with automatic indexing
int oscore_store_file(OSCoreStorage *storage, const char *name, const char *path, 
                     const char *data, size_t data_size, const char *type) {
    if (!storage || !name || !data || !type) {
        return -1;
    }
    
    // Build full storage path
    char *full_path = oscore_build_path(storage, name);
    if (!full_path) {
        return -1;
    }
    
    // Write file using optimized fs layer
    int result = fs_write_file(full_path, data, data_size);
    if (result < 0) {
        free(full_path);
        return -1;
    }
    
    // Add to query index
    time_t now = time(NULL);
    const char *index_path = path ? path : full_path;
    
    if (query_add_file(&storage->query_index, name, index_path, data_size, 
                       now, now, type) != 0) {
        // Rollback: delete the file
        fs_delete_file(full_path);
        free(full_path);
        return -1;
    }
    
    free(full_path);
    return 0;
}

// Retrieve a file using optimized read
int oscore_retrieve_file(OSCoreStorage *storage, const char *name, char **data, size_t *size) {
    if (!storage || !name || !data || !size) {
        return -1;
    }
    
    // Build full path
    char *full_path = oscore_build_path(storage, name);
    if (!full_path) {
        return -1;
    }
    
    // Get file size
    struct stat st;
    if (stat(full_path, &st) != 0) {
        free(full_path);
        return -1;
    }
    
    // Allocate buffer
    *size = st.st_size;
    *data = malloc(*size + 1);
    if (!*data) {
        free(full_path);
        return -1;
    }
    
    // Read using optimized fs layer
    int result = fs_read_file(full_path, *data, *size + 1);
    free(full_path);
    
    if (result < 0) {
        free(*data);
        *data = NULL;
        *size = 0;
        return -1;
    }
    
    return 0;
}

// Query files and retrieve them efficiently (uses mmap for large files)
int oscore_query_and_retrieve(OSCoreStorage *storage, const char *name, 
                              char **data, size_t *size) {
    if (!storage || !name || !data || !size) {
        return -1;
    }
    
    // First, query the index to verify file exists
    FileMetadata *results[1];
    size_t result_count = 0;
    
    if (query_by_name(&storage->query_index, name, results, &result_count) != 0) {
        return -1;
    }
    
    if (result_count == 0) {
        return -1; // File not found
    }
    
    // Build full path
    char *full_path = oscore_build_path(storage, name);
    if (!full_path) {
        return -1;
    }
    
    // For large files, use memory-mapped I/O
    FileMetadata *meta = results[0];
    if (meta->size > 1024 * 1024) { // > 1MB
        FSHandle handle;
        if (fs_open_handle(&handle, full_path, O_RDONLY) != 0) {
            free(full_path);
            return -1;
        }
        
        void *mapped_data = NULL;
        ssize_t result = fs_read_mmap(&handle, &mapped_data, size);
        
        if (result > 0) {
            // Copy from mapped region (in production, you might keep it mapped)
            *data = malloc(*size);
            if (*data) {
                memcpy(*data, mapped_data, *size);
            }
        }
        
        fs_close_handle(&handle);
        free(full_path);
        
        return result > 0 && *data ? 0 : -1;
    } else {
        // For small files, use regular read
        free(full_path);
        return oscore_retrieve_file(storage, name, data, size);
    }
}

// Batch store multiple files
int oscore_batch_store(OSCoreStorage *storage, const char **names, const char **paths,
                      const char **data, size_t *sizes, const char **types, int count) {
    if (!storage || !names || !data || !sizes || !types || count <= 0) {
        return -1;
    }
    
    // Build full paths
    char **full_paths = malloc(count * sizeof(char*));
    if (!full_paths) {
        return -1;
    }
    
    for (int i = 0; i < count; i++) {
        full_paths[i] = oscore_build_path(storage, names[i]);
        if (!full_paths[i]) {
            // Cleanup
            for (int j = 0; j < i; j++) {
                free(full_paths[j]);
            }
            free(full_paths);
            return -1;
        }
    }
    
    // Use batch write for better performance
    int result = fs_batch_write((const char**)full_paths, data, sizes, count);
    
    // Add all to index
    if (result == 0) {
        time_t now = time(NULL);
        for (int i = 0; i < count; i++) {
            const char *index_path = paths && paths[i] ? paths[i] : full_paths[i];
            query_add_file(&storage->query_index, names[i], index_path, 
                          sizes[i], now, now, types[i]);
        }
    }
    
    // Cleanup
    for (int i = 0; i < count; i++) {
        free(full_paths[i]);
    }
    free(full_paths);
    
    return result;
}

// Delete file and remove from index
int oscore_delete_file(OSCoreStorage *storage, const char *name) {
    if (!storage || !name) {
        return -1;
    }
    
    // Build full path
    char *full_path = oscore_build_path(storage, name);
    if (!full_path) {
        return -1;
    }
    
    // Delete file
    int result = fs_delete_file(full_path);
    free(full_path);
    
    // Note: Query index doesn't have a delete function in current implementation
    // In production, you'd need to add query_remove_file() to the query module
    
    return result;
}

// Free the storage system
void oscore_free(OSCoreStorage *storage) {
    if (!storage) {
        return;
    }
    
    // Close all active handles
    for (size_t i = 0; i < storage->handle_count; i++) {
        fs_close_handle(&storage->active_handles[i]);
    }
    
    free(storage->active_handles);
    free(storage->storage_root);
    query_free_index(&storage->query_index);
    
    memset(storage, 0, sizeof(OSCoreStorage));
}

// Get statistics
int oscore_get_stats(OSCoreStorage *storage, OSCoreStats *stats) {
    if (!storage || !stats) {
        return -1;
    }
    
    memset(stats, 0, sizeof(OSCoreStats));
    
    stats->total_files = storage->query_index.count;
    
    // Calculate total size
    for (size_t i = 0; i < storage->query_index.count; i++) {
        stats->total_size += storage->query_index.files[i].size;
    }
    
    // Cache stats would need to be tracked in the query module
    stats->cache_hits = 0;
    stats->cache_misses = 0;
    
    return 0;
}
