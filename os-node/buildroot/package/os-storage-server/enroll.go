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

func main() {
	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("   OS NODE ENROLLMENT")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// Load existing controllers config
	config, err := loadControllersConfig()
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
	existing := getControllerByURL(config, controllerURL)
	if existing != nil && existing.Status == "active" {
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

	// Create enrollment client with custom directory
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

	// Remove old entry for this controller if exists (e.g., if re-enrolling)
	config = removeControllerByURL(config, controllerURL)
	
	// Add to config
	config.Controllers = append(config.Controllers, entry)

	// Save config
	if err := saveControllersConfig(config); err != nil {
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
	fmt.Println("To remove enrollment, run 'os-remove-node'.")
}

// generateControllerHash creates a unique directory name for a controller
func generateControllerHash(url string) string {
	clean := strings.ReplaceAll(url, "http://", "")
	clean = strings.ReplaceAll(clean, "https://", "")
	clean = strings.ReplaceAll(clean, ":", "_")
	clean = strings.ReplaceAll(clean, "/", "_")
	clean = strings.ReplaceAll(clean, ".", "_")
	return clean
}

// saveBackwardCompatibility saves to legacy locations for backward compat
func saveBackwardCompatibility(nodeID, controllerURL, nodeURL, certSubDir string) {
	if err := os.MkdirAll(CertDir, 0700); err != nil {
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
