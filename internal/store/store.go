package store

import (
	"log"
	"time"

	"github.com/MediSynth-io/medisynth/internal/database"
	"github.com/MediSynth-io/medisynth/internal/models"
)

// Store handles all database operations
type Store struct{}

// New creates a new store instance
func New() *Store {
	return &Store{}
}

// CreateUser creates a new user
func (s *Store) CreateUser(email, password string) (*models.User, error) {
	return database.CreateUser(email, password)
}

// GetUserByEmail retrieves a user by email
func (s *Store) GetUserByEmail(email string) (*models.User, error) {
	return database.GetUserByEmail(email)
}

// CreateToken creates a new API token
func (s *Store) CreateToken(userID string, name, token string, expiresAt *time.Time) (*models.Token, error) {
	return database.CreateToken(userID, name, token, expiresAt)
}

// GetTokenByValue retrieves a token by its value
func (s *Store) GetTokenByValue(token string) (*models.Token, error) {
	return database.GetTokenByValue(token)
}

// DeleteToken deletes a token
func (s *Store) DeleteToken(userID string, tokenID string) error {
	return database.DeleteToken(userID, tokenID)
}

// GetUserTokens retrieves all tokens for a user
func (s *Store) GetUserTokens(userID string) ([]*models.Token, error) {
	return database.GetUserTokens(userID)
}

// CreateSession creates a new session
func (s *Store) CreateSession(userID string, token string, expiresAt time.Time) error {
	log.Printf("[STORE] CreateSession called - UserID: %s, TokenLength: %d", userID, len(token))
	err := database.CreateSession(userID, token, expiresAt)
	if err != nil {
		log.Printf("[STORE] CreateSession failed: %v", err)
	} else {
		log.Printf("[STORE] CreateSession succeeded")
	}
	return err
}

// ValidateSession validates a session token
func (s *Store) ValidateSession(token string) (*models.Session, error) {
	return database.ValidateSession(token)
}

// DeleteSession deletes a session
func (s *Store) DeleteSession(token string) error {
	return database.DeleteSession(token)
}

// CleanupExpiredSessions removes expired sessions from the database.
func (s *Store) CleanupExpiredSessions() error {
	return database.CleanupExpiredSessions()
}
