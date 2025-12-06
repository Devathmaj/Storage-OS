package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

const (
	ControllersConfigFile = "/etc/os/controllers.json"
)

// ControllerEntry represents a single controller's enrollment data
type ControllerEntry struct {
	ID            string `json:"id"`              // Unique identifier (node_id from controller)
	ControllerURL string `json:"controller_url"`  // Controller URL
	NodeURL       string `json:"node_url"`        // This node's URL for this controller
	EnrolledAt    string `json:"enrolled_at"`     // Enrollment timestamp
	CertFile      string `json:"cert_file"`       // Path to certificate
	KeyFile       string `json:"key_file"`        // Path to private key
	CAFile        string `json:"ca_file"`         // Path to CA certificate
	Status        string `json:"status"`          // "active", "revoked", "removed"
}

// ControllersConfig holds all enrolled controller entries
type ControllersConfig struct {
	Controllers []ControllerEntry `json:"controllers"`
	mu          sync.RWMutex      `json:"-"`
}

// LoadControllersConfig loads the controllers configuration from file
func LoadControllersConfig() (*ControllersConfig, error) {
	config := &ControllersConfig{
		Controllers: []ControllerEntry{},
	}

	data, err := os.ReadFile(ControllersConfigFile)
	if err != nil {
		if os.IsNotExist(err) {
			return config, nil // Return empty config if file doesn't exist
		}
		return nil, fmt.Errorf("read controllers config: %w", err)
	}

	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("parse controllers config: %w", err)
	}

	return config, nil
}

// Save saves the controllers configuration to file
func (c *ControllersConfig) Save() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Ensure directory exists
	if err := os.MkdirAll("/etc/os", 0700); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal controllers config: %w", err)
	}

	if err := os.WriteFile(ControllersConfigFile, data, 0600); err != nil {
		return fmt.Errorf("write controllers config: %w", err)
	}

	return nil
}

// AddController adds a new controller entry
func (c *ControllersConfig) AddController(entry ControllerEntry) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Check if controller URL already exists
	for _, existing := range c.Controllers {
		if existing.ControllerURL == entry.ControllerURL {
			return fmt.Errorf("controller %s is already enrolled", entry.ControllerURL)
		}
	}

	c.Controllers = append(c.Controllers, entry)
	return nil
}

// RemoveController removes a controller entry by its ID
func (c *ControllersConfig) RemoveController(nodeID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, entry := range c.Controllers {
		if entry.ID == nodeID {
			// Remove entry from slice
			c.Controllers = append(c.Controllers[:i], c.Controllers[i+1:]...)
			return nil
		}
	}

	return fmt.Errorf("controller with node ID %s not found", nodeID)
}

// GetControllerByID returns a controller entry by its node ID
func (c *ControllersConfig) GetControllerByID(nodeID string) (*ControllerEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := range c.Controllers {
		if c.Controllers[i].ID == nodeID {
			return &c.Controllers[i], nil
		}
	}

	return nil, fmt.Errorf("controller with node ID %s not found", nodeID)
}

// GetControllerByURL returns a controller entry by its URL
func (c *ControllersConfig) GetControllerByURL(url string) (*ControllerEntry, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	for i := range c.Controllers {
		if c.Controllers[i].ControllerURL == url {
			return &c.Controllers[i], nil
		}
	}

	return nil, fmt.Errorf("controller with URL %s not found", url)
}

// GetActiveControllers returns all controllers with "active" status
func (c *ControllersConfig) GetActiveControllers() []ControllerEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var active []ControllerEntry
	for _, entry := range c.Controllers {
		if entry.Status == "active" {
			active = append(active, entry)
		}
	}
	return active
}

// UpdateControllerStatus updates the status of a controller
func (c *ControllersConfig) UpdateControllerStatus(nodeID, status string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i := range c.Controllers {
		if c.Controllers[i].ID == nodeID {
			c.Controllers[i].Status = status
			return nil
		}
	}

	return fmt.Errorf("controller with node ID %s not found", nodeID)
}

// ListControllers returns a summary of all enrolled controllers
func (c *ControllersConfig) ListControllers() {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.Controllers) == 0 {
		fmt.Println("No controllers enrolled.")
		return
	}

	fmt.Println("\nEnrolled Controllers:")
	fmt.Println("======================================================================")
	for i, entry := range c.Controllers {
		fmt.Printf("%d. Node ID: %s\n", i+1, entry.ID)
		fmt.Printf("   Controller URL: %s\n", entry.ControllerURL)
		fmt.Printf("   Node URL: %s\n", entry.NodeURL)
		fmt.Printf("   Status: %s\n", entry.Status)
		fmt.Printf("   Enrolled At: %s\n", entry.EnrolledAt)
		fmt.Println()
	}
}
