package models

import (
	"database/sql"
	"time"
)

// User represents a user in the system
type User struct {
	ID           string     `json:"id"`
	Username     string     `json:"username"`
	Email        string     `json:"email"`
	PasswordHash string     `json:"-"` // Never send to client
	Role         string     `json:"role"`
	UsedStorage  int64      `json:"used_storage"`
	MaxStorage   int64      `json:"max_storage"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	LastLogin    *time.Time `json:"last_login,omitempty"`
	Status       string     `json:"status"`
}

// CreateUser inserts a new user into the database
func CreateUser(db *sql.DB, user *User) error {
	query := `
		INSERT INTO users (id, username, email, password_hash, role, used_storage, max_storage, status)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`
	_, err := db.Exec(query, user.ID, user.Username, user.Email, user.PasswordHash,
		user.Role, user.UsedStorage, user.MaxStorage, user.Status)
	return err
}

// GetUserByUsername retrieves a user by username
func GetUserByUsername(db *sql.DB, username string) (*User, error) {
	user := &User{}
	query := `
		SELECT id, username, email, password_hash, role, used_storage, max_storage,
		       created_at, updated_at, last_login, status
		FROM users WHERE username = ?
	`
	var lastLogin sql.NullTime
	err := db.QueryRow(query, username).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash,
		&user.Role, &user.UsedStorage, &user.MaxStorage,
		&user.CreatedAt, &user.UpdatedAt, &lastLogin, &user.Status,
	)
	if err != nil {
		return nil, err
	}
	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}
	return user, nil
}

// GetUserByEmail retrieves a user by email
func GetUserByEmail(db *sql.DB, email string) (*User, error) {
	user := &User{}
	query := `
		SELECT id, username, email, password_hash, role, used_storage, max_storage,
		       created_at, updated_at, last_login, status
		FROM users WHERE email = ?
	`
	var lastLogin sql.NullTime
	err := db.QueryRow(query, email).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash,
		&user.Role, &user.UsedStorage, &user.MaxStorage,
		&user.CreatedAt, &user.UpdatedAt, &lastLogin, &user.Status,
	)
	if err != nil {
		return nil, err
	}
	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}
	return user, nil
}

// GetUserByID retrieves a user by ID
func GetUserByID(db *sql.DB, id string) (*User, error) {
	user := &User{}
	query := `
		SELECT id, username, email, password_hash, role, used_storage, max_storage,
		       created_at, updated_at, last_login, status
		FROM users WHERE id = ?
	`
	var lastLogin sql.NullTime
	err := db.QueryRow(query, id).Scan(
		&user.ID, &user.Username, &user.Email, &user.PasswordHash,
		&user.Role, &user.UsedStorage, &user.MaxStorage,
		&user.CreatedAt, &user.UpdatedAt, &lastLogin, &user.Status,
	)
	if err != nil {
		return nil, err
	}
	if lastLogin.Valid {
		user.LastLogin = &lastLogin.Time
	}
	return user, nil
}

// UpdateLastLogin updates the user's last login time
func UpdateLastLogin(db *sql.DB, userID string) error {
	query := `UPDATE users SET last_login = CURRENT_TIMESTAMP, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := db.Exec(query, userID)
	return err
}

// UpdateUsedStorage updates the user's storage usage
func UpdateUsedStorage(db *sql.DB, userID string, delta int64) error {
	query := `UPDATE users SET used_storage = used_storage + ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`
	_, err := db.Exec(query, delta, userID)
	return err
}
