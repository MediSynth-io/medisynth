// User represents a user account
type User struct {
	ID                 string    `json:"id" db:"id"`
	Email              string    `json:"email" db:"email"`
	Password           string    `json:"-" db:"password"`
	IsAdmin            bool      `json:"is_admin" db:"is_admin"`
	ForcePasswordReset bool      `json:"force_password_reset" db:"force_password_reset"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
} 