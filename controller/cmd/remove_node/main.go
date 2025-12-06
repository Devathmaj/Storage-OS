package main

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	controllersConfigPath = "/etc/os/controllers.json"
)

// ControllerEntry represents a single controller enrollment
type ControllerEntry struct {
	ID         string    `json:"id"`
	URL        string    `json:"url"`
	NodeID     string    `json:"node_id"`
	NodeURL    string    `json:"node_url"`
	CertDir    string    `json:"cert_dir"`
	Status     string    `json:"status"`
	EnrolledAt time.Time `json:"enrolled_at"`
}

// ControllersConfig holds all controller enrollments
type ControllersConfig struct {
	Version     int               `json:"version"`
	LastUpdated time.Time         `json:"last_updated"`
	Controllers []ControllerEntry `json:"controllers"`
}

// NodeRemovalRequest is sent to controller when node removes itself
type NodeRemovalRequest struct {
	NodeID string `json:"node_id"`
	Reason string `json:"reason"`
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "list":
		listControllers()
	case "remove":
		if len(os.Args) < 3 {
			fmt.Println("Error: Controller ID required")
			fmt.Println("Usage: os-remove-node remove <controller-id|all>")
			os.Exit(1)
		}
		removeController(os.Args[2])
	case "status":
		showStatus()
	case "help", "-h", "--help":
		printUsage()
	default:
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("os-remove-node - Remove this node from controller(s)")
	fmt.Println()
	fmt.Println("Usage:")
	fmt.Println("  os-remove-node list                  - List all enrolled controllers")
	fmt.Println("  os-remove-node remove <id>           - Remove from specific controller")
	fmt.Println("  os-remove-node remove all            - Remove from all controllers")
	fmt.Println("  os-remove-node status                - Show enrollment status")
	fmt.Println("  os-remove-node help                  - Show this help")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  os-remove-node list")
	fmt.Println("  os-remove-node remove abc123")
	fmt.Println("  os-remove-node remove all")
}

func loadControllers() (*ControllersConfig, error) {
	data, err := os.ReadFile(controllersConfigPath)
	if err != nil {
		if os.IsNotExist(err) {
			return &ControllersConfig{
				Version:     1,
				Controllers: []ControllerEntry{},
			}, nil
		}
		return nil, fmt.Errorf("failed to read config: %w", err)
	}

	var config ControllersConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

func saveControllers(config *ControllersConfig) error {
	config.LastUpdated = time.Now()
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(controllersConfigPath), 0755); err != nil {
		return fmt.Errorf("failed to create config dir: %w", err)
	}

	if err := os.WriteFile(controllersConfigPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

func listControllers() {
	config, err := loadControllers()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	if len(config.Controllers) == 0 {
		fmt.Println("No controllers enrolled")
		return
	}

	fmt.Println("Enrolled Controllers:")
	fmt.Println(strings.Repeat("-", 80))
	for i, c := range config.Controllers {
		fmt.Printf("%d. ID: %s\n", i+1, c.ID)
		fmt.Printf("   URL: %s\n", c.URL)
		fmt.Printf("   Node ID: %s\n", c.NodeID)
		fmt.Printf("   Status: %s\n", c.Status)
		fmt.Printf("   Enrolled: %s\n", c.EnrolledAt.Format(time.RFC3339))
		fmt.Printf("   Cert Dir: %s\n", c.CertDir)
		if i < len(config.Controllers)-1 {
			fmt.Println()
		}
	}
	fmt.Println(strings.Repeat("-", 80))
}

func showStatus() {
	config, err := loadControllers()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Config Version: %d\n", config.Version)
	fmt.Printf("Last Updated: %s\n", config.LastUpdated.Format(time.RFC3339))
	fmt.Printf("Enrolled Controllers: %d\n", len(config.Controllers))

	active := 0
	for _, c := range config.Controllers {
		if c.Status == "active" {
			active++
		}
	}
	fmt.Printf("Active Controllers: %d\n", active)

	// Check if certs exist for each controller
	fmt.Println()
	fmt.Println("Certificate Status:")
	for _, c := range config.Controllers {
		certPath := filepath.Join(c.CertDir, "client.crt")
		if _, err := os.Stat(certPath); err == nil {
			fmt.Printf("  %s: certificates present\n", c.ID[:8])
		} else {
			fmt.Printf("  %s: certificates missing\n", c.ID[:8])
		}
	}
}

func removeController(idOrAll string) {
	config, err := loadControllers()
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	if len(config.Controllers) == 0 {
		fmt.Println("No controllers enrolled")
		return
	}

	if strings.ToLower(idOrAll) == "all" {
		removeAllControllers(config)
		return
	}

	// Find the controller by ID (prefix match allowed)
	var found *ControllerEntry
	var foundIndex int
	for i, c := range config.Controllers {
		if c.ID == idOrAll || strings.HasPrefix(c.ID, idOrAll) {
			found = &config.Controllers[i]
			foundIndex = i
			break
		}
	}

	if found == nil {
		fmt.Printf("Controller with ID '%s' not found\n", idOrAll)
		fmt.Println("Use 'os-remove-node list' to see enrolled controllers")
		os.Exit(1)
	}

	fmt.Printf("Removing from controller: %s\n", found.URL)

	// Notify the controller about removal
	if err := notifyControllerRemoval(found); err != nil {
		fmt.Printf("Warning: Could not notify controller: %v\n", err)
		fmt.Println("Proceeding with local cleanup...")
	} else {
		fmt.Println("Controller notified successfully")
	}

	// Delete certificates
	if err := deleteCertificates(found.CertDir); err != nil {
		fmt.Printf("Warning: Could not delete certificates: %v\n", err)
	} else {
		fmt.Println("Certificates deleted")
	}

	// Remove from config
	config.Controllers = append(config.Controllers[:foundIndex], config.Controllers[foundIndex+1:]...)
	if err := saveControllers(config); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Successfully removed from controller")
}

func removeAllControllers(config *ControllersConfig) {
	fmt.Printf("Removing from %d controller(s)...\n", len(config.Controllers))

	for _, c := range config.Controllers {
		fmt.Printf("\nRemoving from: %s\n", c.URL)

		// Notify controller
		if err := notifyControllerRemoval(&c); err != nil {
			fmt.Printf("  Warning: Could not notify controller: %v\n", err)
		} else {
			fmt.Println("  Controller notified")
		}

		// Delete certificates
		if err := deleteCertificates(c.CertDir); err != nil {
			fmt.Printf("  Warning: Could not delete certificates: %v\n", err)
		} else {
			fmt.Println("  Certificates deleted")
		}
	}

	// Clear all controllers from config
	config.Controllers = []ControllerEntry{}
	if err := saveControllers(config); err != nil {
		fmt.Printf("Error saving config: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("\nSuccessfully removed from all controllers")
}

func notifyControllerRemoval(controller *ControllerEntry) error {
	// Load mTLS certificates for this controller
	certPath := filepath.Join(controller.CertDir, "client.crt")
	keyPath := filepath.Join(controller.CertDir, "client.key")
	caPath := filepath.Join(controller.CertDir, "ca.crt")

	// Check if certificates exist
	if _, err := os.Stat(certPath); os.IsNotExist(err) {
		return fmt.Errorf("client certificate not found")
	}

	// Load client certificate
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return fmt.Errorf("failed to load client certificate: %w", err)
	}

	// Load CA certificate
	caCert, err := os.ReadFile(caPath)
	if err != nil {
		return fmt.Errorf("failed to read CA certificate: %w", err)
	}

	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Create TLS config
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
		RootCAs:      caCertPool,
	}

	// Create HTTP client with mTLS
	client := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
		Timeout: 30 * time.Second,
	}

	// Prepare removal request
	reqBody := NodeRemovalRequest{
		NodeID: controller.NodeID,
		Reason: "user_initiated",
	}

	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send removal notification
	url := fmt.Sprintf("%s/v1/provisioning/node-removal", controller.URL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func deleteCertificates(certDir string) error {
	files := []string{"client.crt", "client.key", "ca.crt"}
	var errors []string

	for _, f := range files {
		path := filepath.Join(certDir, f)
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			errors = append(errors, fmt.Sprintf("%s: %v", f, err))
		}
	}

	// Try to remove the directory if empty
	os.Remove(certDir)

	if len(errors) > 0 {
		return fmt.Errorf("some files could not be deleted: %s", strings.Join(errors, ", "))
	}

	return nil
}
