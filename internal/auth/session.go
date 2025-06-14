package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/MediSynth-io/medisynth/internal/database"
)

var (
	ErrSessionNotFound = errors.New("session not found")
	ErrSessionExpired  = errors.New("session has expired")
)

type Session struct {
	ID        int64
	UserID    int64
	Token     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// CreateSession creates a new session for a user
func CreateSession(userID int64) (string, error) {
	// Generate random token
	tokenBytes := make([]byte, 32)
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", err
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	// Set expiration to 24 hours from now
	expiresAt := time.Now().Add(24 * time.Hour)

	// Create session in database
	err = database.CreateSession(userID, token, expiresAt)
	if err != nil {
		return "", err
	}

	return token, nil
}

// ValidateSession checks if a session is valid and returns the associated user ID
func ValidateSession(token string) (*database.Session, error) {
	session, err := database.GetSessionByToken(token)
	if err != nil {
		return nil, ErrSessionNotFound
	}

	// Check if session is expired
	if session.ExpiresAt.Before(time.Now()) {
		// Delete expired session
		_ = DeleteSession(token)
		return nil, ErrSessionExpired
	}

	return session, nil
}

// DeleteSession removes a session
func DeleteSession(token string) error {
	return database.DeleteSession(token)
}

// CleanupExpiredSessions removes all expired sessions
func CleanupExpiredSessions() error {
	return database.CleanupExpiredSessions()
}
