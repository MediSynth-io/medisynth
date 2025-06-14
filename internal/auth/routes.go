package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
)

// RegisterRequest represents a user registration request
type RegisterRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// LoginRequest represents a user login request
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// APIKeyRequest represents an API key creation request
type APIKeyRequest struct {
	Name      string    `json:"name"`
	ExpiresAt time.Time `json:"expires_at"`
}

// RegisterRoutes sets up the authentication routes
func RegisterRoutes(r chi.Router, userStore UserStore, tokenManager *TokenManager, apiKeyStore APIKeyStore) {
	// Public routes
	r.Group(func(r chi.Router) {
		r.Post("/register", handleRegister(userStore))
		r.Post("/login", handleLogin(userStore, tokenManager))
	})

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(tokenManager))
		r.Use(RequireAuth)

		// API key management
		r.Post("/api-keys", handleCreateAPIKey(apiKeyStore))
		r.Get("/api-keys", handleListAPIKeys(apiKeyStore))
		r.Delete("/api-keys/{id}", handleDeleteAPIKey(apiKeyStore))
	})
}

func handleRegister(userStore UserStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		if !validateEmail(req.Email) {
			http.Error(w, "invalid email", http.StatusBadRequest)
			return
		}

		if !validatePassword(req.Password) {
			http.Error(w, "invalid password", http.StatusBadRequest)
			return
		}

		user, err := NewUser(req.Email, req.Password)
		if err != nil {
			http.Error(w, "failed to create user", http.StatusInternalServerError)
			return
		}

		if err := userStore.Create(user); err != nil {
			http.Error(w, "failed to create user", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":    user.ID,
			"email": user.Email,
		})
	}
}

func handleLogin(userStore UserStore, tokenManager *TokenManager) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Email    string `json:"email"`
			Password string `json:"password"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		user, err := userStore.GetByEmail(req.Email)
		if err != nil {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		if !user.ValidatePassword(req.Password) {
			http.Error(w, "invalid credentials", http.StatusUnauthorized)
			return
		}

		token, err := tokenManager.GenerateToken(user, 24*time.Hour)
		if err != nil {
			http.Error(w, "failed to generate token", http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(map[string]string{
			"token": token,
		})
	}
}

func handleCreateAPIKey(apiKeyStore APIKeyStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Name      string    `json:"name"`
			ExpiresAt time.Time `json:"expires_at"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request", http.StatusBadRequest)
			return
		}

		claims, ok := GetUserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Generate a secure random API key
		key, err := generateAPIKey()
		if err != nil {
			http.Error(w, "failed to generate API key", http.StatusInternalServerError)
			return
		}

		apiKey := &APIKey{
			UserID:    claims.UserID,
			Key:       key,
			Name:      req.Name,
			CreatedAt: time.Now(),
			ExpiresAt: req.ExpiresAt,
		}

		if err := apiKeyStore.Create(apiKey); err != nil {
			http.Error(w, "failed to create API key", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(apiKey)
	}
}

func handleListAPIKeys(apiKeyStore APIKeyStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetUserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		keys, err := apiKeyStore.GetByUserID(claims.UserID)
		if err != nil {
			http.Error(w, "failed to list API keys", http.StatusInternalServerError)
			return
		}

		json.NewEncoder(w).Encode(keys)
	}
}

func handleDeleteAPIKey(apiKeyStore APIKeyStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetUserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		keyID := chi.URLParam(r, "id")
		if keyID == "" {
			http.Error(w, "API key ID required", http.StatusBadRequest)
			return
		}

		// TODO: Add validation to ensure the API key belongs to the user

		if err := apiKeyStore.Delete(claims.UserID); err != nil {
			http.Error(w, "failed to delete API key", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)
	}
}

// generateAPIKey generates a random API key
func generateAPIKey() (string, error) {
	// TODO: Implement secure random API key generation
	return "test-key", nil // Placeholder
}
