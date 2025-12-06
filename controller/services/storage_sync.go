package services

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"storageos/controller/models"

	"github.com/rs/zerolog"
)

// StorageSyncService periodically syncs storage info from all active storage nodes.
type StorageSyncService struct {
	db       *sql.DB
	interval time.Duration
	timeout  time.Duration
	logger   zerolog.Logger
	client   *http.Client
}

// StorageSyncOptions bundles constructor arguments.
type StorageSyncOptions struct {
	DB       *sql.DB
	Interval time.Duration
	Timeout  time.Duration
	Logger   zerolog.Logger
}

// StorageInfoResponse matches the response from /api/storage-info endpoint.
type StorageInfoResponse struct {
	QuotaMB     int64  `json:"quota_mb"`
	UsedMB      int64  `json:"used_mb"`
	AvailableMB int64  `json:"available_mb"`
	DiskTotalMB int64  `json:"disk_total_mb"`
	NodeID      string `json:"node_id"`
	Status      string `json:"status"`
}

// NewStorageSyncService creates a storage sync service.
func NewStorageSyncService(opts StorageSyncOptions) *StorageSyncService {
	if opts.Interval <= 0 {
		opts.Interval = 60 * time.Second
	}
	if opts.Timeout <= 0 {
		opts.Timeout = 10 * time.Second
	}

	return &StorageSyncService{
		db:       opts.DB,
		interval: opts.Interval,
		timeout:  opts.Timeout,
		logger:   opts.Logger,
		client: &http.Client{
			Timeout: opts.Timeout,
		},
	}
}

// Run blocks, syncing storage info until the context is cancelled.
func (s *StorageSyncService) Run(ctx context.Context) {
	// Initial sync
	if err := s.syncAll(ctx); err != nil {
		s.logger.Warn().Err(err).Msg("initial storage sync failed")
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.syncAll(ctx); err != nil {
				s.logger.Warn().Err(err).Msg("storage sync tick failed")
			}
		}
	}
}

// syncAll queries all active storage nodes for their storage info.
func (s *StorageSyncService) syncAll(ctx context.Context) error {
	nodes, err := models.ListActiveStorageNodes(s.db)
	if err != nil {
		return fmt.Errorf("list active nodes: %w", err)
	}

	for _, node := range nodes {
		if err := s.syncNode(ctx, node); err != nil {
			s.logger.Warn().
				Str("node_id", node.ID).
				Str("node_name", node.Name).
				Err(err).
				Msg("failed to sync storage info for node")
			// Mark node as potentially offline if we can't reach it
			continue
		}
	}

	return nil
}

// syncNode queries a single storage node for its storage info.
func (s *StorageSyncService) syncNode(ctx context.Context, node *models.StorageNode) error {
	baseURL, err := node.EffectiveURL()
	if err != nil {
		return fmt.Errorf("get node URL: %w", err)
	}

	url := fmt.Sprintf("%s/api/storage-info", baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var info StorageInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	// Update the storage node with the info
	if err := models.UpdateStorageNodeInfo(s.db, node.ID, info.QuotaMB, info.UsedMB, info.AvailableMB); err != nil {
		return fmt.Errorf("update storage info: %w", err)
	}

	s.logger.Debug().
		Str("node_id", node.ID).
		Int64("quota_mb", info.QuotaMB).
		Int64("used_mb", info.UsedMB).
		Int64("available_mb", info.AvailableMB).
		Str("status", info.Status).
		Msg("synced storage info")

	return nil
}

// SyncNode syncs storage info for a single node by ID (for on-demand sync).
func (s *StorageSyncService) SyncNode(ctx context.Context, nodeID string) error {
	node, err := models.GetStorageNodeByID(s.db, nodeID)
	if err != nil {
		return fmt.Errorf("get node: %w", err)
	}
	return s.syncNode(ctx, node)
}

// GetNodeStorageInfo fetches storage info from a node without updating the DB.
func (s *StorageSyncService) GetNodeStorageInfo(ctx context.Context, node *models.StorageNode) (*StorageInfoResponse, error) {
	baseURL, err := node.EffectiveURL()
	if err != nil {
		return nil, fmt.Errorf("get node URL: %w", err)
	}

	url := fmt.Sprintf("%s/api/storage-info", baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var info StorageInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &info, nil
}
