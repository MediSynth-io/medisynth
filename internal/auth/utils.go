package auth

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
)

// generateAPIKey generates a secure random API key
func generateAPIKey() (string, error) {
	// Generate 32 random bytes
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode as base64 and add a prefix
	key := base64.URLEncoding.EncodeToString(b)
	return fmt.Sprintf("ms_%s", key), nil
}
