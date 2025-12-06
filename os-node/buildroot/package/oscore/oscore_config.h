#ifndef OSCORE_CONFIG_H
#define OSCORE_CONFIG_H

/*
 * OSCore Configuration
 * 
 * This file contains tunable parameters for the OSCore storage system.
 * Adjust these values based on your hardware and workload.
 */

// ============================================================================
// FILESYSTEM LAYER CONFIGURATION
// ============================================================================

// Write buffer size (default: 1MB)
// Larger buffers reduce syscall overhead but use more memory
// Recommended: 1MB - 4MB
#ifndef FS_WRITE_BUFFER_SIZE
#define FS_WRITE_BUFFER_SIZE (1024 * 1024)  // 1MB
#endif

// Read buffer size for buffered reads (default: 64KB)
#ifndef FS_READ_BUFFER_SIZE
#define FS_READ_BUFFER_SIZE (64 * 1024)  // 64KB
#endif

// Threshold for using mmap instead of read (default: 1MB)
// Files larger than this will use memory-mapped I/O
#ifndef FS_MMAP_THRESHOLD
#define FS_MMAP_THRESHOLD (1024 * 1024)  // 1MB
#endif

// Enable huge pages (2MB pages) for better TLB performance
// Requires: echo 256 > /proc/sys/vm/nr_hugepages
#ifndef FS_USE_HUGE_PAGES
#define FS_USE_HUGE_PAGES 1  // 1=enabled, 0=disabled
#endif

// Enable async I/O operations
#ifndef FS_USE_ASYNC_IO
#define FS_USE_ASYNC_IO 1  // 1=enabled, 0=disabled
#endif

// ============================================================================
// QUERY ENGINE CONFIGURATION
// ============================================================================

// LRU cache size (default: 256 entries)
// Larger cache = better hit rate but more memory
#ifndef QUERY_CACHE_SIZE
#define QUERY_CACHE_SIZE 256
#endif

// Bloom filter size in bits (default: 64KB = 524,288 bits)
// Larger filter = fewer false positives but more memory
#ifndef QUERY_BLOOM_SIZE
#define QUERY_BLOOM_SIZE (64 * 1024)  // 64KB bit array
#endif

// Number of hash functions for bloom filter (default: 3)
// More hashes = fewer false positives but slower
#ifndef QUERY_BLOOM_HASHES
#define QUERY_BLOOM_HASHES 3
#endif

// Memory pool block size (default: 64KB)
// Larger blocks reduce allocation overhead
#ifndef QUERY_POOL_BLOCK_SIZE
#define QUERY_POOL_BLOCK_SIZE (64 * 1024)  // 64KB
#endif

// Initial file array capacity (default: 64)
// How many files to pre-allocate space for
#ifndef QUERY_INITIAL_CAPACITY
#define QUERY_INITIAL_CAPACITY 64
#endif

// ============================================================================
// INTEGRATED STORAGE CONFIGURATION
// ============================================================================

// Maximum concurrent file handles (default: 64)
// More handles = better concurrency but more memory
#ifndef STORAGE_MAX_HANDLES
#define STORAGE_MAX_HANDLES 64
#endif

// Default storage directory (can be overridden at runtime)
#ifndef STORAGE_DEFAULT_DIR
#define STORAGE_DEFAULT_DIR "/data/oscore"
#endif

// Enable automatic compression for large files
// Requires zlib or similar compression library
#ifndef STORAGE_AUTO_COMPRESS
#define STORAGE_AUTO_COMPRESS 0  // Not implemented yet
#endif

// Compression threshold (default: 10MB)
#ifndef STORAGE_COMPRESS_THRESHOLD
#define STORAGE_COMPRESS_THRESHOLD (10 * 1024 * 1024)
#endif

// ============================================================================
// PERFORMANCE TUNING
// ============================================================================

// Enable performance monitoring and statistics
#ifndef OSCORE_ENABLE_STATS
#define OSCORE_ENABLE_STATS 1  // 1=enabled, 0=disabled
#endif

// Enable debug logging
#ifndef OSCORE_DEBUG
#define OSCORE_DEBUG 0  // 1=enabled, 0=disabled
#endif

// Thread safety (not implemented yet)
#ifndef OSCORE_THREAD_SAFE
#define OSCORE_THREAD_SAFE 0  // 1=enabled, 0=disabled
#endif

// ============================================================================
// HARDWARE-SPECIFIC OPTIMIZATIONS
// ============================================================================

// CPU cache line size (default: 64 bytes)
// Use 128 for some ARM processors
#ifndef CACHE_LINE_SIZE
#define CACHE_LINE_SIZE 64
#endif

// Enable CPU-specific optimizations
// -march=native is already in Makefile
#ifndef USE_CPU_FEATURES
#define USE_CPU_FEATURES 1  // Use SSE/AVX if available
#endif

// ============================================================================
// LIMITS
// ============================================================================

// Maximum filename length
#ifndef MAX_FILENAME_LENGTH
#define MAX_FILENAME_LENGTH 255
#endif

// Maximum path length
#ifndef MAX_PATH_LENGTH
#define MAX_PATH_LENGTH 4096
#endif

// Maximum file size (0 = unlimited)
#ifndef MAX_FILE_SIZE
#define MAX_FILE_SIZE 0
#endif

// Maximum number of files in index (0 = unlimited)
#ifndef MAX_FILES
#define MAX_FILES 0
#endif

#endif // OSCORE_CONFIG_H
