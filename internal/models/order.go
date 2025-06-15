package models

import (
	"time"
)

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

	// Relations
	User     *User     `json:"user,omitempty"`
	Payments []Payment `json:"payments,omitempty"`
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

	// Relations
	Order *Order `json:"order,omitempty"`
}

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

// HasAmountBTC checks if the order has a Bitcoin amount set
func (o *Order) HasAmountBTC() bool {
	return o.AmountBTC != nil && *o.AmountBTC > 0
}

// GetAmountBTC returns the Bitcoin amount or 0 if not set
func (o *Order) GetAmountBTC() float64 {
	if o.AmountBTC == nil {
		return 0
	}
	return *o.AmountBTC
}

// HasQRCode checks if the order has QR code data
func (o *Order) HasQRCode() bool {
	return o.QRCodeData != nil && *o.QRCodeData != ""
}

// GetQRCodeData returns the QR code data or empty string if not set
func (o *Order) GetQRCodeData() string {
	if o.QRCodeData == nil {
		return ""
	}
	return *o.QRCodeData
}

// HasTransactionHash checks if the order has a transaction hash
func (o *Order) HasTransactionHash() bool {
	return o.TransactionHash != nil && *o.TransactionHash != ""
}

// GetTransactionHash returns the transaction hash or empty string if not set
func (o *Order) GetTransactionHash() string {
	if o.TransactionHash == nil {
		return ""
	}
	return *o.TransactionHash
}

// HasPaymentReceived checks if payment has been received
func (o *Order) HasPaymentReceived() bool {
	return o.PaymentReceivedAt != nil
}

// GetPaymentReceivedAt returns the payment received time or nil if not set
func (o *Order) GetPaymentReceivedAt() *time.Time {
	return o.PaymentReceivedAt
}

// HasExpiration checks if the order has an expiration time set
func (o *Order) HasExpiration() bool {
	return o.ExpiresAt != nil
}

// GetExpiresAt returns the expiration time or nil if not set
func (o *Order) GetExpiresAt() *time.Time {
	return o.ExpiresAt
}

// StatusColor returns a color class for the order status (template helper)
func (o *Order) StatusColor() string {
	return o.GetStatusColor()
}

// StatusDisplay returns a display-friendly status name
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

// GetStatusColor returns a color class for the order status
func (o *Order) GetStatusColor() string {
	switch o.Status {
	case OrderStatusPending:
		return "text-yellow-600 bg-yellow-100"
	case OrderStatusPaid:
		return "text-blue-600 bg-blue-100"
	case OrderStatusConfirmed:
		return "text-green-600 bg-green-100"
	case OrderStatusExpired:
		return "text-gray-600 bg-gray-100"
	case OrderStatusCancelled:
		return "text-red-600 bg-red-100"
	default:
		return "text-gray-600 bg-gray-100"
	}
}

// GetPaymentStatusColor returns a color class for the payment status
func (p *Payment) GetStatusColor() string {
	switch p.Status {
	case PaymentStatusPending:
		return "text-yellow-600 bg-yellow-100"
	case PaymentStatusDetected:
		return "text-blue-600 bg-blue-100"
	case PaymentStatusConfirmed:
		return "text-green-600 bg-green-100"
	case PaymentStatusFailed:
		return "text-red-600 bg-red-100"
	default:
		return "text-gray-600 bg-gray-100"
	}
}
