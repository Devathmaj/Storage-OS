#ifndef OSCORE_INTEGRATED_H
#define OSCORE_INTEGRATED_H

#include "../query/query.h"
#include "../fs/fs.h"

// Integrated file storage and query system
typedef struct {
    FileIndex query_index;
    char *storage_root;
    FSHandle *active_handles;
    size_t max_handles;
    size_t handle_count;
} OSCoreStorage;

// Initialize the integrated storage system
int oscore_init(OSCoreStorage *storage, const char *storage_root, size_t max_handles);

// Store a file with automatic indexing
int oscore_store_file(OSCoreStorage *storage, const char *name, const char *path, 
                     const char *data, size_t data_size, const char *type);

// Retrieve a file using optimized read
int oscore_retrieve_file(OSCoreStorage *storage, const char *name, char **data, size_t *size);

// Query files and retrieve them efficiently
int oscore_query_and_retrieve(OSCoreStorage *storage, const char *name, 
                              char **data, size_t *size);

// Batch store multiple files
int oscore_batch_store(OSCoreStorage *storage, const char **names, const char **paths,
                      const char **data, size_t *sizes, const char **types, int count);

// Delete file and remove from index
int oscore_delete_file(OSCoreStorage *storage, const char *name);

// Free the storage system
void oscore_free(OSCoreStorage *storage);

// Statistics
typedef struct {
    size_t total_files;
    size_t total_size;
    size_t cache_hits;
    size_t cache_misses;
} OSCoreStats;

int oscore_get_stats(OSCoreStorage *storage, OSCoreStats *stats);

#endif // OSCORE_INTEGRATED_H
