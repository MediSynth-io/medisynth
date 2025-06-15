package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/MediSynth-io/medisynth/internal/auth"
)

func (api *Api) TokenAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
			http.Error(w, "Authorization header format must be Bearer {token}", http.StatusUnauthorized)
			return
		}

		tokenString := parts[1]
		token, err := auth.ValidateToken(tokenString)
		if err != nil {
			http.Error(w, "Invalid or expired token", http.StatusUnauthorized)
			return
		}

		// Add user information to context
		ctx := context.WithValue(r.Context(), "userID", token.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
