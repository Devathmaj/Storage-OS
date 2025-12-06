package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"storageos/controller/services/cache"
)

// SearchHandler handles search requests
type SearchHandler struct {
	cache *cache.MetadataCache
}

// NewSearchHandler creates a new search handler
func NewSearchHandler() *SearchHandler {
	return &SearchHandler{
		cache: cache.GetCache(),
	}
}

// HandleSearch handles general search requests
// POST /api/search
func (h *SearchHandler) HandleSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var query cache.SearchQuery
	if err := json.NewDecoder(r.Body).Decode(&query); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	result, err := h.cache.Search(query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleSearchByName searches by exact file name
// GET /api/search/name?q=filename
func (h *SearchHandler) HandleSearchByName(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	name := r.URL.Query().Get("q")
	if name == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	result, err := h.cache.SearchByName(name)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleSearchByPath searches by path prefix
// GET /api/search/path?q=/path/to/files
func (h *SearchHandler) HandleSearchByPath(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	path := r.URL.Query().Get("q")
	if path == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	result, err := h.cache.SearchByPath(path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleSearchByType searches by file type
// GET /api/search/type?q=pdf
func (h *SearchHandler) HandleSearchByType(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fileType := r.URL.Query().Get("q")
	if fileType == "" {
		http.Error(w, "Query parameter 'q' is required", http.StatusBadRequest)
		return
	}

	result, err := h.cache.SearchByType(fileType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleSearchBySize searches by size range
// GET /api/search/size?min=1024&max=1048576
func (h *SearchHandler) HandleSearchBySize(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	minStr := r.URL.Query().Get("min")
	maxStr := r.URL.Query().Get("max")

	var minSize, maxSize uint64
	var err error

	if minStr != "" {
		minSize, err = strconv.ParseUint(minStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid min size", http.StatusBadRequest)
			return
		}
	}

	if maxStr != "" {
		maxSize, err = strconv.ParseUint(maxStr, 10, 64)
		if err != nil {
			http.Error(w, "Invalid max size", http.StatusBadRequest)
			return
		}
	} else {
		maxSize = ^uint64(0) // Max uint64
	}

	result, err := h.cache.SearchBySize(minSize, maxSize)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleAdvancedSearch handles advanced query syntax
// POST /api/search/advanced
// Body: {"query": "name~*.pdf AND size>1024000"}
func (h *SearchHandler) HandleAdvancedSearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Query string `json:"query"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Query == "" {
		http.Error(w, "Query string is required", http.StatusBadRequest)
		return
	}

	result, err := h.cache.SearchAdvanced(req.Query)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandleCacheStats returns cache statistics
// GET /api/search/stats
func (h *SearchHandler) HandleCacheStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := h.cache.GetStats()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// HandleLoadMetadata loads metadata from an OS instance
// POST /api/search/load
// Body: {"os_id": "OS-001", "files": [...]}
func (h *SearchHandler) HandleLoadMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		OSID  string                 `json:"os_id"`
		Files []cache.FileMetadata   `json:"files"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Set OS ID for all files
	for i := range req.Files {
		req.Files[i].OSID = req.OSID
	}

	added, errors := h.cache.AddFiles(req.Files)

	response := map[string]interface{}{
		"added":      added,
		"total":      len(req.Files),
		"errors":     len(errors),
		"error_list": errors,
		"stats":      h.cache.GetStats(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// HandleClearCache clears all cached metadata
// POST /api/search/clear
func (h *SearchHandler) HandleClearCache(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	h.cache.Clear()

	response := map[string]interface{}{
		"success": true,
		"message": "Cache cleared successfully",
		"stats":   h.cache.GetStats(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// RegisterRoutes registers all search routes
func (h *SearchHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/search", h.HandleSearch)
	mux.HandleFunc("/api/search/name", h.HandleSearchByName)
	mux.HandleFunc("/api/search/path", h.HandleSearchByPath)
	mux.HandleFunc("/api/search/type", h.HandleSearchByType)
	mux.HandleFunc("/api/search/size", h.HandleSearchBySize)
	mux.HandleFunc("/api/search/advanced", h.HandleAdvancedSearch)
	mux.HandleFunc("/api/search/stats", h.HandleCacheStats)
	mux.HandleFunc("/api/search/load", h.HandleLoadMetadata)
	mux.HandleFunc("/api/search/clear", h.HandleClearCache)
}
