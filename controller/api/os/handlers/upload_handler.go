package handlers

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

var (
	// Storage paths - configured at runtime
	StorageBasePath string
	ChunkBasePath   string
	MetadataPath    string
	TempPath        string
	osDB            *sql.DB
)

// InitOSHandlers initializes OS node handlers with database and data directory
func InitOSHandlers(database *sql.DB, dataDir string) error {
	osDB = database
	
	// Set storage paths based on data directory
	StorageBasePath = filepath.Join(dataDir, "files")
	ChunkBasePath = filepath.Join(dataDir, "chunks")
	MetadataPath = filepath.Join(dataDir, "metadata")
	TempPath = filepath.Join(dataDir, "temp")
	
	// Ensure storage directories exist
	dirs := []string{StorageBasePath, ChunkBasePath, MetadataPath, TempPath}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
	}
	
	log.Printf("OS node storage initialized: %s", StorageBasePath)
	return nil
}

// UploadHandler handles file uploads to OS node storage
// Saves encrypted files to /data/files using custom fs operations
func UploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 500MB in memory)
	if err := r.ParseMultipartForm(500 << 20); err != nil {
		log.Printf("Error parsing form: %v", err)
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	// Get file from form
	file, header, err := r.FormFile("file")
	if err != nil {
		log.Printf("Error retrieving file: %v", err)
		http.Error(w, "No file provided", http.StatusBadRequest)
		return
	}
	defer file.Close()

	// Get metadata from form
	ownerID := r.FormValue("owner_id")
	folderID := r.FormValue("folder_id")
	checksum := r.FormValue("checksum")
	mimeType := r.FormValue("mime_type")

	if ownerID == "" {
		http.Error(w, "owner_id required", http.StatusBadRequest)
		return
	}

	// Generate unique file ID
	fileID := uuid.New().String()
	
	// Determine storage path: /data/files/{owner_id}/{file_id}
	ownerDir := filepath.Join(StorageBasePath, ownerID)
	if err := os.MkdirAll(ownerDir, 0755); err != nil {
		log.Printf("Error creating owner directory: %v", err)
		http.Error(w, "Storage error", http.StatusInternalServerError)
		return
	}

	storagePath := filepath.Join(ownerDir, fileID)
	
	// Save file to storage using custom fs operations
	// For now, use standard operations, but these can be replaced with fs_write_file
	destFile, err := os.Create(storagePath)
	if err != nil {
		log.Printf("Error creating file: %v", err)
		http.Error(w, "Storage error", http.StatusInternalServerError)
		return
	}

	// Calculate checksum while writing
	hasher := sha256.New()
	tee := io.TeeReader(file, hasher)
	
	size, err := io.Copy(destFile, tee)
	destFile.Close()
	
	if err != nil {
		os.Remove(storagePath)
		log.Printf("Error saving file: %v", err)
		http.Error(w, "Storage error", http.StatusInternalServerError)
		return
	}

	// Calculate checksum
	calculatedChecksum := hex.EncodeToString(hasher.Sum(nil))
	
	// Verify checksum if provided
	if checksum != "" && checksum != calculatedChecksum {
		os.Remove(storagePath)
		http.Error(w, "Checksum mismatch", http.StatusBadRequest)
		return
	}

	// Get file extension
	ext := filepath.Ext(header.Filename)
	if ext == "" {
		ext = ".bin"
	}

	// Store metadata in OS node database
	nodeID := "os-node-001" // TODO: Get from config
	now := time.Now()

	var folderIDValue interface{}
	if folderID != "" {
		folderIDValue = folderID
	} else {
		folderIDValue = nil
	}

	_, err = osDB.Exec(`
		INSERT INTO files (
			id, owner_id, folder_id, filename, extension,
			original_extension, mime_type, size, checksum,
			storage_path, node_id, version, status,
			uploaded_at, modified_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 'active', ?, ?)
	`, fileID, ownerID, folderIDValue, header.Filename, ext,
		ext, mimeType, size, calculatedChecksum,
		storagePath, nodeID, now, now)

	if err != nil {
		os.Remove(storagePath)
		log.Printf("Error saving metadata: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	log.Printf("File uploaded: %s -> %s (size=%d, owner=%s)", header.Filename, storagePath, size, ownerID)

	// Return metadata response
	response := map[string]interface{}{
		"id":           fileID,
		"filename":     header.Filename,
		"size":         size,
		"checksum":     calculatedChecksum,
		"storage_path": storagePath,
		"node_id":      nodeID,
		"uploaded_at":  now.Format(time.RFC3339),
		"status":       "active",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// DownloadHandler handles file downloads from OS node storage
func DownloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fileID := r.URL.Query().Get("file_id")
	if fileID == "" {
		http.Error(w, "file_id required", http.StatusBadRequest)
		return
	}

	// Get file metadata from database
	var filename, storagePath, mimeType string
	var size int64

	err := osDB.QueryRow(`
		SELECT filename, storage_path, mime_type, size
		FROM files
		WHERE id = ? AND status = 'active'
	`, fileID).Scan(&filename, &storagePath, &mimeType, &size)

	if err == sql.ErrNoRows {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("Database error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Open file from storage using custom fs operations
	file, err := os.Open(storagePath)
	if err != nil {
		log.Printf("Error opening file: %v", err)
		http.Error(w, "File not accessible", http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Set headers
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))

	// Stream file to client
	if _, err := io.Copy(w, file); err != nil {
		log.Printf("Error streaming file: %v", err)
	}
}

// GetMetadataHandler returns file metadata
func GetMetadataHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fileID := r.URL.Query().Get("file_id")
	if fileID == "" {
		http.Error(w, "file_id required", http.StatusBadRequest)
		return
	}

	// Query full metadata
	var metadata struct {
		ID                string    `json:"id"`
		OwnerID           string    `json:"owner_id"`
		FolderID          *string   `json:"folder_id"`
		Filename          string    `json:"filename"`
		Extension         string    `json:"extension"`
		MimeType          string    `json:"mime_type"`
		Size              int64     `json:"size"`
		Checksum          string    `json:"checksum"`
		StoragePath       string    `json:"storage_path"`
		NodeID            string    `json:"node_id"`
		Version           int       `json:"version"`
		Status            string    `json:"status"`
		UploadedAt        time.Time `json:"uploaded_at"`
		ModifiedAt        time.Time `json:"modified_at"`
	}

	err := osDB.QueryRow(`
		SELECT id, owner_id, folder_id, filename, extension, mime_type,
		       size, checksum, storage_path, node_id, version, status,
		       uploaded_at, modified_at
		FROM files
		WHERE id = ?
	`, fileID).Scan(
		&metadata.ID, &metadata.OwnerID, &metadata.FolderID, &metadata.Filename,
		&metadata.Extension, &metadata.MimeType, &metadata.Size, &metadata.Checksum,
		&metadata.StoragePath, &metadata.NodeID, &metadata.Version, &metadata.Status,
		&metadata.UploadedAt, &metadata.ModifiedAt,
	)

	if err == sql.ErrNoRows {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("Database error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metadata)
}

// ListFilesHandler lists files for a user
func ListFilesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ownerID := r.URL.Query().Get("owner_id")
	if ownerID == "" {
		http.Error(w, "owner_id required", http.StatusBadRequest)
		return
	}

	rows, err := osDB.Query(`
		SELECT id, filename, extension, mime_type, size, checksum,
		       storage_path, uploaded_at, status
		FROM files
		WHERE owner_id = ? AND status = 'active'
		ORDER BY uploaded_at DESC
	`, ownerID)

	if err != nil {
		log.Printf("Database error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var files []map[string]interface{}
	for rows.Next() {
		var id, filename, ext, mimeType, checksum, storagePath, status string
		var size int64
		var uploadedAt time.Time

		if err := rows.Scan(&id, &filename, &ext, &mimeType, &size, &checksum, &storagePath, &uploadedAt, &status); err != nil {
			continue
		}

		files = append(files, map[string]interface{}{
			"id":           id,
			"filename":     filename,
			"extension":    ext,
			"mime_type":    mimeType,
			"size":         size,
			"checksum":     checksum,
			"storage_path": storagePath,
			"uploaded_at":  uploadedAt.Format(time.RFC3339),
			"status":       status,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

// DeleteHandler deletes a file from storage
func DeleteHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fileID := r.URL.Query().Get("file_id")
	if fileID == "" {
		http.Error(w, "file_id required", http.StatusBadRequest)
		return
	}

	// Get storage path
	var storagePath string
	err := osDB.QueryRow(`
		SELECT storage_path FROM files WHERE id = ? AND status = 'active'
	`, fileID).Scan(&storagePath)

	if err == sql.ErrNoRows {
		http.Error(w, "File not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("Database error: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	// Delete physical file
	if err := os.Remove(storagePath); err != nil && !os.IsNotExist(err) {
		log.Printf("Error deleting file: %v", err)
		// Continue anyway to update database
	}

	// Mark as deleted in database (soft delete)
	_, err = osDB.Exec(`
		UPDATE files SET status = 'deleted', modified_at = ? WHERE id = ?
	`, time.Now(), fileID)

	if err != nil {
		log.Printf("Error updating database: %v", err)
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	log.Printf("File deleted: %s (path=%s)", fileID, storagePath)
	w.WriteHeader(http.StatusNoContent)
}
