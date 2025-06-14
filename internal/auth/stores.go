package auth

import (
	"database/sql"
	"errors"
	"time"
)

// UserStore defines the interface for user storage operations
type UserStore interface {
	Create(user *User) error
	GetByEmail(email string) (*User, error)
	GetByID(id int64) (*User, error)
	Update(user *User) error
	Delete(id int64) error
}

// SQLiteUserStore implements UserStore for SQLite
type SQLiteUserStore struct {
	db *sql.DB
}

// NewUserStore creates a new SQLiteUserStore
func NewUserStore() *SQLiteUserStore {
	return &SQLiteUserStore{
		db: GetDB(),
	}
}

// Create stores a new user in the database
func (s *SQLiteUserStore) Create(user *User) error {
	query := `
		INSERT INTO users (email, password, created_at, updated_at)
		VALUES (?, ?, ?, ?)
	`
	result, err := s.db.Exec(query, user.Email, user.Password, user.CreatedAt, user.UpdatedAt)
	if err != nil {
		return err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return err
	}
	user.ID = id
	return nil
}

// GetByEmail retrieves a user by email
func (s *SQLiteUserStore) GetByEmail(email string) (*User, error) {
	query := `
		SELECT id, email, password, created_at, updated_at
		FROM users
		WHERE email = ?
	`
	user := &User{}
	err := s.db.QueryRow(query, email).Scan(
		&user.ID,
		&user.Email,
		&user.Password,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, errors.New("user not found")
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetByID retrieves a user by ID
func (s *SQLiteUserStore) GetByID(id int64) (*User, error) {
	query := `
		SELECT id, email, password, created_at, updated_at
		FROM users
		WHERE id = ?
	`
	user := &User{}
	err := s.db.QueryRow(query, id).Scan(
		&user.ID,
		&user.Email,
		&user.Password,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, errors.New("user not found")
	}
	if err != nil {
		return nil, err
	}
	return user, nil
}

// Update updates a user's information
func (s *SQLiteUserStore) Update(user *User) error {
	query := `
		UPDATE users
		SET email = ?, password = ?, updated_at = ?
		WHERE id = ?
	`
	_, err := s.db.Exec(query, user.Email, user.Password, time.Now(), user.ID)
	return err
}

// Delete removes a user from the database
func (s *SQLiteUserStore) Delete(id int64) error {
	query := `DELETE FROM users WHERE id = ?`
	_, err := s.db.Exec(query, id)
	return err
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

// NewAPIKeyStore creates a new SQLiteAPIKeyStore
func NewAPIKeyStore() *SQLiteAPIKeyStore {
	return &SQLiteAPIKeyStore{
		db: GetDB(),
	}
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
