package portal

import (
	"log"
	"net/http"
	"time"

	"github.com/MediSynth-io/medisynth/internal/auth"
	"github.com/MediSynth-io/medisynth/internal/database"
)

func (p *Portal) handleHome(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, "base.html", nil)
}

func (p *Portal) handleLogin(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, "login.html", nil)
}

func (p *Portal) handleRegister(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, "register.html", nil)
}

func (p *Portal) handleDocumentation(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, "documentation.html", nil)
}

func (p *Portal) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	user, err := auth.ValidateUser(email, password)
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Create session
	token, err := auth.CreateSession(user.ID)
	if err != nil {
		log.Printf("Error creating session: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	http.Redirect(w, r, "/portal/dashboard", http.StatusSeeOther)
}

func (p *Portal) handleRegisterPost(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	user, err := auth.RegisterUser(email, password)
	if err != nil {
		http.Error(w, "Registration failed", http.StatusBadRequest)
		return
	}

	// Create session
	token, err := auth.CreateSession(user.ID)
	if err != nil {
		log.Printf("Error creating session: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	http.Redirect(w, r, "/portal/dashboard", http.StatusSeeOther)
}

func (p *Portal) handleDashboard(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int64)
	user, err := database.GetUserByID(userID)
	if err != nil {
		log.Printf("Error getting user: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	p.renderTemplate(w, "dashboard.html", user)
}

func (p *Portal) handleTokens(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int64)
	tokens, err := database.GetUserTokens(userID)
	if err != nil {
		log.Printf("Error getting tokens: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Check for a token value in the query params (for display after creation)
	tokenValue := r.URL.Query().Get("new_token")
	data := struct {
		Tokens   []*database.Token
		NewToken string
	}{
		Tokens:   tokens,
		NewToken: tokenValue,
	}

	p.renderTemplate(w, "tokens.html", data)
}

func (p *Portal) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int64)
	name := r.FormValue("name")

	token, err := auth.CreateToken(userID, name)
	if err != nil {
		log.Printf("Error creating token: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Redirect to tokens page with the new token value in the query string
	http.Redirect(w, r, "/portal/tokens?new_token="+token.Token, http.StatusSeeOther)
}

func (p *Portal) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int64)
	tokenID := r.URL.Query().Get("id")

	err := auth.DeleteToken(userID, tokenID)
	if err != nil {
		log.Printf("Error deleting token: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/portal/tokens", http.StatusSeeOther)
}

func (p *Portal) handleLogout(w http.ResponseWriter, r *http.Request) {
	cookie, err := r.Cookie("session")
	if err == nil && cookie.Value != "" {
		_ = auth.DeleteSession(cookie.Value)
	}
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		Expires:  time.Unix(0, 0),
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/portal/login", http.StatusSeeOther)
}
