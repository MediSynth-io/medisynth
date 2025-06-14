package auth

import (
	"database/sql"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var (
	ErrInvalidToken = errors.New("invalid token")
	ErrExpiredToken = errors.New("token has expired")
)

// TokenClaims represents the claims in a JWT token
type TokenClaims struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// TokenManager handles token operations
type TokenManager struct {
	secretKey []byte
}

// NewTokenManager creates a new TokenManager
func NewTokenManager(secretKey string) *TokenManager {
	return &TokenManager{
		secretKey: []byte(secretKey),
	}
}

// GenerateToken creates a new JWT token for a user
func (tm *TokenManager) GenerateToken(user *User, duration time.Duration) (string, error) {
	claims := TokenClaims{
		UserID: user.ID,
		Email:  user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(duration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			NotBefore: jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(tm.secretKey)
}

// ValidateToken validates a JWT token and returns the claims
func (tm *TokenManager) ValidateToken(tokenString string) (*TokenClaims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalidToken
		}
		return tm.secretKey, nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrExpiredToken
		}
		return nil, ErrInvalidToken
	}

	claims, ok := token.Claims.(*TokenClaims)
	if !ok || !token.Valid {
		return nil, ErrInvalidToken
	}

	return claims, nil
}

// APIKey represents an API key for a user
type APIKey struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Key       string    `json:"key"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

// APIKeyStore defines the interface for API key storage operations
type APIKeyStore interface {
	Create(key *APIKey) error
	GetByKey(key string) (*APIKey, error)
	GetByUserID(userID int64) ([]*APIKey, error)
	Delete(id int64) error
}

// SQLiteAPIKeyStore implements APIKeyStore for SQLite
type SQLiteAPIKeyStore struct {
	db *sql.DB
}

// NewSQLiteAPIKeyStore creates a new SQLiteAPIKeyStore
func NewSQLiteAPIKeyStore(db *sql.DB) *SQLiteAPIKeyStore {
	return &SQLiteAPIKeyStore{db: db}
}

// Create stores a new API key in the database
func (s *SQLiteAPIKeyStore) Create(key *APIKey) error {
	query := `
		INSERT INTO api_keys (user_id, key, name, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, key.UserID, key.Key, key.Name, key.CreatedAt, key.ExpiresAt)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	key.ID = id
	return nil
}

// GetByKey retrieves an API key by its value
func (s *SQLiteAPIKeyStore) GetByKey(key string) (*APIKey, error) {
	query := `
		SELECT id, user_id, key, name, created_at, expires_at
		FROM api_keys
		WHERE key = ?
	`
	apiKey := &APIKey{}
	err := s.db.QueryRow(query, key).Scan(
		&apiKey.ID,
		&apiKey.UserID,
		&apiKey.Key,
		&apiKey.Name,
		&apiKey.CreatedAt,
		&apiKey.ExpiresAt,
	)
	if err == sql.ErrNoRows {
		return nil, errors.New("api key not found")
	}
	if err != nil {
		return nil, err
	}
	return apiKey, nil
}

// GetByUserID retrieves all API keys for a user
func (s *SQLiteAPIKeyStore) GetByUserID(userID int64) ([]*APIKey, error) {
	query := `
		SELECT id, user_id, key, name, created_at, expires_at
		FROM api_keys
		WHERE user_id = ?
	`
	rows, err := s.db.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []*APIKey
	for rows.Next() {
		key := &APIKey{}
		err := rows.Scan(
			&key.ID,
			&key.UserID,
			&key.Key,
			&key.Name,
			&key.CreatedAt,
			&key.ExpiresAt,
		)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// Delete removes an API key from the database
func (s *SQLiteAPIKeyStore) Delete(id int64) error {
	query := `DELETE FROM api_keys WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
}
