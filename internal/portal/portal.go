package portal

import (
	"context"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/MediSynth-io/medisynth/internal/auth"
	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/go-chi/chi/v5"
)

type Portal struct {
	templates map[string]*template.Template
	config    *config.Config
}

func New(cfg *config.Config) (*Portal, error) {
	templates := make(map[string]*template.Template)

	// Path to the templates directory
	templateDir := "templates/portal"

	// Find all the page templates
	pages, err := fs.Glob(os.DirFS(templateDir), "*.html")
	if err != nil {
		log.Printf("Error finding templates: %v", err)
		return nil, err
	}

	// For each page, parse it with the base template
	for _, page := range pages {
		if page == "base.html" {
			continue
		}

		ts, err := template.ParseFiles(
			filepath.Join(templateDir, "base.html"),
			filepath.Join(templateDir, page),
		)
		if err != nil {
			log.Printf("Error parsing template %s: %v", page, err)
			return nil, err
		}
		templates[page] = ts
	}

	log.Printf("Successfully loaded templates")

	return &Portal{
		templates: templates,
		config:    cfg,
	}, nil
}

func (p *Portal) Routes() http.Handler {
	r := chi.NewRouter()

	// Static files
	log.Printf("Setting up static file server for directory: static")
	fileServer := http.FileServer(http.Dir("static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))

	// Public routes
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		log.Printf("Handling home page request")
		p.HandleHome(w, r)
	})
	r.Get("/login", p.handleLogin)
	r.Get("/register", p.handleRegister)
	r.Get("/documentation", p.handleDocumentation)
	r.Post("/login", p.handleLoginPost)
	r.Post("/register", p.handleRegisterPost)

	// Favicon
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/favicon.ico")
	})

	// Logout route
	r.Get("/logout", p.handleLogout)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(p.requireAuth)

		r.Get("/dashboard", p.handleDashboard)

		// Token management routes
		r.Route("/tokens", func(r chi.Router) {
			r.Get("/", p.handleTokens)
			r.Post("/create", p.handleCreateToken)
			r.Post("/{id}/delete", p.handleDeleteToken)
		})
	})

	// NotFound handler
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		p.renderTemplate(w, r, "404.html", "Not Found", map[string]interface{}{
			"Path": r.URL.Path,
		})
	})

	return r
}

func (p *Portal) requireAuth(next http.Handler) http.Handler {
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
					Domain:   p.config.Domains.Portal,
					HttpOnly: true,
					Secure:   p.config.Domains.Secure,
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
