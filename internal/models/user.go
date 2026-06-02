package models

import "time"

type Role string

const (
	RoleReader Role = "reader"
	RoleWriter Role = "writer"
	RoleAdmin  Role = "admin"
)

var roleOrder = map[Role]int{
	RoleReader: 0,
	RoleWriter: 1,
	RoleAdmin:  2,
}

func (r Role) AtLeast(min Role) bool {
	rv, ok := roleOrder[r]
	if !ok {
		return false
	}
	mv, ok := roleOrder[min]
	if !ok {
		return false
	}
	return rv >= mv
}

type User struct {
	ID           string
	Username     string
	PasswordHash string
	Role         Role
	OIDCSubject  string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type Session struct {
	Token     string
	UserID    string
	ExpiresAt time.Time
	CreatedAt time.Time
}
