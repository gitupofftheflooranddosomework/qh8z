package postgres

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"
)

func TestIsVerifiedCustomDomainRequiresProEntitlement(t *testing.T) {
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

	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	workspaceID := "ws_tls_" + suffix
	host := "tls-" + suffix + ".example.com"
	if _, err := s.db.ExecContext(ctx, `INSERT INTO workspaces (id, name) VALUES ($1, $2)`, workspaceID, "TLS entitlement test"); err != nil {
		t.Fatalf("insert workspace: %v", err)
	}
	defer func() {
		_, _ = s.db.ExecContext(context.Background(), `DELETE FROM workspaces WHERE id = $1`, workspaceID)
	}()
	if _, err := s.db.ExecContext(ctx, `
INSERT INTO custom_domains (id, workspace_id, host, verification_token, verified_at)
VALUES ($1, $2, $3, $4, NOW())`, "dom_tls_"+suffix, workspaceID, host, "token-"+suffix); err != nil {
		t.Fatalf("insert verified domain: %v", err)
	}

	allowed, err := s.IsVerifiedCustomDomain(ctx, host)
	if err != nil || allowed {
		t.Fatalf("verified free domain allowed = %v, err = %v", allowed, err)
	}

	if _, err := s.db.ExecContext(ctx, `
INSERT INTO workspace_billing (workspace_id, plan_code, status, updated_at)
VALUES ($1, 'pro', 'active', NOW())`, workspaceID); err != nil {
		t.Fatalf("activate pro billing: %v", err)
	}
	allowed, err = s.IsVerifiedCustomDomain(ctx, host)
	if err != nil || !allowed {
		t.Fatalf("active pro domain allowed = %v, err = %v", allowed, err)
	}

	if _, err := s.db.ExecContext(ctx, `UPDATE workspace_billing SET status = 'past_due', updated_at = NOW() WHERE workspace_id = $1`, workspaceID); err != nil {
		t.Fatalf("set past-due billing: %v", err)
	}
	allowed, err = s.IsVerifiedCustomDomain(ctx, host)
	if err != nil || !allowed {
		t.Fatalf("past-due pro domain allowed = %v, err = %v", allowed, err)
	}

	if _, err := s.db.ExecContext(ctx, `UPDATE workspace_billing SET status = 'canceled', updated_at = NOW() WHERE workspace_id = $1`, workspaceID); err != nil {
		t.Fatalf("cancel pro billing: %v", err)
	}
	allowed, err = s.IsVerifiedCustomDomain(ctx, host)
	if err != nil || allowed {
		t.Fatalf("canceled pro domain allowed = %v, err = %v", allowed, err)
	}
}
