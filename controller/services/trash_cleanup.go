package services

import (
	"database/sql"
	"log"
	"time"

	"github.com/google/uuid"
)

// TrashCleanupService handles automatic deletion of items that have been in trash for 30+ days
type TrashCleanupService struct {
	db       *sql.DB
	ticker   *time.Ticker
	stopChan chan bool
}

// NewTrashCleanupService creates a new trash cleanup service
func NewTrashCleanupService(db *sql.DB) *TrashCleanupService {
	return &TrashCleanupService{
		db:       db,
		stopChan: make(chan bool),
	}
}

// Start begins the background cleanup process
func (s *TrashCleanupService) Start() {
	// Run every hour
	s.ticker = time.NewTicker(1 * time.Hour)
	
	// Run immediately on startup
	go s.runCleanup()
	
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.runCleanup()
			case <-s.stopChan:
				s.ticker.Stop()
				return
			}
		}
	}()
	
	log.Println("Trash cleanup service started (30-day retention)")
}

// Stop halts the background cleanup process
func (s *TrashCleanupService) Stop() {
	s.stopChan <- true
}

// runCleanup checks for items in trash older than 30 days and deletes them
func (s *TrashCleanupService) runCleanup() {
	log.Println("Running trash cleanup check...")
	
	// Calculate 30 days ago
	thirtyDaysAgo := time.Now().AddDate(0, 0, -30).Format(time.RFC3339)
	
	// Get expired files
	expiredFiles := s.getExpiredFiles(thirtyDaysAgo)
	
	// Get expired folders
	expiredFolders := s.getExpiredFolders(thirtyDaysAgo)
	
	deletedFiles := 0
	deletedFolders := 0
	
	// Delete expired files
	for _, file := range expiredFiles {
		if err := s.permanentDeleteFile(file); err != nil {
			log.Printf("Failed to delete expired file %s: %v", file.ID, err)
		} else {
			deletedFiles++
		}
	}
	
	// Delete expired folders
	for _, folder := range expiredFolders {
		if err := s.permanentDeleteFolder(folder); err != nil {
			log.Printf("Failed to delete expired folder %s: %v", folder.ID, err)
		} else {
			deletedFolders++
		}
	}
	
	if deletedFiles > 0 || deletedFolders > 0 {
		log.Printf("Trash cleanup complete: deleted %d files, %d folders", deletedFiles, deletedFolders)
	}
}

type expiredFile struct {
	ID          string
	OwnerID     string
	NodeID      string
	StoragePath string
}

type expiredFolder struct {
	ID      string
	OwnerID string
}

func (s *TrashCleanupService) getExpiredFiles(cutoffTime string) []expiredFile {
	rows, err := s.db.Query(`
		SELECT id, owner_id, COALESCE(node_id, ''), COALESCE(storage_path, '')
		FROM files 
		WHERE deleted_at IS NOT NULL AND deleted_at < ?
	`, cutoffTime)
	if err != nil {
		log.Printf("Failed to query expired files: %v", err)
		return nil
	}
	defer rows.Close()
	
	var files []expiredFile
	for rows.Next() {
		var f expiredFile
		if err := rows.Scan(&f.ID, &f.OwnerID, &f.NodeID, &f.StoragePath); err != nil {
			continue
		}
		files = append(files, f)
	}
	return files
}

func (s *TrashCleanupService) getExpiredFolders(cutoffTime string) []expiredFolder {
	// Only get top-level trashed folders (those not inside another trashed folder)
	rows, err := s.db.Query(`
		SELECT id, owner_id
		FROM folders 
		WHERE deleted_at IS NOT NULL AND deleted_at < ?
		AND (parent_id IS NULL OR parent_id NOT IN (
			SELECT id FROM folders WHERE deleted_at IS NOT NULL
		))
	`, cutoffTime)
	if err != nil {
		log.Printf("Failed to query expired folders: %v", err)
		return nil
	}
	defer rows.Close()
	
	var folders []expiredFolder
	for rows.Next() {
		var f expiredFolder
		if err := rows.Scan(&f.ID, &f.OwnerID); err != nil {
			continue
		}
		folders = append(folders, f)
	}
	return folders
}

func (s *TrashCleanupService) permanentDeleteFile(file expiredFile) error {
	// Get file shards for erasure-coded files
	shardRows, err := s.db.Query(`
		SELECT id, node_id, peer_server_id, storage_path 
		FROM file_shards WHERE file_id = ?
	`, file.ID)
	if err != nil {
		return err
	}
	defer shardRows.Close()
	
	// Queue deletions for each shard
	for shardRows.Next() {
		var shardID string
		var shardNodeID, peerServerID, shardPath sql.NullString
		
		if err := shardRows.Scan(&shardID, &shardNodeID, &peerServerID, &shardPath); err != nil {
			continue
		}
		
		// Check if node/peer is online
		isOnline := false
		if shardNodeID.Valid && shardNodeID.String != "" {
			isOnline = s.isStorageNodeOnline(shardNodeID.String)
		} else if peerServerID.Valid && peerServerID.String != "" {
			isOnline = s.isPeerServerOnline(peerServerID.String)
		}
		
		if !isOnline {
			// Queue for later deletion
			nodeIDStr := ""
			peerIDStr := ""
			if shardNodeID.Valid {
				nodeIDStr = shardNodeID.String
			}
			if peerServerID.Valid {
				peerIDStr = peerServerID.String
			}
			s.queuePendingDeletion(file.ID, shardID, nodeIDStr, peerIDStr, shardPath.String, "shard")
		}
		// Note: Online nodes will have the files deleted when they sync with controller
	}
	
	// Queue deletion for main file if node is offline
	if file.NodeID != "" {
		if !s.isStorageNodeOnline(file.NodeID) {
			s.queuePendingDeletion(file.ID, "", file.NodeID, "", file.StoragePath, "file")
		}
	}
	
	// Delete file shards from DB
	_, err = s.db.Exec(`DELETE FROM file_shards WHERE file_id = ?`, file.ID)
	if err != nil {
		log.Printf("Failed to delete file shards from DB: %v", err)
	}
	
	// Delete file from DB
	_, err = s.db.Exec(`DELETE FROM files WHERE id = ?`, file.ID)
	return err
}

func (s *TrashCleanupService) permanentDeleteFolder(folder expiredFolder) error {
	// Delete all files in this folder
	fileRows, err := s.db.Query(`SELECT id, owner_id, COALESCE(node_id, ''), COALESCE(storage_path, '') FROM files WHERE folder_id = ?`, folder.ID)
	if err != nil {
		return err
	}
	defer fileRows.Close()
	
	var files []expiredFile
	for fileRows.Next() {
		var f expiredFile
		if err := fileRows.Scan(&f.ID, &f.OwnerID, &f.NodeID, &f.StoragePath); err != nil {
			continue
		}
		files = append(files, f)
	}
	
	for _, f := range files {
		if err := s.permanentDeleteFile(f); err != nil {
			log.Printf("Warning: failed to delete file %s: %v", f.ID, err)
		}
	}
	
	// Recursively delete subfolders
	subRows, err := s.db.Query(`SELECT id, owner_id FROM folders WHERE parent_id = ?`, folder.ID)
	if err != nil {
		return err
	}
	defer subRows.Close()
	
	var subfolders []expiredFolder
	for subRows.Next() {
		var f expiredFolder
		if err := subRows.Scan(&f.ID, &f.OwnerID); err != nil {
			continue
		}
		subfolders = append(subfolders, f)
	}
	
	for _, sub := range subfolders {
		if err := s.permanentDeleteFolder(sub); err != nil {
			log.Printf("Warning: failed to delete subfolder %s: %v", sub.ID, err)
		}
	}
	
	// Delete folder from DB
	_, err = s.db.Exec(`DELETE FROM folders WHERE id = ?`, folder.ID)
	return err
}

func (s *TrashCleanupService) isStorageNodeOnline(nodeID string) bool {
	var isActive bool
	err := s.db.QueryRow(`SELECT is_active FROM storage_nodes WHERE id = ?`, nodeID).Scan(&isActive)
	if err != nil {
		return false
	}
	return isActive
}

func (s *TrashCleanupService) isPeerServerOnline(peerID string) bool {
	var isActive bool
	err := s.db.QueryRow(`SELECT is_active FROM peer_servers WHERE id = ?`, peerID).Scan(&isActive)
	if err != nil {
		return false
	}
	return isActive
}

func (s *TrashCleanupService) queuePendingDeletion(fileID, shardID, nodeID, peerServerID, storagePath, targetType string) {
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
	
	_, err := s.db.Exec(`
		INSERT INTO pending_deletions (id, file_id, shard_id, node_id, peer_server_id, storage_path, target_type, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'pending')
	`, id, fileID, shardIDVal, nodeIDVal, peerIDVal, pathVal, targetType)
	
	if err != nil {
		log.Printf("Failed to queue pending deletion: %v", err)
	}
}

// ProcessPendingDeletions is called when a node comes online to process any pending deletions
func ProcessPendingDeletions(db *sql.DB, nodeID string, deleteFunc func(fileID string) error) {
	// Get all pending deletions for this node
	rows, err := db.Query(`
		SELECT id, file_id, storage_path, target_type 
		FROM pending_deletions 
		WHERE node_id = ? AND status = 'pending'
	`, nodeID)
	if err != nil {
		log.Printf("Failed to query pending deletions: %v", err)
		return
	}
	defer rows.Close()
	
	for rows.Next() {
		var id, fileID, targetType string
		var storagePath sql.NullString
		
		if err := rows.Scan(&id, &fileID, &storagePath, &targetType); err != nil {
			continue
		}
		
		// Try to delete
		if err := deleteFunc(fileID); err != nil {
			// Update retry count
			_, _ = db.Exec(`
				UPDATE pending_deletions 
				SET retry_count = retry_count + 1, 
				    last_retry_at = ?,
				    error_message = ?
				WHERE id = ?
			`, time.Now().Format(time.RFC3339), err.Error(), id)
			log.Printf("Failed to delete %s: %v", fileID, err)
		} else {
			// Mark as completed
			_, _ = db.Exec(`UPDATE pending_deletions SET status = 'completed' WHERE id = ?`, id)
		}
	}
}

// ProcessPendingPeerDeletions is called when a peer server comes online
func ProcessPendingPeerDeletions(db *sql.DB, peerServerID string, deleteFunc func(fileID string) error) {
	// Get all pending deletions for this peer
	rows, err := db.Query(`
		SELECT id, file_id, storage_path, target_type 
		FROM pending_deletions 
		WHERE peer_server_id = ? AND status = 'pending'
	`, peerServerID)
	if err != nil {
		log.Printf("Failed to query pending peer deletions: %v", err)
		return
	}
	defer rows.Close()
	
	for rows.Next() {
		var id, fileID, targetType string
		var storagePath sql.NullString
		
		if err := rows.Scan(&id, &fileID, &storagePath, &targetType); err != nil {
			continue
		}
		
		// Try to delete
		if err := deleteFunc(fileID); err != nil {
			// Update retry count
			_, _ = db.Exec(`
				UPDATE pending_deletions 
				SET retry_count = retry_count + 1, 
				    last_retry_at = ?,
				    error_message = ?
				WHERE id = ?
			`, time.Now().Format(time.RFC3339), err.Error(), id)
			log.Printf("Failed to delete from peer %s: %v", fileID, err)
		} else {
			// Mark as completed
			_, _ = db.Exec(`UPDATE pending_deletions SET status = 'completed' WHERE id = ?`, id)
		}
	}
}
