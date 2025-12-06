package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"

	"github.com/google/uuid"
)

// ContentItem is an alias for FileMetadata used in favorites/recent
type ContentItem = FileMetadata

// FavoritesListResponse is the response for listing favorites
type FavoritesListResponse struct {
	Items      []ContentItem `json:"items"`
	TotalCount int           `json:"total_count"`
}

// RecentListResponse is the response for listing recent items
type RecentListResponse struct {
	Items      []ContentItem `json:"items"`
	TotalCount int           `json:"total_count"`
}

// AddFavoriteRequest is the request to add an item to favorites
type AddFavoriteRequest struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "file" or "folder"
}

// ListFavoritesHandler returns all favorite items for the current user
func ListFavoritesHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)

	var items []ContentItem

	// Get favorite files
	fileRows, err := globalDB.Query(`
		SELECT f.id, f.filename, f.size, f.mime_type, f.modified_at, f.node_id, fav.created_at
		FROM favorites fav
		JOIN files f ON fav.file_id = f.id
		WHERE fav.user_id = ? AND f.deleted_at IS NULL
		ORDER BY fav.created_at DESC
	`, userID)
	if err != nil {
		log.Printf("Failed to query favorite files: %v", err)
		http.Error(w, "Failed to retrieve favorites", http.StatusInternalServerError)
		return
	}
	defer fileRows.Close()

	for fileRows.Next() {
		var item ContentItem
		var modifiedAt, addedAt string
		var mimeType, nodeID sql.NullString
		
		if err := fileRows.Scan(&item.ID, &item.Name, &item.Size, &mimeType, &modifiedAt, &nodeID, &addedAt); err != nil {
			continue
		}

		storedName := item.Name
		item.Type = "file"
		if mimeType.Valid {
			item.MimeType = mimeType.String
		}
		item.IsCompressed = isCompressedStoredName(storedName)
		item.Name = displayNameForStored(storedName)
		if item.IsCompressed {
			item.FileType = "archive"
		} else {
			item.FileType = classifyFileType(item.MimeType, item.Name)
		}
		item.Previewable = !item.IsCompressed && isPreviewable(item.MimeType, item.FileType)
		item.Modified = formatTime(modifiedAt)
		if nodeID.Valid {
			item.NodeID = nodeID.String
		}

		items = append(items, item)
	}

	// Get favorite folders
	folderRows, err := globalDB.Query(`
		SELECT f.id, f.name, f.total_size, f.modified_at, f.total_files, f.total_folders, fav.created_at
		FROM favorites fav
		JOIN folders f ON fav.folder_id = f.id
		WHERE fav.user_id = ? AND f.deleted_at IS NULL
		ORDER BY fav.created_at DESC
	`, userID)
	if err != nil {
		log.Printf("Failed to query favorite folders: %v", err)
		http.Error(w, "Failed to retrieve favorites", http.StatusInternalServerError)
		return
	}
	defer folderRows.Close()

	for folderRows.Next() {
		var item ContentItem
		var updatedAt, addedAt string
		var totalFiles, totalFolders int
		
		if err := folderRows.Scan(&item.ID, &item.Name, &item.Size, &updatedAt, &totalFiles, &totalFolders, &addedAt); err != nil {
			continue
		}

		item.Type = "folder"
		item.Modified = formatTime(updatedAt)
		item.TotalFiles = totalFiles
		item.TotalFolders = totalFolders

		items = append(items, item)
	}

	response := FavoritesListResponse{
		Items:      items,
		TotalCount: len(items),
	}

	if response.Items == nil {
		response.Items = []ContentItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// AddFavoriteHandler adds a file or folder to favorites
func AddFavoriteHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)

	var req AddFavoriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ID == "" || req.Type == "" {
		http.Error(w, "ID and type are required", http.StatusBadRequest)
		return
	}

	id := uuid.New().String()
	var fileID, folderID interface{}

	if req.Type == "file" {
		// Verify file exists and belongs to user
		var ownerID string
		err := globalDB.QueryRow(`SELECT owner_id FROM files WHERE id = ? AND deleted_at IS NULL`, req.ID).Scan(&ownerID)
		if err == sql.ErrNoRows {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		} else if err != nil {
			log.Printf("Error checking file: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if ownerID != userID {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}
		fileID = req.ID
	} else if req.Type == "folder" {
		// Verify folder exists and belongs to user
		var ownerID string
		err := globalDB.QueryRow(`SELECT owner_id FROM folders WHERE id = ? AND deleted_at IS NULL`, req.ID).Scan(&ownerID)
		if err == sql.ErrNoRows {
			http.Error(w, "Folder not found", http.StatusNotFound)
			return
		} else if err != nil {
			log.Printf("Error checking folder: %v", err)
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if ownerID != userID {
			http.Error(w, "Unauthorized", http.StatusForbidden)
			return
		}
		folderID = req.ID
	} else {
		http.Error(w, "Invalid type", http.StatusBadRequest)
		return
	}

	// Add to favorites (ignore if already exists)
	_, err := globalDB.Exec(`
		INSERT OR IGNORE INTO favorites (id, user_id, file_id, folder_id)
		VALUES (?, ?, ?, ?)
	`, id, userID, fileID, folderID)

	if err != nil {
		log.Printf("Failed to add favorite: %v", err)
		http.Error(w, "Failed to add favorite", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Added to favorites",
	})
}

// RemoveFavoriteHandler removes a file or folder from favorites
func RemoveFavoriteHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)

	var req AddFavoriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ID == "" || req.Type == "" {
		http.Error(w, "ID and type are required", http.StatusBadRequest)
		return
	}

	var result sql.Result
	var err error

	if req.Type == "file" {
		result, err = globalDB.Exec(`DELETE FROM favorites WHERE user_id = ? AND file_id = ?`, userID, req.ID)
	} else if req.Type == "folder" {
		result, err = globalDB.Exec(`DELETE FROM favorites WHERE user_id = ? AND folder_id = ?`, userID, req.ID)
	} else {
		http.Error(w, "Invalid type", http.StatusBadRequest)
		return
	}

	if err != nil {
		log.Printf("Failed to remove favorite: %v", err)
		http.Error(w, "Failed to remove favorite", http.StatusInternalServerError)
		return
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		http.Error(w, "Favorite not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Removed from favorites",
	})
}

// ListRecentHandler returns recently accessed items for the current user
func ListRecentHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)

	var items []ContentItem

	// Get recent files (ordered by last_accessed or uploaded_at if last_accessed is NULL)
	fileRows, err := globalDB.Query(`
		SELECT id, filename, size, mime_type, modified_at, node_id, 
		       COALESCE(last_accessed, uploaded_at) as access_time
		FROM files
		WHERE owner_id = ? AND deleted_at IS NULL
		ORDER BY access_time DESC
		LIMIT 50
	`, userID)
	if err != nil {
		log.Printf("Failed to query recent files: %v", err)
		http.Error(w, "Failed to retrieve recent items", http.StatusInternalServerError)
		return
	}
	defer fileRows.Close()

	for fileRows.Next() {
		var item ContentItem
		var modifiedAt, accessTime string
		var mimeType, nodeID sql.NullString
		
		if err := fileRows.Scan(&item.ID, &item.Name, &item.Size, &mimeType, &modifiedAt, &nodeID, &accessTime); err != nil {
			continue
		}

		storedName := item.Name
		item.Type = "file"
		if mimeType.Valid {
			item.MimeType = mimeType.String
		}
		item.IsCompressed = isCompressedStoredName(storedName)
		item.Name = displayNameForStored(storedName)
		if item.IsCompressed {
			item.FileType = "archive"
		} else {
			item.FileType = classifyFileType(item.MimeType, item.Name)
		}
		item.Previewable = !item.IsCompressed && isPreviewable(item.MimeType, item.FileType)
		item.Modified = formatTime(accessTime)
		if nodeID.Valid {
			item.NodeID = nodeID.String
		}

		items = append(items, item)
	}

	response := RecentListResponse{
		Items:      items,
		TotalCount: len(items),
	}

	if response.Items == nil {
		response.Items = []ContentItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
