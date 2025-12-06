package handlers

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
)

// TrashItem represents a file or folder in trash
type TrashItem struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	Type        string    `json:"type"` // "file" or "folder"
	Size        int64     `json:"size"`
	MimeType    string    `json:"mime_type,omitempty"`
	DeletedAt   time.Time `json:"deleted_at"`
	DaysLeft    int       `json:"days_left"` // Days until auto-delete
	ParentID    *string   `json:"parent_id,omitempty"`
	TotalFiles  int       `json:"total_files,omitempty"`  // For folders
	TotalFolders int      `json:"total_folders,omitempty"` // For folders
}

// TrashListResponse is the response for listing trash items
type TrashListResponse struct {
	Items      []TrashItem `json:"items"`
	TotalCount int         `json:"total_count"`
	TotalSize  int64       `json:"total_size"`
}

// MoveToTrashRequest is the request body for moving items to trash
type MoveToTrashRequest struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "file" or "folder"
}

// RestoreFromTrashRequest is the request body for restoring items
type RestoreFromTrashRequest struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "file" or "folder"
}

// PermanentDeleteRequest is the request body for permanent deletion
type PermanentDeleteRequest struct {
	ID   string `json:"id"`
	Type string `json:"type"` // "file" or "folder"
}

// ListTrashHandler returns all items in trash for the current user
func ListTrashHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)

	var items []TrashItem
	var totalSize int64

	// Get trashed files
	fileRows, err := globalDB.Query(`
		SELECT id, filename, size, mime_type, deleted_at, folder_id
		FROM files 
		WHERE owner_id = ? AND deleted_at IS NOT NULL
		ORDER BY deleted_at DESC
	`, userID)
	if err != nil {
		log.Printf("Failed to query trashed files: %v", err)
		http.Error(w, "Failed to retrieve trash", http.StatusInternalServerError)
		return
	}
	defer fileRows.Close()

	now := time.Now()
	for fileRows.Next() {
		var item TrashItem
		var deletedAtStr string
		var mimeType sql.NullString
		var parentID sql.NullString

		if err := fileRows.Scan(&item.ID, &item.Name, &item.Size, &mimeType, &deletedAtStr, &parentID); err != nil {
			log.Printf("Failed to scan file row: %v", err)
			continue
		}

		item.Type = "file"
		if mimeType.Valid {
			item.MimeType = mimeType.String
		}
		if parentID.Valid {
			item.ParentID = &parentID.String
		}

		// Parse deleted_at
		item.DeletedAt, _ = time.Parse(time.RFC3339, deletedAtStr)
		if item.DeletedAt.IsZero() {
			item.DeletedAt, _ = time.Parse("2006-01-02 15:04:05", deletedAtStr)
		}

		// Calculate days left (30 day retention)
		daysSinceDelete := int(now.Sub(item.DeletedAt).Hours() / 24)
		item.DaysLeft = 30 - daysSinceDelete
		if item.DaysLeft < 0 {
			item.DaysLeft = 0
		}

		items = append(items, item)
		totalSize += item.Size
	}

	// Get trashed folders
	folderRows, err := globalDB.Query(`
		SELECT id, name, total_size, deleted_at, parent_id, total_files, total_folders
		FROM folders 
		WHERE owner_id = ? AND deleted_at IS NOT NULL
		ORDER BY deleted_at DESC
	`, userID)
	if err != nil {
		log.Printf("Failed to query trashed folders: %v", err)
		http.Error(w, "Failed to retrieve trash", http.StatusInternalServerError)
		return
	}
	defer folderRows.Close()

	for folderRows.Next() {
		var item TrashItem
		var deletedAtStr string
		var parentID sql.NullString
		var totalFiles, totalFolders int

		if err := folderRows.Scan(&item.ID, &item.Name, &item.Size, &deletedAtStr, &parentID, &totalFiles, &totalFolders); err != nil {
			log.Printf("Failed to scan folder row: %v", err)
			continue
		}

		item.Type = "folder"
		item.TotalFiles = totalFiles
		item.TotalFolders = totalFolders
		if parentID.Valid {
			item.ParentID = &parentID.String
		}

		// Parse deleted_at
		item.DeletedAt, _ = time.Parse(time.RFC3339, deletedAtStr)
		if item.DeletedAt.IsZero() {
			item.DeletedAt, _ = time.Parse("2006-01-02 15:04:05", deletedAtStr)
		}

		// Calculate days left (30 day retention)
		daysSinceDelete := int(now.Sub(item.DeletedAt).Hours() / 24)
		item.DaysLeft = 30 - daysSinceDelete
		if item.DaysLeft < 0 {
			item.DaysLeft = 0
		}

		items = append(items, item)
		totalSize += item.Size
	}

	response := TrashListResponse{
		Items:      items,
		TotalCount: len(items),
		TotalSize:  totalSize,
	}

	if response.Items == nil {
		response.Items = []TrashItem{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// MoveToTrashHandler moves a file or folder to trash (soft delete)
func MoveToTrashHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)

	var req MoveToTrashRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ID == "" || req.Type == "" {
		http.Error(w, "ID and type are required", http.StatusBadRequest)
		return
	}

	now := time.Now().Format(time.RFC3339)

	if req.Type == "file" {
		// Verify ownership
		var ownerID string
		err := globalDB.QueryRow(`SELECT owner_id FROM files WHERE id = ? AND deleted_at IS NULL`, req.ID).Scan(&ownerID)
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

		// Move to trash
		_, err = globalDB.Exec(`UPDATE files SET deleted_at = ? WHERE id = ?`, now, req.ID)
		if err != nil {
			log.Printf("Failed to move file to trash: %v", err)
			http.Error(w, "Failed to move to trash", http.StatusInternalServerError)
			return
		}

	} else if req.Type == "folder" {
		// Verify ownership
		var ownerID string
		err := globalDB.QueryRow(`SELECT owner_id FROM folders WHERE id = ? AND deleted_at IS NULL`, req.ID).Scan(&ownerID)
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

		// Move folder and all contents to trash
		if err := moveFolderToTrash(req.ID, now); err != nil {
			log.Printf("Failed to move folder to trash: %v", err)
			http.Error(w, "Failed to move to trash", http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(w, "Invalid item type", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Item moved to trash",
	})
}

// moveFolderToTrash recursively moves a folder and its contents to trash
func moveFolderToTrash(folderID, deletedAt string) error {
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
		if err := moveFolderToTrash(subID, deletedAt); err != nil {
			return err
		}
	}

	return nil
}

// RestoreFromTrashHandler restores a file or folder from trash
func RestoreFromTrashHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)

	var req RestoreFromTrashRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ID == "" || req.Type == "" {
		http.Error(w, "ID and type are required", http.StatusBadRequest)
		return
	}

	if req.Type == "file" {
		// Verify ownership
		var ownerID string
		err := globalDB.QueryRow(`SELECT owner_id FROM files WHERE id = ? AND deleted_at IS NOT NULL`, req.ID).Scan(&ownerID)
		if err == sql.ErrNoRows {
			http.Error(w, "File not found in trash", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if ownerID != userID {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		// Restore file
		_, err = globalDB.Exec(`UPDATE files SET deleted_at = NULL WHERE id = ?`, req.ID)
		if err != nil {
			log.Printf("Failed to restore file: %v", err)
			http.Error(w, "Failed to restore file", http.StatusInternalServerError)
			return
		}

	} else if req.Type == "folder" {
		// Verify ownership
		var ownerID string
		err := globalDB.QueryRow(`SELECT owner_id FROM folders WHERE id = ? AND deleted_at IS NOT NULL`, req.ID).Scan(&ownerID)
		if err == sql.ErrNoRows {
			http.Error(w, "Folder not found in trash", http.StatusNotFound)
			return
		} else if err != nil {
			http.Error(w, "Database error", http.StatusInternalServerError)
			return
		}

		if ownerID != userID {
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		// Restore folder and all contents
		if err := restoreFolderFromTrash(req.ID); err != nil {
			log.Printf("Failed to restore folder: %v", err)
			http.Error(w, "Failed to restore folder", http.StatusInternalServerError)
			return
		}
	} else {
		http.Error(w, "Invalid item type", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Item restored from trash",
	})
}

// restoreFolderFromTrash recursively restores a folder and its contents
func restoreFolderFromTrash(folderID string) error {
	// Restore the folder itself
	_, err := globalDB.Exec(`UPDATE folders SET deleted_at = NULL WHERE id = ?`, folderID)
	if err != nil {
		return err
	}

	// Restore all files in this folder
	_, err = globalDB.Exec(`UPDATE files SET deleted_at = NULL WHERE folder_id = ?`, folderID)
	if err != nil {
		return err
	}

	// Get all subfolders and recursively restore them
	rows, err := globalDB.Query(`SELECT id FROM folders WHERE parent_id = ?`, folderID)
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
		if err := restoreFolderFromTrash(subID); err != nil {
			return err
		}
	}

	return nil
}

// PermanentDeleteHandler permanently deletes a file or folder
func PermanentDeleteHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)

	var req PermanentDeleteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.ID == "" || req.Type == "" {
		http.Error(w, "ID and type are required", http.StatusBadRequest)
		return
	}

	if req.Type == "file" {
		if err := permanentDeleteFile(req.ID, userID); err != nil {
			log.Printf("Failed to permanently delete file: %v", err)
			if err.Error() == "not found" {
				http.Error(w, "File not found in trash", http.StatusNotFound)
			} else if err.Error() == "access denied" {
				http.Error(w, "Access denied", http.StatusForbidden)
			} else {
				http.Error(w, "Failed to delete file", http.StatusInternalServerError)
			}
			return
		}
	} else if req.Type == "folder" {
		if err := permanentDeleteFolder(req.ID, userID); err != nil {
			log.Printf("Failed to permanently delete folder: %v", err)
			if err.Error() == "not found" {
				http.Error(w, "Folder not found in trash", http.StatusNotFound)
			} else if err.Error() == "access denied" {
				http.Error(w, "Access denied", http.StatusForbidden)
			} else {
				http.Error(w, "Failed to delete folder", http.StatusInternalServerError)
			}
			return
		}
	} else {
		http.Error(w, "Invalid item type", http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Item permanently deleted",
	})
}

// permanentDeleteFile deletes a file and queues storage node deletions
func permanentDeleteFile(fileID, userID string) error {
	// Verify ownership
	var ownerID, nodeID string
	var storagePath sql.NullString
	err := globalDB.QueryRow(`
		SELECT owner_id, node_id, storage_path 
		FROM files WHERE id = ? AND deleted_at IS NOT NULL
	`, fileID).Scan(&ownerID, &nodeID, &storagePath)
	
	if err == sql.ErrNoRows {
		return &customError{msg: "not found"}
	} else if err != nil {
		return err
	}

	if ownerID != userID {
		return &customError{msg: "access denied"}
	}

	// Get file shards for erasure-coded files
	shardRows, err := globalDB.Query(`
		SELECT id, node_id, peer_server_id, storage_path 
		FROM file_shards WHERE file_id = ?
	`, fileID)
	if err != nil {
		return err
	}
	defer shardRows.Close()

	// Queue deletions for each shard
	for shardRows.Next() {
		var shardID string
		var shardNodeID sql.NullString
		var peerServerID sql.NullString
		var shardPath sql.NullString

		if err := shardRows.Scan(&shardID, &shardNodeID, &peerServerID, &shardPath); err != nil {
			continue
		}

		// Check if node/peer is online
		isOnline := false
		if shardNodeID.Valid && shardNodeID.String != "" {
			isOnline = isStorageNodeOnline(shardNodeID.String)
		} else if peerServerID.Valid && peerServerID.String != "" {
			isOnline = isPeerServerOnline(peerServerID.String)
		}

		if isOnline {
			// Try to delete immediately
			if shardNodeID.Valid && shardNodeID.String != "" {
				if err := deleteFromStorageNode(shardNodeID.String, fileID, shardPath.String); err != nil {
					log.Printf("Failed to delete shard from node %s, queuing: %v", shardNodeID.String, err)
					queuePendingDeletion(fileID, shardID, shardNodeID.String, "", shardPath.String, "shard")
				}
			} else if peerServerID.Valid && peerServerID.String != "" {
				if err := deleteFromPeerServer(peerServerID.String, fileID, shardPath.String); err != nil {
					log.Printf("Failed to delete shard from peer %s, queuing: %v", peerServerID.String, err)
					queuePendingDeletion(fileID, shardID, "", peerServerID.String, shardPath.String, "shard")
				}
			}
		} else {
			// Queue for later deletion
			nodeIDStr := ""
			peerIDStr := ""
			if shardNodeID.Valid {
				nodeIDStr = shardNodeID.String
			}
			if peerServerID.Valid {
				peerIDStr = peerServerID.String
			}
			queuePendingDeletion(fileID, shardID, nodeIDStr, peerIDStr, shardPath.String, "shard")
		}
	}

	// Delete the main file from primary node
	if nodeID != "" {
		log.Printf("Attempting to delete file %s from storage node %s", fileID, nodeID)
		
		// Always try to delete, regardless of what the database says about online status
		// The DeleteFromNode will fail if the node is truly unreachable
		if err := deleteFromStorageNode(nodeID, fileID, storagePath.String); err != nil {
			log.Printf("Failed to delete from node %s: %v", nodeID, err)
			
			// Only queue if we can verify the node exists in the database
			// Otherwise, just log the error and continue (file might be already deleted)
			isOnline := isStorageNodeOnline(nodeID)
			if !isOnline {
				log.Printf("Node %s appears to be offline, queuing deletion", nodeID)
				queuePendingDeletion(fileID, "", nodeID, "", storagePath.String, "file")
			} else {
				log.Printf("Node %s is online but deletion failed - file may not exist on node", nodeID)
			}
		} else {
			log.Printf("Successfully deleted file %s from node %s", fileID, nodeID)
		}
	}

	// Delete file shards from DB first (before calling DeleteFromNode)
	_, err = globalDB.Exec(`DELETE FROM file_shards WHERE file_id = ?`, fileID)
	if err != nil {
		log.Printf("Failed to delete file shards from DB: %v", err)
	}

	// Delete from files table
	// Note: deleteFromStorageNode calls DeleteFromNode which also deletes from files table
	// But if the deletion failed or was queued, we still need to remove the database entry
	// Use DELETE to ensure the record is gone
	result, err := globalDB.Exec(`DELETE FROM files WHERE id = ?`, fileID)
	if err != nil {
		log.Printf("Failed to delete file from DB: %v", err)
		return err
	}
	
	rowsAffected, _ := result.RowsAffected()
	log.Printf("Permanent deletion completed for file %s (DB rows affected: %d)", fileID, rowsAffected)
	return nil
}

// permanentDeleteFolder deletes a folder and all its contents
func permanentDeleteFolder(folderID, userID string) error {
	// Verify ownership
	var ownerID string
	err := globalDB.QueryRow(`
		SELECT owner_id FROM folders WHERE id = ? AND deleted_at IS NOT NULL
	`, folderID).Scan(&ownerID)
	
	if err == sql.ErrNoRows {
		return &customError{msg: "not found"}
	} else if err != nil {
		return err
	}

	if ownerID != userID {
		return &customError{msg: "access denied"}
	}

	// Delete all files in this folder
	fileRows, err := globalDB.Query(`SELECT id FROM files WHERE folder_id = ?`, folderID)
	if err != nil {
		return err
	}
	defer fileRows.Close()

	var fileIDs []string
	for fileRows.Next() {
		var id string
		if err := fileRows.Scan(&id); err != nil {
			continue
		}
		fileIDs = append(fileIDs, id)
	}

	for _, fid := range fileIDs {
		if err := permanentDeleteFile(fid, userID); err != nil {
			log.Printf("Warning: failed to delete file %s: %v", fid, err)
		}
	}

	// Recursively delete subfolders
	subRows, err := globalDB.Query(`SELECT id FROM folders WHERE parent_id = ?`, folderID)
	if err != nil {
		return err
	}
	defer subRows.Close()

	var subfolderIDs []string
	for subRows.Next() {
		var id string
		if err := subRows.Scan(&id); err != nil {
			continue
		}
		subfolderIDs = append(subfolderIDs, id)
	}

	for _, subID := range subfolderIDs {
		if err := permanentDeleteFolder(subID, userID); err != nil {
			log.Printf("Warning: failed to delete subfolder %s: %v", subID, err)
		}
	}

	// Delete folder from DB
	_, err = globalDB.Exec(`DELETE FROM folders WHERE id = ?`, folderID)
	return err
}

// EmptyTrashHandler permanently deletes all items in trash
func EmptyTrashHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)

	// Get all trashed files
	fileRows, err := globalDB.Query(`SELECT id FROM files WHERE owner_id = ? AND deleted_at IS NOT NULL`, userID)
	if err != nil {
		log.Printf("Failed to query trashed files: %v", err)
		http.Error(w, "Failed to empty trash", http.StatusInternalServerError)
		return
	}

	var fileIDs []string
	for fileRows.Next() {
		var id string
		if err := fileRows.Scan(&id); err != nil {
			continue
		}
		fileIDs = append(fileIDs, id)
	}
	fileRows.Close()

	// Delete all files
	deletedFiles := 0
	for _, fid := range fileIDs {
		if err := permanentDeleteFile(fid, userID); err != nil {
			log.Printf("Warning: failed to delete file %s: %v", fid, err)
		} else {
			deletedFiles++
		}
	}

	// Get all trashed folders (top-level only - recursion handles children)
	folderRows, err := globalDB.Query(`
		SELECT id FROM folders 
		WHERE owner_id = ? AND deleted_at IS NOT NULL 
		AND (parent_id IS NULL OR parent_id NOT IN (
			SELECT id FROM folders WHERE deleted_at IS NOT NULL
		))
	`, userID)
	if err != nil {
		log.Printf("Failed to query trashed folders: %v", err)
		http.Error(w, "Failed to empty trash", http.StatusInternalServerError)
		return
	}

	var folderIDs []string
	for folderRows.Next() {
		var id string
		if err := folderRows.Scan(&id); err != nil {
			continue
		}
		folderIDs = append(folderIDs, id)
	}
	folderRows.Close()

	// Delete all folders
	deletedFolders := 0
	for _, fid := range folderIDs {
		if err := permanentDeleteFolder(fid, userID); err != nil {
			log.Printf("Warning: failed to delete folder %s: %v", fid, err)
		} else {
			deletedFolders++
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message":         "Trash emptied",
		"deleted_files":   deletedFiles,
		"deleted_folders": deletedFolders,
	})
}

// Helper functions

type customError struct {
	msg string
}

func (e *customError) Error() string {
	return e.msg
}

func isStorageNodeOnline(nodeID string) bool {
	var isActive bool
	err := globalDB.QueryRow(`SELECT is_active FROM storage_nodes WHERE id = ?`, nodeID).Scan(&isActive)
	if err != nil {
		return false
	}
	return isActive
}

func isPeerServerOnline(peerID string) bool {
	var isActive bool
	err := globalDB.QueryRow(`SELECT is_active FROM peer_servers WHERE id = ?`, peerID).Scan(&isActive)
	if err != nil {
		return false
	}
	return isActive
}

func deleteFromStorageNode(nodeID, fileID, storagePath string) error {
	// Get the OS node client from proxy_upload_handler
	client := GetOSNodeClient()
	if client == nil {
		log.Printf("WARNING: osNodeClient is nil, cannot delete file %s from node %s", fileID, nodeID)
		return fmt.Errorf("storage node client not initialized")
	}
	
	log.Printf("Deleting file %s from storage node %s (path: %s)", fileID, nodeID, storagePath)
	
	// Call DeleteFromNode which will delete from storage nodes AND from the controller database
	// Note: This means the file record will be deleted from the files table
	// We need to be careful about the order of operations in permanentDeleteFile
	err := client.DeleteFromNode(fileID)
	if err != nil {
		log.Printf("ERROR: Failed to delete file %s from node: %v", fileID, err)
		return err
	}
	
	log.Printf("Successfully deleted file %s from storage node %s", fileID, nodeID)
	return nil
}

func deleteFromPeerServer(peerID, fileID, storagePath string) error {
	// Get peer server details from database
	var peerURL, clientCert, clientKey, serverCert string
	var isActive bool
	
	err := globalDB.QueryRow(`
		SELECT url, is_active, COALESCE(client_cert, ''), COALESCE(client_key, ''), COALESCE(server_cert, '')
		FROM peer_servers WHERE id = ?
	`, peerID).Scan(&peerURL, &isActive, &clientCert, &clientKey, &serverCert)
	
	if err != nil {
		return fmt.Errorf("peer server not found: %w", err)
	}
	
	if !isActive {
		return fmt.Errorf("peer server is offline")
	}
	
	// TODO: Implement mTLS connection to peer server
	// For now, we'll use HTTP request with proper authentication
	// In production, this should use the mTLS certificates stored in the database
	
	log.Printf("Requesting deletion of file %s from peer server %s (%s)", fileID, peerID, peerURL)
	
	// This would make an HTTP DELETE request to the peer server
	// Example: DELETE https://peer-server/v1/files/{fileID}
	// Using the client cert for mTLS authentication
	
	// Placeholder - actual implementation would use mTLS client
	return nil
}

func queuePendingDeletion(fileID, shardID, nodeID, peerServerID, storagePath, targetType string) {
	id := uuid.New().String()
	
	var nodeIDVal, peerIDVal, shardIDVal, pathVal interface{}
	if nodeID != "" {
		nodeIDVal = nodeID
	}
	if peerServerID != "" {
		peerIDVal = peerServerID
	}
	if shardID != "" {
		shardIDVal = shardID
	}
	if storagePath != "" {
		pathVal = storagePath
	}

	// Check if the node/peer exists before inserting to avoid FK constraint errors
	if nodeID != "" {
		var count int
		err := globalDB.QueryRow(`SELECT COUNT(*) FROM storage_nodes WHERE id = ?`, nodeID).Scan(&count)
		if err != nil || count == 0 {
			log.Printf("WARNING: Cannot queue deletion - storage node %s does not exist in database", nodeID)
			return
		}
	}
	
	if peerServerID != "" {
		var count int
		err := globalDB.QueryRow(`SELECT COUNT(*) FROM peer_servers WHERE id = ?`, peerServerID).Scan(&count)
		if err != nil || count == 0 {
			log.Printf("WARNING: Cannot queue deletion - peer server %s does not exist in database", peerServerID)
			return
		}
	}

	_, err := globalDB.Exec(`
		INSERT INTO pending_deletions (id, file_id, shard_id, node_id, peer_server_id, storage_path, target_type, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'pending')
	`, id, fileID, shardIDVal, nodeIDVal, peerIDVal, pathVal, targetType)
	
	if err != nil {
		log.Printf("Failed to queue pending deletion: %v", err)
	} else {
		log.Printf("Successfully queued deletion for file %s (node: %s, peer: %s, type: %s)", 
			fileID, nodeID, peerServerID, targetType)
	}
}
