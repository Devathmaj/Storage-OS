#ifndef FS_H
#define FS_H

#include <stddef.h>
#include <sys/types.h>
#include <aio.h>

// File handle for optimized operations
typedef struct {
    int fd;
    void *mmap_addr;
    size_t mmap_size;
    char *write_buffer;
    size_t buffer_pos;
    size_t buffer_capacity;
    struct aiocb aio_cb;  // Async I/O control block
    int async_pending;
} FSHandle;

// Function to create a new file
int fs_create_file(const char *filepath);

// Function to read from a file into a buffer
int fs_read_file(const char *filepath, char *buffer, size_t buffer_size);

// Function to write data to a file
int fs_write_file(const char *filepath, const char *data, size_t data_size);

// Function to delete a file
int fs_delete_file(const char *filepath);

// Optimized functions with memory mapping
int fs_open_handle(FSHandle *handle, const char *filepath, int flags);
int fs_close_handle(FSHandle *handle);
ssize_t fs_read_mmap(FSHandle *handle, void **data, size_t *size);
ssize_t fs_write_buffered(FSHandle *handle, const char *data, size_t size);
int fs_flush_buffer(FSHandle *handle);
ssize_t fs_handle_read(FSHandle *handle, char *buffer, size_t size);

// Batch operations for efficiency
int fs_batch_write(const char **filepaths, const char **data, size_t *sizes, int count);
int fs_batch_read(const char **filepaths, char **buffers, size_t *buffer_sizes, int count);

// Zero-copy operations
ssize_t fs_sendfile(const char *source, const char *dest);

// Async I/O operations
int fs_write_async(FSHandle *handle, const char *data, size_t size);
int fs_read_async(FSHandle *handle, char *buffer, size_t size);
int fs_wait_async(FSHandle *handle);

// Huge page memory allocation for performance
void* fs_alloc_huge_page(size_t size);
void fs_free_huge_page(void *ptr, size_t size);

#endif // FS_H