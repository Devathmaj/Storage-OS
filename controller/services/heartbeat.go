package services

import (
    "context"
    "database/sql"
    "fmt"
    "time"

    "github.com/rs/zerolog"
    "golang.org/x/sys/unix"
)

// HeartbeatService periodically records the local node status.
type HeartbeatService struct {
    db            *sql.DB
    nodeID        string
    hostname      string
    region        string
    dataPath      string
    interval      time.Duration
    offlineAfter  time.Duration
    logger        zerolog.Logger
}

// HeartbeatOptions bundles constructor arguments.
type HeartbeatOptions struct {
    DB           *sql.DB
    NodeID       string
    Hostname     string
    Region       string
    DataPath     string
    Interval     time.Duration
    OfflineAfter time.Duration
    Logger       zerolog.Logger
}

// NewHeartbeatService creates a ready to run heartbeat loop.
func NewHeartbeatService(opts HeartbeatOptions) *HeartbeatService {
    return &HeartbeatService{
        db:           opts.DB,
        nodeID:       opts.NodeID,
        hostname:     opts.Hostname,
        region:       opts.Region,
        dataPath:     opts.DataPath,
        interval:     opts.Interval,
        offlineAfter: opts.OfflineAfter,
        logger:       opts.Logger,
    }
}

// Run blocks, emitting heartbeats until the context is cancelled.
func (s *HeartbeatService) Run(ctx context.Context) {
    if s.interval <= 0 {
        s.interval = 30 * time.Second
    }
    if err := s.tick(ctx); err != nil {
        s.logger.Warn().Err(err).Msg("initial heartbeat failed")
    }

    ticker := time.NewTicker(s.interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            if err := s.markDraining(context.Background()); err != nil {
                s.logger.Warn().Err(err).Msg("mark draining failed")
            }
            return
        case <-ticker.C:
            if err := s.tick(ctx); err != nil {
                s.logger.Warn().Err(err).Msg("heartbeat tick failed")
            }
        }
    }
}

func (s *HeartbeatService) tick(ctx context.Context) error {
    free, load, err := s.diskStats()
    if err != nil {
        s.logger.Debug().Err(err).Msg("disk stats lookup failed")
    }

    query := `
INSERT INTO nodes (node_id, hostname, region, available_space, load_factor, status, last_heartbeat)
VALUES (?, ?, ?, ?, ?, 'healthy', CURRENT_TIMESTAMP)
ON CONFLICT(node_id)
DO UPDATE SET
    hostname = excluded.hostname,
    region = excluded.region,
    available_space = excluded.available_space,
    load_factor = excluded.load_factor,
    status = 'healthy',
    last_heartbeat = CURRENT_TIMESTAMP;
`

    if _, err := s.db.ExecContext(ctx, query, s.nodeID, s.hostname, s.region, free, load); err != nil {
        return fmt.Errorf("upsert heartbeat: %w", err)
    }

    if s.offlineAfter > 0 {
        horizon := fmt.Sprintf("-%d seconds", int(s.offlineAfter.Round(time.Second).Seconds()))
        purge := `
UPDATE nodes
SET status = 'offline'
WHERE node_id <> ?
  AND last_heartbeat < DATETIME('now', ?);
`
        if _, err := s.db.ExecContext(ctx, purge, s.nodeID, horizon); err != nil {
            s.logger.Warn().Err(err).Msg("mark offline failed")
        }
    }

    return nil
}

func (s *HeartbeatService) markDraining(ctx context.Context) error {
    _, err := s.db.ExecContext(ctx, `
UPDATE nodes
SET status = 'draining', last_heartbeat = CURRENT_TIMESTAMP
WHERE node_id = ?;
`, s.nodeID)
    return err
}

func (s *HeartbeatService) diskStats() (int64, float64, error) {
    path := s.dataPath
    if path == "" {
        path = "."
    }

    var stat unix.Statfs_t
    if err := unix.Statfs(path, &stat); err != nil {
        return 0, 0, err
    }

    free := int64(stat.Bavail) * int64(stat.Bsize)
    total := int64(stat.Blocks) * int64(stat.Bsize)
    var load float64
    if total > 0 {
        used := float64(total-free) / float64(total)
        if used < 0 {
            used = 0
        }
        if used > 1 {
            used = 1
        }
        load = used
    }

    return free, load, nil
}
