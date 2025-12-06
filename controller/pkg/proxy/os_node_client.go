package proxy

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"storageos/controller/models"
)

// OSNodeClient handles communication with one or more OS storage nodes.
type OSNodeClient struct {
	db        *sql.DB
	client    *http.Client
	mu        sync.RWMutex
	endpoints []nodeEndpoint
}

type nodeEndpoint struct {
	ID   string
	Name string
	URL  string
}

// NewOSNodeClient creates a new OS node client that automatically pulls
// its endpoints from the controller database (with an env fallback).
func NewOSNodeClient(db *sql.DB) *OSNodeClient {
	c := &OSNodeClient{
		db: db,
		client: &http.Client{
			Timeout: 300 * time.Second, // allow large uploads
		},
	}
	if err := c.reloadEndpoints(); err != nil {
		log.Printf("warning: failed to load storage nodes: %v", err)
	}
	return c
}

// Reload refreshes the cached endpoints from the database.
func (c *OSNodeClient) Reload() error {
	return c.reloadEndpoints()
}

func (c *OSNodeClient) reloadEndpoints() error {
	if c.db == nil {
		return errors.New("database connection is not available")
	}
	nodes, err := models.ListActiveStorageNodes(c.db)
	if err != nil {
		return err
	}

	var endpoints []nodeEndpoint
	for _, n := range nodes {
		baseURL, err := n.EffectiveURL()
		if err != nil {
			log.Printf("warning: skipping storage node %s (%s): %v", n.ID, n.Name, err)
			continue
		}
		endpoints = append(endpoints, nodeEndpoint{ID: n.ID, Name: n.Name, URL: strings.TrimRight(baseURL, "/")})
	}

	if len(endpoints) == 0 {
		if legacy := strings.TrimSpace(os.Getenv("OS_NODE_URL")); legacy != "" {
			endpoints = append(endpoints, nodeEndpoint{ID: "legacy-env", Name: "Legacy OS Node", URL: strings.TrimRight(legacy, "/")})
		}
	}

	c.mu.Lock()
	c.endpoints = endpoints
	c.mu.Unlock()
	return nil
}

func (c *OSNodeClient) endpointsSnapshot() []nodeEndpoint {
	c.mu.RLock()
	defer c.mu.RUnlock()
	clone := make([]nodeEndpoint, len(c.endpoints))
	copy(clone, c.endpoints)
	return clone
}

// NodeCount returns the number of currently configured endpoints.
func (c *OSNodeClient) NodeCount() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.endpoints)
}

func (c *OSNodeClient) requireEndpoints() ([]nodeEndpoint, error) {
	endpoints := c.endpointsSnapshot()
	if len(endpoints) == 0 {
		return nil, errors.New("no storage nodes configured")
	}
	return endpoints, nil
}

// FileMetadata represents file metadata from OS node (exact schema.sql structure)
type FileMetadata struct {
	ID                string    `json:"id"`
	OwnerID           string    `json:"owner_id"`
	FolderID          string    `json:"folder_id"`
	Filename          string    `json:"filename"`
	Extension         string    `json:"extension"`
	OriginalExtension string    `json:"original_extension"`
	MimeType          string    `json:"mime_type"`
	Size              int64     `json:"size"`
	OriginalSize      int64     `json:"original_size"`
	Checksum          string    `json:"checksum"`
	StoragePath       string    `json:"storage_path"`
	EncryptedKey      []byte    `json:"encrypted_key"`
	EphemeralPubKey   []byte    `json:"ephemeral_pub_key"`
	NodeID            string    `json:"node_id"`
	Version           int       `json:"version"`
	Status            string    `json:"status"`
	IsIndexed         bool      `json:"is_indexed"`
	ParentArchiveID   string    `json:"parent_archive_id"`
	RelativePath      string    `json:"relative_path"`
	UploadedAt        time.Time `json:"uploaded_at"`
	ModifiedAt        time.Time `json:"modified_at"`
}

// UploadToNode uploads a file to every configured storage node and returns
// the first successful metadata response.
func (c *OSNodeClient) UploadToNode(ownerID, folderID, filename, mimeType, checksum string, file io.ReadSeeker, size int64) (*FileMetadata, error) {
	endpoints, err := c.requireEndpoints()
	if err != nil {
		return nil, err
	}
	_ = size // maintained for backward compatibility

	var first *FileMetadata
	var errs []error
	for _, endpoint := range endpoints {
		if _, seekErr := file.Seek(0, io.SeekStart); seekErr != nil {
			return nil, fmt.Errorf("rewind file before upload: %w", seekErr)
		}

		metadata, err := c.uploadToEndpoint(endpoint, ownerID, folderID, filename, mimeType, checksum, file)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", endpoint.Name, err))
			continue
		}
		if first == nil {
			first = metadata
		}
	}

	if first == nil {
		return nil, fmt.Errorf("upload failed: %w", errors.Join(errs...))
	}

	return first, nil
}

func (c *OSNodeClient) uploadToEndpoint(ep nodeEndpoint, ownerID, folderID, filename, mimeType, checksum string, file io.Reader) (*FileMetadata, error) {
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	_ = writer.WriteField("owner_id", ownerID)
	if folderID != "" {
		_ = writer.WriteField("folder_id", folderID)
	}
	if checksum != "" {
		_ = writer.WriteField("checksum", checksum)
	}
	if mimeType != "" {
		_ = writer.WriteField("mime_type", mimeType)
	}

	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("create form file: %w", err)
	}

	if _, err := io.Copy(part, file); err != nil {
		return nil, fmt.Errorf("copy file: %w", err)
	}

	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close writer: %w", err)
	}

	url := fmt.Sprintf("%s/api/upload", ep.URL)
	req, err := http.NewRequest("POST", url, body)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("upload failed: status=%d, body=%s", resp.StatusCode, string(respBody))
	}

	var metadata FileMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	return &metadata, nil
}

// DownloadFromNode downloads a file from the first node that responds.
func (c *OSNodeClient) DownloadFromNode(fileID string) (io.ReadCloser, *FileMetadata, error) {
	endpoints, err := c.requireEndpoints()
	if err != nil {
		return nil, nil, err
	}

	var errs []error
	for _, ep := range endpoints {
		metadata, err := c.getMetadataFromEndpoint(ep, fileID)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s metadata: %w", ep.Name, err))
			continue
		}

		url := fmt.Sprintf("%s/api/download?file_id=%s", ep.URL, fileID)
		resp, err := c.client.Get(url)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s download: %w", ep.Name, err))
			continue
		}

		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			errs = append(errs, fmt.Errorf("%s download failed: status=%d", ep.Name, resp.StatusCode))
			continue
		}

		return resp.Body, metadata, nil
	}

	return nil, nil, fmt.Errorf("download failed: %w", errors.Join(errs...))
}

// ListFiles lists files for a user from the first node that responds.
func (c *OSNodeClient) ListFiles(ownerID string) ([]FileMetadata, error) {
	endpoints, err := c.requireEndpoints()
	if err != nil {
		return nil, err
	}

	var errs []error
	for _, ep := range endpoints {
		url := fmt.Sprintf("%s/api/files?owner_id=%s", ep.URL, ownerID)
		resp, err := c.client.Get(url)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s list: %w", ep.Name, err))
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			errs = append(errs, fmt.Errorf("%s list failed: status=%d", ep.Name, resp.StatusCode))
			continue
		}
		var files []FileMetadata
		if err := json.NewDecoder(resp.Body).Decode(&files); err != nil {
			resp.Body.Close()
			errs = append(errs, fmt.Errorf("%s list parse: %w", ep.Name, err))
			continue
		}
		resp.Body.Close()
		return files, nil
	}

	return nil, fmt.Errorf("list failed: %w", errors.Join(errs...))
}

// DeleteFromNode deletes a file from every configured node.
func (c *OSNodeClient) DeleteFromNode(fileID string) error {
	endpoints, err := c.requireEndpoints()
	if err != nil {
		return err
	}

	var errs []error
	success := false
	for _, ep := range endpoints {
		if err := c.deleteFromEndpoint(ep, fileID); err != nil {
			errs = append(errs, fmt.Errorf("%s delete: %w", ep.Name, err))
			continue
		}
		success = true
	}

	if !success {
		return fmt.Errorf("delete failed: %w", errors.Join(errs...))
	}

	_, err = c.db.Exec(`DELETE FROM files WHERE id = ?`, fileID)
	return err
}

// storeFileReference stores minimal reference info in controller DB for routing
func (c *OSNodeClient) storeFileReference(metadata *FileMetadata) error {
	// Only store file_id, owner_id, and node_id for routing decisions
	// All other metadata lives in the OS node database
	stmt := `INSERT INTO files (id, owner_id, node_id, status, filename) 
		VALUES (?, ?, ?, 'active', '')
		ON CONFLICT(id) DO UPDATE SET node_id = excluded.node_id`

	_, err := c.db.Exec(stmt, metadata.ID, metadata.OwnerID, metadata.NodeID)
	return err
}

// GetMetadataFromNode retrieves metadata from the first node that responds.
func (c *OSNodeClient) GetMetadataFromNode(fileID string) (*FileMetadata, error) {
	endpoints, err := c.requireEndpoints()
	if err != nil {
		return nil, err
	}
	var errs []error
	for _, ep := range endpoints {
		metadata, err := c.getMetadataFromEndpoint(ep, fileID)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s metadata: %w", ep.Name, err))
			continue
		}
		return metadata, nil
	}
	return nil, fmt.Errorf("metadata lookup failed: %w", errors.Join(errs...))
}

// getNodeIDForFile gets which node has the file
func (c *OSNodeClient) getNodeIDForFile(fileID string) (string, error) {
	var nodeID string
	err := c.db.QueryRow(`SELECT node_id FROM files WHERE id = ?`, fileID).Scan(&nodeID)
	return nodeID, err
}

// HealthCheck ensures at least one node is reachable.
func (c *OSNodeClient) HealthCheck() error {
	endpoints, err := c.requireEndpoints()
	if err != nil {
		return err
	}
	var errs []error
	for _, ep := range endpoints {
		url := fmt.Sprintf("%s/health", ep.URL)
		resp, err := c.client.Get(url)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s health: %w", ep.Name, err))
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			errs = append(errs, fmt.Errorf("%s unhealthy: status=%d", ep.Name, resp.StatusCode))
			continue
		}
		return nil
	}
	return fmt.Errorf("health check failed: %w", errors.Join(errs...))
}

// CheckNodeHealth checks if a specific node is healthy
func (c *OSNodeClient) CheckNodeHealth(nodeURL string) bool {
	url := fmt.Sprintf("%s/health", strings.TrimRight(nodeURL, "/"))
	resp, err := c.client.Get(url)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func (c *OSNodeClient) getMetadataFromEndpoint(ep nodeEndpoint, fileID string) (*FileMetadata, error) {
	url := fmt.Sprintf("%s/api/metadata?file_id=%s", ep.URL, fileID)
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get metadata failed: status=%d", resp.StatusCode)
	}

	var metadata FileMetadata
	if err := json.NewDecoder(resp.Body).Decode(&metadata); err != nil {
		return nil, fmt.Errorf("parse metadata: %w", err)
	}

	return &metadata, nil
}

func (c *OSNodeClient) deleteFromEndpoint(ep nodeEndpoint, fileID string) error {
	url := fmt.Sprintf("%s/api/delete?file_id=%s", ep.URL, fileID)
	req, err := http.NewRequest("DELETE", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("delete failed: status=%d", resp.StatusCode)
	}

	return nil
}
