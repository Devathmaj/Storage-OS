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

	"storageos/controller/core/compress"
	"storageos/controller/core/crypto"
	"storageos/controller/models"
)

const (
	processedFoldersDir = "testing/folders"
)

// AcceptFolder processes an uploaded folder.
// It can optionally compress the entire folder into a tar.zst archive (or keep it as raw tar),
// encrypts the archive using AES-256-GCM, encrypts the AES key using X25519,
// and creates metadata for the folder.
// Flow: Entire Folder -> (Tar | Tar.Zstd) -> AES-GCM Encrypted -> Store (as single file with encrypted key metadata)
func AcceptFolder(ctx context.Context, folderPath string, ownerID string, folderName string, compressBeforeEncrypt bool, parentFolderID *string) error {
	_ = ctx

	log.Printf("Processing folder: %s (name: %s) for owner: %s (parent=%v)", folderPath, folderName, ownerID, parentFolderID)

	// Count files in folder for logging
	fileCount := 0
	filepath.Walk(folderPath, func(path string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			fileCount++
		}
		return nil
	})

	// Ensure processed folders directory exists (in controller/testing/folders)
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	processedDir := filepath.Join(currentDir, processedFoldersDir)
	if err := os.MkdirAll(processedDir, 0755); err != nil {
		return fmt.Errorf("failed to create processed folders directory: %w", err)
	}

	archiveSuffix := ".tar"
	if compressBeforeEncrypt {
		archiveSuffix = ".tar.zst"
	}
	archivePath := filepath.Join(processedDir, fmt.Sprintf("%s%s", folderName, archiveSuffix))

	if compressBeforeEncrypt {
		log.Printf("Compressing folder to: %s", archivePath)
		if err := compress.CompressFolder(folderPath, archivePath, 3); err != nil {
			return fmt.Errorf("compression failed: %w", err)
		}
	} else {
		log.Printf("Compression disabled for folder %s, creating raw tar archive: %s", folderName, archivePath)
		if err := compress.ArchiveFolder(folderPath, archivePath); err != nil {
			return fmt.Errorf("tar archive creation failed: %w", err)
		}
	}

	// Step 2: Generate a random 32-byte AES-256 key for the folder
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		return fmt.Errorf("failed to generate AES key: %w", err)
	}

	// Step 3: Encrypt the archive using AES-256-GCM
	encryptedPath := archivePath + ".enc"
	log.Printf("Encrypting folder to: %s", encryptedPath)
	if err := crypto.EncryptFile(archivePath, encryptedPath, aesKey); err != nil {
		return fmt.Errorf("AES encryption failed: %w", err)
	}

	// Step 4: Encrypt the AES key using X25519
	receiverPub := crypto.GetHardcodedPublicKey()
	ephemeralPriv, ephemeralPub, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate ephemeral keypair: %w", err)
	}

	shared, err := crypto.DeriveSharedSecret(ephemeralPriv, receiverPub)
	if err != nil {
		return fmt.Errorf("failed to derive shared secret: %w", err)
	}

	keyEncryptionKey, err := crypto.DeriveAES256KeyFromShared(shared, []byte("folder-key-encryption-v1"))
	if err != nil {
		return fmt.Errorf("failed to derive key encryption key: %w", err)
	}

	// Encrypt the AES key
	encryptedKey, err := crypto.EncryptData(aesKey, keyEncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt AES key: %w", err)
	}

	log.Printf("Encrypted AES key stored in metadata (length: %d bytes)", len(encryptedKey))
	log.Printf("Ephemeral public key (for decryption): %x", ephemeralPub)

	// Step 5: Create folder metadata in database (stored as a single "file" entry)
	fileInfo, err := os.Stat(encryptedPath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	encHandle, err := os.Open(encryptedPath)
	if err != nil {
		return fmt.Errorf("failed to open encrypted archive: %w", err)
	}

	// Calculate checksum of encrypted archive
	hasher := sha256.New()
	if _, err := io.Copy(hasher, encHandle); err != nil {
		return fmt.Errorf("failed to checksum archive: %w", err)
	}
	checksum := fmt.Sprintf("%x", hasher.Sum(nil))
	if _, err := encHandle.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed to rewind archive: %w", err)
	}

	parentIDValue := ""
	var folderIDPtr *string
	if parentFolderID != nil && *parentFolderID != "" {
		parentIDValue = *parentFolderID
		folderIDPtr = parentFolderID
	}

	archiveFilename := filepath.Base(encryptedPath)
	osMetadata, err := getOSNodeClient().UploadToNode(
		ownerID,
		parentIDValue,
		archiveFilename,
		"application/octet-stream",
		checksum,
		encHandle,
		fileInfo.Size(),
	)
	if err != nil {
		return fmt.Errorf("failed to upload archive to OS node: %w", err)
	}
	encHandle.Close()

	// Store folder metadata referencing OS node object
	originalExt := "tar"
	if compressBeforeEncrypt {
		originalExt = "tar.zst"
	}
	controllerFile := &models.File{
		ID:                osMetadata.ID,
		OwnerID:           ownerID,
		FolderID:          folderIDPtr,
		Filename:          archiveFilename,
		Extension:         "enc",
		OriginalExtension: originalExt,
		MimeType:          "application/x-tar",
		Size:              osMetadata.Size,
		OriginalSize:      fileInfo.Size(),
		Checksum:          checksum,
		StoragePath:       osMetadata.StoragePath,
		EncryptedKey:      encryptedKey,
		EphemeralPubKey:   ephemeralPub,
		NodeID:            osMetadata.NodeID,
		Version:           1,
		Status:            "active",
		IsIndexed:         false,
		ParentArchiveID:   nil,
		RelativePath:      "",
	}

	if err := models.CreateFile(fileService.db, controllerFile); err != nil {
		return fmt.Errorf("failed to persist folder archive: %w", err)
	}

	log.Printf("Created folder metadata: ID=%s, Name=%s, Size=%d", controllerFile.ID, controllerFile.Filename, controllerFile.Size)

	// Remove local encrypted archive now that it resides in OS node
	os.Remove(encryptedPath)

	// Step 6: Clean up intermediate files
	os.Remove(archivePath)   // Remove unencrypted archive
	os.RemoveAll(folderPath) // Remove original uploaded folder

	log.Printf("Folder processing complete! Compressed and encrypted %d files in folder %s", fileCount, folderName)
	log.Printf("  - Encrypted folder: %s", encryptedPath)
	log.Printf("  - Encrypted key: stored in metadata")
	log.Printf("  - Ephemeral pub key: stored in metadata")
	log.Printf("  - Owner: %s", ownerID)

	return nil
}
