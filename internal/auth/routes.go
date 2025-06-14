package auth

import (
	"github.com/go-chi/chi/v5"
)

// RegisterRoutes sets up the authentication routes
func RegisterRoutes(r chi.Router, userStore UserStore, tokenManager *TokenManager, apiKeyStore APIKeyStore) {
	// Public routes
	r.Group(func(r chi.Router) {
		r.Post("/register", RegisterHandler)
		r.Post("/login", LoginHandler)
	})

	// Protected routes
	r.Group(func(r chi.Router) {
		r.Use(AuthMiddleware(tokenManager))
		r.Use(RequireAuth)

		// API key management
		r.Post("/api-keys", CreateTokenHandler)
		r.Get("/api-keys", ListTokensHandler)
		r.Delete("/api-keys/{id}", DeleteTokenHandler)
	})
}
