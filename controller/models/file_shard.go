package models

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// FileShard represents a single shard of an erasure-coded file.
type FileShard struct {
	ID          string `json:"id"`
	FileID      string `json:"file_id"`
	ShardIndex  int    `json:"shard_index"`
	ShardType   string `json:"shard_type"` // "data" or "parity"
	NodeID      string `json:"node_id"`
	ShardSize   int64  `json:"shard_size"`
	Checksum    string `json:"checksum,omitempty"`
	StoragePath string `json:"storage_path,omitempty"`
	Status      string `json:"status"` // "active", "degraded", "lost", "recovering"
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// CreateFileShard inserts a new file shard record.
func CreateFileShard(db *sql.DB, shard *FileShard) (*FileShard, error) {
	if shard == nil {
		return nil, errors.New("shard payload is required")
	}

	if strings.TrimSpace(shard.FileID) == "" {
		return nil, errors.New("file_id is required")
	}
	if strings.TrimSpace(shard.NodeID) == "" {
		return nil, errors.New("node_id is required")
	}
	if shard.ShardType != "data" && shard.ShardType != "parity" {
		return nil, errors.New("shard_type must be 'data' or 'parity'")
	}

	if shard.ID == "" {
		shard.ID = uuid.New().String()
	}
	if shard.Status == "" {
		shard.Status = "active"
	}

	stmt := `INSERT INTO file_shards (id, file_id, shard_index, shard_type, node_id, shard_size, checksum, storage_path, status)
	         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := db.Exec(stmt, shard.ID, shard.FileID, shard.ShardIndex, shard.ShardType,
		shard.NodeID, shard.ShardSize, shardStringOrNil(shard.Checksum), shardStringOrNil(shard.StoragePath), shard.Status)
	if err != nil {
		return nil, fmt.Errorf("create file shard: %w", err)
	}

	return GetFileShardByID(db, shard.ID)
}

// CreateFileShardsInBatch inserts multiple file shards in a single transaction.
func CreateFileShardsInBatch(db *sql.DB, shards []*FileShard) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`INSERT INTO file_shards (id, file_id, shard_index, shard_type, node_id, shard_size, checksum, storage_path, status)
	                         VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return fmt.Errorf("prepare statement: %w", err)
	}
	defer stmt.Close()

	for _, shard := range shards {
		if shard.ID == "" {
			shard.ID = uuid.New().String()
		}
		if shard.Status == "" {
			shard.Status = "active"
		}

		_, err := stmt.Exec(shard.ID, shard.FileID, shard.ShardIndex, shard.ShardType,
			shard.NodeID, shard.ShardSize, shardStringOrNil(shard.Checksum), shardStringOrNil(shard.StoragePath), shard.Status)
		if err != nil {
			return fmt.Errorf("insert shard %d: %w", shard.ShardIndex, err)
		}
	}

	return tx.Commit()
}

// GetFileShardByID fetches a shard by its ID.
func GetFileShardByID(db *sql.DB, id string) (*FileShard, error) {
	row := db.QueryRow(`SELECT id, file_id, shard_index, shard_type, node_id, shard_size, checksum, storage_path, status, created_at, updated_at
	                    FROM file_shards WHERE id = ?`, id)
	return scanFileShard(row)
}

// GetFileShardsByFileID fetches all shards for a file.
func GetFileShardsByFileID(db *sql.DB, fileID string) ([]*FileShard, error) {
	rows, err := db.Query(`SELECT id, file_id, shard_index, shard_type, node_id, shard_size, checksum, storage_path, status, created_at, updated_at
	                       FROM file_shards WHERE file_id = ? ORDER BY shard_index ASC`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shards []*FileShard
	for rows.Next() {
		shard, err := scanFileShard(rows)
		if err != nil {
			return nil, err
		}
		shards = append(shards, shard)
	}
	return shards, rows.Err()
}

// GetActiveFileShardsByFileID fetches active shards for a file.
func GetActiveFileShardsByFileID(db *sql.DB, fileID string) ([]*FileShard, error) {
	rows, err := db.Query(`SELECT id, file_id, shard_index, shard_type, node_id, shard_size, checksum, storage_path, status, created_at, updated_at
	                       FROM file_shards WHERE file_id = ? AND status = 'active' ORDER BY shard_index ASC`, fileID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shards []*FileShard
	for rows.Next() {
		shard, err := scanFileShard(rows)
		if err != nil {
			return nil, err
		}
		shards = append(shards, shard)
	}
	return shards, rows.Err()
}

// GetFileShardsByNodeID fetches all shards stored on a node.
func GetFileShardsByNodeID(db *sql.DB, nodeID string) ([]*FileShard, error) {
	rows, err := db.Query(`SELECT id, file_id, shard_index, shard_type, node_id, shard_size, checksum, storage_path, status, created_at, updated_at
	                       FROM file_shards WHERE node_id = ? ORDER BY created_at ASC`, nodeID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var shards []*FileShard
	for rows.Next() {
		shard, err := scanFileShard(rows)
		if err != nil {
			return nil, err
		}
		shards = append(shards, shard)
	}
	return shards, rows.Err()
}

// UpdateShardStatus updates the status of a shard.
func UpdateShardStatus(db *sql.DB, shardID, status string) error {
	if status != "active" && status != "degraded" && status != "lost" && status != "recovering" {
		return errors.New("invalid status")
	}

	stmt := `UPDATE file_shards SET status = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	res, err := db.Exec(stmt, status, shardID)
	if err != nil {
		return fmt.Errorf("update shard status: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// UpdateShardStoragePath updates the storage path of a shard (after upload to node).
func UpdateShardStoragePath(db *sql.DB, shardID, storagePath string) error {
	stmt := `UPDATE file_shards SET storage_path = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	res, err := db.Exec(stmt, storagePath, shardID)
	if err != nil {
		return fmt.Errorf("update shard storage path: %w", err)
	}
	affected, _ := res.RowsAffected()
	if affected == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// DeleteFileShardsByFileID deletes all shards for a file.
func DeleteFileShardsByFileID(db *sql.DB, fileID string) error {
	_, err := db.Exec(`DELETE FROM file_shards WHERE file_id = ?`, fileID)
	return err
}

// MarkLostShardsOnNode marks all shards on a node as lost.
func MarkLostShardsOnNode(db *sql.DB, nodeID string) error {
	_, err := db.Exec(`UPDATE file_shards SET status = 'lost', updated_at = CURRENT_TIMESTAMP 
	                   WHERE node_id = ? AND status = 'active'`, nodeID)
	return err
}

// CountActiveShardsForFile counts active shards for a file.
func CountActiveShardsForFile(db *sql.DB, fileID string) (int, error) {
	var count int
	err := db.QueryRow(`SELECT COUNT(*) FROM file_shards WHERE file_id = ? AND status = 'active'`, fileID).Scan(&count)
	return count, err
}

// GetFilesWithDegradedShards returns file IDs that have lost shards and need recovery.
func GetFilesWithDegradedShards(db *sql.DB) ([]string, error) {
	rows, err := db.Query(`SELECT DISTINCT file_id FROM file_shards WHERE status IN ('lost', 'degraded')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var fileIDs []string
	for rows.Next() {
		var fileID string
		if err := rows.Scan(&fileID); err != nil {
			return nil, err
		}
		fileIDs = append(fileIDs, fileID)
	}
	return fileIDs, rows.Err()
}

// ShardDistribution returns a summary of shard distribution for a file.
type ShardDistribution struct {
	FileID          string                 `json:"file_id"`
	TotalShards     int                    `json:"total_shards"`
	DataShards      int                    `json:"data_shards"`
	ParityShards    int                    `json:"parity_shards"`
	ActiveShards    int                    `json:"active_shards"`
	LostShards      int                    `json:"lost_shards"`
	ShardsByNode    map[string]int         `json:"shards_by_node"`
	CanRecover      bool                   `json:"can_recover"`
}

// GetShardDistribution returns distribution information for a file's shards.
func GetShardDistribution(db *sql.DB, fileID string, requiredDataShards int) (*ShardDistribution, error) {
	shards, err := GetFileShardsByFileID(db, fileID)
	if err != nil {
		return nil, err
	}

	dist := &ShardDistribution{
		FileID:       fileID,
		TotalShards:  len(shards),
		ShardsByNode: make(map[string]int),
	}

	for _, shard := range shards {
		if shard.ShardType == "data" {
			dist.DataShards++
		} else {
			dist.ParityShards++
		}

		if shard.Status == "active" {
			dist.ActiveShards++
		} else if shard.Status == "lost" {
			dist.LostShards++
		}

		dist.ShardsByNode[shard.NodeID]++
	}

	dist.CanRecover = dist.ActiveShards >= requiredDataShards

	return dist, nil
}

func scanFileShard(scanner rowScanner) (*FileShard, error) {
	var (
		id          string
		fileID      string
		shardIndex  int
		shardType   string
		nodeID      string
		shardSize   int64
		checksum    sql.NullString
		storagePath sql.NullString
		status      string
		createdAt   sql.NullString
		updatedAt   sql.NullString
	)

	if err := scanner.Scan(&id, &fileID, &shardIndex, &shardType, &nodeID, &shardSize, &checksum, &storagePath, &status, &createdAt, &updatedAt); err != nil {
		return nil, err
	}

	return &FileShard{
		ID:          id,
		FileID:      fileID,
		ShardIndex:  shardIndex,
		ShardType:   shardType,
		NodeID:      nodeID,
		ShardSize:   shardSize,
		Checksum:    checksum.String,
		StoragePath: storagePath.String,
		Status:      status,
		CreatedAt:   createdAt.String,
		UpdatedAt:   updatedAt.String,
	}, nil
}

// shardStringOrNil is a helper to convert empty strings to nil for SQL.
func shardStringOrNil(value string) interface{} {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return nil
	}
	return trimmed
}

// Calculate total storage used by a file's shards.
func GetTotalShardStorageForFile(db *sql.DB, fileID string) (int64, error) {
	var total sql.NullInt64
	err := db.QueryRow(`SELECT SUM(shard_size) FROM file_shards WHERE file_id = ?`, fileID).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total.Int64, nil
}

// GetNodeStorageUsedByShards calculates total storage used by shards on a node.
func GetNodeStorageUsedByShards(db *sql.DB, nodeID string) (int64, error) {
	var total sql.NullInt64
	err := db.QueryRow(`SELECT SUM(shard_size) FROM file_shards WHERE node_id = ? AND status = 'active'`, nodeID).Scan(&total)
	if err != nil {
		return 0, err
	}
	return total.Int64, nil
}

// Helper function for current time formatting
func currentTimestamp() string {
	return time.Now().UTC().Format("2006-01-02 15:04:05")
}
