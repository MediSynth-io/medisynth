# Authentication System

This package implements a comprehensive authentication system for the MediSynth API, including user management, JWT-based authentication, and API key management.

## Features

- User registration and login
- JWT-based authentication
- API key management
- Password hashing with bcrypt
- SQLite storage backend
- Configurable settings via environment variables

## Configuration

The authentication system can be configured using environment variables:

- `AUTH_JWT_SECRET_KEY`: Secret key for JWT token signing (required in production)
- `AUTH_JWT_TOKEN_DURATION`: Duration for JWT tokens (default: 24h)
- `AUTH_API_KEY_PREFIX`: Prefix for API keys (default: "ms_")
- `AUTH_MIN_PASSWORD_LEN`: Minimum password length (default: 8)

## API Endpoints

### Public Endpoints

- `POST /register`: Register a new user
  ```json
  {
    "email": "user@example.com",
    "password": "securepassword"
  }
  ```

- `POST /login`: Login and get JWT token
  ```json
  {
    "email": "user@example.com",
    "password": "securepassword"
  }
  ```

### Protected Endpoints

All protected endpoints require a valid JWT token in the `Authorization` header:
```
Authorization: Bearer <token>
```

- `POST /api-keys`: Create a new API key
  ```json
  {
    "name": "My Dev Key",
    "created_at": "2023-01-01T12:00:00Z",
    "expires_at": "2025-12-31T23:59:59Z"
  }
  ```

- `GET /api-keys`: List all API keys for the authenticated user

- `DELETE /api-keys/{id}`: Delete an API key

## API Key Authentication

For API endpoints that support API key authentication, include the API key in the `X-API-Key` header:
```
X-API-Key: ms_<api-key>
```

## Security Considerations

1. Always use HTTPS in production
2. Set a strong JWT secret key in production
3. Regularly rotate API keys
4. Implement rate limiting
5. Monitor for suspicious activity
6. Keep dependencies up to date

## Database Schema

The authentication system uses two main tables:

### Users Table
```sql
CREATE TABLE users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT NOT NULL UNIQUE,
    password TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    updated_at TIMESTAMP NOT NULL
);
```

### API Keys Table
```sql
CREATE TABLE api_keys (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    key TEXT NOT NULL UNIQUE,
    name TEXT NOT NULL,
    created_at TIMESTAMP NOT NULL,
    expires_at TIMESTAMP NOT NULL,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);
```

## Usage Example

```go
package main

import (
    "database/sql"
    "log"

    "github.com/MediSynth-io/medisynth/internal/auth"
    _ "github.com/mattn/go-sqlite3"
)

func main() {
    // Initialize database
    db, err := sql.Open("sqlite3", "auth.db")
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    // Load configuration
    config, err := auth.LoadConfig()
    if err != nil {
        log.Fatal(err)
    }

    // Initialize stores
    userStore := auth.NewSQLiteUserStore(db)
    apiKeyStore := auth.NewSQLiteAPIKeyStore(db)
    tokenManager := auth.NewTokenManager(config.JWTSecretKey)

    // Set up routes
    r := chi.NewRouter()
    auth.RegisterRoutes(r, userStore, tokenManager, apiKeyStore)

    // Start server
    log.Fatal(http.ListenAndServe(":8080", r))
} 