package models

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// PeerServer represents a federated server connection
type PeerServer struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	URL            string `json:"url"`
	PeerType       string `json:"peer_type"` // "inbound" or "outbound"
	RequestStatus  string `json:"request_status"` // "pending", "accepted", "rejected", "revoked"
	QuotaAllocated int64  `json:"quota_allocated"`
	QuotaUsed      int64  `json:"quota_used"`
	IsActive       bool   `json:"is_active"`
	LastActive     string `json:"last_active,omitempty"`
	RequestToken   string `json:"request_token,omitempty"`
	RequestedAt    string `json:"requested_at"`
	AcceptedAt     string `json:"accepted_at,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
	// mTLS certificates for secure communication
	ClientCert     string `json:"client_cert,omitempty"`     // Certificate for this server to authenticate to peer
	ClientKey      string `json:"client_key,omitempty"`      // Private key for client certificate
	ServerCert     string `json:"server_cert,omitempty"`     // Certificate for peer server authentication
}

// CreatePeerServer creates a new peer server connection request
func CreatePeerServer(db *sql.DB, peer *PeerServer) (*PeerServer, error) {
	if peer == nil {
		return nil, errors.New("peer server payload is required")
	}

	peer.Name = strings.TrimSpace(peer.Name)
	peer.URL = strings.TrimSpace(peer.URL)

	if peer.Name == "" {
		return nil, errors.New("peer server name is required")
	}
	if peer.URL == "" {
		return nil, errors.New("peer server URL is required")
	}
	if peer.PeerType != "inbound" && peer.PeerType != "outbound" {
		return nil, errors.New("peer_type must be 'inbound' or 'outbound'")
	}

	if peer.ID == "" {
		peer.ID = uuid.New().String()
	}
	if peer.RequestStatus == "" {
		peer.RequestStatus = "pending"
	}
	if peer.RequestToken == "" {
		peer.RequestToken = uuid.New().String()
	}

	stmt := `INSERT INTO peer_servers (
		id, name, url, peer_type, request_status, quota_allocated, quota_used,
		is_active, request_token, client_cert, client_key, server_cert
	) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	_, err := db.Exec(stmt,
		peer.ID,
		peer.Name,
		peer.URL,
		peer.PeerType,
		peer.RequestStatus,
		peer.QuotaAllocated,
		peer.QuotaUsed,
		peer.IsActive,
		peer.RequestToken,
		peer.ClientCert,
		peer.ClientKey,
		peer.ServerCert,
	)
	if err != nil {
		return nil, fmt.Errorf("create peer server: %w", err)
	}

	return GetPeerServerByID(db, peer.ID)
}

// UpdatePeerServer updates an existing peer server
func UpdatePeerServer(db *sql.DB, peer *PeerServer) (*PeerServer, error) {
	if peer == nil || strings.TrimSpace(peer.ID) == "" {
		return nil, errors.New("peer server id is required")
	}

	stmt := `UPDATE peer_servers
	         SET name = ?, url = ?, request_status = ?, quota_allocated = ?,
			     quota_used = ?, is_active = ?, client_cert = ?, client_key = ?, 
			     server_cert = ?, updated_at = CURRENT_TIMESTAMP
	         WHERE id = ?`

	res, err := db.Exec(stmt,
		peer.Name,
		peer.URL,
		peer.RequestStatus,
		peer.QuotaAllocated,
		peer.QuotaUsed,
		peer.IsActive,
		peer.ClientCert,
		peer.ClientKey,
		peer.ServerCert,
		peer.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("update peer server: %w", err)
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		return nil, sql.ErrNoRows
	}

	return GetPeerServerByID(db, peer.ID)
}

// AcceptPeerRequest accepts an inbound peer connection and sets quota
func AcceptPeerRequest(db *sql.DB, peerID string, quotaAllocated int64) error {
	stmt := `UPDATE peer_servers
	         SET request_status = 'accepted', quota_allocated = ?,
			     accepted_at = ?, updated_at = CURRENT_TIMESTAMP
	         WHERE id = ? AND peer_type = 'inbound' AND request_status = 'pending'`

	now := time.Now().Format(time.RFC3339)
	res, err := db.Exec(stmt, quotaAllocated, now, peerID)
	if err != nil {
		return fmt.Errorf("accept peer request: %w", err)
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		return errors.New("peer request not found or already processed")
	}

	return nil
}

// RejectPeerRequest rejects an inbound peer connection request
func RejectPeerRequest(db *sql.DB, peerID string) error {
	stmt := `UPDATE peer_servers
	         SET request_status = 'rejected', updated_at = CURRENT_TIMESTAMP
	         WHERE id = ? AND peer_type = 'inbound' AND request_status = 'pending'`

	res, err := db.Exec(stmt, peerID)
	if err != nil {
		return fmt.Errorf("reject peer request: %w", err)
	}

	affected, _ := res.RowsAffected()
	if affected == 0 {
		return errors.New("peer request not found or already processed")
	}

	return nil
}

// UpdatePeerServerStatus updates the health status of a peer server
func UpdatePeerServerStatus(db *sql.DB, peerID string, isActive bool) error {
	now := time.Now().Format(time.RFC3339)
	stmt := `UPDATE peer_servers
	         SET is_active = ?, last_active = ?, updated_at = CURRENT_TIMESTAMP
	         WHERE id = ?`

	_, err := db.Exec(stmt, isActive, now, peerID)
	return err
}

// DeletePeerServer removes a peer server connection
func DeletePeerServer(db *sql.DB, id string) error {
	if strings.TrimSpace(id) == "" {
		return errors.New("peer server id is required")
	}
	_, err := db.Exec(`DELETE FROM peer_servers WHERE id = ?`, id)
	return err
}

// ListPeerServers returns all peer servers
func ListPeerServers(db *sql.DB) ([]*PeerServer, error) {
	stmt := `SELECT id, name, url, peer_type, request_status, quota_allocated, quota_used,
	                is_active, last_active, request_token, requested_at, accepted_at,
	                created_at, updated_at, client_cert, client_key, server_cert
	         FROM peer_servers
	         ORDER BY created_at DESC`

	rows, err := db.Query(stmt)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []*PeerServer
	for rows.Next() {
		peer, err := scanPeerServer(rows)
		if err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}
	return peers, rows.Err()
}

// ListPeerServersByType returns peer servers filtered by type
func ListPeerServersByType(db *sql.DB, peerType string) ([]*PeerServer, error) {
	stmt := `SELECT id, name, url, peer_type, request_status, quota_allocated, quota_used,
	                is_active, last_active, request_token, requested_at, accepted_at,
	                created_at, updated_at, client_cert, client_key, server_cert
	         FROM peer_servers
	         WHERE peer_type = ?
	         ORDER BY created_at DESC`

	rows, err := db.Query(stmt, peerType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var peers []*PeerServer
	for rows.Next() {
		peer, err := scanPeerServer(rows)
		if err != nil {
			return nil, err
		}
		peers = append(peers, peer)
	}
	return peers, rows.Err()
}

// GetPeerServerByID fetches a peer server by ID
func GetPeerServerByID(db *sql.DB, id string) (*PeerServer, error) {
	stmt := `SELECT id, name, url, peer_type, request_status, quota_allocated, quota_used,
	                is_active, last_active, request_token, requested_at, accepted_at,
	                created_at, updated_at, client_cert, client_key, server_cert
	         FROM peer_servers
	         WHERE id = ?`

	row := db.QueryRow(stmt, id)
	return scanPeerServer(row)
}

// GetPeerServerByToken fetches a peer server by request token
func GetPeerServerByToken(db *sql.DB, token string) (*PeerServer, error) {
	stmt := `SELECT id, name, url, peer_type, request_status, quota_allocated, quota_used,
	                is_active, last_active, request_token, requested_at, accepted_at,
	                created_at, updated_at, client_cert, client_key, server_cert
	         FROM peer_servers
	         WHERE request_token = ?`

	row := db.QueryRow(stmt, token)
	return scanPeerServer(row)
}

type peerRowScanner interface {
	Scan(dest ...any) error
}

func scanPeerServer(scanner peerRowScanner) (*PeerServer, error) {
	var (
		id             string
		name           string
		url            string
		peerType       string
		requestStatus  string
		quotaAllocated int64
		quotaUsed      int64
		isActive       sql.NullBool
		lastActive     sql.NullString
		requestToken   sql.NullString
		requestedAt    sql.NullString
		acceptedAt     sql.NullString
		createdAt      sql.NullString
		updatedAt      sql.NullString
		clientCert     sql.NullString
		clientKey      sql.NullString
		serverCert     sql.NullString
	)

	err := scanner.Scan(
		&id, &name, &url, &peerType, &requestStatus,
		&quotaAllocated, &quotaUsed, &isActive, &lastActive,
		&requestToken, &requestedAt, &acceptedAt,
		&createdAt, &updatedAt, &clientCert, &clientKey, &serverCert,
	)
	if err != nil {
		return nil, err
	}

	peer := &PeerServer{
		ID:             id,
		Name:           name,
		URL:            url,
		PeerType:       peerType,
		RequestStatus:  requestStatus,
		QuotaAllocated: quotaAllocated,
		QuotaUsed:      quotaUsed,
		IsActive:       isActive.Valid && isActive.Bool,
		LastActive:     lastActive.String,
		RequestToken:   requestToken.String,
		RequestedAt:    requestedAt.String,
		AcceptedAt:     acceptedAt.String,
		CreatedAt:      createdAt.String,
		UpdatedAt:      updatedAt.String,
		ClientCert:     clientCert.String,
		ClientKey:      clientKey.String,
		ServerCert:     serverCert.String,
	}

	return peer, nil
}
