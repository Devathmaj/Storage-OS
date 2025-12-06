package handlers

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"storageos/controller/models"
)

var (
	serverName string
	serverURL  string
)

// InitPeerServerHandlers initializes peer server handlers with configuration
func InitPeerServerHandlers(database interface{}, name, url string) {
	db = database.(*sql.DB)
	serverName = name
	serverURL = url
	log.Printf("Peer server handlers initialized: %s (%s)", serverName, serverURL)
}

type peerServerRequest struct {
	Name string `json:"name"`
	URL  string `json:"url"`
}

type peerConnectionRequest struct {
	RequestToken   string `json:"request_token"`
	SourceName     string `json:"source_name"`
	SourceURL      string `json:"source_url"`
	RequestedQuota int64  `json:"requested_quota"`
}

type peerAcceptRequest struct {
	QuotaAllocated int64 `json:"quota_allocated"`
}

// ListPeerServersHandler returns all peer server connections
func ListPeerServersHandler(w http.ResponseWriter, r *http.Request) {
	peerType := r.URL.Query().Get("type") // "inbound", "outbound", or empty for all

	var peers []*models.PeerServer
	var err error

	if peerType != "" {
		peers, err = models.ListPeerServersByType(db, peerType)
	} else {
		peers, err = models.ListPeerServers(db)
	}

	if err != nil {
		log.Printf("ERROR: failed to list peer servers: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to list peer servers")
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"peers": peers,
		"count": len(peers),
	})
}

// RequestPeerConnectionHandler initiates a connection request to another server
func RequestPeerConnectionHandler(w http.ResponseWriter, r *http.Request) {
	var payload peerServerRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if payload.Name == "" || payload.URL == "" {
		writeError(w, http.StatusBadRequest, "name and url are required")
		return
	}

	// Create outbound peer record (pending)
	peer := &models.PeerServer{
		Name:     payload.Name,
		URL:      payload.URL,
		PeerType: "outbound",
	}

	created, err := models.CreatePeerServer(db, peer)
	if err != nil {
		log.Printf("RequestPeerConnectionHandler: failed to create peer: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Send connection request to the peer server
	go sendPeerConnectionRequest(created)

	log.Printf("Peer connection request created: %s -> %s (token: %s)", created.Name, created.URL, created.RequestToken)
	writeJSON(w, http.StatusCreated, created)
}

// ReceivePeerConnectionHandler receives a connection request from another server
func ReceivePeerConnectionHandler(w http.ResponseWriter, r *http.Request) {
	var payload peerConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if payload.RequestToken == "" || payload.SourceName == "" || payload.SourceURL == "" {
		writeError(w, http.StatusBadRequest, "request_token, source_name, and source_url are required")
		return
	}

	// Create inbound peer record (pending approval)
	peer := &models.PeerServer{
		Name:          payload.SourceName,
		URL:           payload.SourceURL,
		PeerType:      "inbound",
		RequestToken:  payload.RequestToken,
		RequestStatus: "pending",
	}

	_, err := models.CreatePeerServer(db, peer)
	if err != nil {
		log.Printf("ReceivePeerConnectionHandler: failed to create inbound peer: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("Received peer connection request from: %s (%s)", payload.SourceName, payload.SourceURL)
	writeJSON(w, http.StatusCreated, map[string]string{
		"status":  "pending",
		"message": "Connection request received. Awaiting administrator approval.",
	})
}

// AcceptPeerConnectionHandler accepts an inbound peer connection request
func AcceptPeerConnectionHandler(w http.ResponseWriter, r *http.Request) {
	peerID := chi.URLParam(r, "peerID")
	if peerID == "" {
		writeError(w, http.StatusBadRequest, "peer id is required")
		return
	}

	var payload peerAcceptRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	if payload.QuotaAllocated <= 0 {
		writeError(w, http.StatusBadRequest, "quota_allocated must be greater than 0")
		return
	}

	// Accept the peer request
	if err := models.AcceptPeerRequest(db, peerID, payload.QuotaAllocated); err != nil {
		log.Printf("AcceptPeerConnectionHandler: failed to accept: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Get the updated peer
	peer, err := models.GetPeerServerByID(db, peerID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to fetch updated peer")
		return
	}

	// Generate mTLS certificates for secure communication
	if err := generatePeerCertificates(peer); err != nil {
		log.Printf("AcceptPeerConnectionHandler: failed to generate certificates: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to generate certificates")
		return
	}

	// Update peer with certificates
	if _, err := models.UpdatePeerServer(db, peer); err != nil {
		log.Printf("AcceptPeerConnectionHandler: failed to update peer with certificates: %v", err)
		writeError(w, http.StatusInternalServerError, "failed to save certificates")
		return
	}

	// Notify the requesting server that the connection was accepted
	go notifyPeerAccepted(peer)

	log.Printf("Accepted peer connection: %s (quota: %d bytes) with mTLS certificates", peer.Name, peer.QuotaAllocated)
	writeJSON(w, http.StatusOK, peer)
}

// RejectPeerConnectionHandler rejects an inbound peer connection request
func RejectPeerConnectionHandler(w http.ResponseWriter, r *http.Request) {
	peerID := chi.URLParam(r, "peerID")
	if peerID == "" {
		writeError(w, http.StatusBadRequest, "peer id is required")
		return
	}

	if err := models.RejectPeerRequest(db, peerID); err != nil {
		log.Printf("RejectPeerConnectionHandler: failed to reject: %v", err)
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	log.Printf("Rejected peer connection: %s", peerID)
	w.WriteHeader(http.StatusNoContent)
}

// ReceivePeerAcceptanceHandler receives acceptance notification from peer server
func ReceivePeerAcceptanceHandler(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		RequestToken   string `json:"request_token"`
		QuotaAllocated int64  `json:"quota_allocated"`
		ClientCert     string `json:"client_cert"` // Certificate for this server to use as client
		ServerCert     string `json:"server_cert"` // Certificate for peer server
		CACert         string `json:"ca_cert"`     // CA certificate for validation
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	// Find the outbound peer by token
	peer, err := models.GetPeerServerByToken(db, payload.RequestToken)
	if err != nil {
		writeError(w, http.StatusNotFound, "peer request not found")
		return
	}

	// Update the peer status to accepted and store certificates
	peer.RequestStatus = "accepted"
	peer.QuotaAllocated = payload.QuotaAllocated
	peer.ClientCert = payload.ClientCert
	peer.ServerCert = payload.ServerCert

	if _, err := models.UpdatePeerServer(db, peer); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update peer status")
		return
	}

	log.Printf("Peer connection accepted by %s (quota: %d bytes) with mTLS certificates", peer.Name, peer.QuotaAllocated)
	writeJSON(w, http.StatusOK, map[string]string{"status": "accepted"})
}

// DeletePeerServerHandler removes a peer server connection
func DeletePeerServerHandler(w http.ResponseWriter, r *http.Request) {
	peerID := chi.URLParam(r, "peerID")
	if peerID == "" {
		writeError(w, http.StatusBadRequest, "peer id is required")
		return
	}

	if err := models.DeletePeerServer(db, peerID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete peer server")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// Helper: Send connection request to peer server
func sendPeerConnectionRequest(peer *models.PeerServer) {
	requestPayload := peerConnectionRequest{
		RequestToken:   peer.RequestToken,
		SourceName:     serverName,
		SourceURL:      serverURL,
		RequestedQuota: 1073741824, // 1GB default
	}

	body, _ := json.Marshal(requestPayload)
	resp, err := http.Post(
		fmt.Sprintf("%s/v1/peers/receive-request", peer.URL),
		"application/json",
		bytes.NewBuffer(body),
	)

	if err != nil {
		log.Printf("Failed to send peer connection request to %s: %v", peer.URL, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		log.Printf("Peer server %s returned error: %d", peer.URL, resp.StatusCode)
		return
	}

	log.Printf("Successfully sent connection request to %s", peer.URL)
}

// Helper: Notify peer that connection was accepted
func notifyPeerAccepted(peer *models.PeerServer) {
	payload := map[string]interface{}{
		"request_token":   peer.RequestToken,
		"quota_allocated": peer.QuotaAllocated,
		"client_cert":     peer.ClientCert,     // Certificate for peer to use as client
		"server_cert":     peer.ServerCert,     // Certificate for peer to use as server
		"ca_cert":         CertAuth.GetCACertBase64(), // CA certificate for validation
	}

	body, _ := json.Marshal(payload)
	resp, err := http.Post(
		fmt.Sprintf("%s/v1/peers/receive-acceptance", peer.URL),
		"application/json",
		bytes.NewBuffer(body),
	)

	if err != nil {
		log.Printf("Failed to notify peer %s of acceptance: %v", peer.URL, err)
		return
	}
	defer resp.Body.Close()

	log.Printf("Notified peer %s of connection acceptance with mTLS certificates", peer.Name)
}

// UpdatePeerQuotaHandler updates quota usage for a peer
func UpdatePeerQuotaHandler(w http.ResponseWriter, r *http.Request) {
	peerID := chi.URLParam(r, "peerID")
	if peerID == "" {
		writeError(w, http.StatusBadRequest, "peer id is required")
		return
	}

	var payload struct {
		QuotaUsed int64 `json:"quota_used"`
	}

	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	peer, err := models.GetPeerServerByID(db, peerID)
	if err != nil {
		writeError(w, http.StatusNotFound, "peer not found")
		return
	}

	peer.QuotaUsed = payload.QuotaUsed
	updated, err := models.UpdatePeerServer(db, peer)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update quota")
		return
	}

	writeJSON(w, http.StatusOK, updated)
}

// Helper: Generate mTLS certificates for peer communication
func generatePeerCertificates(peer *models.PeerServer) error {
	if CertAuth == nil {
		return fmt.Errorf("certificate authority not initialized")
	}

	// Generate client certificate for this server (to authenticate to peer)
	clientCertPEM, clientKeyPEM, err := CertAuth.GenerateServerCertificate(
		[]string{"localhost"}, // DNS names
		[]string{"127.0.0.1"}, // IP addresses
	)
	if err != nil {
		return fmt.Errorf("generate client certificate: %w", err)
	}

	// For server certificate, we'll generate a certificate that the peer can use
	// In a real implementation, the peer would generate their own server certificate
	// For now, we'll generate a placeholder server certificate for the peer
	serverCertPEM, _, err := CertAuth.GenerateServerCertificate(
		[]string{"localhost"},
		[]string{"127.0.0.1"},
	)
	if err != nil {
		return fmt.Errorf("generate server certificate: %w", err)
	}

	peer.ClientCert = string(clientCertPEM)
	peer.ClientKey = string(clientKeyPEM)
	peer.ServerCert = string(serverCertPEM)

	return nil
}

// Helper: Parse bytes from string (e.g., "1GB" -> 1073741824)
func parseBytes(s string) (int64, error) {
	// Simple parser for now
	var value float64
	var unit string
	_, err := fmt.Sscanf(s, "%f%s", &value, &unit)
	if err != nil {
		return strconv.ParseInt(s, 10, 64)
	}

	multiplier := int64(1)
	switch unit {
	case "KB", "kb":
		multiplier = 1024
	case "MB", "mb":
		multiplier = 1024 * 1024
	case "GB", "gb":
		multiplier = 1024 * 1024 * 1024
	case "TB", "tb":
		multiplier = 1024 * 1024 * 1024 * 1024
	}

	return int64(value * float64(multiplier)), nil
}
