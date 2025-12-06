package enrollment

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// MTLSClient handles secure mTLS communication with the controller
type MTLSClient struct {
	httpClient    *http.Client
	controllerURL string
	nodeID        string
	certDir       string
}

// NewMTLSClient creates a new mTLS client
func NewMTLSClient(controllerURL, certDir string) (*MTLSClient, error) {
	// Load node ID
	nodeIDPath := filepath.Join(certDir, NodeIDFile)
	nodeIDBytes, err := os.ReadFile(nodeIDPath)
	if err != nil {
		return nil, fmt.Errorf("read node ID: %w", err)
	}
	nodeID := string(nodeIDBytes)

	// Load client certificate and key
	certPath := filepath.Join(certDir, CertFile)
	keyPath := filepath.Join(certDir, KeyFile)
	
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	// Load CA certificate
	caPath := filepath.Join(certDir, CAFile)
	caCert, err := os.ReadFile(caPath)
	if err != nil {
		return nil, fmt.Errorf("read CA certificate: %w", err)
	}

	// Create cert pool with CA
	caCertPool := x509.NewCertPool()
	if !caCertPool.AppendCertsFromPEM(caCert) {
		return nil, fmt.Errorf("failed to append CA certificate")
	}

	// Create TLS config
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
		MinVersion:   tls.VersionTLS13,
		CipherSuites: []uint16{
			tls.TLS_AES_128_GCM_SHA256,
			tls.TLS_AES_256_GCM_SHA384,
			tls.TLS_CHACHA20_POLY1305_SHA256,
		},
	}

	// Create HTTP client with mTLS
	httpClient := &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}

	return &MTLSClient{
		httpClient:    httpClient,
		controllerURL: controllerURL,
		nodeID:        nodeID,
		certDir:       certDir,
	}, nil
}

// Get performs an authenticated GET request
func (c *MTLSClient) Get(path string) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", c.controllerURL, path)
	return c.httpClient.Get(url)
}

// Post performs an authenticated POST request
func (c *MTLSClient) Post(path, contentType string, body []byte) (*http.Response, error) {
	url := fmt.Sprintf("%s%s", c.controllerURL, path)
	req, err := http.NewRequest("POST", url, nil)
	if err != nil {
		return nil, err
	}
	
	req.Header.Set("Content-Type", contentType)
	
	return c.httpClient.Do(req)
}

// GetNodeID returns the node ID
func (c *MTLSClient) GetNodeID() string {
	return c.nodeID
}

// CheckCertificateExpiry checks if the certificate is about to expire
func (c *MTLSClient) CheckCertificateExpiry() (time.Time, bool, error) {
	certPath := filepath.Join(c.certDir, CertFile)
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("read certificate: %w", err)
	}

	// Parse certificate
	cert, err := x509.ParseCertificate(certPEM)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("parse certificate: %w", err)
	}

	expiresAt := cert.NotAfter
	daysUntilExpiry := time.Until(expiresAt).Hours() / 24

	// Consider renewal if less than 30 days remaining
	shouldRenew := daysUntilExpiry < 30

	return expiresAt, shouldRenew, nil
}

// RenewCertificate requests a new certificate from the controller
func (c *MTLSClient) RenewCertificate() error {
	fmt.Println("\n🔄 Renewing certificate...")

	// Generate new key and CSR
	enrollClient := NewEnrollmentClient(c.controllerURL)
	privateKey, csrPEM, err := enrollClient.GenerateKeyAndCSR()
	if err != nil {
		return fmt.Errorf("generate key and CSR: %w", err)
	}

	// TODO: Send renewal request to controller using mTLS
	// This would be implemented similar to the enrollment request
	// but would use the mTLS client for authentication

	fmt.Println("✓ Certificate renewal requested")
	
	// For now, just store the new key (certificate would come from controller response)
	_ = privateKey
	_ = csrPEM

	return fmt.Errorf("certificate renewal not fully implemented yet")
}
