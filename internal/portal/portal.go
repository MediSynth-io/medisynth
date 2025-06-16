package portal

import (
	"context"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/MediSynth-io/medisynth/internal/auth"
	"github.com/MediSynth-io/medisynth/internal/config"
	"github.com/MediSynth-io/medisynth/internal/database"
	"github.com/MediSynth-io/medisynth/internal/s3"
	"github.com/go-chi/chi/v5"
)

type Portal struct {
	templates map[string]*template.Template
	config    *config.Config
	s3Client  *s3.Client
}

var funcMap = template.FuncMap{
	"humanizeBytes": func(bytes int64) string {
		if bytes < 1024 {
			return fmt.Sprintf("%d B", bytes)
		}
		if bytes < 1024*1024 {
			return fmt.Sprintf("%.2f KB", float64(bytes)/1024)
		}
		if bytes < 1024*1024*1024 {
			return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
		}
		return fmt.Sprintf("%.2f GB", float64(bytes)/(1024*1024*1024))
	},
}

func loadTemplates() (map[string]*template.Template, error) {
	templates := make(map[string]*template.Template)
	templateDir := "templates/portal"

	pages, err := fs.Glob(os.DirFS(templateDir), "*.html")
	if err != nil {
		return nil, fmt.Errorf("failed to glob templates: %w", err)
	}

	for _, page := range pages {
		name := filepath.Base(page)

		// Create a new template with the base name and func map.
		ts := template.New(name).Funcs(funcMap)

		// Determine which base template to use
		var baseTemplate string
		if strings.HasPrefix(name, "admin-") {
			baseTemplate = "admin-base.html"
		} else {
			baseTemplate = "base.html"
		}

		// Parse the files, using the appropriate base template
		ts, err := ts.ParseFiles(
			filepath.Join(templateDir, baseTemplate),
			filepath.Join(templateDir, page),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to parse template %s: %w", page, err)
		}
		templates[page] = ts
	}

	log.Printf("Successfully loaded %d templates", len(templates))
	return templates, nil
}

func New(cfg *config.Config) (*Portal, error) {
	templates, err := loadTemplates()
	if err != nil {
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	// Create S3 client
	s3Client, err := s3.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create S3 client: %w", err)
	}

	return &Portal{
		templates: templates,
		config:    cfg,
		s3Client:  s3Client,
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
		log.Printf("Handling root request for host: %s", r.Host)

		// Check if this is the main domain (medisynth.io) or subdomain (portal.medisynth.io)
		if strings.Contains(r.Host, "portal.") {
			// This is the portal subdomain, check if user is logged in
			var userID string

			// Check for valid session cookie
			cookie, err := r.Cookie("session")
			if err == nil && cookie.Value != "" {
				// Validate the session
				userID, err = auth.ValidateSession(cookie.Value)
				if err == nil && userID != "" {
					log.Printf("[HOME] Found valid session for user: %s", userID)
				}
			}

			if userID != "" {
				// User is logged in, add user context and serve logged-in home page
				ctx := r.Context()
				ctx = context.WithValue(ctx, "userID", userID)
				p.HandleHome(w, r.WithContext(ctx))
			} else {
				// User is not logged in, serve the public portal home page
				log.Printf("[HOME] No valid session found, serving public home page")
				p.HandleHome(w, r)
			}
		} else {
			// This is the main domain, serve the landing page
			p.handleLanding(w, r)
		}
	})
	r.Get("/about", p.handleAboutRedirect)
	r.Get("/login", p.handleLoginRedirect)
	r.Get("/register", p.handleRegisterRedirect)
	r.Get("/privacy", p.handlePrivacyPolicy)
	r.Post("/login", p.handleLoginRedirect)
	r.Post("/register", p.handleRegisterRedirect)

	// Favicon
	r.Get("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "static/favicon.ico")
	})

	// Logout route
	r.Get("/logout", p.handleLogout)

	// Debug route to verify service is running
	r.Get("/debug", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"service":"medisynth-portal","status":"running","timestamp":"` + time.Now().Format(time.RFC3339) + `"}`))
	})

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(p.requireAuth)

		r.Get("/dashboard", p.handleDashboard)
		r.Get("/pricing", p.handlePricing)
		r.Get("/documentation", p.handleDocumentation)
		r.Handle("/swagger/*", http.HandlerFunc(p.handleSwaggerProxy))
		r.Get("/jobs", p.handleJobs)
		r.Get("/jobs/new", p.handleNewJob)
		r.Post("/jobs/new", p.handleCreateJob)
		r.Get("/jobs/{jobID}/outputs", p.handleJobOutputs)

		// Token management routes
		r.Route("/tokens", func(r chi.Router) {
			r.Get("/", p.handleTokens)
			r.Post("/create", p.handleCreateToken)
			r.Post("/{id}/delete", p.handleDeleteToken)
		})
	})

	// Admin routes
	r.Group(func(r chi.Router) {
		r.Use(p.requireAuth)
		r.Use(p.requireAdmin)
		r.Use(p.adminNavMiddleware)

		r.Get("/admin", p.handleAdminDashboard)
		r.Get("/admin/dashboard", p.handleAdminDashboard)
		r.Get("/admin/users", p.handleAdminUsers)
		r.Post("/admin/users/{userID}/delete", p.handleAdminDeleteUser)
		r.Post("/admin/users/{userID}/force-password-reset", p.handleAdminForcePasswordReset)
		r.Get("/admin/orders", p.handleAdminOrders)
		r.Post("/admin/orders/create", p.handleCreateOrder)
		r.Get("/admin/orders/{orderID}/edit", p.handleAdminEditOrderForm)
		r.Post("/admin/orders/{orderID}/edit", p.handleAdminUpdateOrder)
	})

	// User order routes
	r.Group(func(r chi.Router) {
		r.Use(p.requireAuth)

		r.Get("/orders", p.handleUserOrders)
		r.Get("/orders/create", p.handleCreateOrderForm)
		r.Get("/orders/{id}", p.handleOrderDetails)
		r.Post("/orders/create", p.handleCreateUserOrder)
	})

	// NotFound handler
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		p.renderTemplate(w, r, "404.html", "Not Found", map[string]interface{}{
			"Path": r.URL.Path,
		})
	})

	// Walk and log all registered routes
	walkFunc := func(method string, route string, handler http.Handler, middlewares ...func(http.Handler) http.Handler) error {
		log.Printf("Registered route: %s %s", method, route)
		return nil
	}
	if err := chi.Walk(r, walkFunc); err != nil {
		log.Printf("Error walking routes: %s\n", err.Error())
	}

	return r
}

func (p *Portal) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[AUTH] Checking authentication for %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)

		cookie, err := r.Cookie("session")
		if err != nil || cookie.Value == "" {
			log.Printf("[AUTH] No session cookie found: %v", err)
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}

		log.Printf("[AUTH] Found session cookie, length: %d", len(cookie.Value))

		userID, err := auth.ValidateSession(cookie.Value)
		if err != nil {
			log.Printf("[AUTH] Session validation failed: %v", err)
			// Clear invalid session cookie
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
			return
		}

		log.Printf("[AUTH] Session validation successful for user: %s", userID)
		ctx := r.Context()
		ctx = context.WithValue(ctx, "userID", userID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (p *Portal) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := r.Context().Value("userID").(string)
		if !ok {
			logRequest(r, "ADMIN_AUTH", "Forbidden: No user ID in context")
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		user, err := database.GetUserByID(userID)
		if err != nil {
			logRequest(r, "ADMIN_AUTH", "Forbidden: User not found with ID:", userID, "Error:", err)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		if !p.config.IsAdmin(user.Email) {
			logRequest(r, "ADMIN_AUTH", "Forbidden: User is not an admin:", user.Email)
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}

		logRequest(r, "ADMIN_AUTH", "Admin access granted")
		next.ServeHTTP(w, r)
	})
}

func (p *Portal) adminNavMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nav := []struct {
			Name string
			Path string
			Icon string // (Optional) for future use
		}{
			{"Dashboard", "/admin/dashboard", ""},
			{"Users", "/admin/users", ""},
			{"Orders", "/admin/orders", ""},
		}
		ctx := context.WithValue(r.Context(), "adminNav", nav)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (p *Portal) renderTemplate(w http.ResponseWriter, r *http.Request, templateName string, title string, data interface{}) {
	tmpl, err := template.New(templateName).Funcs(template.FuncMap{
		"humanizeBytes": func(bytes int64) string {
			if bytes < 1024 {
				return fmt.Sprintf("%d B", bytes)
			}
			if bytes < 1024*1024 {
				return fmt.Sprintf("%.2f KB", float64(bytes)/1024)
			}
			if bytes < 1024*1024*1024 {
				return fmt.Sprintf("%.2f MB", float64(bytes)/(1024*1024))
			}
			return fmt.Sprintf("%.2f GB", float64(bytes)/(1024*1024*1024))
		},
	}).ParseFiles(
		"templates/portal/"+templateName,
		"templates/portal/base.html",
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// ... rest of the function ...
}
