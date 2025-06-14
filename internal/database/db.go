package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/MediSynth-io/medisynth/internal/config"
	_ "github.com/mattn/go-sqlite3"
)

var (
	db *sql.DB
)

// Init initializes the SQLite database connection
func Init() error {
	// Load configuration
	cfg, err := config.LoadConfig("app.yml")
	if err != nil {
		return fmt.Errorf("failed to load config: %v", err)
	}

	// Create data directory if it doesn't exist
	dataDir := filepath.Dir(cfg.Database.Path)
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return fmt.Errorf("failed to create data directory: %v", err)
	}

	// Try to connect to the database with retries
	var lastErr error
	for i := 0; i < cfg.Database.MaxRetries; i++ {
		// Open database connection with WAL mode if enabled
		dsn := cfg.Database.Path
		if cfg.Database.WALMode {
			dsn += "?_journal=WAL"
		}
		db, err = sql.Open("sqlite3", dsn)
		if err != nil {
			lastErr = fmt.Errorf("failed to open database: %v", err)
			log.Printf("Attempt %d/%d failed: %v", i+1, cfg.Database.MaxRetries, lastErr)
			time.Sleep(time.Duration(cfg.Database.RetryDelay) * time.Second)
			continue
		}

		// Test the connection
		if err := db.Ping(); err != nil {
			lastErr = fmt.Errorf("failed to ping database: %v", err)
			log.Printf("Attempt %d/%d failed: %v", i+1, cfg.Database.MaxRetries, lastErr)
			time.Sleep(time.Duration(cfg.Database.RetryDelay) * time.Second)
			continue
		}

		// If we get here, the connection was successful
		break
	}

	if lastErr != nil {
		return fmt.Errorf("failed to connect to database after %d attempts: %v", cfg.Database.MaxRetries, lastErr)
	}

	// Set connection pool settings
	db.SetMaxOpenConns(1) // SQLite only supports one writer
	db.SetMaxIdleConns(1)
	db.SetConnMaxLifetime(time.Hour)

	// Initialize tables
	if err := initTables(); err != nil {
		return fmt.Errorf("failed to initialize tables: %v", err)
	}

	log.Printf("Database initialized successfully at %s (WAL mode: %v)", cfg.Database.Path, cfg.Database.WALMode)
	return nil
}

// GetDB returns the database connection
func GetDB() *sql.DB {
	return db
}

// Close closes the database connection
func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

// initTables creates the necessary tables if they don't exist
func initTables() error {
	// Create users table
	_, err := db.Exec(`
		CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create users table: %v", err)
	}

	// Create api_tokens table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS api_tokens (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token TEXT UNIQUE NOT NULL,
			name TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create api_tokens table: %v", err)
	}

	// Create sessions table
	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS sessions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER NOT NULL,
			token TEXT UNIQUE NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL,
			FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
		)
	`)
	if err != nil {
		return fmt.Errorf("failed to create sessions table: %v", err)
	}

	// Create indexes
	_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
		CREATE INDEX IF NOT EXISTS idx_api_tokens_token ON api_tokens(token);
		CREATE INDEX IF NOT EXISTS idx_api_tokens_user_id ON api_tokens(user_id);
		CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
		CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
		CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
	`)
	if err != nil {
		return fmt.Errorf("failed to create indexes: %v", err)
	}

	return nil
}

// User functions
func CreateUser(email, password string) (*User, error) {
	result, err := db.Exec(
		"INSERT INTO users (email, password_hash) VALUES (?, ?)",
		email, password,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return GetUserByID(id)
}

func GetUserByID(id int64) (*User, error) {
	var user User
	err := db.QueryRow(
		"SELECT id, email, password_hash, created_at, updated_at FROM users WHERE id = ?",
		id,
	).Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

func GetUserByEmail(email string) (*User, error) {
	var user User
	err := db.QueryRow(
		"SELECT id, email, password_hash, created_at, updated_at FROM users WHERE email = ?",
		email,
	).Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Token functions
func CreateToken(userID int64, name, token string, expiresAt *time.Time) (*Token, error) {
	result, err := db.Exec(
		"INSERT INTO api_tokens (user_id, name, token, expires_at) VALUES (?, ?, ?, ?)",
		userID, name, token, expiresAt,
	)
	if err != nil {
		return nil, err
	}

	id, err := result.LastInsertId()
	if err != nil {
		return nil, err
	}

	return GetTokenByID(id)
}

func GetTokenByID(id int64) (*Token, error) {
	var token Token
	err := db.QueryRow(
		"SELECT id, user_id, name, token, created_at, expires_at FROM api_tokens WHERE id = ?",
		id,
	).Scan(&token.ID, &token.UserID, &token.Name, &token.Token, &token.CreatedAt, &token.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &token, nil
}

func GetTokenByValue(token string) (*Token, error) {
	var tokenObj Token
	err := db.QueryRow(
		"SELECT id, user_id, name, token, created_at, expires_at FROM api_tokens WHERE token = ?",
		token,
	).Scan(&tokenObj.ID, &tokenObj.UserID, &tokenObj.Name, &tokenObj.Token, &tokenObj.CreatedAt, &tokenObj.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &tokenObj, nil
}

func GetUserTokens(userID int64) ([]*Token, error) {
	rows, err := db.Query(
		"SELECT id, user_id, name, token, created_at, expires_at FROM api_tokens WHERE user_id = ?",
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*Token
	for rows.Next() {
		var token Token
		err := rows.Scan(&token.ID, &token.UserID, &token.Name, &token.Token, &token.CreatedAt, &token.ExpiresAt)
		if err != nil {
			return nil, err
		}
		tokens = append(tokens, &token)
	}
	return tokens, nil
}

func DeleteToken(userID int64, tokenID string) error {
	result, err := db.Exec(
		"DELETE FROM api_tokens WHERE id = ? AND user_id = ?",
		tokenID, userID,
	)
	if err != nil {
		return err
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return errors.New("token not found or not owned by user")
	}
	return nil
}

// Session functions
func CreateSession(userID int64, token string, expiresAt time.Time) error {
	_, err := db.Exec(
		"INSERT INTO sessions (user_id, token, expires_at) VALUES (?, ?, ?)",
		userID, token, expiresAt,
	)
	return err
}

func GetSessionByToken(token string) (*Session, error) {
	var session Session
	err := db.QueryRow(
		"SELECT id, user_id, token, created_at, expires_at FROM sessions WHERE token = ?",
		token,
	).Scan(&session.ID, &session.UserID, &session.Token, &session.CreatedAt, &session.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func DeleteSession(token string) error {
	_, err := db.Exec("DELETE FROM sessions WHERE token = ?", token)
	return err
}

func CleanupExpiredSessions() error {
	_, err := db.Exec("DELETE FROM sessions WHERE expires_at < CURRENT_TIMESTAMP")
	return err
}
