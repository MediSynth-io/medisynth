package database

import (
	"database/sql"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/MediSynth-io/medisynth/internal/models"
	_ "github.com/mattn/go-sqlite3"
)

var dbConn *sql.DB

// Init initializes the database connection and schema
func Init(cfg *config.Config) error {
	if dbConn != nil {
		return nil
	}

	// Ensure data directory exists
	dataDir := filepath.Dir(cfg.Database.Path)
	if err := createDataDir(dataDir); err != nil {
		return err
	}

	// Open database connection
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=on", cfg.Database.Path)
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	// Initialize schema
	if err = initSchema(db); err != nil {
		db.Close()
		return fmt.Errorf("failed to initialize schema: %v", err)
	}

	dbConn = db
	log.Printf("Database initialized at %s", cfg.Database.Path)
	return nil
}

// GetConnection returns the database connection
func GetConnection() *sql.DB {
	return dbConn
}

// initSchema creates the database schema if it doesn't exist
func initSchema(db *sql.DB) error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			updated_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			created_at DATETIME NOT NULL,
			expires_at DATETIME,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL,
			token TEXT UNIQUE NOT NULL,
			created_at DATETIME NOT NULL,
			expires_at DATETIME NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)`,
		`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
		`CREATE INDEX IF NOT EXISTS idx_tokens_user_id ON tokens(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token)`,
	}

	for _, query := range queries {
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute schema query: %v", err)
		}
	}
	return nil
}

// createDataDir ensures the data directory exists
func createDataDir(dir string) error {
	// Directory creation is handled by Docker volume mount
	return nil
}

// CreateUser creates a new user
func CreateUser(email, password string) (*models.User, error) {
	now := time.Now()
	user := &models.User{
		ID:        generateID(),
		Email:     email,
		Password:  password,
		CreatedAt: now,
		UpdatedAt: now,
	}

	_, err := dbConn.Exec(
		"INSERT INTO users (id, email, password, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
		user.ID, user.Email, user.Password, user.CreatedAt, user.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}

	return user, nil
}

// GetUserByEmail retrieves a user by email
func GetUserByEmail(email string) (*models.User, error) {
	user := &models.User{}
	err := dbConn.QueryRow(
		"SELECT id, email, password, created_at, updated_at FROM users WHERE email = ?",
		email,
	).Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return user, nil
}

// CreateToken creates a new API token
func CreateToken(userID string, name, token string, expiresAt *time.Time) (*models.Token, error) {
	t := &models.Token{
		ID:        generateID(),
		UserID:    userID,
		Token:     token,
		Name:      name,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	_, err := dbConn.Exec(
		"INSERT INTO tokens (id, user_id, token, name, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)",
		t.ID, t.UserID, t.Token, t.Name, t.CreatedAt, t.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}

	return t, nil
}

// GetTokenByValue retrieves a token by its value
func GetTokenByValue(token string) (*models.Token, error) {
	t := &models.Token{}
	err := dbConn.QueryRow(
		"SELECT id, user_id, token, name, created_at, expires_at FROM tokens WHERE token = ?",
		token,
	).Scan(&t.ID, &t.UserID, &t.Token, &t.Name, &t.CreatedAt, &t.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// DeleteToken deletes a token
func DeleteToken(userID string, tokenID string) error {
	result, err := dbConn.Exec("DELETE FROM tokens WHERE id = ? AND user_id = ?", tokenID, userID)
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
func GetUserTokens(userID string) ([]*models.Token, error) {
	rows, err := dbConn.Query(
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

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return tokens, nil
}

// generateID generates a unique ID
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
