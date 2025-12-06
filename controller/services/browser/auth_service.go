package services

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"storageos/controller/models"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserExists         = errors.New("user already exists")
	ErrInvalidToken       = errors.New("invalid token")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserDisabled       = errors.New("user account is disabled")
)

// JWT secret key - in production, load from environment or config
var jwtSecret = []byte("your-secret-key-change-this-in-production")

// Claims represents JWT claims
type Claims struct {
	UserID   string `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	jwt.RegisteredClaims
}

// AuthService handles authentication logic
type AuthService struct {
	db *sql.DB
}

// NewAuthService creates a new auth service
func NewAuthService(db *sql.DB) *AuthService {
	return &AuthService{db: db}
}

// DB returns the database connection
func (s *AuthService) DB() *sql.DB {
	return s.db
}

// Register creates a new user account
func (s *AuthService) Register(username, email, password string) (*models.User, error) {
	// Check if user already exists
	if _, err := models.GetUserByUsername(s.db, username); err == nil {
		return nil, ErrUserExists
	}
	if _, err := models.GetUserByEmail(s.db, email); err == nil {
		return nil, ErrUserExists
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}

	// Create user
	user := &models.User{
		ID:           uuid.New().String(),
		Username:     username,
		Email:        email,
		PasswordHash: string(hashedPassword),
		Role:         "user",
		UsedStorage:  0,
		MaxStorage:   1073741824, // 1 GB
		Status:       "active",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	if err := models.CreateUser(s.db, user); err != nil {
		return nil, fmt.Errorf("create user: %w", err)
	}

	return user, nil
}

// Login authenticates a user and returns a JWT token
func (s *AuthService) Login(username, password string) (string, *models.User, error) {
	// Get user by username or email
	user, err := models.GetUserByUsername(s.db, username)
	if err != nil {
		// Try email if username fails
		user, err = models.GetUserByEmail(s.db, username)
		if err != nil {
			return "", nil, ErrInvalidCredentials
		}
	}

	// Check if user is active
	if user.Status != "active" {
		return "", nil, ErrUserDisabled
	}

	// Verify password
	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return "", nil, ErrInvalidCredentials
	}

	// Update last login
	if err := models.UpdateLastLogin(s.db, user.ID); err != nil {
		// Log error but don't fail login
		fmt.Printf("Failed to update last login: %v\n", err)
	}

	// Generate JWT token
	token, err := s.GenerateToken(user)
	if err != nil {
		return "", nil, fmt.Errorf("generate token: %w", err)
	}

	return token, user, nil
}

// GenerateToken creates a JWT token for a user
func (s *AuthService) GenerateToken(user *models.User) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		UserID:   user.ID,
		Username: user.Username,
		Email:    user.Email,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "storageos",
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(jwtSecret)
	if err != nil {
		return "", err
	}

	return tokenString, nil
}

// ValidateToken validates a JWT token and returns the claims
func (s *AuthService) ValidateToken(tokenString string) (*Claims, error) {
	claims := &Claims{}

	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return jwtSecret, nil
	})

	if err != nil {
		return nil, ErrInvalidToken
	}

	if !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// GetUser retrieves a user by ID
func (s *AuthService) GetUser(userID string) (*models.User, error) {
	user, err := models.GetUserByID(s.db, userID)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrUserNotFound
		}
		return nil, err
	}
	return user, nil
}

// SetJWTSecret sets the JWT secret key
func SetJWTSecret(secret string) {
	jwtSecret = []byte(secret)
}
