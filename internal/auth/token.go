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
	ErrTokenNotFound = errors.New("token not found")
	ErrTokenExpired  = errors.New("token has expired")
)

type APIToken struct {
	ID        int64
	UserID    int64
	Token     string
	Name      string
	CreatedAt time.Time
	ExpiresAt *time.Time
}

// GenerateToken creates a new API token for a user
func GenerateToken(userID int64, name string, expiresIn time.Duration) (*APIToken, error) {
	// Generate random token
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return nil, err
	}
	token := hex.EncodeToString(tokenBytes)

	// Calculate expiration time
	var expiresAt *time.Time
	if expiresIn > 0 {
		exp := time.Now().Add(expiresIn)
		expiresAt = &exp
	}

	// Insert token into database
	result, err := database.GetDB().Exec(
		"INSERT INTO api_tokens (user_id, token, name, expires_at) VALUES (?, ?, ?, ?)",
		userID, token, name, expiresAt,
	)
	if err != nil {
		return nil, err
	}

	// Get the new token's ID
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return &APIToken{
		ID:        id,
		UserID:    userID,
		Token:     token,
		Name:      name,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}, nil
}

// ValidateToken checks if a token is valid and returns the associated user ID
func ValidateToken(token string) (int64, error) {
	var userID int64
	var expiresAt sql.NullTime
	err := database.GetDB().QueryRow(
		"SELECT user_id, expires_at FROM api_tokens WHERE token = ?",
		token,
	).Scan(&userID, &expiresAt)
	if err == sql.ErrNoRows {
		return 0, ErrTokenNotFound
	}
	if err != nil {
		return 0, err
	}

	// Check if token has expired
	if expiresAt.Valid && expiresAt.Time.Before(time.Now()) {
		return 0, ErrTokenExpired
	}

	return userID, nil
}

// ListUserTokens returns all API tokens for a user
func ListUserTokens(userID int64) ([]*APIToken, error) {
	rows, err := database.GetDB().Query(
		"SELECT id, user_id, token, name, created_at, expires_at FROM api_tokens WHERE user_id = ?",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*APIToken
	for rows.Next() {
		var token APIToken
		var expiresAt sql.NullTime
		err := rows.Scan(&token.ID, &token.UserID, &token.Token, &token.Name, &token.CreatedAt, &expiresAt)
		if err != nil {
			return nil, err
		}
		if expiresAt.Valid {
			token.ExpiresAt = &expiresAt.Time
		}
		tokens = append(tokens, &token)
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}

	return tokens, nil
}

// DeleteToken removes an API token
func DeleteToken(token string) error {
	result, err := database.GetDB().Exec("DELETE FROM api_tokens WHERE token = ?", token)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrTokenNotFound
	}

	return nil
}
