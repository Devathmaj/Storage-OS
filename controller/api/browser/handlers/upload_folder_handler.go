package handlers

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	services "storageos/controller/services/browser"
)

const (
	maxFolderUploadSize = 0 // No limit
	tempFolderDir       = "/tmp/folder_uploads"
)

// UploadFolderHandler handles folder uploads from the browser client.
// It receives multiple files as multipart form data, creates a temporary
// directory structure, and passes it to the services layer for processing.
func UploadFolderHandler(w http.ResponseWriter, r *http.Request) {
	// Only accept POST
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Create multipart reader manually to avoid size limits
	mr, err := r.MultipartReader()
	if err != nil {
		log.Printf("Error creating multipart reader: %v", err)
		http.Error(w, "Invalid form data", http.StatusBadRequest)
		return
	}

	// Variables to collect form data
	var folderName string
	var parentFolderTarget string
	var compressFolders = true
	var files []*uploadedFile
	fileMetadata := make(map[string]string)
	
	// Ensure temp directory exists
	if err := os.MkdirAll(tempFolderDir, 0755); err != nil {
		log.Printf("Error creating temp directory: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Create a unique temp folder for this upload
	tempFolderPath := filepath.Join(tempFolderDir, fmt.Sprintf("upload_%d", os.Getpid()))
	if err := os.MkdirAll(tempFolderPath, 0755); err != nil {
		log.Printf("Error creating temp folder: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	totalSize := int64(0)
	fileCount := 0

	// Read all parts
	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			log.Printf("Error reading multipart: %v", err)
			http.Error(w, "Error reading upload", http.StatusBadRequest)
			return
		}

		formName := part.FormName()

		switch formName {
		case "folderName":
			buf := make([]byte, 1024)
			n, _ := part.Read(buf)
			folderName = string(buf[:n])
			part.Close()
			continue
		case "compress":
			buf := make([]byte, 10)
			n, _ := part.Read(buf)
			compressFolders = strings.ToLower(strings.TrimSpace(string(buf[:n]))) != "false"
			part.Close()
			continue
		case "folder_id":
			buf := make([]byte, 128)
			n, _ := part.Read(buf)
			parentFolderTarget = strings.TrimSpace(string(buf[:n]))
			part.Close()
			continue
		case "file_metadata":
			payload, err := io.ReadAll(part)
			part.Close()
			if err != nil {
				log.Printf("Error reading file metadata: %v", err)
				http.Error(w, "Invalid metadata", http.StatusBadRequest)
				return
			}
			var entries []fileMetadataEntry
			if err := json.Unmarshal(payload, &entries); err != nil {
				log.Printf("Error parsing file metadata: %v", err)
				http.Error(w, "Invalid metadata", http.StatusBadRequest)
				return
			}
			for _, entry := range entries {
				cleanPath := sanitizeRelativePath(entry.RelativePath)
				fileMetadata[entry.Field] = cleanPath
			}
			normalizeMetadataPaths(fileMetadata)
			continue
		}

		filename := part.FileName()
		if filename == "" {
			part.Close()
			continue
		}

		relativePath := fileMetadata[formName]
		if relativePath == "" {
			relativePath = filename
		}
		relativePath = sanitizeRelativePath(relativePath)
		if relativePath == "" {
			relativePath = filename
		}

		log.Printf("DEBUG: Received file part='%s' resolvedPath='%s'", formName, relativePath)

		// Create nested directories if needed
		filePath := filepath.Join(tempFolderPath, filepath.FromSlash(relativePath))
		fileDir := filepath.Dir(filePath)
		
		if err := os.MkdirAll(fileDir, 0755); err != nil {
			log.Printf("Error creating subdirectory: %v", err)
			part.Close()
			http.Error(w, "Error creating directory structure", http.StatusInternalServerError)
			return
		}

		// Create destination file
		dst, err := os.Create(filePath)
		if err != nil {
			log.Printf("Error creating file: %v", err)
			part.Close()
			http.Error(w, "Error saving file", http.StatusInternalServerError)
			return
		}

		// Copy file contents
		size, err := io.Copy(dst, part)
		dst.Close()
		part.Close()
		
		if err != nil {
			log.Printf("Error saving file: %v", err)
			http.Error(w, "Error saving file", http.StatusInternalServerError)
			return
		}

		totalSize += size
		fileCount++
		files = append(files, &uploadedFile{
			filename: relativePath,
			size:     size,
		})
	}

	// Set defaults
	if folderName == "" {
		folderName = "uploaded_folder"
	}
	folderName = sanitizeFolderName(folderName)
	
	// Get authenticated user ID from context
	claims, ok := r.Context().Value("user").(*services.Claims)
	if !ok || claims == nil {
		log.Printf("Failed to get user from context")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	
	ownerID := claims.UserID

	var parentFolderPtr *string
	if parentFolderTarget != "" {
		var folderOwner string
		err := db.QueryRow(`SELECT owner_id FROM folders WHERE id = ?`, parentFolderTarget).Scan(&folderOwner)
		if err == sql.ErrNoRows {
			log.Printf("Parent folder %s not found", parentFolderTarget)
			http.Error(w, "Parent folder not found", http.StatusNotFound)
			return
		} else if err != nil {
			log.Printf("Failed to verify parent folder: %v", err)
			http.Error(w, "Failed to verify folder", http.StatusInternalServerError)
			return
		}
		if folderOwner != ownerID {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
		parentID := parentFolderTarget
		parentFolderPtr = &parentID
	}

	if fileCount == 0 {
		http.Error(w, "No files provided", http.StatusBadRequest)
		return
	}

	// Automatically determine storage mode based on compression:
	// - If compressed: use quick storage (single archive file, no expansion)
	// - If not compressed: use indexed storage (preserve folder/file structure)
	indexFiles := !compressFolders

	log.Printf("Received folder upload: %s with %d files (%d bytes), indexFiles=%v, compress=%v", folderName, fileCount, totalSize, indexFiles, compressFolders)

	// Pass to service layer for processing (compression + encryption)
	var processErr error
	if indexFiles {
		// Process with file indexing (uncompressed - preserve structure)
		processErr = services.AcceptFolderIndexed(r.Context(), tempFolderPath, ownerID, folderName, compressFolders, parentFolderPtr)
	} else {
		// Process as single archive without indexing (compressed - single file)
		processErr = services.AcceptFolder(r.Context(), tempFolderPath, ownerID, folderName, compressFolders, parentFolderPtr)
	}

	if processErr != nil {
		log.Printf("Error processing folder: %v", processErr)
		http.Error(w, "Error processing folder", http.StatusInternalServerError)
		return
	}

	// Return success response
	response := map[string]interface{}{
		"success":    true,
		"folderName": folderName,
		"fileCount":  fileCount,
		"totalSize":  totalSize,
		"indexed":    indexFiles,
		"message":    "Folder uploaded and processed successfully",
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// sanitizeFolderName removes dangerous characters from folder names
func sanitizeFolderName(name string) string {
	// Remove path separators and dangerous characters
	name = strings.ReplaceAll(name, "/", "_")
	name = strings.ReplaceAll(name, "\\", "_")
	name = strings.ReplaceAll(name, "..", "_")
	name = strings.TrimSpace(name)
	
	if name == "" {
		name = "folder"
	}
	
	return name
}

// uploadedFile tracks file information during upload
type uploadedFile struct {
	filename string
	size     int64
}

type fileMetadataEntry struct {
	Field        string `json:"field"`
	RelativePath string `json:"relativePath"`
}

func sanitizeRelativePath(path string) string {
	path = strings.ReplaceAll(path, "\\", "/")
	path = strings.TrimSpace(path)
	path = strings.Trim(path, "/")
	if path == "" {
		return ""
	}
	parts := strings.Split(path, "/")
	var cleanParts []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." || part == ".." {
			continue
		}
		cleanParts = append(cleanParts, part)
	}
	return strings.Join(cleanParts, "/")
}

func normalizeMetadataPaths(meta map[string]string) {
	root := commonRootFolder(meta)
	if root == "" {
		return
	}

	prefix := root + "/"
	for field, value := range meta {
		if value == "" {
			continue
		}
		if value == root {
			meta[field] = extractLastComponent(value)
			continue
		}
		if strings.HasPrefix(value, prefix) {
			trimmed := strings.TrimPrefix(value, prefix)
			if trimmed == "" {
				trimmed = extractLastComponent(value)
			}
			meta[field] = trimmed
		}
	}
}

func commonRootFolder(meta map[string]string) string {
	root := ""
	for _, value := range meta {
		if value == "" {
			continue
		}
		parts := strings.Split(value, "/")
		if len(parts) <= 1 {
			return ""
		}
		candidate := parts[0]
		if root == "" {
			root = candidate
			continue
		}
		if candidate != root {
			return ""
		}
	}
	return root
}

func extractLastComponent(path string) string {
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return path
	}
	return parts[len(parts)-1]
}

// createZipFromFolder creates a zip file from a directory (unused, kept for reference)
func createZipFromFolder(sourceDir, zipPath string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	zipWriter := zip.NewWriter(zipFile)
	defer zipWriter.Close()

	// Walk through the directory
	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip the root directory itself
		if path == sourceDir {
			return nil
		}

		// Create zip header
		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		// Get relative path
		relativePath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		header.Name = relativePath

		// Set compression method
		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		// If it's a file, write its contents
		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(writer, file)
			return err
		}

		return nil
	})
}
