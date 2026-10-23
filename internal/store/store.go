// Package store define la persistencia de la información PROPIA de la aplicación:
// usuarios, sesiones y bitácora de auditoría. Los datos de GitHub NO se replican aquí.
package store

import (
	"context"
	"errors"
	"time"
)

var (
	ErrNotFound  = errors.New("no encontrado")
	ErrDuplicate = errors.New("registro duplicado")
)

const (
	RoleAdmin  = "admin"
	RoleViewer = "viewer"
)

type User struct {
	ID                 string    `bson:"_id" json:"id"`
	Email              string    `bson:"email" json:"email"`
	Name               string    `bson:"name" json:"name"`
	PasswordHash       string    `bson:"password_hash" json:"-"`
	Role               string    `bson:"role" json:"role"`
	Active             bool      `bson:"active" json:"active"`
	MustChangePassword bool      `bson:"must_change_password" json:"mustChangePassword"`
	CreatedAt          time.Time `bson:"created_at" json:"createdAt"`
	UpdatedAt          time.Time `bson:"updated_at" json:"updatedAt"`
	LastLoginAt        time.Time `bson:"last_login_at,omitempty" json:"lastLoginAt,omitempty"`
}

type Session struct {
	TokenHash  string    `bson:"_id"`
	UserID     string    `bson:"user_id"`
	CSRFToken  string    `bson:"csrf_token"`
	CreatedAt  time.Time `bson:"created_at"`
	LastSeenAt time.Time `bson:"last_seen_at"`
	ExpiresAt  time.Time `bson:"expires_at"` // límite absoluto (índice TTL en Mongo)
	IP         string    `bson:"ip"`
	UserAgent  string    `bson:"user_agent"`
}

type AuditEntry struct {
	ID        string    `bson:"_id" json:"id"`
	At        time.Time `bson:"at" json:"at"`
	ActorID   string    `bson:"actor_id" json:"actorId"`
	ActorMail string    `bson:"actor_email" json:"actorEmail"`
	Action    string    `bson:"action" json:"action"`
	Target    string    `bson:"target" json:"target"`
	IP        string    `bson:"ip" json:"ip"`
	Success   bool      `bson:"success" json:"success"`
}

type Store interface {
	CountUsers(ctx context.Context) (int64, error)
	CreateUser(ctx context.Context, u *User) error
	GetUserByID(ctx context.Context, id string) (*User, error)
	GetUserByEmail(ctx context.Context, email string) (*User, error)
	ListUsers(ctx context.Context) ([]User, error)
	UpdateUser(ctx context.Context, u *User) error

	CreateSession(ctx context.Context, s *Session) error
	GetSession(ctx context.Context, tokenHash string) (*Session, error)
	TouchSession(ctx context.Context, tokenHash string, at time.Time) error
	DeleteSession(ctx context.Context, tokenHash string) error
	DeleteUserSessions(ctx context.Context, userID string) error

	AddAudit(ctx context.Context, e *AuditEntry) error
	ListAudit(ctx context.Context, limit int) ([]AuditEntry, error)

	Ping(ctx context.Context) error
	Close(ctx context.Context) error
}
