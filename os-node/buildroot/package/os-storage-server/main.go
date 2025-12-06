package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/yaml.v3"
)

// Config represents the server configuration
type Config struct {
	ListenAddr string `yaml:"listen_addr"`
	DataDir    string `yaml:"data_dir"`
	DBPath     string `yaml:"db_path"`
	NodeID     string `yaml:"node_id"`
	NodeURL    string `yaml:"node_url"`
}

// Server handles HTTP requests and database operations
type Server struct {
	config *Config
	db     *sql.DB
}

// FileUploadRequest represents a file upload
type FileUploadRequest struct {
	OwnerID     string `json:"owner_id"`
	FolderID    string `json:"folder_id"`
	Filename    string `json:"filename"`
	MimeType    string `json:"mime_type"`
	Size        int64  `json:"size"`
	Checksum    string `json:"checksum"`
	EncryptedKey []byte `json:"encrypted_key"`
	EphemeralPubKey []byte `json:"ephemeral_pub_key"`
}

// FileMetadata represents stored file metadata (exact schema.sql structure)
type FileMetadata struct {
	ID                string    `json:"id"`
	OwnerID           string    `json:"owner_id"`
	FolderID          string    `json:"folder_id"`
	Filename          string    `json:"filename"`
	Extension         string    `json:"extension"`
	OriginalExtension string    `json:"original_extension"`
	MimeType          string    `json:"mime_type"`
	Size              int64     `json:"size"`
	OriginalSize      int64     `json:"original_size"`
	Checksum          string    `json:"checksum"`
	StoragePath       string    `json:"storage_path"`
	EncryptedKey      []byte    `json:"encrypted_key"`
	EphemeralPubKey   []byte    `json:"ephemeral_pub_key"`
	NodeID            string    `json:"node_id"`
	Version           int       `json:"version"`
	Status            string    `json:"status"`
	IsIndexed         bool      `json:"is_indexed"`
	ParentArchiveID   string    `json:"parent_archive_id"`
	RelativePath      string    `json:"relative_path"`
	UploadedAt        time.Time `json:"uploaded_at"`
	ModifiedAt        time.Time `json:"modified_at"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	return &cfg, nil
}

func initDatabase(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		db.Close()
		return nil, fmt.Errorf("enable WAL: %w", err)
	}

	// Create schema (exact schema from controller/db/schema.sql - only tables needed in OS)
	schema := `
	CREATE TABLE IF NOT EXISTS files (
		id TEXT PRIMARY KEY,
		owner_id TEXT REFERENCES users(id),
		folder_id TEXT REFERENCES folders(id),
		filename TEXT NOT NULL,
		extension TEXT,
		original_extension TEXT,
		mime_type TEXT,
		size BIGINT DEFAULT 0,
		original_size BIGINT DEFAULT 0,
		checksum TEXT,
		storage_path TEXT,
		encrypted_key BLOB,
		ephemeral_pub_key BLOB,
		node_id TEXT,
		version INT DEFAULT 1,
		status TEXT DEFAULT 'active',
		is_indexed BOOLEAN DEFAULT 0,
		parent_archive_id TEXT,
		relative_path TEXT,
		uploaded_at TEXT DEFAULT CURRENT_TIMESTAMP,
		modified_at TEXT DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS folders (
		id TEXT PRIMARY KEY,
		owner_id TEXT,
		parent_id TEXT REFERENCES folders(id),
		name TEXT NOT NULL,
		is_archived BOOLEAN DEFAULT 0,
		archive_file_id TEXT,
		total_files INTEGER DEFAULT 0,
		total_size BIGINT DEFAULT 0,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP
	);

	CREATE TABLE IF NOT EXISTS file_shares (
		id TEXT PRIMARY KEY,
		file_id TEXT REFERENCES files(id),
		owner_id TEXT,
		shared_with TEXT,
		access TEXT DEFAULT 'read',
		expires_at TEXT,
		created_at TEXT DEFAULT CURRENT_TIMESTAMP
	);

	CREATE INDEX IF NOT EXISTS idx_files_owner_id ON files(owner_id);
	CREATE INDEX IF NOT EXISTS idx_files_folder_id ON files(folder_id);
	CREATE INDEX IF NOT EXISTS idx_files_filename ON files(filename);
	CREATE INDEX IF NOT EXISTS idx_files_status ON files(status);
	CREATE INDEX IF NOT EXISTS idx_folders_owner_id ON folders(owner_id);
	CREATE INDEX IF NOT EXISTS idx_folders_parent_id ON folders(parent_id);
	`

	if _, err := db.ExecContext(context.Background(), schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}

	return db, nil
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"node_id": s.config.NodeID,
		"data_dir": s.config.DataDir,
	})
}

// StorageInfo represents storage quota and usage information
type StorageInfo struct {
	QuotaMB     int64  `json:"quota_mb"`
	UsedMB      int64  `json:"used_mb"`
	AvailableMB int64  `json:"available_mb"`
	DiskTotalMB int64  `json:"disk_total_mb"`
	NodeID      string `json:"node_id"`
	Status      string `json:"status"` // healthy, warning, critical
}

// getStorageInfo reads storage quota and usage from storagemgr
func (s *Server) getStorageInfo() (*StorageInfo, error) {
	info := &StorageInfo{
		NodeID: s.config.NodeID,
		Status: "healthy",
	}

	// Read quota from config file
	quotaFile := "/etc/storage-quota.conf"
	if data, err := os.ReadFile(quotaFile); err == nil {
		lines := string(data)
		for _, line := range splitLines(lines) {
			if len(line) > 0 && line[0] != '#' {
				if kv := splitKV(line); len(kv) == 2 {
					switch kv[0] {
					case "QUOTA_LIMIT_MB":
						fmt.Sscanf(kv[1], "%d", &info.QuotaMB)
					case "DISK_SIZE_MB":
						fmt.Sscanf(kv[1], "%d", &info.DiskTotalMB)
					}
				}
			}
		}
	}

	// Get actual usage from df command
	// Parse df -m /data output
	cmd := exec.Command("df", "-m", "/data")
	output, err := cmd.Output()
	if err == nil {
		lines := splitLines(string(output))
		if len(lines) >= 2 {
			fields := splitFields(lines[1])
			if len(fields) >= 4 {
				fmt.Sscanf(fields[0], "%d", &info.DiskTotalMB) // Total
				fmt.Sscanf(fields[1], "%d", &info.UsedMB)      // Used
				fmt.Sscanf(fields[2], "%d", &info.AvailableMB) // Available
			}
		}
	}

	// Calculate available within quota
	quotaRemaining := info.QuotaMB - info.UsedMB
	if quotaRemaining < 0 {
		quotaRemaining = 0
	}
	// Available is minimum of quota remaining and disk available
	if quotaRemaining < info.AvailableMB {
		info.AvailableMB = quotaRemaining
	}

	// Set status based on usage
	usagePercent := float64(0)
	if info.QuotaMB > 0 {
		usagePercent = float64(info.UsedMB) / float64(info.QuotaMB) * 100
	}
	if usagePercent >= 95 || info.AvailableMB < 10 {
		info.Status = "critical"
	} else if usagePercent >= 80 {
		info.Status = "warning"
	}

	return info, nil
}

// splitLines splits string by newlines
func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			line := s[start:i]
			if len(line) > 0 && line[len(line)-1] == '\r' {
				line = line[:len(line)-1]
			}
			lines = append(lines, line)
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}

// splitKV splits key=value
func splitKV(s string) []string {
	for i := 0; i < len(s); i++ {
		if s[i] == '=' {
			return []string{s[:i], s[i+1:]}
		}
	}
	return []string{s}
}

// splitFields splits by whitespace
func splitFields(s string) []string {
	var fields []string
	start := -1
	for i := 0; i < len(s); i++ {
		if s[i] == ' ' || s[i] == '\t' {
			if start >= 0 {
				fields = append(fields, s[start:i])
				start = -1
			}
		} else {
			if start < 0 {
				start = i
			}
		}
	}
	if start >= 0 {
		fields = append(fields, s[start:])
	}
	return fields
}

func (s *Server) handleStorageInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	info, err := s.getStorageInfo()
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get storage info: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(info)
}

// ShardUploadResponse represents the response from shard upload
type ShardUploadResponse struct {
	StoragePath string `json:"storage_path"`
	ShardIndex  int    `json:"shard_index"`
	Size        int64  `json:"size"`
	NodeID      string `json:"node_id"`
}

// handleUploadShard handles uploading a single shard for Reed-Solomon erasure coding
func (s *Server) handleUploadShard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form (max 100MB per shard)
	if err := r.ParseMultipartForm(100 << 20); err != nil {
		http.Error(w, fmt.Sprintf("parse form: %v", err), http.StatusBadRequest)
		return
	}

	shardFile, _, err := r.FormFile("shard")
	if err != nil {
		http.Error(w, fmt.Sprintf("get shard file: %v", err), http.StatusBadRequest)
		return
	}
	defer shardFile.Close()

	// Get metadata from form
	fileID := r.FormValue("file_id")
	shardIndexStr := r.FormValue("shard_index")
	ownerID := r.FormValue("owner_id")

	if fileID == "" || shardIndexStr == "" || ownerID == "" {
		http.Error(w, "file_id, shard_index, and owner_id are required", http.StatusBadRequest)
		return
	}

	var shardIndex int
	fmt.Sscanf(shardIndexStr, "%d", &shardIndex)

	// Create shard storage path: shards/{owner_id}/{file_id}/shard_{index}
	shardDir := filepath.Join(s.config.DataDir, "shards", ownerID, fileID)
	if err := os.MkdirAll(shardDir, 0755); err != nil {
		http.Error(w, fmt.Sprintf("create shard dir: %v", err), http.StatusInternalServerError)
		return
	}

	shardFilename := fmt.Sprintf("shard_%d", shardIndex)
	relPath := filepath.Join("shards", ownerID, fileID, shardFilename)
	fullPath := filepath.Join(s.config.DataDir, relPath)

	// Save shard to disk
	dst, err := os.Create(fullPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("create shard file: %v", err), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, shardFile)
	if err != nil {
		os.Remove(fullPath)
		http.Error(w, fmt.Sprintf("write shard: %v", err), http.StatusInternalServerError)
		return
	}

	// Return success response
	response := ShardUploadResponse{
		StoragePath: relPath,
		ShardIndex:  shardIndex,
		Size:        written,
		NodeID:      s.config.NodeID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// handleDownloadShard handles downloading a single shard
func (s *Server) handleDownloadShard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	storagePath := r.URL.Query().Get("path")
	if storagePath == "" {
		http.Error(w, "path parameter required", http.StatusBadRequest)
		return
	}

	// Ensure path is within data directory (security check)
	fullPath := filepath.Join(s.config.DataDir, storagePath)
	cleanPath := filepath.Clean(fullPath)
	if !hasPrefix(cleanPath, filepath.Clean(s.config.DataDir)) {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	// Open shard file
	file, err := os.Open(fullPath)
	if err != nil {
		if os.IsNotExist(err) {
			http.Error(w, "shard not found", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("open shard: %v", err), http.StatusInternalServerError)
		}
		return
	}
	defer file.Close()

	// Get file info for size
	info, err := file.Stat()
	if err != nil {
		http.Error(w, fmt.Sprintf("stat shard: %v", err), http.StatusInternalServerError)
		return
	}

	// Set headers
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filepath.Base(storagePath)))

	// Stream shard data
	if _, err := io.Copy(w, file); err != nil {
		fmt.Fprintf(os.Stderr, "stream shard error: %v\n", err)
	}
}

// hasPrefix checks if path has the given prefix
func hasPrefix(path, prefix string) bool {
	if len(path) < len(prefix) {
		return false
	}
	return path[:len(prefix)] == prefix
}

func (s *Server) handleUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB max
		http.Error(w, fmt.Sprintf("parse form: %v", err), http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, fmt.Sprintf("get file: %v", err), http.StatusBadRequest)
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

	// Generate file ID and storage path
	fileID := fmt.Sprintf("file-%d-%s", time.Now().UnixNano(), ownerID)
	// Store with format: {owner_id}/{file_id}_{original_filename}
	fileName := fmt.Sprintf("%s_%s", fileID, header.Filename)
	relPath := filepath.Join(ownerID, fileName)
	fullPath := filepath.Join(s.config.DataDir, relPath)

	// Create directory
	if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
		http.Error(w, fmt.Sprintf("create dir: %v", err), http.StatusInternalServerError)
		return
	}

	// Save file
	dst, err := os.Create(fullPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("create file: %v", err), http.StatusInternalServerError)
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, file)
	if err != nil {
		os.Remove(fullPath)
		http.Error(w, fmt.Sprintf("write file: %v", err), http.StatusInternalServerError)
		return
	}

	// Extract extension
	ext := filepath.Ext(header.Filename)
	if ext != "" {
		ext = ext[1:] // Remove leading dot
	}

	// Store metadata in database with exact schema structure
	stmt := `INSERT INTO files (
		id, owner_id, folder_id, filename, extension, original_extension, 
		mime_type, size, original_size, checksum, storage_path, node_id, 
		version, status, is_indexed, uploaded_at, modified_at
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 'active', 0, datetime('now'), datetime('now'))`

	_, err = s.db.Exec(stmt, fileID, ownerID, folderID, header.Filename, 
		ext, ext, mimeType, written, written, checksum, relPath, s.config.NodeID)
	if err != nil {
		os.Remove(fullPath)
		http.Error(w, fmt.Sprintf("save metadata: %v", err), http.StatusInternalServerError)
		return
	}

	// Return metadata
	metadata := FileMetadata{
		ID:          fileID,
		OwnerID:     ownerID,
		FolderID:    folderID,
		Filename:    header.Filename,
		StoragePath: relPath,
		Size:        written,
		Checksum:    checksum,
		NodeID:      s.config.NodeID,
		UploadedAt:  time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(metadata)
}

func (s *Server) handleDownload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fileID := r.URL.Query().Get("file_id")
	if fileID == "" {
		http.Error(w, "file_id required", http.StatusBadRequest)
		return
	}

	// Get metadata from database
	var storagePath, filename, mimeType string
	var size int64
	err := s.db.QueryRow(`SELECT storage_path, filename, mime_type, size FROM files WHERE id = ?`, 
		fileID).Scan(&storagePath, &filename, &mimeType, &size)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "file not found", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("query error: %v", err), http.StatusInternalServerError)
		}
		return
	}

	// Open file
	fullPath := filepath.Join(s.config.DataDir, storagePath)
	file, err := os.Open(fullPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("open file: %v", err), http.StatusInternalServerError)
		return
	}
	defer file.Close()

	// Set headers
	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", size))

	// Stream file
	if _, err := io.Copy(w, file); err != nil {
		fmt.Fprintf(os.Stderr, "stream error: %v\n", err)
	}
}

func (s *Server) handleListFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	ownerID := r.URL.Query().Get("owner_id")
	if ownerID == "" {
		http.Error(w, "owner_id required", http.StatusBadRequest)
		return
	}

	rows, err := s.db.Query(`
		SELECT id, owner_id, folder_id, filename, extension, original_extension, 
			mime_type, size, original_size, checksum, storage_path, node_id, 
			version, status, uploaded_at, modified_at
		FROM files WHERE owner_id = ? AND status = 'active'
		ORDER BY uploaded_at DESC
	`, ownerID)
	if err != nil {
		http.Error(w, fmt.Sprintf("query error: %v", err), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	files := []FileMetadata{}
	for rows.Next() {
		var f FileMetadata
		var uploadedAt, modifiedAt string
		var folderID, extension, originalExt, checksum sql.NullString
		
		err := rows.Scan(&f.ID, &f.OwnerID, &folderID, &f.Filename, &extension, 
			&originalExt, &f.MimeType, &f.Size, &f.OriginalSize, &checksum, 
			&f.StoragePath, &f.NodeID, &f.Version, &f.Status, &uploadedAt, &modifiedAt)
		if err != nil {
			continue
		}
		
		if folderID.Valid {
			f.FolderID = folderID.String
		}
		if extension.Valid {
			f.Extension = extension.String
		}
		if originalExt.Valid {
			f.OriginalExtension = originalExt.String
		}
		if checksum.Valid {
			f.Checksum = checksum.String
		}
		
		f.UploadedAt, _ = time.Parse("2006-01-02 15:04:05", uploadedAt)
		f.ModifiedAt, _ = time.Parse("2006-01-02 15:04:05", modifiedAt)
		files = append(files, f)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(files)
}

func (s *Server) handleGetMetadata(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fileID := r.URL.Query().Get("file_id")
	if fileID == "" {
		http.Error(w, "file_id required", http.StatusBadRequest)
		return
	}

	var f FileMetadata
	var uploadedAt, modifiedAt string
	var folderID, extension, originalExt, checksum, parentArchiveID, relativePath sql.NullString
	
	err := s.db.QueryRow(`
		SELECT id, owner_id, folder_id, filename, extension, original_extension,
			mime_type, size, original_size, checksum, storage_path, node_id,
			version, status, is_indexed, parent_archive_id, relative_path,
			uploaded_at, modified_at
		FROM files WHERE id = ?
	`, fileID).Scan(&f.ID, &f.OwnerID, &folderID, &f.Filename, &extension, &originalExt,
		&f.MimeType, &f.Size, &f.OriginalSize, &checksum, &f.StoragePath, &f.NodeID,
		&f.Version, &f.Status, &f.IsIndexed, &parentArchiveID, &relativePath,
		&uploadedAt, &modifiedAt)
	
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "file not found", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("query error: %v", err), http.StatusInternalServerError)
		}
		return
	}

	// Handle nullable fields
	if folderID.Valid {
		f.FolderID = folderID.String
	}
	if extension.Valid {
		f.Extension = extension.String
	}
	if originalExt.Valid {
		f.OriginalExtension = originalExt.String
	}
	if checksum.Valid {
		f.Checksum = checksum.String
	}
	if parentArchiveID.Valid {
		f.ParentArchiveID = parentArchiveID.String
	}
	if relativePath.Valid {
		f.RelativePath = relativePath.String
	}
	
	f.UploadedAt, _ = time.Parse("2006-01-02 15:04:05", uploadedAt)
	f.ModifiedAt, _ = time.Parse("2006-01-02 15:04:05", modifiedAt)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(f)
}

func (s *Server) handleDeleteFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fileID := r.URL.Query().Get("file_id")
	if fileID == "" {
		http.Error(w, "file_id required", http.StatusBadRequest)
		return
	}

	// Get storage path
	var storagePath string
	err := s.db.QueryRow(`SELECT storage_path FROM files WHERE id = ?`, fileID).Scan(&storagePath)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "file not found", http.StatusNotFound)
		} else {
			http.Error(w, fmt.Sprintf("query error: %v", err), http.StatusInternalServerError)
		}
		return
	}

	// Delete file from disk
	fullPath := filepath.Join(s.config.DataDir, storagePath)
	if err := os.Remove(fullPath); err != nil && !os.IsNotExist(err) {
		http.Error(w, fmt.Sprintf("delete file: %v", err), http.StatusInternalServerError)
		return
	}

	// Mark as deleted in database
	_, err = s.db.Exec(`UPDATE files SET status = 'deleted', modified_at = datetime('now') WHERE id = ?`, fileID)
	if err != nil {
		http.Error(w, fmt.Sprintf("update metadata: %v", err), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func main() {
	// Load configuration
	configPath := os.Getenv("OS_STORAGE_CONFIG")
	if configPath == "" {
		configPath = "/etc/osstorage/config.yaml"
	}

	config, err := loadConfig(configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Ensure data directory exists
	if err := os.MkdirAll(config.DataDir, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create data dir: %v\n", err)
		os.Exit(1)
	}

	// Initialize database
	db, err := initDatabase(config.DBPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to init database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	server := &Server{
		config: config,
		db:     db,
	}

	// Setup HTTP routes
	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/api/storage-info", server.handleStorageInfo)
	mux.HandleFunc("/api/upload", server.handleUpload)
	mux.HandleFunc("/api/upload-shard", server.handleUploadShard)
	mux.HandleFunc("/api/download", server.handleDownload)
	mux.HandleFunc("/api/download-shard", server.handleDownloadShard)
	mux.HandleFunc("/api/files", server.handleListFiles)
	mux.HandleFunc("/api/metadata", server.handleGetMetadata)
	mux.HandleFunc("/api/delete", server.handleDeleteFile)

	fmt.Printf("OS Storage Server starting on %s\n", config.ListenAddr)
	fmt.Printf("Node ID: %s\n", config.NodeID)
	fmt.Printf("Data directory: %s\n", config.DataDir)
	fmt.Printf("Database: %s\n", config.DBPath)

	httpServer := &http.Server{
		Addr:           config.ListenAddr,
		Handler:        mux,
		ReadTimeout:    300 * time.Second,  // 5 min for large uploads
		WriteTimeout:   300 * time.Second,
		MaxHeaderBytes: 1 << 20, // 1 MB
	}

	if err := httpServer.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
		os.Exit(1)
	}
}
