package auth

import (
	"database/sql"
	"errors"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User represents a system user
type User struct {
	ID        int64     `json:"id"`
	Email     string    `json:"email"`
	Password  string    `json:"-"` // Password is never exposed in JSON
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// NewUser creates a new user with a hashed password
func NewUser(email, password string) (*User, error) {
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	return &User{
		Email:     email,
		Password:  string(hashedPassword),
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// ValidatePassword checks if the provided password matches the user's password
func (u *User) ValidatePassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}

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

// NewSQLiteUserStore creates a new SQLiteUserStore
func NewSQLiteUserStore(db *sql.DB) *SQLiteUserStore {
	return &SQLiteUserStore{db: db}
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
