package healthcheck

import (
	"context"
	"database/sql"
	"log"
	"net/http"
	"time"

	"storageos/controller/models"
	rootservices "storageos/controller/services"
)

// Monitor periodically checks the health of all storage nodes
type Monitor struct {
	db              *sql.DB
	interval        time.Duration
	client          *http.Client
	stopCh          chan struct{}
	previousStatus  map[string]bool // Track previous node status for detecting "came online" events
}

// NewMonitor creates a new health check monitor
func NewMonitor(db *sql.DB, interval time.Duration) *Monitor {
	if interval == 0 {
		interval = 30 * time.Second // Default: check every 30 seconds
	}

	return &Monitor{
		db:       db,
		interval: interval,
		client: &http.Client{
			Timeout: 5 * time.Second, // Quick timeout for health checks
		},
		stopCh:         make(chan struct{}),
		previousStatus: make(map[string]bool),
	}
}

// Start begins the health monitoring loop
func (m *Monitor) Start(ctx context.Context) {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	log.Printf("Health monitor started (interval: %v)", m.interval)

	// Run initial check immediately
	m.checkAllNodes()

	for {
		select {
		case <-ticker.C:
			m.checkAllNodes()
		case <-m.stopCh:
			log.Println("Health monitor stopped")
			return
		case <-ctx.Done():
			log.Println("Health monitor stopped (context cancelled)")
			return
		}
	}
}

// Stop stops the health monitoring loop
func (m *Monitor) Stop() {
	close(m.stopCh)
}

// checkAllNodes checks all storage nodes and updates their status
func (m *Monitor) checkAllNodes() {
	nodes, err := models.ListStorageNodes(m.db)
	if err != nil {
		log.Printf("Health monitor: failed to list nodes: %v", err)
		return
	}

	for _, node := range nodes {
		isHealthy := m.checkNode(node)
		wasOnline := m.previousStatus[node.ID]
		
		// Check if node just came online
		if isHealthy && !wasOnline {
			log.Printf("Health monitor: node %s came online, processing pending deletions", node.Name)
			// Process any pending deletions for this node
			go m.processPendingDeletions(node.ID)
		}
		
		m.previousStatus[node.ID] = isHealthy
		
		if err := m.updateNodeStatus(node.ID, isHealthy); err != nil {
			log.Printf("Health monitor: failed to update node %s status: %v", node.Name, err)
		}
	}

	// Also check peer servers
	peers, err := models.ListPeerServers(m.db)
	if err != nil {
		log.Printf("Health monitor: failed to list peer servers: %v", err)
		return
	}

	for _, peer := range peers {
		if peer.RequestStatus == "accepted" {
			isHealthy := m.checkPeerServer(peer)
			wasOnline := m.previousStatus["peer-"+peer.ID]
			
			// Check if peer just came online
			if isHealthy && !wasOnline {
				log.Printf("Health monitor: peer %s came online, processing pending deletions", peer.Name)
				// Process any pending deletions for this peer
				go m.processPendingPeerDeletions(peer.ID)
			}
			
			m.previousStatus["peer-"+peer.ID] = isHealthy
			
			if err := models.UpdatePeerServerStatus(m.db, peer.ID, isHealthy); err != nil {
				log.Printf("Health monitor: failed to update peer %s status: %v", peer.Name, err)
			}
		}
	}
}

// checkNode performs a health check on a single node
func (m *Monitor) checkNode(node *models.StorageNode) bool {
	baseURL, err := node.EffectiveURL()
	if err != nil {
		log.Printf("Health monitor: node %s has invalid URL: %v", node.Name, err)
		return false
	}

	healthURL := baseURL + "/health"
	resp, err := m.client.Get(healthURL)
	if err != nil {
		log.Printf("Health monitor: node %s unreachable: %v", node.Name, err)
		return false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("Health monitor: node %s unhealthy (status: %d)", node.Name, resp.StatusCode)
		return false
	}

	return true
}

// checkPeerServer performs a health check on a peer server
func (m *Monitor) checkPeerServer(peer *models.PeerServer) bool {
	healthURL := peer.URL + "/v1/healthz"
	resp, err := m.client.Get(healthURL)
	if err != nil {
		return false
	}
	defer resp.Body.Close()

	return resp.StatusCode == http.StatusOK
}

// updateNodeStatus updates the is_active flag and last_active timestamp for a node
func (m *Monitor) updateNodeStatus(nodeID string, isHealthy bool) error {
	now := time.Now().Format(time.RFC3339)

	stmt := `UPDATE storage_nodes 
	         SET is_active = ?, last_active = ?, updated_at = CURRENT_TIMESTAMP 
	         WHERE id = ?`

	_, err := m.db.Exec(stmt, isHealthy, now, nodeID)
	if err != nil {
		return err
	}

	return nil
}

// processPendingDeletions processes pending file deletions for a node that just came online
func (m *Monitor) processPendingDeletions(nodeID string) {
	// Get the node info to create a delete function
	node, err := models.GetStorageNodeByID(m.db, nodeID)
	if err != nil {
		log.Printf("Failed to get node info for pending deletions: %v", err)
		return
	}
	
	baseURL, err := node.EffectiveURL()
	if err != nil {
		log.Printf("Failed to get node URL for pending deletions: %v", err)
		return
	}
	
	deleteFunc := func(fileID string) error {
		// Make DELETE request to the node
		req, err := http.NewRequest("DELETE", baseURL+"/api/delete?file_id="+fileID, nil)
		if err != nil {
			return err
		}
		
		resp, err := m.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			return &httpError{StatusCode: resp.StatusCode}
		}
		
		return nil
	}
	
	rootservices.ProcessPendingDeletions(m.db, nodeID, deleteFunc)
}

// processPendingPeerDeletions processes pending file deletions for a peer that just came online
func (m *Monitor) processPendingPeerDeletions(peerServerID string) {
	// Get the peer info
	peer, err := models.GetPeerServerByID(m.db, peerServerID)
	if err != nil {
		log.Printf("Failed to get peer info for pending deletions: %v", err)
		return
	}
	
	deleteFunc := func(fileID string) error {
		// Make DELETE request to the peer server
		req, err := http.NewRequest("DELETE", peer.URL+"/v1/browser/delete?file_id="+fileID, nil)
		if err != nil {
			return err
		}
		
		// TODO: Add mTLS authentication
		
		resp, err := m.client.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			return &httpError{StatusCode: resp.StatusCode}
		}
		
		return nil
	}
	
	rootservices.ProcessPendingPeerDeletions(m.db, peerServerID, deleteFunc)
}

type httpError struct {
	StatusCode int
}

func (e *httpError) Error() string {
	return "HTTP error: " + http.StatusText(e.StatusCode)
}
