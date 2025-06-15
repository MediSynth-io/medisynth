package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/MediSynth-io/medisynth/internal/models"
	"github.com/MediSynth-io/medisynth/internal/store"
)

var (
	dataStore *store.Store
)

// SetStore sets the store for the auth package
func SetStore(s *store.Store) {
	dataStore = s
}

// RegisterUser creates a new user
func RegisterUser(email, password string) (*models.User, error) {
	// Create user with hashed password
	user, err := models.NewUser(email, password)
	if err != nil {
		return nil, err
	}

	// Store user in database
	user, err = dataStore.CreateUser(user.Email, user.Password)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// ValidateUser validates user credentials
func ValidateUser(email, password string) (*models.User, error) {
	user, err := dataStore.GetUserByEmail(email)
	if err != nil {
		return nil, err
	}

	if !user.ValidatePassword(password) {
		return nil, errors.New("invalid password")
	}

	return user, nil
}

// CreateToken creates a new API token for a user
func CreateToken(userID string, name string) (*models.Token, error) {
	// Generate random token
	tokenStr, err := generateRandomToken()
	if err != nil {
		return nil, err
	}

	// Set expiration to 1 year from now
	expiresAt := time.Now().AddDate(1, 0, 0)

	// Create token in database
	token, err := dataStore.CreateToken(userID, name, tokenStr, &expiresAt)
	if err != nil {
		return nil, err
	}

	return token, nil
}

// ValidateToken validates an API token
func ValidateToken(token string) (*models.Token, error) {
	t, err := dataStore.GetTokenByValue(token)
	if err != nil {
		return nil, err
	}

	// Check if token is expired
	if t.ExpiresAt != nil && t.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("token expired")
	}

	return t, nil
}

// DeleteToken deletes an API token
func DeleteToken(userID string, tokenID string) error {
	return dataStore.DeleteToken(userID, tokenID)
}

// ListTokens lists all tokens for a user
func ListTokens(userID string) ([]*models.Token, error) {
	return dataStore.GetUserTokens(userID)
}

// generateRandomToken generates a random token string
func generateRandomToken() (string, error) {
	tokenBytes := make([]byte, 32)
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(tokenBytes), nil
}

// CreateSession creates a new session for a user
func CreateSession(userID string) (string, error) {
	token, err := generateRandomToken()
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().Add(24 * time.Hour)
	err = dataStore.CreateSession(userID, token, expiresAt)
	if err != nil {
		return "", err
	}
	return token, nil
}

// ValidateSession validates a session token and returns the user ID
func ValidateSession(token string) (string, error) {
	session, err := dataStore.ValidateSession(token)
	if err != nil {
		return "", err
	}
	return session.UserID, nil
}

// DeleteSession deletes a user's session
func DeleteSession(token string) error {
	return dataStore.DeleteSession(token)
}

// CleanupExpiredSessions removes expired session records from the database.
func CleanupExpiredSessions() error {
	return dataStore.CleanupExpiredSessions()
}
