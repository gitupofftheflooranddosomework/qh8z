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
	ErrLimitExceeded   = errors.New("limit exceeded")
)

const (
	RoleOwner  = "owner"
	RoleAdmin  = "admin"
	RoleMember = "member"
)

const (
	URLRuleAllow = "allow"
	URLRuleBlock = "block"

	URLRuleHost   = "host"
	URLRuleDomain = "domain"
)

const (
	AbuseStatusOpen     = "open"
	AbuseStatusReviewed = "reviewed"
	AbuseStatusResolved = "resolved"
)

const (
	PlanFree = "free"
	PlanPro  = "pro"
)

const (
	BillingStatusActive   = "active"
	BillingStatusPastDue  = "past_due"
	BillingStatusCanceled = "canceled"
)

type Link struct {
	Slug             string     `json:"slug"`
	URL              string     `json:"url"`
	WorkspaceID      string     `json:"workspaceId,omitempty"`
	CreatedByUserID  string     `json:"createdByUserId,omitempty"`
	DomainID         string     `json:"domainId,omitempty"`
	DomainHost       string     `json:"domainHost,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	Visits           int64      `json:"visits"`
	DisabledAt       *time.Time `json:"disabledAt,omitempty"`
	SuspendedAt      *time.Time `json:"suspendedAt,omitempty"`
	SuspensionReason string     `json:"suspensionReason,omitempty"`
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

type AnalyticsDay struct {
	Date   string `json:"date"`
	Visits int64  `json:"visits"`
}

type AnalyticsLink struct {
	Slug       string `json:"slug"`
	URL        string `json:"url"`
	DomainHost string `json:"domainHost,omitempty"`
	Visits     int64  `json:"visits"`
}

type AnalyticsReferrer struct {
	Referrer string `json:"referrer"`
	Visits   int64  `json:"visits"`
}

type WorkspaceAnalytics struct {
	From         time.Time           `json:"from"`
	To           time.Time           `json:"to"`
	TotalLinks   int64               `json:"totalLinks"`
	ActiveLinks  int64               `json:"activeLinks"`
	TotalVisits  int64               `json:"totalVisits"`
	PeriodVisits int64               `json:"periodVisits"`
	Daily        []AnalyticsDay      `json:"daily"`
	TopLinks     []AnalyticsLink     `json:"topLinks"`
	Referrers    []AnalyticsReferrer `json:"referrers"`
}

type CustomDomain struct {
	ID                string     `json:"id"`
	WorkspaceID       string     `json:"workspaceId"`
	Host              string     `json:"host"`
	VerificationToken string     `json:"-"`
	VerifiedAt        *time.Time `json:"verifiedAt,omitempty"`
	CreatedAt         time.Time  `json:"createdAt"`
}

type WorkspaceUsage struct {
	WorkspaceID           string `json:"workspaceId"`
	PlanCode              string `json:"plan"`
	Links                 int64  `json:"links"`
	CustomDomains         int64  `json:"customDomains"`
	LinksCreatedThisMonth int64  `json:"linksCreatedThisMonth"`
}

type BillingState struct {
	WorkspaceID            string     `json:"workspaceId"`
	PlanCode               string     `json:"plan"`
	Status                 string     `json:"status"`
	ProviderCustomerID     string     `json:"-"`
	ProviderSubscriptionID string     `json:"-"`
	CurrentPeriodEnd       *time.Time `json:"currentPeriodEnd,omitempty"`
	UpdatedAt              time.Time  `json:"updatedAt"`
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

type RateLimitResult struct {
	Allowed   bool
	Limit     int
	Remaining int
	ResetAt   time.Time
}

type URLRule struct {
	ID        int64     `json:"id"`
	Action    string    `json:"action"`
	MatchType string    `json:"matchType"`
	Pattern   string    `json:"pattern"`
	Reason    string    `json:"reason,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
}

type AbuseReport struct {
	ID             string     `json:"id"`
	Slug           string     `json:"slug"`
	DestinationURL string     `json:"destinationUrl"`
	Category       string     `json:"category"`
	Details        string     `json:"details,omitempty"`
	ReporterEmail  string     `json:"reporterEmail,omitempty"`
	Status         string     `json:"status"`
	ReviewNotes    string     `json:"reviewNotes,omitempty"`
	CreatedAt      time.Time  `json:"createdAt"`
	ReviewedAt     *time.Time `json:"reviewedAt,omitempty"`
}
