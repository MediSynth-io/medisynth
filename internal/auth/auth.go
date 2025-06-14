package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"time"

	"github.com/MediSynth-io/medisynth/internal/database"
	"golang.org/x/crypto/bcrypt"
)

func RegisterUser(email, password string) (*database.User, error) {
	// Hash password
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	// Create user in database
	user, err := database.CreateUser(email, string(hashedPassword))
	if err != nil {
		return nil, err
	}

	return user, nil
}

func ValidateUser(email, password string) (*database.User, error) {
	user, err := database.GetUserByEmail(email)
	if err != nil {
		return nil, err
	}

	// Compare passwords
	err = bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password))
	if err != nil {
		return nil, errors.New("invalid password")
	}

	return user, nil
}

func CreateToken(userID int64, name string) (*database.Token, error) {
	// Generate random token
	tokenBytes := make([]byte, 32)
	_, err := rand.Read(tokenBytes)
	if err != nil {
		return nil, err
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	// Set expiration to 1 year from now
	expiresAt := time.Now().AddDate(1, 0, 0)

	// Create token in database
	tokenObj, err := database.CreateToken(userID, name, token, &expiresAt)
	if err != nil {
		return nil, err
	}

	return tokenObj, nil
}

func ValidateToken(token string) (*database.Token, error) {
	tokenObj, err := database.GetTokenByValue(token)
	if err != nil {
		return nil, err
	}

	// Check if token is expired
	if tokenObj.ExpiresAt != nil && tokenObj.ExpiresAt.Before(time.Now()) {
		return nil, errors.New("token expired")
	}

	return tokenObj, nil
}

func DeleteToken(userID int64, tokenID string) error {
	return database.DeleteToken(userID, tokenID)
}

func ListTokens(userID int64) ([]*database.Token, error) {
	return database.GetUserTokens(userID)
}
