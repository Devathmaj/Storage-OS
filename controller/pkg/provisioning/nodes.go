package provisioning

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// NodeService handles OS node management
type NodeService struct {
	db *sql.DB
}

// NewNodeService creates a new node service
func NewNodeService(db *sql.DB) *NodeService {
	return &NodeService{db: db}
}

// OSNode represents an enrolled OS node
type OSNode struct {
	ID                    string    `json:"id"`
	Certificate           string    `json:"certificate"`
	CertificateFingerprint string   `json:"certificate_fingerprint"`
	CertificateCN         string    `json:"certificate_cn"`
	EnrollmentRequestID   string    `json:"enrollment_request_id"`
	SystemInfo            string    `json:"system_info"`
	EnrolledAt            string    `json:"enrolled_at"`
	CertificateExpiresAt  string    `json:"certificate_expires_at"`
	LastSeen              *string   `json:"last_seen"`
	Status                string    `json:"status"`
	RevokedAt             *string   `json:"revoked_at"`
	RevokedByUserID       *string   `json:"revoked_by_user_id"`
	RevocationReason      *string   `json:"revocation_reason"`
}

// RegisterNode registers a newly enrolled node
func (s *NodeService) RegisterNode(enrollmentRequestID, certificate, fingerprint, cn, systemInfo string, expiresAt time.Time) (string, error) {
	// Generate node ID
	nodeID := fmt.Sprintf("node-%d", time.Now().Unix())

	_, err := s.db.Exec(`
		INSERT INTO os_nodes (
			id, certificate, certificate_fingerprint, certificate_cn,
			enrollment_request_id, system_info, certificate_expires_at, status
		) VALUES (?, ?, ?, ?, ?, ?, ?, 'active')
	`, nodeID, certificate, fingerprint, cn, enrollmentRequestID, systemInfo, expiresAt.Format(time.RFC3339))

	if err != nil {
		return "", fmt.Errorf("register node: %w", err)
	}

	return nodeID, nil
}

// GetNodeByID retrieves a node by its ID
func (s *NodeService) GetNodeByID(nodeID string) (*OSNode, error) {
	var node OSNode
	var lastSeen, revokedAt sql.NullString
	var revokedByUserID, revocationReason sql.NullString

	err := s.db.QueryRow(`
		SELECT id, certificate, certificate_fingerprint, certificate_cn,
			   enrollment_request_id, system_info, enrolled_at,
			   certificate_expires_at, last_seen, status,
			   revoked_at, revoked_by_user_id, revocation_reason
		FROM os_nodes
		WHERE id = ?
	`, nodeID).Scan(
		&node.ID, &node.Certificate, &node.CertificateFingerprint, &node.CertificateCN,
		&node.EnrollmentRequestID, &node.SystemInfo, &node.EnrolledAt,
		&node.CertificateExpiresAt, &lastSeen, &node.Status,
		&revokedAt, &revokedByUserID, &revocationReason,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("node not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query node: %w", err)
	}

	if lastSeen.Valid {
		node.LastSeen = &lastSeen.String
	}
	if revokedAt.Valid {
		node.RevokedAt = &revokedAt.String
	}
	if revokedByUserID.Valid {
		node.RevokedByUserID = &revokedByUserID.String
	}
	if revocationReason.Valid {
		node.RevocationReason = &revocationReason.String
	}

	return &node, nil
}

// GetNodeByFingerprint retrieves a node by certificate fingerprint
func (s *NodeService) GetNodeByFingerprint(fingerprint string) (*OSNode, error) {
	var node OSNode
	var lastSeen, revokedAt sql.NullString
	var revokedByUserID, revocationReason sql.NullString

	err := s.db.QueryRow(`
		SELECT id, certificate, certificate_fingerprint, certificate_cn,
			   enrollment_request_id, system_info, enrolled_at,
			   certificate_expires_at, last_seen, status,
			   revoked_at, revoked_by_user_id, revocation_reason
		FROM os_nodes
		WHERE certificate_fingerprint = ?
	`, fingerprint).Scan(
		&node.ID, &node.Certificate, &node.CertificateFingerprint, &node.CertificateCN,
		&node.EnrollmentRequestID, &node.SystemInfo, &node.EnrolledAt,
		&node.CertificateExpiresAt, &lastSeen, &node.Status,
		&revokedAt, &revokedByUserID, &revocationReason,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("node not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query node: %w", err)
	}

	if lastSeen.Valid {
		node.LastSeen = &lastSeen.String
	}
	if revokedAt.Valid {
		node.RevokedAt = &revokedAt.String
	}
	if revokedByUserID.Valid {
		node.RevokedByUserID = &revokedByUserID.String
	}
	if revocationReason.Valid {
		node.RevocationReason = &revocationReason.String
	}

	return &node, nil
}

// GetNodeByCN retrieves a node by certificate Common Name
func (s *NodeService) GetNodeByCN(cn string) (*OSNode, error) {
	var node OSNode
	var lastSeen, revokedAt sql.NullString
	var revokedByUserID, revocationReason sql.NullString

	err := s.db.QueryRow(`
		SELECT id, certificate, certificate_fingerprint, certificate_cn,
			   enrollment_request_id, system_info, enrolled_at,
			   certificate_expires_at, last_seen, status,
			   revoked_at, revoked_by_user_id, revocation_reason
		FROM os_nodes
		WHERE certificate_cn = ? AND status = 'active'
	`, cn).Scan(
		&node.ID, &node.Certificate, &node.CertificateFingerprint, &node.CertificateCN,
		&node.EnrollmentRequestID, &node.SystemInfo, &node.EnrolledAt,
		&node.CertificateExpiresAt, &lastSeen, &node.Status,
		&revokedAt, &revokedByUserID, &revocationReason,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("node not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query node: %w", err)
	}

	if lastSeen.Valid {
		node.LastSeen = &lastSeen.String
	}
	if revokedAt.Valid {
		node.RevokedAt = &revokedAt.String
	}
	if revokedByUserID.Valid {
		node.RevokedByUserID = &revokedByUserID.String
	}
	if revocationReason.Valid {
		node.RevocationReason = &revocationReason.String
	}

	return &node, nil
}

// ListNodes returns all registered nodes
func (s *NodeService) ListNodes() ([]OSNode, error) {
	rows, err := s.db.Query(`
		SELECT id, certificate, certificate_fingerprint, certificate_cn,
			   enrollment_request_id, system_info, enrolled_at,
			   certificate_expires_at, last_seen, status,
			   revoked_at, revoked_by_user_id, revocation_reason
		FROM os_nodes
		ORDER BY enrolled_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query nodes: %w", err)
	}
	defer rows.Close()

	var nodes []OSNode
	for rows.Next() {
		var node OSNode
		var lastSeen, revokedAt sql.NullString
		var revokedByUserID, revocationReason sql.NullString

		if err := rows.Scan(
			&node.ID, &node.Certificate, &node.CertificateFingerprint, &node.CertificateCN,
			&node.EnrollmentRequestID, &node.SystemInfo, &node.EnrolledAt,
			&node.CertificateExpiresAt, &lastSeen, &node.Status,
			&revokedAt, &revokedByUserID, &revocationReason,
		); err != nil {
			return nil, fmt.Errorf("scan node: %w", err)
		}

		if lastSeen.Valid {
			node.LastSeen = &lastSeen.String
		}
		if revokedAt.Valid {
			node.RevokedAt = &revokedAt.String
		}
		if revokedByUserID.Valid {
			node.RevokedByUserID = &revokedByUserID.String
		}
		if revocationReason.Valid {
			node.RevocationReason = &revocationReason.String
		}

		nodes = append(nodes, node)
	}

	return nodes, nil
}

// UpdateLastSeen updates the last seen timestamp for a node
func (s *NodeService) UpdateLastSeen(nodeID string) error {
	_, err := s.db.Exec(`
		UPDATE os_nodes
		SET last_seen = ?
		WHERE id = ?
	`, time.Now().Format(time.RFC3339), nodeID)

	if err != nil {
		return fmt.Errorf("update last seen: %w", err)
	}

	return nil
}

// RevokeNode revokes a node's certificate
func (s *NodeService) RevokeNode(nodeID, revokedByUserID, reason string) error {
	// Get node info first
	node, err := s.GetNodeByID(nodeID)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update node status
	_, err = tx.Exec(`
		UPDATE os_nodes
		SET status = 'revoked', revoked_at = ?, revoked_by_user_id = ?, revocation_reason = ?
		WHERE id = ?
	`, time.Now().Format(time.RFC3339), revokedByUserID, reason, nodeID)

	if err != nil {
		return fmt.Errorf("update node status: %w", err)
	}

	// Add to revocation list
	revocationID := uuid.New().String()
	_, err = tx.Exec(`
		INSERT INTO certificate_revocations (id, node_id, certificate_fingerprint, certificate_cn, revoked_by_user_id, reason)
		VALUES (?, ?, ?, ?, ?, ?)
	`, revocationID, nodeID, node.CertificateFingerprint, node.CertificateCN, revokedByUserID, reason)

	if err != nil {
		return fmt.Errorf("add to revocation list: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// IsRevoked checks if a certificate is revoked
func (s *NodeService) IsRevoked(fingerprint string) (bool, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM certificate_revocations
		WHERE certificate_fingerprint = ?
	`, fingerprint).Scan(&count)

	if err != nil {
		return false, fmt.Errorf("check revocation: %w", err)
	}

	return count > 0, nil
}

// RenewCertificate renews a node's certificate
func (s *NodeService) RenewCertificate(nodeID, newCertificate, newFingerprint string, expiresAt time.Time) error {
	// Get old certificate info
	node, err := s.GetNodeByID(nodeID)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Update node certificate
	_, err = tx.Exec(`
		UPDATE os_nodes
		SET certificate = ?, certificate_fingerprint = ?, certificate_expires_at = ?
		WHERE id = ?
	`, newCertificate, newFingerprint, expiresAt.Format(time.RFC3339), nodeID)

	if err != nil {
		return fmt.Errorf("update certificate: %w", err)
	}

	// Record renewal
	renewalID := uuid.New().String()
	_, err = tx.Exec(`
		INSERT INTO certificate_renewals (id, node_id, old_certificate_fingerprint, new_certificate_fingerprint, new_expires_at)
		VALUES (?, ?, ?, ?, ?)
	`, renewalID, nodeID, node.CertificateFingerprint, newFingerprint, expiresAt.Format(time.RFC3339))

	if err != nil {
		return fmt.Errorf("record renewal: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit transaction: %w", err)
	}

	return nil
}

// MarkNodeRemoved marks a node as removed (initiated by the OS node itself)
func (s *NodeService) MarkNodeRemoved(nodeID, reason string) error {
	_, err := s.db.Exec(`
		UPDATE os_nodes
		SET status = 'removed', revocation_reason = ?, revoked_at = ?
		WHERE id = ?
	`, reason, time.Now().Format(time.RFC3339), nodeID)

	if err != nil {
		return fmt.Errorf("mark node removed: %w", err)
	}

	return nil
}
