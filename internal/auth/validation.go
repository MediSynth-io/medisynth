package auth

// validateEmail checks if an email is valid
func validateEmail(email string) bool {
	// TODO: Implement proper email validation
	return len(email) > 0 && len(email) < 255
}

// validatePassword checks if a password is valid
func validatePassword(password string) bool {
	// TODO: Implement proper password validation
	return len(password) >= 8
}
