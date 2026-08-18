package postgres

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

func testHash(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}

func TestPostgresStoreRoundTrip(t *testing.T) {
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
	userID := "usr_test_" + suffix
	workspaceID := "ws_test_" + suffix
	email := "user-" + suffix + "@example.com"
	sessionHash := testHash("session-" + suffix)
	verificationHash := testHash("verification-" + suffix)

	reg := core.Registration{
		User:         core.User{ID: userID, Email: email, PasswordHash: "test-hash", CreatedAt: now},
		Workspace:    core.Workspace{ID: workspaceID, Name: "Test Workspace", CreatedAt: now},
		Membership:   core.Membership{WorkspaceID: workspaceID, UserID: userID, Role: core.RoleOwner, CreatedAt: now},
		Verification: core.EmailVerification{TokenHash: verificationHash, UserID: userID, CreatedAt: now, ExpiresAt: now.Add(time.Hour)},
		Session:      core.Session{TokenHash: sessionHash, UserID: userID, CreatedAt: now, ExpiresAt: now.Add(time.Hour), LastSeenAt: now},
		Audit:        []core.AuditEntry{{WorkspaceID: workspaceID, ActorUserID: userID, Action: "workspace.created", ResourceType: "workspace", ResourceID: workspaceID, CreatedAt: now}},
	}
	if err := s.Register(ctx, reg); err != nil {
		t.Fatalf("register: %v", err)
	}
	if _, err := s.UserByEmail(ctx, email); err != nil {
		t.Fatalf("user by email: %v", err)
	}

	auth, err := s.ResolveSession(ctx, sessionHash, workspaceID, now.Add(time.Second))
	if err != nil {
		t.Fatalf("resolve session: %v", err)
	}
	if auth.UserID != userID || auth.WorkspaceID != workspaceID || auth.EmailVerified {
		t.Fatalf("unexpected pre-verification auth: %+v", auth)
	}
	verified, err := s.ConsumeEmailVerification(ctx, verificationHash, now.Add(2*time.Second))
	if err != nil {
		t.Fatalf("consume verification: %v", err)
	}
	if verified.EmailVerifiedAt == nil {
		t.Fatal("email was not marked verified")
	}
	auth, err = s.ResolveSession(ctx, sessionHash, workspaceID, now.Add(3*time.Second))
	if err != nil || !auth.EmailVerified {
		t.Fatalf("verified session = %+v, err = %v", auth, err)
	}

	slug := "test-" + suffix
	created := core.Link{Slug: slug, URL: "https://example.com/durable", WorkspaceID: workspaceID, CreatedByUserID: userID, CreatedAt: now}
	if err := s.CreateOwnedLink(ctx, created, core.AuditEntry{WorkspaceID: workspaceID, ActorUserID: userID, Action: "link.created", ResourceType: "link", ResourceID: slug, CreatedAt: now}); err != nil {
		t.Fatalf("create owned link: %v", err)
	}
	got, err := s.GetWorkspaceLink(ctx, workspaceID, slug)
	if err != nil {
		t.Fatalf("get workspace link: %v", err)
	}
	if got.URL != created.URL || got.WorkspaceID != workspaceID || got.Visits != 0 {
		t.Fatalf("unexpected link: %+v", got)
	}

	count, err := s.RecordVisit(ctx, core.Visit{Slug: slug, VisitedAt: now.Add(time.Second), Referer: "https://ref.example/", UserAgent: "qh8z-test"})
	if err != nil || count != 1 {
		t.Fatalf("record visit count = %d, err = %v", count, err)
	}
	stats, err := s.StatsForWorkspace(ctx, workspaceID, slug, 10)
	if err != nil || stats.TotalVisits != 1 || len(stats.Recent) != 1 {
		t.Fatalf("unexpected stats: %+v, err = %v", stats, err)
	}

	keyHash := testHash("api-key-" + suffix)
	apiKey := core.APIKey{ID: "key_test_" + suffix, WorkspaceID: workspaceID, Name: "test", KeyHash: keyHash, Scopes: []string{"analytics:read", "links:write"}, CreatedByUserID: userID, CreatedAt: now}
	if err := s.CreateAPIKey(ctx, apiKey, core.AuditEntry{WorkspaceID: workspaceID, ActorUserID: userID, Action: "api_key.created", ResourceType: "api_key", ResourceID: apiKey.ID, CreatedAt: now}); err != nil {
		t.Fatalf("create API key: %v", err)
	}
	apiAuth, err := s.ResolveAPIKey(ctx, keyHash, now.Add(time.Second))
	if err != nil || apiAuth.APIKeyID != apiKey.ID || len(apiAuth.Scopes) != 2 {
		t.Fatalf("resolve API key = %+v, err = %v", apiAuth, err)
	}

	workspace2 := core.Workspace{ID: "ws_second_" + suffix, Name: "Second Workspace", CreatedAt: now.Add(time.Second)}
	if err := s.CreateWorkspace(ctx, workspace2, userID, core.AuditEntry{WorkspaceID: workspace2.ID, ActorUserID: userID, Action: "workspace.created", ResourceType: "workspace", ResourceID: workspace2.ID, CreatedAt: now}); err != nil {
		t.Fatalf("create second workspace: %v", err)
	}
	workspaces, err := s.ListWorkspaces(ctx, userID)
	if err != nil || len(workspaces) != 2 {
		t.Fatalf("workspaces = %+v, err = %v", workspaces, err)
	}

	audit, err := s.ListAudit(ctx, workspaceID, 100)
	if err != nil || len(audit) < 3 {
		t.Fatalf("audit = %+v, err = %v", audit, err)
	}

	windowStart := now.Truncate(time.Minute)
	firstLimit, err := s.CheckRateLimit(ctx, "test:"+suffix, windowStart, windowStart.Add(time.Minute), 1)
	if err != nil || !firstLimit.Allowed {
		t.Fatalf("first rate limit = %+v, err = %v", firstLimit, err)
	}
	secondLimit, err := s.CheckRateLimit(ctx, "test:"+suffix, windowStart, windowStart.Add(time.Minute), 1)
	if err != nil || secondLimit.Allowed {
		t.Fatalf("second rate limit = %+v, err = %v", secondLimit, err)
	}

	rule, err := s.CreateURLRule(ctx, core.URLRule{Action: core.URLRuleBlock, MatchType: core.URLRuleDomain, Pattern: "blocked-" + suffix + ".example.net", Reason: "test", CreatedAt: now})
	if err != nil {
		t.Fatalf("create URL rule: %v", err)
	}
	matched, err := s.MatchURLRule(ctx, "sub."+rule.Pattern)
	if err != nil || matched.ID != rule.ID {
		t.Fatalf("matched rule = %+v, err = %v", matched, err)
	}

	reportID := "abr_test_" + suffix
	report := core.AbuseReport{ID: reportID, Slug: slug, DestinationURL: created.URL, Category: "phishing", Details: "test", Status: core.AbuseStatusOpen, CreatedAt: now}
	if err := s.CreateAbuseReport(ctx, report); err != nil {
		t.Fatalf("create abuse report: %v", err)
	}
	reports, err := s.ListAbuseReports(ctx, core.AbuseStatusOpen, 10)
	if err != nil || len(reports) == 0 {
		t.Fatalf("list abuse reports = %+v, err = %v", reports, err)
	}
	updatedReport, err := s.UpdateAbuseReport(ctx, reportID, core.AbuseStatusReviewed, "reviewed", now.Add(time.Minute))
	if err != nil || updatedReport.Status != core.AbuseStatusReviewed {
		t.Fatalf("review report = %+v, err = %v", updatedReport, err)
	}

	suspended, err := s.SetLinkSuspension(ctx, slug, true, "abuse review", now.Add(time.Minute), core.AuditEntry{Action: "admin.link_suspended", ResourceType: "link", ResourceID: slug, CreatedAt: now.Add(time.Minute)})
	if err != nil || suspended.SuspendedAt == nil {
		t.Fatalf("suspend link = %+v, err = %v", suspended, err)
	}
	readSuspended, err := s.GetLink(ctx, slug)
	if err != nil || readSuspended.SuspendedAt == nil {
		t.Fatalf("read suspended link = %+v, err = %v", readSuspended, err)
	}
	if _, err := s.SetLinkSuspension(ctx, slug, false, "", now.Add(2*time.Minute), core.AuditEntry{Action: "admin.link_unsuspended", ResourceType: "link", ResourceID: slug, CreatedAt: now.Add(2 * time.Minute)}); err != nil {
		t.Fatalf("unsuspend link: %v", err)
	}

	// A second store proves all migrations are idempotent and data survives new connections.
	s2, err := Open(ctx, dsn)
	if err != nil {
		t.Fatalf("reopen store: %v", err)
	}
	defer s2.Close()
	if _, err := s2.GetWorkspaceLink(ctx, workspaceID, slug); err != nil {
		t.Fatalf("link did not persist across connections: %v", err)
	}
}
