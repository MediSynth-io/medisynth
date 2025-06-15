package db

import "github.com/MediSynth-io/medisynth/internal/models"

// GetAllUsers retrieves all users from the database
func (db *DB) GetAllUsers() ([]models.User, error) {
	var users []models.User
	err := db.db.Find(&users).Error
	return users, err
}

// UpdateUserState updates a user's state
func (db *DB) UpdateUserState(userID string, state models.AccountState) error {
	return db.db.Model(&models.User{}).Where("id = ?", userID).Update("state", state).Error
}

// ForcePasswordReset forces a user to reset their password on next login
func (db *DB) ForcePasswordReset(userID string) error {
	return db.db.Model(&models.User{}).Where("id = ?", userID).Update("force_password_reset", true).Error
}
