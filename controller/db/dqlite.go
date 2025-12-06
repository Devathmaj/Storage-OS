// +build dqlite

package db

import (
    "context"
    "database/sql"
    "errors"
    "fmt"
    "os"
    "path/filepath"
    "time"

    dqlite "github.com/canonical/go-dqlite/v3"
    "github.com/canonical/go-dqlite/v3/app"
    "github.com/canonical/go-dqlite/v3/client"
)

// Options controls the lifecycle of an embedded dqlite node.
type Options struct {
    DataDir       string
    ListenAddress string
    Peers         []string
    Bootstrap     bool
    Voters        int
    StandBys      int
    LogFunc       client.LogFunc
    ReadyTimeout  time.Duration
}

// Instance groups the dqlite app handle and the SQL connection.
type Instance struct {
    App *app.App
    DB  *sql.DB
}

// Start launches an embedded dqlite node and opens the metadata database.
func Start(ctx context.Context, opts Options) (*Instance, error) {
    if opts.DataDir == "" {
        return nil, errors.New("dqlite data directory is required")
    }
    if opts.ListenAddress == "" {
        return nil, errors.New("dqlite listen address is required")
    }
    if opts.Voters == 0 {
        opts.Voters = 3
    }
    if opts.StandBys < 0 {
        opts.StandBys = 0
    }
    if opts.ReadyTimeout == 0 {
        opts.ReadyTimeout = 30 * time.Second
    }
    if opts.LogFunc == nil {
        opts.LogFunc = client.DefaultLogFunc
    }

    if err := os.MkdirAll(opts.DataDir, 0o755); err != nil {
        return nil, fmt.Errorf("create data dir: %w", err)
    }

    appOpts := []app.Option{
        app.WithAddress(opts.ListenAddress),
        app.WithLogFunc(opts.LogFunc),
        app.WithVoters(opts.Voters),
        app.WithStandBys(opts.StandBys),
    }

    if !opts.Bootstrap && len(opts.Peers) > 0 {
        appOpts = append(appOpts, app.WithCluster(opts.Peers))
    }

    application, err := app.New(filepath.Clean(opts.DataDir), appOpts...)
    if err != nil {
        return nil, fmt.Errorf("start dqlite app: %w", err)
    }

    readyCtx, cancel := context.WithTimeout(ctx, opts.ReadyTimeout)
    defer cancel()
    if err := application.Ready(readyCtx); err != nil {
        application.Close()
        return nil, fmt.Errorf("wait for dqlite readiness: %w", err)
    }

    database, err := application.Open(ctx, "metadata")
    if err != nil {
        application.Close()
        return nil, fmt.Errorf("open metadata database: %w", err)
    }

	if _, err := database.ExecContext(ctx, "PRAGMA foreign_keys = ON"); err != nil {
		database.Close()
		application.Close()
		return nil, fmt.Errorf("enable foreign keys: %w", err)
	}

	// Run migrations for existing databases
	if err := RunMigrations(database); err != nil {
		database.Close()
		application.Close()
		return nil, fmt.Errorf("run migrations: %w", err)
	}

	return &Instance{App: application, DB: database}, nil
}// Close hands the node over to another peer (when possible) and releases resources.
func (i *Instance) Close(ctx context.Context) error {
    if i == nil {
        return nil
    }

    var firstErr error

    if ctx != nil {
        handoverCtx := ctx
        if _, ok := ctx.Deadline(); !ok {
            var cancel context.CancelFunc
            handoverCtx, cancel = context.WithTimeout(context.Background(), 15*time.Second)
            defer cancel()
        }
        if err := i.App.Handover(handoverCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
            firstErr = err
        }
    }

    if i.DB != nil {
        if err := i.DB.Close(); err != nil && firstErr == nil {
            firstErr = err
        }
    }

    if i.App != nil {
        if err := i.App.Close(); err != nil && firstErr == nil {
            firstErr = err
        }
    }

    return firstErr
}

// AddPeer registers a new node into the running cluster via the leader.
func (i *Instance) AddPeer(ctx context.Context, info client.NodeInfo) error {
    cli, err := i.App.FindLeader(ctx)
    if err != nil {
        return fmt.Errorf("connect leader: %w", err)
    }
    defer cli.Close()

    if info.ID == 0 {
        info.ID = dqlite.GenerateID(info.Address)
    }
    if info.Role == 0 {
        info.Role = client.Spare
    }

    if err := cli.Add(ctx, info); err != nil {
        return fmt.Errorf("raft add node: %w", err)
    }
    return nil
}
