package storage

import (
	"context"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

type Store interface {
	CreateLink(context.Context, core.Link) error
	CreateOwnedLink(context.Context, core.Link, core.AuditEntry) error
	GetLink(context.Context, string) (core.Link, error)
	GetWorkspaceLink(context.Context, string, string) (core.Link, error)
	GetCustomDomainLink(context.Context, string, string) (core.Link, error)
	ListWorkspaceLinks(context.Context, string, int, int) ([]core.Link, error)
	UpdateWorkspaceLink(context.Context, string, string, string, string, time.Time, core.AuditEntry) (core.Link, error)
	SetWorkspaceLinkDisabled(context.Context, string, string, bool, time.Time, core.AuditEntry) (core.Link, error)
	DeleteWorkspaceLink(context.Context, string, string, core.AuditEntry) error
	RecordVisit(context.Context, core.Visit) (int64, error)
	Stats(context.Context, string, int) (core.Stats, error)
	StatsForWorkspace(context.Context, string, string, int) (core.Stats, error)
	WorkspaceAnalytics(context.Context, string, time.Time, time.Time, int) (core.WorkspaceAnalytics, error)
	WorkspaceUsage(context.Context, string, time.Time) (core.WorkspaceUsage, error)

	CreateCustomDomain(context.Context, core.CustomDomain, core.AuditEntry) error
	ListCustomDomains(context.Context, string) ([]core.CustomDomain, error)
	GetCustomDomain(context.Context, string, string) (core.CustomDomain, error)
	SetCustomDomainVerified(context.Context, string, string, time.Time, core.AuditEntry) (core.CustomDomain, error)
	DeleteCustomDomain(context.Context, string, string, core.AuditEntry) error

	GetBillingState(context.Context, string) (core.BillingState, error)
	UpsertBillingState(context.Context, core.BillingState, core.AuditEntry) error
	ClaimBillingEvent(context.Context, string, time.Time) (bool, error)
	ReleaseBillingEvent(context.Context, string) error

	Register(context.Context, core.Registration) error
	UserByEmail(context.Context, string) (core.User, error)
	CreateSession(context.Context, core.Session, core.AuditEntry) error
	DeleteSession(context.Context, []byte) error
	ResolveSession(context.Context, []byte, string, time.Time) (core.AuthContext, error)
	CreateEmailVerification(context.Context, core.EmailVerification) error
	ConsumeEmailVerification(context.Context, []byte, time.Time) (core.User, error)

	CreateWorkspace(context.Context, core.Workspace, string, core.AuditEntry) error
	ListWorkspaces(context.Context, string) ([]core.Workspace, error)
	AddWorkspaceMember(context.Context, core.Membership, core.AuditEntry) error
	ListWorkspaceMembers(context.Context, string) ([]core.Membership, error)

	CreateAPIKey(context.Context, core.APIKey, core.AuditEntry) error
	ResolveAPIKey(context.Context, []byte, time.Time) (core.AuthContext, error)

	WriteAudit(context.Context, core.AuditEntry) error
	ListAudit(context.Context, string, int) ([]core.AuditEntry, error)

	CheckRateLimit(context.Context, string, time.Time, time.Time, int) (core.RateLimitResult, error)

	MatchURLRule(context.Context, string) (core.URLRule, error)
	CreateURLRule(context.Context, core.URLRule) (core.URLRule, error)
	ListURLRules(context.Context) ([]core.URLRule, error)
	DeleteURLRule(context.Context, int64) error

	CreateAbuseReport(context.Context, core.AbuseReport) error
	ListAbuseReports(context.Context, string, int) ([]core.AbuseReport, error)
	UpdateAbuseReport(context.Context, string, string, string, time.Time) (core.AbuseReport, error)
	SetLinkSuspension(context.Context, string, bool, string, time.Time, core.AuditEntry) (core.Link, error)

	Ping(context.Context) error
	Close() error
}
