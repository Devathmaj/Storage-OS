package enrollment

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// Certificate and key storage paths (default)
	CertDir    = "/etc/os"
	KeyFile    = "os.key"
	CertFile   = "os.crt"
	CAFile     = "ca.crt"
	NodeIDFile = "node_id"
)

// SystemInfo contains information about the OS node
type SystemInfo struct {
	Hostname   string `json:"hostname"`
	CPU        string `json:"cpu"`
	OSVersion  string `json:"os_version"`
	NetworkID  string `json:"network_id"`
}

// EnrollmentClient handles the enrollment process
type EnrollmentClient struct {
	controllerURL string
	certDir       string
}

// NewEnrollmentClient creates a new enrollment client with default cert directory
func NewEnrollmentClient(controllerURL string) *EnrollmentClient {
	return &EnrollmentClient{
		controllerURL: controllerURL,
		certDir:       CertDir,
	}
}

// NewEnrollmentClientWithDir creates a new enrollment client with custom cert directory
func NewEnrollmentClientWithDir(controllerURL, certDir string) *EnrollmentClient {
	return &EnrollmentClient{
		controllerURL: controllerURL,
		certDir:       certDir,
	}
}

// IsEnrolled checks if the node is already enrolled
func (c *EnrollmentClient) IsEnrolled() bool {
	certPath := filepath.Join(c.certDir, CertFile)
	keyPath := filepath.Join(c.certDir, KeyFile)
	caPath := filepath.Join(c.certDir, CAFile)
	nodeIDPath := filepath.Join(c.certDir, NodeIDFile)

	// Check if all required files exist
	for _, path := range []string{certPath, keyPath, caPath, nodeIDPath} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return false
		}
	}

	return true
}

// PromptForOTP displays enrollment prompt and gets OTP from user
func (c *EnrollmentClient) PromptForOTP() (string, error) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("   OS NODE ENROLLMENT")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("\nThis node is not registered.")
	fmt.Println("An administrator must generate an OTP from the controller dashboard.")
	fmt.Println("\nEnter the 6-digit OTP to request enrollment:")
	fmt.Print("> ")

	reader := bufio.NewReader(os.Stdin)
	otp, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read OTP: %w", err)
	}

	otp = strings.TrimSpace(otp)

	// Validate OTP format (6 digits)
	if len(otp) != 6 {
		return "", fmt.Errorf("OTP must be 6 digits")
	}

	for _, c := range otp {
		if c < '0' || c > '9' {
			return "", fmt.Errorf("OTP must contain only digits")
		}
	}

	return otp, nil
}

// GenerateKeyAndCSR generates a private key and CSR
func (c *EnrollmentClient) GenerateKeyAndCSR() (privateKey *ecdsa.PrivateKey, csrPEM []byte, err error) {
	// Generate ECDSA private key (P-256)
	privateKey, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate private key: %w", err)
	}

	// Get system hostname
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "unknown"
	}

	// Create CSR template
	template := x509.CertificateRequest{
		Subject: pkix.Name{
			Organization: []string{"StorageOS"},
			CommonName:   hostname,
		},
		SignatureAlgorithm: x509.ECDSAWithSHA256,
	}

	// Create CSR
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &template, privateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("create CSR: %w", err)
	}

	csrPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	return privateKey, csrPEM, nil
}

// GetSystemInfo collects system information
func (c *EnrollmentClient) GetSystemInfo() SystemInfo {
	hostname, _ := os.Hostname()

	// Try to read CPU info
	cpuInfo := "unknown"
	if data, err := os.ReadFile("/proc/cpuinfo"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "model name") {
				parts := strings.Split(line, ":")
				if len(parts) > 1 {
					cpuInfo = strings.TrimSpace(parts[1])
					break
				}
			}
		}
	}

	// Try to read OS version
	osVersion := "unknown"
	if data, err := os.ReadFile("/etc/os-release"); err == nil {
		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "PRETTY_NAME=") {
				osVersion = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
				break
			}
		}
	}

	// Network ID (could be MAC address or similar)
	networkID := "unknown"

	return SystemInfo{
		Hostname:  hostname,
		CPU:       cpuInfo,
		OSVersion: osVersion,
		NetworkID: networkID,
	}
}

// SendEnrollmentRequest sends enrollment request to controller
func (c *EnrollmentClient) SendEnrollmentRequest(otp string, csrPEM []byte, systemInfo SystemInfo, nodeURL string) (string, error) {
	reqBody := map[string]interface{}{
		"otp":         otp,
		"csr":         base64.StdEncoding.EncodeToString(csrPEM),
		"node_url":    nodeURL,
		"system_info": systemInfo,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/v1/provisioning/register-request", c.controllerURL)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		var errResp map[string]string
		if err := json.Unmarshal(body, &errResp); err == nil {
			return "", fmt.Errorf("enrollment rejected: %s", errResp["error"])
		}
		return "", fmt.Errorf("enrollment rejected")
	}

	if resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("unexpected status: %d, body: %s", resp.StatusCode, string(body))
	}

	var response struct {
		RequestID string `json:"request_id"`
		Status    string `json:"status"`
		Message   string `json:"message"`
	}

	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("parse response: %w", err)
	}

	fmt.Printf("\n✓ Enrollment request submitted successfully!\n")
	fmt.Printf("  Request ID: %s\n", response.RequestID)
	fmt.Printf("  Status: %s\n", response.Status)
	fmt.Printf("  %s\n\n", response.Message)

	return response.RequestID, nil
}

// ApprovalResponse represents the approval response with certificate
type ApprovalResponse struct {
	NodeID        string `json:"node_id"`
	Certificate   string `json:"certificate"`
	CACertificate string `json:"ca_certificate"`
	ExpiresAt     string `json:"expires_at"`
}

// PollForApproval polls the controller for approval status and returns certificate when approved
func (c *EnrollmentClient) PollForApproval(requestID string, timeout time.Duration) (*ApprovalResponse, error) {
	url := fmt.Sprintf("%s/v1/provisioning/register-status/%s", c.controllerURL, requestID)
	deadline := time.Now().Add(timeout)

	fmt.Println("Waiting for administrator approval...")
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-time.After(time.Until(deadline)):
			return nil, fmt.Errorf("timeout waiting for approval")
		case <-ticker.C:
			resp, err := http.Get(url)
			if err != nil {
				fmt.Printf("⚠ Error checking status: %v\n", err)
				continue
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				continue
			}

			var status struct {
				Status        string `json:"status"`
				Message       string `json:"message"`
				NodeID        string `json:"node_id"`
				Certificate   string `json:"certificate"`
				CACertificate string `json:"ca_certificate"`
				ExpiresAt     string `json:"expires_at"`
			}

			if err := json.Unmarshal(body, &status); err != nil {
				continue
			}

			fmt.Printf("  Status: %s - %s\n", status.Status, status.Message)

			if status.Status == "approved" {
				fmt.Println("\n✓ Enrollment approved!")
				return &ApprovalResponse{
					NodeID:        status.NodeID,
					Certificate:   status.Certificate,
					CACertificate: status.CACertificate,
					ExpiresAt:     status.ExpiresAt,
				}, nil
			} else if status.Status == "rejected" {
				return nil, fmt.Errorf("enrollment rejected by administrator")
			}
		}
	}
}

// SaveCredentials saves the private key, certificate, and CA certificate
func (c *EnrollmentClient) SaveCredentials(privateKey *ecdsa.PrivateKey, nodeID, certificate, caCertificate string) error {
	// Create cert directory
	if err := os.MkdirAll(c.certDir, 0700); err != nil {
		return fmt.Errorf("create cert directory: %w", err)
	}

	// Save private key
	keyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return fmt.Errorf("marshal private key: %w", err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: keyBytes,
	})

	keyPath := filepath.Join(c.certDir, KeyFile)
	if err := os.WriteFile(keyPath, keyPEM, 0600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	// Decode and save certificate
	certBytes, err := base64.StdEncoding.DecodeString(certificate)
	if err != nil {
		return fmt.Errorf("decode certificate: %w", err)
	}

	certPath := filepath.Join(c.certDir, CertFile)
	if err := os.WriteFile(certPath, certBytes, 0644); err != nil {
		return fmt.Errorf("write certificate: %w", err)
	}

	// Decode and save CA certificate
	caBytes, err := base64.StdEncoding.DecodeString(caCertificate)
	if err != nil {
		return fmt.Errorf("decode CA certificate: %w", err)
	}

	caPath := filepath.Join(c.certDir, CAFile)
	if err := os.WriteFile(caPath, caBytes, 0644); err != nil {
		return fmt.Errorf("write CA certificate: %w", err)
	}

	// Save node ID
	nodeIDPath := filepath.Join(c.certDir, NodeIDFile)
	if err := os.WriteFile(nodeIDPath, []byte(nodeID), 0644); err != nil {
		return fmt.Errorf("write node ID: %w", err)
	}

	fmt.Println("\n✓ Credentials saved successfully:")
	fmt.Printf("  Private key:      %s\n", keyPath)
	fmt.Printf("  Certificate:      %s\n", certPath)
	fmt.Printf("  CA certificate:   %s\n", caPath)
	fmt.Printf("  Node ID:          %s\n", nodeIDPath)

	return nil
}

// RunEnrollment runs the complete enrollment process
func (c *EnrollmentClient) RunEnrollment() error {
	// Check if already enrolled
	if c.IsEnrolled() {
		fmt.Println("✓ Node is already enrolled")
		return nil
	}

	// Prompt for OTP
	otp, err := c.PromptForOTP()
	if err != nil {
		return fmt.Errorf("get OTP: %w", err)
	}

	return c.RunEnrollmentWithOTP(otp)
}

// RunEnrollmentWithOTP runs enrollment with a provided OTP (no prompting)
func (c *EnrollmentClient) RunEnrollmentWithOTP(otp string) error {
	_, err := c.RunEnrollmentWithOTPAndGetNodeID(otp)
	return err
}

// RunEnrollmentWithOTPAndGetNodeID runs enrollment and returns the assigned node ID
func (c *EnrollmentClient) RunEnrollmentWithOTPAndGetNodeID(otp string) (string, error) {
	fmt.Println("\nGenerating cryptographic key pair...")

	// Generate key and CSR
	privateKey, csrPEM, err := c.GenerateKeyAndCSR()
	if err != nil {
		return "", fmt.Errorf("generate key and CSR: %w", err)
	}

	fmt.Println("✓ Key pair generated")

	// Get system info
	systemInfo := c.GetSystemInfo()
	fmt.Printf("\nSystem Information:\n")
	fmt.Printf("  Hostname:    %s\n", systemInfo.Hostname)
	fmt.Printf("  CPU:         %s\n", systemInfo.CPU)
	fmt.Printf("  OS Version:  %s\n", systemInfo.OSVersion)
	fmt.Printf("  Network ID:  %s\n\n", systemInfo.NetworkID)

	// Read node URL from file (saved by enroll.go)
	nodeURL := ""
	if data, err := os.ReadFile(filepath.Join(c.certDir, "node_url")); err == nil {
		nodeURL = strings.TrimSpace(string(data))
	}
	if nodeURL == "" {
		return "", fmt.Errorf("node_url not found - run enrollment from os-storage-enroll command")
	}

	// Send enrollment request
	requestID, err := c.SendEnrollmentRequest(otp, csrPEM, systemInfo, nodeURL)
	if err != nil {
		return "", fmt.Errorf("send enrollment request: %w", err)
	}

	// Poll for approval (30 minute timeout)
	approvalResp, err := c.PollForApproval(requestID, 30*time.Minute)
	if err != nil {
		return "", fmt.Errorf("wait for approval: %w", err)
	}

	// Save credentials
	if err := c.SaveCredentials(privateKey, approvalResp.NodeID, approvalResp.Certificate, approvalResp.CACertificate); err != nil {
		return "", fmt.Errorf("save credentials: %w", err)
	}

	fmt.Println("\n✅ Enrollment successful!")
	fmt.Println("You can now run 'server-connect' to start the storage server.")

	return approvalResp.NodeID, nil
}
