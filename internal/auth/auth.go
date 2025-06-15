package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"log"
	"strings"
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
	log.Printf("[AUTH] Starting session creation for user: %s", userID)

	token, err := generateRandomToken()
	if err != nil {
		log.Printf("[AUTH] Failed to generate random token for user %s: %v", userID, err)
		return "", err
	}
	log.Printf("[AUTH] Generated token for user %s, token length: %d", userID, len(token))

	expiresAt := time.Now().Add(24 * time.Hour)
	log.Printf("[AUTH] Session will expire at: %v", expiresAt)

	log.Printf("[AUTH] Calling dataStore.CreateSession for user %s", userID)
	err = dataStore.CreateSession(userID, token, expiresAt)
	if err != nil {
		log.Printf("[AUTH] dataStore.CreateSession failed for user %s: %v", userID, err)
		return "", err
	}

	log.Printf("[AUTH] Session created successfully for user %s", userID)
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

// --- Validation Helpers ---

// PasswordRequirements defines the complexity requirements for a password
type PasswordRequirements struct {
	MinLength int
	HasUpper  bool
	HasLower  bool
	HasNumber bool
	HasSymbol bool
}

// GetPasswordRequirements returns the current password policy
func GetPasswordRequirements() PasswordRequirements {
	return PasswordRequirements{
		MinLength: 8,
		HasUpper:  true,
		HasLower:  true,
		HasNumber: true,
		HasSymbol: true,
	}
}

// ValidatePassword checks if a password meets the complexity requirements.
func ValidatePassword(password string) bool {
	var (
		hasUpper  bool
		hasLower  bool
		hasNumber bool
		hasSymbol bool
	)
	if len(password) < GetPasswordRequirements().MinLength {
		return false
	}
	for _, char := range password {
		switch {
		case 'A' <= char && char <= 'Z':
			hasUpper = true
		case 'a' <= char && char <= 'z':
			hasLower = true
		case '0' <= char && char <= '9':
			hasNumber = true
		default:
			hasSymbol = true
		}
	}
	return hasUpper && hasLower && hasNumber && hasSymbol
}

// ValidateEmail checks if an email has a valid format.
func ValidateEmail(email string) bool {
	// A very basic email validation check
	return strings.Contains(email, "@") && strings.Contains(email, ".")
}
