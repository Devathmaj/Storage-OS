package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"storageos/controller/core/compress"
	"storageos/controller/core/crypto"
)

const (
	processedFilesDir = "testing/files"
)

// AcceptFile handles an accepted uploaded file.
// It optionally compresses the file using Zstandard, encrypts it using AES-256-GCM,
// and encrypts the AES key using X25519 key exchange.
// Flow: Original -> (Optional Zstd) -> AES-GCM Encrypted -> Store (with encrypted key metadata)
func AcceptFile(ctx context.Context, filePath string, ownerID string, compressBeforeEncrypt bool) error {
	_ = ctx

	log.Printf("Processing file: %s for owner: %s", filePath, ownerID)

	// Get current working directory
	currentDir, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	// Ensure processed files directory exists (in controller/testing/files)
	processedDir := filepath.Join(currentDir, processedFilesDir)
	if err := os.MkdirAll(processedDir, 0755); err != nil {
		return fmt.Errorf("failed to create processed files directory: %w", err)
	}

	baseName := filepath.Base(filePath)

	archiveSourcePath := filePath
	var compressedPath string
	var encryptedFilename string

	if compressBeforeEncrypt {
		// Step 1: Compress the file using Zstandard
		compressedPath = filepath.Join(processedDir, baseName+".zst")
		log.Printf("Compressing file to: %s", compressedPath)
		if err := compress.CompressFile(filePath, compressedPath, 3); err != nil {
			return fmt.Errorf("compression failed: %w", err)
		}
		archiveSourcePath = compressedPath
		encryptedFilename = baseName + ".zst.enc"
	} else {
		log.Printf("Compression disabled for %s, encrypting original bytes only", baseName)
		encryptedFilename = baseName + ".enc"
	}

	// Step 2: Generate a random 32-byte AES-256 key for this file
	aesKey := make([]byte, 32)
	if _, err := rand.Read(aesKey); err != nil {
		return fmt.Errorf("failed to generate AES key: %w", err)
	}
	log.Printf("Generated AES-256 key: %s", hex.EncodeToString(aesKey))

	// Step 3: Encrypt the (possibly compressed) file using AES-256-GCM
	encryptedPath := filepath.Join(processedDir, encryptedFilename)
	log.Printf("Encrypting file to: %s", encryptedPath)
	if err := crypto.EncryptFile(archiveSourcePath, encryptedPath, aesKey); err != nil {
		return fmt.Errorf("AES encryption failed: %w", err)
	}

	// Step 4: Encrypt the AES key using X25519
	// Get the hardcoded public key (receiver's public key)
	receiverPub := crypto.GetHardcodedPublicKey()
	
	// Generate ephemeral key pair for this encryption
	ephemeralPriv, ephemeralPub, err := crypto.GenerateX25519KeyPair()
	if err != nil {
		return fmt.Errorf("failed to generate ephemeral keypair: %w", err)
	}
	
	log.Printf("Ephemeral public key: %s", hex.EncodeToString(ephemeralPub))

	// Derive shared secret and then AES key for encrypting the file's AES key
	shared, err := crypto.DeriveSharedSecret(ephemeralPriv, receiverPub)
	if err != nil {
		return fmt.Errorf("failed to derive shared secret: %w", err)
	}

	keyEncryptionKey, err := crypto.DeriveAES256KeyFromShared(shared, []byte("file-key-encryption-v1"))
	if err != nil {
		return fmt.Errorf("failed to derive key encryption key: %w", err)
	}

	// Encrypt the AES key directly (no file I/O needed)
	encryptedKey, err := crypto.EncryptData(aesKey, keyEncryptionKey)
	if err != nil {
		return fmt.Errorf("failed to encrypt AES key: %w", err)
	}

	log.Printf("Encrypted AES key stored in metadata (length: %d bytes)", len(encryptedKey))
	log.Printf("Ephemeral public key (for decryption): %s", hex.EncodeToString(ephemeralPub))

	// Step 5: Create file metadata in database
	fileInfo, err := os.Stat(encryptedPath)
	if err != nil {
		return fmt.Errorf("failed to get file info: %w", err)
	}

	fileMetadata, err := fileService.CreateFileMetadataWithKey(ownerID, baseName, encryptedPath, fileInfo.Size(), encryptedKey, ephemeralPub)
	if err != nil {
		return fmt.Errorf("failed to create file metadata: %w", err)
	}

	log.Printf("Created file metadata: ID=%s, Size=%d", fileMetadata.ID, fileMetadata.Size)

	// Step 6: Clean up intermediate files
	os.Remove(filePath) // Remove original uploaded file
	if compressBeforeEncrypt && compressedPath != "" {
		os.Remove(compressedPath) // Remove compressed intermediate
	}

	// Store metadata (in real implementation, save to DB)
	log.Printf("File processing complete!")
	log.Printf("  - Encrypted file: %s", encryptedPath)
	log.Printf("  - Encrypted key: stored in metadata")
	log.Printf("  - Ephemeral pub key: stored in metadata")
	log.Printf("  - Owner: %s", ownerID)

	// TODO: Store metadata in database:
	// - encryptedPath (final file location)
	// - encryptedKey (encrypted AES key in metadata)
	// - ephemeralPub (ephemeral public key in metadata)
	// - ownerID, filename, size, timestamps, etc.

	return nil
}
