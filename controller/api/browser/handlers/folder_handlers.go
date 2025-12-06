package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

var globalDB *sql.DB

// InitFolderHandlers initializes folder handlers with database
func InitFolderHandlers(database *sql.DB) {
	globalDB = database
}

// CreateFolderRequest represents the request to create a new folder
type CreateFolderRequest struct {
	Name     string `json:"name"`
	ParentID string `json:"parent_id"` // empty for root level
}

// CreateFolderHandler creates a new folder in the controller metadata
func CreateFolderHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)

	var req CreateFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Folder name is required", http.StatusBadRequest)
		return
	}

	// Validate parent folder exists and belongs to user if specified
	if req.ParentID != "" {
		var parentOwnerID string
		err := globalDB.QueryRow(`
			SELECT owner_id FROM folders WHERE id = ?
		`, req.ParentID).Scan(&parentOwnerID)

		if err == sql.ErrNoRows {
			http.Error(w, "Parent folder not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if parentOwnerID != userID {
			http.Error(w, "Access denied to parent folder", http.StatusForbidden)
			return
		}
	}

	// Create folder
	folderID := uuid.New().String()
	now := time.Now().Format(time.RFC3339)

	var parentIDValue interface{}
	if req.ParentID != "" {
		parentIDValue = req.ParentID
	} else {
		parentIDValue = nil
	}

	_, err := globalDB.Exec(`
		INSERT INTO folders (
			id, owner_id, parent_id, name,
			total_files, total_folders, total_size,
			created_at, modified_at
		) VALUES (?, ?, ?, ?, 0, 0, 0, datetime('now'), datetime('now'))
	`, folderID, userID, parentIDValue, req.Name)

	if err != nil {
		http.Error(w, "Failed to create folder", http.StatusInternalServerError)
		return
	}

	// Update parent folder's subfolder count
	if req.ParentID != "" {
		_, _ = globalDB.Exec(`
			UPDATE folders 
			SET total_folders = total_folders + 1,
				modified_at = datetime('now')
			WHERE id = ?
		`, req.ParentID)
	}

	response := map[string]interface{}{
		"id":         folderID,
		"name":       req.Name,
		"type":       "folder",
		"parent_id":  req.ParentID,
		"created_at": now,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListContentsRequest represents the request to list folder contents
type ListContentsRequest struct {
	FolderID string `json:"folder_id"` // empty for root level
}

// FileMetadata represents file metadata for frontend
type FileMetadata struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Type         string `json:"type"` // "file" or "folder"
	FileType     string `json:"file_type,omitempty"`
	Size         int64  `json:"size,omitempty"`
	Modified     string `json:"modified"`
	NodeID       string `json:"node_id,omitempty"`
	ParentID     string `json:"parent_id,omitempty"`
	TotalFiles   int    `json:"total_files,omitempty"`
	TotalFolders int    `json:"total_folders,omitempty"`
	IsCompressed bool   `json:"is_compressed,omitempty"`
	MimeType     string `json:"mime_type,omitempty"`
	Previewable  bool   `json:"previewable,omitempty"`
	IsFavorite   bool   `json:"is_favorite,omitempty"`
}

// ListContentsHandler lists files and folders in a directory from controller metadata
func ListContentsHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)
	folderID := r.URL.Query().Get("folder_id")

	if folderID != "" {
		// Verify folder exists and user has access
		var folderOwnerID string
		err := globalDB.QueryRow(`
			SELECT owner_id FROM folders WHERE id = ? AND deleted_at IS NULL
		`, folderID).Scan(&folderOwnerID)

		if err == sql.ErrNoRows {
			http.Error(w, "Folder not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if folderOwnerID != userID {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}
	}

	var contents []FileMetadata

	// Fetch folders (exclude trashed items) with favorite status
	folderQuery := `
		SELECT f.id, f.name, f.total_files, f.total_folders, f.total_size, f.modified_at,
			   CASE WHEN fav.id IS NOT NULL THEN 1 ELSE 0 END as is_favorite
		FROM folders f
		LEFT JOIN favorites fav ON fav.folder_id = f.id AND fav.user_id = ?
		WHERE f.owner_id = ? AND f.deleted_at IS NULL AND `
	var folderArgs []interface{}
	if folderID == "" {
		folderQuery += "f.parent_id IS NULL"
		folderArgs = []interface{}{userID, userID}
	} else {
		folderQuery += "f.parent_id = ?"
		folderArgs = []interface{}{userID, userID, folderID}
	}
	folderQuery += " ORDER BY f.name"

	folderRows, err := globalDB.Query(folderQuery, folderArgs...)
	if err != nil {
		http.Error(w, "Failed to query folders", http.StatusInternalServerError)
		return
	}
	defer folderRows.Close()

	for folderRows.Next() {
		var meta FileMetadata
		var modifiedAt string
		var isFavorite int
		if err := folderRows.Scan(&meta.ID, &meta.Name, &meta.TotalFiles, &meta.TotalFolders, &meta.Size, &modifiedAt, &isFavorite); err != nil {
			continue
		}
		meta.Type = "folder"
		meta.Modified = formatTime(modifiedAt)
		meta.IsFavorite = isFavorite == 1
		if folderID != "" {
			meta.ParentID = folderID
		}
		contents = append(contents, meta)
	}

	// Fetch files (exclude trashed items) with favorite status
	fileQuery := `
		SELECT f.id, f.filename, f.mime_type, f.size, f.modified_at, f.node_id,
			   CASE WHEN fav.id IS NOT NULL THEN 1 ELSE 0 END as is_favorite
		FROM files f
		LEFT JOIN favorites fav ON fav.file_id = f.id AND fav.user_id = ?
		WHERE f.owner_id = ? AND f.deleted_at IS NULL AND `
	var fileArgs []interface{}
	if folderID == "" {
		fileQuery += "f.folder_id IS NULL"
		fileArgs = []interface{}{userID, userID}
	} else {
		fileQuery += "f.folder_id = ?"
		fileArgs = []interface{}{userID, userID, folderID}
	}
	fileQuery += " ORDER BY f.filename"

	fileRows, err := globalDB.Query(fileQuery, fileArgs...)
	if err != nil {
		http.Error(w, "Failed to query files", http.StatusInternalServerError)
		return
	}
	defer fileRows.Close()

	for fileRows.Next() {
		var meta FileMetadata
		var modifiedAt string
		var nodeID sql.NullString
		var mimeType sql.NullString
		var isFavorite int
		if err := fileRows.Scan(&meta.ID, &meta.Name, &mimeType, &meta.Size, &modifiedAt, &nodeID, &isFavorite); err != nil {
			continue
		}
		storedName := meta.Name
		meta.Type = "file"
		if mimeType.Valid {
			meta.MimeType = mimeType.String
		}
		meta.IsCompressed = isCompressedStoredName(storedName)
		meta.Name = displayNameForStored(storedName)
		if meta.IsCompressed {
			meta.FileType = "archive"
		} else {
			meta.FileType = classifyFileType(meta.MimeType, meta.Name)
		}
		meta.Previewable = !meta.IsCompressed && isPreviewable(meta.MimeType, meta.FileType)
		meta.IsFavorite = isFavorite == 1
		if nodeID.Valid {
			meta.NodeID = nodeID.String
		}
		meta.Modified = formatTime(modifiedAt)
		if folderID != "" {
			meta.ParentID = folderID
		}
		contents = append(contents, meta)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"contents":  contents,
		"folder_id": folderID,
	})
}

// formatTime converts RFC3339 timestamp to human-readable format
func formatTime(timestamp string) string {
	t, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return timestamp
	}

	now := time.Now()
	diff := now.Sub(t)

	if diff < time.Minute {
		return "Just now"
	} else if diff < time.Hour {
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return fmt.Sprintf("%d minutes ago", mins)
	} else if diff < 24*time.Hour {
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return fmt.Sprintf("%d hours ago", hours)
	} else if diff < 48*time.Hour {
		return "Yesterday"
	} else if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	} else if diff < 30*24*time.Hour {
		weeks := int(diff.Hours() / 24 / 7)
		if weeks == 1 {
			return "1 week ago"
		}
		return fmt.Sprintf("%d weeks ago", weeks)
	} else {
		return t.Format("Jan 2, 2006")
	}
}

func classifyFileType(mimeType, filename string) string {
	mime := strings.ToLower(mimeType)
	name := strings.ToLower(filename)
	switch {
	case strings.HasPrefix(mime, "image/"):
		return "image"
	case strings.HasPrefix(mime, "video/"):
		return "video"
	case strings.HasPrefix(mime, "audio/"):
		return "audio"
	case strings.HasPrefix(mime, "text/") || strings.Contains(mime, "json"):
		return "text"
	case strings.Contains(mime, "pdf") || strings.Contains(mime, "document"):
		return "document"
	case isArchiveFilename(name):
		return "archive"
	default:
		return "file"
	}
}

func isPreviewable(mimeType, fileType string) bool {
	if fileType == "archive" {
		return false
	}
	if mimeType == "" {
		return fileType == "image" || fileType == "video" || fileType == "audio" || fileType == "text"
	}
	mime := strings.ToLower(mimeType)
	switch {
	case strings.HasPrefix(mime, "image/"):
		return true
	case strings.HasPrefix(mime, "video/"):
		return true
	case strings.HasPrefix(mime, "audio/"):
		return true
	case strings.HasPrefix(mime, "text/"):
		return true
	case mime == "application/pdf" || strings.Contains(mime, "json"):
		return true
	default:
		return false
	}
}

func isArchiveFilename(name string) bool {
	clean := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(clean, ".tar.zst.enc") || strings.HasSuffix(clean, ".tar.zst") || strings.HasSuffix(clean, ".tar.enc") || strings.HasSuffix(clean, ".tar")
}

func isCompressedStoredName(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(lower, ".zst.enc") || strings.HasSuffix(lower, ".tar.enc")
}

func displayNameForStored(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".tar.zst.enc"):
		return name[:len(name)-len(".tar.zst.enc")]
	case strings.HasSuffix(lower, ".zst.enc"):
		return name[:len(name)-len(".zst.enc")]
	case strings.HasSuffix(lower, ".tar.enc"):
		return name[:len(name)-len(".tar.enc")]
	case strings.HasSuffix(lower, ".enc"):
		return name[:len(name)-len(".enc")]
	default:
		return name
	}
}

// DeleteItemHandler moves a file or folder to trash (soft delete)
func DeleteItemHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)
	itemID := r.URL.Query().Get("id")
	itemType := r.URL.Query().Get("type") // "file" or "folder"

	if itemID == "" || itemType == "" {
		http.Error(w, "Item ID and type are required", http.StatusBadRequest)
		return
	}

	now := time.Now().Format(time.RFC3339)

	if itemType == "file" {
		// Verify ownership
		var ownerID string
		err := globalDB.QueryRow(`SELECT owner_id FROM files WHERE id = ? AND deleted_at IS NULL`, itemID).Scan(&ownerID)
		if err == sql.ErrNoRows {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if ownerID != userID {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		// Move to trash (soft delete)
		_, err = globalDB.Exec(`UPDATE files SET deleted_at = ? WHERE id = ?`, now, itemID)

		if err != nil {
			http.Error(w, "Failed to move file to trash", http.StatusInternalServerError)
			return
		}

	} else if itemType == "folder" {
		// Verify ownership
		var ownerID string
		err := globalDB.QueryRow(`SELECT owner_id FROM folders WHERE id = ? AND deleted_at IS NULL`, itemID).Scan(&ownerID)
		if err == sql.ErrNoRows {
			http.Error(w, "Folder not found", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if ownerID != userID {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		// Move folder to trash (recursively)
		if err := moveFolderToTrashRecursive(itemID, now); err != nil {
			http.Error(w, "Failed to move folder to trash", http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(w, "Invalid item type", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Item deleted successfully",
	})
}

// moveFolderToTrashRecursive moves a folder and all its contents to trash
func moveFolderToTrashRecursive(folderID, deletedAt string) error {
	// Move the folder itself
	_, err := globalDB.Exec(`UPDATE folders SET deleted_at = ? WHERE id = ?`, deletedAt, folderID)
	if err != nil {
		return err
	}

	// Move all files in this folder
	_, err = globalDB.Exec(`UPDATE files SET deleted_at = ? WHERE folder_id = ? AND deleted_at IS NULL`, deletedAt, folderID)
	if err != nil {
		return err
	}

	// Get all subfolders and recursively move them
	rows, err := globalDB.Query(`SELECT id FROM folders WHERE parent_id = ? AND deleted_at IS NULL`, folderID)
	if err != nil {
		return err
	}
	defer rows.Close()

	var subfolderIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		subfolderIDs = append(subfolderIDs, id)
	}

	for _, subID := range subfolderIDs {
		if err := moveFolderToTrashRecursive(subID, deletedAt); err != nil {
			return err
		}
	}

	return nil
}
