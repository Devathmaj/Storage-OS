package cache

/*
#cgo CFLAGS: -I${SRCDIR}/../../../os-node/buildroot/package/oscore/query
#cgo LDFLAGS: -L${SRCDIR}/../../../os-node/buildroot/package/oscore/query -lquery -lm

#include "query.h"
#include <stdlib.h>
#include <string.h>
*/
import "C"
import (
	"fmt"
	"sync"
	"time"
	"unsafe"
)

const (
	// Default memory limit: 512MB
	DefaultMemoryLimit = 512 * 1024 * 1024
	// Minimum memory limit: 64MB
	MinMemoryLimit = 64 * 1024 * 1024
	// Maximum memory limit: 1GB
	MaxMemoryLimit = 1024 * 1024 * 1024
)

// FileMetadata represents a file's metadata
type FileMetadata struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Size     uint64    `json:"size"`
	Type     string    `json:"type"`
	Created  time.Time `json:"created"`
	Modified time.Time `json:"modified"`
	OSID     string    `json:"os_id"`
}

// SearchQuery represents a search request
type SearchQuery struct {
	Name     string `json:"name,omitempty"`
	Path     string `json:"path,omitempty"`
	Type     string `json:"type,omitempty"`
	MinSize  uint64 `json:"min_size,omitempty"`
	MaxSize  uint64 `json:"max_size,omitempty"`
	Advanced string `json:"advanced,omitempty"` // Advanced query string
}

// SearchResult represents search results with stats
type SearchResult struct {
	Files        []FileMetadata `json:"files"`
	TotalCount   int            `json:"total_count"`
	QueryTime    int64          `json:"query_time_ms"`
	MemoryUsed   uint64         `json:"memory_used"`
	CacheHit     bool           `json:"cache_hit"`
}

// MetadataCache manages in-memory file metadata using C query engine
type MetadataCache struct {
	mu          sync.RWMutex
	index       *C.FileIndex
	memoryLimit uint64
	osFileMap   map[string]string // file_id -> os_id mapping
	initialized bool
}

var (
	globalCache *MetadataCache
	once        sync.Once
)

// GetCache returns the global metadata cache instance
func GetCache() *MetadataCache {
	once.Do(func() {
		globalCache = NewMetadataCache(DefaultMemoryLimit)
	})
	return globalCache
}

// NewMetadataCache creates a new metadata cache with specified memory limit
func NewMetadataCache(memoryLimit uint64) *MetadataCache {
	if memoryLimit < MinMemoryLimit {
		memoryLimit = MinMemoryLimit
	}
	if memoryLimit > MaxMemoryLimit {
		memoryLimit = MaxMemoryLimit
	}

	cache := &MetadataCache{
		memoryLimit: memoryLimit,
		osFileMap:   make(map[string]string),
	}

	// Initialize C index
	cache.index = (*C.FileIndex)(C.malloc(C.size_t(unsafe.Sizeof(C.FileIndex{}))))
	if cache.index == nil {
		panic("failed to allocate memory for FileIndex")
	}

	ret := C.query_init_index_with_limit(cache.index, C.size_t(memoryLimit))
	if ret != 0 {
		C.free(unsafe.Pointer(cache.index))
		panic("failed to initialize query index")
	}

	cache.initialized = true
	return cache
}

// AddFile adds a file to the cache
func (c *MetadataCache) AddFile(file FileMetadata) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.initialized {
		return fmt.Errorf("cache not initialized")
	}

	// Convert strings to C strings
	cName := C.CString(file.Name)
	cPath := C.CString(file.Path)
	cType := C.CString(file.Type)
	defer C.free(unsafe.Pointer(cName))
	defer C.free(unsafe.Pointer(cPath))
	defer C.free(unsafe.Pointer(cType))

	// Check if we can add this file
	canAdd := C.query_can_add_file(c.index, cName, cPath, cType)
	if canAdd == 0 {
		return fmt.Errorf("cannot add file: memory limit would be exceeded")
	}

	// Add to C index
	ret := C.query_add_file(
		c.index,
		cName,
		cPath,
		C.size_t(file.Size),
		C.time_t(file.Created.Unix()),
		C.time_t(file.Modified.Unix()),
		cType,
	)

	if ret != 0 {
		if ret == -2 {
			return fmt.Errorf("memory limit exceeded")
		}
		return fmt.Errorf("failed to add file to index")
	}

	// Store OS mapping
	c.osFileMap[file.ID] = file.OSID

	return nil
}

// AddFiles bulk adds multiple files
func (c *MetadataCache) AddFiles(files []FileMetadata) (added int, errors []error) {
	for _, file := range files {
		if err := c.AddFile(file); err != nil {
			errors = append(errors, fmt.Errorf("file %s: %v", file.Name, err))
		} else {
			added++
		}
	}
	return
}

// SearchByName searches files by exact name
func (c *MetadataCache) SearchByName(name string) (*SearchResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	start := time.Now()
	
	cName := C.CString(name)
	defer C.free(unsafe.Pointer(cName))

	// Allocate results array
	maxResults := 10000
	results := make([]*C.FileMetadata, maxResults)
	cResults := (**C.FileMetadata)(unsafe.Pointer(&results[0]))
	
	var resultCount C.size_t
	ret := C.query_by_name(c.index, cName, cResults, &resultCount)
	
	if ret != 0 {
		return nil, fmt.Errorf("query failed")
	}

	files := c.convertResults(results[:resultCount])
	
	return &SearchResult{
		Files:      files,
		TotalCount: int(resultCount),
		QueryTime:  time.Since(start).Milliseconds(),
		MemoryUsed: c.GetMemoryUsage(),
		CacheHit:   true,
	}, nil
}

// SearchByPath searches files by path prefix
func (c *MetadataCache) SearchByPath(path string) (*SearchResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	start := time.Now()
	
	cPath := C.CString(path)
	defer C.free(unsafe.Pointer(cPath))

	maxResults := 10000
	results := make([]*C.FileMetadata, maxResults)
	cResults := (**C.FileMetadata)(unsafe.Pointer(&results[0]))
	
	var resultCount C.size_t
	ret := C.query_by_path(c.index, cPath, cResults, &resultCount)
	
	if ret != 0 {
		return nil, fmt.Errorf("query failed")
	}

	files := c.convertResults(results[:resultCount])
	
	return &SearchResult{
		Files:      files,
		TotalCount: int(resultCount),
		QueryTime:  time.Since(start).Milliseconds(),
		MemoryUsed: c.GetMemoryUsage(),
		CacheHit:   true,
	}, nil
}

// SearchByType searches files by type
func (c *MetadataCache) SearchByType(fileType string) (*SearchResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	start := time.Now()
	
	cType := C.CString(fileType)
	defer C.free(unsafe.Pointer(cType))

	maxResults := 10000
	results := make([]*C.FileMetadata, maxResults)
	cResults := (**C.FileMetadata)(unsafe.Pointer(&results[0]))
	
	var resultCount C.size_t
	ret := C.query_by_type(c.index, cType, cResults, &resultCount)
	
	if ret != 0 {
		return nil, fmt.Errorf("query failed")
	}

	files := c.convertResults(results[:resultCount])
	
	return &SearchResult{
		Files:      files,
		TotalCount: int(resultCount),
		QueryTime:  time.Since(start).Milliseconds(),
		MemoryUsed: c.GetMemoryUsage(),
		CacheHit:   true,
	}, nil
}

// SearchBySize searches files by size range
func (c *MetadataCache) SearchBySize(minSize, maxSize uint64) (*SearchResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	start := time.Now()
	
	maxResults := 10000
	results := make([]*C.FileMetadata, maxResults)
	cResults := (**C.FileMetadata)(unsafe.Pointer(&results[0]))
	
	var resultCount C.size_t
	ret := C.query_by_size_range(c.index, C.size_t(minSize), C.size_t(maxSize), cResults, &resultCount)
	
	if ret != 0 {
		return nil, fmt.Errorf("query failed")
	}

	files := c.convertResults(results[:resultCount])
	
	return &SearchResult{
		Files:      files,
		TotalCount: int(resultCount),
		QueryTime:  time.Since(start).Milliseconds(),
		MemoryUsed: c.GetMemoryUsage(),
		CacheHit:   true,
	}, nil
}

// SearchAdvanced performs advanced query with complex conditions
func (c *MetadataCache) SearchAdvanced(queryString string) (*SearchResult, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	start := time.Now()
	
	cQuery := C.CString(queryString)
	defer C.free(unsafe.Pointer(cQuery))

	maxResults := 10000
	results := make([]*C.FileMetadata, maxResults)
	cResults := (**C.FileMetadata)(unsafe.Pointer(&results[0]))
	
	var resultCount C.size_t
	ret := C.query_advanced(c.index, cQuery, cResults, &resultCount)
	
	if ret != 0 {
		return nil, fmt.Errorf("advanced query failed")
	}

	files := c.convertResults(results[:resultCount])
	
	return &SearchResult{
		Files:      files,
		TotalCount: int(resultCount),
		QueryTime:  time.Since(start).Milliseconds(),
		MemoryUsed: c.GetMemoryUsage(),
		CacheHit:   true,
	}, nil
}

// Search performs a combined search using multiple criteria
func (c *MetadataCache) Search(query SearchQuery) (*SearchResult, error) {
	// If advanced query provided, use that
	if query.Advanced != "" {
		return c.SearchAdvanced(query.Advanced)
	}

	// Otherwise, build query based on criteria
	if query.Name != "" {
		return c.SearchByName(query.Name)
	}
	
	if query.Path != "" {
		return c.SearchByPath(query.Path)
	}
	
	if query.Type != "" {
		return c.SearchByType(query.Type)
	}
	
	if query.MinSize > 0 || query.MaxSize > 0 {
		maxSize := query.MaxSize
		if maxSize == 0 {
			maxSize = ^uint64(0) // Max uint64
		}
		return c.SearchBySize(query.MinSize, maxSize)
	}

	return &SearchResult{
		Files:      []FileMetadata{},
		TotalCount: 0,
		QueryTime:  0,
		MemoryUsed: c.GetMemoryUsage(),
	}, nil
}

// GetMemoryUsage returns current memory usage in bytes
func (c *MetadataCache) GetMemoryUsage() uint64 {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	if !c.initialized {
		return 0
	}
	
	return uint64(C.query_get_memory_usage(c.index))
}

// GetStats returns cache statistics
func (c *MetadataCache) GetStats() map[string]interface{} {
	c.mu.RLock()
	defer c.mu.RUnlock()

	stats := map[string]interface{}{
		"memory_used":   c.GetMemoryUsage(),
		"memory_limit":  c.memoryLimit,
		"memory_pct":    float64(c.GetMemoryUsage()) / float64(c.memoryLimit) * 100,
		"file_count":    int(c.index.count),
		"max_files":     int(c.index.max_files),
		"initialized":   c.initialized,
	}

	return stats
}

// GetOSID returns the OS ID for a given file ID
func (c *MetadataCache) GetOSID(fileID string) (string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	
	osID, ok := c.osFileMap[fileID]
	return osID, ok
}

// Clear removes all cached metadata
func (c *MetadataCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		C.query_free_index(c.index)
		C.query_init_index_with_limit(c.index, C.size_t(c.memoryLimit))
		c.osFileMap = make(map[string]string)
	}
}

// Close frees all resources
func (c *MetadataCache) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.initialized {
		C.query_free_index(c.index)
		C.free(unsafe.Pointer(c.index))
		c.initialized = false
	}
}

// convertResults converts C FileMetadata array to Go slice
func (c *MetadataCache) convertResults(cResults []*C.FileMetadata) []FileMetadata {
	files := make([]FileMetadata, 0, len(cResults))
	
	for _, cFile := range cResults {
		if cFile == nil {
			continue
		}
		
		file := FileMetadata{
			Name:     C.GoString(cFile.name),
			Path:     C.GoString(cFile.path),
			Size:     uint64(cFile.size),
			Type:     C.GoString(cFile._type),
			Created:  time.Unix(int64(cFile.created), 0),
			Modified: time.Unix(int64(cFile.modified), 0),
		}
		
		// Generate ID from path + name
		file.ID = fmt.Sprintf("%s/%s", file.Path, file.Name)
		
		// Lookup OS ID
		if osID, ok := c.osFileMap[file.ID]; ok {
			file.OSID = osID
		}
		
		files = append(files, file)
	}
	
	return files
}
