package portal

import (
	"context"
	"html/template"
	"net/http"
	"time"

	"github.com/MediSynth-io/medisynth/internal/auth"
	"github.com/go-chi/chi/v5"
)

type Portal struct {
	templates *template.Template
}

func New() (*Portal, error) {
	// Load templates
	templates, err := template.ParseGlob("templates/portal/*.html")
	if err != nil {
		return nil, err
	}

	return &Portal{
		templates: templates,
	}, nil
}

func (p *Portal) Routes() http.Handler {
	r := chi.NewRouter()

	// Public routes
	r.Get("/", p.HandleHome)
	r.Get("/login", p.handleLogin)
	r.Get("/register", p.handleRegister)
	r.Get("/documentation", p.handleDocumentation)
	r.Post("/login", p.handleLoginPost)
	r.Post("/register", p.handleRegisterPost)

	// Logout route
	r.Get("/logout", p.handleLogout)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(requireAuth)
		r.Get("/dashboard", p.handleDashboard)
		r.Get("/tokens", p.handleTokens)
		r.Post("/tokens/create", p.handleCreateToken)
		r.Delete("/tokens/{id}", p.handleDeleteToken)
	})

	return r
}

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session")
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		session, err := auth.ValidateSession(cookie.Value)
		if err != nil {
			if err == auth.ErrSessionExpired {
				// Clear expired session cookie
				http.SetCookie(w, &http.Cookie{
					Name:     "session",
					Value:    "",
					Path:     "/",
					Domain:   "portal.medisynth.io",
					HttpOnly: true,
					Secure:   true,
					Expires:  time.Unix(0, 0),
					SameSite: http.SameSiteStrictMode,
				})
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx := r.Context()
		ctx = context.WithValue(ctx, "userID", session.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
