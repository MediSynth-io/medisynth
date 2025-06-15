package portal

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/MediSynth-io/medisynth/internal/auth"
	"github.com/MediSynth-io/medisynth/internal/database"
	"github.com/go-chi/chi/v5"
)

func (p *Portal) handleLanding(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, r, "landing.html", "MediSynth", map[string]interface{}{})
}

func (p *Portal) HandleHome(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, r, "home.html", "Home", map[string]interface{}{})
}

func (p *Portal) handleLogin(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, r, "login.html", "Login", map[string]interface{}{})
}

func (p *Portal) handleRegister(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, r, "register.html", "Register", map[string]interface{}{
		"PasswordRequirements": auth.GetPasswordRequirements(),
	})
}

func (p *Portal) handleDocumentation(w http.ResponseWriter, r *http.Request) {
	p.renderTemplate(w, r, "documentation.html", "Documentation", map[string]interface{}{})
}

func (p *Portal) handleLoginPost(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	user, err := auth.ValidateUser(email, password)
	if err != nil {
		p.renderTemplate(w, r, "login.html", "Login", map[string]interface{}{
			"Error": "Invalid email or password",
			"Email": email,
		})
		return
	}

	// Create session
	token, err := auth.CreateSession(user.ID)
	if err != nil {
		log.Printf("Error creating session: %v", err)
		p.renderTemplate(w, r, "login.html", "Login", map[string]interface{}{
			"Error": "Failed to create session. Please try again.",
			"Email": email,
		})
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		Domain:   p.config.DomainPortal,
		HttpOnly: true,
		Secure:   p.config.DomainSecure,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (p *Portal) handleRegisterPost(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	data := map[string]interface{}{
		"Email":                email,
		"PasswordRequirements": auth.GetPasswordRequirements(),
	}

	if !auth.ValidateEmail(email) {
		data["Error"] = "Please enter a valid email address"
		p.renderTemplate(w, r, "register.html", "Register", data)
		return
	}

	if password != confirmPassword {
		data["Error"] = "Passwords do not match"
		p.renderTemplate(w, r, "register.html", "Register", data)
		return
	}

	if !auth.ValidatePassword(password) {
		data["Error"] = "Password does not meet the requirements"
		p.renderTemplate(w, r, "register.html", "Register", data)
		return
	}

	user, err := auth.RegisterUser(email, password)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE constraint") {
			data["Error"] = "This email is already registered"
		} else {
			data["Error"] = "Registration failed. Please try again."
			log.Printf("Registration error: %v", err)
		}
		p.renderTemplate(w, r, "register.html", "Register", data)
		return
	}

	// Create session
	token, err := auth.CreateSession(user.ID)
	if err != nil {
		log.Printf("Error creating session: %v", err)
		data["Error"] = "Registration successful but failed to log in. Please try logging in."
		p.renderTemplate(w, r, "register.html", "Register", data)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    token,
		Path:     "/",
		Domain:   p.config.DomainPortal,
		HttpOnly: true,
		Secure:   p.config.DomainSecure,
		SameSite: http.SameSiteStrictMode,
		Expires:  time.Now().Add(24 * time.Hour),
	})

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (p *Portal) handleDashboard(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)

	// Get API tokens count as a simple metric
	tokens, err := database.GetUserTokens(userID)
	if err != nil {
		log.Printf("Error getting tokens: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	data := struct {
		APIRequests      int    `json:"apiRequests"`
		RecordsGenerated int    `json:"recordsGenerated"`
		AccountType      string `json:"accountType"`
	}{
		APIRequests:      len(tokens) * 10,  // Simple placeholder: 10 requests per token
		RecordsGenerated: len(tokens) * 100, // Simple placeholder: 100 records per token
		AccountType:      "Free",            // Default to free tier for now
	}

	p.renderTemplate(w, r, "dashboard.html", "Dashboard", data)
}

func (p *Portal) handleTokens(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)
	tokens, err := database.GetUserTokens(userID)
	if err != nil {
		log.Printf("Error getting tokens: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Check for a token value in the query params (for display after creation)
	tokenValue := r.URL.Query().Get("new_token")
	data := map[string]interface{}{
		"Data": map[string]interface{}{
			"Tokens": tokens,
		},
		"NewToken": tokenValue,
	}

	p.renderTemplate(w, r, "tokens.html", "API Tokens", data)
}

func (p *Portal) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)
	name := r.FormValue("name")

	if name == "" {
		http.Error(w, "Token name is required", http.StatusBadRequest)
		return
	}

	token, err := auth.CreateToken(userID, name)
	if err != nil {
		log.Printf("Error creating token: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Redirect to tokens page with the new token value in the query string
	http.Redirect(w, r, "/tokens?new_token="+token.Token, http.StatusSeeOther)
}

func (p *Portal) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(string)
	tokenID := chi.URLParam(r, "id")
	if tokenID == "" {
		http.Error(w, "Token ID required", http.StatusBadRequest)
		return
	}

	err := auth.DeleteToken(userID, tokenID)
	if err != nil {
		log.Printf("Error deleting token: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
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
		Domain:   p.config.DomainPortal,
		HttpOnly: true,
		Secure:   p.config.DomainSecure,
		Expires:  time.Unix(0, 0),
		SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (p *Portal) renderTemplate(w http.ResponseWriter, r *http.Request, tmplName string, pageTitle string, data interface{}) {
	log.Printf("Rendering template: %s", tmplName)

	ts, ok := p.templates[tmplName]
	if !ok {
		log.Printf("Error: template %s not found", tmplName)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Create a map to hold template data
	var templateData map[string]interface{}

	// If data is already a map, use it directly
	if existingMap, ok := data.(map[string]interface{}); ok {
		templateData = existingMap
	} else {
		// Otherwise wrap non-nil data in the Data field
		templateData = map[string]interface{}{}
		if data != nil {
			templateData["Data"] = data
		}
	}

	// Add user context and active page info
	templateData["ActivePage"] = pageTitle
	if userID, ok := r.Context().Value("userID").(string); ok {
		user, err := database.GetUserByID(userID)
		if err == nil {
			templateData["User"] = user
		}
	}

	log.Printf("Executing template %s with data: %+v", tmplName, templateData)
	err := ts.ExecuteTemplate(w, "base.html", templateData)
	if err != nil {
		log.Printf("Error rendering template %s: %v", tmplName, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}
	log.Printf("Successfully rendered template: %s", tmplName)
}

// responseWriter is a wrapper around http.ResponseWriter that includes the request context
type responseWriter struct {
	http.ResponseWriter
	req *http.Request
}

func (rw *responseWriter) Context() context.Context {
	return rw.req.Context()
}
