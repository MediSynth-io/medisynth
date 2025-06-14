package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
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
func CreateSession(userID int64) (*Session, error) {
	// Generate random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)

	// Set expiration to 24 hours
	expiresAt := time.Now().Add(24 * time.Hour)

	// Insert session into database
	result, err := database.GetDB().Exec(
		"INSERT INTO sessions (user_id, token, expires_at) VALUES (?, ?, ?)",
		userID, token, expiresAt,
	)
	if err != nil {
		return nil, err
	}

	// Get the new session's ID
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &Session{
		ID:        id,
		UserID:    userID,
		Token:     token,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}, nil
}

// ValidateSession checks if a session is valid and returns the associated user ID
func ValidateSession(token string) (int64, error) {
	var userID int64
	var expiresAt time.Time
	err := database.GetDB().QueryRow(
		"SELECT user_id, expires_at FROM sessions WHERE token = ?",
		token,
	).Scan(&userID, &expiresAt)
	if err == sql.ErrNoRows {
		return 0, ErrSessionNotFound
	}
	if err != nil {
		return 0, err
	}

	// Check if session has expired
	if expiresAt.Before(time.Now()) {
		return 0, ErrSessionExpired
	}

	return userID, nil
}

// DeleteSession removes a session
func DeleteSession(token string) error {
	result, err := database.GetDB().Exec("DELETE FROM sessions WHERE token = ?", token)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrSessionNotFound
	}

	return nil
}

// CleanupExpiredSessions removes all expired sessions
func CleanupExpiredSessions() error {
	_, err := database.GetDB().Exec("DELETE FROM sessions WHERE expires_at < ?", time.Now())
	return err
}
