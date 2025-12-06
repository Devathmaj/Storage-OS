#include "fs.h"
#include <fcntl.h>
#include <unistd.h>
#include <stdio.h>
#include <errno.h>
#include <string.h>
#include <stdlib.h>
#include <sys/mman.h>
#include <sys/stat.h>
#include <sys/sendfile.h>
#include <sys/syscall.h>
#include <aio.h>

#define WRITE_BUFFER_SIZE 1048576  // 1MB buffer for writes

// Function to create a new file
int fs_create_file(const char *filepath) {
    int fd = syscall(SYS_open, filepath, O_CREAT | O_WRONLY | O_TRUNC, 0644);
    if (fd == -1) {
        perror("fs_create_file: open failed");
        return -1;
    }
    syscall(SYS_close, fd);
    return 0;
}

// Function to read from a file into a buffer (optimized with readahead hint)
int fs_read_file(const char *filepath, char *buffer, size_t buffer_size) {
    if (!buffer || buffer_size == 0) {
        errno = EINVAL;
        return -1;
    }

    int fd = syscall(SYS_open, filepath, O_RDONLY);
    if (fd == -1) {
        perror("fs_read_file: open failed");
        return -1;
    }
    
    // Hint for sequential read optimization (ignore errors as it's just a hint)
    (void)posix_fadvise(fd, 0, 0, POSIX_FADV_SEQUENTIAL);

    size_t remaining = buffer_size - 1;
    ssize_t total = 0;
    while (remaining > 0) {
        ssize_t bytes = syscall(SYS_read, fd, buffer + total, remaining);
        if (bytes == 0) {
            break; // EOF
        }
        if (bytes < 0) {
            if (errno == EINTR) {
                continue;
            }
            perror("fs_read_file: read failed");
            syscall(SYS_close, fd);
            return -1;
        }
        total += bytes;
        remaining -= (size_t)bytes;
    }
    buffer[total] = '\0';
    syscall(SYS_close, fd);
    return (int)total;
}

// Function to write data to a file (optimized with write hints)
int fs_write_file(const char *filepath, const char *data, size_t data_size) {
    int fd = syscall(SYS_open, filepath, O_WRONLY | O_CREAT | O_TRUNC, 0644);
    if (fd == -1) {
        perror("fs_write_file: open failed");
        return -1;
    }
    
    // Hint for sequential write optimization (ignore errors as it's just a hint)
    (void)posix_fadvise(fd, 0, data_size, POSIX_FADV_SEQUENTIAL);
    
    size_t total = 0;
    while (total < data_size) {
        ssize_t written = syscall(SYS_write, fd, data + total, data_size - total);
        if (written < 0) {
            if (errno == EINTR) {
                continue;
            }
            perror("fs_write_file: write failed");
            syscall(SYS_close, fd);
            return -1;
        }
        total += (size_t)written;
    }
    syscall(SYS_close, fd);
    return (int)total;
}

// Function to delete a file
int fs_delete_file(const char *filepath) {
    if (unlink(filepath) == -1) {
        perror("fs_delete_file: unlink failed");
        return -1;
    }
    return 0;
}

// Open a file handle for optimized operations
int fs_open_handle(FSHandle *handle, const char *filepath, int flags) {
    memset(handle, 0, sizeof(FSHandle));
    
    handle->fd = syscall(SYS_open, filepath, flags, 0644);
    if (handle->fd == -1) {
        perror("fs_open_handle: open failed");
        return -1;
    }
    
    // Allocate write buffer for buffered writes
    if (flags & O_WRONLY || flags & O_RDWR) {
        handle->write_buffer = fs_alloc_huge_page(WRITE_BUFFER_SIZE);
        if (!handle->write_buffer) {
            // Fallback to regular malloc
            handle->write_buffer = malloc(WRITE_BUFFER_SIZE);
            if (!handle->write_buffer) {
                close(handle->fd);
                return -1;
            }
        }
        handle->buffer_capacity = WRITE_BUFFER_SIZE;
        handle->buffer_pos = 0;
    }
    
    return 0;
}

// Close file handle and cleanup
int fs_close_handle(FSHandle *handle) {
    if (handle->write_buffer) {
        fs_flush_buffer(handle);  // Flush any pending writes
        fs_free_huge_page(handle->write_buffer, handle->buffer_capacity);
    }
    
    if (handle->mmap_addr) {
        munmap(handle->mmap_addr, handle->mmap_size);
    }
    
    // Do not close fd here, as it's managed by the caller
    // if (handle->fd != -1) {
    //     syscall(SYS_close, handle->fd);
    // }
    
    return 0;
}

ssize_t fs_handle_read(FSHandle *handle, char *buffer, size_t size) {
    if (!handle || handle->fd == -1 || !buffer || size == 0) {
        errno = EINVAL;
        return -1;
    }

    while (1) {
        ssize_t bytes = syscall(SYS_read, handle->fd, buffer, size);
        if (bytes < 0) {
            if (errno == EINTR) {
                continue;
            }
            perror("fs_handle_read: read failed");
            return -1;
        }
        return bytes;
    }
}

// Memory-mapped read for large files (zero-copy)
ssize_t fs_read_mmap(FSHandle *handle, void **data, size_t *size) {
    struct stat sb;
    if (syscall(SYS_fstat, handle->fd, &sb) == -1) {
        perror("fs_read_mmap: fstat failed");
        return -1;
    }
    
    *size = sb.st_size;
    if (*size == 0) {
        *data = NULL;
        return 0;
    }
    
    *data = mmap(NULL, *size, PROT_READ, MAP_PRIVATE, handle->fd, 0);
    if (*data == MAP_FAILED) {
        perror("fs_read_mmap: mmap failed");
        return -1;
    }
    
    // Hint for sequential access
    madvise(*data, *size, MADV_SEQUENTIAL);
    
    handle->mmap_addr = *data;
    handle->mmap_size = *size;
    
    return *size;
}

// Buffered write for improved performance
ssize_t fs_write_buffered(FSHandle *handle, const char *data, size_t size) {
    if (!handle->write_buffer) {
        return write(handle->fd, data, size);
    }
    
    size_t written = 0;
    while (written < size) {
        size_t to_copy = size - written;
        size_t available = handle->buffer_capacity - handle->buffer_pos;
        
        if (to_copy > available) {
            to_copy = available;
        }
        
        memcpy(handle->write_buffer + handle->buffer_pos, data + written, to_copy);
        handle->buffer_pos += to_copy;
        written += to_copy;
        
        // Flush if buffer is full
        if (handle->buffer_pos >= handle->buffer_capacity) {
            if (fs_flush_buffer(handle) == -1) {
                return -1;
            }
        }
    }
    
    return written;
}

// Flush write buffer
int fs_flush_buffer(FSHandle *handle) {
    if (!handle->write_buffer || handle->buffer_pos == 0) {
        return 0;
    }

    size_t total = 0;
    while (total < handle->buffer_pos) {
        ssize_t written = syscall(SYS_write, handle->fd, handle->write_buffer + total, handle->buffer_pos - total);
        if (written < 0) {
            if (errno == EINTR) {
                continue;
            }
            perror("fs_flush_buffer: write failed");
            return -1;
        }
        total += (size_t)written;
    }

    handle->buffer_pos = 0;
    return 0;
}

// Batch write operations (reduces syscall overhead)
int fs_batch_write(const char **filepaths, const char **data, size_t *sizes, int count) {
    for (int i = 0; i < count; i++) {
        if (fs_write_file(filepaths[i], data[i], sizes[i]) < 0) {
            return -1;
        }
    }
    return 0;
}

// Batch read operations
int fs_batch_read(const char **filepaths, char **buffers, size_t *buffer_sizes, int count) {
    for (int i = 0; i < count; i++) {
        if (fs_read_file(filepaths[i], buffers[i], buffer_sizes[i]) < 0) {
            return -1;
        }
    }
    return 0;
}

// Zero-copy file transfer using sendfile
ssize_t fs_sendfile(const char *source, const char *dest) {
    int src_fd = syscall(SYS_open, source, O_RDONLY);
    if (src_fd == -1) {
        perror("fs_sendfile: open source failed");
        return -1;
    }
    
    struct stat sb;
    if (syscall(SYS_fstat, src_fd, &sb) == -1) {
        perror("fs_sendfile: fstat failed");
        syscall(SYS_close, src_fd);
        return -1;
    }
    
    int dst_fd = syscall(SYS_open, dest, O_WRONLY | O_CREAT | O_TRUNC, 0644);
    if (dst_fd == -1) {
        perror("fs_sendfile: open dest failed");
        syscall(SYS_close, src_fd);
        return -1;
    }
    
    ssize_t total = 0;
    off_t offset = 0;
    while (offset < sb.st_size) {
        ssize_t sent = sendfile(dst_fd, src_fd, &offset, sb.st_size - offset);
        if (sent == -1) {
            perror("fs_sendfile: sendfile failed");
            syscall(SYS_close, src_fd);
            syscall(SYS_close, dst_fd);
            return -1;
        }
        total += sent;
    }
    
    syscall(SYS_close, src_fd);
    syscall(SYS_close, dst_fd);
    return total;
}

// Async write operation
int fs_write_async(FSHandle *handle, const char *data, size_t size) {
    if (handle->async_pending) {
        return -1; // Previous async operation still pending
    }
    
    memset(&handle->aio_cb, 0, sizeof(struct aiocb));
    handle->aio_cb.aio_fildes = handle->fd;
    handle->aio_cb.aio_buf = (void*)data;
    handle->aio_cb.aio_nbytes = size;
    handle->aio_cb.aio_offset = 0; // Append mode
    
    if (aio_write(&handle->aio_cb) == -1) {
        return -1;
    }
    
    handle->async_pending = 1;
    return 0;
}

// Async read operation
int fs_read_async(FSHandle *handle, char *buffer, size_t size) {
    if (handle->async_pending) {
        return -1; // Previous async operation still pending
    }
    
    memset(&handle->aio_cb, 0, sizeof(struct aiocb));
    handle->aio_cb.aio_fildes = handle->fd;
    handle->aio_cb.aio_buf = buffer;
    handle->aio_cb.aio_nbytes = size;
    handle->aio_cb.aio_offset = 0;
    
    if (aio_read(&handle->aio_cb) == -1) {
        return -1;
    }
    
    handle->async_pending = 1;
    return 0;
}

// Wait for async operation to complete
int fs_wait_async(FSHandle *handle) {
    if (!handle->async_pending) {
        return 0;
    }
    
    const struct aiocb *aio_list[1] = {&handle->aio_cb};
    if (aio_suspend(aio_list, 1, NULL) == -1) {
        return -1;
    }
    
    ssize_t result = aio_return(&handle->aio_cb);
    handle->async_pending = 0;
    
    return result;
}

// Huge page memory allocation (2MB pages for better TLB performance)
void* fs_alloc_huge_page(size_t size) {
    // Align to 2MB boundary
    size_t aligned_size = (size + 0x1FFFFF) & ~0x1FFFFF; // 2MB alignment
    
    void *ptr = mmap(NULL, aligned_size, PROT_READ | PROT_WRITE, 
                     MAP_PRIVATE | MAP_ANONYMOUS | MAP_HUGETLB, -1, 0);
    
    if (ptr == MAP_FAILED) {
        // Fallback to regular allocation if huge pages not available
        return malloc(size);
    }
    
    // Advise kernel to use huge pages
    madvise(ptr, aligned_size, MADV_HUGEPAGE);
    return ptr;
}

void fs_free_huge_page(void *ptr, size_t size) {
    if (!ptr) return;
    
    size_t aligned_size = (size + 0x1FFFFF) & ~0x1FFFFF;
    
    // Try munmap first (for huge pages)
    if (munmap(ptr, aligned_size) == 0) {
        return;
    }
    
    // Fallback to regular free
    free(ptr);
}