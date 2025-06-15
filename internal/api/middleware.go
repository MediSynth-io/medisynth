package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/MediSynth-io/medisynth/internal/auth"
)

// UnifiedAuthMiddleware checks for a valid session cookie or a bearer token.
// It brings together session-based auth for the portal and token-based auth for the API.
func (api *Api) UnifiedAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// First, try to authenticate using a Bearer token from the Authorization header.
		// This is for programmatic API access.
		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && strings.ToLower(parts[0]) == "bearer" {
				tokenStr := parts[1]
				token, err := auth.ValidateToken(tokenStr)
				if err == nil {
					// Token is valid, add user ID to context and proceed.
					ctx := context.WithValue(r.Context(), "userID", token.UserID)
					next.ServeHTTP(w, r.WithContext(ctx))
					return
				}
			}
		}

		// If no valid bearer token, try to authenticate using a session cookie.
		// This is for users logged into the web portal.
		cookie, err := r.Cookie("session")
		if err == nil {
			sessionToken := cookie.Value
			userID, err := auth.ValidateSession(sessionToken)
			if err == nil {
				// Session is valid, add user ID to context and proceed.
				ctx := context.WithValue(r.Context(), "userID", userID)
				next.ServeHTTP(w, r.WithContext(ctx))
				return
			}
		}

		// If neither method succeeds, the user is not authenticated.
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}
