package portal

import (
	"html/template"
	"net/http"
	"path/filepath"
	"time"

	"github.com/MediSynth-io/medisynth/internal/auth"
	"github.com/go-chi/chi/v5"
)

type Portal struct {
	templates *template.Template
}

func New() (*Portal, error) {
	// Load templates
	templates, err := template.ParseGlob(filepath.Join("templates", "portal", "*.html"))
	if err != nil {
		return nil, err
	}

	return &Portal{
		templates: templates,
	}, nil
}

func (p *Portal) Routes() chi.Router {
	r := chi.NewRouter()

	// Public routes
	r.Get("/", p.handleHome)
	r.Get("/login", p.handleLoginPage)
	r.Get("/register", p.handleRegisterPage)
	r.Post("/login", p.handleLogin)
	r.Post("/register", p.handleRegister)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(auth.RequireAuth)
		r.Get("/dashboard", p.handleDashboard)
		r.Get("/tokens", p.handleTokensPage)
		r.Post("/tokens", p.handleCreateToken)
		r.Delete("/tokens", p.handleDeleteToken)
	})

	return r
}

func (p *Portal) handleHome(w http.ResponseWriter, r *http.Request) {
	p.templates.ExecuteTemplate(w, "home.html", nil)
}

func (p *Portal) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	p.templates.ExecuteTemplate(w, "login.html", nil)
}

func (p *Portal) handleRegisterPage(w http.ResponseWriter, r *http.Request) {
	p.templates.ExecuteTemplate(w, "register.html", nil)
}

func (p *Portal) handleLogin(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	user, err := auth.Authenticate(email, password)
	if err != nil {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Create session
	session, err := auth.CreateSession(user.ID)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (p *Portal) handleRegister(w http.ResponseWriter, r *http.Request) {
	email := r.FormValue("email")
	password := r.FormValue("password")

	user, err := auth.Register(email, password)
	if err != nil {
		if err == auth.ErrEmailAlreadyTaken {
			http.Error(w, "Email already taken", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to register", http.StatusInternalServerError)
		return
	}

	// Create session
	session, err := auth.CreateSession(user.ID)
	if err != nil {
		http.Error(w, "Failed to create session", http.StatusInternalServerError)
		return
	}

	// Set session cookie
	http.SetCookie(w, &http.Cookie{
		Name:     "session",
		Value:    session.Token,
		Path:     "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteStrictMode,
	})

	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

func (p *Portal) handleDashboard(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int64)
	user, err := auth.GetUserByID(userID)
	if err != nil {
		http.Error(w, "Failed to get user", http.StatusInternalServerError)
		return
	}

	p.templates.ExecuteTemplate(w, "dashboard.html", user)
}

func (p *Portal) handleTokensPage(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int64)
	tokens, err := auth.ListUserTokens(userID)
	if err != nil {
		http.Error(w, "Failed to get tokens", http.StatusInternalServerError)
		return
	}

	p.templates.ExecuteTemplate(w, "tokens.html", tokens)
}

func (p *Portal) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	userID := r.Context().Value("userID").(int64)
	name := r.FormValue("name")
	expiresIn := 30 * 24 * time.Hour // 30 days

	token, err := auth.GenerateToken(userID, name, expiresIn)
	if err != nil {
		http.Error(w, "Failed to create token", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
}

func (p *Portal) handleDeleteToken(w http.ResponseWriter, r *http.Request) {
	token := r.FormValue("token")
	if err := auth.DeleteToken(token); err != nil {
		http.Error(w, "Failed to delete token", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/tokens", http.StatusSeeOther)
}
