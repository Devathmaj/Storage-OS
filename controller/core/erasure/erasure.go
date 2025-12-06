// Package erasure provides Reed-Solomon erasure coding for fault-tolerant file distribution.
//
// The RS parameters are dynamically determined based on the number of available storage nodes:
//   - node_count < 3: No erasure coding (single copy or replication)
//   - node_count == 3: RS(2,1) - 2 data shards, 1 parity shard
//   - node_count == 4: RS(3,1) - 3 data shards, 1 parity shard
//   - node_count == 5: RS(4,1) - 4 data shards, 1 parity shard
//   - node_count == 6: RS(4,2) - 4 data shards, 2 parity shards
//   - node_count == 7: RS(5,2) - 5 data shards, 2 parity shards
//   - node_count >= 8: RS(6,2) - 6 data shards, 2 parity shards
package erasure

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/klauspost/reedsolomon"
)

// RSParams holds the Reed-Solomon configuration.
type RSParams struct {
	DataShards   int  // Number of data shards (k)
	ParityShards int  // Number of parity shards (m)
	TotalShards  int  // Total shards (k + m)
	Enabled      bool // Whether RS encoding is enabled
}

// Shard represents a single shard of encoded data.
type Shard struct {
	Index    int    // Shard index (0 to TotalShards-1)
	Data     []byte // Shard data
	Size     int64  // Size of this shard in bytes
	Checksum string // SHA-256 checksum of shard data
	IsParity bool   // True if this is a parity shard
}

// EncodedFile represents a file that has been RS encoded.
type EncodedFile struct {
	OriginalSize   int64     // Original file size
	Shards         []*Shard  // All shards (data + parity)
	Params         RSParams  // RS parameters used
	OriginalChksum string    // SHA-256 of original file
}

// DetermineRSParams returns the appropriate RS parameters based on node count.
func DetermineRSParams(nodeCount int) RSParams {
	switch {
	case nodeCount < 3:
		return RSParams{
			DataShards:   1,
			ParityShards: 0,
			TotalShards:  1,
			Enabled:      false,
		}
	case nodeCount == 3:
		return RSParams{
			DataShards:   2,
			ParityShards: 1,
			TotalShards:  3,
			Enabled:      true,
		}
	case nodeCount == 4:
		return RSParams{
			DataShards:   3,
			ParityShards: 1,
			TotalShards:  4,
			Enabled:      true,
		}
	case nodeCount == 5:
		return RSParams{
			DataShards:   4,
			ParityShards: 1,
			TotalShards:  5,
			Enabled:      true,
		}
	case nodeCount == 6:
		return RSParams{
			DataShards:   4,
			ParityShards: 2,
			TotalShards:  6,
			Enabled:      true,
		}
	case nodeCount == 7:
		return RSParams{
			DataShards:   5,
			ParityShards: 2,
			TotalShards:  7,
			Enabled:      true,
		}
	default: // nodeCount >= 8
		return RSParams{
			DataShards:   6,
			ParityShards: 2,
			TotalShards:  8,
			Enabled:      true,
		}
	}
}

// Encoder handles Reed-Solomon encoding and decoding.
type Encoder struct {
	params RSParams
	enc    reedsolomon.Encoder
}

// NewEncoder creates a new RS encoder with the given parameters.
func NewEncoder(params RSParams) (*Encoder, error) {
	if !params.Enabled {
		return &Encoder{params: params}, nil
	}

	enc, err := reedsolomon.New(params.DataShards, params.ParityShards)
	if err != nil {
		return nil, fmt.Errorf("create RS encoder: %w", err)
	}

	return &Encoder{
		params: params,
		enc:    enc,
	}, nil
}

// NewEncoderForNodeCount creates an encoder with parameters appropriate for the given node count.
func NewEncoderForNodeCount(nodeCount int) (*Encoder, error) {
	params := DetermineRSParams(nodeCount)
	return NewEncoder(params)
}

// Params returns the RS parameters for this encoder.
func (e *Encoder) Params() RSParams {
	return e.params
}

// EncodeFile reads a file and encodes it into shards.
func (e *Encoder) EncodeFile(filePath string) (*EncodedFile, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer file.Close()

	return e.Encode(file)
}

// Encode reads from a reader and encodes the data into shards.
func (e *Encoder) Encode(r io.Reader) (*EncodedFile, error) {
	// Read all data
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read data: %w", err)
	}

	return e.EncodeBytes(data)
}

// EncodeBytes encodes byte data into shards.
func (e *Encoder) EncodeBytes(data []byte) (*EncodedFile, error) {
	// Calculate checksum of original data
	hash := sha256.Sum256(data)
	originalChecksum := hex.EncodeToString(hash[:])

	// If RS encoding is not enabled, return single shard
	if !e.params.Enabled {
		shardChecksum := sha256.Sum256(data)
		return &EncodedFile{
			OriginalSize: int64(len(data)),
			Shards: []*Shard{
				{
					Index:    0,
					Data:     data,
					Size:     int64(len(data)),
					Checksum: hex.EncodeToString(shardChecksum[:]),
					IsParity: false,
				},
			},
			Params:         e.params,
			OriginalChksum: originalChecksum,
		}, nil
	}

	// Split data into shards
	shards, err := e.enc.Split(data)
	if err != nil {
		return nil, fmt.Errorf("split data: %w", err)
	}

	// Encode parity
	if err := e.enc.Encode(shards); err != nil {
		return nil, fmt.Errorf("encode parity: %w", err)
	}

	// Create shard objects
	result := &EncodedFile{
		OriginalSize:   int64(len(data)),
		Shards:         make([]*Shard, len(shards)),
		Params:         e.params,
		OriginalChksum: originalChecksum,
	}

	for i, shardData := range shards {
		shardHash := sha256.Sum256(shardData)
		result.Shards[i] = &Shard{
			Index:    i,
			Data:     shardData,
			Size:     int64(len(shardData)),
			Checksum: hex.EncodeToString(shardHash[:]),
			IsParity: i >= e.params.DataShards,
		}
	}

	return result, nil
}

// Decode reconstructs the original data from shards.
// Some shards may be nil (missing), but at least DataShards must be present.
func (e *Encoder) Decode(shards []*Shard, originalSize int64) ([]byte, error) {
	if !e.params.Enabled {
		if len(shards) == 0 || shards[0] == nil {
			return nil, errors.New("no shard data available")
		}
		return shards[0].Data, nil
	}

	// Convert to raw shard format
	rawShards := make([][]byte, e.params.TotalShards)
	presentCount := 0
	for _, shard := range shards {
		if shard != nil && shard.Index < len(rawShards) {
			rawShards[shard.Index] = shard.Data
			presentCount++
		}
	}

	if presentCount < e.params.DataShards {
		return nil, fmt.Errorf("insufficient shards: have %d, need %d", presentCount, e.params.DataShards)
	}

	// Reconstruct missing shards if any
	if err := e.enc.Reconstruct(rawShards); err != nil {
		return nil, fmt.Errorf("reconstruct shards: %w", err)
	}

	// Verify shards
	ok, err := e.enc.Verify(rawShards)
	if err != nil {
		return nil, fmt.Errorf("verify shards: %w", err)
	}
	if !ok {
		return nil, errors.New("shard verification failed")
	}

	// Join shards to get original data
	var buf bytes.Buffer
	if err := e.enc.Join(&buf, rawShards, int(originalSize)); err != nil {
		return nil, fmt.Errorf("join shards: %w", err)
	}

	return buf.Bytes(), nil
}

// DecodeToWriter reconstructs data and writes to a writer.
func (e *Encoder) DecodeToWriter(w io.Writer, shards []*Shard, originalSize int64) error {
	data, err := e.Decode(shards, originalSize)
	if err != nil {
		return err
	}

	_, err = w.Write(data)
	return err
}

// VerifyShard checks if a shard's data matches its checksum.
func VerifyShard(shard *Shard) bool {
	if shard == nil || shard.Data == nil {
		return false
	}
	hash := sha256.Sum256(shard.Data)
	return hex.EncodeToString(hash[:]) == shard.Checksum
}

// CanRecover returns true if the file can be recovered with the available shards.
func (e *Encoder) CanRecover(availableShardCount int) bool {
	if !e.params.Enabled {
		return availableShardCount >= 1
	}
	return availableShardCount >= e.params.DataShards
}

// MaxTolerableLoss returns the maximum number of shards that can be lost while still allowing recovery.
func (e *Encoder) MaxTolerableLoss() int {
	if !e.params.Enabled {
		return 0
	}
	return e.params.ParityShards
}

// ShardSizeForFile calculates the size of each shard for a file of given size.
func (e *Encoder) ShardSizeForFile(fileSize int64) int64 {
	if !e.params.Enabled || e.params.DataShards == 0 {
		return fileSize
	}
	// Each data shard is roughly fileSize / dataShards, rounded up
	shardSize := (fileSize + int64(e.params.DataShards) - 1) / int64(e.params.DataShards)
	return shardSize
}

// TotalStorageRequired returns total storage needed for a file including parity.
func (e *Encoder) TotalStorageRequired(fileSize int64) int64 {
	if !e.params.Enabled {
		return fileSize
	}
	shardSize := e.ShardSizeForFile(fileSize)
	return shardSize * int64(e.params.TotalShards)
}

// StorageOverhead returns the storage overhead factor (total / original).
func (e *Encoder) StorageOverhead() float64 {
	if !e.params.Enabled || e.params.DataShards == 0 {
		return 1.0
	}
	return float64(e.params.TotalShards) / float64(e.params.DataShards)
}
