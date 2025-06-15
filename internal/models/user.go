package models

import (
	"time"
)

// AccountState represents the current state of a user's account
type AccountState string

const (
	AccountStateActive  AccountState = "active"  // Normal active account
	AccountStateOnHold  AccountState = "on_hold" // Account temporarily suspended
	AccountStatePaid    AccountState = "paid"    // Paid subscription account
	AccountStateFree    AccountState = "free"    // Free tier account
	AccountStateDeleted AccountState = "deleted" // Soft-deleted account
)

// User represents a user in the system
type User struct {
	ID        string       `json:"id" db:"id"`
	Email     string       `json:"email" db:"email"`
	Password  string       `json:"-" db:"password"` // Password is never sent to client
	State     AccountState `json:"state" db:"state"`
	CreatedAt time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt time.Time    `json:"updated_at" db:"updated_at"`
	LastLogin time.Time    `json:"last_login" db:"last_login"`
}

// IsActive returns true if the user's account is in an active state
func (u *User) IsActive() bool {
	return u.State == AccountStateActive || u.State == AccountStatePaid || u.State == AccountStateFree
}

// CanUseAPI returns true if the user can use the API
func (u *User) CanUseAPI() bool {
	return u.IsActive() && u.State != AccountStateOnHold
}
