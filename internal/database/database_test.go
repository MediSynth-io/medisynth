package database

import (
	"os"
	"testing"
	"time"

	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

// DatabaseTestSuite defines the test suite
type DatabaseTestSuite struct {
	suite.Suite
	dbType string
}

// SetupTest initializes the database for each test
func (s *DatabaseTestSuite) SetupTest() {
	var cfg *config.Config
	var err error

	// Use environment variables to switch between test databases
	s.dbType = os.Getenv("DB_TYPE")
	if s.dbType == "postgres" {
		cfg = &config.Config{
			DatabaseType:     "postgres",
			DatabaseHost:     "localhost",
			DatabasePort:     "5433", // Use a different port for testing
			DatabaseName:     "medisynth_test",
			DatabaseUser:     "medisynth_test",
			DatabasePassword: "testpassword",
			DatabaseSSLMode:  "disable",
		}
	} else {
		s.dbType = "sqlite" // Default to SQLite
		cfg = &config.Config{
			DatabaseType: "sqlite",
			DatabasePath: "test_medisynth.db",
		}
		// Clean up previous test database
		os.Remove("test_medisynth.db")
	}

	err = Init(cfg)
	assert.NoError(s.T(), err, "Database initialization should succeed")
}

// TearDownTest cleans up the database after each test
func (s *DatabaseTestSuite) TearDownTest() {
	if s.dbType == "sqlite" {
		os.Remove("test_medisynth.db")
	} else {
		// Clean up tables in PostgreSQL
		dbConn.Exec("DROP TABLE IF EXISTS sessions, tokens, users CASCADE")
	}
	dbConn = nil // Reset connection
}

// TestDatabaseTestSuite runs the test suite
func TestDatabaseTestSuite(t *testing.T) {
	suite.Run(t, new(DatabaseTestSuite))
}

// TestCreateAndGetUser tests user creation and retrieval
func (s *DatabaseTestSuite) TestCreateAndGetUser() {
	// Create user
	email := "test@example.com"
	password := "password123"
	user, err := CreateUser(email, password)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), user)
	assert.NotEmpty(s.T(), user.ID)
	assert.Equal(s.T(), email, user.Email)

	// Get user by email
	retrievedUser, err := GetUserByEmail(email)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), retrievedUser)
	assert.Equal(s.T(), user.ID, retrievedUser.ID)

	// Get user by ID
	retrievedUserByID, err := GetUserByID(user.ID)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), retrievedUserByID)
	assert.Equal(s.T(), user.ID, retrievedUserByID.ID)
}

// TestCreateAndGetToken tests token creation and retrieval
func (s *DatabaseTestSuite) TestCreateAndGetToken() {
	// Create user first
	user, _ := CreateUser("tokenuser@example.com", "password")

	// Create token
	tokenName := "test-token"
	tokenValue := "test-token-value"
	expiresAt := time.Now().Add(1 * time.Hour)
	token, err := CreateToken(user.ID, tokenName, tokenValue, &expiresAt)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), token)
	assert.NotEmpty(s.T(), token.ID)

	// Get token by value
	retrievedToken, err := GetTokenByValue(tokenValue)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), retrievedToken)
	assert.Equal(s.T(), token.ID, retrievedToken.ID)
	assert.Equal(s.T(), user.ID, retrievedToken.UserID)

	// Get user tokens
	userTokens, err := GetUserTokens(user.ID)
	assert.NoError(s.T(), err)
	assert.Len(s.T(), userTokens, 1)
	assert.Equal(s.T(), token.ID, userTokens[0].ID)
}

// TestCreateAndGetSession tests session creation and retrieval
func (s *DatabaseTestSuite) TestCreateAndGetSession() {
	// Create user
	user, _ := CreateUser("sessionuser@example.com", "password")

	// Create session
	sessionToken := "test-session-token"
	expiresAt := time.Now().Add(24 * time.Hour)
	session, err := CreateSession(user.ID, sessionToken, expiresAt)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), session)
	assert.NotEmpty(s.T(), session.ID)

	// Get session by token
	retrievedSession, err := GetSessionByToken(sessionToken)
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), retrievedSession)
	assert.Equal(s.T(), session.ID, retrievedSession.ID)
	assert.Equal(s.T(), user.ID, retrievedSession.UserID)
}

// TestDeleteToken tests token deletion
func (s *DatabaseTestSuite) TestDeleteToken() {
	// Setup: Create user and token
	user, _ := CreateUser("deleteuser@example.com", "password")
	token, _ := CreateToken(user.ID, "token-to-delete", "token-value-to-delete", nil)

	// Delete token
	err := DeleteToken(user.ID, token.ID)
	assert.NoError(s.T(), err)

	// Verify deletion
	deletedToken, err := GetTokenByValue("token-value-to-delete")
	assert.Error(s.T(), err) // Should be an error (e.g., sql.ErrNoRows)
	assert.Nil(s.T(), deletedToken)

	// Test deleting non-existent token
	err = DeleteToken(user.ID, "non-existent-id")
	assert.Error(s.T(), err)
}

// TestDeleteSession tests session deletion
func (s *DatabaseTestSuite) TestDeleteSession() {
	// Setup: Create user and session
	user, _ := CreateUser("deletesession@example.com", "password")
	session, _ := CreateSession(user.ID, "session-to-delete", time.Now().Add(1*time.Hour))

	// Delete session
	err := DeleteSession(session.Token)
	assert.NoError(s.T(), err)

	// Verify deletion
	deletedSession, err := GetSessionByToken("session-to-delete")
	assert.Error(s.T(), err)
	assert.Nil(s.T(), deletedSession)
}

// TestCleanupExpiredSessions tests cleanup of expired sessions
func (s *DatabaseTestSuite) TestCleanupExpiredSessions() {
	// Setup: Create user and sessions (one expired, one valid)
	user, _ := CreateUser("expuser@example.com", "password")
	expiredExpiresAt := time.Now().Add(-1 * time.Hour)
	validExpiresAt := time.Now().Add(1 * time.Hour)
	CreateSession(user.ID, "expired-session", expiredExpiresAt)
	CreateSession(user.ID, "valid-session", validExpiresAt)

	// Cleanup expired sessions
	err := CleanupExpiredSessions()
	assert.NoError(s.T(), err)

	// Verify results
	expiredSession, err := GetSessionByToken("expired-session")
	assert.Error(s.T(), err)
	assert.Nil(s.T(), expiredSession)

	validSession, err := GetSessionByToken("valid-session")
	assert.NoError(s.T(), err)
	assert.NotNil(s.T(), validSession)
}
