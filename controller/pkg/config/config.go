package config

import (
    "errors"
    "fmt"
    "os"
    "strings"
    "time"

    "github.com/google/uuid"
    "gopkg.in/yaml.v3"
)

// Duration is a YAML-friendly wrapper around time.Duration.
type Duration struct {
    time.Duration
}

// NewDuration constructs a Duration from a time.Duration value.
func NewDuration(d time.Duration) Duration {
    return Duration{Duration: d}
}

// UnmarshalYAML implements yaml unmarshalling for duration strings.
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
    if value == nil {
        return errors.New("nil duration node")
    }
    var input string
    if err := value.Decode(&input); err != nil {
        return err
    }
    parsed, err := time.ParseDuration(strings.TrimSpace(input))
    if err != nil {
        return fmt.Errorf("parse duration %q: %w", input, err)
    }
    d.Duration = parsed
    return nil
}

// MarshalYAML renders durations using the canonical string representation.
func (d Duration) MarshalYAML() (interface{}, error) {
    return d.Duration.String(), nil
}

// Or returns the wrapped duration or the fallback when zero.
func (d Duration) Or(fallback time.Duration) time.Duration {
    if d.Duration == 0 {
        return fallback
    }
    return d.Duration
}

// Config describes the runtime behaviour of the storage controller.
type Config struct {
    Node      NodeConfig      `yaml:"node"`
    Heartbeat HeartbeatConfig `yaml:"heartbeat"`
    Logging   LoggingConfig   `yaml:"logging"`
}

// NodeConfig controls cluster identity and dqlite networking.
type NodeConfig struct {
    ID         string   `yaml:"id"`
    Listen     string   `yaml:"listen"`
    APIListen  string   `yaml:"api_listen"`
    DataDir    string   `yaml:"data_dir"`
    Region     string   `yaml:"region"`
    Bootstrap  bool     `yaml:"bootstrap"`
    Peers      []string `yaml:"peers"`
    Voters     int      `yaml:"voters"`
    StandBys   int      `yaml:"standbys"`
}

// HeartbeatConfig controls periodic node health reporting.
type HeartbeatConfig struct {
    Interval     Duration `yaml:"interval"`
    OfflineAfter Duration `yaml:"offline_after"`
}

// LoggingConfig controls structured logger verbosity.
type LoggingConfig struct {
    Level string `yaml:"level"`
}

// Default returns opinionated defaults that favour a single-node bootstrap.
func Default() Config {
    return Config{
        Node: NodeConfig{
            Listen:    "0.0.0.0:9001",
            APIListen: "0.0.0.0:8080",
            DataDir:   "/var/lib/storageos",
            Bootstrap: true,
            Voters:    3,
        },
        Heartbeat: HeartbeatConfig{
            Interval:     NewDuration(30 * time.Second),
            OfflineAfter: NewDuration(5 * time.Minute),
        },
        Logging: LoggingConfig{Level: "info"},
    }
}

// Load merges defaults with YAML from disk (if present).
func Load(path string) (Config, error) {
    cfg := Default()

    if path == "" {
        return cfg, nil
    }

    data, err := os.ReadFile(path)
    if err != nil {
        if errors.Is(err, os.ErrNotExist) {
            return finalize(cfg)
        }
        return cfg, fmt.Errorf("read config %s: %w", path, err)
    }
    if len(data) > 0 {
        if err := yaml.Unmarshal(data, &cfg); err != nil {
            return cfg, fmt.Errorf("parse config %s: %w", path, err)
        }
    }

    return finalize(cfg)
}

// ApplyDefaults re-applies missing defaults after unmarshalling.
func finalize(cfg Config) (Config, error) {
    if cfg.Node.Listen == "" {
        cfg.Node.Listen = "0.0.0.0:9001"
    }
    if cfg.Node.APIListen == "" {
        cfg.Node.APIListen = "0.0.0.0:8080"
    }
    if cfg.Node.DataDir == "" {
        cfg.Node.DataDir = "/var/lib/storageos"
    }
    if cfg.Node.Voters == 0 {
        cfg.Node.Voters = 3
    }
    if cfg.Heartbeat.Interval.Duration == 0 {
        cfg.Heartbeat.Interval = NewDuration(30 * time.Second)
    }
    if cfg.Heartbeat.OfflineAfter.Duration == 0 {
        cfg.Heartbeat.OfflineAfter = NewDuration(5 * time.Minute)
    }
    if cfg.Logging.Level == "" {
        cfg.Logging.Level = "info"
    }
    if cfg.Node.ID == "" {
        if host, err := os.Hostname(); err == nil {
            cfg.Node.ID = host
        }
    }
    if cfg.Node.ID == "" {
        cfg.Node.ID = uuid.NewString()
    }

    // Normalise peers by trimming whitespace and discarding empties.
    peers := make([]string, 0, len(cfg.Node.Peers))
    for _, peer := range cfg.Node.Peers {
        peer = strings.TrimSpace(peer)
        if peer != "" {
            peers = append(peers, peer)
        }
    }
    cfg.Node.Peers = peers

    return cfg, nil
}

// OverrideFromEnv applies optional environment overrides (no error on parse failure).
func OverrideFromEnv(cfg *Config) {
    if v := strings.TrimSpace(os.Getenv("STORAGEOS_NODE_ID")); v != "" {
        cfg.Node.ID = v
    }
    if v := strings.TrimSpace(os.Getenv("STORAGEOS_LISTEN")); v != "" {
        cfg.Node.Listen = v
    }
    if v := strings.TrimSpace(os.Getenv("STORAGEOS_API_LISTEN")); v != "" {
        cfg.Node.APIListen = v
    }
    if v := strings.TrimSpace(os.Getenv("STORAGEOS_DATA_DIR")); v != "" {
        cfg.Node.DataDir = v
    }
    if v := strings.TrimSpace(os.Getenv("STORAGEOS_BOOTSTRAP")); v != "" {
        cfg.Node.Bootstrap = strings.EqualFold(v, "true") || v == "1"
    }
    if v := strings.TrimSpace(os.Getenv("STORAGEOS_PEERS")); v != "" {
        cfg.Node.Peers = splitCSV(v)
    }
    if v := strings.TrimSpace(os.Getenv("STORAGEOS_LOG_LEVEL")); v != "" {
        cfg.Logging.Level = strings.ToLower(v)
    }
}

func splitCSV(raw string) []string {
    parts := strings.Split(raw, ",")
    out := make([]string, 0, len(parts))
    for _, part := range parts {
        part = strings.TrimSpace(part)
        if part != "" {
            out = append(out, part)
        }
    }
    return out
}
