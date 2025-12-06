#ifndef QUERY_H
#define QUERY_H

#include <stddef.h>
#include <time.h>
#include <stdint.h>
#include "khash.h"

// Memory pool for efficient allocation
#define POOL_BLOCK_SIZE 1024
typedef struct MemBlock {
    char *data;
    size_t used;
    size_t capacity;
    struct MemBlock *next;
} MemBlock;

typedef struct {
    MemBlock *current;
    size_t block_size;
} MemoryPool;

// Bloom filter for fast existence checks (lightweight)
#define BLOOM_SIZE 65536  // 64KB bit array
#define BLOOM_HASHES 3

typedef struct {
    uint8_t bits[BLOOM_SIZE / 8];  // Bit array
} BloomFilter;

// Define hash types
KHASH_MAP_INIT_STR(str_list, void*)  // string -> pointer to list

// File metadata structure (optimized layout for cache efficiency)
typedef struct {
    char *name;
    char *path;
    char *type;  // e.g., "txt", "jpg"
    size_t size;
    time_t created;
    time_t modified;
} FileMetadata;

// LRU cache for query results (minimal implementation)
#define CACHE_SIZE 256
typedef struct CacheEntry {
    char *key;
    FileMetadata **results;
    size_t count;
    struct CacheEntry *prev, *next;
} CacheEntry;

typedef struct {
    CacheEntry *head, *tail;
    CacheEntry entries[CACHE_SIZE];
    int count;
} LRUCache;

// Optimized array-based list for better performance
typedef struct {
    FileMetadata **files;
    size_t count;
    size_t capacity;
} FileArray;

// Trie node for path prefixes (optimized with smaller children array for common paths)
typedef struct TrieNode {
    struct TrieNode *children[256];
    FileArray files;  // array instead of linked list
    uint8_t has_files;
} TrieNode;

// AVL node for balanced size queries (replaces basic BST)
typedef struct AVLNode {
    size_t size;
    FileArray files;
    struct AVLNode *left;
    struct AVLNode *right;
    int height;
} AVLNode;

// Memory limits for file index (512MB max)
#define MAX_INDEX_MEMORY (512 * 1024 * 1024)  // 512MB
#define MIN_INDEX_MEMORY (16 * 1024 * 1024)   // 16MB minimum
#define METADATA_OVERHEAD 128  // Estimated overhead per file entry

// Index structure with hash tables, trie, and AVL tree
typedef struct {
    khash_t(str_list) *name_index;
    khash_t(str_list) *type_index;
    TrieNode *path_trie;
    AVLNode *size_tree;
    FileMetadata *files;  // Keep array for iteration if needed
    size_t count;
    size_t capacity;
    // Lightweight optimizations for OS environment
    BloomFilter bloom_filter;  // Fast existence checks
    LRUCache result_cache;     // Query result caching
    MemoryPool mem_pool;
    // Memory tracking
    size_t memory_used;        // Current memory usage in bytes
    size_t memory_limit;       // Maximum allowed memory
    size_t max_files;          // Maximum number of files we can store
} FileIndex;

// Functions for basic queries
int query_init_index(FileIndex *index);
int query_init_index_with_limit(FileIndex *index, size_t memory_limit);
int query_add_file(FileIndex *index, const char *name, const char *path, size_t size, time_t created, time_t modified, const char *type);
int query_by_name(FileIndex *index, const char *name, FileMetadata **results, size_t *result_count);
int query_by_path(FileIndex *index, const char *path, FileMetadata **results, size_t *result_count);
int query_by_size_range(FileIndex *index, size_t min_size, size_t max_size, FileMetadata **results, size_t *result_count);
int query_by_type(FileIndex *index, const char *type, FileMetadata **results, size_t *result_count);
void query_free_index(FileIndex *index);
size_t query_get_memory_usage(FileIndex *index);
int query_can_add_file(FileIndex *index, const char *name, const char *path, const char *type);

// Advanced query function (placeholder for boolean/regex)
int query_advanced(FileIndex *index, const char *query_string, FileMetadata **results, size_t *result_count);

// Trie functions (optimized)
TrieNode* trie_create();
void trie_insert(TrieNode *root, const char *path, FileMetadata *file);
void trie_collect_prefix(TrieNode *node, FileMetadata **results, size_t *result_count);
void trie_free(TrieNode *node);

// AVL tree functions (balanced for O(log n) operations)
AVLNode* avl_create();
void avl_insert(AVLNode **root, size_t size, FileMetadata *file);
void avl_collect_range(AVLNode *node, size_t min_size, size_t max_size, FileMetadata **results, size_t *result_count);
void avl_free(AVLNode *node);
int avl_height(AVLNode *node);
AVLNode* avl_rotate_left(AVLNode *node);
AVLNode* avl_rotate_right(AVLNode *node);
int avl_balance_factor(AVLNode *node);

// FileArray helper functions
void file_array_init(FileArray *arr);
int file_array_add(FileArray *arr, FileMetadata *file);
void file_array_free(FileArray *arr);

// Memory pool functions
void mempool_init(MemoryPool *pool, size_t block_size);
void* mempool_alloc(MemoryPool *pool, size_t size);
void mempool_free(MemoryPool *pool);

// Bloom filter functions
void bloom_init(BloomFilter *filter);
void bloom_add(BloomFilter *filter, const char *key);
int bloom_check(BloomFilter *filter, const char *key);

// LRU cache functions
void lru_init(LRUCache *cache);
FileMetadata** lru_get(LRUCache *cache, const char *key, size_t *count);
void lru_put(LRUCache *cache, const char *key, FileMetadata **results, size_t count);

#endif // QUERY_H
