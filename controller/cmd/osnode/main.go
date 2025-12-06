package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"storageos/controller/api/os/handlers"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	_ "github.com/mattn/go-sqlite3"
)

// OS Node Server - runs inside VM to handle file storage
// Separate from controller/main.go (controller server)

var (
	port       = flag.String("port", "8082", "OS node HTTP port")
	dbPath     = flag.String("db", "/data/metadata/osnode.db", "Database path")
	nodeID     = flag.String("node-id", "os-node-001", "OS node identifier")
	dataDir    = flag.String("data", "/data", "Data directory root")
)

func main() {
	flag.Parse()

	// Initialize logger
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Printf("Starting OS Node %s on port %s", *nodeID, *port)

	// Ensure data directory exists
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Initialize database
	db, err := initDatabase(*dbPath)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	// Initialize OS handlers with database
	if err := handlers.InitOSHandlers(db, *dataDir); err != nil {
		log.Fatalf("Failed to initialize handlers: %v", err)
	}

	// Create router
	r := chi.NewRouter()
	
	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	// Health check
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","node_id":%q}`, *nodeID)
	})

	// OS Node API routes - receive files from controller and store to /data/files
	r.Route("/api", func(r chi.Router) {
		r.Post("/upload", handlers.UploadHandler)        // Save to /data/files/{owner_id}/{file_id}
		r.Get("/download", handlers.DownloadHandler)     // Read from storage path
		r.Get("/metadata", handlers.GetMetadataHandler)  // Query file metadata
		r.Get("/files", handlers.ListFilesHandler)       // List user's files
		r.Delete("/delete", handlers.DeleteHandler)      // Delete file from storage
	})

	// Start server
	srv := &http.Server{
		Addr:         ":" + *port,
		Handler:      r,
		ReadTimeout:  60 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Graceful shutdown
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		log.Println("Shutting down server...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("Server shutdown error: %v", err)
		}
	}()

	log.Printf("OS Node listening on :%s", *port)
	log.Printf("Storage directory: %s", handlers.StorageBasePath)
	log.Printf("API endpoints:")
	log.Printf("  POST   /api/upload   - Upload file to /data/files")
	log.Printf("  GET    /api/download - Download file from storage")
	log.Printf("  GET    /api/metadata - Get file metadata")
	log.Printf("  GET    /api/files    - List user files")
	log.Printf("  DELETE /api/delete   - Delete file")

	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("Server error: %v", err)
	}

	log.Println("Server stopped")
}

func initDatabase(dbPath string) (*sql.DB, error) {
	// Ensure database directory exists
	dbDir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dbDir, 0755); err != nil {
		return nil, fmt.Errorf("create database directory: %w", err)
	}

	// Open database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Hour)

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping database: %w", err)
	}

	// Load schema
	schemaPath := filepath.Join(filepath.Dir(dbPath), "os_node_schema.sql")
	if _, err := os.Stat(schemaPath); err == nil {
		log.Printf("Loading schema from %s", schemaPath)
		schema, err := os.ReadFile(schemaPath)
		if err != nil {
			return nil, fmt.Errorf("read schema: %w", err)
		}

		if _, err := db.Exec(string(schema)); err != nil {
			return nil, fmt.Errorf("execute schema: %w", err)
		}
		log.Println("Database schema loaded successfully")
	} else {
		// Create basic schema inline if file not found
		log.Println("Creating basic schema (schema file not found)")
		schema := `
		CREATE TABLE IF NOT EXISTS files (
			id TEXT PRIMARY KEY,
			owner_id TEXT NOT NULL,
			folder_id TEXT,
			filename TEXT NOT NULL,
			extension TEXT,
			original_extension TEXT,
			mime_type TEXT,
			size INTEGER NOT NULL,
			checksum TEXT NOT NULL,
			storage_path TEXT NOT NULL,
			node_id TEXT NOT NULL,
			version INTEGER DEFAULT 1,
			status TEXT DEFAULT 'active',
			uploaded_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			modified_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
			accessed_at TIMESTAMP
		);
		CREATE INDEX IF NOT EXISTS idx_files_owner_id ON files(owner_id);
		CREATE INDEX IF NOT EXISTS idx_files_status ON files(status);
		`
		if _, err := db.Exec(schema); err != nil {
			return nil, fmt.Errorf("create schema: %w", err)
		}
	}

	log.Println("Database initialized successfully")
	return db, nil
}
