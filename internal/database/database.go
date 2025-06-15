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
	"github.com/MediSynth-io/medisynth/internal/models"
	_ "github.com/lib/pq" // PostgreSQL driver
	_ "github.com/mattn/go-sqlite3"
)

var dbConn *sql.DB
var dbType string

// Init initializes the database connection and schema
func Init(cfg *config.Config) error {
	if dbConn != nil {
		return nil
	}

	log.Printf("=== DATABASE INITIALIZATION DEBUG ===")
	log.Printf("Database type: %s", cfg.DatabaseType)

	var db *sql.DB
	var err error

	switch cfg.DatabaseType {
	case "postgres":
		db, err = initPostgreSQL(cfg)
	case "sqlite", "":
		db, err = initSQLite(cfg)
	default:
		return fmt.Errorf("unsupported database type: %s", cfg.DatabaseType)
	}

	if err != nil {
		return err
	}

	// Test the connection
	if err := db.Ping(); err != nil {
		db.Close()
		return fmt.Errorf("failed to ping database: %v", err)
	}

	// Check existing data BEFORE schema initialization
	log.Printf("Checking existing database contents...")
	if err := debugExistingData(db); err != nil {
		log.Printf("Warning: Could not check existing data: %v", err)
	}

	// Initialize schema
	log.Printf("Initializing database schema")
	if err = initSchema(db, cfg.DatabaseType); err != nil {
		db.Close()
		return fmt.Errorf("failed to initialize schema: %v", err)
	}

	// Check data AFTER schema initialization
	log.Printf("Checking database contents after schema init...")
	if err := debugExistingData(db); err != nil {
		log.Printf("Warning: Could not check data after init: %v", err)
	}

	dbConn = db
	dbType = cfg.DatabaseType
	log.Printf("=== DATABASE INITIALIZED SUCCESSFULLY ===")
	return nil
}

// initPostgreSQL initializes PostgreSQL connection
func initPostgreSQL(cfg *config.Config) (*sql.DB, error) {
	log.Printf("Initializing PostgreSQL connection...")
	log.Printf("Host: %s, Port: %s, Database: %s, User: %s",
		cfg.DatabaseHost, cfg.DatabasePort, cfg.DatabaseName, cfg.DatabaseUser)

	// Build connection string
	connStr := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=%s",
		cfg.DatabaseHost,
		cfg.DatabasePort,
		cfg.DatabaseUser,
		cfg.DatabasePassword,
		cfg.DatabaseName,
		cfg.DatabaseSSLMode,
	)

	log.Printf("Connecting to PostgreSQL...")
	db, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("failed to open PostgreSQL connection: %v", err)
	}

	// Configure connection pool
	if cfg.DatabaseMaxConns > 0 {
		db.SetMaxOpenConns(cfg.DatabaseMaxConns)
	}
	if cfg.DatabaseMaxIdle > 0 {
		db.SetMaxIdleConns(cfg.DatabaseMaxIdle)
	}
	if cfg.DatabaseConnMaxLifetime != "" && cfg.DatabaseConnMaxLifetime != "0" {
		if duration, err := time.ParseDuration(cfg.DatabaseConnMaxLifetime); err == nil {
			db.SetConnMaxLifetime(duration)
		}
	}

	log.Printf("PostgreSQL connection configured successfully")
	return db, nil
}

// initSQLite initializes SQLite connection
func initSQLite(cfg *config.Config) (*sql.DB, error) {
	log.Printf("Initializing SQLite connection at path: %s", cfg.DatabasePath)

	// Check if database file exists before we open it
	if stat, err := os.Stat(cfg.DatabasePath); err == nil {
		log.Printf("Database file EXISTS - Size: %d bytes, Modified: %v", stat.Size(), stat.ModTime())
	} else {
		log.Printf("Database file does NOT exist yet: %v", err)
	}

	// Ensure data directory exists
	dataDir := filepath.Dir(cfg.DatabasePath)
	log.Printf("Ensuring data directory exists: %s", dataDir)

	if err := createDataDir(dataDir); err != nil {
		return nil, fmt.Errorf("failed to create data directory: %w", err)
	}

	// Check if we can write to the directory
	if err := checkWritePermissions(dataDir); err != nil {
		return nil, fmt.Errorf("insufficient permissions for data directory %s: %w", dataDir, err)
	}

	// Open database connection
	dsn := fmt.Sprintf("%s?_journal_mode=WAL&_foreign_keys=on", cfg.DatabasePath)
	log.Printf("Opening SQLite database with DSN: %s", dsn)

	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to open SQLite database: %v", err)
	}

	log.Printf("SQLite connection opened successfully")
	return db, nil
}

// GetConnection returns the database connection
func GetConnection() *sql.DB {
	return dbConn
}

// initSchema creates the database schema if it doesn't exist
func initSchema(db *sql.DB, dbType string) error {
	var queries []string

	if dbType == "postgres" {
		// PostgreSQL schema with UUIDs and proper types
		queries = []string{
			`CREATE EXTENSION IF NOT EXISTS "uuid-ossp"`,
			`CREATE TABLE IF NOT EXISTS users (
				id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
				email VARCHAR(255) UNIQUE NOT NULL,
				password VARCHAR(255) NOT NULL,
				created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
			)`,
			`CREATE TABLE IF NOT EXISTS tokens (
				id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				token VARCHAR(255) UNIQUE NOT NULL,
				name VARCHAR(255) NOT NULL,
				created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
				expires_at TIMESTAMP WITH TIME ZONE
			)`,
			`CREATE TABLE IF NOT EXISTS sessions (
				id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				token VARCHAR(255) UNIQUE NOT NULL,
				created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
				expires_at TIMESTAMP WITH TIME ZONE NOT NULL
			)`,
			`CREATE TABLE IF NOT EXISTS jobs (
				id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				job_id VARCHAR(255) NOT NULL UNIQUE,
				status VARCHAR(50) NOT NULL,
				parameters JSONB,
				output_format VARCHAR(50),
				output_path TEXT,
				output_size BIGINT,
				patient_count INTEGER,
				error_message TEXT,
				created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
				completed_at TIMESTAMP WITH TIME ZONE
			)`,
			`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
			`CREATE INDEX IF NOT EXISTS idx_tokens_user_id ON tokens(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_tokens_token ON tokens(token)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`,
			`CREATE INDEX IF NOT EXISTS idx_jobs_user_id ON jobs(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status)`,
		}
	} else {
		// SQLite schema (original)
		queries = []string{
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
			`CREATE TABLE IF NOT EXISTS jobs (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				job_id TEXT NOT NULL,
				status TEXT NOT NULL,
				parameters TEXT,
				output_format TEXT,
				output_path TEXT,
				output_size INTEGER,
				patient_count INTEGER,
				error_message TEXT,
				created_at DATETIME NOT NULL,
				completed_at DATETIME,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
			`CREATE INDEX IF NOT EXISTS idx_tokens_user_id ON tokens(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token)`,
		}
	}

	for _, query := range queries {
		log.Printf("Executing schema query: %s", query[:min(len(query), 80)]+"...")
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute schema query: %v", err)
		}
	}
	return nil
}

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
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
	user := &models.User{
		Email:    email,
		Password: password,
	}

	if dbType == "postgres" {
		// PostgreSQL with UUID auto-generation
		err := dbConn.QueryRow(
			"INSERT INTO users (email, password) VALUES ($1, $2) RETURNING id, created_at, updated_at",
			user.Email, user.Password,
		).Scan(&user.ID, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return nil, err
		}
	} else {
		// SQLite with manual ID generation
		now := time.Now()
		user.ID = GenerateID()
		user.CreatedAt = now
		user.UpdatedAt = now

		_, err := dbConn.Exec(
			"INSERT INTO users (id, email, password, created_at, updated_at) VALUES (?, ?, ?, ?, ?)",
			user.ID, user.Email, user.Password, user.CreatedAt, user.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
	}

	return user, nil
}

// GetUserByEmail retrieves a user by email
func GetUserByEmail(email string) (*models.User, error) {
	user := &models.User{}
	var err error

	if dbType == "postgres" {
		err = dbConn.QueryRow(
			"SELECT id, email, password, created_at, updated_at FROM users WHERE email = $1",
			email,
		).Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	} else {
		err = dbConn.QueryRow(
			"SELECT id, email, password, created_at, updated_at FROM users WHERE email = ?",
			email,
		).Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	}

	if err != nil {
		return nil, err
	}
	return user, nil
}

// GetUserByID retrieves a user by their ID
func GetUserByID(id string) (*models.User, error) {
	user := &models.User{}
	var err error

	if dbType == "postgres" {
		err = dbConn.QueryRow(
			"SELECT id, email, password, created_at, updated_at FROM users WHERE id = $1",
			id,
		).Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	} else {
		err = dbConn.QueryRow(
			"SELECT id, email, password, created_at, updated_at FROM users WHERE id = ?",
			id,
		).Scan(&user.ID, &user.Email, &user.Password, &user.CreatedAt, &user.UpdatedAt)
	}

	if err != nil {
		return nil, err
	}
	return user, nil
}

// CreateToken creates a new API token
func CreateToken(userID, name, token string, expiresAt *time.Time) (*models.Token, error) {
	t := &models.Token{
		UserID:    userID,
		Token:     token,
		Name:      name,
		ExpiresAt: expiresAt,
	}

	if dbType == "postgres" {
		err := dbConn.QueryRow(
			"INSERT INTO tokens (user_id, token, name, expires_at) VALUES ($1, $2, $3, $4) RETURNING id, created_at",
			t.UserID, t.Token, t.Name, t.ExpiresAt,
		).Scan(&t.ID, &t.CreatedAt)
		if err != nil {
			return nil, err
		}
	} else {
		t.ID = GenerateID()
		t.CreatedAt = time.Now()
		_, err := dbConn.Exec(
			"INSERT INTO tokens (id, user_id, token, name, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)",
			t.ID, t.UserID, t.Token, t.Name, t.CreatedAt, t.ExpiresAt,
		)
		if err != nil {
			return nil, err
		}
	}

	return t, nil
}

// GetTokenByValue retrieves a token by its value
func GetTokenByValue(token string) (*models.Token, error) {
	t := &models.Token{}
	var query string
	if dbType == "postgres" {
		query = "SELECT id, user_id, token, name, created_at, expires_at FROM tokens WHERE token = $1"
	} else {
		query = "SELECT id, user_id, token, name, created_at, expires_at FROM tokens WHERE token = ?"
	}
	err := dbConn.QueryRow(query, token).Scan(&t.ID, &t.UserID, &t.Token, &t.Name, &t.CreatedAt, &t.ExpiresAt)
	if err != nil {
		return nil, err
	}
	return t, nil
}

// DeleteToken deletes a token
func DeleteToken(userID string, tokenID string) error {
	var query string
	if dbType == "postgres" {
		query = "DELETE FROM tokens WHERE id = $1 AND user_id = $2"
	} else {
		query = "DELETE FROM tokens WHERE id = ? AND user_id = ?"
	}
	result, err := dbConn.Exec(query, tokenID, userID)
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
	var query string
	if dbType == "postgres" {
		query = "SELECT id, user_id, token, name, created_at, expires_at FROM tokens WHERE user_id = $1"
	} else {
		query = "SELECT id, user_id, token, name, created_at, expires_at FROM tokens WHERE user_id = ?"
	}
	rows, err := dbConn.Query(query, userID)
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
func CreateSession(userID string, token string, expiresAt time.Time) error {
	query := `INSERT INTO sessions (id, user_id, token, expires_at) VALUES ($1, $2, $3, $4)`
	_, err := dbConn.Exec(query, GenerateID(), userID, token, expiresAt)
	return err
}

// ValidateSession retrieves a user by session token
func ValidateSession(token string) (*models.Session, error) {
	var session models.Session
	query := `SELECT id, user_id, token, created_at, expires_at FROM sessions WHERE token = $1`
	err := dbConn.QueryRow(query, token).Scan(&session.ID, &session.UserID, &session.Token, &session.CreatedAt, &session.ExpiresAt)
	if err != nil {
		return nil, err
	}
	// Check for expiration
	if session.ExpiresAt.Before(time.Now()) {
		// Optionally, delete the expired session
		DeleteSession(token)
		return nil, errors.New("session expired")
	}
	return &session, nil
}

// DeleteSession deletes a session by its token
func DeleteSession(token string) error {
	query := `DELETE FROM sessions WHERE token = $1`
	_, err := dbConn.Exec(query, token)
	return err
}

// CleanupExpiredSessions deletes all expired sessions
func CleanupExpiredSessions() error {
	var query string
	if dbType == "postgres" {
		query = "DELETE FROM sessions WHERE expires_at < $1"
	} else {
		query = "DELETE FROM sessions WHERE expires_at < ?"
	}
	_, err := dbConn.Exec(query, time.Now())
	return err
}

// GenerateID generates a unique ID for SQLite, not needed for PostgreSQL
func GenerateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// debugExistingData checks and logs existing database contents
func debugExistingData(db *sql.DB) error {
	// Check if tables exist
	tables := []string{"users", "tokens", "sessions"}
	for _, table := range tables {
		var count int
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s", table)
		err := db.QueryRow(query).Scan(&count)
		if err != nil {
			if err.Error() == "no such table: "+table {
				log.Printf("Table '%s' does not exist yet", table)
			} else {
				log.Printf("Error checking table '%s': %v", table, err)
			}
		} else {
			log.Printf("Table '%s' has %d records", table, count)
		}
	}
	return nil
}
