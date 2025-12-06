package handlers

import (
	"archive/zip"
	"context"
	"crypto/rand"
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
	"storageos/controller/core/compress"
	"storageos/controller/core/crypto"
	"storageos/controller/models"
	"storageos/controller/pkg/proxy"
	services "storageos/controller/services/browser"
	"strings"
)

var (
	osNodeClient *proxy.OSNodeClient
	db           *sql.DB
)

// GetOSNodeClient returns the initialized OS node client (for use by other handlers)
func GetOSNodeClient() *proxy.OSNodeClient {
	return osNodeClient
}

// InitFileHandlers initializes the file handlers with database connection
func InitFileHandlers(database *sql.DB) {
	db = database
	osNodeClient = proxy.NewOSNodeClient(db)
	nodeCount := osNodeClient.NodeCount()
	log.Printf("File handlers initialized with %d storage node(s)", nodeCount)
	if nodeCount == 0 {
		log.Printf("WARNING: No storage nodes configured! File uploads will fail.")
		log.Printf("Add storage nodes via: POST /v1/settings/storage-nodes")
	}
}

// UploadFileHandler handles file uploads by proxying to OS storage node
func UploadFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse multipart form
	if err := r.ParseMultipartForm(100 << 20); err != nil { // 100MB memory buffer
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

	// Get authenticated user from context
	userID, ok := getUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Get optional parameters
	folderID := r.FormValue("folder_id")
	enableCompression := r.FormValue("compress") != "false" // Default compress=true

	// Save to temp file for processing
	tempDir := "/tmp/uploads"
	if err := os.MkdirAll(tempDir, 0755); err != nil {
		log.Printf("Error creating temp dir: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	tempFile := filepath.Join(tempDir, header.Filename)
	dst, err := os.Create(tempFile)
	if err != nil {
		log.Printf("Error creating temp file: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if _, err := io.Copy(dst, file); err != nil {
		dst.Close()
		os.Remove(tempFile)
		log.Printf("Error saving temp file: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	dst.Close()

	// Process file (compress + encrypt)
	processedFile, encryptedKey, ephemeralPubKey, originalSize, processedSize, err := processFileForStorage(tempFile, enableCompression)
	if err != nil {
		os.Remove(tempFile)
		log.Printf("Error processing file: %v", err)
		http.Error(w, fmt.Sprintf("Processing failed: %v", err), http.StatusInternalServerError)
		return
	}
	defer os.Remove(processedFile)
	defer os.Remove(tempFile)
	encryptedFilename := filepath.Base(processedFile)

	// Note: encryptedKey is stored encrypted via X25519, client derives it using ephemeralPubKey
	_ = encryptedKey // Key is encrypted and sent via separate channel

	// Open processed (encrypted) file
	processedFileHandle, err := os.Open(processedFile)
	if err != nil {
		log.Printf("Error opening processed file: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer processedFileHandle.Close()

	// Calculate SHA-256 checksum of encrypted file
	hasher := sha256.New()
	if _, err := io.Copy(hasher, processedFileHandle); err != nil {
		log.Printf("Error calculating checksum: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	fileChecksum := hex.EncodeToString(hasher.Sum(nil))

	// Reopen file for upload (we consumed it while calculating checksum)
	processedFileHandle.Close()
	processedFileHandle, err = os.Open(processedFile)
	if err != nil {
		log.Printf("Error reopening processed file: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer processedFileHandle.Close()

	// Detect mime type
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	log.Printf("Uploading encrypted file %s (source: %s) for user %s to OS node (checksum: %s)", encryptedFilename, header.Filename, userID, fileChecksum)

	// Upload encrypted file to OS node
	if osNodeClient == nil {
		log.Printf("ERROR: osNodeClient is nil - file handlers may not be initialized")
		http.Error(w, "Storage service not initialized", http.StatusInternalServerError)
		return
	}

	log.Printf("Attempting upload to OS node (available nodes: %d)", osNodeClient.NodeCount())
	metadata, err := osNodeClient.UploadToNode(
		userID,
		folderID,
		encryptedFilename,
		mimeType,
		fileChecksum, // Send SHA-256 checksum of encrypted file
		processedFileHandle,
		processedSize,
	)
	if err != nil {
		log.Printf("Failed to upload to OS node: %v", err)
		http.Error(w, fmt.Sprintf("Upload failed: %v", err), http.StatusInternalServerError)
		return
	}

	log.Printf("File uploaded successfully: %s (ID: %s, Node: %s, Encrypted: true)", header.Filename, metadata.ID, metadata.NodeID)

	// Persist full metadata so downloads can decrypt files later
	var folderIDPtr *string
	if folderID != "" {
		folderIDPtr = &folderID
	}

	originalExt := strings.TrimPrefix(strings.ToLower(filepath.Ext(header.Filename)), ".")
	controllerFile := &models.File{
		ID:                metadata.ID,
		OwnerID:           userID,
		FolderID:          folderIDPtr,
		Filename:          encryptedFilename,
		Extension:         originalExt,
		OriginalExtension: originalExt,
		MimeType:          getMimeType(header.Filename),
		Size:              metadata.Size,
		OriginalSize:      originalSize,
		Checksum:          fileChecksum,
		StoragePath:       metadata.StoragePath,
		EncryptedKey:      encryptedKey,
		EphemeralPubKey:   ephemeralPubKey,
		NodeID:            metadata.NodeID,
		Version:           1,
		Status:            "active",
		IsIndexed:         false,
		RelativePath:      "",
	}

	if err := models.CreateFile(db, controllerFile); err != nil {
		log.Printf("Warning: failed to save file metadata: %v", err)
	} else {
		log.Printf("Saved encrypted metadata for file %s (%s)", controllerFile.Filename, controllerFile.ID)
	}

	// Return success response
	response := map[string]interface{}{
		"success":       true,
		"file_id":       metadata.ID,
		"filename":      metadata.Filename,
		"size":          metadata.Size,
		"original_size": originalSize,
		"storage_path":  metadata.StoragePath,
		"node_id":       metadata.NodeID,
		"uploaded_at":   metadata.UploadedAt,
		"encrypted":     true,
		"compressed":    enableCompression,
		"ephemeral_key": hex.EncodeToString(ephemeralPubKey),
		"message":       "File uploaded to OS storage successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(response)
}

// DownloadFileHandler handles file downloads from OS storage node
func DownloadFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fileID := r.URL.Query().Get("file_id")
	if fileID == "" {
		http.Error(w, "file_id required", http.StatusBadRequest)
		return
	}

	// Get authenticated user from context
	userID, ok := getUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	fileRecord, err := models.GetFileByID(db, fileID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			log.Printf("Failed to load file metadata: %v", err)
			http.Error(w, "Failed to load file metadata", http.StatusInternalServerError)
		}
		return
	}

	if fileRecord.OwnerID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	decryptedPath, decryptedSize, err := fetchAndDecryptFile(fileRecord, true)
	if err != nil {
		log.Printf("Failed to decrypt file %s: %v", fileID, err)
		http.Error(w, "Failed to decrypt file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(decryptedPath)

	decryptedFile, err := os.Open(decryptedPath)
	if err != nil {
		log.Printf("Failed to open decrypted file %s: %v", decryptedPath, err)
		http.Error(w, "Failed to stream file", http.StatusInternalServerError)
		return
	}
	defer decryptedFile.Close()

	filename := downloadFilenameFor(fileRecord)
	if filename == "" {
		filename = fileID
	}
	mimeType := fileRecord.MimeType
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", fmt.Sprintf("%d", decryptedSize))
	w.Header().Set("Content-Type", mimeType)

	if _, err := io.Copy(w, decryptedFile); err != nil {
		log.Printf("Error streaming decrypted file %s: %v", fileID, err)
	}
}

// PreviewFileHandler streams decrypted file content inline for browser previews
func PreviewFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fileID := r.URL.Query().Get("file_id")
	if fileID == "" {
		http.Error(w, "file_id required", http.StatusBadRequest)
		return
	}

	userID, ok := getUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	fileRecord, err := models.GetFileByID(db, fileID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			log.Printf("Failed to load file metadata: %v", err)
			http.Error(w, "Failed to load file metadata", http.StatusInternalServerError)
		}
		return
	}

	if fileRecord.OwnerID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if isArchiveFile(fileRecord) || isCompressedFile(fileRecord) {
		http.Error(w, "Preview not available for compressed items", http.StatusBadRequest)
		return
	}

	decryptedPath, _, err := fetchAndDecryptFile(fileRecord, true)
	if err != nil {
		log.Printf("Failed to decrypt file %s for preview: %v", fileID, err)
		http.Error(w, "Failed to decrypt file", http.StatusInternalServerError)
		return
	}
	defer os.Remove(decryptedPath)

	previewFile, err := os.Open(decryptedPath)
	if err != nil {
		log.Printf("Failed to open decrypted preview file %s: %v", decryptedPath, err)
		http.Error(w, "Failed to stream file", http.StatusInternalServerError)
		return
	}
	defer previewFile.Close()

	info, err := previewFile.Stat()
	if err != nil {
		log.Printf("Failed to stat preview file %s: %v", decryptedPath, err)
		http.Error(w, "Failed to stream file", http.StatusInternalServerError)
		return
	}

	downloadName := downloadFilenameFor(fileRecord)
	mimeType := fileRecord.MimeType
	if mimeType == "" || mimeType == "application/octet-stream" {
		buf := make([]byte, 512)
		n, _ := previewFile.Read(buf)
		mimeType = http.DetectContentType(buf[:n])
		if _, err := previewFile.Seek(0, io.SeekStart); err != nil {
			log.Printf("Failed to rewind preview file: %v", err)
			http.Error(w, "Failed to stream file", http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", mimeType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("inline; filename=%q", downloadName))
	http.ServeContent(w, r, downloadName, info.ModTime(), previewFile)
}

// DownloadFolderHandler packages a folder tree into a zip and streams it to the browser
func DownloadFolderHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	folderID := r.URL.Query().Get("folder_id")
	if folderID == "" {
		http.Error(w, "folder_id required", http.StatusBadRequest)
		return
	}

	userID, ok := getUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var folderName, ownerID string
	err := db.QueryRow(`SELECT name, owner_id FROM folders WHERE id = ?`, folderID).Scan(&folderName, &ownerID)
	if err == sql.ErrNoRows {
		http.Error(w, "Folder not found", http.StatusNotFound)
		return
	} else if err != nil {
		log.Printf("Failed to load folder metadata: %v", err)
		http.Error(w, "Failed to load folder", http.StatusInternalServerError)
		return
	}

	if ownerID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	childIDs, err := collectDescendantFolderIDs(folderID)
	if err != nil {
		log.Printf("Failed to enumerate subfolders: %v", err)
		http.Error(w, "Failed to prepare folder", http.StatusInternalServerError)
		return
	}
	allFolderIDs := append([]string{folderID}, childIDs...)
	files, err := loadFilesForFolderTree(allFolderIDs)
	if err != nil {
		log.Printf("Failed to load files for folder %s: %v", folderID, err)
		http.Error(w, "Failed to load folder contents", http.StatusInternalServerError)
		return
	}
	if len(files) == 0 {
		http.Error(w, "Folder has no downloadable files", http.StatusNotFound)
		return
	}

	workingDir, err := os.MkdirTemp("", "folder-download-*")
	if err != nil {
		log.Printf("Failed to create temp dir: %v", err)
		http.Error(w, "Failed to prepare download", http.StatusInternalServerError)
		return
	}
	defer os.RemoveAll(workingDir)

	rootName := sanitizeDownloadName(folderName)
	rootPath := filepath.Join(workingDir, rootName)
	if err := os.MkdirAll(rootPath, 0755); err != nil {
		log.Printf("Failed to create root folder: %v", err)
		http.Error(w, "Failed to prepare download", http.StatusInternalServerError)
		return
	}

	for _, file := range files {
		decPath, _, err := fetchAndDecryptFile(file, false)
		if err != nil {
			log.Printf("Failed to decrypt %s: %v", file.ID, err)
			http.Error(w, "Failed to decrypt folder contents", http.StatusInternalServerError)
			return
		}

		relPath := strings.TrimSpace(file.RelativePath)
		if relPath == "" {
			relPath = downloadFilenameFor(file)
		}
		if relPath == "" {
			relPath = file.ID
		}
		targetPath := filepath.Join(rootPath, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
			os.Remove(decPath)
			log.Printf("Failed to create path %s: %v", targetPath, err)
			http.Error(w, "Failed to prepare folder structure", http.StatusInternalServerError)
			return
		}

		if err := os.Rename(decPath, targetPath); err != nil {
			if copyErr := copyFile(decPath, targetPath); copyErr != nil {
				os.Remove(decPath)
				log.Printf("Failed to place file %s: %v", targetPath, copyErr)
				http.Error(w, "Failed to prepare folder", http.StatusInternalServerError)
				return
			}
			os.Remove(decPath)
		}
	}

	zipPath := filepath.Join(workingDir, rootName+".zip")
	if err := zipDirectory(rootPath, zipPath); err != nil {
		log.Printf("Failed to zip folder %s: %v", rootPath, err)
		http.Error(w, "Failed to compress folder", http.StatusInternalServerError)
		return
	}

	zipFile, err := os.Open(zipPath)
	if err != nil {
		log.Printf("Failed to open zip %s: %v", zipPath, err)
		http.Error(w, "Failed to stream folder", http.StatusInternalServerError)
		return
	}
	defer zipFile.Close()

	info, err := zipFile.Stat()
	if err != nil {
		log.Printf("Failed to stat zip %s: %v", zipPath, err)
		http.Error(w, "Failed to stream folder", http.StatusInternalServerError)
		return
	}

	zipName := rootName + ".zip"
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", zipName))
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", info.Size()))

	if _, err := io.Copy(w, zipFile); err != nil {
		log.Printf("Error streaming folder zip %s: %v", zipName, err)
	}
}

// ListFilesHandler lists files for the authenticated user from controller metadata
func ListFilesHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get authenticated user from context
	userID, ok := getUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Query files from controller database (filtered by owner_id)
	rows, err := db.Query(`
		SELECT id, name, type, size, node_id, folder_id, created_at, modified_at
		FROM files
		WHERE owner_id = ?
		ORDER BY modified_at DESC
	`, userID)

	if err != nil {
		log.Printf("Failed to query files: %v", err)
		http.Error(w, "Failed to list files", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var files []map[string]interface{}
	for rows.Next() {
		var id, name, fileType, nodeID string
		var size int64
		var folderID, createdAt, modifiedAt sql.NullString

		if err := rows.Scan(&id, &name, &fileType, &size, &nodeID, &folderID, &createdAt, &modifiedAt); err != nil {
			log.Printf("Error scanning file row: %v", err)
			continue
		}

		file := map[string]interface{}{
			"id":      id,
			"name":    name,
			"type":    fileType,
			"size":    size,
			"node_id": nodeID,
		}

		if folderID.Valid {
			file["folder_id"] = folderID.String
		}
		if createdAt.Valid {
			file["created_at"] = createdAt.String
		}
		if modifiedAt.Valid {
			file["modified_at"] = modifiedAt.String
		}

		files = append(files, file)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"files": files,
		"count": len(files),
	})
}

// DeleteFileHandler deletes a file from OS storage
func DeleteFileHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	fileID := r.URL.Query().Get("file_id")
	if fileID == "" {
		http.Error(w, "file_id required", http.StatusBadRequest)
		return
	}

	// Get authenticated user from context
	userID, ok := getUserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Verify user owns the file from OS metadata
	metadata, err := osNodeClient.GetMetadataFromNode(fileID)
	if err != nil {
		if err == sql.ErrNoRows {
			http.Error(w, "File not found", http.StatusNotFound)
		} else {
			log.Printf("Failed to get metadata: %v", err)
			http.Error(w, "Failed to retrieve file metadata", http.StatusInternalServerError)
		}
		return
	}

	if metadata.OwnerID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	// Delete from OS node
	if err := osNodeClient.DeleteFromNode(fileID); err != nil {
		log.Printf("Failed to delete from OS node: %v", err)
		http.Error(w, "Delete failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// getUserIDFromContext extracts user ID from request context
func getUserIDFromContext(ctx context.Context) (string, bool) {
	// Try to get from Claims (if using JWT middleware)
	if claims, ok := ctx.Value("user").(*services.Claims); ok && claims != nil {
		return claims.UserID, true
	}

	// Fallback to string value
	if userID, ok := ctx.Value("user_id").(string); ok {
		return userID, true
	}

	return "", false
}

// processFileForStorage compresses (optional) and encrypts a file for storage
// Returns: processedFilePath, encryptedKey, ephemeralPubKey, originalSize, processedSize, error
func processFileForStorage(filePath string, enableCompression bool) (string, []byte, []byte, int64, int64, error) {
	// Get original file size
	fileInfo, err := os.Stat(filePath)
	if err != nil {
		return "", nil, nil, 0, 0, fmt.Errorf("stat file: %w", err)
	}
	originalSize := fileInfo.Size()

	baseName := filepath.Base(filePath)
	processedDir := filepath.Dir(filePath)

	archiveSourcePath := filePath
	var compressedPath string
	var encryptedFilename string

	if enableCompression {
		// Compress the file using Zstandard
		compressedPath = filepath.Join(processedDir, baseName+".zst")
		log.Printf("Compressing file to: %s", compressedPath)
		if err := compress.CompressFile(filePath, compressedPath, 3); err != nil {
			return "", nil, nil, 0, 0, fmt.Errorf("compression failed: %w", err)
		}
		archiveSourcePath = compressedPath
		encryptedFilename = baseName + ".zst.enc"
	} else {
		log.Printf("Compression disabled, encrypting original file")
		encryptedFilename = baseName + ".enc"
	}

	// Generate random AES-256 key
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		return "", nil, nil, 0, 0, fmt.Errorf("generate AES key: %w", err)
	}

	// Encrypt file with AES-256-GCM
	encryptedPath := filepath.Join(processedDir, encryptedFilename)
	log.Printf("Encrypting file to: %s", encryptedPath)
	if err := crypto.EncryptFile(archiveSourcePath, encryptedPath, aesKey); err != nil {
		return "", nil, nil, 0, 0, fmt.Errorf("AES encryption: %w", err)
	}

	// Get encrypted file size
	encFileInfo, err := os.Stat(encryptedPath)
	if err != nil {
		return "", nil, nil, 0, 0, fmt.Errorf("stat encrypted file: %w", err)
	}
	processedSize := encFileInfo.Size()

	// Encrypt AES key using X25519
	receiverPub := crypto.GetHardcodedPublicKey()
	ephemeralPriv, ephemeralPub, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		return "", nil, nil, 0, 0, fmt.Errorf("generate ephemeral keypair: %w", err)
	}

	shared, err := crypto.DeriveSharedSecret(ephemeralPriv, receiverPub)
	if err != nil {
		return "", nil, nil, 0, 0, fmt.Errorf("derive shared secret: %w", err)
	}

	keyEncryptionKey, err := crypto.DeriveAES256KeyFromShared(shared, []byte("file-key-encryption-v1"))
	if err != nil {
		return "", nil, nil, 0, 0, fmt.Errorf("derive key encryption key: %w", err)
	}

	encryptedKey, err := crypto.EncryptData(aesKey, keyEncryptionKey)
	if err != nil {
		return "", nil, nil, 0, 0, fmt.Errorf("encrypt AES key: %w", err)
	}

	log.Printf("File processed: original=%d, processed=%d, compressed=%v", originalSize, processedSize, enableCompression)

	return encryptedPath, encryptedKey, ephemeralPub, originalSize, processedSize, nil
}

// detectFileType determines the file type category for frontend display
func detectFileType(filename string) string {
	ext := filepath.Ext(filename)
	if ext == "" {
		return "file"
	}
	ext = ext[1:] // Remove leading dot

	// Image types
	imageExts := map[string]bool{
		"jpg": true, "jpeg": true, "png": true, "gif": true,
		"bmp": true, "svg": true, "webp": true, "ico": true,
	}
	if imageExts[ext] {
		return "image"
	}

	// Video types
	videoExts := map[string]bool{
		"mp4": true, "avi": true, "mov": true, "wmv": true,
		"flv": true, "mkv": true, "webm": true, "m4v": true,
	}
	if videoExts[ext] {
		return "video"
	}

	// Audio types
	audioExts := map[string]bool{
		"mp3": true, "wav": true, "flac": true, "aac": true,
		"ogg": true, "m4a": true, "wma": true,
	}
	if audioExts[ext] {
		return "audio"
	}

	// Document types
	docExts := map[string]bool{
		"pdf": true, "doc": true, "docx": true, "txt": true,
		"rtf": true, "odt": true, "xls": true, "xlsx": true,
		"ppt": true, "pptx": true, "csv": true,
	}
	if docExts[ext] {
		return "document"
	}

	return "file"
}

// getMimeType returns MIME type based on file extension
func getMimeType(filename string) string {
	ext := strings.ToLower(filepath.Ext(filename))
	ext = strings.TrimPrefix(ext, ".")

	mimeTypes := map[string]string{
		// Images
		"jpg": "image/jpeg", "jpeg": "image/jpeg", "png": "image/png",
		"gif": "image/gif", "bmp": "image/bmp", "svg": "image/svg+xml",
		"webp": "image/webp", "ico": "image/x-icon",
		// Videos
		"mp4": "video/mp4", "avi": "video/x-msvideo", "mov": "video/quicktime",
		"wmv": "video/x-ms-wmv", "flv": "video/x-flv", "mkv": "video/x-matroska",
		"webm": "video/webm",
		// Audio
		"mp3": "audio/mpeg", "wav": "audio/wav", "flac": "audio/flac",
		"aac": "audio/aac", "ogg": "audio/ogg", "m4a": "audio/mp4",
		// Documents
		"pdf": "application/pdf", "doc": "application/msword",
		"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"txt":  "text/plain", "csv": "text/csv",
		"xls":  "application/vnd.ms-excel",
		"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
	}

	if mime, ok := mimeTypes[ext]; ok {
		return mime
	}
	return "application/octet-stream"
}

func fetchAndDecryptFile(file *models.File, allowDecompression bool) (string, int64, error) {
	reader, _, err := osNodeClient.DownloadFromNode(file.ID)
	if err != nil {
		return "", 0, fmt.Errorf("download from node: %w", err)
	}
	defer reader.Close()
	decryptedPath, decryptedSize, err := decryptStreamToTemp(file, reader)
	if err != nil {
		return "", 0, err
	}

	if allowDecompression && requiresDecompression(file) {
		decompPath, decompSize, err := decompressDecryptedFile(file, decryptedPath)
		if err != nil {
			os.Remove(decryptedPath)
			return "", 0, err
		}
		os.Remove(decryptedPath)
		return decompPath, decompSize, nil
	}

	return decryptedPath, decryptedSize, nil
}

func decryptStreamToTemp(file *models.File, reader io.Reader) (string, int64, error) {
	encTemp, err := os.CreateTemp("", "enc-*")
	if err != nil {
		return "", 0, fmt.Errorf("create temp file: %w", err)
	}
	encPath := encTemp.Name()
	defer os.Remove(encPath)
	if _, err := io.Copy(encTemp, reader); err != nil {
		encTemp.Close()
		return "", 0, fmt.Errorf("write encrypted temp: %w", err)
	}
	if err := encTemp.Close(); err != nil {
		return "", 0, fmt.Errorf("close encrypted temp: %w", err)
	}

	aesKey, err := deriveFileAESKey(file)
	if err != nil {
		return "", 0, err
	}

	decTemp, err := os.CreateTemp("", "dec-*")
	if err != nil {
		return "", 0, fmt.Errorf("create decrypted temp: %w", err)
	}
	decPath := decTemp.Name()
	decTemp.Close()

	if err := crypto.DecryptFile(encPath, decPath, aesKey); err != nil {
		os.Remove(decPath)
		return "", 0, fmt.Errorf("decrypt file: %w", err)
	}

	info, err := os.Stat(decPath)
	if err != nil {
		os.Remove(decPath)
		return "", 0, fmt.Errorf("stat decrypted file: %w", err)
	}

	return decPath, info.Size(), nil
}

func deriveFileAESKey(file *models.File) ([]byte, error) {
	if len(file.EncryptedKey) == 0 || len(file.EphemeralPubKey) == 0 {
		return nil, fmt.Errorf("missing encryption metadata for file %s", file.ID)
	}
	receiverPriv := crypto.GetHardcodedPrivateKey()
	shared, err := crypto.DeriveSharedSecret(receiverPriv, file.EphemeralPubKey)
	if err != nil {
		return nil, fmt.Errorf("derive shared secret: %w", err)
	}
	info := determineKeyInfo(file)
	kek, err := crypto.DeriveAES256KeyFromShared(shared, info)
	if err != nil {
		return nil, fmt.Errorf("derive key encryption key: %w", err)
	}
	aesKey, err := crypto.DecryptData(file.EncryptedKey, kek)
	if err != nil {
		return nil, fmt.Errorf("decrypt AES key: %w", err)
	}
	if len(aesKey) != 32 {
		return nil, fmt.Errorf("invalid AES key size for file %s", file.ID)
	}
	return aesKey, nil
}

func determineKeyInfo(file *models.File) []byte {
	if file.RelativePath != "" || strings.HasSuffix(strings.ToLower(file.Filename), ".tar.zst.enc") {
		return []byte("folder-key-encryption-v1")
	}
	return []byte("file-key-encryption-v1")
}

func downloadFilenameFor(file *models.File) string {
	if file == nil {
		return ""
	}
	name := file.Filename
	if name == "" {
		return ""
	}
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".zst.enc"):
		return name[:len(name)-len(".zst.enc")]
	case strings.HasSuffix(lower, ".enc"):
		return name[:len(name)-len(".enc")]
	default:
		return name
	}
}

func isArchiveFile(file *models.File) bool {
	if file == nil {
		return false
	}
	name := strings.ToLower(file.Filename)
	return strings.HasSuffix(name, ".tar.zst.enc")
}

func isCompressedFile(file *models.File) bool {
	if file == nil {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(file.Filename))
	return strings.HasSuffix(name, ".zst.enc")
}

func requiresDecompression(file *models.File) bool {
	return isCompressedFile(file)
}

func decompressDecryptedFile(file *models.File, decryptedPath string) (string, int64, error) {
	pattern := "decompressed-*"
	if finalName := downloadFilenameFor(file); finalName != "" {
		if ext := filepath.Ext(finalName); ext != "" {
			pattern = fmt.Sprintf("decompressed-*%s", ext)
		}
	}
	outFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", 0, fmt.Errorf("create decompressed temp: %w", err)
	}
	decompPath := outFile.Name()
	outFile.Close()

	if err := compress.DecompressFile(decryptedPath, decompPath); err != nil {
		os.Remove(decompPath)
		return "", 0, fmt.Errorf("decompress file: %w", err)
	}

	info, err := os.Stat(decompPath)
	if err != nil {
		os.Remove(decompPath)
		return "", 0, fmt.Errorf("stat decompressed file: %w", err)
	}

	return decompPath, info.Size(), nil
}

func collectDescendantFolderIDs(rootID string) ([]string, error) {
	var result []string
	visited := map[string]bool{rootID: true}
	queue := []string{rootID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		rows, err := db.Query(`SELECT id FROM folders WHERE parent_id = ?`, current)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var child string
			if err := rows.Scan(&child); err != nil {
				rows.Close()
				return nil, err
			}
			if !visited[child] {
				visited[child] = true
				result = append(result, child)
				queue = append(queue, child)
			}
		}
		rows.Close()
	}
	return result, nil
}

func loadFilesForFolderTree(folderIDs []string) ([]*models.File, error) {
	var files []*models.File
	for _, id := range folderIDs {
		folderFiles, err := models.GetFilesByFolderID(db, id)
		if err != nil {
			return nil, err
		}
		files = append(files, folderFiles...)
	}
	return files, nil
}

func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}
	return dstFile.Sync()
}

func zipDirectory(sourceDir, zipPath string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		relPath = filepath.ToSlash(relPath)
		if info.IsDir() {
			if relPath == "." {
				return nil
			}
			_, err := zipWriter.Create(relPath + "/")
			return err
		}
		zipEntry, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}
		file, err := os.Open(path)
		if err != nil {
			return err
		}
		if _, err := io.Copy(zipEntry, file); err != nil {
			file.Close()
			return err
		}
		return file.Close()
	})
}

func sanitizeDownloadName(name string) string {
	clean := strings.TrimSpace(name)
	if clean == "" {
		clean = "folder"
	}
	replacer := strings.NewReplacer("/", "_", "\\", "_", "..", "_")
	clean = replacer.Replace(clean)
	return clean
}
