package core

import (
	"errors"
	"time"
)

var (
	ErrNotFound        = errors.New("not found")
	ErrConflict        = errors.New("conflict")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrForbidden       = errors.New("forbidden")
	ErrExpired         = errors.New("expired")
	ErrEmailUnverified = errors.New("email unverified")
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

type Link struct {
	Slug            string    `json:"slug"`
	URL             string    `json:"url"`
	WorkspaceID     string    `json:"workspaceId,omitempty"`
	CreatedByUserID string    `json:"createdByUserId,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	Visits          int64     `json:"visits"`
}

type Visit struct {
	Slug      string    `json:"-"`
	VisitedAt time.Time `json:"visitedAt"`
	Referer   string    `json:"referer,omitempty"`
	UserAgent string    `json:"userAgent,omitempty"`
}

type Stats struct {
	TotalVisits int64   `json:"totalVisits"`
	Recent      []Visit `json:"recentVisits"`
}

type User struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	PasswordHash    string     `json:"-"`
	EmailVerifiedAt *time.Time `json:"emailVerifiedAt,omitempty"`
	CreatedAt       time.Time  `json:"createdAt"`
}

type Workspace struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	CreatedAt time.Time `json:"createdAt"`
}

type Membership struct {
	WorkspaceID string    `json:"workspaceId"`
	UserID      string    `json:"userId"`
	Email       string    `json:"email,omitempty"`
	Role        string    `json:"role"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Session struct {
	TokenHash  []byte
	UserID     string
	CreatedAt  time.Time
	ExpiresAt  time.Time
	LastSeenAt time.Time
}

type EmailVerification struct {
	TokenHash []byte
	UserID    string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
}

type APIKey struct {
	ID              string     `json:"id"`
	WorkspaceID     string     `json:"workspaceId"`
	Name            string     `json:"name"`
	KeyHash         []byte     `json:"-"`
	Scopes          []string   `json:"scopes"`
	CreatedByUserID string     `json:"createdByUserId"`
	CreatedAt       time.Time  `json:"createdAt"`
	LastUsedAt      *time.Time `json:"lastUsedAt,omitempty"`
	RevokedAt       *time.Time `json:"revokedAt,omitempty"`
}

type AuditEntry struct {
	ID            int64             `json:"id"`
	WorkspaceID   string            `json:"workspaceId,omitempty"`
	ActorUserID   string            `json:"actorUserId,omitempty"`
	ActorAPIKeyID string            `json:"actorApiKeyId,omitempty"`
	Action        string            `json:"action"`
	ResourceType  string            `json:"resourceType"`
	ResourceID    string            `json:"resourceId"`
	Metadata      map[string]string `json:"metadata,omitempty"`
	CreatedAt     time.Time         `json:"createdAt"`
}

type Registration struct {
	User         User
	Workspace    Workspace
	Membership   Membership
	Verification EmailVerification
	Session      Session
	Audit        []AuditEntry
}

type AuthContext struct {
	UserID        string   `json:"userId"`
	Email         string   `json:"email,omitempty"`
	EmailVerified bool     `json:"emailVerified"`
	WorkspaceID   string   `json:"workspaceId"`
	Role          string   `json:"role,omitempty"`
	APIKeyID      string   `json:"apiKeyId,omitempty"`
	Scopes        []string `json:"scopes,omitempty"`
	Credential    string   `json:"-"`
}
