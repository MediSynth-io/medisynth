package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5/middleware"
)

type contextKey string

const (
	UserContextKey contextKey = "user"
)

// AuthMiddleware creates a middleware that validates JWT tokens
func AuthMiddleware(tokenManager *TokenManager) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}

			parts := strings.Split(authHeader, " ")
			if len(parts) != 2 || parts[0] != "Bearer" {
				http.Error(w, "invalid authorization header", http.StatusUnauthorized)
				return
			}

			claims, err := tokenManager.ValidateToken(parts[1])
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// APIKeyMiddleware creates a middleware that validates API keys
func APIKeyMiddleware(apiKeyStore APIKeyStore) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			apiKey := r.Header.Get("X-API-Key")
			if apiKey == "" {
				http.Error(w, "api key required", http.StatusUnauthorized)
				return
			}

			key, err := apiKeyStore.GetByKey(apiKey)
			if err != nil {
				http.Error(w, "invalid api key", http.StatusUnauthorized)
				return
			}

			ctx := context.WithValue(r.Context(), UserContextKey, key)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// GetUserFromContext retrieves the user claims from the context
func GetUserFromContext(ctx context.Context) (*TokenClaims, bool) {
	claims, ok := ctx.Value(UserContextKey).(*TokenClaims)
	return claims, ok
}

// GetAPIKeyFromContext retrieves the API key from the context
func GetAPIKeyFromContext(ctx context.Context) (*APIKey, bool) {
	key, ok := ctx.Value(UserContextKey).(*APIKey)
	return key, ok
}

// RequireAuth is a middleware that ensures the request has a valid JWT token
func RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims, ok := GetUserFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Add request ID to context for tracking
		ctx := context.WithValue(r.Context(), middleware.RequestIDKey, claims.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireAPIKey is a middleware that ensures the request has a valid API key
func RequireAPIKey(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, ok := GetAPIKeyFromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Add request ID to context for tracking
		ctx := context.WithValue(r.Context(), middleware.RequestIDKey, key.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
