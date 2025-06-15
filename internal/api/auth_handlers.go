package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/MediSynth-io/medisynth/internal/auth"
	"github.com/go-chi/chi/v5"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

func (api *Api) RegisterHandler(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if !auth.ValidateEmail(creds.Email) {
		http.Error(w, "Invalid email format", http.StatusBadRequest)
		return
	}

	if !auth.ValidatePassword(creds.Password) {
		http.Error(w, "Password does not meet requirements", http.StatusBadRequest)
		return
	}

	user, err := auth.RegisterUser(creds.Email, creds.Password)
	if err != nil {
		http.Error(w, "Registration failed: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// Do not return password hash
	user.Password = ""
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

func (api *Api) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var creds credentials
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	user, err := auth.ValidateUser(creds.Email, creds.Password)
	if err != nil {
		http.Error(w, "Invalid email or password", http.StatusUnauthorized)
		return
	}

	sessionToken, err := auth.CreateSession(user.ID)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     "session_token",
		Value:    sessionToken,
		Expires:  time.Now().Add(24 * time.Hour),
		HttpOnly: true,
		Secure:   true, // Set to true in production
		SameSite: http.SameSiteLaxMode,
	})

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}

func (api *Api) CreateTokenHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)

	var req struct {
		Name string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	token, err := auth.CreateToken(userID, req.Name)
	if err != nil {
		http.Error(w, "Failed to create token", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(token)
}

func (api *Api) ListTokensHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)

	tokens, err := auth.ListTokens(userID)
	if err != nil {
		http.Error(w, "Failed to list tokens", http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(tokens)
}

func (api *Api) DeleteTokenHandler(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)
	tokenID := chi.URLParam(r, "tokenID")

	if err := auth.DeleteToken(userID, tokenID); err != nil {
		http.Error(w, "Failed to delete token", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "success"})
}
