package models

import (
    "database/sql"
    "errors"
    "fmt"
    "net/url"
    "strings"

    "github.com/google/uuid"
)

// StorageNode represents a controller-managed storage endpoint.
type StorageNode struct {
    ID          string `json:"id"`
    Name        string `json:"name"`
    URL         string `json:"url,omitempty"`
    IPAddress   string `json:"ip_address,omitempty"`
    Port        *int   `json:"port,omitempty"`
    IsActive    bool   `json:"is_active"`
    LastActive  string `json:"last_active,omitempty"`
    QuotaMB     int64  `json:"quota_mb"`
    UsedMB      int64  `json:"used_mb"`
    AvailableMB int64  `json:"available_mb"`
    CreatedAt   string `json:"created_at"`
    UpdatedAt   string `json:"updated_at"`
}

// EffectiveURL resolves the HTTP base URL for the node.
func (n *StorageNode) EffectiveURL() (string, error) {
    if trimmed := strings.TrimSpace(n.URL); trimmed != "" {
        return strings.TrimRight(trimmed, "/"), nil
    }

    if strings.TrimSpace(n.IPAddress) != "" && n.Port != nil {
        addr := strings.TrimSpace(n.IPAddress)
        if strings.HasPrefix(addr, "http://") || strings.HasPrefix(addr, "https://") {
            addr = strings.TrimRight(addr, "/")
            return fmt.Sprintf("%s:%d", addr, *n.Port), nil
        }
        return fmt.Sprintf("http://%s:%d", addr, *n.Port), nil
    }

    return "", errors.New("storage node missing URL or IP configuration")
}

// CreateStorageNode inserts a new node definition.
func CreateStorageNode(db *sql.DB, node *StorageNode) (*StorageNode, error) {
    if node == nil {
        return nil, errors.New("node payload is required")
    }

    if err := validateStorageNodeInput(node); err != nil {
        return nil, err
    }

    if node.ID == "" {
        node.ID = uuid.New().String()
    }
    if !node.IsActive {
        node.IsActive = true
    }

    stmt := `INSERT INTO storage_nodes (id, name, url, ip_address, port, is_active)
             VALUES (?, ?, ?, ?, ?, ?)`
    if _, err := db.Exec(stmt, node.ID, node.Name, stringOrNil(node.URL), stringOrNil(node.IPAddress), portDriverValue(node.Port), node.IsActive); err != nil {
        return nil, fmt.Errorf("create storage node: %w", err)
    }

    return GetStorageNodeByID(db, node.ID)
}

// UpdateStorageNode updates an existing node definition.
func UpdateStorageNode(db *sql.DB, node *StorageNode) (*StorageNode, error) {
    if node == nil || strings.TrimSpace(node.ID) == "" {
        return nil, errors.New("node id is required")
    }
    if err := validateStorageNodeInput(node); err != nil {
        return nil, err
    }

    stmt := `UPDATE storage_nodes
             SET name = ?, url = ?, ip_address = ?, port = ?, is_active = ?, updated_at = CURRENT_TIMESTAMP
             WHERE id = ?`
    res, err := db.Exec(stmt, node.Name, stringOrNil(node.URL), stringOrNil(node.IPAddress), portDriverValue(node.Port), node.IsActive, node.ID)
    if err != nil {
        return nil, fmt.Errorf("update storage node: %w", err)
    }
    affected, _ := res.RowsAffected()
    if affected == 0 {
        return nil, sql.ErrNoRows
    }

    return GetStorageNodeByID(db, node.ID)
}

// UpdateStorageNodeInfo updates the storage metrics for a node.
func UpdateStorageNodeInfo(db *sql.DB, nodeID string, quotaMB, usedMB, availableMB int64) error {
    if strings.TrimSpace(nodeID) == "" {
        return errors.New("node id is required")
    }

    stmt := `UPDATE storage_nodes
             SET quota_mb = ?, used_mb = ?, available_mb = ?, last_active = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP
             WHERE id = ?`
    res, err := db.Exec(stmt, quotaMB, usedMB, availableMB, nodeID)
    if err != nil {
        return fmt.Errorf("update storage node info: %w", err)
    }
    affected, _ := res.RowsAffected()
    if affected == 0 {
        return sql.ErrNoRows
    }

    return nil
}

// ListNodesWithAvailableStorage returns active nodes with at least minMB available space, sorted by available space descending.
func ListNodesWithAvailableStorage(db *sql.DB, minMB int64) ([]*StorageNode, error) {
    rows, err := db.Query(`SELECT id, name, url, ip_address, port, is_active, last_active, quota_mb, used_mb, available_mb, created_at, updated_at 
                           FROM storage_nodes 
                           WHERE is_active = 1 AND available_mb >= ?
                           ORDER BY available_mb DESC`, minMB)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var nodes []*StorageNode
    for rows.Next() {
        node, err := scanStorageNode(rows)
        if err != nil {
            return nil, err
        }
        nodes = append(nodes, node)
    }
    return nodes, rows.Err()
}

// DeleteStorageNode removes a node definition.
func DeleteStorageNode(db *sql.DB, id string) error {
    if strings.TrimSpace(id) == "" {
        return errors.New("node id is required")
    }
    _, err := db.Exec(`DELETE FROM storage_nodes WHERE id = ?`, id)
    return err
}

// ListStorageNodes returns every stored node (active and inactive).
func ListStorageNodes(db *sql.DB) ([]*StorageNode, error) {
    rows, err := db.Query(`SELECT id, name, url, ip_address, port, is_active, last_active, quota_mb, used_mb, available_mb, created_at, updated_at FROM storage_nodes ORDER BY created_at ASC`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var nodes []*StorageNode
    for rows.Next() {
        node, err := scanStorageNode(rows)
        if err != nil {
            return nil, err
        }
        nodes = append(nodes, node)
    }
    return nodes, rows.Err()
}

// ListActiveStorageNodes returns only active nodes.
func ListActiveStorageNodes(db *sql.DB) ([]*StorageNode, error) {
    rows, err := db.Query(`SELECT id, name, url, ip_address, port, is_active, last_active, quota_mb, used_mb, available_mb, created_at, updated_at FROM storage_nodes WHERE is_active = 1 ORDER BY created_at ASC`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var nodes []*StorageNode
    for rows.Next() {
        node, err := scanStorageNode(rows)
        if err != nil {
            return nil, err
        }
        nodes = append(nodes, node)
    }
    return nodes, rows.Err()
}

// GetStorageNodeByID fetches a node by primary key.
func GetStorageNodeByID(db *sql.DB, id string) (*StorageNode, error) {
    row := db.QueryRow(`SELECT id, name, url, ip_address, port, is_active, last_active, quota_mb, used_mb, available_mb, created_at, updated_at FROM storage_nodes WHERE id = ?`, id)
    return scanStorageNode(row)
}

type rowScanner interface {
    Scan(dest ...any) error
}

func scanStorageNode(scanner rowScanner) (*StorageNode, error) {
    var (
        urlValue     sql.NullString
        ipValue      sql.NullString
        portValue    sql.NullInt64
        isActiveVal  sql.NullBool
        lastActive   sql.NullString
        quotaMB      sql.NullInt64
        usedMB       sql.NullInt64
        availableMB  sql.NullInt64
        created      sql.NullString
        updated      sql.NullString
        id           string
        name         string
    )

    if err := scanner.Scan(&id, &name, &urlValue, &ipValue, &portValue, &isActiveVal, &lastActive, &quotaMB, &usedMB, &availableMB, &created, &updated); err != nil {
        return nil, err
    }

    var portPtr *int
    if portValue.Valid {
        value := int(portValue.Int64)
        portPtr = &value
    }

    node := &StorageNode{
        ID:          id,
        Name:        name,
        URL:         strings.TrimSpace(urlValue.String),
        IPAddress:   strings.TrimSpace(ipValue.String),
        Port:        portPtr,
        IsActive:    isActiveVal.Valid && isActiveVal.Bool,
        LastActive:  lastActive.String,
        QuotaMB:     quotaMB.Int64,
        UsedMB:      usedMB.Int64,
        AvailableMB: availableMB.Int64,
        CreatedAt:   created.String,
        UpdatedAt:   updated.String,
    }

    return node, nil
}

func validateStorageNodeInput(node *StorageNode) error {
    node.Name = strings.TrimSpace(node.Name)
    if node.Name == "" {
        return errors.New("node name is required")
    }

    node.URL = normalizeURL(node.URL)
    node.IPAddress = strings.TrimSpace(node.IPAddress)

    hasURL := node.URL != ""
    hasIP := node.IPAddress != ""
    if !hasURL && !hasIP {
        return errors.New("provide either a URL or an IP address")
    }

    if hasIP && node.Port == nil {
        return errors.New("port is required when IP address is provided")
    }

    if node.Port != nil {
        if *node.Port <= 0 {
            return errors.New("port must be greater than zero")
        }
        if *node.Port > 65535 {
            return errors.New("port must be less than or equal to 65535")
        }
    }
    return nil
}

func normalizeURL(raw string) string {
    trimmed := strings.TrimSpace(raw)
    if trimmed == "" {
        return ""
    }

    if !strings.HasPrefix(trimmed, "http://") && !strings.HasPrefix(trimmed, "https://") {
        trimmed = "http://" + trimmed
    }

    parsed, err := url.Parse(trimmed)
    if err != nil || parsed.Host == "" {
        return ""
    }

    return strings.TrimRight(trimmed, "/")
}

func stringOrNil(value string) interface{} {
    trimmed := strings.TrimSpace(value)
    if trimmed == "" {
        return nil
    }
    return trimmed
}

func portDriverValue(port *int) interface{} {
    if port == nil {
        return nil
    }
    return *port
}
