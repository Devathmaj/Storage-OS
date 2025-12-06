package main

import (
	"bufio"
	"bytes"
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
	fmt.Println("   OS NODE REMOVAL")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println()

	// Load controllers config
	config, err := loadControllersConfig()
	if err != nil || len(config.Controllers) == 0 {
		// Check legacy enrollment
		if isLegacyEnrolled() {
			fmt.Println("Legacy enrollment detected.")
			fmt.Println("Do you want to remove the legacy enrollment? (yes/no)")
			if confirmAction() {
				removeLegacyEnrollment()
			}
			return
		}
		fmt.Println("❌ No enrolled controllers found.")
		os.Exit(1)
	}

	// List enrolled controllers
	fmt.Println("Enrolled Controllers:")
	fmt.Println(strings.Repeat("-", 60))
	activeCount := 0
	for i, c := range config.Controllers {
		status := c.Status
		if status == "" {
			status = "unknown"
		}
		fmt.Printf("%d. %s\n", i+1, c.ControllerURL)
		fmt.Printf("   Node ID: %s\n", c.ID)
		fmt.Printf("   Status: %s\n", status)
		fmt.Printf("   Enrolled: %s\n", c.EnrolledAt)
		fmt.Println()
		if status == "active" {
			activeCount++
		}
	}

	if activeCount == 0 {
		fmt.Println("No active enrollments to remove.")
		os.Exit(0)
	}

	// Ask which controller to remove from
	reader := bufio.NewReader(os.Stdin)
	fmt.Print("Enter number of controller to remove from (or 'all' to remove all): ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(strings.ToLower(choice))

	if choice == "all" {
		fmt.Println("\n⚠️  This will remove enrollment from ALL controllers.")
		fmt.Println("Are you sure? (yes/no)")
		if confirmAction() {
			for _, c := range config.Controllers {
				if c.Status == "active" {
					removeFromController(config, c)
				}
			}
			fmt.Println("\n✅ Removed from all controllers")
		}
		return
	}

	// Parse number
	var idx int
	_, err = fmt.Sscanf(choice, "%d", &idx)
	if err != nil || idx < 1 || idx > len(config.Controllers) {
		fmt.Println("❌ Invalid selection")
		os.Exit(1)
	}

	controller := config.Controllers[idx-1]
	if controller.Status != "active" {
		fmt.Printf("❌ Controller %s is already %s\n", controller.ControllerURL, controller.Status)
		os.Exit(1)
	}

	fmt.Printf("\n⚠️  This will remove enrollment from:\n")
	fmt.Printf("   Controller: %s\n", controller.ControllerURL)
	fmt.Printf("   Node ID: %s\n", controller.ID)
	fmt.Println("\nThe controller will be notified and the node will be marked as unavailable.")
	fmt.Println("Are you sure? (yes/no)")

	if confirmAction() {
		removeFromController(config, controller)
		fmt.Println("\n✅ Successfully removed from controller")
	} else {
		fmt.Println("Cancelled.")
	}
}

func confirmAction() bool {
	reader := bufio.NewReader(os.Stdin)
	response, _ := reader.ReadString('\n')
	response = strings.TrimSpace(strings.ToLower(response))
	return response == "yes" || response == "y"
}

func removeFromController(config *ControllersConfig, controller ControllerEntry) {
	fmt.Printf("\nRemoving from %s...\n", controller.ControllerURL)

	// 1. Notify controller that we're leaving
	notifyControllerOfRemoval(controller)

	// 2. Delete local certificates for this controller
	deleteControllerCerts(controller)

	// 3. Update config status
	updateControllerStatus(config, controller.ID, "removed")

	fmt.Printf("✓ Cleaned up enrollment for %s\n", controller.ControllerURL)
}

func notifyControllerOfRemoval(controller ControllerEntry) {
	fmt.Println("   Notifying controller...")

	// Create notification payload
	payload := map[string]interface{}{
		"node_id":     controller.ID,
		"action":      "node_removal",
		"reason":      "User initiated removal via os-remove-node",
		"removed_at":  time.Now().Format(time.RFC3339),
	}

	jsonData, err := json.Marshal(payload)
	if err != nil {
		fmt.Printf("   ⚠️ Warning: failed to create notification: %v\n", err)
		return
	}

	// Send to controller's node-removal endpoint
	url := fmt.Sprintf("%s/v1/provisioning/node-removal", controller.ControllerURL)
	
	client := &http.Client{Timeout: 10 * time.Second}
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		fmt.Printf("   ⚠️ Warning: failed to create request: %v\n", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("   ⚠️ Warning: failed to notify controller: %v\n", err)
		fmt.Println("   Controller may not be reachable. Continuing with local cleanup...")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
		fmt.Println("   ✓ Controller notified successfully")
	} else {
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("   ⚠️ Warning: controller returned status %d: %s\n", resp.StatusCode, string(body))
	}
}

func deleteControllerCerts(controller ControllerEntry) {
	fmt.Println("   Deleting local certificates...")

	// Delete certificate files
	filesToDelete := []string{
		controller.CertFile,
		controller.KeyFile,
		controller.CAFile,
	}

	for _, file := range filesToDelete {
		if file != "" {
			if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
				fmt.Printf("   ⚠️ Warning: failed to delete %s: %v\n", file, err)
			}
		}
	}

	// Try to delete the controller's cert directory
	if controller.CertFile != "" {
		certDir := filepath.Dir(controller.CertFile)
		// Only delete if it's a subdirectory of /etc/os/controllers
		if strings.HasPrefix(certDir, filepath.Join(CertDir, "controllers")) {
			if err := os.RemoveAll(certDir); err != nil && !os.IsNotExist(err) {
				fmt.Printf("   ⚠️ Warning: failed to delete cert directory: %v\n", err)
			}
		}
	}

	fmt.Println("   ✓ Local certificates deleted")
}

func updateControllerStatus(config *ControllersConfig, nodeID, status string) {
	for i := range config.Controllers {
		if config.Controllers[i].ID == nodeID {
			config.Controllers[i].Status = status
			break
		}
	}

	if err := saveControllersConfig(config); err != nil {
		fmt.Printf("   ⚠️ Warning: failed to update config: %v\n", err)
	}
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

func isLegacyEnrolled() bool {
	for _, path := range []string{"/etc/os/os.crt", "/etc/os/os.key", "/etc/os/node_id"} {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return false
		}
	}
	return true
}

func removeLegacyEnrollment() {
	fmt.Println("\nRemoving legacy enrollment...")

	// Try to notify controller
	controllerURL := ""
	if data, err := os.ReadFile("/etc/os/controller_url"); err == nil {
		controllerURL = strings.TrimSpace(string(data))
	}

	nodeID := ""
	if data, err := os.ReadFile("/etc/os/node_id"); err == nil {
		nodeID = strings.TrimSpace(string(data))
	}

	if controllerURL != "" && nodeID != "" {
		controller := ControllerEntry{
			ID:            nodeID,
			ControllerURL: controllerURL,
		}
		notifyControllerOfRemoval(controller)
	}

	// Delete legacy files
	legacyFiles := []string{
		"/etc/os/os.crt",
		"/etc/os/os.key",
		"/etc/os/ca.crt",
		"/etc/os/node_id",
		"/etc/os/controller_url",
		"/etc/os/node_url",
	}

	for _, file := range legacyFiles {
		if err := os.Remove(file); err != nil && !os.IsNotExist(err) {
			fmt.Printf("⚠️ Warning: failed to delete %s: %v\n", file, err)
		}
	}

	fmt.Println("✅ Legacy enrollment removed")
}
