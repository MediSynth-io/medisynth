package database

import (
	"database/sql"
	"fmt"
	"log"
	"os"
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

	log.Printf("Initializing database at path: %s", cfg.DatabasePath)

	// Ensure data directory exists
	dataDir := filepath.Dir(cfg.DatabasePath)
	log.Printf("Ensuring data directory exists: %s", dataDir)

	if err := createDataDir(dataDir); err != nil {
		return fmt.Errorf("failed to create data directory: %w", err)
	}

	// Check if we can write to the directory
	if err := checkWritePermissions(dataDir); err != nil {
		return fmt.Errorf("insufficient permissions for data directory %s: %w", dataDir, err)
	}

	// Open database connection
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=on", cfg.DatabasePath)
	log.Printf("Opening database connection with DSN: %s", dsn)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return fmt.Errorf("failed to open database: %v", err)
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %v", err)
	}

	// Initialize schema
	log.Printf("Initializing database schema")
	if err = initSchema(db); err != nil {
		db.Close()
		return fmt.Errorf("failed to initialize schema: %v", err)
	}

	dbConn = db
	log.Printf("Database initialized successfully at %s", cfg.DatabasePath)
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

// createDataDir ensures the data directory exists with proper permissions
func createDataDir(dir string) error {
	// Check if directory already exists
	if stat, err := os.Stat(dir); err == nil {
		if !stat.IsDir() {
			return fmt.Errorf("path %s exists but is not a directory", dir)
		}
		log.Printf("Data directory already exists: %s", dir)
		return nil
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("failed to stat directory %s: %w", dir, err)
	}

	// Directory doesn't exist, create it
	log.Printf("Creating data directory: %s", dir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", dir, err)
	}

	log.Printf("Data directory created successfully: %s", dir)
	return nil
}

// checkWritePermissions verifies that we can write to the directory
func checkWritePermissions(dir string) error {
	testFile := filepath.Join(dir, ".write_test")

	// Try to create a test file
	file, err := os.Create(testFile)
	if err != nil {
		return fmt.Errorf("cannot create test file: %w", err)
	}
	file.Close()

	// Try to remove the test file
	if err := os.Remove(testFile); err != nil {
		log.Printf("Warning: failed to remove test file %s: %v", testFile, err)
	}

	log.Printf("Write permissions verified for directory: %s", dir)
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

// GetUserByID retrieves a user by their ID
func GetUserByID(id string) (*models.User, error) {
	user := &models.User{}
	err := dbConn.QueryRow(
		"SELECT id, email, password, created_at, updated_at FROM users WHERE id = ?",
		id,
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

// CreateSession creates a new session for a user
func CreateSession(userID string, token string, expiresAt time.Time) (*models.Session, error) {
	session := &models.Session{
		ID:        generateID(),
		UserID:    userID,
		Token:     token,
		CreatedAt: time.Now(),
		ExpiresAt: expiresAt,
	}

	_, err := dbConn.Exec(
		"INSERT INTO sessions (id, user_id, token, created_at, expires_at) VALUES (?, ?, ?, ?, ?)",
		session.ID, session.UserID, session.Token, session.CreatedAt, session.ExpiresAt,
	)
	if err != nil {
		return nil, err
	}

	return session, nil
}

// GetSessionByToken retrieves a session by its token
func GetSessionByToken(token string) (*models.Session, error) {
	session := &models.Session{}
	err := dbConn.QueryRow(
		"SELECT id, user_id, token, created_at, expires_at FROM sessions WHERE token = ?",
		token,
	).Scan(&session.ID, &session.UserID, &session.Token, &session.CreatedAt, &session.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return session, nil
}

// DeleteSession deletes a session
func DeleteSession(token string) error {
	_, err := dbConn.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

// CleanupExpiredSessions deletes all expired sessions
func CleanupExpiredSessions() error {
	_, err := dbConn.Exec("DELETE FROM sessions WHERE expires_at < ?", time.Now())
	return err
}

// generateID generates a unique ID
func generateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}
