package handlers

import (
    "database/sql"
    "encoding/json"
    "log"
    "net/http"

    "github.com/go-chi/chi/v5"

    "storageos/controller/models"
)

type storageNodeRequest struct {
    Name      string `json:"name"`
    URL       string `json:"url"`
    IPAddress string `json:"ip_address"`
    Port      *int   `json:"port"`
    IsActive  *bool  `json:"is_active"`
}

// ListStorageNodesHandler returns all configured storage nodes.
func ListStorageNodesHandler(w http.ResponseWriter, r *http.Request) {
    log.Printf("ListStorageNodesHandler called, db=%v", db)
    if db == nil {
        log.Printf("ERROR: db is nil in ListStorageNodesHandler")
        writeError(w, http.StatusInternalServerError, "database not initialized")
        return
    }
    nodes, err := models.ListStorageNodes(db)
    if err != nil {
        log.Printf("ERROR: failed to list storage nodes: %v", err)
        writeError(w, http.StatusInternalServerError, "failed to list storage nodes")
        return
    }
    log.Printf("Successfully listed %d storage nodes", len(nodes))
    writeJSON(w, http.StatusOK, map[string]interface{}{"nodes": nodes, "count": len(nodes)})
}

// CreateStorageNodeHandler creates a new storage node definition.
func CreateStorageNodeHandler(w http.ResponseWriter, r *http.Request) {
    var payload storageNodeRequest
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        writeError(w, http.StatusBadRequest, "invalid JSON payload")
        return
    }
    log.Printf("CreateStorageNodeHandler called with payload: name=%s, url=%s, ip=%s, port=%v", payload.Name, payload.URL, payload.IPAddress, payload.Port)

    node := &models.StorageNode{
        Name:      payload.Name,
        URL:       payload.URL,
        IPAddress: payload.IPAddress,
        Port:      payload.Port,
        IsActive:  payload.IsActive == nil || *payload.IsActive,
    }

    created, err := models.CreateStorageNode(db, node)
    if err != nil {
        log.Printf("CreateStorageNodeHandler: failed to create node: %v", err)
        writeModelError(w, err)
        return
    }

    refreshNodeClient()
    log.Printf("CreateStorageNodeHandler: created node: id=%s, name=%s, url=%s", created.ID, created.Name, created.URL)
    writeJSON(w, http.StatusCreated, created)
}

// UpdateStorageNodeHandler updates an existing node definition.
func UpdateStorageNodeHandler(w http.ResponseWriter, r *http.Request) {
    nodeID := chi.URLParam(r, "nodeID")
    if nodeID == "" {
        writeError(w, http.StatusBadRequest, "node id is required")
        return
    }

    var payload storageNodeRequest
    if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
        writeError(w, http.StatusBadRequest, "invalid JSON payload")
        return
    }

    existing, err := models.GetStorageNodeByID(db, nodeID)
    if err != nil {
        if err == sql.ErrNoRows {
            writeError(w, http.StatusNotFound, "node not found")
        } else {
            writeError(w, http.StatusInternalServerError, "failed to load node")
        }
        return
    }

    node := &models.StorageNode{
        ID:        nodeID,
        Name:      payload.Name,
        URL:       payload.URL,
        IPAddress: payload.IPAddress,
        Port:      payload.Port,
        IsActive:  existing.IsActive,
    }
    if payload.IsActive != nil {
        node.IsActive = *payload.IsActive
    }

    updated, err := models.UpdateStorageNode(db, node)
    if err != nil {
        writeModelError(w, err)
        return
    }

    refreshNodeClient()
    writeJSON(w, http.StatusOK, updated)
}

// DeleteStorageNodeHandler removes a storage node definition.
func DeleteStorageNodeHandler(w http.ResponseWriter, r *http.Request) {
    nodeID := chi.URLParam(r, "nodeID")
    if nodeID == "" {
        writeError(w, http.StatusBadRequest, "node id is required")
        return
    }

    if err := models.DeleteStorageNode(db, nodeID); err != nil {
        writeError(w, http.StatusInternalServerError, "failed to delete node")
        return
    }

    refreshNodeClient()
    w.WriteHeader(http.StatusNoContent)
}

func writeModelError(w http.ResponseWriter, err error) {
    switch err {
    case sql.ErrNoRows:
        writeError(w, http.StatusNotFound, "node not found")
    default:
        writeError(w, http.StatusBadRequest, err.Error())
    }
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(status)
    if payload != nil {
        _ = json.NewEncoder(w).Encode(payload)
    }
}

func writeError(w http.ResponseWriter, status int, message string) {
    writeJSON(w, status, map[string]string{"error": message})
}

func refreshNodeClient() {
    client := GetOSNodeClient()
    if client == nil {
        log.Printf("refreshNodeClient: osNodeClient is nil")
        return
    }
    log.Printf("Refreshing storage node client...")
    if err := client.Reload(); err != nil {
        // log only; handlers shouldn't fail after DB mutation succeeds
        log.Printf("warning: failed to reload storage nodes: %v", err)
        return
    }
    log.Printf("Storage nodes reloaded successfully (count: %d)", client.NodeCount())
}
