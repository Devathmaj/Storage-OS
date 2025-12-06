package main

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"
)

const (
	CertDir               = "/etc/os"
	ControllersConfigFile = "/etc/os/controllers.json"
)

// ControllerEntry represents a single controller's enrollment data
type ControllerEntry struct {
	ID            string `json:"id"`
	ControllerURL string `json:"controller_url"`
	NodeURL       string `json:"node_url"`
	EnrolledAt    string `json:"enrolled_at"`
	CertFile      string `json:"cert_file"`
	KeyFile       string `json:"key_file"`
	CAFile        string `json:"ca_file"`
	Status        string `json:"status"`
}

// ControllersConfig holds all enrolled controller entries
type ControllersConfig struct {
	Controllers []ControllerEntry `json:"controllers"`
}

type RegisterRequest struct {
	OTP        string                 `json:"otp"`
	CSR        string                 `json:"csr"`
	NodeURL    string                 `json:"node_url"`
	SystemInfo map[string]interface{} `json:"system_info"`
}

type RegisterResponse struct {
	Status    string `json:"status"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type StatusResponse struct {
	Status        string `json:"status"`
	Message       string `json:"message"`
	NodeID        string `json:"node_id,omitempty"`
	Certificate   string `json:"certificate,omitempty"`
	CACertificate string `json:"ca_certificate,omitempty"`
	ExpiresAt     string `json:"expires_at,omitempty"`
}

func main() {
	fmt.Println("============================================================")
	fmt.Println("   OS Storage Node Enrollment")
	fmt.Println("============================================================")
	fmt.Println()

	// Load existing controllers config
	config, err := loadControllersConfig()
	if err != nil {
		fmt.Printf("Warning: could not load existing config: %v\n", err)
		config = &ControllersConfig{Controllers: []ControllerEntry{}}
	}

	// Show existing enrollments
	if len(config.Controllers) > 0 {
		fmt.Printf("Currently enrolled with %d controller(s):\n", len(config.Controllers))
		for i, c := range config.Controllers {
			fmt.Printf("  %d. %s (Node ID: %s, Status: %s)\n", i+1, c.ControllerURL, c.ID, c.Status)
		}
		fmt.Println()
	}

	// Prompt for controller URL
	fmt.Print("Enter Controller URL (e.g., https://controller.example.com:8081): ")
	var controllerURL string
	fmt.Scanln(&controllerURL)
	controllerURL = strings.TrimSpace(controllerURL)

	if controllerURL == "" {
		log.Fatal("Controller URL is required")
	}

	// Ensure URL has protocol
	if !strings.HasPrefix(controllerURL, "http://") && !strings.HasPrefix(controllerURL, "https://") {
		controllerURL = "https://" + controllerURL
	}

	// Check if already enrolled with this controller
	existing := getControllerByURL(config, controllerURL)
	if existing != nil && existing.Status == "active" {
		fmt.Printf("Already enrolled with controller %s (Node ID: %s)\n", controllerURL, existing.ID)
		fmt.Println("Use 'os-remove-node' to remove enrollment first, or enroll with a different controller.")
		os.Exit(1)
	}

	fmt.Printf("Using Controller URL: %s\n", controllerURL)
	fmt.Println()

	// Prompt for Node URL (how controller will reach this node)
	fmt.Print("Enter this Node's URL (e.g., http://192.168.1.100:8082): ")
	var nodeURL string
	fmt.Scanln(&nodeURL)
	nodeURL = strings.TrimSpace(nodeURL)

	if nodeURL == "" {
		log.Fatal("Node URL is required - this is how the controller will send files to this node")
	}

	// Ensure URL has protocol
	if !strings.HasPrefix(nodeURL, "http://") && !strings.HasPrefix(nodeURL, "https://") {
		nodeURL = "http://" + nodeURL
	}

	fmt.Printf("Using Node URL: %s\n", nodeURL)
	fmt.Println()

	// Prompt for OTP
	fmt.Print("Enter OTP from controller dashboard: ")
	otpBytes, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		log.Fatal("Failed to read OTP:", err)
	}
	otp := strings.TrimSpace(string(otpBytes))
	fmt.Println() // New line after password input

	if otp == "" {
		log.Fatal("OTP is required")
	}

	fmt.Println("Generating certificate request...")
	fmt.Println()

	// Generate RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		log.Fatal("Failed to generate private key:", err)
	}

	// Create CSR
	csrTemplate := x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName: "os-node",
		},
	}

	csrBytes, err := x509.CreateCertificateRequest(rand.Reader, &csrTemplate, privateKey)
	if err != nil {
		log.Fatal("Failed to create CSR:", err)
	}

	// Encode CSR to PEM
	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrBytes,
	})

	// Base64 encode CSR
	csrB64 := base64.StdEncoding.EncodeToString(csrPEM)

	// Get system info
	systemInfo := map[string]interface{}{
		"hostname": getHostname(),
		"os":       "buildroot",
		"arch":     "x86_64",
	}

	// Create request
	req := RegisterRequest{
		OTP:        otp,
		CSR:        csrB64,
		NodeURL:    nodeURL,
		SystemInfo: systemInfo,
	}

	// Marshal to JSON
	reqBytes, err := json.Marshal(req)
	if err != nil {
		log.Fatal("Failed to marshal request:", err)
	}

	// Make HTTP request
	url := controllerURL + "/v1/provisioning/register-request"
	fmt.Printf("Sending enrollment request to %s...\n", url)

	resp, err := http.Post(url, "application/json", bytes.NewReader(reqBytes))
	if err != nil {
		log.Fatal("Failed to send request:", err)
	}
	defer resp.Body.Close()

	// Read response
	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatal("Failed to read response:", err)
	}

	var regResp RegisterResponse
	if err := json.Unmarshal(respBytes, &regResp); err != nil {
		log.Fatal("Failed to parse response:", err)
	}

	if resp.StatusCode != http.StatusAccepted {
		fmt.Printf("Enrollment failed: %s\n", regResp.Message)
		os.Exit(1)
	}

	requestID := regResp.RequestID
	fmt.Printf("Enrollment request submitted (ID: %s)\n", requestID)
	fmt.Println("Waiting for administrator approval...")
	fmt.Println()

	// Poll for status
	statusURL := controllerURL + "/v1/provisioning/register-status/" + requestID

	for {
		resp, err := http.Get(statusURL)
		if err != nil {
			log.Printf("Failed to check status: %v", err)
			continue
		}

		respBytes, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			log.Printf("Failed to read status response: %v", err)
			continue
		}

		var statusResp StatusResponse
		if err := json.Unmarshal(respBytes, &statusResp); err != nil {
			log.Printf("Failed to parse status response: %v", err)
			continue
		}

		switch statusResp.Status {
		case "approved":
			fmt.Println("Enrollment approved!")
			fmt.Printf("Node ID: %s\n", statusResp.NodeID)
			fmt.Println()

			// Create unique directory for this controller's credentials
			controllerHash := generateControllerHash(controllerURL)
			certSubDir := filepath.Join(CertDir, "controllers", controllerHash)

			// Save certificate and key
			if err := saveCertificateToDir(statusResp.Certificate, statusResp.CACertificate, privateKey, certSubDir); err != nil {
				log.Fatal("Failed to save certificate:", err)
			}

			// Create controller entry
			entry := ControllerEntry{
				ID:            statusResp.NodeID,
				ControllerURL: controllerURL,
				NodeURL:       nodeURL,
				EnrolledAt:    time.Now().Format(time.RFC3339),
				CertFile:      filepath.Join(certSubDir, "os.crt"),
				KeyFile:       filepath.Join(certSubDir, "os.key"),
				CAFile:        filepath.Join(certSubDir, "ca.crt"),
				Status:        "active",
			}

			// Remove old entry for this controller if exists
			config = removeControllerByURL(config, controllerURL)

			// Add to config
			config.Controllers = append(config.Controllers, entry)

			// Save config
			if err := saveControllersConfig(config); err != nil {
				fmt.Printf("Warning: failed to save controllers config: %v\n", err)
			}

			// Also maintain backward compatibility - save to standard locations
			saveBackwardCompatibility(statusResp.NodeID, controllerURL, nodeURL, certSubDir)

			fmt.Println("Enrollment complete!")
			fmt.Println("Certificate and private key saved to /etc/os/")
			fmt.Println("Node ID saved to /etc/os/node_id")
			fmt.Println("Node URL saved to /etc/os/node_url")
			fmt.Println("Controller URL saved to /etc/os/controller_url")
			fmt.Printf("Total controllers enrolled: %d\n", len(config.Controllers))
			fmt.Println()
			fmt.Println("You can now start the storage server with: server-connect")
			fmt.Println("To enroll with additional controllers, run 'os-storage-enroll' again.")
			fmt.Println("To remove enrollment, run 'os-remove-node'.")
			return

		case "rejected":
			fmt.Printf("Enrollment rejected: %s\n", statusResp.Message)
			os.Exit(1)

		case "pending":
			fmt.Printf("Waiting: %s\n", statusResp.Message)
			fmt.Println("Checking again in 5 seconds...")
			fmt.Println()

		default:
			fmt.Printf("Unknown status: %s\n", statusResp.Status)
		}

		// Wait before next check
		select {
		case <-time.After(5 * time.Second):
		}
	}
}

func getHostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		return "os-node"
	}
	return hostname
}

func generateControllerHash(url string) string {
	clean := strings.ReplaceAll(url, "http://", "")
	clean = strings.ReplaceAll(clean, "https://", "")
	clean = strings.ReplaceAll(clean, ":", "_")
	clean = strings.ReplaceAll(clean, "/", "_")
	clean = strings.ReplaceAll(clean, ".", "_")
	return clean
}

func saveCertificateToDir(certB64, caCertB64 string, privateKey *rsa.PrivateKey, certDir string) error {
	// Decode certificate
	certPEM, err := base64.StdEncoding.DecodeString(certB64)
	if err != nil {
		return fmt.Errorf("decode certificate: %w", err)
	}

	// Decode CA certificate
	caCertPEM, err := base64.StdEncoding.DecodeString(caCertB64)
	if err != nil {
		return fmt.Errorf("decode CA certificate: %w", err)
	}

	// Encode private key to PEM
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Ensure directory exists
	if err := os.MkdirAll(certDir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// Write certificate
	if err := os.WriteFile(filepath.Join(certDir, "os.crt"), certPEM, 0644); err != nil {
		return fmt.Errorf("write certificate: %w", err)
	}

	// Write private key
	if err := os.WriteFile(filepath.Join(certDir, "os.key"), keyPEM, 0600); err != nil {
		return fmt.Errorf("write private key: %w", err)
	}

	// Write CA certificate
	if err := os.WriteFile(filepath.Join(certDir, "ca.crt"), caCertPEM, 0644); err != nil {
		return fmt.Errorf("write CA certificate: %w", err)
	}

	return nil
}

func saveCertificate(certB64, caCertB64 string, privateKey *rsa.PrivateKey) error {
	return saveCertificateToDir(certB64, caCertB64, privateKey, CertDir)
}

func saveBackwardCompatibility(nodeID, controllerURL, nodeURL, certSubDir string) {
	if err := os.MkdirAll(CertDir, 0755); err != nil {
		return
	}

	os.WriteFile(filepath.Join(CertDir, "node_id"), []byte(nodeID), 0644)
	os.WriteFile(filepath.Join(CertDir, "controller_url"), []byte(controllerURL), 0644)
	os.WriteFile(filepath.Join(CertDir, "node_url"), []byte(nodeURL), 0644)

	copyFile(filepath.Join(certSubDir, "os.crt"), filepath.Join(CertDir, "os.crt"))
	copyFile(filepath.Join(certSubDir, "os.key"), filepath.Join(CertDir, "os.key"))
	copyFile(filepath.Join(certSubDir, "ca.crt"), filepath.Join(CertDir, "ca.crt"))
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0600)
}

func loadControllersConfig() (*ControllersConfig, error) {
	config := &ControllersConfig{Controllers: []ControllerEntry{}}

	data, err := os.ReadFile(ControllersConfigFile)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil
		}
		return nil, err
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, err
	}

	return config, nil
}

func saveControllersConfig(config *ControllersConfig) error {
	if err := os.MkdirAll(CertDir, 0700); err != nil {
		return err
	}

	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(ControllersConfigFile, data, 0600)
}

func getControllerByURL(config *ControllersConfig, url string) *ControllerEntry {
	for i := range config.Controllers {
		if config.Controllers[i].ControllerURL == url {
			return &config.Controllers[i]
		}
	}
	return nil
}

func removeControllerByURL(config *ControllersConfig, url string) *ControllersConfig {
	var newControllers []ControllerEntry
	for _, c := range config.Controllers {
		if c.ControllerURL != url {
			newControllers = append(newControllers, c)
		}
	}
	config.Controllers = newControllers
	return config
}