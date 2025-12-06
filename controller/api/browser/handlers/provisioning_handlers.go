package handlers

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"storageos/controller/models"
	"storageos/controller/pkg/ca"
	"storageos/controller/pkg/provisioning"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
)

var (
	otpService  *provisioning.OTPService
	nodeService *provisioning.NodeService
	CertAuth    *ca.CertificateAuthority
)

// InitProvisioningHandlers initializes the provisioning service handlers
func InitProvisioningHandlers(db *sql.DB, caDir string) error {
	otpService = provisioning.NewOTPService(db)
	nodeService = provisioning.NewNodeService(db)

	// Initialize or load CA
	var err error
	CertAuth, err = ca.NewCA(caDir)
	if err != nil {
		return fmt.Errorf("initialize CA: %w", err)
	}

	log.Println("✓ Certificate Authority initialized")
	return nil
}

// GenerateOTPHandler generates a new OTP for enrollment
// POST /v1/provisioning/generate-otp
func GenerateOTPHandler(w http.ResponseWriter, r *http.Request) {
	// Get user ID from context (set by auth middleware)
	userID, ok := r.Context().Value("user_id").(string)
	if !ok {
		userID = "system" // Fallback if not authenticated
	}

	otp, err := otpService.GenerateOTP(userID)
	if err != nil {
		log.Printf("Error generating OTP: %v", err)
		http.Error(w, "Failed to generate OTP", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"otp":        otp,
		"valid_for":  "5 minutes",
		"expires_at": "in 5 minutes",
	})
}

// RegisterRequestRequest represents the enrollment request from OS node
type RegisterRequestRequest struct {
	OTP        string                 `json:"otp"`
	CSR        string                 `json:"csr"`
	NodeURL    string                 `json:"node_url"`
	SystemInfo map[string]interface{} `json:"system_info"`
}

// RegisterNodeRequest represents the node registration request
type RegisterNodeRequest struct {
	NodeID string `json:"node_id"`
	URL    string `json:"url"`
}

// RegisterRequestHandler handles enrollment requests from OS nodes
// POST /v1/provisioning/register-request
func RegisterRequestHandler(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.OTP == "" || req.CSR == "" {
		http.Error(w, "OTP and CSR are required", http.StatusBadRequest)
		return
	}

	// Validate node URL is provided
	if req.NodeURL == "" {
		http.Error(w, "node_url is required - this is how the controller will reach this node", http.StatusBadRequest)
		return
	}

	// Validate OTP
	valid, errMsg, err := otpService.ValidateOTP(req.OTP)
	if err != nil {
		log.Printf("Error validating OTP: %v", err)
		http.Error(w, "Failed to validate OTP", http.StatusInternalServerError)
		return
	}

	if !valid {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(map[string]string{
			"error": errMsg,
		})
		return
	}

	// Decode and validate CSR
	_, err = base64.StdEncoding.DecodeString(req.CSR)
	if err != nil {
		http.Error(w, "Invalid CSR encoding", http.StatusBadRequest)
		return
	}

	// TODO: Additional CSR validation could be added here
	// The CSR is validated and signed later during approval

	// Add node_url to system info for storage
	if req.SystemInfo == nil {
		req.SystemInfo = make(map[string]interface{})
	}
	req.SystemInfo["node_url"] = req.NodeURL

	// Convert system info to JSON string
	systemInfoJSON, err := json.Marshal(req.SystemInfo)
	if err != nil {
		log.Printf("Error marshaling system info: %v", err)
		http.Error(w, "Invalid system info", http.StatusBadRequest)
		return
	}

	// Create enrollment request
	requestID, err := otpService.CreateEnrollmentRequest(req.OTP, req.CSR, string(systemInfoJSON))
	if err != nil {
		log.Printf("Error creating enrollment request: %v", err)
		http.Error(w, "Failed to create enrollment request", http.StatusInternalServerError)
		return
	}

	log.Printf("New enrollment request created: %s (OTP: %s, NodeURL: %s)", requestID, req.OTP, req.NodeURL)

	// Return success - admin must approve
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "pending",
		"message":    "Enrollment request submitted. Waiting for administrator approval.",
		"request_id": requestID,
	})
}

// ListPendingEnrollmentsHandler lists all pending enrollment requests
// GET /v1/provisioning/enrollments/pending
func ListPendingEnrollmentsHandler(w http.ResponseWriter, r *http.Request) {
	requests, err := otpService.ListPendingEnrollmentRequests()
	if err != nil {
		log.Printf("Error listing pending enrollments: %v", err)
		http.Error(w, "Failed to list enrollments", http.StatusInternalServerError)
		return
	}

	// Parse system info for each request
	type EnhancedRequest struct {
		provisioning.EnrollmentRequest
		ParsedSystemInfo map[string]interface{} `json:"parsed_system_info"`
	}

	enhanced := make([]EnhancedRequest, 0, len(requests))
	for _, req := range requests {
		var systemInfo map[string]interface{}
		if err := json.Unmarshal([]byte(req.SystemInfo), &systemInfo); err != nil {
			systemInfo = map[string]interface{}{"error": "failed to parse"}
		}

		enhanced = append(enhanced, EnhancedRequest{
			EnrollmentRequest: req,
			ParsedSystemInfo:  systemInfo,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count":    len(enhanced),
		"requests": enhanced,
	})
}

// ApproveEnrollmentRequest represents the approval request
type ApproveEnrollmentRequest struct {
	RequestID string `json:"request_id"`
}

// ApproveEnrollmentHandler approves an enrollment request and issues certificate
// POST /v1/provisioning/enrollments/{requestID}/approve
func ApproveEnrollmentHandler(w http.ResponseWriter, r *http.Request) {
	requestID := chi.URLParam(r, "requestID")
	if requestID == "" {
		http.Error(w, "Request ID is required", http.StatusBadRequest)
		return
	}

	// Get user ID from context
	userID, ok := r.Context().Value("user_id").(string)
	if !ok {
		userID = "system"
	}

	// Get enrollment request
	enrollReq, err := otpService.GetEnrollmentRequest(requestID)
	if err != nil {
		log.Printf("Error getting enrollment request: %v", err)
		http.Error(w, "Enrollment request not found", http.StatusNotFound)
		return
	}

	if enrollReq.Status != "pending" {
		http.Error(w, "Request already processed", http.StatusBadRequest)
		return
	}

	// Decode CSR
	csrBytes, err := base64.StdEncoding.DecodeString(enrollReq.CSR)
	if err != nil {
		log.Printf("Error decoding CSR: %v", err)
		http.Error(w, "Invalid CSR", http.StatusBadRequest)
		return
	}

	// Generate node ID (will be used as CN in certificate)
	nodeID := fmt.Sprintf("node-%s", uuid.New().String())
	
	// Sign CSR with CA
	certPEM, expiresAt, err := CertAuth.SignCSR(csrBytes, nodeID)
	if err != nil {
		log.Printf("Error signing CSR: %v", err)
		http.Error(w, "Failed to sign certificate", http.StatusInternalServerError)
		return
	}

	// Calculate certificate fingerprint
	fingerprint, err := ca.GetCertificateFingerprint(certPEM)
	if err != nil {
		log.Printf("Error calculating fingerprint: %v", err)
		http.Error(w, "Failed to process certificate", http.StatusInternalServerError)
		return
	}

	// Register node
	actualNodeID, err := nodeService.RegisterNode(
		requestID,
		base64.StdEncoding.EncodeToString(certPEM),
		fingerprint,
		nodeID,
		enrollReq.SystemInfo,
		expiresAt,
	)
	if err != nil {
		log.Printf("Error registering node: %v", err)
		http.Error(w, "Failed to register node", http.StatusInternalServerError)
		return
	}

	// Approve the enrollment request
	if err := otpService.ApproveEnrollmentRequest(requestID, userID); err != nil {
		log.Printf("Error approving request: %v", err)
		http.Error(w, "Failed to approve request", http.StatusInternalServerError)
		return
	}

	// Mark OTP as used
	if err := otpService.MarkOTPUsed(enrollReq.OTP, actualNodeID); err != nil {
		log.Printf("Warning: failed to mark OTP as used: %v", err)
	}

	// Auto-register as storage node if node_url was provided
	var systemInfo map[string]interface{}
	if err := json.Unmarshal([]byte(enrollReq.SystemInfo), &systemInfo); err == nil {
		if nodeURL, ok := systemInfo["node_url"].(string); ok && nodeURL != "" {
			// Create storage node entry automatically
			storageNode := &models.StorageNode{
				ID:       actualNodeID,
				Name:     fmt.Sprintf("OS Node %s", actualNodeID),
				URL:      nodeURL,
				IsActive: true,
			}

			// Try to create storage node
			_, err := models.CreateStorageNode(db, storageNode)
			if err != nil {
				log.Printf("Warning: failed to auto-register storage node: %v", err)
			} else {
				log.Printf("Auto-registered storage node %s at %s", actualNodeID, nodeURL)
				
				// Reload storage node endpoints
				if osNodeClient := GetOSNodeClient(); osNodeClient != nil {
					if err := osNodeClient.Reload(); err != nil {
						log.Printf("Warning: failed to reload storage nodes: %v", err)
					}
				}
			}
		}
	}

	log.Printf("Enrollment approved: node %s (request %s)", actualNodeID, requestID)

	// Return certificate to node
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"node_id":        actualNodeID,
		"certificate":    base64.StdEncoding.EncodeToString(certPEM),
		"ca_certificate": CertAuth.GetCACertBase64(),
		"expires_at":     expiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// RejectEnrollmentRequest represents the rejection request
type RejectEnrollmentRequest struct {
	Reason string `json:"reason"`
}

// RejectEnrollmentHandler rejects an enrollment request
// POST /v1/provisioning/enrollments/{requestID}/reject
func RejectEnrollmentHandler(w http.ResponseWriter, r *http.Request) {
	requestID := chi.URLParam(r, "requestID")
	if requestID == "" {
		http.Error(w, "Request ID is required", http.StatusBadRequest)
		return
	}

	var req RejectEnrollmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Reason = "Rejected by administrator"
	}

	// Get user ID from context
	userID, ok := r.Context().Value("user_id").(string)
	if !ok {
		userID = "system"
	}

	// Reject the request
	if err := otpService.RejectEnrollmentRequest(requestID, userID, req.Reason); err != nil {
		log.Printf("Error rejecting request: %v", err)
		http.Error(w, "Failed to reject request", http.StatusInternalServerError)
		return
	}

	log.Printf("Enrollment rejected: request %s (reason: %s)", requestID, req.Reason)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "rejected",
		"message": "Enrollment request rejected",
	})
}

// CheckEnrollmentStatusHandler allows OS node to poll for approval status
// GET /v1/provisioning/register-status/{requestID}
func CheckEnrollmentStatusHandler(w http.ResponseWriter, r *http.Request) {
	requestID := chi.URLParam(r, "requestID")
	if requestID == "" {
		http.Error(w, "Request ID is required", http.StatusBadRequest)
		return
	}

	enrollReq, err := otpService.GetEnrollmentRequest(requestID)
	if err != nil {
		http.Error(w, "Request not found", http.StatusNotFound)
		return
	}

	response := map[string]interface{}{
		"status":       enrollReq.Status,
		"requested_at": enrollReq.RequestedAt,
	}

	if enrollReq.Status == "approved" {
		// Find the node that was created for this enrollment request
		nodes, err := nodeService.ListNodes()
		if err == nil {
			for _, node := range nodes {
				if node.EnrollmentRequestID == requestID {
					response["message"] = "Enrollment approved. Certificate issued."
					response["node_id"] = node.ID
					response["certificate"] = node.Certificate
					response["ca_certificate"] = CertAuth.GetCACertBase64()
					response["expires_at"] = node.CertificateExpiresAt
					break
				}
			}
		}
		// Fallback message if node not found
		if response["node_id"] == nil {
			response["message"] = "Enrollment approved. Certificate issued."
		}
	} else if enrollReq.Status == "rejected" {
		response["message"] = "Enrollment rejected"
	} else {
		response["message"] = "Waiting for administrator approval"
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// ListNodesHandler lists all enrolled nodes
// GET /v1/provisioning/nodes
func ListNodesHandler(w http.ResponseWriter, r *http.Request) {
	nodes, err := nodeService.ListNodes()
	if err != nil {
		log.Printf("Error listing nodes: %v", err)
		http.Error(w, "Failed to list nodes", http.StatusInternalServerError)
		return
	}

	// Parse system info for each node
	type EnhancedNode struct {
		provisioning.OSNode
		ParsedSystemInfo map[string]interface{} `json:"parsed_system_info"`
	}

	enhanced := make([]EnhancedNode, 0, len(nodes))
	for _, node := range nodes {
		var systemInfo map[string]interface{}
		if err := json.Unmarshal([]byte(node.SystemInfo), &systemInfo); err != nil {
			systemInfo = map[string]interface{}{"error": "failed to parse"}
		}

		enhanced = append(enhanced, EnhancedNode{
			OSNode:           node,
			ParsedSystemInfo: systemInfo,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"count": len(enhanced),
		"nodes": enhanced,
	})
}

// RevokeNodeRequest represents a revocation request
type RevokeNodeRequest struct {
	Reason string `json:"reason"`
}

// RevokeNodeHandler revokes a node's certificate
// POST /v1/provisioning/nodes/{nodeID}/revoke
func RevokeNodeHandler(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "nodeID")
	if nodeID == "" {
		http.Error(w, "Node ID is required", http.StatusBadRequest)
		return
	}

	var req RevokeNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		req.Reason = "Revoked by administrator"
	}

	// Get user ID from context
	userID, ok := r.Context().Value("user_id").(string)
	if !ok {
		userID = "system"
	}

	// Revoke the node certificate
	if err := nodeService.RevokeNode(nodeID, userID, req.Reason); err != nil {
		log.Printf("Error revoking node: %v", err)
		http.Error(w, "Failed to revoke node", http.StatusInternalServerError)
		return
	}

	// Mark storage node as unavailable
	if err := markStorageNodeUnavailable(nodeID, req.Reason); err != nil {
		log.Printf("Warning: failed to mark storage node as unavailable: %v", err)
		// Don't fail the request, node was still revoked
	}

	// Reload storage node endpoints to exclude revoked node
	if osNodeClient := GetOSNodeClient(); osNodeClient != nil {
		if err := osNodeClient.Reload(); err != nil {
			log.Printf("Warning: failed to reload storage nodes: %v", err)
		}
	}

	log.Printf("Node revoked: %s (reason: %s)", nodeID, req.Reason)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "revoked",
		"message": "Node certificate revoked and storage node marked as unavailable",
	})
}

// RenewCertificateHandler handles certificate renewal requests from nodes
// POST /v1/provisioning/renew-certificate
func RenewCertificateHandler(w http.ResponseWriter, r *http.Request) {
	// Node must be authenticated via mTLS (certificate verified by middleware)
	nodeID, ok := r.Context().Value("node_id").(string)
	if !ok {
		http.Error(w, "Node not authenticated", http.StatusUnauthorized)
		return
	}

	var req RegisterRequestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Decode CSR
	csrBytes, err := base64.StdEncoding.DecodeString(req.CSR)
	if err != nil {
		http.Error(w, "Invalid CSR encoding", http.StatusBadRequest)
		return
	}

	// Sign new certificate
	certPEM, expiresAt, err := CertAuth.SignCSR(csrBytes, nodeID)
	if err != nil {
		log.Printf("Error signing renewal CSR: %v", err)
		http.Error(w, "Failed to sign certificate", http.StatusInternalServerError)
		return
	}

	// Calculate new fingerprint
	fingerprint, err := ca.GetCertificateFingerprint(certPEM)
	if err != nil {
		log.Printf("Error calculating fingerprint: %v", err)
		http.Error(w, "Failed to process certificate", http.StatusInternalServerError)
		return
	}

	// Update node certificate
	if err := nodeService.RenewCertificate(nodeID, base64.StdEncoding.EncodeToString(certPEM), fingerprint, expiresAt); err != nil {
		log.Printf("Error renewing certificate: %v", err)
		http.Error(w, "Failed to renew certificate", http.StatusInternalServerError)
		return
	}

	log.Printf("Certificate renewed for node: %s", nodeID)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"certificate":    base64.StdEncoding.EncodeToString(certPEM),
		"ca_certificate": CertAuth.GetCACertBase64(),
		"expires_at":     expiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// GenerateBrowserCertificateRequest represents the browser certificate request
type GenerateBrowserCertificateRequest struct {
	EncryptionKey string `json:"encryption_key"`
	CSR           string `json:"csr"`
	UserID        string `json:"user_id"`
}

// RegisterNodeHandler allows an enrolled OS node to register itself as a storage node
// POST /v1/provisioning/register-node
func RegisterNodeHandler(w http.ResponseWriter, r *http.Request) {
	var req RegisterNodeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.NodeID == "" || req.URL == "" {
		http.Error(w, "node_id and url are required", http.StatusBadRequest)
		return
	}

	// Verify the node is enrolled
	node, err := nodeService.GetNodeByID(req.NodeID)
	if err != nil {
		log.Printf("Error getting node %s: %v", req.NodeID, err)
		http.Error(w, "Node not found or not enrolled", http.StatusNotFound)
		return
	}

	if node.Status != "active" {
		http.Error(w, "Node is not active", http.StatusBadRequest)
		return
	}

	// Create storage node entry
	storageNode := &models.StorageNode{
		ID:        req.NodeID,
		Name:      fmt.Sprintf("OS Node %s", req.NodeID),
		URL:       req.URL,
		IsActive:  true,
	}

	// Try to create, if it exists, update
	existing, err := models.GetStorageNodeByID(db, req.NodeID)
	if err == nil {
		// Update existing
		storageNode.ID = existing.ID
		_, err = models.UpdateStorageNode(db, storageNode)
		if err != nil {
			log.Printf("Error updating storage node: %v", err)
			http.Error(w, "Failed to update storage node", http.StatusInternalServerError)
			return
		}
	} else {
		// Create new
		_, err = models.CreateStorageNode(db, storageNode)
		if err != nil {
			log.Printf("Error creating storage node: %v", err)
			http.Error(w, "Failed to create storage node", http.StatusInternalServerError)
			return
		}
	}

	log.Printf("OS node %s registered as storage node at %s", req.NodeID, req.URL)

	// Reload storage node endpoints
	if osNodeClient := GetOSNodeClient(); osNodeClient != nil {
		if err := osNodeClient.Reload(); err != nil {
			log.Printf("Warning: failed to reload storage nodes: %v", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "registered",
		"message": "Node registered as storage node",
	})
}

// GenerateBrowserCertificateHandler generates a client certificate for browser mTLS
// POST /v1/browser/get-certificate
func GenerateBrowserCertificateHandler(w http.ResponseWriter, r *http.Request) {
	var req GenerateBrowserCertificateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate required fields
	if req.EncryptionKey == "" || req.CSR == "" || req.UserID == "" {
		http.Error(w, "Encryption key, CSR, and user_id are required", http.StatusBadRequest)
		return
	}

	// Validate encryption key against environment variable
	expectedKey := os.Getenv("MTLS_ENCRYPTION_KEY")
	if expectedKey == "" {
		log.Printf("MTLS_ENCRYPTION_KEY not set")
		http.Error(w, "Server configuration error", http.StatusInternalServerError)
		return
	}

	if req.EncryptionKey != expectedKey {
		log.Printf("Invalid encryption key provided")
		http.Error(w, "Invalid encryption key", http.StatusUnauthorized)
		return
	}

	// Parse the simplified CSR (JSON format with base64 public key)
	// Remove PEM headers/footers and decode
	csrData := strings.TrimSpace(req.CSR)
	csrData = strings.Replace(csrData, "-----BEGIN CERTIFICATE REQUEST-----", "", 1)
	csrData = strings.Replace(csrData, "-----END CERTIFICATE REQUEST-----", "", 1)
	csrData = strings.Replace(csrData, "\n", "", -1)

	// Decode base64 JSON
	csrJsonBytes, err := base64.StdEncoding.DecodeString(csrData)
	if err != nil {
		log.Printf("Error decoding CSR JSON: %v", err)
		http.Error(w, "Invalid CSR format", http.StatusBadRequest)
		return
	}

	// Parse JSON
	var csrInfo struct {
		Subject struct {
			CommonName       string `json:"commonName"`
			OrganizationName string `json:"organizationName"`
		} `json:"subject"`
		PublicKey string `json:"publicKey"`
	}
	if err := json.Unmarshal(csrJsonBytes, &csrInfo); err != nil {
		log.Printf("Error parsing CSR JSON: %v", err)
		http.Error(w, "Invalid CSR structure", http.StatusBadRequest)
		return
	}

	// Decode the public key
	publicKeyBytes, err := base64.StdEncoding.DecodeString(csrInfo.PublicKey)
	if err != nil {
		log.Printf("Error decoding public key: %v", err)
		http.Error(w, "Invalid public key in CSR", http.StatusBadRequest)
		return
	}

	// Generate browser certificate CN
	browserID := fmt.Sprintf("browser-%s", req.UserID)

	// For this simplified approach, we'll create a certificate directly from the public key
	certPEM, expiresAt, err := CertAuth.SignCertificateFromPublicKey(publicKeyBytes, browserID)
	if err != nil {
		log.Printf("Error signing browser certificate: %v", err)
		http.Error(w, "Failed to sign certificate", http.StatusInternalServerError)
		return
	}

	log.Printf("Browser certificate generated for user: %s", req.UserID)

	// Return certificate to browser
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"certificate":    base64.StdEncoding.EncodeToString(certPEM),
		"ca_certificate": CertAuth.GetCACertBase64(),
		"expires_at":     expiresAt.Format("2006-01-02T15:04:05Z07:00"),
	})
}

// NodeStatusHandler allows OS nodes to check their status with the controller
// GET /v1/provisioning/node-status/{nodeID}
func NodeStatusHandler(w http.ResponseWriter, r *http.Request) {
	nodeID := chi.URLParam(r, "nodeID")
	if nodeID == "" {
		http.Error(w, "Node ID is required", http.StatusBadRequest)
		return
	}

	// Get node status
	node, err := nodeService.GetNodeByID(nodeID)
	if err != nil {
		log.Printf("Error getting node %s: %v", nodeID, err)
		http.Error(w, "Node not found", http.StatusNotFound)
		return
	}

	// Update last seen timestamp
	nodeService.UpdateLastSeen(nodeID)

	response := map[string]interface{}{
		"node_id": node.ID,
		"status":  node.Status,
	}

	// Include revocation details if revoked
	if node.Status == "revoked" {
		if node.RevokedAt != nil {
			response["revoked_at"] = *node.RevokedAt
		}
		if node.RevocationReason != nil {
			response["revocation_reason"] = *node.RevocationReason
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// NodeRemovalRequest represents a node removal notification from the OS
type NodeRemovalRequest struct {
	NodeID    string `json:"node_id"`
	Action    string `json:"action"`
	Reason    string `json:"reason"`
	RemovedAt string `json:"removed_at"`
}

// NodeRemovalHandler handles notifications from OS nodes when they remove themselves
// POST /v1/provisioning/node-removal
func NodeRemovalHandler(w http.ResponseWriter, r *http.Request) {
	var req NodeRemovalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.NodeID == "" {
		http.Error(w, "node_id is required", http.StatusBadRequest)
		return
	}

	log.Printf("Node removal notification received from %s: %s", req.NodeID, req.Reason)

	// Mark storage node as inactive/unavailable
	if err := markStorageNodeUnavailable(req.NodeID, req.Reason); err != nil {
		log.Printf("Warning: failed to mark storage node as unavailable: %v", err)
	}

	// Update node status to 'removed' (different from 'revoked')
	if err := nodeService.MarkNodeRemoved(req.NodeID, req.Reason); err != nil {
		log.Printf("Warning: failed to update node status: %v", err)
	}

	// Reload storage node endpoints
	if osNodeClient := GetOSNodeClient(); osNodeClient != nil {
		if err := osNodeClient.Reload(); err != nil {
			log.Printf("Warning: failed to reload storage nodes: %v", err)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "acknowledged",
		"message": "Node removal acknowledged",
	})
}

// markStorageNodeUnavailable marks a storage node as inactive
func markStorageNodeUnavailable(nodeID, reason string) error {
	// Get storage node
	storageNode, err := models.GetStorageNodeByID(db, nodeID)
	if err != nil {
		return fmt.Errorf("storage node not found: %w", err)
	}

	// Mark as inactive
	storageNode.IsActive = false
	_, err = models.UpdateStorageNode(db, storageNode)
	if err != nil {
		return fmt.Errorf("update storage node: %w", err)
	}

	log.Printf("Storage node %s marked as unavailable (reason: %s)", nodeID, reason)
	return nil
}
