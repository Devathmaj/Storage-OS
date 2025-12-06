package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strings"

	services "storageos/controller/services/browser"
)

var authService *services.AuthService

// InitAuthHandlers initializes the auth service with a database connection
func InitAuthHandlers(db *sql.DB) {
	authService = services.NewAuthService(db)
}

// RegisterRequest represents a registration request
type RegisterRequest struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// LoginResponse represents a login response
type LoginResponse struct {
	Token string      `json:"token"`
	User  interface{} `json:"user"`
}

// RegisterHandler handles user registration
func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	// Check if this is the first user (should use /v1/system/init-admin instead)
	var userCount int
	if err := authService.DB().QueryRow(`SELECT COUNT(*) FROM users`).Scan(&userCount); err == nil && userCount == 0 {
		respondError(w, http.StatusForbidden, "Please use /v1/system/init-admin to create the first admin user")
		return
	}

	// After first user, registration is disabled (admin must create users)
	respondError(w, http.StatusForbidden, "Public registration is disabled. Please contact your administrator.")
	return
}

// LoginHandler handles user login
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.Username == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	token, user, err := authService.Login(req.Username, req.Password)
	if err != nil {
		if err == services.ErrInvalidCredentials {
			respondError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		if err == services.ErrUserDisabled {
			respondError(w, http.StatusForbidden, "user account is disabled")
			return
		}
		respondError(w, http.StatusInternalServerError, "login failed")
		return
	}

	respondJSON(w, http.StatusOK, LoginResponse{
		Token: token,
		User:  user,
	})
}

// MeHandler returns the current user's information
func MeHandler(w http.ResponseWriter, r *http.Request) {
	// Get user from context (set by auth middleware)
	claims, ok := r.Context().Value("user").(*services.Claims)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	user, err := authService.GetUser(claims.UserID)
	if err != nil {
		if err == services.ErrUserNotFound {
			respondError(w, http.StatusNotFound, "user not found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to get user")
		return
	}

	respondJSON(w, http.StatusOK, user)
}

// LogoutHandler handles user logout (client-side only, token is removed from client)
func LogoutHandler(w http.ResponseWriter, r *http.Request) {
	// In a JWT-based system, logout is typically handled client-side
	// The client just removes the token from storage
	respondJSON(w, http.StatusOK, map[string]string{"message": "logged out successfully"})
}

// AuthMiddleware validates JWT token and adds user claims to context
func AuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if authService is initialized
		if authService == nil {
			log.Printf("AuthMiddleware: ERROR - authService is nil!")
			respondError(w, http.StatusInternalServerError, "authentication service not initialized")
			return
		}

		// Get token from Authorization header
		authHeader := r.Header.Get("Authorization")
		log.Printf("AuthMiddleware called for %s %s AuthorizationPresent=%v", r.Method, r.URL.Path, authHeader != "")
		if authHeader == "" {
			log.Printf("AuthMiddleware: missing Authorization header")
			respondError(w, http.StatusUnauthorized, "missing authorization header")
			return
		}

		// Extract token from "Bearer <token>"
		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			respondError(w, http.StatusUnauthorized, "invalid authorization header format")
			return
		}

		token := parts[1]
		log.Printf("AuthMiddleware: validating token (first 20 chars): %s...", token[:min(20, len(token))])
		claims, err := authService.ValidateToken(token)
		if err != nil {
			log.Printf("AuthMiddleware: token validation failed: %v", err)
			respondError(w, http.StatusUnauthorized, "invalid token")
			return
		}

		log.Printf("AuthMiddleware: token validated successfully for user %s (%s)", claims.Username, claims.UserID)

		// Add claims to context
		ctx := context.WithValue(r.Context(), "user", claims)
		ctx = context.WithValue(ctx, "user_id", claims.UserID)
		
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func respondJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, map[string]string{"error": message})
}
