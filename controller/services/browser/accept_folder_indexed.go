package services

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"

	"storageos/controller/core/crypto"
	"storageos/controller/models"

	"github.com/google/uuid"
)

// FolderIndexEntry represents metadata for a file inside a folder
type FolderIndexEntry struct {
	RelativePath string
	OriginalSize int64
	Checksum     string
	MimeType     string
	Extension    string
}

// AcceptFolderIndexed processes an uploaded folder with individual file indexing.
// It indexes all files, optionally compresses the folder, encrypts it, and stores both
// the archive metadata and individual file metadata for browsing.
func AcceptFolderIndexed(ctx context.Context, folderPath string, ownerID string, folderName string, compressBeforeEncrypt bool, parentFolderID *string) error {
	_ = ctx

	log.Printf("Processing folder WITH INDEXING: %s (name: %s) for owner: %s (parent=%v)", folderPath, folderName, ownerID, parentFolderID)

	// Step 1: Index all files in the folder
	var fileIndex []FolderIndexEntry
	var totalOriginalSize int64

	err := filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get relative path from folder root
		relPath, err := filepath.Rel(folderPath, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path: %w", err)
		}

		// Calculate checksum
		file, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("failed to open file for checksum: %w", err)
		}
		defer file.Close()

		hasher := sha256.New()
		if _, err := io.Copy(hasher, file); err != nil {
			return fmt.Errorf("failed to calculate checksum: %w", err)
		}
		checksum := fmt.Sprintf("%x", hasher.Sum(nil))

		// Determine extension and mime type
		ext := strings.TrimPrefix(filepath.Ext(relPath), ".")
		mimeType := getMimeTypeFromExtension(ext)

		fileIndex = append(fileIndex, FolderIndexEntry{
			RelativePath: relPath,
			OriginalSize: info.Size(),
			Checksum:     checksum,
			MimeType:     mimeType,
			Extension:    ext,
		})

		totalOriginalSize += info.Size()

		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to index folder: %w", err)
	}

	log.Printf("Indexed %d files, total original size: %d bytes", len(fileIndex), totalOriginalSize)

	// Get current directory for temp files
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Step 7: Create folder entry in database (root folder)
	folderID := uuid.New().String()
	folder := &models.Folder{
		ID:         folderID,
		OwnerID:    ownerID,
		ParentID:   parentFolderID,
		Name:       folderName,
		IsArchived: false, // Not archived - browsable structure
		TotalFiles: len(fileIndex),
		TotalSize:  totalOriginalSize,
	}

	if err := models.CreateFolder(fileService.db, folder); err != nil {
		return fmt.Errorf("failed to create folder metadata: %w", err)
	}

	log.Printf("Created folder entry: ID=%s, Name=%s, Files=%d", folderID, folderName, len(fileIndex))

	// Step 8: Build folder hierarchy map
	folderMap := make(map[string]string)
	folderMap[""] = folderID // Root folder (empty path)

	// Extract unique folder paths from file index
	uniqueFolders := make(map[string]bool)
	for _, entry := range fileIndex {
		dir := filepath.Dir(entry.RelativePath)
		log.Printf("DEBUG: File=%s, Dir=%s", entry.RelativePath, dir)
		if dir == "." {
			dir = "" // Root level
		}

		// Add all parent directories
		parts := strings.Split(dir, string(filepath.Separator))
		currentPath := ""
		for _, part := range parts {
			if part == "" {
				continue
			}
			if currentPath == "" {
				currentPath = part
			} else {
				currentPath = filepath.Join(currentPath, part)
			}
			uniqueFolders[currentPath] = true
		}
	}

	log.Printf("Found %d unique subfolders in archive", len(uniqueFolders))

	// Sort folder paths to ensure parents are created before children
	var folderPaths []string
	for path := range uniqueFolders {
		folderPaths = append(folderPaths, path)
	}

	// Sort by path depth (fewer separators = earlier in hierarchy)
	sortFoldersByDepth := func(paths []string) {
		for i := 0; i < len(paths)-1; i++ {
			for j := i + 1; j < len(paths); j++ {
				if strings.Count(paths[i], string(filepath.Separator)) > strings.Count(paths[j], string(filepath.Separator)) {
					paths[i], paths[j] = paths[j], paths[i]
				}
			}
		}
	}
	sortFoldersByDepth(folderPaths)

	// Create folder entries for each unique directory
	for _, folderPath := range folderPaths {
		subFolderID := uuid.New().String()

		// Determine parent folder ID
		parentPath := filepath.Dir(folderPath)
		if parentPath == "." {
			parentPath = ""
		}
		parentID := folderMap[parentPath]

		// Get folder name (last component of path)
		folderName := filepath.Base(folderPath)

		subFolder := &models.Folder{
			ID:         subFolderID,
			OwnerID:    ownerID,
			ParentID:   &parentID,
			Name:       folderName,
			IsArchived: false,
			TotalFiles: 0, // Will be calculated later
			TotalSize:  0,
		}

		if err := models.CreateFolder(fileService.db, subFolder); err != nil {
			log.Printf("Warning: failed to create subfolder %s: %v", folderPath, err)
			continue
		}

		folderMap[folderPath] = subFolderID
		log.Printf("Created subfolder: %s (ID=%s, ParentID=%s)", folderPath, subFolderID, parentID)
	}

	// Step 9: Encrypt and upload each file individually to preserve structure
	log.Printf("Encrypting and uploading %d files individually...", len(fileIndex))

	for _, entry := range fileIndex {
		// Determine the correct folder_id based on file's directory
		fileDir := filepath.Dir(entry.RelativePath)
		if fileDir == "." {
			fileDir = "" // Root level
		}
		fileFolderID := folderMap[fileDir]
		if fileFolderID == "" {
			fileFolderID = folderID // Fallback to root folder
		}

		// Original file path
		originalFilePath := filepath.Join(folderPath, entry.RelativePath)

		// Generate encryption keys for this file
		aesKey := make([]byte, 32)
		if _, err := rand.Read(aesKey); err != nil {
			log.Printf("Warning: failed to generate key for %s: %v", entry.RelativePath, err)
			continue
		}

		// Encrypt file
		encryptedDir := filepath.Join(currentDir, processedFoldersDir, "encrypted")
		if err := os.MkdirAll(encryptedDir, 0755); err != nil {
			log.Printf("Warning: failed to create encrypted dir: %v", err)
			continue
		}

		encryptedStoredName := fmt.Sprintf("%s.enc", filepath.Base(entry.RelativePath))
		encryptedFilePath := filepath.Join(encryptedDir, encryptedStoredName)
		if err := crypto.EncryptFile(originalFilePath, encryptedFilePath, aesKey); err != nil {
			log.Printf("Warning: failed to encrypt %s: %v", entry.RelativePath, err)
			continue
		}

		// Encrypt the AES key using X25519
		receiverPub := crypto.GetHardcodedPublicKey()
		ephemeralPriv, ephemeralPub, err := crypto.GenerateX25519KeyPair()
		if err != nil {
			log.Printf("Warning: failed to generate keypair for %s: %v", entry.RelativePath, err)
			os.Remove(encryptedFilePath)
			continue
		}

		shared, err := crypto.DeriveSharedSecret(ephemeralPriv, receiverPub)
		if err != nil {
			log.Printf("Warning: failed to derive secret for %s: %v", entry.RelativePath, err)
			os.Remove(encryptedFilePath)
			continue
		}

		keyEncryptionKey, err := crypto.DeriveAES256KeyFromShared(shared, []byte("folder-key-encryption-v1"))
		if err != nil {
			log.Printf("Warning: failed to derive KEK for %s: %v", entry.RelativePath, err)
			os.Remove(encryptedFilePath)
			continue
		}

		encryptedKey, err := crypto.EncryptData(aesKey, keyEncryptionKey)
		if err != nil {
			log.Printf("Warning: failed to encrypt key for %s: %v", entry.RelativePath, err)
			os.Remove(encryptedFilePath)
			continue
		}

		// Get encrypted file size
		encFileInfo, err := os.Stat(encryptedFilePath)
		if err != nil {
			log.Printf("Warning: failed to stat encrypted file %s: %v", entry.RelativePath, err)
			os.Remove(encryptedFilePath)
			continue
		}

		// Upload encrypted file to OS node
		encFileHandle, err := os.Open(encryptedFilePath)
		if err != nil {
			log.Printf("Warning: failed to open encrypted file %s: %v", entry.RelativePath, err)
			os.Remove(encryptedFilePath)
			continue
		}

		// Calculate checksum
		hasher := sha256.New()
		io.Copy(hasher, encFileHandle)
		fileChecksum := fmt.Sprintf("%x", hasher.Sum(nil))
		encFileHandle.Seek(0, 0) // Reset to beginning

		osMetadata, err := getOSNodeClient().UploadToNode(
			ownerID,
			fileFolderID,
			encryptedStoredName,
			"application/octet-stream",
			fileChecksum,
			encFileHandle,
			encFileInfo.Size(),
		)
		encFileHandle.Close()
		os.Remove(encryptedFilePath) // Clean up local encrypted file

		if err != nil {
			log.Printf("Warning: failed to upload %s to OS node: %v", entry.RelativePath, err)
			continue
		}

		// Create file entry in controller database
		fileID := osMetadata.ID
		file := &models.File{
			ID:                fileID,
			OwnerID:           ownerID,
			FolderID:          &fileFolderID,
			Filename:          encryptedStoredName,
			Extension:         entry.Extension,
			OriginalExtension: entry.Extension,
			MimeType:          entry.MimeType,
			Size:              encFileInfo.Size(),
			OriginalSize:      entry.OriginalSize,
			Checksum:          entry.Checksum, // Original file checksum
			StoragePath:       osMetadata.StoragePath,
			EncryptedKey:      encryptedKey,
			EphemeralPubKey:   ephemeralPub,
			NodeID:            osMetadata.NodeID,
			Version:           1,
			Status:            "active",
			IsIndexed:         true,
			ParentArchiveID:   nil,
			RelativePath:      entry.RelativePath,
		}

		if err := models.CreateFile(fileService.db, file); err != nil {
			log.Printf("Warning: failed to create file entry for %s: %v", entry.RelativePath, err)
			continue
		}

		log.Printf("Uploaded: %s -> Node %s (encrypted size: %d)", entry.RelativePath, osMetadata.NodeID, encFileInfo.Size())
	}

	// Step 10: Clean up
	os.RemoveAll(folderPath) // Remove original uploaded folder

	log.Printf("Indexed folder processing complete!")
	log.Printf("  - Folder ID: %s", folderID)
	log.Printf("  - Subfolders created: %d", len(folderPaths))
	log.Printf("  - Files uploaded: %d", len(fileIndex))
	log.Printf("  - Original size: %d bytes", totalOriginalSize)

	return nil
}

// getMimeTypeFromExtension returns a MIME type based on file extension
func getMimeTypeFromExtension(ext string) string {
	mimeTypes := map[string]string{
		"txt":  "text/plain",
		"html": "text/html",
		"css":  "text/css",
		"js":   "application/javascript",
		"json": "application/json",
		"xml":  "application/xml",
		"pdf":  "application/pdf",
		"zip":  "application/zip",
		"png":  "image/png",
		"jpg":  "image/jpeg",
		"jpeg": "image/jpeg",
		"gif":  "image/gif",
		"svg":  "image/svg+xml",
		"mp3":  "audio/mpeg",
		"mp4":  "video/mp4",
		"avi":  "video/x-msvideo",
		"doc":  "application/msword",
		"docx": "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		"xls":  "application/vnd.ms-excel",
		"xlsx": "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
		"ppt":  "application/vnd.ms-powerpoint",
		"pptx": "application/vnd.openxmlformats-officedocument.presentationml.presentation",
	}

	if mime, ok := mimeTypes[strings.ToLower(ext)]; ok {
		return mime
	}

	return "application/octet-stream"
}
