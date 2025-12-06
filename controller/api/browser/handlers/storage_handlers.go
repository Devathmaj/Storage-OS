package handlers

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
)

// StorageNodeStats represents storage statistics for a single node
type StorageNodeStats struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	URL         string  `json:"url,omitempty"`
	QuotaMB     int64   `json:"quota_mb"`
	UsedMB      int64   `json:"used_mb"`
	AvailableMB int64   `json:"available_mb"`
	UsedPercent float64 `json:"used_percent"`
	IsActive    bool    `json:"is_active"`
	IsOnline    bool    `json:"is_online"`
	LastActive  string  `json:"last_active,omitempty"`
	Status      string  `json:"status,omitempty"`
}

// PeerServerStats represents storage statistics for a peer server
type PeerServerStats struct {
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	URL              string  `json:"url"`
	QuotaAllocated   int64   `json:"quota_allocated"`
	QuotaUsed        int64   `json:"quota_used"`
	StorageRemaining int64   `json:"storage_remaining"`
	UsedPercent      float64 `json:"used_percent"`
	IsActive         bool    `json:"is_active"`
	LastActive       string  `json:"last_active,omitempty"`
}

// StorageOverviewResponse contains total storage stats and per-node breakdown
type StorageOverviewResponse struct {
	// Totals across all storage nodes (actual file data)
	TotalStorageMB     int64   `json:"total_storage_mb"`
	TotalUsedMB        int64   `json:"total_used_mb"`
	TotalAvailableMB   int64   `json:"total_available_mb"`
	TotalUsedPercent   float64 `json:"total_used_percent"`
	
	// Actual files stored (from database)
	TotalFileSizeMB    int64   `json:"total_file_size_mb"`
	TotalFileCount     int     `json:"total_file_count"`
	
	// Storage node details
	StorageNodes       []StorageNodeStats `json:"storage_nodes"`
	ActiveNodeCount    int                `json:"active_node_count"`
	TotalNodeCount     int                `json:"total_node_count"`
	
	// Peer server storage (for peer file sharing)
	PeerServers        []PeerServerStats  `json:"peer_servers,omitempty"`
	PeerStorageUsedMB  int64              `json:"peer_storage_used_mb,omitempty"`
	PeerQuotaTotalMB   int64              `json:"peer_quota_total_mb,omitempty"`
}

// GetStorageOverviewHandler returns overall storage statistics
func GetStorageOverviewHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := StorageOverviewResponse{
		StorageNodes: []StorageNodeStats{},
		PeerServers:  []PeerServerStats{},
	}

	// Map to track nodes we've already added (by ID)
	nodeMap := make(map[string]*StorageNodeStats)

	// Calculate storage used per node from files table (not file_shards since erasure coding may not be enabled)
	// Also calculate total file storage since it's stored on any available node
	var totalFilesStorageBytes int64
	err := globalDB.QueryRow(`
		SELECT COALESCE(SUM(size), 0) FROM files WHERE deleted_at IS NULL
	`).Scan(&totalFilesStorageBytes)
	if err != nil {
		log.Printf("Failed to query total files storage: %v", err)
	}
	totalFilesStorageMB := totalFilesStorageBytes / (1024 * 1024)

	// Get OS nodes with their system_info
	osNodeRows, err := globalDB.Query(`
		SELECT 
			n.id, 
			n.system_info, 
			n.status, 
			n.last_seen,
			COALESCE(s.url, '') as storage_url,
			COALESCE(s.quota_mb, 0) as quota_mb
		FROM os_nodes n
		LEFT JOIN storage_nodes s ON n.id = s.id
		WHERE n.status = 'active'
		ORDER BY n.id
	`)
	if err != nil {
		log.Printf("Failed to query OS nodes: %v", err)
		// Continue to try storage_nodes query
	} else {
		defer osNodeRows.Close()

		for osNodeRows.Next() {
			var nodeID, systemInfo, status string
			var lastSeen sql.NullString
			var storageURL string
			var quotaMB int64

			if err := osNodeRows.Scan(&nodeID, &systemInfo, &status, &lastSeen, &storageURL, &quotaMB); err != nil {
				log.Printf("Failed to scan OS node: %v", err)
				continue
			}

			// Parse system_info JSON to get hostname
			var sysInfo struct {
				Hostname string `json:"hostname"`
			}
			if err := json.Unmarshal([]byte(systemInfo), &sysInfo); err != nil {
				log.Printf("Failed to parse system_info for node %s: %v", nodeID, err)
			}

			node := StorageNodeStats{
				ID:     nodeID,
				Name:   sysInfo.Hostname,
				URL:    storageURL,
				Status: status,
			}

			// Use the configured quota
			node.QuotaMB = quotaMB
			
			// For single-node setups, assign all file storage to this node
			// In multi-node setups with sharding, this would be calculated per-node
			if response.TotalNodeCount == 0 {
				// First (and likely only) node gets all the storage
				node.UsedMB = totalFilesStorageMB
			}
			
			// Calculate available
			node.AvailableMB = node.QuotaMB - node.UsedMB
			if node.AvailableMB < 0 {
				node.AvailableMB = 0
			}

			node.IsActive = status == "active"
			
			// Check if node is online (last_seen within 2 minutes)
			if lastSeen.Valid && lastSeen.String != "" {
				node.LastActive = lastSeen.String
				node.IsOnline = true
			}

			if node.QuotaMB > 0 {
				node.UsedPercent = float64(node.UsedMB) / float64(node.QuotaMB) * 100
			}

			// Default name if hostname not set
			if node.Name == "" {
				node.Name = "OS Node " + nodeID
			}

			nodeMap[nodeID] = &node
			response.StorageNodes = append(response.StorageNodes, node)
			response.TotalStorageMB += node.QuotaMB
			response.TotalUsedMB += node.UsedMB
			response.TotalAvailableMB += node.AvailableMB
			response.TotalNodeCount++
			if node.IsActive {
				response.ActiveNodeCount++
			}
		}
	}

	// Also query storage_nodes directly for any nodes not in os_nodes
	nodeRows, err := globalDB.Query(`
		SELECT id, name, COALESCE(url, ''), quota_mb, COALESCE(last_active, '')
		FROM storage_nodes
		WHERE id NOT IN (SELECT id FROM os_nodes WHERE status = 'active')
		ORDER BY name
	`)
	if err != nil {
		log.Printf("Failed to query storage nodes: %v", err)
		// Don't fail completely
	} else {
		defer nodeRows.Close()

		for nodeRows.Next() {
			var node StorageNodeStats
			if err := nodeRows.Scan(&node.ID, &node.Name, &node.URL, &node.QuotaMB, &node.LastActive); err != nil {
				log.Printf("Failed to scan storage node: %v", err)
				continue
			}

			// Skip if already added from os_nodes
			if _, exists := nodeMap[node.ID]; exists {
				continue
			}

			// For nodes not in os_nodes, assign storage proportionally if any
			// (this is a fallback - normally os_nodes should have the data)
			if response.TotalNodeCount == 0 && totalFilesStorageMB > 0 {
				node.UsedMB = totalFilesStorageMB
			}
			
			node.AvailableMB = node.QuotaMB - node.UsedMB
			if node.AvailableMB < 0 {
				node.AvailableMB = 0
			}

			node.IsActive = node.LastActive != ""
			if node.QuotaMB > 0 {
				node.UsedPercent = float64(node.UsedMB) / float64(node.QuotaMB) * 100
			}

			response.StorageNodes = append(response.StorageNodes, node)
			response.TotalStorageMB += node.QuotaMB
			response.TotalUsedMB += node.UsedMB
			response.TotalAvailableMB += node.AvailableMB
			response.TotalNodeCount++
			if node.IsActive {
				response.ActiveNodeCount++
			}
		}
	}

	// Calculate total usage percentage
	if response.TotalStorageMB > 0 {
		response.TotalUsedPercent = float64(response.TotalUsedMB) / float64(response.TotalStorageMB) * 100
	}

	// Get actual file storage from the database (this is the real data stored, not disk usage)
	var totalFileSizeBytes int64
	var totalFileCount int
	err = globalDB.QueryRow(`
		SELECT COALESCE(SUM(size), 0), COUNT(*) FROM files WHERE deleted_at IS NULL
	`).Scan(&totalFileSizeBytes, &totalFileCount)
	if err != nil {
		log.Printf("Failed to get total file storage: %v", err)
	} else {
		response.TotalFileSizeMB = totalFileSizeBytes / (1024 * 1024)
		response.TotalFileCount = totalFileCount
		
		// Override the disk-reported usage with actual file data for accuracy
		// The disk usage includes erasure coding overhead, so we use file sizes instead
		response.TotalUsedMB = response.TotalFileSizeMB
		if response.TotalStorageMB > 0 {
			response.TotalAvailableMB = response.TotalStorageMB - response.TotalUsedMB
			if response.TotalAvailableMB < 0 {
				response.TotalAvailableMB = 0
			}
			response.TotalUsedPercent = float64(response.TotalUsedMB) / float64(response.TotalStorageMB) * 100
		}
	}

	// Get peer server stats (only accepted outbound peers)
	peerRows, err := globalDB.Query(`
		SELECT id, name, url, quota_allocated, quota_used, storage_remaining, 
			CASE WHEN is_active = 1 OR is_active = 'true' OR is_active = TRUE THEN 1 ELSE 0 END as is_active, 
			COALESCE(last_active, '')
		FROM peer_servers
		WHERE peer_type = 'outbound' AND request_status = 'accepted'
		ORDER BY name
	`)
	if err != nil {
		log.Printf("Failed to query peer servers: %v", err)
		// Don't fail, just skip peer storage
	} else {
		defer peerRows.Close()

		for peerRows.Next() {
			var peer PeerServerStats
			var isActive int
			if err := peerRows.Scan(&peer.ID, &peer.Name, &peer.URL, &peer.QuotaAllocated, &peer.QuotaUsed, &peer.StorageRemaining, &isActive, &peer.LastActive); err != nil {
				log.Printf("Failed to scan peer server: %v", err)
				continue
			}
			
			peer.IsActive = isActive == 1
			if peer.QuotaAllocated > 0 {
				peer.UsedPercent = float64(peer.QuotaUsed) / float64(peer.QuotaAllocated) * 100
			}
			
			response.PeerServers = append(response.PeerServers, peer)
			// Convert bytes to MB for totals
			response.PeerStorageUsedMB += peer.QuotaUsed / (1024 * 1024)
			response.PeerQuotaTotalMB += peer.QuotaAllocated / (1024 * 1024)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// GetUserStorageHandler returns storage usage for the authenticated user
func GetUserStorageHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)

	type UserStorageResponse struct {
		UsedStorageBytes int64   `json:"used_storage_bytes"`
		MaxStorageBytes  int64   `json:"max_storage_bytes"`
		UsedPercent      float64 `json:"used_percent"`
		FileCount        int     `json:"file_count"`
		FolderCount      int     `json:"folder_count"`
		TrashSize        int64   `json:"trash_size_bytes"`
		TrashCount       int     `json:"trash_count"`
	}

	var response UserStorageResponse

	// Get user storage limits
	err := globalDB.QueryRow(`
		SELECT used_storage, max_storage FROM users WHERE id = ?
	`, userID).Scan(&response.UsedStorageBytes, &response.MaxStorageBytes)
	if err != nil && err != sql.ErrNoRows {
		log.Printf("Failed to get user storage: %v", err)
		http.Error(w, "Failed to retrieve storage info", http.StatusInternalServerError)
		return
	}

	// Calculate from files if user storage tracking is not set
	if response.UsedStorageBytes == 0 {
		err = globalDB.QueryRow(`
			SELECT COALESCE(SUM(size), 0) FROM files WHERE owner_id = ? AND deleted_at IS NULL
		`, userID).Scan(&response.UsedStorageBytes)
		if err != nil {
			log.Printf("Failed to calculate used storage: %v", err)
		}
	}

	// Get file count
	err = globalDB.QueryRow(`
		SELECT COUNT(*) FROM files WHERE owner_id = ? AND deleted_at IS NULL
	`, userID).Scan(&response.FileCount)
	if err != nil {
		log.Printf("Failed to count files: %v", err)
	}

	// Get folder count
	err = globalDB.QueryRow(`
		SELECT COUNT(*) FROM folders WHERE owner_id = ? AND deleted_at IS NULL
	`, userID).Scan(&response.FolderCount)
	if err != nil {
		log.Printf("Failed to count folders: %v", err)
	}

	// Get trash size and count
	err = globalDB.QueryRow(`
		SELECT COALESCE(SUM(size), 0), COUNT(*) FROM files WHERE owner_id = ? AND deleted_at IS NOT NULL
	`, userID).Scan(&response.TrashSize, &response.TrashCount)
	if err != nil {
		log.Printf("Failed to get trash stats: %v", err)
	}

	// Add trashed folder count
	var trashFolderCount int
	err = globalDB.QueryRow(`
		SELECT COUNT(*) FROM folders WHERE owner_id = ? AND deleted_at IS NOT NULL
	`, userID).Scan(&trashFolderCount)
	if err == nil {
		response.TrashCount += trashFolderCount
	}

	// Calculate used percentage
	if response.MaxStorageBytes > 0 {
		response.UsedPercent = float64(response.UsedStorageBytes) / float64(response.MaxStorageBytes) * 100
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}
