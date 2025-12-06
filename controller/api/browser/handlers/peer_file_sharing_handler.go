package handlers

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"

	"github.com/go-chi/chi/v5"
	"storageos/controller/models"
)

// PeerFileSharingSettingsResponse represents the full peer file sharing configuration
type PeerFileSharingSettingsResponse struct {
	Settings *models.PeerFileSharingSettings `json:"settings"`
	Outbound []*models.PeerSharingOutbound   `json:"outbound"`
	Inbound  []*models.PeerSharingInbound    `json:"inbound"`
}

// GetPeerFileSharingSettingsHandler returns the peer file sharing settings
func GetPeerFileSharingSettingsHandler(w http.ResponseWriter, r *http.Request) {
	settings, err := models.GetPeerFileSharingSettings(db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get settings: "+err.Error())
		return
	}

	outbound, err := models.ListPeerSharingOutbound(db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get outbound peers: "+err.Error())
		return
	}

	inbound, err := models.ListPeerSharingInbound(db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get inbound peers: "+err.Error())
		return
	}

	response := PeerFileSharingSettingsResponse{
		Settings: settings,
		Outbound: outbound,
		Inbound:  inbound,
	}

	writeJSON(w, http.StatusOK, response)
}

// UpdatePeerFileSharingSettingsHandler updates the peer file sharing settings
func UpdatePeerFileSharingSettingsHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Enabled                bool `json:"enabled"`
		CompleteStorageEnabled bool `json:"complete_storage_enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	// If disabling complete storage, warn about data loss
	currentSettings, err := models.GetPeerFileSharingSettings(db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get current settings")
		return
	}

	if currentSettings.CompleteStorageEnabled && !payload.CompleteStorageEnabled {
		// Count files stored on peer servers
		peers, _ := models.ListPeerSharingOutbound(db)
		totalFiles := 0
		for _, p := range peers {
			count, _ := models.CountShardsOnPeerServer(db, p.PeerServerID)
			totalFiles += count
		}

		if totalFiles > 0 {
			writeJSON(w, http.StatusConflict, map[string]interface{}{
				"warning":          "Disabling complete storage will make files on peer servers inaccessible",
				"files_on_peers":   totalFiles,
				"requires_confirm": true,
			})
			return
		}
	}

	settings, err := models.UpdatePeerFileSharingSettings(db, payload.Enabled, payload.CompleteStorageEnabled)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update settings: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

// ConfirmDisableCompleteStorageHandler handles confirmed disable of complete storage
func ConfirmDisableCompleteStorageHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Confirmed bool `json:"confirmed"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if !payload.Confirmed {
		writeError(w, http.StatusBadRequest, "confirmation required")
		return
	}

	// Get current settings
	currentSettings, err := models.GetPeerFileSharingSettings(db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get settings")
		return
	}

	// Disable complete storage
	settings, err := models.UpdatePeerFileSharingSettings(db, currentSettings.Enabled, false)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update settings")
		return
	}

	writeJSON(w, http.StatusOK, settings)
}

// AddPeerSharingOutboundHandler adds a peer server to outbound sharing list
func AddPeerSharingOutboundHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		PeerServerID string `json:"peer_server_id"`
		Priority     int    `json:"priority"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if payload.PeerServerID == "" {
		writeError(w, http.StatusBadRequest, "peer_server_id is required")
		return
	}

	pso, err := models.AddPeerSharingOutbound(db, payload.PeerServerID, payload.Priority)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, pso)
}

// AddPeerSharingInboundHandler adds a peer server to inbound sharing list
func AddPeerSharingInboundHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		PeerServerID string `json:"peer_server_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if payload.PeerServerID == "" {
		writeError(w, http.StatusBadRequest, "peer_server_id is required")
		return
	}

	psi, err := models.AddPeerSharingInbound(db, payload.PeerServerID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusCreated, psi)
}

// UpdatePeerSharingOutboundHandler updates an outbound sharing configuration
func UpdatePeerSharingOutboundHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	var payload struct {
		Enabled bool `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if err := models.UpdatePeerSharingOutboundEnabled(db, id, payload.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// UpdatePeerSharingInboundHandler updates an inbound sharing configuration
func UpdatePeerSharingInboundHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	var payload struct {
		Enabled bool `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if err := models.UpdatePeerSharingInboundEnabled(db, id, payload.Enabled); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// RemovePeerSharingOutboundHandler removes a peer server from outbound sharing list
func RemovePeerSharingOutboundHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := models.RemovePeerSharingOutbound(db, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// RemovePeerSharingInboundHandler removes a peer server from inbound sharing list
func RemovePeerSharingInboundHandler(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}

	if err := models.RemovePeerSharingInbound(db, id); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

// GetAvailablePeersForSharingHandler returns peers that can be added to sharing lists
func GetAvailablePeersForSharingHandler(w http.ResponseWriter, r *http.Request) {
	peerType := r.URL.Query().Get("type") // "inbound" or "outbound"
	
	peers, err := models.ListPeerServersByType(db, peerType)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list peers: "+err.Error())
		return
	}

	// Filter to only accepted peers
	var available []*models.PeerServer
	for _, p := range peers {
		if p.RequestStatus == "accepted" {
			available = append(available, p)
		}
	}

	writeJSON(w, http.StatusOK, available)
}

// PeerStorageInfoResponse contains storage info returned by peer health check
type PeerStorageInfoResponse struct {
	PeerID           string `json:"peer_id"`
	StorageRemaining int64  `json:"storage_remaining"`
	QuotaAllocated   int64  `json:"quota_allocated"`
	QuotaUsed        int64  `json:"quota_used"`
	IsActive         bool   `json:"is_active"`
}

// GetPeerStorageInfoHandler handles requests from other servers to get our storage info
func GetPeerStorageInfoHandler(w http.ResponseWriter, r *http.Request) {
	// Calculate our remaining storage by summing up storage nodes
	nodes, err := models.ListActiveStorageNodes(db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get storage info")
		return
	}

	var totalAvailable int64
	for _, node := range nodes {
		totalAvailable += int64(node.AvailableMB) * 1024 * 1024 // Convert MB to bytes
	}

	response := PeerStorageInfoResponse{
		StorageRemaining: totalAvailable,
		IsActive:         true,
	}

	writeJSON(w, http.StatusOK, response)
}

// CheckPeerStorageHandler checks storage availability on a peer server
func CheckPeerStorageHandler(w http.ResponseWriter, r *http.Request) {
	peerID := chi.URLParam(r, "peerID")
	if peerID == "" {
		writeError(w, http.StatusBadRequest, "peer id is required")
		return
	}

	peer, err := models.GetPeerServerByID(db, peerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "peer not found")
		return
	}

	// Make request to peer server to get storage info
	storageInfo, err := fetchPeerStorageInfo(peer)
	if err != nil {
		log.Printf("Failed to fetch storage info from peer %s: %v", peer.Name, err)
		// Update peer as inactive
		models.UpdatePeerServerStatus(db, peerID, false)
		writeError(w, http.StatusServiceUnavailable, "peer server unreachable")
		return
	}

	// Update peer storage info in our database
	models.UpdatePeerServerStorageRemaining(db, peerID, storageInfo.StorageRemaining)
	models.UpdatePeerServerStatus(db, peerID, true)

	writeJSON(w, http.StatusOK, storageInfo)
}

// fetchPeerStorageInfo makes an mTLS request to get storage info from peer
func fetchPeerStorageInfo(peer *models.PeerServer) (*PeerStorageInfoResponse, error) {
	client, err := createPeerHTTPClient(peer)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	resp, err := client.Get(fmt.Sprintf("%s/v1/peers/storage-info", peer.URL))
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("peer returned %d: %s", resp.StatusCode, string(body))
	}

	var info PeerStorageInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &info, nil
}

// createPeerHTTPClient creates an HTTP client with mTLS for peer communication
func createPeerHTTPClient(peer *models.PeerServer) (*http.Client, error) {
	if peer.ClientCert == "" || peer.ClientKey == "" {
		// No mTLS configured, use regular HTTP client
		return &http.Client{}, nil
	}

	cert, err := tls.X509KeyPair([]byte(peer.ClientCert), []byte(peer.ClientKey))
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	if peer.ServerCert != "" {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM([]byte(peer.ServerCert))
		tlsConfig.RootCAs = caCertPool
	} else {
		tlsConfig.InsecureSkipVerify = true
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}, nil
}

// ReceivePeerFileHandler handles incoming file transfers from peer servers
func ReceivePeerFileHandler(w http.ResponseWriter, r *http.Request) {
	// Check if peer file sharing is enabled
	settings, err := models.GetPeerFileSharingSettings(db)
	if err != nil || !settings.Enabled {
		writeError(w, http.StatusForbidden, "peer file sharing is not enabled")
		return
	}

	// Parse the request
	var payload struct {
		FileID       string `json:"file_id"`
		OwnerID      string `json:"owner_id"`
		Filename     string `json:"filename"`
		ShardIndex   int    `json:"shard_index"`
		ShardType    string `json:"shard_type"`
		ShardData    []byte `json:"shard_data"`
		Checksum     string `json:"checksum"`
		SourceServer string `json:"source_server"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	// Verify source server is in our inbound sharing list
	inbound, err := models.ListPeerSharingInbound(db)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check inbound peers")
		return
	}

	var sourcePeer *models.PeerServer
	for _, p := range inbound {
		if p.Enabled && p.PeerServer != nil && p.PeerServer.URL == payload.SourceServer {
			sourcePeer = p.PeerServer
			break
		}
	}

	if sourcePeer == nil {
		writeError(w, http.StatusForbidden, "source server is not authorized for file sharing")
		return
	}

	// TODO: Store the shard on local storage nodes
	// This would involve selecting a node and uploading the shard data

	log.Printf("Received file shard from peer %s: file=%s shard=%d", sourcePeer.Name, payload.FileID, payload.ShardIndex)

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":  "received",
		"file_id": payload.FileID,
	})
}

// TransferShardToPeerHandler transfers a shard to a peer server
func TransferShardToPeerHandler(w http.ResponseWriter, r *http.Request) {
	// Check if peer file sharing is enabled
	settings, err := models.GetPeerFileSharingSettings(db)
	if err != nil || !settings.Enabled {
		writeError(w, http.StatusForbidden, "peer file sharing is not enabled")
		return
	}

	var payload struct {
		FileID     string `json:"file_id"`
		ShardIndex int    `json:"shard_index"`
		PeerID     string `json:"peer_id"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	// Get the peer server
	peer, err := models.GetPeerServerByID(db, payload.PeerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "peer not found")
		return
	}

	// Get the shard data
	shards, err := models.GetFileShardsByFileID(db, payload.FileID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to get shards")
		return
	}

	var targetShard *models.FileShard
	for _, s := range shards {
		if s.ShardIndex == payload.ShardIndex {
			targetShard = s
			break
		}
	}

	if targetShard == nil {
		writeError(w, http.StatusNotFound, "shard not found")
		return
	}

	// TODO: Fetch shard data from storage node and transfer to peer
	// This would involve reading from the node and sending to peer

	log.Printf("Transferring shard %d of file %s to peer %s", payload.ShardIndex, payload.FileID, peer.Name)

	writeJSON(w, http.StatusAccepted, map[string]string{
		"status":   "transferring",
		"file_id":  payload.FileID,
		"peer_id":  peer.ID,
	})
}

// SendFileToPeer sends a file shard to a peer server
func SendFileToPeer(peer *models.PeerServer, fileID, ownerID, filename string, shardIndex int, shardType string, shardData []byte, checksum string) error {
	client, err := createPeerHTTPClient(peer)
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}

	payload := map[string]interface{}{
		"file_id":       fileID,
		"owner_id":      ownerID,
		"filename":      filename,
		"shard_index":   shardIndex,
		"shard_type":    shardType,
		"shard_data":    shardData,
		"checksum":      checksum,
		"source_server": serverURL,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	resp, err := client.Post(
		fmt.Sprintf("%s/v1/peers/receive-file", peer.URL),
		"application/json",
		bytes.NewBuffer(body),
	)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("peer returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}
