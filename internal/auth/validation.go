package auth

import (
	"regexp"
	"unicode"
)

var emailRegex = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)

// ValidateEmail checks if an email is valid
func ValidateEmail(email string) bool {
	return emailRegex.MatchString(email) && len(email) < 255
}

// ValidatePassword checks if a password is valid
func ValidatePassword(password string) bool {
	if len(password) < 8 || len(password) > 72 {
		return false
	}

	var (
		hasUpper   bool
		hasLower   bool
		hasNumber  bool
		hasSpecial bool
	)

	for _, char := range password {
		switch {
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsNumber(char):
			hasNumber = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	// Password must contain at least 3 of the 4 character types
	requirements := 0
	if hasUpper {
		requirements++
	}
	if hasLower {
		requirements++
	}
	if hasNumber {
		requirements++
	}
	if hasSpecial {
		requirements++
	}

	return requirements >= 3
}

// GetPasswordRequirements returns a list of password requirements
func GetPasswordRequirements() []string {
	return []string{
		"At least 8 characters long",
		"Maximum 72 characters",
		"Must contain at least 3 of the following:",
		"- Uppercase letters (A-Z)",
		"- Lowercase letters (a-z)",
		"- Numbers (0-9)",
		"- Special characters (!@#$%^&*...)",
	}
}
