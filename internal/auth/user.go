package auth

import (
	"database/sql"
	"errors"
	"time"

	"github.com/MediSynth-io/medisynth/internal/database"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrInvalidPassword   = errors.New("invalid password")
	ErrEmailAlreadyTaken = errors.New("email already taken")
)

type User struct {
	ID           int64
	Email        string
	PasswordHash string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Register creates a new user with the given email and password
func Register(email, password string) (*User, error) {
	// Check if user already exists
	var exists bool
	err := database.GetDB().QueryRow("SELECT EXISTS(SELECT 1 FROM users WHERE email = ?)", email).Scan(&exists)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, ErrEmailAlreadyTaken
	}

	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Insert new user
	result, err := database.GetDB().Exec(
		"INSERT INTO users (email, password_hash) VALUES (?, ?)",
		email, string(hashedPassword),
	)
	if err != nil {
		return nil, err
	}

	// Get the new user's ID
	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	// Fetch the created user
	return GetUserByID(id)
}

// GetUserByID retrieves a user by their ID
func GetUserByID(id int64) (*User, error) {
	var user User
	err := database.GetDB().QueryRow(
		"SELECT id, email, password_hash, created_at, updated_at FROM users WHERE id = ?",
		id,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetUserByEmail retrieves a user by their email
func GetUserByEmail(email string) (*User, error) {
	var user User
	err := database.GetDB().QueryRow(
		"SELECT id, email, password_hash, created_at, updated_at FROM users WHERE email = ?",
		email,
	).Scan(&user.ID, &user.Email, &user.PasswordHash, &user.CreatedAt, &user.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Authenticate verifies the user's credentials
func Authenticate(email, password string) (*User, error) {
	user, err := GetUserByEmail(email)
	if err != nil {
		return nil, err
	}

	err = bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	if err != nil {
		return nil, ErrInvalidPassword
	}

	return user, nil
}
