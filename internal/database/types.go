package database

import "time"

type User struct {
	ID        int64
	Email     string
	Password  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type Token struct {
	ID         int64
	UserID     int64
	Name       string
	Token      string
	CreatedAt  time.Time
	ExpiresAt  *time.Time
	LastUsedAt *time.Time
}

type Session struct {
	ID        int64
	UserID    int64
	Token     string
	CreatedAt time.Time
	ExpiresAt time.Time
}
