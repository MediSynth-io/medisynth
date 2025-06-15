package models

import (
	"encoding/json"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// User represents a user account in the database
type User struct {
	ID                 string    `json:"id" db:"id"`
	Email              string    `json:"email" db:"email"`
	Password           string    `json:"-" db:"password"`
	IsAdmin            bool      `json:"is_admin" db:"is_admin"`
	ForcePasswordReset bool      `json:"force_password_reset" db:"force_password_reset"`
	CreatedAt          time.Time `json:"created_at" db:"created_at"`
	UpdatedAt          time.Time `json:"updated_at" db:"updated_at"`
}

// ValidatePassword checks if the provided password is valid for the user.
func (u *User) ValidatePassword(password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password))
	return err == nil
}

// Token represents an API token
type Token struct {
	ID        string     `json:"id" db:"id"`
	UserID    string     `json:"user_id" db:"user_id"`
	Token     string     `json:"token" db:"token"`
	Name      string     `json:"name" db:"name"`
	CreatedAt time.Time  `json:"created_at" db:"created_at"`
	ExpiresAt *time.Time `json:"expires_at,omitempty" db:"expires_at"`
}

// Session represents a user's session
type Session struct {
	ID        string    `json:"id" db:"id"`
	UserID    string    `json:"user_id" db:"user_id"`
	Token     string    `json:"token" db:"token"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
	ExpiresAt time.Time `json:"expires_at" db:"expires_at"`
}

// --- Order & Payment Models ---

type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"
	OrderStatusPaid      OrderStatus = "paid"
	OrderStatusConfirmed OrderStatus = "confirmed"
	OrderStatusExpired   OrderStatus = "expired"
	OrderStatusCancelled OrderStatus = "cancelled"
)

type PaymentStatus string

const (
	PaymentStatusPending   PaymentStatus = "pending"
	PaymentStatusDetected  PaymentStatus = "detected"
	PaymentStatusConfirmed PaymentStatus = "confirmed"
	PaymentStatusFailed    PaymentStatus = "failed"
)

// Order represents a Bitcoin payment order
type Order struct {
	ID                string      `json:"id" db:"id"`
	UserID            string      `json:"user_id" db:"user_id"`
	OrderNumber       string      `json:"order_number" db:"order_number"`
	Description       string      `json:"description" db:"description"`
	AmountUSD         float64     `json:"amount_usd" db:"amount_usd"`
	AmountBTC         *float64    `json:"amount_btc" db:"amount_btc"`
	BTCAddress        string      `json:"btc_address" db:"btc_address"`
	QRCodeData        *string     `json:"qr_code_data" db:"qr_code_data"`
	Status            OrderStatus `json:"status" db:"status"`
	PaymentReceivedAt *time.Time  `json:"payment_received_at" db:"payment_received_at"`
	TransactionHash   *string     `json:"transaction_hash" db:"transaction_hash"`
	Confirmations     int         `json:"confirmations" db:"confirmations"`
	ExpiresAt         *time.Time  `json:"expires_at" db:"expires_at"`
	CreatedAt         time.Time   `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time   `json:"updated_at" db:"updated_at"`
	User              *User       `json:"user,omitempty"`
	Payments          []*Payment  `json:"payments,omitempty"`
}

// Payment represents a Bitcoin payment transaction
type Payment struct {
	ID              string        `json:"id" db:"id"`
	OrderID         string        `json:"order_id" db:"order_id"`
	TransactionHash string        `json:"transaction_hash" db:"transaction_hash"`
	AmountBTC       float64       `json:"amount_btc" db:"amount_btc"`
	Confirmations   int           `json:"confirmations" db:"confirmations"`
	Status          PaymentStatus `json:"status" db:"status"`
	DetectedAt      time.Time     `json:"detected_at" db:"detected_at"`
	ConfirmedAt     *time.Time    `json:"confirmed_at" db:"confirmed_at"`
	CreatedAt       time.Time     `json:"created_at" db:"created_at"`
	Order           *Order        `json:"order,omitempty"`
}

// --- Job & Synthea Models ---

type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

// SyntheaParams defines the parameters for a Synthea generation job
type SyntheaParams struct {
	Population   *int    `json:"population,omitempty"`
	Gender       *string `json:"gender,omitempty"`
	AgeMin       *int    `json:"ageMin,omitempty"`
	AgeMax       *int    `json:"ageMax,omitempty"`
	State        *string `json:"state,omitempty"`
	City         *string `json:"city,omitempty"`
	OutputFormat *string `json:"outputFormat,omitempty"`
}

// Job represents a data generation job
type Job struct {
	ID             string         `json:"id" db:"id"`
	UserID         string         `json:"user_id" db:"user_id"`
	JobID          string         `json:"job_id" db:"job_id"`
	Status         JobStatus      `json:"status" db:"status"`
	ParametersJSON []byte         `json:"-" db:"parameters"` // Raw JSON for DB
	Parameters     *SyntheaParams `json:"parameters" gorm:"-"`
	OutputFormat   *string        `json:"output_format" db:"output_format"`
	OutputPath     *string        `json:"output_path" db:"output_path"`
	OutputSize     *int64         `json:"output_size" db:"output_size"`
	PatientCount   *int           `json:"patient_count" db:"patient_count"`
	ErrorMessage   *string        `json:"error_message" db:"error_message"`
	CreatedAt      time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at" db:"updated_at"`
	CompletedAt    *time.Time     `json:"completed_at" db:"completed_at"`
}

// JobFile represents a file generated by a job
type JobFile struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	Timestamp time.Time `json:"timestamp"`
	URL       string    `json:"url"`
}

// MarshalParameters converts the Parameters struct to JSON bytes for the database.
func (j *Job) MarshalParameters() error {
	if j.Parameters == nil {
		j.ParametersJSON = nil
		return nil
	}
	bytes, err := json.Marshal(j.Parameters)
	if err != nil {
		return err
	}
	j.ParametersJSON = bytes
	return nil
}

// UnmarshalParameters converts the JSON bytes from the database to the Parameters struct.
func (j *Job) UnmarshalParameters() error {
	if j.ParametersJSON == nil {
		return nil
	}
	params := &SyntheaParams{}
	if err := json.Unmarshal(j.ParametersJSON, params); err != nil {
		return err
	}
	j.Parameters = params
	return nil
}

// --- Template Helper Methods ---
// These are methods attached to the models to help with presentation logic in templates.

// IsExpired checks if the order has expired
func (o *Order) IsExpired() bool {
	if o.ExpiresAt == nil {
		return false
	}
	return time.Now().After(*o.ExpiresAt)
}

// IsPaymentComplete checks if the order has been fully paid and confirmed
func (o *Order) IsPaymentComplete() bool {
	return o.Status == OrderStatusConfirmed
}

// StatusDisplay returns a display-friendly status name for an order.
func (o *Order) StatusDisplay() string {
	switch o.Status {
	case OrderStatusPending:
		return "Pending Payment"
	case OrderStatusPaid:
		return "Payment Received"
	case OrderStatusConfirmed:
		return "Confirmed"
	case OrderStatusExpired:
		return "Expired"
	case OrderStatusCancelled:
		return "Cancelled"
	default:
		return string(o.Status)
	}
}

// StatusColor returns a Tailwind CSS color class based on the order status.
func (o *Order) StatusColor() string {
	switch o.Status {
	case OrderStatusPending:
		return "bg-yellow-100 text-yellow-800"
	case OrderStatusPaid:
		return "bg-blue-100 text-blue-800"
	case OrderStatusConfirmed:
		return "bg-green-100 text-green-800"
	case OrderStatusExpired, OrderStatusCancelled:
		return "bg-red-100 text-red-800"
	default:
		return "bg-gray-100 text-gray-800"
	}
}
