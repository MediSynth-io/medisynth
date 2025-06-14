package portal

import (
	"html/template"
	"log"
	"net/http"

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
	r.Get("/", p.handleHome)
	r.Get("/login", p.handleLogin)
	r.Get("/register", p.handleRegister)
	r.Post("/login", p.handleLoginPost)
	r.Post("/register", p.handleRegisterPost)

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(requireAuth)
		r.Get("/dashboard", p.handleDashboard)
		r.Get("/tokens", p.handleTokens)
		r.Post("/tokens", p.handleCreateToken)
		r.Delete("/tokens/{id}", p.handleDeleteToken)
	})

	return r
}

func (p *Portal) renderTemplate(w http.ResponseWriter, tmpl string, data interface{}) {
	err := p.templates.ExecuteTemplate(w, tmpl, data)
	if err != nil {
		log.Printf("Error rendering template %s: %v", tmpl, err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}
}

func requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// TODO: Implement session validation
		next.ServeHTTP(w, r)
	})
}
