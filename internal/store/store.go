package store

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/MediSynth-io/medisynth/internal/models"
)

// Store handles all database operations
type Store struct {
	db *sql.DB
}

// New creates a new store instance
func New(db *sql.DB) *Store {
	return &Store{db: db}
}

// CreateUser creates a new user
func (s *Store) CreateUser(email, password string) (*models.User, error) {
	now := time.Now()
	user := &models.User{
		Email:     email,
		Password:  password,
		CreatedAt: now,
		UpdatedAt: now,
	}

	result, err := s.db.Exec(
		"INSERT INTO users (email, password, created_at, updated_at) VALUES (?, ?, ?, ?)",
		user.Email, user.Password, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}
	user.ID = id

	return user, nil
}

// GetUserByEmail retrieves a user by email
func (s *Store) GetUserByEmail(email string) (*models.User, error) {
	user := &models.User{}
	err := s.db.QueryRow(
		"SELECT id, email, password, created_at, updated_at FROM users WHERE email = ?",
		email,
	).Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// CreateToken creates a new API token
func (s *Store) CreateToken(userID int64, name, token string, expiresAt *time.Time) (*models.Token, error) {
	t := &models.Token{
		ID:        generateID(),
		UserID:    userID,
		Token:     token,
		Name:      name,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	_, err := s.db.Exec(
		"INSERT INTO tokens (id, user_id, token, name, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)",
		t.ID, t.UserID, t.Token, t.Name, t.CreatedAt, t.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}

	return t, nil
}

// GetTokenByValue retrieves a token by its value
func (s *Store) GetTokenByValue(token string) (*models.Token, error) {
	t := &models.Token{}
	err := s.db.QueryRow(
		"SELECT id, user_id, token, name, created_at, expires_at FROM tokens WHERE token = ?",
		token,
	).Scan(&t.ID, &t.UserID, &t.Token, &t.Name, &t.CreatedAt, &t.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// DeleteToken deletes a token
func (s *Store) DeleteToken(userID int64, tokenID string) error {
	result, err := s.db.Exec("DELETE FROM tokens WHERE id = ? AND user_id = ?", tokenID, userID)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// GetUserTokens retrieves all tokens for a user
func (s *Store) GetUserTokens(userID int64) ([]*models.Token, error) {
	rows, err := s.db.Query(
		"SELECT id, user_id, token, name, created_at, expires_at FROM tokens WHERE user_id = ?",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*models.Token
	for rows.Next() {
		t := &models.Token{}
		err := rows.Scan(&t.ID, &t.UserID, &t.Token, &t.Name, &t.CreatedAt, &t.ExpiresAt)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, t)
	}
	return tokens, rows.Err()
}

// generateID generates a unique ID
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
