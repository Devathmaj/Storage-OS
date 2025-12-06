package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"storageos/controller/api/browser/handlers"
	"storageos/controller/db"
	services "storageos/controller/services/browser"
	"storageos/controller/services/healthcheck"
	rootservices "storageos/controller/services"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
)

func main() {
	// Load environment variables from .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
	}

	// Setup basic logging
	logger := zerolog.New(os.Stdout).With().Timestamp().Logger()

	logger.Info().Msg("Starting StorageOS controller with authentication...")

	// Initialize SQLite database
	database, err := db.InitSQLite("./storageos.db")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize database: %v\n", err)
		os.Exit(1)
	}
	defer database.Close()

	logger.Info().Msg("✓ Database initialized successfully")

	// Get server configuration from environment
	serverName := os.Getenv("SERVER_NAME")
	if serverName == "" {
		serverName = "StorageOS Controller"
	}
	serverURL := os.Getenv("SERVER_URL")
	if serverURL == "" {
		serverURL = "http://localhost:8081"
	}

	logger.Info().Str("name", serverName).Str("url", serverURL).Msg("✓ Server configuration loaded")

	// Initialize auth handlers
	handlers.InitAuthHandlers(database)
	
	// Initialize file handlers (for file upload service)
	handlers.InitFileHandlers(database)
	
	// Initialize folder handlers
	handlers.InitFolderHandlers(database)
	
	// Initialize file service with shared OS node client from handlers
	services.InitFileService(database, handlers.GetOSNodeClient)

	// Initialize peer server handlers with server config
	handlers.InitPeerServerHandlers(database, serverName, serverURL)

	// Initialize provisioning handlers (OS node enrollment with mTLS)
	caDir := os.Getenv("CA_DIR")
	if caDir == "" {
		caDir = "./certs"
	}
	if err := handlers.InitProvisioningHandlers(database, caDir); err != nil {
		logger.Error().Err(err).Msg("Failed to initialize provisioning handlers")
		os.Exit(1)
	}
	logger.Info().Msg("✓ Certificate Authority initialized")

	// Initialize health monitoring service
	healthMonitor := healthcheck.NewMonitor(database, 30*time.Second)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go healthMonitor.Start(ctx)

	logger.Info().Msg("✓ Health monitoring started (30s interval)")

	// Initialize storage sync service for storage nodes
	storageSync := rootservices.NewStorageSyncService(rootservices.StorageSyncOptions{
		DB:       database,
		Interval: 60 * time.Second,
		Timeout:  10 * time.Second,
		Logger:   logger,
	})
	go storageSync.Run(ctx)

	logger.Info().Msg("✓ Storage sync service started (60s interval)")

	// Initialize trash cleanup service (30-day retention)
	trashCleanup := rootservices.NewTrashCleanupService(database)
	trashCleanup.Start()
	defer trashCleanup.Stop()

	logger.Info().Msg("✓ Trash cleanup service started (30-day retention)")

	// Setup router
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	// Log all incoming requests (method & path) for debugging
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			log.Printf("Incoming request: %s %s", r.Method, r.URL.Path)
			next.ServeHTTP(w, r)
		})
	})
	r.Use(corsMiddleware)

	// Health check
	r.Get("/v1/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	})

	// System initialization endpoints (public)
	r.Get("/v1/system/init", handlers.CheckSystemInitHandler)
	r.Post("/v1/system/init-admin", handlers.CreateAdminUserHandler)

	// Public auth endpoints
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
		
		// Folder operations
		r.Get("/v1/browser/folder/contents", handlers.ListContentsHandler)
		r.Post("/v1/browser/folder", handlers.CreateFolderHandler)
		r.Delete("/v1/browser/folder", handlers.DeleteItemHandler)
		
		// Trash operations
		r.Get("/v1/browser/trash", handlers.ListTrashHandler)
		r.Post("/v1/browser/trash", handlers.MoveToTrashHandler)
		r.Post("/v1/browser/trash/restore", handlers.RestoreFromTrashHandler)
		r.Delete("/v1/browser/trash", handlers.PermanentDeleteHandler)
		r.Delete("/v1/browser/trash/empty", handlers.EmptyTrashHandler)
		
		// Favorites operations
		r.Get("/v1/browser/favorites", handlers.ListFavoritesHandler)
		r.Post("/v1/browser/favorites", handlers.AddFavoriteHandler)
		r.Delete("/v1/browser/favorites", handlers.RemoveFavoriteHandler)
		
		// Recent files
		r.Get("/v1/browser/recent", handlers.ListRecentHandler)

		// Admin user management
		r.Post("/v1/admin/users", handlers.AdminCreateUserHandler)
		r.Get("/v1/admin/users", handlers.ListUsersHandler)
	})

	// Storage node settings (protected)
	r.With(handlers.AuthMiddleware).Get("/v1/settings/storage-nodes", handlers.ListStorageNodesHandler)
	r.With(handlers.AuthMiddleware).Get("/v1/settings/storage-nodes/", handlers.ListStorageNodesHandler)
	r.With(handlers.AuthMiddleware).Post("/v1/settings/storage-nodes", handlers.CreateStorageNodeHandler)
	r.With(handlers.AuthMiddleware).Post("/v1/settings/storage-nodes/", handlers.CreateStorageNodeHandler)
	r.With(handlers.AuthMiddleware).Put("/v1/settings/storage-nodes/{nodeID}", handlers.UpdateStorageNodeHandler)
	r.With(handlers.AuthMiddleware).Delete("/v1/settings/storage-nodes/{nodeID}", handlers.DeleteStorageNodeHandler)
	
	// Storage overview (protected)
	r.With(handlers.AuthMiddleware).Get("/v1/settings/storage/overview", handlers.GetStorageOverviewHandler)
	r.With(handlers.AuthMiddleware).Get("/v1/settings/storage/user", handlers.GetUserStorageHandler)

	// Peer server management (protected)
	r.With(handlers.AuthMiddleware).Get("/v1/peers", handlers.ListPeerServersHandler)
	r.With(handlers.AuthMiddleware).Post("/v1/peers/request", handlers.RequestPeerConnectionHandler)
	r.With(handlers.AuthMiddleware).Post("/v1/peers/{peerID}/accept", handlers.AcceptPeerConnectionHandler)
	r.With(handlers.AuthMiddleware).Post("/v1/peers/{peerID}/reject", handlers.RejectPeerConnectionHandler)
	r.With(handlers.AuthMiddleware).Delete("/v1/peers/{peerID}", handlers.DeletePeerServerHandler)
	r.With(handlers.AuthMiddleware).Put("/v1/peers/{peerID}/quota", handlers.UpdatePeerQuotaHandler)
	r.With(handlers.AuthMiddleware).Get("/v1/peers/{peerID}/storage", handlers.CheckPeerStorageHandler)
	
	// Peer file sharing settings (protected)
	r.With(handlers.AuthMiddleware).Get("/v1/peer-sharing/settings", handlers.GetPeerFileSharingSettingsHandler)
	r.With(handlers.AuthMiddleware).Put("/v1/peer-sharing/settings", handlers.UpdatePeerFileSharingSettingsHandler)
	r.With(handlers.AuthMiddleware).Post("/v1/peer-sharing/settings/confirm-disable", handlers.ConfirmDisableCompleteStorageHandler)
	r.With(handlers.AuthMiddleware).Get("/v1/peer-sharing/available", handlers.GetAvailablePeersForSharingHandler)
	r.With(handlers.AuthMiddleware).Post("/v1/peer-sharing/outbound", handlers.AddPeerSharingOutboundHandler)
	r.With(handlers.AuthMiddleware).Put("/v1/peer-sharing/outbound/{id}", handlers.UpdatePeerSharingOutboundHandler)
	r.With(handlers.AuthMiddleware).Delete("/v1/peer-sharing/outbound/{id}", handlers.RemovePeerSharingOutboundHandler)
	r.With(handlers.AuthMiddleware).Post("/v1/peer-sharing/inbound", handlers.AddPeerSharingInboundHandler)
	r.With(handlers.AuthMiddleware).Put("/v1/peer-sharing/inbound/{id}", handlers.UpdatePeerSharingInboundHandler)
	r.With(handlers.AuthMiddleware).Delete("/v1/peer-sharing/inbound/{id}", handlers.RemovePeerSharingInboundHandler)
	
	// Peer-to-peer communication (public - called by other servers)
	r.Post("/v1/peers/receive-request", handlers.ReceivePeerConnectionHandler)
	r.Post("/v1/peers/receive-acceptance", handlers.ReceivePeerAcceptanceHandler)
	r.Get("/v1/peers/storage-info", handlers.GetPeerStorageInfoHandler)
	r.Post("/v1/peers/receive-file", handlers.ReceivePeerFileHandler)

	// OS Node Provisioning endpoints (public - for enrollment)
	r.Post("/v1/provisioning/register-request", handlers.RegisterRequestHandler)
	r.Get("/v1/provisioning/register-status/{requestID}", handlers.CheckEnrollmentStatusHandler)
	r.Post("/v1/provisioning/register-node", handlers.RegisterNodeHandler)
	r.Get("/v1/provisioning/node-status/{nodeID}", handlers.NodeStatusHandler)
	r.Post("/v1/provisioning/node-removal", handlers.NodeRemovalHandler)

        // OS Node Provisioning endpoints (protected - admin only)
        r.With(handlers.AuthMiddleware).Post("/v1/provisioning/generate-otp", handlers.GenerateOTPHandler)
        r.With(handlers.AuthMiddleware).Get("/v1/provisioning/enrollments/pending", handlers.ListPendingEnrollmentsHandler)
        r.With(handlers.AuthMiddleware).Post("/v1/provisioning/enrollments/{requestID}/approve", handlers.ApproveEnrollmentHandler)
        r.With(handlers.AuthMiddleware).Post("/v1/provisioning/enrollments/{requestID}/reject", handlers.RejectEnrollmentHandler)
        r.With(handlers.AuthMiddleware).Get("/v1/provisioning/nodes", handlers.ListNodesHandler)
        r.With(handlers.AuthMiddleware).Post("/v1/provisioning/nodes/{nodeID}/revoke", handlers.RevokeNodeHandler)

        // Browser certificate generation (public - validated by encryption key)
        r.Post("/v1/browser/get-certificate", handlers.GenerateBrowserCertificateHandler)	// Certificate renewal endpoint (requires mTLS - will be handled by separate server)
	// r.Post("/v1/provisioning/renew-certificate", handlers.RenewCertificateHandler)

	// Create HTTP server
	addr := ":8081"
	httpServer := &http.Server{
		Addr:           addr,
		Handler:        r,
		MaxHeaderBytes: 1 << 20, // 1 MB for headers
	}

	logger.Info().Str("addr", addr).Msg("✓ Server listening on")
	logger.Info().Msg("\n📍 Auth endpoints:")
	logger.Info().Str("register", fmt.Sprintf("POST http://localhost:%s/v1/auth/register", addr)).Msg("  •")
	logger.Info().Str("login", fmt.Sprintf("POST http://localhost:%s/v1/auth/login", addr)).Msg("  •")
	logger.Info().Str("me", fmt.Sprintf("GET  http://localhost:%s/v1/auth/me (protected)", addr)).Msg("  •")
	logger.Info().Str("logout", fmt.Sprintf("POST http://localhost:%s/v1/auth/logout", addr)).Msg("  •")
	logger.Info().Msg("\n📤 Upload endpoints (protected):")
	logger.Info().Str("upload", fmt.Sprintf("POST http://localhost:%s/v1/browser/upload", addr)).Msg("  •")
	logger.Info().Str("upload-folder", fmt.Sprintf("POST http://localhost:%s/v1/browser/upload-folder", addr)).Msg("  •")
	logger.Info().Msg("\n🔐 Use Authorization: Bearer <token> header for protected routes")
	logger.Info().Msg("")

	// Setup graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigChan
		logger.Info().Msg("Shutting down gracefully...")
		cancel() // Stop health monitor
		healthMonitor.Stop()
		os.Exit(0)
	}()

	if err := httpServer.ListenAndServe(); err != nil {
		fmt.Fprintf(os.Stderr, "Server failed: %v\n", err)
		os.Exit(1)
	}
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
