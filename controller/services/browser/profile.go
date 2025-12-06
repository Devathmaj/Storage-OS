package services

import "time"

// UserProfile is a minimal user profile structure returned by GetProfile.
type UserProfile struct {
    UserID    string
    Name      string
    Email     string
    CreatedAt time.Time
}

// GetProfile retrieves a user's profile. This is a stub that should query
// the user store or database.
func GetProfile(userID string) (UserProfile, error) {
    _ = userID
    return UserProfile{}, nil
}
