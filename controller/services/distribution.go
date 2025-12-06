package services

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"sort"

	"storageos/controller/core/erasure"
	"storageos/controller/models"

	"github.com/rs/zerolog"
)

// DistributionService handles file distribution across storage nodes with RS encoding.
type DistributionService struct {
	db     *sql.DB
	logger zerolog.Logger
	client *http.Client
}

// DistributionOptions bundles constructor arguments.
type DistributionOptions struct {
	DB     *sql.DB
	Logger zerolog.Logger
}

// NewDistributionService creates a new distribution service.
func NewDistributionService(opts DistributionOptions) *DistributionService {
	return &DistributionService{
		db:     opts.DB,
		logger: opts.Logger,
		client: &http.Client{},
	}
}

// NodeSelection represents a node selected for storing a shard.
type NodeSelection struct {
	Node         *models.StorageNode
	PeerServer   *models.PeerServer // If set, shard should be stored on peer server
	ShardIndex   int
	ShardType    string // "data" or "parity"
}

// DistributionPlan represents the plan for distributing shards across nodes.
type DistributionPlan struct {
	Params           erasure.RSParams
	Nodes            []*NodeSelection
	TotalSize        int64
	ShardSize        int64
	CanProceed       bool
	Reason           string
	UsePeerServers   bool  // Whether some shards will be stored on peer servers
	PeerShardCount   int   // Number of shards to store on peer servers
}

// DistributedFile represents a file that has been distributed across nodes.
type DistributedFile struct {
	FileID         string
	OriginalSize   int64
	Checksum       string
	RSParams       erasure.RSParams
	Shards         []*models.FileShard
	SuccessCount   int
	FailedCount    int
	StorageUsed    int64
	PeerShardCount int // Number of shards stored on peer servers
}

// SelectNodesForDistribution selects optimal nodes for storing shards based on available storage.
func (s *DistributionService) SelectNodesForDistribution(ctx context.Context, fileSize int64) (*DistributionPlan, error) {
	// Get all active storage nodes with storage info
	nodes, err := models.ListActiveStorageNodes(s.db)
	if err != nil {
		return nil, fmt.Errorf("list active nodes: %w", err)
	}

	if len(nodes) == 0 {
		return &DistributionPlan{
			CanProceed: false,
			Reason:     "no active storage nodes available",
		}, nil
	}

	// Determine RS parameters based on node count
	params := erasure.DetermineRSParams(len(nodes))

	// Calculate shard size
	encoder, err := erasure.NewEncoder(params)
	if err != nil {
		return nil, fmt.Errorf("create encoder: %w", err)
	}
	shardSize := encoder.ShardSizeForFile(fileSize)
	totalRequired := encoder.TotalStorageRequired(fileSize)

	// Sort nodes by available storage (descending)
	sortedNodes := make([]*models.StorageNode, len(nodes))
	copy(sortedNodes, nodes)
	sort.Slice(sortedNodes, func(i, j int) bool {
		return sortedNodes[i].AvailableMB > sortedNodes[j].AvailableMB
	})

	// Filter nodes that have enough space for at least one shard
	shardSizeMB := (shardSize + 1024*1024 - 1) / (1024 * 1024) // Round up to MB
	eligibleNodes := make([]*models.StorageNode, 0)
	lowStorageNodes := make([]*models.StorageNode, 0)
	for _, node := range sortedNodes {
		if node.AvailableMB >= shardSizeMB {
			eligibleNodes = append(eligibleNodes, node)
		} else if node.AvailableMB > 0 {
			lowStorageNodes = append(lowStorageNodes, node)
		}
	}

	// Check if we have enough eligible nodes
	if len(eligibleNodes) < params.TotalShards {
		// If we don't have enough nodes, we might need to reduce RS parameters or fail
		if len(eligibleNodes) < 1 {
			return &DistributionPlan{
				Params:     params,
				CanProceed: false,
				Reason:     "no nodes with sufficient storage",
			}, nil
		}
		
		// Recalculate with available nodes
		params = erasure.DetermineRSParams(len(eligibleNodes))
		encoder, _ = erasure.NewEncoder(params)
		shardSize = encoder.ShardSizeForFile(fileSize)
		totalRequired = encoder.TotalStorageRequired(fileSize)
	}

	// Select nodes for each shard - distribute evenly across nodes
	selections := make([]*NodeSelection, params.TotalShards)
	for i := 0; i < params.TotalShards; i++ {
		nodeIndex := i % len(eligibleNodes)
		shardType := "data"
		if i >= params.DataShards {
			shardType = "parity"
		}
		
		selections[i] = &NodeSelection{
			Node:       eligibleNodes[nodeIndex],
			ShardIndex: i,
			ShardType:  shardType,
		}
	}

	plan := &DistributionPlan{
		Params:     params,
		Nodes:      selections,
		TotalSize:  totalRequired,
		ShardSize:  shardSize,
		CanProceed: true,
	}

	// Check if peer file sharing is enabled and we have low storage
	peerSettings, err := models.GetPeerFileSharingSettings(s.db)
	if err == nil && peerSettings.Enabled {
		// Check if we should offload some shards to peer servers
		totalAvailable := int64(0)
		for _, node := range eligibleNodes {
			totalAvailable += int64(node.AvailableMB) * 1024 * 1024
		}

		// If local storage is getting low (less than 20% available after this file), consider peer offload
		storageThreshold := float64(0.2)
		if float64(totalAvailable-totalRequired)/float64(totalAvailable) < storageThreshold {
			plan = s.considerPeerServerOffload(plan, params, shardSize, peerSettings.CompleteStorageEnabled)
		}
	}

	return plan, nil
}

// considerPeerServerOffload modifies the distribution plan to use peer servers for some shards
func (s *DistributionService) considerPeerServerOffload(plan *DistributionPlan, params erasure.RSParams, shardSize int64, completeStorageEnabled bool) *DistributionPlan {
	// Get enabled peer servers for outbound file sharing
	peerServers, err := models.GetEnabledOutboundPeersForSharing(s.db)
	if err != nil || len(peerServers) == 0 {
		return plan // No peer servers available
	}

	// Determine how many shards to offload
	shardsToOffload := 0
	if completeStorageEnabled {
		// Offload all shards
		shardsToOffload = params.TotalShards
	} else {
		// Only offload parity shards (copies) - up to 2
		shardsToOffload = min(2, params.ParityShards)
	}

	if shardsToOffload == 0 {
		return plan
	}

	// Find peer servers with enough storage
	shardSizeBytes := shardSize
	availablePeers := make([]*models.PeerServer, 0)
	for _, peer := range peerServers {
		// Check if peer has storage remaining (from last health check)
		// QuotaAllocated field is being used for storage_remaining in the query
		if peer.QuotaAllocated > shardSizeBytes {
			availablePeers = append(availablePeers, peer)
		}
	}

	if len(availablePeers) == 0 {
		s.logger.Info().Msg("No peer servers with sufficient storage for shard offload")
		return plan
	}

	// Assign peer servers to parity shards (or all shards if complete storage)
	peerIndex := 0
	offloadedCount := 0
	
	if completeStorageEnabled {
		// Replace all node selections with peer servers
		for i := range plan.Nodes {
			if offloadedCount >= shardsToOffload {
				break
			}
			plan.Nodes[i].PeerServer = availablePeers[peerIndex%len(availablePeers)]
			plan.Nodes[i].Node = nil // Will be decided by peer server
			peerIndex++
			offloadedCount++
		}
	} else {
		// Only replace parity shard selections
		for i := range plan.Nodes {
			if offloadedCount >= shardsToOffload {
				break
			}
			if plan.Nodes[i].ShardType == "parity" {
				plan.Nodes[i].PeerServer = availablePeers[peerIndex%len(availablePeers)]
				// Keep Node set - peer will decide actual storage node
				peerIndex++
				offloadedCount++
			}
		}
	}

	plan.UsePeerServers = offloadedCount > 0
	plan.PeerShardCount = offloadedCount

	s.logger.Info().
		Int("total_shards", params.TotalShards).
		Int("peer_shards", offloadedCount).
		Bool("complete_storage", completeStorageEnabled).
		Msg("Distribution plan includes peer server offload")

	return plan
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// DistributeFile encodes a file with RS and distributes shards to selected nodes.
func (s *DistributionService) DistributeFile(ctx context.Context, fileID string, data []byte, ownerID string, filename string) (*DistributedFile, error) {
	fileSize := int64(len(data))

	// Get distribution plan
	plan, err := s.SelectNodesForDistribution(ctx, fileSize)
	if err != nil {
		return nil, fmt.Errorf("select nodes: %w", err)
	}

	if !plan.CanProceed {
		return nil, fmt.Errorf("cannot distribute file: %s", plan.Reason)
	}

	// Create encoder and encode data
	encoder, err := erasure.NewEncoder(plan.Params)
	if err != nil {
		return nil, fmt.Errorf("create encoder: %w", err)
	}

	encoded, err := encoder.EncodeBytes(data)
	if err != nil {
		return nil, fmt.Errorf("encode data: %w", err)
	}

	result := &DistributedFile{
		FileID:       fileID,
		OriginalSize: fileSize,
		Checksum:     encoded.OriginalChksum,
		RSParams:     plan.Params,
		Shards:       make([]*models.FileShard, len(encoded.Shards)),
	}

	// Upload each shard to its assigned node or peer server
	for i, shard := range encoded.Shards {
		selection := plan.Nodes[i]
		
		// Create shard record
		shardRecord := &models.FileShard{
			FileID:     fileID,
			ShardIndex: shard.Index,
			ShardType:  selection.ShardType,
			ShardSize:  shard.Size,
			Checksum:   shard.Checksum,
			Status:     "active",
		}

		var uploadErr error
		var storagePath string

		if selection.PeerServer != nil {
			// Upload to peer server
			storagePath, uploadErr = s.uploadShardToPeerServer(ctx, selection.PeerServer, shard.Data, fileID, shard.Index, ownerID, filename, selection.ShardType)
			if uploadErr == nil {
				// Mark shard as stored on peer server
				shardRecord.NodeID = "peer:" + selection.PeerServer.ID
				// Note: peer_server_id should be set via a model function
				result.PeerShardCount++
			}
		} else if selection.Node != nil {
			// Upload to local storage node
			shardRecord.NodeID = selection.Node.ID
			storagePath, uploadErr = s.uploadShardToNode(ctx, selection.Node, shard.Data, fileID, shard.Index, ownerID, filename)
		} else {
			uploadErr = fmt.Errorf("no node or peer server assigned for shard %d", shard.Index)
		}

		if uploadErr != nil {
			s.logger.Error().
				Err(uploadErr).
				Int("shard_index", shard.Index).
				Msg("failed to upload shard")
			shardRecord.Status = "lost"
			shardRecord.NodeID = "unknown" // Need some value for DB
			result.FailedCount++
		} else {
			shardRecord.StoragePath = storagePath
			result.SuccessCount++
			result.StorageUsed += shard.Size
		}

		result.Shards[i] = shardRecord
	}

	// Save all shard records to database
	if err := models.CreateFileShardsInBatch(s.db, result.Shards); err != nil {
		return nil, fmt.Errorf("save shard records: %w", err)
	}

	// Check if we have enough shards for recovery
	if !encoder.CanRecover(result.SuccessCount) {
		return nil, fmt.Errorf("distribution failed: only %d of %d required shards uploaded", 
			result.SuccessCount, plan.Params.DataShards)
	}

	return result, nil
}

// uploadShardToPeerServer uploads a shard to a peer server for storage
func (s *DistributionService) uploadShardToPeerServer(ctx context.Context, peer *models.PeerServer, data []byte, fileID string, shardIndex int, ownerID, filename, shardType string) (string, error) {
	// Create mTLS client for peer communication
	client, err := s.createPeerClient(peer)
	if err != nil {
		return "", fmt.Errorf("create peer client: %w", err)
	}

	// Prepare the shard transfer request
	payload := map[string]interface{}{
		"file_id":       fileID,
		"owner_id":      ownerID,
		"filename":      filename,
		"shard_index":   shardIndex,
		"shard_type":    shardType,
		"shard_data":    data,
		"checksum":      "", // TODO: Calculate checksum
		"source_server": "", // TODO: Get our server URL
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/v1/peers/receive-file", peer.URL)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("peer returned %d: %s", resp.StatusCode, string(respBody))
	}

	var result struct {
		StoragePath string `json:"storage_path"`
		NodeID      string `json:"node_id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Sprintf("peer_%s_%s_shard_%d", peer.ID, fileID, shardIndex), nil
	}

	return result.StoragePath, nil
}

// createPeerClient creates an HTTP client with mTLS for peer communication
func (s *DistributionService) createPeerClient(peer *models.PeerServer) (*http.Client, error) {
	if peer.ClientCert == "" || peer.ClientKey == "" {
		// No mTLS configured, use regular HTTP client
		return s.client, nil
	}

	cert, err := tls.X509KeyPair([]byte(peer.ClientCert), []byte(peer.ClientKey))
	if err != nil {
		return nil, fmt.Errorf("load client certificate: %w", err)
	}

	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{cert},
	}

	if peer.ServerCert != "" {
		caCertPool := x509.NewCertPool()
		caCertPool.AppendCertsFromPEM([]byte(peer.ServerCert))
		tlsConfig.RootCAs = caCertPool
	} else {
		tlsConfig.InsecureSkipVerify = true
	}

	return &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}, nil
}

// uploadShardToNode uploads a shard to a storage node.
func (s *DistributionService) uploadShardToNode(ctx context.Context, node *models.StorageNode, data []byte, fileID string, shardIndex int, ownerID, filename string) (string, error) {
	baseURL, err := node.EffectiveURL()
	if err != nil {
		return "", fmt.Errorf("get node URL: %w", err)
	}

	url := fmt.Sprintf("%s/api/upload-shard", baseURL)

	// Create multipart form
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)

	// Add shard data
	shardFilename := fmt.Sprintf("%s_shard_%d", fileID, shardIndex)
	part, err := writer.CreateFormFile("shard", shardFilename)
	if err != nil {
		return "", fmt.Errorf("create form file: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return "", fmt.Errorf("write shard data: %w", err)
	}

	// Add metadata
	writer.WriteField("file_id", fileID)
	writer.WriteField("shard_index", fmt.Sprintf("%d", shardIndex))
	writer.WriteField("owner_id", ownerID)
	writer.WriteField("original_filename", filename)

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close writer: %w", err)
	}

	// Create request
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, &buf)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())

	// Send request
	resp, err := s.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", fmt.Errorf("upload failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to get storage path
	var result struct {
		StoragePath string `json:"storage_path"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		// If we can't parse the response, assume success but no path
		return shardFilename, nil
	}

	return result.StoragePath, nil
}

// RetrieveFile retrieves and reconstructs a file from its shards.
func (s *DistributionService) RetrieveFile(ctx context.Context, fileID string, originalSize int64, rsDataShards int) ([]byte, error) {
	// Get all shards for the file
	shardRecords, err := models.GetActiveFileShardsByFileID(s.db, fileID)
	if err != nil {
		return nil, fmt.Errorf("get shard records: %w", err)
	}

	if len(shardRecords) == 0 {
		return nil, fmt.Errorf("no shards found for file")
	}

	// Determine RS params from first shard set
	var params erasure.RSParams
	dataCount := 0
	parityCount := 0
	for _, sr := range shardRecords {
		if sr.ShardType == "data" {
			dataCount++
		} else {
			parityCount++
		}
	}
	if dataCount == 0 {
		dataCount = rsDataShards // Use provided value if can't determine
	}
	params = erasure.RSParams{
		DataShards:   dataCount,
		ParityShards: parityCount,
		TotalShards:  dataCount + parityCount,
		Enabled:      parityCount > 0,
	}

	encoder, err := erasure.NewEncoder(params)
	if err != nil {
		return nil, fmt.Errorf("create encoder: %w", err)
	}

	if !encoder.CanRecover(len(shardRecords)) {
		return nil, fmt.Errorf("insufficient shards for recovery: have %d, need %d", len(shardRecords), params.DataShards)
	}

	// Download shards from nodes
	shards := make([]*erasure.Shard, params.TotalShards)
	for _, record := range shardRecords {
		node, err := models.GetStorageNodeByID(s.db, record.NodeID)
		if err != nil {
			s.logger.Warn().Err(err).Str("node_id", record.NodeID).Msg("failed to get node")
			continue
		}

		data, err := s.downloadShardFromNode(ctx, node, record.StoragePath)
		if err != nil {
			s.logger.Warn().Err(err).
				Str("node_id", record.NodeID).
				Int("shard_index", record.ShardIndex).
				Msg("failed to download shard")
			continue
		}

		shards[record.ShardIndex] = &erasure.Shard{
			Index:    record.ShardIndex,
			Data:     data,
			Size:     record.ShardSize,
			Checksum: record.Checksum,
			IsParity: record.ShardType == "parity",
		}
	}

	// Reconstruct original data
	return encoder.Decode(shards, originalSize)
}

// downloadShardFromNode downloads a shard from a storage node.
func (s *DistributionService) downloadShardFromNode(ctx context.Context, node *models.StorageNode, storagePath string) ([]byte, error) {
	baseURL, err := node.EffectiveURL()
	if err != nil {
		return nil, fmt.Errorf("get node URL: %w", err)
	}

	url := fmt.Sprintf("%s/api/download-shard?path=%s", baseURL, storagePath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := s.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return nil, fmt.Errorf("download failed with status %d: %s", resp.StatusCode, string(body))
	}

	return io.ReadAll(resp.Body)
}

// GetDistributionStats returns statistics about file distribution across nodes.
type DistributionStats struct {
	TotalNodes      int              `json:"total_nodes"`
	ActiveNodes     int              `json:"active_nodes"`
	TotalFiles      int              `json:"total_files"`
	TotalShards     int              `json:"total_shards"`
	DegradedFiles   int              `json:"degraded_files"`
	RSParams        erasure.RSParams `json:"rs_params"`
	StorageOverhead float64          `json:"storage_overhead"`
}

// GetDistributionStats returns distribution statistics.
func (s *DistributionService) GetDistributionStats(ctx context.Context) (*DistributionStats, error) {
	nodes, err := models.ListStorageNodes(s.db)
	if err != nil {
		return nil, err
	}

	activeNodes, err := models.ListActiveStorageNodes(s.db)
	if err != nil {
		return nil, err
	}

	degradedFiles, err := models.GetFilesWithDegradedShards(s.db)
	if err != nil {
		return nil, err
	}

	// Get total shard count
	var totalShards int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM file_shards WHERE status = 'active'`).Scan(&totalShards)
	if err != nil {
		return nil, err
	}

	// Get unique file count with shards
	var totalFiles int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(DISTINCT file_id) FROM file_shards`).Scan(&totalFiles)
	if err != nil {
		return nil, err
	}

	params := erasure.DetermineRSParams(len(activeNodes))
	encoder, _ := erasure.NewEncoder(params)

	return &DistributionStats{
		TotalNodes:      len(nodes),
		ActiveNodes:     len(activeNodes),
		TotalFiles:      totalFiles,
		TotalShards:     totalShards,
		DegradedFiles:   len(degradedFiles),
		RSParams:        params,
		StorageOverhead: encoder.StorageOverhead(),
	}, nil
}
