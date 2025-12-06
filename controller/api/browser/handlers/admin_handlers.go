package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

// CheckSystemInitHandler checks if the system has any users
func CheckSystemInitHandler(w http.ResponseWriter, r *http.Request) {
	var count int
	err := globalDB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		log.Printf("Failed to count users: %v", err)
		http.Error(w, "Failed to check system status", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"has_users":     count > 0,
		"needs_setup":   count == 0,
		"user_count":    count,
	})
}

// CreateAdminUserRequest represents the initial admin user creation
type CreateAdminUserRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// CreateAdminUserHandler creates the first admin user (only if no users exist)
func CreateAdminUserHandler(w http.ResponseWriter, r *http.Request) {
	// Check if any users already exist
	var count int
	err := globalDB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&count)
	if err != nil {
		log.Printf("Failed to count users: %v", err)
		http.Error(w, "Failed to check system status", http.StatusInternalServerError)
		return
	}

	if count > 0 {
		http.Error(w, "System already initialized", http.StatusForbidden)
		return
	}

	var req CreateAdminUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input
	if req.Username == "" || req.Email == "" || req.Password == "" {
		http.Error(w, "Username, email, and password are required", http.StatusBadRequest)
		return
	}

	if len(req.Password) < 6 {
		http.Error(w, "Password must be at least 6 characters", http.StatusBadRequest)
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash password: %v", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Create admin user (no storage quota limit for admin)
	userID := uuid.New().String()
	now := time.Now().Format(time.RFC3339)
	
	_, err = globalDB.Exec(`
		INSERT INTO users (id, username, email, password_hash, role, used_storage, max_storage, created_at, updated_at, status)
		VALUES (?, ?, ?, ?, 'admin', 0, 0, ?, ?, 'active')
	`, userID, req.Username, req.Email, string(hashedPassword), now, now)

	if err != nil {
		log.Printf("Failed to create admin user: %v", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "Admin user created successfully",
		"user_id": userID,
		"username": req.Username,
	})
}

// CreateUserRequest represents admin creating a new user
type CreateUserRequest struct {
	Username     string `json:"username"`
	Email        string `json:"email"`
	Password     string `json:"password"`
	MaxStorageGB int64  `json:"max_storage_gb"` // Storage quota in GB
}

// AdminCreateUserHandler allows admin to create new users with custom quotas
func AdminCreateUserHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)

	// Check if user is admin
	var role string
	err := globalDB.QueryRow(`SELECT role FROM users WHERE id = ?`, userID).Scan(&role)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if role != "admin" {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		return
	}

	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate input
	if req.Username == "" || req.Email == "" || req.Password == "" {
		http.Error(w, "Username, email, and password are required", http.StatusBadRequest)
		return
	}

	if len(req.Password) < 6 {
		http.Error(w, "Password must be at least 6 characters", http.StatusBadRequest)
		return
	}

	if req.MaxStorageGB <= 0 {
		req.MaxStorageGB = 10 // Default 10GB
	}

	// Check if user already exists
	var existingCount int
	err = globalDB.QueryRow(`SELECT COUNT(*) FROM users WHERE username = ? OR email = ?`, req.Username, req.Email).Scan(&existingCount)
	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		return
	}

	if existingCount > 0 {
		http.Error(w, "User with this username or email already exists", http.StatusConflict)
		return
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Printf("Failed to hash password: %v", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	// Create user with specified quota (convert GB to bytes)
	newUserID := uuid.New().String()
	maxStorageBytes := req.MaxStorageGB * 1024 * 1024 * 1024
	now := time.Now().Format(time.RFC3339)
	
	_, err = globalDB.Exec(`
		INSERT INTO users (id, username, email, password_hash, role, used_storage, max_storage, created_at, updated_at, status)
		VALUES (?, ?, ?, ?, 'user', 0, ?, ?, ?, 'active')
	`, newUserID, req.Username, req.Email, string(hashedPassword), maxStorageBytes, now, now)

	if err != nil {
		log.Printf("Failed to create user: %v", err)
		http.Error(w, "Failed to create user", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"message": "User created successfully",
		"user_id": newUserID,
		"username": req.Username,
		"max_storage_gb": req.MaxStorageGB,
	})
}

// ListUsersHandler lists all users (admin only)
func ListUsersHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("user_id").(string)

	// Check if user is admin
	var role string
	err := globalDB.QueryRow(`SELECT role FROM users WHERE id = ?`, userID).Scan(&role)
	if err != nil {
		http.Error(w, "User not found", http.StatusNotFound)
		return
	}

	if role != "admin" {
		http.Error(w, "Forbidden: Admin access required", http.StatusForbidden)
		return
	}

	rows, err := globalDB.Query(`
		SELECT id, username, email, role, used_storage, max_storage, status, created_at
		FROM users
		ORDER BY created_at DESC
	`)
	if err != nil {
		log.Printf("Failed to list users: %v", err)
		http.Error(w, "Failed to retrieve users", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var users []map[string]interface{}
	for rows.Next() {
		var id, username, email, role, status, createdAt string
		var usedStorage, maxStorage int64

		if err := rows.Scan(&id, &username, &email, &role, &usedStorage, &maxStorage, &status, &createdAt); err != nil {
			continue
		}

		users = append(users, map[string]interface{}{
			"id":           id,
			"username":     username,
			"email":        email,
			"role":         role,
			"used_storage": usedStorage,
			"max_storage":  maxStorage,
			"status":       status,
			"created_at":   createdAt,
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"users": users,
		"total": len(users),
	})
}
