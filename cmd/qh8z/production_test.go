package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

func TestSecretValueAndDatabaseURLFromFiles(t *testing.T) {
	dir := t.TempDir()
	secretPath := filepath.Join(dir, "database-password")
	if err := os.WriteFile(secretPath, []byte("p@ss:word\n"), 0o600); err != nil {
		t.Fatalf("write secret: %v", err)
	}
	t.Setenv("DATABASE_URL", "")
	t.Setenv("DATABASE_URL_FILE", "")
	t.Setenv("DATABASE_PASSWORD", "")
	t.Setenv("DATABASE_PASSWORD_FILE", secretPath)
	t.Setenv("DATABASE_HOST", "db.internal")
	t.Setenv("DATABASE_PORT", "5433")
	t.Setenv("DATABASE_USER", "qh8z_user")
	t.Setenv("DATABASE_NAME", "qh8z_prod")
	t.Setenv("DATABASE_SSLMODE", "require")

	dsn, err := databaseURL()
	if err != nil {
		t.Fatalf("databaseURL: %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse DSN: %v", err)
	}
	password, ok := parsed.User.Password()
	if !ok || password != "p@ss:word" || parsed.Host != "db.internal:5433" || parsed.Path != "/qh8z_prod" || parsed.Query().Get("sslmode") != "require" {
		t.Fatalf("unexpected DSN components: %s", dsn)
	}
}

func TestTLSAuthorizationRequiresVerifiedPaidCustomDomain(t *testing.T) {
	a, fm := testApp()
	owner, _ := registerAndVerify(t, a, fm, "tls-owner@example.com")
	now := time.Now().UTC()
	domain := core.CustomDomain{
		ID:                "dom_tls_test",
		WorkspaceID:       owner.Workspace.ID,
		Host:              "go.customer.example.org",
		VerificationToken: "verification-token",
		CreatedAt:         now,
	}
	if err := a.store.CreateCustomDomain(context.Background(), domain, core.AuditEntry{}); err != nil {
		t.Fatalf("create custom domain: %v", err)
	}

	request := func() int {
		req := httptest.NewRequest(http.MethodGet, "/internal/tls/allow?domain=go.customer.example.org", nil)
		rec := httptest.NewRecorder()
		a.routes().ServeHTTP(rec, req)
		return rec.Code
	}

	if got := request(); got != http.StatusForbidden {
		t.Fatalf("unverified TLS authorization = %d", got)
	}

	if _, err := a.store.SetCustomDomainVerified(context.Background(), owner.Workspace.ID, domain.ID, now, core.AuditEntry{}); err != nil {
		t.Fatalf("verify custom domain: %v", err)
	}
	if got := request(); got != http.StatusForbidden {
		t.Fatalf("verified free-plan TLS authorization = %d", got)
	}

	billing := core.BillingState{
		WorkspaceID:            owner.Workspace.ID,
		PlanCode:               core.PlanPro,
		Status:                 core.BillingStatusActive,
		ProviderCustomerID:     "cus_tls_test",
		ProviderSubscriptionID: "sub_tls_test",
		UpdatedAt:              now,
	}
	if err := a.store.UpsertBillingState(context.Background(), billing, core.AuditEntry{}); err != nil {
		t.Fatalf("activate pro billing: %v", err)
	}
	if got := request(); got != http.StatusNoContent {
		t.Fatalf("active pro TLS authorization = %d", got)
	}

	billing.Status = core.BillingStatusPastDue
	billing.UpdatedAt = now.Add(time.Second)
	if err := a.store.UpsertBillingState(context.Background(), billing, core.AuditEntry{}); err != nil {
		t.Fatalf("set past-due billing: %v", err)
	}
	if got := request(); got != http.StatusNoContent {
		t.Fatalf("past-due pro TLS authorization = %d", got)
	}

	billing.Status = core.BillingStatusCanceled
	billing.UpdatedAt = now.Add(2 * time.Second)
	if err := a.store.UpsertBillingState(context.Background(), billing, core.AuditEntry{}); err != nil {
		t.Fatalf("cancel pro billing: %v", err)
	}
	if got := request(); got != http.StatusForbidden {
		t.Fatalf("canceled pro TLS authorization = %d", got)
	}

	invalid := httptest.NewRequest(http.MethodGet, "/internal/tls/allow?domain=127.0.0.1", nil)
	invalidRec := httptest.NewRecorder()
	a.routes().ServeHTTP(invalidRec, invalid)
	if invalidRec.Code != http.StatusForbidden {
		t.Fatalf("IP TLS authorization = %d", invalidRec.Code)
	}
}

func TestMetricsExposeServiceAndStorageHealth(t *testing.T) {
	a, _ := testApp()
	healthReq := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	healthRec := httptest.NewRecorder()
	a.routes().ServeHTTP(healthRec, healthReq)
	if healthRec.Code != http.StatusOK {
		t.Fatalf("health status = %d", healthRec.Code)
	}
	metricsReq := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	metricsRec := httptest.NewRecorder()
	a.routes().ServeHTTP(metricsRec, metricsReq)
	if metricsRec.Code != http.StatusOK {
		t.Fatalf("metrics status = %d", metricsRec.Code)
	}
	body := metricsRec.Body.String()
	for _, metric := range []string{"qh8z_http_requests_total", "qh8z_http_5xx_total", "qh8z_http_rate_limited_total", "qh8z_storage_up 1"} {
		if !strings.Contains(body, metric) {
			t.Fatalf("metrics missing %q:\n%s", metric, body)
		}
	}
}
