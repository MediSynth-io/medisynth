package database

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
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

	// Run migrations
	log.Printf("Running database migrations")
	if err = RunMigrations(db, cfg.DatabaseType); err != nil {
		db.Close()
		return fmt.Errorf("failed to run migrations: %v", err)
	}

	// Check data AFTER schema initialization
	log.Printf("Checking database contents after schema init...")
	if err := debugExistingData(db); err != nil {
		log.Printf("Warning: Could not check data after init: %v", err)
	}

	dbConn = db
	dbType = cfg.DatabaseType
	log.Printf("=== DATABASE INITIALIZED SUCCESSFULLY ===")
	log.Printf("Database type set to: %s", dbType)
	log.Printf("Database connection established: %v", dbConn != nil)

	// Test basic database functionality
	var testCount int
	testQuery := "SELECT COUNT(*) FROM sessions"
	err = dbConn.QueryRow(testQuery).Scan(&testCount)
	if err != nil {
		log.Printf("WARNING: Could not query sessions table: %v", err)
	} else {
		log.Printf("Sessions table accessible, current count: %d", testCount)
	}

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
				is_admin BOOLEAN NOT NULL DEFAULT FALSE,
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
			`CREATE TABLE IF NOT EXISTS orders (
				id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				order_number VARCHAR(50) NOT NULL UNIQUE,
				description TEXT NOT NULL,
				amount_usd DECIMAL(10,2) NOT NULL,
				amount_btc DECIMAL(16,8),
				btc_address VARCHAR(255) NOT NULL,
				qr_code_data TEXT,
				status VARCHAR(50) NOT NULL DEFAULT 'pending',
				payment_received_at TIMESTAMP WITH TIME ZONE,
				transaction_hash VARCHAR(255),
				confirmations INTEGER DEFAULT 0,
				expires_at TIMESTAMP WITH TIME ZONE,
				created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
			)`,
			`CREATE TABLE IF NOT EXISTS payments (
				id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
				order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
				transaction_hash VARCHAR(255) NOT NULL,
				amount_btc DECIMAL(16,8) NOT NULL,
				confirmations INTEGER NOT NULL DEFAULT 0,
				status VARCHAR(50) NOT NULL DEFAULT 'pending',
				detected_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
				confirmed_at TIMESTAMP WITH TIME ZONE,
				created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
			)`,
			`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
			`CREATE INDEX IF NOT EXISTS idx_tokens_user_id ON tokens(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_tokens_token ON tokens(token)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at)`,
			`CREATE INDEX IF NOT EXISTS idx_jobs_user_id ON jobs(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status)`,
			`CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status)`,
			`CREATE INDEX IF NOT EXISTS idx_orders_order_number ON orders(order_number)`,
			`CREATE INDEX IF NOT EXISTS idx_payments_order_id ON payments(order_id)`,
			`CREATE INDEX IF NOT EXISTS idx_payments_transaction_hash ON payments(transaction_hash)`,
		}
	} else {
		// SQLite schema (original)
		queries = []string{
			`CREATE TABLE IF NOT EXISTS users (
				id TEXT PRIMARY KEY,
				email TEXT UNIQUE NOT NULL,
				password TEXT NOT NULL,
				is_admin BOOLEAN NOT NULL DEFAULT 0,
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
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
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
			`CREATE TABLE IF NOT EXISTS orders (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				order_number TEXT NOT NULL UNIQUE,
				description TEXT NOT NULL,
				amount_usd REAL NOT NULL,
				amount_btc REAL,
				btc_address TEXT NOT NULL,
				qr_code_data TEXT,
				status TEXT NOT NULL DEFAULT 'pending',
				payment_received_at DATETIME,
				transaction_hash TEXT,
				confirmations INTEGER DEFAULT 0,
				expires_at DATETIME,
				created_at DATETIME NOT NULL,
				updated_at DATETIME NOT NULL,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			)`,
			`CREATE TABLE IF NOT EXISTS payments (
				id TEXT PRIMARY KEY,
				order_id TEXT NOT NULL,
				transaction_hash TEXT NOT NULL,
				amount_btc REAL NOT NULL,
				confirmations INTEGER NOT NULL DEFAULT 0,
				status TEXT NOT NULL DEFAULT 'pending',
				detected_at DATETIME NOT NULL,
				confirmed_at DATETIME,
				created_at DATETIME NOT NULL,
				FOREIGN KEY (order_id) REFERENCES orders(id) ON DELETE CASCADE
			)`,
			`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
			`CREATE INDEX IF NOT EXISTS idx_tokens_user_id ON tokens(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token)`,
			`CREATE INDEX IF NOT EXISTS idx_jobs_user_id ON jobs(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status)`,
			`CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id)`,
			`CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status)`,
			`CREATE INDEX IF NOT EXISTS idx_orders_order_number ON orders(order_number)`,
			`CREATE INDEX IF NOT EXISTS idx_payments_order_id ON payments(order_id)`,
			`CREATE INDEX IF NOT EXISTS idx_payments_transaction_hash ON payments(transaction_hash)`,
		}
	}

	for _, query := range queries {
		log.Printf("Executing schema query: %s", query[:min(len(query), 80)]+"...")
		if _, err := db.Exec(query); err != nil {
			return fmt.Errorf("failed to execute schema query: %v", err)
		}
	}

	// Run migrations for existing databases
	if err := runMigrations(db, dbType); err != nil {
		log.Printf("Warning: Migration failed: %v", err)
		// Don't fail initialization, just log the warning
	}

	return nil
}

// runMigrations runs database migrations for existing databases
func runMigrations(db *sql.DB, dbType string) error {
	log.Printf("Running database migrations...")

	// Migration 1: Add is_admin column if it doesn't exist
	if err := addIsAdminColumn(db, dbType); err != nil {
		return fmt.Errorf("failed to add is_admin column: %v", err)
	}

	log.Printf("Database migrations completed successfully")
	return nil
}

// addIsAdminColumn adds the is_admin column if it doesn't exist
func addIsAdminColumn(db *sql.DB, dbType string) error {
	log.Printf("Checking if is_admin column exists...")

	// Check if column already exists
	var columnExists bool
	if dbType == "postgres" {
		var count int
		err := db.QueryRow(`
			SELECT COUNT(*) 
			FROM information_schema.columns 
			WHERE table_name = 'users' AND column_name = 'is_admin'
		`).Scan(&count)
		if err != nil {
			return err
		}
		columnExists = count > 0
	} else {
		// For SQLite, we'll just try to add the column and ignore errors if it exists
		columnExists = false
	}

	if columnExists {
		log.Printf("is_admin column already exists, skipping migration")
		return nil
	}

	log.Printf("Adding is_admin column to users table...")

	var query string
	if dbType == "postgres" {
		query = "ALTER TABLE users ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT FALSE"
	} else {
		query = "ALTER TABLE users ADD COLUMN is_admin BOOLEAN NOT NULL DEFAULT 0"
	}

	_, err := db.Exec(query)
	if err != nil {
		// For SQLite, the column might already exist, so we'll check for specific error
		if strings.Contains(err.Error(), "duplicate column name") {
			log.Printf("is_admin column already exists (SQLite), skipping migration")
			return nil
		}
		return err
	}

	log.Printf("Successfully added is_admin column")
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
			"SELECT id, email, password, is_admin, created_at, updated_at FROM users WHERE email = $1",
			email,
		).Scan(&user.ID, &user.Email, &user.Password, &user.IsAdmin, &user.CreatedAt, &user.UpdatedAt)
	} else {
		err = dbConn.QueryRow(
			"SELECT id, email, password, is_admin, created_at, updated_at FROM users WHERE email = ?",
			email,
		).Scan(&user.ID, &user.Email, &user.Password, &user.IsAdmin, &user.CreatedAt, &user.UpdatedAt)
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
			"SELECT id, email, password, is_admin, created_at, updated_at FROM users WHERE id = $1",
			id,
		).Scan(&user.ID, &user.Email, &user.Password, &user.IsAdmin, &user.CreatedAt, &user.UpdatedAt)
	} else {
		err = dbConn.QueryRow(
			"SELECT id, email, password, is_admin, created_at, updated_at FROM users WHERE id = ?",
			id,
		).Scan(&user.ID, &user.Email, &user.Password, &user.IsAdmin, &user.CreatedAt, &user.UpdatedAt)
	}

	if err != nil {
		return nil, err
	}
	return user, nil
}

// MakeUserAdmin makes a user an admin
func MakeUserAdmin(userID string) error {
	var query string
	if dbType == "postgres" {
		query = "UPDATE users SET is_admin = true WHERE id = $1"
	} else {
		query = "UPDATE users SET is_admin = 1 WHERE id = ?"
	}

	result, err := dbConn.Exec(query, userID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}

	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// GetAllUsers retrieves all users (admin only)
func GetAllUsers() ([]*models.User, error) {
	var query string
	if dbType == "postgres" {
		query = "SELECT id, email, password, is_admin, created_at, updated_at FROM users ORDER BY created_at DESC"
	} else {
		query = "SELECT id, email, password, is_admin, created_at, updated_at FROM users ORDER BY created_at DESC"
	}

	rows, err := dbConn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		user := &models.User{}
		err := rows.Scan(&user.ID, &user.Email, &user.Password, &user.IsAdmin, &user.CreatedAt, &user.UpdatedAt)
		if err != nil {
			return nil, err
		}
		users = append(users, user)
	}

	return users, nil
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
	log.Printf("[DB] Starting session creation - UserID: %s, TokenLength: %d, ExpiresAt: %v", userID, len(token), expiresAt)
	log.Printf("[DB] Database type: %s", dbType)
	log.Printf("[DB] Database connection status: %v", dbConn != nil)

	var query string
	var err error

	if dbType == "postgres" {
		log.Printf("[DB] Using PostgreSQL syntax with auto-generated UUID")
		query = `INSERT INTO sessions (user_id, token, expires_at) VALUES ($1, $2, $3)`
		log.Printf("[DB] PostgreSQL query: %s", query)
		log.Printf("[DB] PostgreSQL values - UserID: %s, Token: %s, ExpiresAt: %v",
			userID, token[:10]+"...", expiresAt)
		_, err = dbConn.Exec(query, userID, token, expiresAt)
	} else {
		log.Printf("[DB] Using SQLite syntax with manual ID generation")
		sessionID := GenerateID()
		query = `INSERT INTO sessions (id, user_id, token, expires_at) VALUES (?, ?, ?, ?)`
		log.Printf("[DB] SQLite query: %s", query)
		log.Printf("[DB] SQLite values - ID: %s, UserID: %s, Token: %s, ExpiresAt: %v",
			sessionID, userID, token[:10]+"...", expiresAt)
		_, err = dbConn.Exec(query, sessionID, userID, token, expiresAt)
	}

	if err != nil {
		log.Printf("[DB] Session creation failed: %v", err)
	} else {
		log.Printf("[DB] Session created successfully")
	}

	return err
}

// ValidateSession retrieves a user by session token
func ValidateSession(token string) (*models.Session, error) {
	var session models.Session
	var query string
	if dbType == "postgres" {
		query = `SELECT id, user_id, token, created_at, expires_at FROM sessions WHERE token = $1`
	} else {
		query = `SELECT id, user_id, token, created_at, expires_at FROM sessions WHERE token = ?`
	}
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
	var query string
	if dbType == "postgres" {
		query = `DELETE FROM sessions WHERE token = $1`
	} else {
		query = `DELETE FROM sessions WHERE token = ?`
	}
	_, err := dbConn.Exec(query, token)
	return err
}

// CleanupExpiredSessions removes all sessions that have passed their expiration time.
func CleanupExpiredSessions() error {
	var query string
	if dbType == "postgres" {
		query = `DELETE FROM sessions WHERE expires_at < $1`
	} else {
		query = `DELETE FROM sessions WHERE expires_at < ?`
	}
	_, err := dbConn.Exec(query, time.Now())
	return err
}

// GenerateID generates a unique ID for SQLite, not needed for PostgreSQL
func GenerateID() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// ============================================================================
// ORDER & PAYMENT FUNCTIONS
// ============================================================================

// GetUserCount returns the total number of users
func GetUserCount() (int, error) {
	var count int
	var query string
	if dbType == "postgres" {
		query = "SELECT COUNT(*) FROM users"
	} else {
		query = "SELECT COUNT(*) FROM users"
	}
	err := dbConn.QueryRow(query).Scan(&count)
	return count, err
}

// GetOrderCount returns the total number of orders
func GetOrderCount() (int, error) {
	var count int
	var query string
	if dbType == "postgres" {
		query = "SELECT COUNT(*) FROM orders"
	} else {
		query = "SELECT COUNT(*) FROM orders"
	}
	err := dbConn.QueryRow(query).Scan(&count)
	return count, err
}

// GetTotalRevenue returns the total revenue from confirmed orders
func GetTotalRevenue() (float64, error) {
	var revenue float64
	var query string
	if dbType == "postgres" {
		query = "SELECT COALESCE(SUM(amount_usd), 0) FROM orders WHERE status = 'confirmed'"
	} else {
		query = "SELECT COALESCE(SUM(amount_usd), 0) FROM orders WHERE status = 'confirmed'"
	}
	err := dbConn.QueryRow(query).Scan(&revenue)
	return revenue, err
}

// GetRecentOrders returns the most recent orders
func GetRecentOrders(limit int) ([]*models.Order, error) {
	var query string
	if dbType == "postgres" {
		query = `SELECT id, user_id, order_number, description, amount_usd, amount_btc, 
				btc_address, status, confirmations, created_at, updated_at 
				FROM orders ORDER BY created_at DESC LIMIT $1`
	} else {
		query = `SELECT id, user_id, order_number, description, amount_usd, amount_btc, 
				btc_address, status, confirmations, created_at, updated_at 
				FROM orders ORDER BY created_at DESC LIMIT ?`
	}

	rows, err := dbConn.Query(query, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []*models.Order
	for rows.Next() {
		order := &models.Order{}
		err := rows.Scan(&order.ID, &order.UserID, &order.OrderNumber, &order.Description,
			&order.AmountUSD, &order.AmountBTC, &order.BTCAddress, &order.Status,
			&order.Confirmations, &order.CreatedAt, &order.UpdatedAt)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, nil
}

// GetAllOrders returns all orders
func GetAllOrders() ([]*models.Order, error) {
	var query string
	if dbType == "postgres" {
		query = `SELECT id, user_id, order_number, description, amount_usd, amount_btc, 
				btc_address, status, confirmations, created_at, updated_at 
				FROM orders ORDER BY created_at DESC`
	} else {
		query = `SELECT id, user_id, order_number, description, amount_usd, amount_btc, 
				btc_address, status, confirmations, created_at, updated_at 
				FROM orders ORDER BY created_at DESC`
	}

	rows, err := dbConn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []*models.Order
	for rows.Next() {
		order := &models.Order{}
		err := rows.Scan(&order.ID, &order.UserID, &order.OrderNumber, &order.Description,
			&order.AmountUSD, &order.AmountBTC, &order.BTCAddress, &order.Status,
			&order.Confirmations, &order.CreatedAt, &order.UpdatedAt)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, nil
}

// GetAllPayments returns all payments
func GetAllPayments() ([]*models.Payment, error) {
	var query string
	if dbType == "postgres" {
		query = `SELECT id, order_id, transaction_hash, amount_btc, confirmations, 
				status, detected_at, created_at FROM payments ORDER BY detected_at DESC`
	} else {
		query = `SELECT id, order_id, transaction_hash, amount_btc, confirmations, 
				status, detected_at, created_at FROM payments ORDER BY detected_at DESC`
	}

	rows, err := dbConn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var payments []*models.Payment
	for rows.Next() {
		payment := &models.Payment{}
		err := rows.Scan(&payment.ID, &payment.OrderID, &payment.TransactionHash,
			&payment.AmountBTC, &payment.Confirmations, &payment.Status,
			&payment.DetectedAt, &payment.CreatedAt)
		if err != nil {
			return nil, err
		}
		payments = append(payments, payment)
	}

	return payments, nil
}

// CreateOrder creates a new order
func CreateOrder(userID, description string, amountUSD float64, btcAddress string) (*models.Order, error) {
	order := &models.Order{
		UserID:      userID,
		Description: description,
		AmountUSD:   amountUSD,
		BTCAddress:  btcAddress,
		Status:      models.OrderStatusPending,
	}

	// Generate order number
	order.OrderNumber = fmt.Sprintf("ORD-%d", time.Now().Unix())

	// Set expiration (24 hours from now)
	expiresAt := time.Now().Add(24 * time.Hour)
	order.ExpiresAt = &expiresAt

	if dbType == "postgres" {
		err := dbConn.QueryRow(`
			INSERT INTO orders (user_id, order_number, description, amount_usd, btc_address, expires_at)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING id, created_at, updated_at`,
			order.UserID, order.OrderNumber, order.Description, order.AmountUSD,
			order.BTCAddress, order.ExpiresAt,
		).Scan(&order.ID, &order.CreatedAt, &order.UpdatedAt)
		if err != nil {
			return nil, err
		}
	} else {
		order.ID = GenerateID()
		order.CreatedAt = time.Now()
		order.UpdatedAt = time.Now()

		_, err := dbConn.Exec(`
			INSERT INTO orders (id, user_id, order_number, description, amount_usd, btc_address, expires_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			order.ID, order.UserID, order.OrderNumber, order.Description, order.AmountUSD,
			order.BTCAddress, order.ExpiresAt, order.CreatedAt, order.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
	}

	return order, nil
}

// GetUserOrders returns all orders for a specific user
func GetUserOrders(userID string) ([]*models.Order, error) {
	var query string
	if dbType == "postgres" {
		query = `SELECT id, user_id, order_number, description, amount_usd, amount_btc, 
				btc_address, status, confirmations, created_at, updated_at 
				FROM orders WHERE user_id = $1 ORDER BY created_at DESC`
	} else {
		query = `SELECT id, user_id, order_number, description, amount_usd, amount_btc, 
				btc_address, status, confirmations, created_at, updated_at 
				FROM orders WHERE user_id = ? ORDER BY created_at DESC`
	}

	rows, err := dbConn.Query(query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []*models.Order
	for rows.Next() {
		order := &models.Order{}
		err := rows.Scan(&order.ID, &order.UserID, &order.OrderNumber, &order.Description,
			&order.AmountUSD, &order.AmountBTC, &order.BTCAddress, &order.Status,
			&order.Confirmations, &order.CreatedAt, &order.UpdatedAt)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, nil
}

// GetOrderByID returns an order by ID for a specific user
func GetOrderByID(orderID, userID string) (*models.Order, error) {
	order := &models.Order{}
	var query string
	if dbType == "postgres" {
		query = `SELECT id, user_id, order_number, description, amount_usd, amount_btc, 
				btc_address, qr_code_data, status, confirmations, created_at, updated_at,
				expires_at, payment_received_at, transaction_hash
				FROM orders WHERE id = $1 AND user_id = $2`
	} else {
		query = `SELECT id, user_id, order_number, description, amount_usd, amount_btc, 
				btc_address, qr_code_data, status, confirmations, created_at, updated_at,
				expires_at, payment_received_at, transaction_hash
				FROM orders WHERE id = ? AND user_id = ?`
	}

	err := dbConn.QueryRow(query, orderID, userID).Scan(
		&order.ID, &order.UserID, &order.OrderNumber, &order.Description,
		&order.AmountUSD, &order.AmountBTC, &order.BTCAddress, &order.QRCodeData,
		&order.Status, &order.Confirmations, &order.CreatedAt, &order.UpdatedAt,
		&order.ExpiresAt, &order.PaymentReceivedAt, &order.TransactionHash)

	if err != nil {
		return nil, err
	}

	return order, nil
}

// GetPendingOrders returns all orders with pending status
func GetPendingOrders() ([]*models.Order, error) {
	var query string
	if dbType == "postgres" {
		query = `SELECT id, user_id, order_number, description, amount_usd, amount_btc, 
				btc_address, qr_code_data, status, confirmations, created_at, updated_at,
				expires_at, payment_received_at, transaction_hash
				FROM orders WHERE status = 'pending' ORDER BY created_at DESC`
	} else {
		query = `SELECT id, user_id, order_number, description, amount_usd, amount_btc, 
				btc_address, qr_code_data, status, confirmations, created_at, updated_at,
				expires_at, payment_received_at, transaction_hash
				FROM orders WHERE status = 'pending' ORDER BY created_at DESC`
	}

	rows, err := dbConn.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var orders []*models.Order
	for rows.Next() {
		order := &models.Order{}
		err := rows.Scan(&order.ID, &order.UserID, &order.OrderNumber, &order.Description,
			&order.AmountUSD, &order.AmountBTC, &order.BTCAddress, &order.QRCodeData,
			&order.Status, &order.Confirmations, &order.CreatedAt, &order.UpdatedAt,
			&order.ExpiresAt, &order.PaymentReceivedAt, &order.TransactionHash)
		if err != nil {
			return nil, err
		}
		orders = append(orders, order)
	}

	return orders, nil
}

// UpdateOrderPayment updates an order with payment information
func UpdateOrderPayment(orderID string, txHash string, amountBTC float64, confirmations int) error {
	var query string
	now := time.Now()

	if dbType == "postgres" {
		query = `UPDATE orders SET 
				transaction_hash = $1, 
				amount_btc = $2, 
				confirmations = $3, 
				payment_received_at = $4,
				status = CASE 
					WHEN $3 >= 6 THEN 'confirmed'
					WHEN $3 > 0 THEN 'paid'
					ELSE 'pending'
				END,
				updated_at = $4
				WHERE id = $5`
	} else {
		query = `UPDATE orders SET 
				transaction_hash = ?, 
				amount_btc = ?, 
				confirmations = ?, 
				payment_received_at = ?,
				status = CASE 
					WHEN ? >= 6 THEN 'confirmed'
					WHEN ? > 0 THEN 'paid'
					ELSE 'pending'
				END,
				updated_at = ?
				WHERE id = ?`
	}

	if dbType == "postgres" {
		_, err := dbConn.Exec(query, txHash, amountBTC, confirmations, now, orderID)
		return err
	} else {
		_, err := dbConn.Exec(query, txHash, amountBTC, confirmations, now, confirmations, confirmations, now, orderID)
		return err
	}
}

// UpdateOrderConfirmations updates the confirmation count for an order
func UpdateOrderConfirmations(orderID string, confirmations int) error {
	var query string
	now := time.Now()

	if dbType == "postgres" {
		query = `UPDATE orders SET 
				confirmations = $1, 
				status = CASE 
					WHEN $1 >= 6 THEN 'confirmed'
					WHEN $1 > 0 THEN 'paid'
					ELSE 'pending'
				END,
				updated_at = $2
				WHERE id = $3`
	} else {
		query = `UPDATE orders SET 
				confirmations = ?, 
				status = CASE 
					WHEN ? >= 6 THEN 'confirmed'
					WHEN ? > 0 THEN 'paid'
					ELSE 'pending'
				END,
				updated_at = ?
				WHERE id = ?`
	}

	if dbType == "postgres" {
		_, err := dbConn.Exec(query, confirmations, now, orderID)
		return err
	} else {
		_, err := dbConn.Exec(query, confirmations, confirmations, confirmations, now, orderID)
		return err
	}
}

// CreatePayment creates a new payment record
func CreatePayment(payment *models.Payment) error {
	if dbType == "postgres" {
		err := dbConn.QueryRow(`
			INSERT INTO payments (order_id, transaction_hash, amount_btc, confirmations, status, detected_at, confirmed_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			RETURNING id, created_at`,
			payment.OrderID, payment.TransactionHash, payment.AmountBTC, payment.Confirmations,
			payment.Status, payment.DetectedAt, payment.ConfirmedAt,
		).Scan(&payment.ID, &payment.CreatedAt)
		return err
	} else {
		payment.ID = GenerateID()
		payment.CreatedAt = time.Now()

		_, err := dbConn.Exec(`
			INSERT INTO payments (id, order_id, transaction_hash, amount_btc, confirmations, status, detected_at, confirmed_at, created_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			payment.ID, payment.OrderID, payment.TransactionHash, payment.AmountBTC, payment.Confirmations,
			payment.Status, payment.DetectedAt, payment.ConfirmedAt, payment.CreatedAt,
		)
		return err
	}
}

// UpdateOrderBitcoinData updates an order with Bitcoin amount and QR code data
func UpdateOrderBitcoinData(orderID string, amountBTC float64, qrCodeData string) error {
	log.Printf("[DB] Updating order %s with Bitcoin data: amount=%.8f BTC, qr_code_length=%d", orderID, amountBTC, len(qrCodeData))

	var query string
	now := time.Now()

	if dbType == "postgres" {
		query = `UPDATE orders SET 
				amount_btc = $1, 
				qr_code_data = $2,
				updated_at = $3
				WHERE id = $4`
	} else {
		query = `UPDATE orders SET 
				amount_btc = ?, 
				qr_code_data = ?,
				updated_at = ?
				WHERE id = ?`
	}

	result, err := dbConn.Exec(query, amountBTC, qrCodeData, now, orderID)
	if err != nil {
		log.Printf("[DB] Error updating order %s with Bitcoin data: %v", orderID, err)
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		log.Printf("[DB] Error getting rows affected for order %s: %v", orderID, err)
	} else {
		log.Printf("[DB] Successfully updated order %s with Bitcoin data, rows affected: %d", orderID, rowsAffected)
	}

	return nil
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
