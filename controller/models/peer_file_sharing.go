package models

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// PeerFileSharingSettings represents the global peer file sharing configuration
type PeerFileSharingSettings struct {
	ID                     int    `json:"id"`
	Enabled                bool   `json:"enabled"`
	CompleteStorageEnabled bool   `json:"complete_storage_enabled"`
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
}

// PeerSharingOutbound represents a server we send files to
type PeerSharingOutbound struct {
	ID           string      `json:"id"`
	PeerServerID string      `json:"peer_server_id"`
	PeerServer   *PeerServer `json:"peer_server,omitempty"`
	Enabled      bool        `json:"enabled"`
	Priority     int         `json:"priority"`
	CreatedAt    string      `json:"created_at"`
}

// PeerSharingInbound represents a server that sends files to us
type PeerSharingInbound struct {
	ID           string      `json:"id"`
	PeerServerID string      `json:"peer_server_id"`
	PeerServer   *PeerServer `json:"peer_server,omitempty"`
	Enabled      bool        `json:"enabled"`
	CreatedAt    string      `json:"created_at"`
}

// GetPeerFileSharingSettings retrieves the peer file sharing settings
func GetPeerFileSharingSettings(db *sql.DB) (*PeerFileSharingSettings, error) {
	row := db.QueryRow(`
		SELECT id, enabled, complete_storage_enabled, created_at, updated_at
		FROM peer_file_sharing_settings
		LIMIT 1
	`)

	var settings PeerFileSharingSettings
	var createdAt, updatedAt sql.NullString

	err := row.Scan(&settings.ID, &settings.Enabled, &settings.CompleteStorageEnabled,
		&createdAt, &updatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			// Create default settings if none exist
			_, err := db.Exec(`INSERT INTO peer_file_sharing_settings (enabled, complete_storage_enabled) VALUES (0, 0)`)
			if err != nil {
				return nil, err
			}
			return GetPeerFileSharingSettings(db)
		}
		return nil, err
	}

	settings.CreatedAt = createdAt.String
	settings.UpdatedAt = updatedAt.String

	return &settings, nil
}

// UpdatePeerFileSharingSettings updates the peer file sharing settings
func UpdatePeerFileSharingSettings(db *sql.DB, enabled, completeStorageEnabled bool) (*PeerFileSharingSettings, error) {
	now := time.Now().Format(time.RFC3339)
	_, err := db.Exec(`
		UPDATE peer_file_sharing_settings
		SET enabled = ?, complete_storage_enabled = ?, updated_at = ?
		WHERE id = (SELECT id FROM peer_file_sharing_settings LIMIT 1)
	`, enabled, completeStorageEnabled, now)
	if err != nil {
		return nil, err
	}

	return GetPeerFileSharingSettings(db)
}

// ListPeerSharingOutbound returns all outbound peer sharing configurations
func ListPeerSharingOutbound(db *sql.DB) ([]*PeerSharingOutbound, error) {
	rows, err := db.Query(`
		SELECT pso.id, pso.peer_server_id, pso.enabled, pso.priority, pso.created_at,
		       ps.id, ps.name, ps.url, ps.peer_type, ps.request_status, ps.quota_allocated,
		       ps.quota_used, ps.storage_remaining, ps.is_active
		FROM peer_sharing_outbound pso
		JOIN peer_servers ps ON ps.id = pso.peer_server_id
		WHERE ps.request_status = 'accepted' AND ps.peer_type = 'outbound'
		ORDER BY pso.priority ASC, pso.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*PeerSharingOutbound
	for rows.Next() {
		var pso PeerSharingOutbound
		var ps PeerServer
		var createdAt sql.NullString
		var storageRemaining sql.NullInt64

		err := rows.Scan(&pso.ID, &pso.PeerServerID, &pso.Enabled, &pso.Priority, &createdAt,
			&ps.ID, &ps.Name, &ps.URL, &ps.PeerType, &ps.RequestStatus,
			&ps.QuotaAllocated, &ps.QuotaUsed, &storageRemaining, &ps.IsActive)
		if err != nil {
			return nil, err
		}

		pso.CreatedAt = createdAt.String
		ps.QuotaAllocated = storageRemaining.Int64 // Map to existing field for convenience
		pso.PeerServer = &ps
		result = append(result, &pso)
	}

	return result, rows.Err()
}

// ListPeerSharingInbound returns all inbound peer sharing configurations
func ListPeerSharingInbound(db *sql.DB) ([]*PeerSharingInbound, error) {
	rows, err := db.Query(`
		SELECT psi.id, psi.peer_server_id, psi.enabled, psi.created_at,
		       ps.id, ps.name, ps.url, ps.peer_type, ps.request_status, ps.quota_allocated,
		       ps.quota_used, ps.storage_remaining, ps.is_active
		FROM peer_sharing_inbound psi
		JOIN peer_servers ps ON ps.id = psi.peer_server_id
		WHERE ps.request_status = 'accepted' AND ps.peer_type = 'inbound'
		ORDER BY psi.created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*PeerSharingInbound
	for rows.Next() {
		var psi PeerSharingInbound
		var ps PeerServer
		var createdAt sql.NullString
		var storageRemaining sql.NullInt64

		err := rows.Scan(&psi.ID, &psi.PeerServerID, &psi.Enabled, &createdAt,
			&ps.ID, &ps.Name, &ps.URL, &ps.PeerType, &ps.RequestStatus,
			&ps.QuotaAllocated, &ps.QuotaUsed, &storageRemaining, &ps.IsActive)
		if err != nil {
			return nil, err
		}

		psi.CreatedAt = createdAt.String
		psi.PeerServer = &ps
		result = append(result, &psi)
	}

	return result, rows.Err()
}

// AddPeerSharingOutbound adds a peer server to outbound sharing list
func AddPeerSharingOutbound(db *sql.DB, peerServerID string, priority int) (*PeerSharingOutbound, error) {
	// Verify peer server exists and is accepted outbound
	peer, err := GetPeerServerByID(db, peerServerID)
	if err != nil {
		return nil, errors.New("peer server not found")
	}
	if peer.PeerType != "outbound" || peer.RequestStatus != "accepted" {
		return nil, errors.New("peer server must be an accepted outbound connection")
	}

	id := uuid.New().String()
	_, err = db.Exec(`
		INSERT INTO peer_sharing_outbound (id, peer_server_id, enabled, priority)
		VALUES (?, ?, 1, ?)
		ON CONFLICT(peer_server_id) DO UPDATE SET enabled = 1, priority = ?
	`, id, peerServerID, priority, priority)
	if err != nil {
		return nil, err
	}

	return &PeerSharingOutbound{
		ID:           id,
		PeerServerID: peerServerID,
		Enabled:      true,
		Priority:     priority,
		PeerServer:   peer,
	}, nil
}

// AddPeerSharingInbound adds a peer server to inbound sharing list
func AddPeerSharingInbound(db *sql.DB, peerServerID string) (*PeerSharingInbound, error) {
	// Verify peer server exists and is accepted inbound
	peer, err := GetPeerServerByID(db, peerServerID)
	if err != nil {
		return nil, errors.New("peer server not found")
	}
	if peer.PeerType != "inbound" || peer.RequestStatus != "accepted" {
		return nil, errors.New("peer server must be an accepted inbound connection")
	}

	id := uuid.New().String()
	_, err = db.Exec(`
		INSERT INTO peer_sharing_inbound (id, peer_server_id, enabled)
		VALUES (?, ?, 1)
		ON CONFLICT(peer_server_id) DO UPDATE SET enabled = 1
	`, id, peerServerID)
	if err != nil {
		return nil, err
	}

	return &PeerSharingInbound{
		ID:           id,
		PeerServerID: peerServerID,
		Enabled:      true,
		PeerServer:   peer,
	}, nil
}

// UpdatePeerSharingOutboundEnabled enables/disables an outbound sharing configuration
func UpdatePeerSharingOutboundEnabled(db *sql.DB, id string, enabled bool) error {
	res, err := db.Exec(`UPDATE peer_sharing_outbound SET enabled = ? WHERE id = ?`, enabled, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdatePeerSharingInboundEnabled enables/disables an inbound sharing configuration
func UpdatePeerSharingInboundEnabled(db *sql.DB, id string, enabled bool) error {
	res, err := db.Exec(`UPDATE peer_sharing_inbound SET enabled = ? WHERE id = ?`, enabled, id)
	if err != nil {
		return err
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// RemovePeerSharingOutbound removes a peer server from outbound sharing list
func RemovePeerSharingOutbound(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM peer_sharing_outbound WHERE id = ?`, id)
	return err
}

// RemovePeerSharingInbound removes a peer server from inbound sharing list
func RemovePeerSharingInbound(db *sql.DB, id string) error {
	_, err := db.Exec(`DELETE FROM peer_sharing_inbound WHERE id = ?`, id)
	return err
}

// GetEnabledOutboundPeersForSharing returns active outbound peers configured for file sharing
func GetEnabledOutboundPeersForSharing(db *sql.DB) ([]*PeerServer, error) {
	rows, err := db.Query(`
		SELECT ps.id, ps.name, ps.url, ps.peer_type, ps.request_status, ps.quota_allocated,
		       ps.quota_used, ps.storage_remaining, ps.is_active, ps.client_cert, ps.client_key, ps.server_cert
		FROM peer_servers ps
		JOIN peer_sharing_outbound pso ON pso.peer_server_id = ps.id
		WHERE pso.enabled = 1 AND ps.request_status = 'accepted' AND ps.is_active = 1
		ORDER BY pso.priority ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []*PeerServer
	for rows.Next() {
		var ps PeerServer
		var storageRemaining sql.NullInt64
		var clientCert, clientKey, serverCert sql.NullString

		err := rows.Scan(&ps.ID, &ps.Name, &ps.URL, &ps.PeerType, &ps.RequestStatus,
			&ps.QuotaAllocated, &ps.QuotaUsed, &storageRemaining, &ps.IsActive,
			&clientCert, &clientKey, &serverCert)
		if err != nil {
			return nil, err
		}

		ps.ClientCert = clientCert.String
		ps.ClientKey = clientKey.String
		ps.ServerCert = serverCert.String
		result = append(result, &ps)
	}

	return result, rows.Err()
}

// UpdatePeerServerStorageRemaining updates the storage_remaining field for a peer server
func UpdatePeerServerStorageRemaining(db *sql.DB, peerID string, storageRemaining int64) error {
	_, err := db.Exec(`
		UPDATE peer_servers 
		SET storage_remaining = ?, last_active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, storageRemaining, time.Now().Format(time.RFC3339), peerID)
	return err
}

// GetFilesFromPeerServer returns files received from a specific peer server
func GetFilesFromPeerServer(db *sql.DB, peerServerID string) ([]string, error) {
	rows, err := db.Query(`SELECT id FROM files WHERE source_server_id = ?`, peerServerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fileIDs []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		fileIDs = append(fileIDs, id)
	}
	return fileIDs, rows.Err()
}

// GetShardsOnPeerServer returns shards stored on a specific peer server
func GetShardsOnPeerServer(db *sql.DB, peerServerID string) ([]*FileShard, error) {
	rows, err := db.Query(`
		SELECT id, file_id, shard_index, shard_type, node_id, shard_size, checksum, 
		       storage_path, status, created_at, updated_at
		FROM file_shards 
		WHERE peer_server_id = ?
	`, peerServerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shards []*FileShard
	for rows.Next() {
		shard, err := scanFileShard(rows)
		if err != nil {
			return nil, err
		}
		shards = append(shards, shard)
	}
	return shards, rows.Err()
}

// CountFilesFromPeerServer counts files received from a peer server
func CountFilesFromPeerServer(db *sql.DB, peerServerID string) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM files WHERE source_server_id = ?`, peerServerID).Scan(&count)
	return count, err
}

// CountShardsOnPeerServer counts shards stored on a peer server
func CountShardsOnPeerServer(db *sql.DB, peerServerID string) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM file_shards WHERE peer_server_id = ?`, peerServerID).Scan(&count)
	return count, err
}
