package database

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// Migration represents a database migration
type Migration struct {
	Version     int
	Description string
	SQL         string
}

// GetMigrations returns all database migrations
func GetMigrations(dbType string) []Migration {
	if dbType == "postgres" {
		return getPostgresMigrations()
	}
	return getSQLiteMigrations()
}

// getPostgresMigrations returns PostgreSQL migrations
func getPostgresMigrations() []Migration {
	return []Migration{
		{
			Version:     1,
			Description: "Create users table",
			SQL: `CREATE TABLE IF NOT EXISTS users (
				id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
				email VARCHAR(255) UNIQUE NOT NULL,
				password VARCHAR(255) NOT NULL,
				is_admin BOOLEAN NOT NULL DEFAULT FALSE,
				created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
				updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
			)`,
		},
		{
			Version:     2,
			Description: "Create tokens table",
			SQL: `CREATE TABLE IF NOT EXISTS tokens (
				id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				token VARCHAR(255) UNIQUE NOT NULL,
				name VARCHAR(255) NOT NULL,
				created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
				expires_at TIMESTAMP WITH TIME ZONE
			)`,
		},
		{
			Version:     3,
			Description: "Create sessions table",
			SQL: `CREATE TABLE IF NOT EXISTS sessions (
				id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
				user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
				token VARCHAR(255) UNIQUE NOT NULL,
				created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
				expires_at TIMESTAMP WITH TIME ZONE NOT NULL
			)`,
		},
		{
			Version:     4,
			Description: "Create jobs table",
			SQL: `CREATE TABLE IF NOT EXISTS jobs (
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
		},
		{
			Version:     5,
			Description: "Create orders table",
			SQL: `CREATE TABLE IF NOT EXISTS orders (
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
		},
		{
			Version:     6,
			Description: "Create payments table",
			SQL: `CREATE TABLE IF NOT EXISTS payments (
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
		},
		{
			Version:     7,
			Description: "Create indexes",
			SQL: `CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
				CREATE INDEX IF NOT EXISTS idx_tokens_user_id ON tokens(user_id);
				CREATE INDEX IF NOT EXISTS idx_tokens_token ON tokens(token);
				CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
				CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
				CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);
				CREATE INDEX IF NOT EXISTS idx_jobs_user_id ON jobs(user_id);
				CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
				CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id);
				CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
				CREATE INDEX IF NOT EXISTS idx_orders_order_number ON orders(order_number);
				CREATE INDEX IF NOT EXISTS idx_payments_order_id ON payments(order_id);
				CREATE INDEX IF NOT EXISTS idx_payments_transaction_hash ON payments(transaction_hash);`,
		},
		{
			Version:     8,
			Description: "Add force password reset to users",
			SQL:         "ALTER TABLE users ADD COLUMN force_password_reset BOOLEAN NOT NULL DEFAULT FALSE;",
		},
	}
}

// getSQLiteMigrations returns SQLite migrations
func getSQLiteMigrations() []Migration {
	return []Migration{
		{
			Version:     1,
			Description: "Create users table",
			SQL: `CREATE TABLE IF NOT EXISTS users (
				id TEXT PRIMARY KEY,
				email TEXT UNIQUE NOT NULL,
				password TEXT NOT NULL,
				is_admin BOOLEAN NOT NULL DEFAULT 0,
				created_at DATETIME NOT NULL,
				updated_at DATETIME NOT NULL
			)`,
		},
		{
			Version:     2,
			Description: "Create tokens table",
			SQL: `CREATE TABLE IF NOT EXISTS tokens (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				token TEXT UNIQUE NOT NULL,
				name TEXT NOT NULL,
				created_at DATETIME NOT NULL,
				expires_at DATETIME,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			)`,
		},
		{
			Version:     3,
			Description: "Create sessions table",
			SQL: `CREATE TABLE IF NOT EXISTS sessions (
				id TEXT PRIMARY KEY,
				user_id TEXT NOT NULL,
				token TEXT UNIQUE NOT NULL,
				created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
				expires_at DATETIME NOT NULL,
				FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
			)`,
		},
		{
			Version:     4,
			Description: "Create jobs table",
			SQL: `CREATE TABLE IF NOT EXISTS jobs (
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
		},
		{
			Version:     5,
			Description: "Create orders table",
			SQL: `CREATE TABLE IF NOT EXISTS orders (
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
		},
		{
			Version:     6,
			Description: "Create payments table",
			SQL: `CREATE TABLE IF NOT EXISTS payments (
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
		},
		{
			Version:     7,
			Description: "Create indexes",
			SQL: `CREATE INDEX IF NOT EXISTS idx_users_email ON users(email);
				CREATE INDEX IF NOT EXISTS idx_tokens_user_id ON tokens(user_id);
				CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);
				CREATE INDEX IF NOT EXISTS idx_sessions_token ON sessions(token);
				CREATE INDEX IF NOT EXISTS idx_jobs_user_id ON jobs(user_id);
				CREATE INDEX IF NOT EXISTS idx_jobs_status ON jobs(status);
				CREATE INDEX IF NOT EXISTS idx_orders_user_id ON orders(user_id);
				CREATE INDEX IF NOT EXISTS idx_orders_status ON orders(status);
				CREATE INDEX IF NOT EXISTS idx_orders_order_number ON orders(order_number);
				CREATE INDEX IF NOT EXISTS idx_payments_order_id ON payments(order_id);
				CREATE INDEX IF NOT EXISTS idx_payments_transaction_hash ON payments(transaction_hash);`,
		},
		{
			Version:     8,
			Description: "Add force password reset to users",
			SQL:         "ALTER TABLE users ADD COLUMN force_password_reset BOOLEAN NOT NULL DEFAULT 0;",
		},
	}
}

// createMigrationsTable creates the migrations tracking table
func createMigrationsTable(db *sql.DB, dbType string) error {
	var query string
	if dbType == "postgres" {
		query = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW()
		)`
	} else {
		query = `CREATE TABLE IF NOT EXISTS schema_migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`
	}

	_, err := db.Exec(query)
	return err
}

// getAppliedMigrations returns the list of applied migration versions
func getAppliedMigrations(db *sql.DB) (map[int]bool, error) {
	applied := make(map[int]bool)

	rows, err := db.Query("SELECT version FROM schema_migrations")
	if err != nil {
		return applied, err
	}
	defer rows.Close()

	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return applied, err
		}
		applied[version] = true
	}

	return applied, nil
}

// recordMigration records that a migration has been applied
func recordMigration(db *sql.DB, dbType string, version int) error {
	var query string
	if dbType == "postgres" {
		query = "INSERT INTO schema_migrations (version) VALUES ($1)"
	} else {
		query = "INSERT INTO schema_migrations (version) VALUES (?)"
	}
	_, err := db.Exec(query, version)
	return err
}

// RunMigrations runs all pending migrations
func RunMigrations(db *sql.DB, dbType string) error {
	log.Printf("=== RUNNING DATABASE MIGRATIONS ===")

	// Create migrations table
	if err := createMigrationsTable(db, dbType); err != nil {
		return fmt.Errorf("failed to create migrations table: %v", err)
	}

	// Get applied migrations
	applied, err := getAppliedMigrations(db)
	if err != nil {
		return fmt.Errorf("failed to get applied migrations: %v", err)
	}

	// Get all migrations
	migrations := GetMigrations(dbType)

	// Apply pending migrations
	for _, migration := range migrations {
		if applied[migration.Version] {
			log.Printf("Migration %d already applied: %s", migration.Version, migration.Description)
			continue
		}

		log.Printf("Applying migration %d: %s", migration.Version, migration.Description)

		// Split SQL by semicolon and execute each statement
		statements := strings.Split(migration.SQL, ";")
		for _, stmt := range statements {
			stmt = strings.TrimSpace(stmt)
			if stmt == "" {
				continue
			}

			if _, err := db.Exec(stmt); err != nil {
				return fmt.Errorf("failed to apply migration %d: %v", migration.Version, err)
			}
		}

		// Record migration as applied
		if err := recordMigration(db, dbType, migration.Version); err != nil {
			return fmt.Errorf("failed to record migration %d: %v", migration.Version, err)
		}

		log.Printf("Successfully applied migration %d", migration.Version)
	}

	log.Printf("=== MIGRATIONS COMPLETE ===")
	return nil
}
