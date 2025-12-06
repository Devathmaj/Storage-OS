package api

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"storageos/controller/services/cache"
)

// FileProxyHandler handles file fetch requests from OS instances
type FileProxyHandler struct {
	cache      *cache.MetadataCache
	httpClient *http.Client
}

// NewFileProxyHandler creates a new file proxy handler
func NewFileProxyHandler() *FileProxyHandler {
	return &FileProxyHandler{
		cache: cache.GetCache(),
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// HandleGetFile fetches a file from the OS instance
// GET /api/files/{file_id}
func (h *FileProxyHandler) HandleGetFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract file ID from path
	fileID := r.URL.Path[len("/api/files/"):]
	if fileID == "" {
		http.Error(w, "File ID is required", http.StatusBadRequest)
		return
	}

	// Get OS ID from cache
	osID, ok := h.cache.GetOSID(fileID)
	if !ok {
		http.Error(w, "File not found in cache", http.StatusNotFound)
		return
	}

	// Get OS instance URL (from database or config)
	osURL, err := h.getOSURL(osID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get OS URL: %v", err), http.StatusInternalServerError)
		return
	}

	// Fetch file from OS
	fileURL := fmt.Sprintf("%s/api/files/%s", osURL, fileID)
	resp, err := h.httpClient.Get(fileURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch file from OS: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("OS returned status %d", resp.StatusCode), resp.StatusCode)
		return
	}

	// Copy headers
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}

	// Stream file content
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// HandleDownloadFile downloads a file with proper headers
// GET /api/files/{file_id}/download
func (h *FileProxyHandler) HandleDownloadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract file ID
	path := r.URL.Path
	fileID := path[len("/api/files/"):]
	if len(fileID) > 9 && fileID[len(fileID)-9:] == "/download" {
		fileID = fileID[:len(fileID)-9]
	}

	if fileID == "" {
		http.Error(w, "File ID is required", http.StatusBadRequest)
		return
	}

	// Get OS ID from cache
	osID, ok := h.cache.GetOSID(fileID)
	if !ok {
		http.Error(w, "File not found in cache", http.StatusNotFound)
		return
	}

	// Get OS instance URL
	osURL, err := h.getOSURL(osID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get OS URL: %v", err), http.StatusInternalServerError)
		return
	}

	// Fetch file from OS
	fileURL := fmt.Sprintf("%s/api/files/%s/download", osURL, fileID)
	resp, err := h.httpClient.Get(fileURL)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to fetch file from OS: %v", err), http.StatusInternalServerError)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, fmt.Sprintf("OS returned status %d", resp.StatusCode), resp.StatusCode)
		return
	}

	// Set download headers
	w.Header().Set("Content-Type", "application/octet-stream")
	if contentDisposition := resp.Header.Get("Content-Disposition"); contentDisposition != "" {
		w.Header().Set("Content-Disposition", contentDisposition)
	}
	if contentLength := resp.Header.Get("Content-Length"); contentLength != "" {
		w.Header().Set("Content-Length", contentLength)
	}

	// Stream file content
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// HandleFileInfo returns file metadata from cache
// GET /api/files/{file_id}/info
func (h *FileProxyHandler) HandleFileInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract file ID
	path := r.URL.Path
	fileID := path[len("/api/files/"):]
	if len(fileID) > 5 && fileID[len(fileID)-5:] == "/info" {
		fileID = fileID[:len(fileID)-5]
	}

	if fileID == "" {
		http.Error(w, "File ID is required", http.StatusBadRequest)
		return
	}

	// Search cache for this file
	result, err := h.cache.SearchByName(fileID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if result.TotalCount == 0 {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result.Files[0])
}

// HandleBulkFetch fetches multiple files metadata
// POST /api/files/bulk
// Body: {"file_ids": ["id1", "id2", ...]}
func (h *FileProxyHandler) HandleBulkFetch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		FileIDs []string `json:"file_ids"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	results := make(map[string]interface{})

	for _, fileID := range req.FileIDs {
		result, err := h.cache.SearchByName(fileID)
		if err != nil || result.TotalCount == 0 {
			results[fileID] = map[string]string{"error": "not found"}
		} else {
			results[fileID] = result.Files[0]
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}

// getOSURL retrieves the URL for an OS instance
// TODO: Implement proper OS registry/database lookup
func (h *FileProxyHandler) getOSURL(osID string) (string, error) {
	// For now, return a placeholder
	// In production, this should query your OS registry database
	// or configuration to get the actual Cloudflare tunnel URL
	
	// Example: https://os-001.example.com
	return fmt.Sprintf("https://%s.example.com", osID), nil
}

// RegisterRoutes registers all file proxy routes
func (h *FileProxyHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/files/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		
		if len(path) > 14 && path[len(path)-9:] == "/download" {
			h.HandleDownloadFile(w, r)
		} else if len(path) > 10 && path[len(path)-5:] == "/info" {
			h.HandleFileInfo(w, r)
		} else if path == "/api/files/bulk" {
			h.HandleBulkFetch(w, r)
		} else {
			h.HandleGetFile(w, r)
		}
	})
}
