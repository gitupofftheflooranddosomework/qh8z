package postgres

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

func TestPostgresCommercialLifecycle(t *testing.T) {
	dsn := os.Getenv("QH8Z_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("QH8Z_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	s, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer s.Close()

	now := time.Now().UTC()
	suffix := fmt.Sprintf("%d", now.UnixNano())
	userID := "usr_commercial_" + suffix
	workspaceID := "ws_commercial_" + suffix
	email := "commercial-" + suffix + "@example.com"
	reg := core.Registration{
		User:       core.User{ID: userID, Email: email, PasswordHash: "test-hash", EmailVerifiedAt: &now, CreatedAt: now},
		Workspace:  core.Workspace{ID: workspaceID, Name: "Commercial Workspace", CreatedAt: now},
		Membership: core.Membership{WorkspaceID: workspaceID, UserID: userID, Role: core.RoleOwner, CreatedAt: now},
		Session:    core.Session{TokenHash: testHash("commercial-session-" + suffix), UserID: userID, CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now},
		Audit:      []core.AuditEntry{{WorkspaceID: workspaceID, ActorUserID: userID, Action: "workspace.created", ResourceType: "workspace", ResourceID: workspaceID, CreatedAt: now}},
	}
	if err := s.Register(ctx, reg); err != nil {
		t.Fatalf("register commercial workspace: %v", err)
	}

	billing := core.BillingState{
		WorkspaceID:            workspaceID,
		PlanCode:               core.PlanPro,
		Status:                 core.BillingStatusActive,
		ProviderCustomerID:     "cus_" + suffix,
		ProviderSubscriptionID: "sub_" + suffix,
		UpdatedAt:              now,
	}
	if err := s.UpsertBillingState(ctx, billing, core.AuditEntry{WorkspaceID: workspaceID, Action: "billing.subscription_updated", ResourceType: "billing", ResourceID: billing.ProviderSubscriptionID, CreatedAt: now}); err != nil {
		t.Fatalf("upsert billing: %v", err)
	}
	gotBilling, err := s.GetBillingState(ctx, workspaceID)
	if err != nil || gotBilling.PlanCode != core.PlanPro || gotBilling.ProviderCustomerID != billing.ProviderCustomerID {
		t.Fatalf("billing = %+v, err = %v", gotBilling, err)
	}

	domain := core.CustomDomain{
		ID:                "dom_" + suffix,
		WorkspaceID:       workspaceID,
		Host:              "go-" + suffix + ".example.com",
		VerificationToken: "token-" + suffix,
		CreatedAt:         now,
	}
	if err := s.CreateCustomDomain(ctx, domain, core.AuditEntry{WorkspaceID: workspaceID, ActorUserID: userID, Action: "custom_domain.created", ResourceType: "custom_domain", ResourceID: domain.ID, CreatedAt: now}); err != nil {
		t.Fatalf("create custom domain: %v", err)
	}
	domains, err := s.ListCustomDomains(ctx, workspaceID)
	if err != nil || len(domains) != 1 || domains[0].VerifiedAt != nil {
		t.Fatalf("domains = %+v, err = %v", domains, err)
	}
	verified, err := s.SetCustomDomainVerified(ctx, workspaceID, domain.ID, now.Add(time.Second), core.AuditEntry{WorkspaceID: workspaceID, ActorUserID: userID, Action: "custom_domain.verified", ResourceType: "custom_domain", ResourceID: domain.ID, CreatedAt: now.Add(time.Second)})
	if err != nil || verified.VerifiedAt == nil {
		t.Fatalf("verify domain = %+v, err = %v", verified, err)
	}

	slug := "commercial-" + suffix
	link := core.Link{
		Slug:            slug,
		URL:             "https://example.com/original",
		WorkspaceID:     workspaceID,
		CreatedByUserID: userID,
		DomainID:        domain.ID,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := s.CreateOwnedLink(ctx, link, core.AuditEntry{WorkspaceID: workspaceID, ActorUserID: userID, Action: "link.created", ResourceType: "link", ResourceID: slug, CreatedAt: now}); err != nil {
		t.Fatalf("create branded link: %v", err)
	}
	branded, err := s.GetCustomDomainLink(ctx, domain.Host, slug)
	if err != nil || branded.DomainHost != domain.Host {
		t.Fatalf("get branded link = %+v, err = %v", branded, err)
	}
	links, err := s.ListWorkspaceLinks(ctx, workspaceID, 50, 0)
	if err != nil || len(links) != 1 {
		t.Fatalf("workspace links = %+v, err = %v", links, err)
	}

	updatedAt := now.Add(2 * time.Second)
	updated, err := s.UpdateWorkspaceLink(ctx, workspaceID, slug, "https://example.com/updated", domain.ID, updatedAt, core.AuditEntry{WorkspaceID: workspaceID, ActorUserID: userID, Action: "link.updated", ResourceType: "link", ResourceID: slug, CreatedAt: updatedAt})
	if err != nil || updated.URL != "https://example.com/updated" || updated.DomainHost != domain.Host {
		t.Fatalf("update link = %+v, err = %v", updated, err)
	}

	if _, err := s.RecordVisit(ctx, core.Visit{Slug: slug, VisitedAt: now.Add(3 * time.Second), Referer: "https://ref.example/", UserAgent: "commercial-test"}); err != nil {
		t.Fatalf("record commercial visit: %v", err)
	}
	analytics, err := s.WorkspaceAnalytics(ctx, workspaceID, now.Add(-time.Minute), now.Add(time.Minute), 10)
	if err != nil || analytics.TotalLinks != 1 || analytics.PeriodVisits != 1 || len(analytics.TopLinks) != 1 || len(analytics.Referrers) != 1 {
		t.Fatalf("analytics = %+v, err = %v", analytics, err)
	}
	usage, err := s.WorkspaceUsage(ctx, workspaceID, time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC))
	if err != nil || usage.PlanCode != core.PlanPro || usage.Links != 1 || usage.CustomDomains != 1 || usage.LinksCreatedThisMonth != 1 {
		t.Fatalf("usage = %+v, err = %v", usage, err)
	}

	disabled, err := s.SetWorkspaceLinkDisabled(ctx, workspaceID, slug, true, now.Add(4*time.Second), core.AuditEntry{WorkspaceID: workspaceID, ActorUserID: userID, Action: "link.disabled", ResourceType: "link", ResourceID: slug, CreatedAt: now.Add(4 * time.Second)})
	if err != nil || disabled.DisabledAt == nil {
		t.Fatalf("disable link = %+v, err = %v", disabled, err)
	}
	if _, err := s.RecordVisit(ctx, core.Visit{Slug: slug, VisitedAt: now.Add(5 * time.Second)}); !errors.Is(err, core.ErrNotFound) {
		t.Fatalf("disabled link visit err = %v", err)
	}

	eventID := "evt_" + suffix
	claimed, err := s.ClaimBillingEvent(ctx, eventID, now)
	if err != nil || !claimed {
		t.Fatalf("first billing event claim = %v, err = %v", claimed, err)
	}
	claimed, err = s.ClaimBillingEvent(ctx, eventID, now)
	if err != nil || claimed {
		t.Fatalf("duplicate billing event claim = %v, err = %v", claimed, err)
	}
	if err := s.ReleaseBillingEvent(ctx, eventID); err != nil {
		t.Fatalf("release billing event: %v", err)
	}
	claimed, err = s.ClaimBillingEvent(ctx, eventID, now)
	if err != nil || !claimed {
		t.Fatalf("billing event reclaim = %v, err = %v", claimed, err)
	}

	if err := s.DeleteWorkspaceLink(ctx, workspaceID, slug, core.AuditEntry{WorkspaceID: workspaceID, ActorUserID: userID, Action: "link.deleted", ResourceType: "link", ResourceID: slug, CreatedAt: now.Add(6 * time.Second)}); err != nil {
		t.Fatalf("delete link: %v", err)
	}
	if err := s.DeleteCustomDomain(ctx, workspaceID, domain.ID, core.AuditEntry{WorkspaceID: workspaceID, ActorUserID: userID, Action: "custom_domain.deleted", ResourceType: "custom_domain", ResourceID: domain.ID, CreatedAt: now.Add(7 * time.Second)}); err != nil {
		t.Fatalf("delete custom domain: %v", err)
	}
}
