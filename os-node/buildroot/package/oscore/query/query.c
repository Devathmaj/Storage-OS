#include "query.h"
#include <stdlib.h>
#include <string.h>
#include <stdio.h>
#include <time.h>

static inline size_t mempool_align_size(size_t size) {
    const size_t alignment = sizeof(void*);
    return (size + alignment - 1) & ~(alignment - 1);
}

static char* lru_strdup(const char *src) {
    size_t len = strlen(src) + 1;
    char *copy = malloc(len);
    if (!copy) {
        return NULL;
    }
    memcpy(copy, src, len);
    return copy;
}

static void lru_detach(LRUCache *cache, CacheEntry *entry) {
    if (!entry) {
        return;
    }

    if (entry->prev) {
        entry->prev->next = entry->next;
    } else if (cache->head == entry) {
        cache->head = entry->next;
    }

    if (entry->next) {
        entry->next->prev = entry->prev;
    } else if (cache->tail == entry) {
        cache->tail = entry->prev;
    }

    entry->prev = NULL;
    entry->next = NULL;
}

static void lru_attach_front(LRUCache *cache, CacheEntry *entry) {
    if (!entry) {
        return;
    }

    entry->prev = NULL;
    entry->next = cache->head;
    if (cache->head) {
        cache->head->prev = entry;
    } else {
        cache->tail = entry;
    }
    cache->head = entry;
}

static CacheEntry* lru_find_entry(LRUCache *cache, const char *key) {
    CacheEntry *entry = cache->head;
    while (entry) {
        if (entry->key && strcmp(entry->key, key) == 0) {
            return entry;
        }
        entry = entry->next;
    }
    return NULL;
}

// Memory pool implementation
void mempool_init(MemoryPool *pool, size_t block_size) {
    pool->current = NULL;
    pool->block_size = block_size ? mempool_align_size(block_size) : mempool_align_size(POOL_BLOCK_SIZE);
}

void* mempool_alloc(MemoryPool *pool, size_t size) {
    if (!size) {
        return NULL;
    }

    size = mempool_align_size(size);

    if (!pool->current || pool->current->used + size > pool->current->capacity) {
        size_t capacity = size > pool->block_size ? size : pool->block_size;
        MemBlock *new_block = malloc(sizeof(MemBlock));
        if (!new_block) {
            return NULL;
        }
        new_block->data = malloc(capacity);
        if (!new_block->data) {
            free(new_block);
            return NULL;
        }
        new_block->used = 0;
        new_block->capacity = capacity;
        new_block->next = pool->current;
        pool->current = new_block;
    }

    void *ptr = pool->current->data + pool->current->used;
    pool->current->used += size;
    return ptr;
}

void mempool_free(MemoryPool *pool) {
    MemBlock *block = pool->current;
    while (block) {
        MemBlock *next = block->next;
        free(block->data);
        free(block);
        block = next;
    }
    pool->current = NULL;
}

// Bloom filter implementation (lightweight existence check)
static uint32_t bloom_hash(const char *key, uint32_t seed) {
    uint32_t hash = seed;
    while (*key) {
        hash = hash * 31 + *key++;
    }
    return hash % BLOOM_SIZE;
}

void bloom_init(BloomFilter *filter) {
    memset(filter->bits, 0, sizeof(filter->bits));
}

void bloom_add(BloomFilter *filter, const char *key) {
    for (int i = 0; i < BLOOM_HASHES; i++) {
        uint32_t hash = bloom_hash(key, i * 0x9e3779b9);
        filter->bits[hash / 8] |= (1 << (hash % 8));
    }
}

int bloom_check(BloomFilter *filter, const char *key) {
    for (int i = 0; i < BLOOM_HASHES; i++) {
        uint32_t hash = bloom_hash(key, i * 0x9e3779b9);
        if (!(filter->bits[hash / 8] & (1 << (hash % 8)))) {
            return 0; // Definitely not present
        }
    }
    return 1; // Might be present (false positive possible)
}

// LRU cache implementation (minimal, fixed size)
void lru_init(LRUCache *cache) {
    cache->head = cache->tail = NULL;
    cache->count = 0;
    memset(cache->entries, 0, sizeof(cache->entries));
}

FileMetadata** lru_get(LRUCache *cache, const char *key, size_t *count) {
    CacheEntry *entry = lru_find_entry(cache, key);
    if (entry) {
        *count = entry->count;
        if (entry != cache->head) {
            lru_detach(cache, entry);
            lru_attach_front(cache, entry);
        }
        return entry->results;
    }
    return NULL;
}

void lru_put(LRUCache *cache, const char *key, FileMetadata **results, size_t count) {
    if (!key || !results || !count) {
        return;
    }

    CacheEntry *entry = lru_find_entry(cache, key);

    if (!entry) {
        if (cache->count < CACHE_SIZE) {
            for (int i = 0; i < CACHE_SIZE; i++) {
                if (!cache->entries[i].key) {
                    entry = &cache->entries[i];
                    break;
                }
            }
        } else {
            entry = cache->tail;
        }
    }

    if (!entry) {
        return;
    }

    int had_key = entry->key != NULL;
    if (had_key) {
        lru_detach(cache, entry);
        free(entry->key);
        entry->key = NULL;
        free(entry->results);
        entry->results = NULL;
        if (cache->count > 0) {
            cache->count--;
        }
    }

    char *key_copy = lru_strdup(key);
    if (!key_copy) {
        return;
    }

    FileMetadata **copy = malloc(count * sizeof(FileMetadata*));
    if (!copy) {
        free(key_copy);
        return;
    }
    memcpy(copy, results, count * sizeof(FileMetadata*));

    entry->key = key_copy;
    entry->results = copy;
    entry->count = count;
    lru_attach_front(cache, entry);
    if (cache->count < CACHE_SIZE) {
        cache->count++;
    }
}

// FileArray helper functions
void file_array_init(FileArray *arr) {
    arr->files = NULL;
    arr->count = 0;
    arr->capacity = 0;
}

int file_array_add(FileArray *arr, FileMetadata *file) {
    if (arr->count >= arr->capacity) {
        size_t new_capacity = arr->capacity == 0 ? 4 : arr->capacity * 2;
        FileMetadata **new_files = realloc(arr->files, new_capacity * sizeof(FileMetadata*));
        if (!new_files) return -1;
        arr->files = new_files;
        arr->capacity = new_capacity;
    }
    arr->files[arr->count++] = file;
    return 0;
}

void file_array_free(FileArray *arr) {
    free(arr->files);
    arr->files = NULL;
    arr->count = 0;
    arr->capacity = 0;
}

int query_init_index(FileIndex *index) {
    return query_init_index_with_limit(index, MAX_INDEX_MEMORY);
}

int query_init_index_with_limit(FileIndex *index, size_t memory_limit) {
    if (!index) {
        return -1;
    }
    
    // Clamp memory limit between min and max
    if (memory_limit < MIN_INDEX_MEMORY) {
        memory_limit = MIN_INDEX_MEMORY;
    }
    if (memory_limit > MAX_INDEX_MEMORY) {
        memory_limit = MAX_INDEX_MEMORY;
    }
    
    index->name_index = kh_init(str_list);
    index->type_index = kh_init(str_list);
    index->path_trie = trie_create();
    index->size_tree = avl_create();
    index->files = NULL;
    index->count = 0;
    index->capacity = 0;
    index->memory_used = 0;
    index->memory_limit = memory_limit;
    
    // Calculate max files based on memory limit
    // Average per file: name(~50) + path(~100) + type(~10) + overhead(~128) = ~288 bytes
    index->max_files = memory_limit / 288;
    
    // Initialize lightweight optimizations
    bloom_init(&index->bloom_filter);
    lru_init(&index->result_cache);
    mempool_init(&index->mem_pool, 64 * 1024); // 64KB blocks
    
    // Account for base structure memory
    index->memory_used = sizeof(FileIndex) + sizeof(BloomFilter) + sizeof(LRUCache);
    
    return 0;
}

// Get current memory usage
size_t query_get_memory_usage(FileIndex *index) {
    if (!index) {
        return 0;
    }
    return index->memory_used;
}

// Check if we can add a file without exceeding memory limit
int query_can_add_file(FileIndex *index, const char *name, const char *path, const char *type) {
    if (!index || !name || !path || !type) {
        return 0;
    }
    
    // Estimate memory needed for this file
    size_t estimated = strlen(name) + strlen(path) + strlen(type) + 3 + METADATA_OVERHEAD;
    
    // Check if adding this file would exceed limit
    if (index->memory_used + estimated > index->memory_limit) {
        return 0;
    }
    
    // Check if we've reached max file count
    if (index->count >= index->max_files) {
        return 0;
    }
    
    return 1;
}

// Add a file to the index (optimized with intern strings for type)
int query_add_file(FileIndex *index, const char *name, const char *path, size_t size, time_t created, time_t modified, const char *type) {
    if (!index || !name || !path || !type) {
        return -1;
    }
    
    // Check memory limit before adding
    if (!query_can_add_file(index, name, path, type)) {
        return -2;  // Memory limit exceeded
    }

    if (index->count >= index->capacity) {
        size_t new_capacity = index->capacity == 0 ? 64 : index->capacity * 2;  // Start larger
        FileMetadata *new_files = realloc(index->files, new_capacity * sizeof(FileMetadata));
        if (!new_files) {
            return -1;
        }
        index->files = new_files;
        index->capacity = new_capacity;
    }

    FileMetadata *file = &index->files[index->count];

    // Use memory pool for strings to reduce malloc overhead
    size_t name_len = strlen(name) + 1;
    size_t path_len = strlen(path) + 1;
    size_t type_len = strlen(type) + 1;

    file->name = mempool_alloc(&index->mem_pool, name_len);
    file->path = mempool_alloc(&index->mem_pool, path_len);
    file->type = mempool_alloc(&index->mem_pool, type_len);

    if (!file->name || !file->path || !file->type) {
        return -1;
    }

    memcpy(file->name, name, name_len);
    memcpy(file->path, path, path_len);
    memcpy(file->type, type, type_len);

    file->size = size;
    file->created = created;
    file->modified = modified;
    
    // Update memory usage
    index->memory_used += name_len + path_len + type_len + sizeof(FileMetadata);

    int absent;
    khint_t k = kh_put(str_list, index->name_index, file->name, &absent);
    if (absent < 0) {
        return -1;
    }
    FileArray *name_bucket = (FileArray*)kh_value(index->name_index, k);
    if (!name_bucket) {
        name_bucket = mempool_alloc(&index->mem_pool, sizeof(FileArray));
        if (!name_bucket) {
            return -1;
        }
        file_array_init(name_bucket);
        kh_value(index->name_index, k) = name_bucket;
    }
    if (file_array_add(name_bucket, file) != 0) {
        return -1;
    }

    k = kh_put(str_list, index->type_index, file->type, &absent);
    if (absent < 0) {
        return -1;
    }
    FileArray *type_bucket = (FileArray*)kh_value(index->type_index, k);
    if (!type_bucket) {
        type_bucket = mempool_alloc(&index->mem_pool, sizeof(FileArray));
        if (!type_bucket) {
            return -1;
        }
        file_array_init(type_bucket);
        kh_value(index->type_index, k) = type_bucket;
    }
    if (file_array_add(type_bucket, file) != 0) {
        return -1;
    }

    // Add to path trie
    trie_insert(index->path_trie, file->path, file);

    // Add to AVL tree (auto-balancing)
    avl_insert(&index->size_tree, file->size, file);

    // Add to bloom filter for fast existence checks
    bloom_add(&index->bloom_filter, file->name);

    index->count++;
    return 0;
}

// Query by name (using hash)
int query_by_name(FileIndex *index, const char *name, FileMetadata **results, size_t *result_count) {
    if (!index || !name || !results || !result_count) {
        return -1;
    }

    // Check bloom filter first (fast negative)
    if (!bloom_check(&index->bloom_filter, name)) {
        *result_count = 0;
        return 0;
    }
    
    // Check LRU cache
    FileMetadata **cached = lru_get(&index->result_cache, name, result_count);
    if (cached) {
        memcpy(results, cached, *result_count * sizeof(FileMetadata*));
        return 0;
    }
    
    // Perform actual query
    *result_count = 0;
    khint_t k = kh_get(str_list, index->name_index, name);
    if (k != kh_end(index->name_index)) {
        FileArray *bucket = (FileArray*)kh_value(index->name_index, k);
        if (bucket && bucket->count) {
            memcpy(results, bucket->files, bucket->count * sizeof(FileMetadata*));
            *result_count = bucket->count;
            lru_put(&index->result_cache, name, results, *result_count);
        }
    }
    return 0;
}

// Query by path (using trie)
int query_by_path(FileIndex *index, const char *path, FileMetadata **results, size_t *result_count) {
    if (!index || !path || !results || !result_count) {
        return -1;
    }

    *result_count = 0;
    TrieNode *node = index->path_trie;
    for (size_t i = 0; path[i] && node; i++) {
        node = node->children[(unsigned char)path[i]];
    }
    if (node) {
        trie_collect_prefix(node, results, result_count);
    }
    return 0;
}

// Query by size range (using balanced AVL tree)
int query_by_size_range(FileIndex *index, size_t min_size, size_t max_size, FileMetadata **results, size_t *result_count) {
    if (!index || !results || !result_count) {
        return -1;
    }

    *result_count = 0;
    avl_collect_range(index->size_tree, min_size, max_size, results, result_count);
    return 0;
}

// Query by type (using hash)
int query_by_type(FileIndex *index, const char *type, FileMetadata **results, size_t *result_count) {
    if (!index || !type || !results || !result_count) {
        return -1;
    }

    *result_count = 0;
    khint_t k = kh_get(str_list, index->type_index, type);
    if (k != kh_end(index->type_index)) {
        FileArray *bucket = (FileArray*)kh_value(index->type_index, k);
        if (bucket && bucket->count) {
            memcpy(results, bucket->files, bucket->count * sizeof(FileMetadata*));
            *result_count = bucket->count;
        }
    }
    return 0;
}

// Free the index
void query_free_index(FileIndex *index) {
    // Free hash lists
    khint_t k;
    for (k = 0; k < kh_end(index->name_index); ++k) {
        if (kh_exist(index->name_index, k)) {
            FileArray *bucket = (FileArray*)kh_value(index->name_index, k);
            if (bucket) {
                file_array_free(bucket);
            }
        }
    }
    kh_destroy(str_list, index->name_index);

    for (k = 0; k < kh_end(index->type_index); ++k) {
        if (kh_exist(index->type_index, k)) {
            FileArray *bucket = (FileArray*)kh_value(index->type_index, k);
            if (bucket) {
                file_array_free(bucket);
            }
        }
    }
    kh_destroy(str_list, index->type_index);

    // Free trie and AVL tree
    trie_free(index->path_trie);
    avl_free(index->size_tree);

    // Free LRU cache entries
    for (int i = 0; i < CACHE_SIZE; i++) {
        if (index->result_cache.entries[i].key) {
            free(index->result_cache.entries[i].key);
            free(index->result_cache.entries[i].results);
            index->result_cache.entries[i].key = NULL;
            index->result_cache.entries[i].results = NULL;
            index->result_cache.entries[i].count = 0;
            index->result_cache.entries[i].prev = NULL;
            index->result_cache.entries[i].next = NULL;
        }
    }
    index->result_cache.head = NULL;
    index->result_cache.tail = NULL;
    index->result_cache.count = 0;

    // Free array
    free(index->files);
    index->files = NULL;
    index->count = 0;
    index->capacity = 0;
    
    // Free memory pool
    mempool_free(&index->mem_pool);
}

// Trie functions (optimized with FileArray)
TrieNode* trie_create() {
    TrieNode *node = calloc(1, sizeof(TrieNode));
    file_array_init(&node->files);
    node->has_files = 0;
    return node;
}

void trie_insert(TrieNode *root, const char *path, FileMetadata *file) {
    if (!root || !file) {
        return;
    }

    TrieNode *node = root;
    if (file_array_add(&node->files, file) != 0) {
        return;
    }
    node->has_files = 1;

    if (!path) {
        return;
    }

    for (size_t i = 0; path[i]; i++) {
        unsigned char c = (unsigned char)path[i];
        if (!node->children[c]) {
            node->children[c] = trie_create();
        }
        node = node->children[c];
        if (file_array_add(&node->files, file) != 0) {
            return;
        }
        node->has_files = 1;
    }
}

void trie_collect_prefix(TrieNode *node, FileMetadata **results, size_t *result_count) {
    if (!node || !node->has_files || !results || !result_count) {
        return;
    }

    if (node->files.count) {
        memcpy(&results[*result_count], node->files.files, node->files.count * sizeof(FileMetadata*));
        *result_count += node->files.count;
    }
}

void trie_free(TrieNode *node) {
    if (!node) return;
    for (int i = 0; i < 256; i++) {
        if (node->children[i]) {
            trie_free(node->children[i]);
        }
    }
    file_array_free(&node->files);
    free(node);
}

// AVL tree functions (balanced BST for O(log n) operations)
AVLNode* avl_create() {
    return NULL;
}

int avl_height(AVLNode *node) {
    return node ? node->height : 0;
}

int avl_balance_factor(AVLNode *node) {
    return node ? avl_height(node->left) - avl_height(node->right) : 0;
}

AVLNode* avl_rotate_right(AVLNode *y) {
    AVLNode *x = y->left;
    AVLNode *T2 = x->right;
    
    x->right = y;
    y->left = T2;
    
    y->height = 1 + (avl_height(y->left) > avl_height(y->right) ? avl_height(y->left) : avl_height(y->right));
    x->height = 1 + (avl_height(x->left) > avl_height(x->right) ? avl_height(x->left) : avl_height(x->right));
    
    return x;
}

AVLNode* avl_rotate_left(AVLNode *x) {
    AVLNode *y = x->right;
    AVLNode *T2 = y->left;
    
    y->left = x;
    x->right = T2;
    
    x->height = 1 + (avl_height(x->left) > avl_height(x->right) ? avl_height(x->left) : avl_height(x->right));
    y->height = 1 + (avl_height(y->left) > avl_height(y->right) ? avl_height(y->left) : avl_height(y->right));
    
    return y;
}

void avl_insert(AVLNode **root, size_t size, FileMetadata *file) {
    if (!*root) {
        *root = malloc(sizeof(AVLNode));
        (*root)->size = size;
        file_array_init(&(*root)->files);
        file_array_add(&(*root)->files, file);
        (*root)->left = (*root)->right = NULL;
        (*root)->height = 1;
        return;
    }
    
    if (size < (*root)->size) {
        avl_insert(&(*root)->left, size, file);
    } else if (size > (*root)->size) {
        avl_insert(&(*root)->right, size, file);
    } else {
        file_array_add(&(*root)->files, file);
        return;
    }
    
    // Update height
    (*root)->height = 1 + (avl_height((*root)->left) > avl_height((*root)->right) ? 
                           avl_height((*root)->left) : avl_height((*root)->right));
    
    // Balance the tree
    int balance = avl_balance_factor(*root);
    
    // Left Left Case
    if (balance > 1 && size < (*root)->left->size) {
        *root = avl_rotate_right(*root);
        return;
    }
    
    // Right Right Case
    if (balance < -1 && size > (*root)->right->size) {
        *root = avl_rotate_left(*root);
        return;
    }
    
    // Left Right Case
    if (balance > 1 && size > (*root)->left->size) {
        (*root)->left = avl_rotate_left((*root)->left);
        *root = avl_rotate_right(*root);
        return;
    }
    
    // Right Left Case
    if (balance < -1 && size < (*root)->right->size) {
        (*root)->right = avl_rotate_right((*root)->right);
        *root = avl_rotate_left(*root);
        return;
    }
}

void avl_collect_range(AVLNode *node, size_t min_size, size_t max_size, FileMetadata **results, size_t *result_count) {
    if (!node) return;

    if (node->size > min_size) {
        avl_collect_range(node->left, min_size, max_size, results, result_count);
    }

    if (node->size >= min_size && node->size <= max_size) {
        for (size_t i = 0; i < node->files.count; i++) {
            results[(*result_count)++] = node->files.files[i];
        }
    }

    if (node->size < max_size) {
        avl_collect_range(node->right, min_size, max_size, results, result_count);
    }
}

void avl_free(AVLNode *node) {
    if (!node) return;
    avl_free(node->left);
    avl_free(node->right);
    file_array_free(&node->files);
    free(node);
}