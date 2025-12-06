package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	
	"os-storage-server/enrollment"
)

const (
	CertDir = "/etc/os"
)

func main() {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("   OS NODE ENROLLMENT")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// Load existing controllers config
	config, err := LoadControllersConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Warning: could not load existing config: %v\n", err)
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

	reader := bufio.NewReader(os.Stdin)

	// Prompt for controller URL
	fmt.Print("Enter Controller URL (e.g., http://192.168.1.100:8081): ")
	controllerURL, _ := reader.ReadString('\n')
	controllerURL = strings.TrimSpace(controllerURL)
	
	if controllerURL == "" {
		fmt.Println("❌ Controller URL is required")
		os.Exit(1)
	}
	
	// Ensure URL has protocol
	if !strings.HasPrefix(controllerURL, "http://") && !strings.HasPrefix(controllerURL, "https://") {
		controllerURL = "http://" + controllerURL
	}

	// Check if already enrolled with this controller
	existing, err := config.GetControllerByURL(controllerURL)
	if err == nil && existing.Status == "active" {
		fmt.Printf("❌ Already enrolled with controller %s (Node ID: %s)\n", controllerURL, existing.ID)
		fmt.Println("   Use 'os-remove-node' to remove enrollment first, or enroll with a different controller.")
		os.Exit(1)
	}
	
	fmt.Printf("✓ Using Controller URL: %s\n", controllerURL)
	fmt.Println()
	
	// Prompt for node URL
	fmt.Print("Enter Node URL (e.g., http://192.168.1.100:8082): ")
	nodeURL, _ := reader.ReadString('\n')
	nodeURL = strings.TrimSpace(nodeURL)
	
	if nodeURL == "" {
		fmt.Println("❌ Node URL is required")
		os.Exit(1)
	}
	
	// Ensure URL has protocol
	if !strings.HasPrefix(nodeURL, "http://") && !strings.HasPrefix(nodeURL, "https://") {
		nodeURL = "http://" + nodeURL
	}
	
	fmt.Printf("✓ Using Node URL: %s\n", nodeURL)
	fmt.Println()
	
	// Create unique directory for this controller's credentials
	controllerHash := generateControllerHash(controllerURL)
	certSubDir := filepath.Join(CertDir, "controllers", controllerHash)
	if err := os.MkdirAll(certSubDir, 0700); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create cert directory: %v\n", err)
		os.Exit(1)
	}

	// Create enrollment client
	enrollmentClient := enrollment.NewEnrollmentClientWithDir(controllerURL, certSubDir)
	
	// Prompt for OTP
	fmt.Println("An administrator must generate an OTP from the controller dashboard.")
	fmt.Println()
	fmt.Print("Enter the 6-digit OTP: ")
	otp, _ := reader.ReadString('\n')
	otp = strings.TrimSpace(otp)
	
	if len(otp) != 6 {
		fmt.Println("❌ OTP must be exactly 6 digits")
		os.Exit(1)
	}
	
	for _, c := range otp {
		if c < '0' || c > '9' {
			fmt.Println("❌ OTP must contain only digits")
			os.Exit(1)
		}
	}

	// Save temporary info for enrollment (enrollment package needs these)
	tempNodeURLPath := filepath.Join(certSubDir, "node_url")
	if err := os.WriteFile(tempNodeURLPath, []byte(nodeURL), 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to save node URL: %v\n", err)
		os.Exit(1)
	}

	// Run enrollment with the OTP
	nodeID, err := enrollmentClient.RunEnrollmentWithOTPAndGetNodeID(otp)
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n❌ Enrollment failed: %v\n", err)
		os.Exit(1)
	}

	// Create controller entry
	entry := ControllerEntry{
		ID:            nodeID,
		ControllerURL: controllerURL,
		NodeURL:       nodeURL,
		EnrolledAt:    time.Now().Format(time.RFC3339),
		CertFile:      filepath.Join(certSubDir, "os.crt"),
		KeyFile:       filepath.Join(certSubDir, "os.key"),
		CAFile:        filepath.Join(certSubDir, "ca.crt"),
		Status:        "active",
	}

	// Add to config
	if err := config.AddController(entry); err != nil {
		// If duplicate, update it
		config.RemoveController(nodeID)
		config.AddController(entry)
	}

	// Save config
	if err := config.Save(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: failed to save controllers config: %v\n", err)
	}

	// Also maintain backward compatibility - save latest node info to standard locations
	saveBackwardCompatibility(nodeID, controllerURL, nodeURL, certSubDir)

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✅ Enrollment successful!")
	fmt.Printf("   Node ID: %s\n", nodeID)
	fmt.Printf("   Controller: %s\n", controllerURL)
	fmt.Printf("   Total controllers enrolled: %d\n", len(config.Controllers))
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()
	fmt.Println("You can now run 'server-connect' to start the storage server.")
	fmt.Println("To enroll with additional controllers, run 'os-storage-enroll' again.")
	fmt.Println("To list enrolled controllers, check /etc/os/controllers.json")
}

// generateControllerHash creates a unique directory name for a controller
func generateControllerHash(url string) string {
	// Simple hash based on URL - remove protocol and special chars
	clean := strings.ReplaceAll(url, "http://", "")
	clean = strings.ReplaceAll(clean, "https://", "")
	clean = strings.ReplaceAll(clean, ":", "_")
	clean = strings.ReplaceAll(clean, "/", "_")
	clean = strings.ReplaceAll(clean, ".", "_")
	return clean
}

// saveBackwardCompatibility saves to legacy locations for backward compat
func saveBackwardCompatibility(nodeID, controllerURL, nodeURL, certSubDir string) {
	// Save to standard /etc/os locations (for server-connect and other tools)
	if err := os.MkdirAll(CertDir, 0700); err != nil {
		return
	}

	// Node ID
	os.WriteFile(filepath.Join(CertDir, "node_id"), []byte(nodeID), 0644)
	// Controller URL
	os.WriteFile(filepath.Join(CertDir, "controller_url"), []byte(controllerURL), 0644)
	// Node URL  
	os.WriteFile(filepath.Join(CertDir, "node_url"), []byte(nodeURL), 0644)

	// Copy certs to standard location (latest enrollment)
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

// LoadControllersConfig loads from the JSON file
func LoadControllersConfig() (*ControllersConfig, error) {
	config := &ControllersConfig{Controllers: []ControllerEntry{}}
	
	data, err := os.ReadFile("/etc/os/controllers.json")
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

// ControllerEntry and ControllersConfig are defined here for standalone compilation
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

type ControllersConfig struct {
	Controllers []ControllerEntry `json:"controllers"`
}

func (c *ControllersConfig) Save() error {
	if err := os.MkdirAll("/etc/os", 0700); err != nil {
		return err
	}
	
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	
	return os.WriteFile("/etc/os/controllers.json", data, 0600)
}

func (c *ControllersConfig) AddController(entry ControllerEntry) error {
	for _, existing := range c.Controllers {
		if existing.ControllerURL == entry.ControllerURL {
			return fmt.Errorf("controller already enrolled")
		}
	}
	c.Controllers = append(c.Controllers, entry)
	return nil
}

func (c *ControllersConfig) RemoveController(nodeID string) error {
	for i, entry := range c.Controllers {
		if entry.ID == nodeID {
			c.Controllers = append(c.Controllers[:i], c.Controllers[i+1:]...)
			return nil
		}
	}
	return fmt.Errorf("not found")
}

func (c *ControllersConfig) GetControllerByURL(url string) (*ControllerEntry, error) {
	for i := range c.Controllers {
		if c.Controllers[i].ControllerURL == url {
			return &c.Controllers[i], nil
		}
	}
	return nil, fmt.Errorf("not found")
}
