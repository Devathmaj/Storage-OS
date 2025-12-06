package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"storageos/controller/api/browser/handlers"

	"github.com/canonical/go-dqlite/v3/client"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/rs/zerolog"
)

// Cluster is the subset of dqlite functionality the API relies on.
type Cluster interface {
	AddPeer(ctx context.Context, info client.NodeInfo) error
}

// Server wires HTTP handlers for cluster and metadata operations.
type Server struct {
	db      *sql.DB
	cluster Cluster
	log     zerolog.Logger
}

// New constructs a Server.
func New(db *sql.DB, cluster Cluster, log zerolog.Logger) *Server {
	// Initialize auth handlers with database connection
	if db != nil {
		handlers.InitAuthHandlers(db)
		handlers.InitFileHandlers(db)
		handlers.InitFolderHandlers(db)
	}
	return &Server{db: db, cluster: cluster, log: log}
}

// Router exposes the HTTP routes.
func (s *Server) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	// Removed timeout middleware for file/folder uploads

	// Add CORS middleware for browser uploads
	r.Use(corsMiddleware)

	r.Get("/v1/healthz", s.health)
	r.Get("/v1/cluster/nodes", s.listNodes)
	r.Post("/v1/cluster/join", s.joinCluster)

	// Auth endpoints (public)
	r.Post("/v1/auth/register", handlers.RegisterHandler)
	r.Post("/v1/auth/login", handlers.LoginHandler)
	r.Post("/v1/auth/logout", handlers.LogoutHandler)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(handlers.AuthMiddleware)

		// User profile
		r.Get("/v1/auth/me", handlers.MeHandler)

		// Browser file operations (protected) - proxied to OS storage nodes
		r.Post("/v1/browser/upload", handlers.UploadFileHandler)
		r.Post("/v1/browser/upload-folder", handlers.UploadFolderHandler)
		r.Get("/v1/browser/download", handlers.DownloadFileHandler)
		r.Get("/v1/browser/preview", handlers.PreviewFileHandler)
		r.Get("/v1/browser/download-folder", handlers.DownloadFolderHandler)
		r.Get("/v1/browser/files", handlers.ListFilesHandler)
		r.Delete("/v1/browser/delete", handlers.DeleteFileHandler)

		// Folder and metadata operations (controller-only, not propagated to OS)
		r.Post("/v1/browser/folder/create", handlers.CreateFolderHandler)
		r.Get("/v1/browser/folder/contents", handlers.ListContentsHandler)
		r.Delete("/v1/browser/item/delete", handlers.DeleteItemHandler)
	})

	// Storage node settings (explicit routes so both trailing/non-trailing work)
	r.With(handlers.AuthMiddleware).Get("/v1/settings/storage-nodes", handlers.ListStorageNodesHandler)
	r.With(handlers.AuthMiddleware).Get("/v1/settings/storage-nodes/", handlers.ListStorageNodesHandler)
	r.With(handlers.AuthMiddleware).Post("/v1/settings/storage-nodes", handlers.CreateStorageNodeHandler)
	r.With(handlers.AuthMiddleware).Post("/v1/settings/storage-nodes/", handlers.CreateStorageNodeHandler)
	r.With(handlers.AuthMiddleware).Put("/v1/settings/storage-nodes/{nodeID}", handlers.UpdateStorageNodeHandler)
	r.With(handlers.AuthMiddleware).Delete("/v1/settings/storage-nodes/{nodeID}", handlers.DeleteStorageNodeHandler)

	// Log not found / method not allowed to help debugging 404s
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		s.log.Info().Str("method", r.Method).Str("path", r.URL.Path).Msg("not found")
		http.NotFound(w, r)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		s.log.Info().Str("method", r.Method).Str("path", r.URL.Path).Msg("method not allowed")
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
	})

	// Debug: print registered routes using chi.Walk
	_ = chi.Walk(r, func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		s.log.Info().Str("method", method).Str("route", route).Msg("registered route")
		return nil
	})

	return r
}

// corsMiddleware adds CORS headers for browser requests
func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	respondJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) listNodes(w http.ResponseWriter, r *http.Request) {
	const stmt = `
SELECT node_id, hostname, region, available_space, load_factor, status, COALESCE(last_heartbeat, CURRENT_TIMESTAMP)
FROM nodes
ORDER BY node_id;
`
	rows, err := s.db.QueryContext(r.Context(), stmt)
	if err != nil {
		s.log.Error().Err(err).Msg("query nodes")
		respondError(w, http.StatusInternalServerError, "query failed")
		return
	}
	defer rows.Close()

	type node struct {
		NodeID         string  `json:"node_id"`
		Hostname       string  `json:"hostname"`
		Region         string  `json:"region"`
		AvailableSpace int64   `json:"available_space"`
		LoadFactor     float64 `json:"load_factor"`
		Status         string  `json:"status"`
		LastHeartbeat  string  `json:"last_heartbeat"`
	}

	payload := []node{}
	for rows.Next() {
		var entry node
		var ts sql.NullString
		if err := rows.Scan(&entry.NodeID, &entry.Hostname, &entry.Region, &entry.AvailableSpace, &entry.LoadFactor, &entry.Status, &ts); err != nil {
			s.log.Error().Err(err).Msg("scan node row")
			respondError(w, http.StatusInternalServerError, "scan failed")
			return
		}
		entry.LastHeartbeat = ts.String
		payload = append(payload, entry)
	}
	if err := rows.Err(); err != nil {
		s.log.Error().Err(err).Msg("iterate nodes")
		respondError(w, http.StatusInternalServerError, "iteration failed")
		return
	}

	respondJSON(w, http.StatusOK, payload)
}

func (s *Server) joinCluster(w http.ResponseWriter, r *http.Request) {
	if s.cluster == nil {
		respondError(w, http.StatusServiceUnavailable, "cluster management disabled")
		return
	}

	type request struct {
		Address string `json:"address"`
		ID      uint64 `json:"id"`
		Role    string `json:"role"`
	}

	var body request
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		respondError(w, http.StatusBadRequest, "invalid JSON payload")
		return
	}

	body.Address = strings.TrimSpace(body.Address)
	if body.Address == "" {
		respondError(w, http.StatusBadRequest, "address is required")
		return
	}

	node := client.NodeInfo{ID: body.ID, Address: body.Address}
	switch strings.ToLower(body.Role) {
	case "", "spare":
		node.Role = client.Spare
	case "standby", "stand-by":
		node.Role = client.StandBy
	case "voter":
		node.Role = client.Voter
	default:
		respondError(w, http.StatusBadRequest, "unsupported role")
		return
	}

	if err := s.cluster.AddPeer(r.Context(), node); err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			respondError(w, http.StatusGatewayTimeout, "cluster timeout")
			return
		}
		s.log.Error().Err(err).Str("address", node.Address).Msg("cluster join failed")
		respondError(w, http.StatusInternalServerError, "cluster join failed")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{"id": node.ID, "address": node.Address, "role": node.Role.String()})
}

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if payload == nil {
		return
	}
	_ = json.NewEncoder(w).Encode(payload)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
