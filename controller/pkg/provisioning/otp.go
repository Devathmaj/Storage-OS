package provisioning

import (
	"crypto/rand"
	"database/sql"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
)

const (
	// OTPLength is the number of digits in an OTP
	OTPLength = 6
	
	// OTPValidity is how long an OTP remains valid
	OTPValidity = 5 * time.Minute
)

// OTPService handles OTP generation and validation
type OTPService struct {
	db *sql.DB
}

// NewOTPService creates a new OTP service
func NewOTPService(db *sql.DB) *OTPService {
	return &OTPService{db: db}
}

// GenerateOTP creates a new 6-digit numeric OTP
func (s *OTPService) GenerateOTP(createdByUserID string) (string, error) {
	// Generate a random 6-digit number
	max := big.NewInt(1000000) // 10^6
	n, err := rand.Int(rand.Reader, max)
	if err != nil {
		return "", fmt.Errorf("generate random number: %w", err)
	}
	
	otp := fmt.Sprintf("%06d", n.Int64())
	expiresAt := time.Now().Add(OTPValidity)

	// Store in database
	_, err = s.db.Exec(`
		INSERT INTO enrollment_otps (otp, expires_at, created_by_user_id, status)
		VALUES (?, ?, ?, 'active')
	`, otp, expiresAt.Format(time.RFC3339), createdByUserID)
	
	if err != nil {
		return "", fmt.Errorf("store OTP: %w", err)
	}

	return otp, nil
}

// ValidateOTP checks if an OTP is valid and unused
func (s *OTPService) ValidateOTP(otp string) (bool, string, error) {
	var status string
	var expiresAt string
	var usedAt sql.NullString

	err := s.db.QueryRow(`
		SELECT status, expires_at, used_at
		FROM enrollment_otps
		WHERE otp = ?
	`, otp).Scan(&status, &expiresAt, &usedAt)

	if err == sql.ErrNoRows {
		return false, "invalid OTP", nil
	}
	if err != nil {
		return false, "", fmt.Errorf("query OTP: %w", err)
	}

	// Check if already used
	if usedAt.Valid {
		return false, "OTP already used", nil
	}

	// Check if expired
	expiry, err := time.Parse(time.RFC3339, expiresAt)
	if err != nil {
		return false, "", fmt.Errorf("parse expiry time: %w", err)
	}

	if time.Now().After(expiry) {
		// Mark as expired
		_, _ = s.db.Exec(`UPDATE enrollment_otps SET status = 'expired' WHERE otp = ?`, otp)
		return false, "OTP expired", nil
	}

	// Check status
	if status != "active" {
		return false, fmt.Sprintf("OTP is %s", status), nil
	}

	return true, "", nil
}

// MarkOTPUsed marks an OTP as used by a node
func (s *OTPService) MarkOTPUsed(otp, nodeID string) error {
	result, err := s.db.Exec(`
		UPDATE enrollment_otps
		SET status = 'used', used_at = ?, used_by_node_id = ?
		WHERE otp = ? AND status = 'active'
	`, time.Now().Format(time.RFC3339), nodeID, otp)

	if err != nil {
		return fmt.Errorf("mark OTP used: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("OTP not found or already used")
	}

	return nil
}

// CleanupExpiredOTPs removes expired OTPs from the database
func (s *OTPService) CleanupExpiredOTPs() error {
	_, err := s.db.Exec(`
		UPDATE enrollment_otps
		SET status = 'expired'
		WHERE status = 'active' AND datetime(expires_at) < datetime('now')
	`)
	
	if err != nil {
		return fmt.Errorf("cleanup expired OTPs: %w", err)
	}

	return nil
}

// ListActiveOTPs returns all active OTPs (for admin dashboard)
func (s *OTPService) ListActiveOTPs() ([]OTPInfo, error) {
	rows, err := s.db.Query(`
		SELECT otp, created_at, expires_at, created_by_user_id
		FROM enrollment_otps
		WHERE status = 'active' AND datetime(expires_at) > datetime('now')
		ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query active OTPs: %w", err)
	}
	defer rows.Close()

	var otps []OTPInfo
	for rows.Next() {
		var info OTPInfo
		if err := rows.Scan(&info.OTP, &info.CreatedAt, &info.ExpiresAt, &info.CreatedByUserID); err != nil {
			return nil, fmt.Errorf("scan OTP: %w", err)
		}
		otps = append(otps, info)
	}

	return otps, nil
}

// OTPInfo contains information about an OTP
type OTPInfo struct {
	OTP             string    `json:"otp"`
	CreatedAt       string    `json:"created_at"`
	ExpiresAt       string    `json:"expires_at"`
	CreatedByUserID string    `json:"created_by_user_id"`
}

// EnrollmentRequest represents a pending enrollment request
type EnrollmentRequest struct {
	ID             string          `json:"id"`
	OTP            string          `json:"otp"`
	CSR            string          `json:"csr"`
	SystemInfo     string          `json:"system_info"`
	RequestedAt    string          `json:"requested_at"`
	Status         string          `json:"status"`
}

// CreateEnrollmentRequest stores a new enrollment request
func (s *OTPService) CreateEnrollmentRequest(otp, csr, systemInfo string) (string, error) {
	id := uuid.New().String()

	_, err := s.db.Exec(`
		INSERT INTO enrollment_requests (id, otp, csr, system_info, status)
		VALUES (?, ?, ?, ?, 'pending')
	`, id, otp, csr, systemInfo)

	if err != nil {
		return "", fmt.Errorf("create enrollment request: %w", err)
	}

	return id, nil
}

// GetEnrollmentRequest retrieves an enrollment request by ID
func (s *OTPService) GetEnrollmentRequest(id string) (*EnrollmentRequest, error) {
	var req EnrollmentRequest
	
	err := s.db.QueryRow(`
		SELECT id, otp, csr, system_info, requested_at, status
		FROM enrollment_requests
		WHERE id = ?
	`, id).Scan(&req.ID, &req.OTP, &req.CSR, &req.SystemInfo, &req.RequestedAt, &req.Status)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("enrollment request not found")
	}
	if err != nil {
		return nil, fmt.Errorf("query enrollment request: %w", err)
	}

	return &req, nil
}

// ListPendingEnrollmentRequests returns all pending enrollment requests
func (s *OTPService) ListPendingEnrollmentRequests() ([]EnrollmentRequest, error) {
	rows, err := s.db.Query(`
		SELECT id, otp, csr, system_info, requested_at, status
		FROM enrollment_requests
		WHERE status = 'pending'
		ORDER BY requested_at DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("query pending requests: %w", err)
	}
	defer rows.Close()

	var requests []EnrollmentRequest
	for rows.Next() {
		var req EnrollmentRequest
		if err := rows.Scan(&req.ID, &req.OTP, &req.CSR, &req.SystemInfo, &req.RequestedAt, &req.Status); err != nil {
			return nil, fmt.Errorf("scan request: %w", err)
		}
		requests = append(requests, req)
	}

	return requests, nil
}

// ApproveEnrollmentRequest approves a pending enrollment request
func (s *OTPService) ApproveEnrollmentRequest(id, reviewedByUserID string) error {
	result, err := s.db.Exec(`
		UPDATE enrollment_requests
		SET status = 'approved', reviewed_at = ?, reviewed_by_user_id = ?
		WHERE id = ? AND status = 'pending'
	`, time.Now().Format(time.RFC3339), reviewedByUserID, id)

	if err != nil {
		return fmt.Errorf("approve request: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("request not found or already processed")
	}

	return nil
}

// RejectEnrollmentRequest rejects a pending enrollment request
func (s *OTPService) RejectEnrollmentRequest(id, reviewedByUserID, reason string) error {
	result, err := s.db.Exec(`
		UPDATE enrollment_requests
		SET status = 'rejected', reviewed_at = ?, reviewed_by_user_id = ?, rejection_reason = ?
		WHERE id = ? AND status = 'pending'
	`, time.Now().Format(time.RFC3339), reviewedByUserID, reason, id)

	if err != nil {
		return fmt.Errorf("reject request: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("check rows affected: %w", err)
	}

	if rows == 0 {
		return fmt.Errorf("request not found or already processed")
	}

	return nil
}
