package auth

import (
	"encoding/json"
	"net/http"
	"time"
)

type registerRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type tokenRequest struct {
	Name      string `json:"name"`
	ExpiresIn int64  `json:"expiresIn"` // Duration in seconds
}

type errorResponse struct {
	Error string `json:"error"`
}

// RegisterHandler handles user registration
func RegisterHandler(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := Register(req.Email, req.Password)
	if err != nil {
		if err == ErrEmailAlreadyTaken {
			http.Error(w, "Email already taken", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to register user", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":    user.ID,
		"email": user.Email,
	})
}

// LoginHandler handles user login and returns an API token
func LoginHandler(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := Authenticate(req.Email, req.Password)
	if err != nil {
		if err == ErrUserNotFound || err == ErrInvalidPassword {
			http.Error(w, "Invalid email or password", http.StatusUnauthorized)
			return
		}
		http.Error(w, "Authentication failed", http.StatusInternalServerError)
		return
	}

	// Generate a token that expires in 30 days
	token, err := GenerateToken(user.ID, "Login token", 30*24*time.Hour)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token": token.Token,
		"user": map[string]interface{}{
			"id":    user.ID,
			"email": user.Email,
		},
	})
}

// CreateTokenHandler creates a new API token for the authenticated user
func CreateTokenHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	var req tokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var expiresIn time.Duration
	if req.ExpiresIn > 0 {
		expiresIn = time.Duration(req.ExpiresIn) * time.Second
	}

	token, err := GenerateToken(userID, req.Name, expiresIn)
	if err != nil {
		http.Error(w, "Failed to generate token", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"token":     token.Token,
		"name":      token.Name,
		"expiresAt": token.ExpiresAt,
	})
}

// ListTokensHandler returns all API tokens for the authenticated user
func ListTokensHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	tokens, err := ListUserTokens(userID)
	if err != nil {
		http.Error(w, "Failed to list tokens", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"tokens": tokens,
	})
}

// DeleteTokenHandler removes an API token
func DeleteTokenHandler(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Authentication required", http.StatusUnauthorized)
		return
	}

	token := r.URL.Query().Get("token")
	if token == "" {
		http.Error(w, "Token parameter required", http.StatusBadRequest)
		return
	}

	// Verify the token belongs to the user
	userIDFromToken, err := ValidateToken(token)
	if err != nil {
		http.Error(w, "Invalid token", http.StatusBadRequest)
		return
	}
	if userIDFromToken != userID {
		http.Error(w, "Unauthorized", http.StatusForbidden)
		return
	}

	if err := DeleteToken(token); err != nil {
		http.Error(w, "Failed to delete token", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
