package services

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"storageos/controller/models"
	"storageos/controller/pkg/proxy"

	"github.com/google/uuid"
)

var (
	fileService     *FileService
	osNodeClientFn  func() *proxy.OSNodeClient // Function to get the shared OS node client
)

// FileService handles file and folder metadata operations
type FileService struct {
	db *sql.DB
}

// NewFileService creates a new file service
func NewFileService(db *sql.DB) *FileService {
	return &FileService{db: db}
}

// InitFileService initializes the global file service
// It takes a function that returns the shared OS node client from handlers
func InitFileService(db *sql.DB, getOSNodeClient func() *proxy.OSNodeClient) {
	fileService = NewFileService(db)
	osNodeClientFn = getOSNodeClient
	log.Printf("Browser services initialized (using shared OS node client)")
}

// getOSNodeClient returns the shared OS node client
func getOSNodeClient() *proxy.OSNodeClient {
	if osNodeClientFn == nil {
		return nil
	}
	return osNodeClientFn()
}

// CreateFileMetadata creates metadata for an uploaded file
func (fs *FileService) CreateFileMetadata(ownerID, filename, storagePath string, size int64) (*models.File, error) {
	// Generate UUID for the file
	fileID := uuid.New().String()

	// Get file extension
	extension := ""
	if dotIndex := strings.LastIndex(filename, "."); dotIndex != -1 {
		extension = filename[dotIndex+1:]
	}

	// Get MIME type (simplified)
	mimeType := "application/octet-stream"
	if extension != "" {
		switch strings.ToLower(extension) {
		case "txt":
			mimeType = "text/plain"
		case "jpg", "jpeg":
			mimeType = "image/jpeg"
		case "png":
			mimeType = "image/png"
		case "pdf":
			mimeType = "application/pdf"
		case "zip":
			mimeType = "application/zip"
		}
	}

	// Calculate checksum (simplified - in real implementation, calculate from file content)
	checksum := fs.calculateChecksum(storagePath)

	now := time.Now().UTC().Format(time.RFC3339)

	file := &models.File{
		ID:          fileID,
		OwnerID:     ownerID,
		Filename:    filename,
		Extension:   extension,
		MimeType:    mimeType,
		Size:        size,
		Checksum:    checksum,
		StoragePath: storagePath,
		NodeID:      "", // Leave empty as requested
		Version:     1,
		Status:      "active",
		UploadedAt:  now,
		ModifiedAt:  now,
	}

	if err := models.CreateFile(fs.db, file); err != nil {
		return nil, fmt.Errorf("failed to create file metadata: %w", err)
	}

	return file, nil
}

// CreateFileMetadataWithKey creates metadata for an uploaded file with encrypted key
func (fs *FileService) CreateFileMetadataWithKey(ownerID, filename, storagePath string, size int64, encryptedKey, ephemeralPubKey []byte) (*models.File, error) {
	// Generate UUID for the file
	fileID := uuid.New().String()

	// Get file extension from the encrypted filename
	extension := ""
	originalExtension := ""

	// Extract original extension before .zst.enc or .tar.zst.enc
	if strings.HasSuffix(filename, ".zst.enc") {
		// Remove .zst.enc to get original name
		originalName := strings.TrimSuffix(filename, ".zst.enc")
		if dotIndex := strings.LastIndex(originalName, "."); dotIndex != -1 {
			originalExtension = originalName[dotIndex+1:]
		}
		extension = "enc"
	} else if strings.HasSuffix(filename, ".tar.zst.enc") {
		// This is a compressed folder
		originalExtension = "tar.zst"
		extension = "enc"
	} else {
		// Regular file
		if dotIndex := strings.LastIndex(filename, "."); dotIndex != -1 {
			extension = filename[dotIndex+1:]
			originalExtension = extension
		}
	}

	// Get MIME type (simplified)
	mimeType := "application/octet-stream"
	if originalExtension != "" {
		switch strings.ToLower(originalExtension) {
		case "txt":
			mimeType = "text/plain"
		case "jpg", "jpeg":
			mimeType = "image/jpeg"
		case "png":
			mimeType = "image/png"
		case "pdf":
			mimeType = "application/pdf"
		case "zip":
			mimeType = "application/zip"
		}
	}

	// Calculate checksum (simplified - in real implementation, calculate from file content)
	checksum := fs.calculateChecksum(storagePath)

	now := time.Now().UTC().Format(time.RFC3339)

	file := &models.File{
		ID:                fileID,
		OwnerID:           ownerID,
		Filename:          filename,
		Extension:         extension,
		OriginalExtension: originalExtension,
		MimeType:          mimeType,
		Size:              size,
		OriginalSize:      0, // Will be set by caller if known
		Checksum:          checksum,
		StoragePath:       storagePath,
		EncryptedKey:      encryptedKey,
		EphemeralPubKey:   ephemeralPubKey,
		NodeID:            "", // Leave empty as requested
		Version:           1,
		Status:            "active",
		IsIndexed:         false,
		ParentArchiveID:   nil,
		RelativePath:      "",
		UploadedAt:        now,
		ModifiedAt:        now,
	}

	if err := models.CreateFile(fs.db, file); err != nil {
		return nil, fmt.Errorf("failed to create file metadata: %w", err)
	}

	return file, nil
}

// CreateFolderMetadata creates metadata for an uploaded folder
func (fs *FileService) CreateFolderMetadata(ownerID, folderName string) (*models.Folder, error) {
	// Generate UUID for the folder
	folderID := uuid.New().String()

	folder := &models.Folder{
		ID:        folderID,
		OwnerID:   ownerID,
		Name:      folderName,
		CreatedAt: time.Now(),
	}

	if err := models.CreateFolder(fs.db, folder); err != nil {
		return nil, fmt.Errorf("failed to create folder metadata: %w", err)
	}

	return folder, nil
}

// CreateFileMetadataWithFolder creates metadata for a file and associates it with a folder
func (fs *FileService) CreateFileMetadataWithFolder(ownerID, filename, storagePath string, size int64, folderID *string, encryptedKey, ephemeralPubKey []byte) (*models.File, error) {
	file, err := fs.CreateFileMetadataWithKey(ownerID, filename, storagePath, size, encryptedKey, ephemeralPubKey)
	if err != nil {
		return nil, err
	}

	if folderID != nil {
		if err := fs.AssociateFileWithFolder(file.ID, *folderID); err != nil {
			return nil, fmt.Errorf("failed to associate file with folder: %w", err)
		}
	}

	return file, nil
}

// calculateChecksum calculates a simple checksum for the file
func (fs *FileService) calculateChecksum(filePath string) string {
	data, err := os.ReadFile(filePath)
	if err != nil {
		// Return empty checksum if file can't be read
		return ""
	}

	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
}

// GetFilesByFolder returns all files in a folder
func (fs *FileService) GetFilesByFolder(folderID string) ([]*models.File, error) {
	return models.GetFilesByFolderID(fs.db, folderID)
}

// GetFileByID returns a file by ID
func (fs *FileService) GetFileByID(fileID string) (*models.File, error) {
	return models.GetFileByID(fs.db, fileID)
}

// AssociateFileWithFolder associates a file with a folder
func (fs *FileService) AssociateFileWithFolder(fileID, folderID string) error {
	query := `UPDATE files SET folder_id = ? WHERE id = ?`
	_, err := fs.db.Exec(query, folderID, fileID)
	return err
}
