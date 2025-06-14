package auth

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

var (
	userStore    *SQLiteUserStore
	tokenManager *TokenManager
	apiKeyStore  *SQLiteAPIKeyStore
)

// GetUserStore returns the global user store instance
func GetUserStore() *SQLiteUserStore {
	if userStore == nil {
		userStore = NewUserStore()
	}
	return userStore
}

// GetTokenManager returns the global token manager instance
func GetTokenManager() *TokenManager {
	if tokenManager == nil {
		tokenManager = NewTokenManager("your-secret-key") // TODO: Get from config
	}
	return tokenManager
}

// GetAPIKeyStore returns the global API key store instance
func GetAPIKeyStore() *SQLiteAPIKeyStore {
	if apiKeyStore == nil {
		apiKeyStore = NewAPIKeyStore()
	}
	return apiKeyStore
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type TokenRequest struct {
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expires_at"`
}

type TokenResponse struct {
	Token string `json:"token"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}

// RegisterHandler handles user registration
func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	if !validateEmail(req.Email) {
		http.Error(w, "Invalid email", http.StatusBadRequest)
		return
	}

	if !validatePassword(req.Password) {
		http.Error(w, "Invalid password", http.StatusBadRequest)
		return
	}

	user, err := NewUser(req.Email, req.Password)
	if err != nil {
		log.Printf("Failed to create user: %v", err)
		http.Error(w, "Registration failed", http.StatusInternalServerError)
		return
	}

	userStore := GetUserStore()
	if err := userStore.Create(user); err != nil {
		log.Printf("Failed to store user: %v", err)
		http.Error(w, "Registration failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":    user.ID,
		"email": user.Email,
	})
}

// LoginHandler handles user login
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	userStore := GetUserStore()
	user, err := userStore.GetByEmail(req.Email)
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	if !user.ValidatePassword(req.Password) {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	tokenManager := GetTokenManager()
	token, err := tokenManager.GenerateToken(user, 24*time.Hour)
	if err != nil {
		log.Printf("Failed to generate token: %v", err)
		http.Error(w, "Login failed", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{
		"token": token,
	})
}

// CreateTokenHandler creates a new API token for the authenticated user
func CreateTokenHandler(w http.ResponseWriter, r *http.Request) {
	claims, ok := GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req TokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}

	key, err := generateAPIKey()
	if err != nil {
		log.Printf("Failed to generate API key: %v", err)
		http.Error(w, "Failed to create API key", http.StatusInternalServerError)
		return
	}

	apiKey := &APIKey{
		UserID:    claims.UserID,
		Key:       key,
		Name:      req.Name,
		CreatedAt: time.Now(),
		ExpiresAt: req.ExpiresAt,
	}

	apiKeyStore := GetAPIKeyStore()
	if err := apiKeyStore.Create(apiKey); err != nil {
		log.Printf("Failed to store API key: %v", err)
		http.Error(w, "Failed to create API key", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(apiKey)
}

// ListTokensHandler returns all API tokens for the authenticated user
func ListTokensHandler(w http.ResponseWriter, r *http.Request) {
	claims, ok := GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	apiKeyStore := GetAPIKeyStore()
	keys, err := apiKeyStore.GetByUserID(claims.UserID)
	if err != nil {
		log.Printf("Failed to list API keys: %v", err)
		http.Error(w, "Failed to list API keys", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(keys)
}

// DeleteTokenHandler removes an API token
func DeleteTokenHandler(w http.ResponseWriter, r *http.Request) {
	claims, ok := GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	keyID := chi.URLParam(r, "id")
	if keyID == "" {
		http.Error(w, "API key ID required", http.StatusBadRequest)
		return
	}

	apiKeyStore := GetAPIKeyStore()
	if err := apiKeyStore.Delete(claims.UserID); err != nil {
		log.Printf("Failed to delete API key: %v", err)
		http.Error(w, "Failed to delete API key", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
