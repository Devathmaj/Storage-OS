#define _GNU_SOURCE
#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <unistd.h>
#include <fcntl.h>
#include <dlfcn.h>
#include <errno.h>
#include <sys/types.h>
#include <sys/stat.h>
#include <stdarg.h>
#include "fs.h"

// Thread-local storage for tracking file descriptors
#define MAX_FD 8192
static FSHandle fd_handles[MAX_FD];
static int fd_initialized[MAX_FD] = {0};

// Original function pointers
static int (*real_open)(const char *pathname, int flags, ...) = NULL;
static int (*real_close)(int fd) = NULL;
static ssize_t (*real_read)(int fd, void *buf, size_t count) = NULL;
static ssize_t (*real_write)(int fd, const void *buf, size_t count) = NULL;
static FILE* (*real_fopen)(const char *pathname, const char *mode) = NULL;
static int (*real_fclose)(FILE *stream) = NULL;
static size_t (*real_fread)(void *ptr, size_t size, size_t nmemb, FILE *stream) = NULL;
static size_t (*real_fwrite)(const void *ptr, size_t size, size_t nmemb, FILE *stream) = NULL;

// Initialize function pointers
static void init_real_functions(void) {
    if (real_open == NULL) {
        real_open = dlsym(RTLD_NEXT, "open");
        real_close = dlsym(RTLD_NEXT, "close");
        real_read = dlsym(RTLD_NEXT, "read");
        real_write = dlsym(RTLD_NEXT, "write");
        real_fopen = dlsym(RTLD_NEXT, "fopen");
        real_fclose = dlsym(RTLD_NEXT, "fclose");
        real_fread = dlsym(RTLD_NEXT, "fread");
        real_fwrite = dlsym(RTLD_NEXT, "fwrite");
    }
}

// Check if file is a regular file (not socket, pipe, device, etc.)
static int is_regular_file(const char *pathname) {
    struct stat st;
    if (stat(pathname, &st) == 0) {
        return S_ISREG(st.st_mode);
    }
    // If file doesn't exist yet, assume it will be regular
    return 1;
}

// Intercepted open() - optimized for regular files
int open(const char *pathname, int flags, ...) {
    init_real_functions();
    
    // Handle variadic mode argument
    mode_t mode = 0;
    if (flags & O_CREAT) {
        va_list args;
        va_start(args, flags);
        mode = va_arg(args, mode_t);
        va_end(args);
    }
    
    // Skip optimization for special files (stdin, stdout, stderr, devices, pipes)
    if (!pathname || pathname[0] == '\0' || 
        strncmp(pathname, "/dev/", 5) == 0 ||
        strncmp(pathname, "/proc/", 6) == 0 ||
        strncmp(pathname, "/sys/", 5) == 0) {
        return real_open(pathname, flags, mode);
    }
    
    // For regular files, use optimized fs layer
    if (is_regular_file(pathname)) {
        int fd = real_open(pathname, flags, mode);
        if (fd >= 0 && fd < MAX_FD) {
            // Initialize optimized handle for this fd
            if (fs_open_handle(&fd_handles[fd], pathname, flags) == 0) {
                fd_initialized[fd] = 1;
                // Return the real fd, but we'll intercept read/write
                real_close(fd_handles[fd].fd);
                fd_handles[fd].fd = fd;
            }
        }
        return fd;
    }
    
    return real_open(pathname, flags, mode);
}

// Intercepted close() - cleanup optimized handles
int close(int fd) {
    init_real_functions();
    
    if (fd >= 0 && fd < MAX_FD && fd_initialized[fd]) {
        fs_close_handle(&fd_handles[fd]);
        fd_initialized[fd] = 0;
    }
    
    return real_close(fd);
}

// Intercepted read() - use optimized fs layer for tracked files
ssize_t read(int fd, void *buf, size_t count) {
    init_real_functions();
    
    if (fd >= 0 && fd < MAX_FD && fd_initialized[fd]) {
        return fs_handle_read(&fd_handles[fd], buf, count);
    }
    
    return real_read(fd, buf, count);
}

// Intercepted write() - use buffered writes for tracked files
ssize_t write(int fd, const void *buf, size_t count) {
    init_real_functions();
    
    if (fd >= 0 && fd < MAX_FD && fd_initialized[fd]) {
        return fs_write_buffered(&fd_handles[fd], buf, count);
    }
    
    return real_write(fd, buf, count);
}

// FILE* wrapper support
typedef struct {
    FILE *stream;
    FSHandle handle;
    char *path;
    int optimized;
} FileWrapper;

#define MAX_FILE_WRAPPERS 256
static FileWrapper file_wrappers[MAX_FILE_WRAPPERS];
static int file_wrapper_count = 0;

static FileWrapper* get_file_wrapper(FILE *stream) {
    for (int i = 0; i < file_wrapper_count; i++) {
        if (file_wrappers[i].stream == stream) {
            return &file_wrappers[i];
        }
    }
    return NULL;
}

// Intercepted fopen() - create optimized wrapper
FILE* fopen(const char *pathname, const char *mode) {
    init_real_functions();
    
    FILE *stream = real_fopen(pathname, mode);
    if (!stream) {
        return NULL;
    }
    
    // Only optimize regular files
    if (is_regular_file(pathname) && file_wrapper_count < MAX_FILE_WRAPPERS) {
        FileWrapper *wrapper = &file_wrappers[file_wrapper_count++];
        wrapper->stream = stream;
        wrapper->path = strdup(pathname);
        
        // Convert mode to flags
        int flags = 0;
        if (strchr(mode, 'r') && strchr(mode, '+')) {
            flags = O_RDWR;
        } else if (strchr(mode, 'r')) {
            flags = O_RDONLY;
        } else if (strchr(mode, 'w')) {
            flags = O_WRONLY | O_CREAT | O_TRUNC;
        } else if (strchr(mode, 'a')) {
            flags = O_WRONLY | O_CREAT | O_APPEND;
        }
        
        if (fs_open_handle(&wrapper->handle, pathname, flags) == 0) {
            wrapper->optimized = 1;
        } else {
            wrapper->optimized = 0;
        }
    }
    
    return stream;
}

// Intercepted fclose() - cleanup wrapper
int fclose(FILE *stream) {
    init_real_functions();
    
    FileWrapper *wrapper = get_file_wrapper(stream);
    if (wrapper && wrapper->optimized) {
        fs_close_handle(&wrapper->handle);
        free(wrapper->path);
        // Remove from array by shifting
        for (int i = 0; i < file_wrapper_count; i++) {
            if (&file_wrappers[i] == wrapper) {
                memmove(&file_wrappers[i], &file_wrappers[i+1], 
                       (file_wrapper_count - i - 1) * sizeof(FileWrapper));
                file_wrapper_count--;
                break;
            }
        }
    }
    
    return real_fclose(stream);
}

// Intercepted fread() - use optimized reads
size_t fread(void *ptr, size_t size, size_t nmemb, FILE *stream) {
    init_real_functions();
    
    FileWrapper *wrapper = get_file_wrapper(stream);
    if (wrapper && wrapper->optimized) {
        ssize_t result = fs_handle_read(&wrapper->handle, ptr, size * nmemb);
        if (result < 0) {
            return 0;
        }
        return result / size;
    }
    
    return real_fread(ptr, size, nmemb, stream);
}

// Intercepted fwrite() - use buffered writes
size_t fwrite(const void *ptr, size_t size, size_t nmemb, FILE *stream) {
    init_real_functions();
    
    FileWrapper *wrapper = get_file_wrapper(stream);
    if (wrapper && wrapper->optimized) {
        ssize_t result = fs_write_buffered(&wrapper->handle, ptr, size * nmemb);
        if (result < 0) {
            return 0;
        }
        return result / size;
    }
    
    return real_fwrite(ptr, size, nmemb, stream);
}

// Constructor to initialize on library load
__attribute__((constructor))
static void fs_preload_init(void) {
    init_real_functions();
    memset(fd_handles, 0, sizeof(fd_handles));
    memset(fd_initialized, 0, sizeof(fd_initialized));
    memset(file_wrappers, 0, sizeof(file_wrappers));
}

// Destructor to cleanup on library unload
__attribute__((destructor))
static void fs_preload_cleanup(void) {
    // Close any remaining open handles
    for (int i = 0; i < MAX_FD; i++) {
        if (fd_initialized[i]) {
            fs_close_handle(&fd_handles[i]);
        }
    }
    
    // Cleanup file wrappers
    for (int i = 0; i < file_wrapper_count; i++) {
        if (file_wrappers[i].optimized) {
            fs_close_handle(&file_wrappers[i].handle);
            free(file_wrappers[i].path);
        }
    }
}
