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
	RecordVisit(context.Context, core.Visit) (int64, error)
	Stats(context.Context, string, int) (core.Stats, error)
	StatsForWorkspace(context.Context, string, string, int) (core.Stats, error)

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

	Ping(context.Context) error
	Close() error
}
