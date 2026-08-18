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

func TestTLSAuthorizationOnlyAllowsVerifiedCustomDomains(t *testing.T) {
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

	unverified := httptest.NewRequest(http.MethodGet, "/internal/tls/allow?domain=go.customer.example.org", nil)
	unverifiedRec := httptest.NewRecorder()
	a.routes().ServeHTTP(unverifiedRec, unverified)
	if unverifiedRec.Code != http.StatusForbidden {
		t.Fatalf("unverified TLS authorization = %d", unverifiedRec.Code)
	}

	if _, err := a.store.SetCustomDomainVerified(context.Background(), owner.Workspace.ID, domain.ID, now, core.AuditEntry{}); err != nil {
		t.Fatalf("verify custom domain: %v", err)
	}
	verified := httptest.NewRequest(http.MethodGet, "/internal/tls/allow?domain=go.customer.example.org", nil)
	verifiedRec := httptest.NewRecorder()
	a.routes().ServeHTTP(verifiedRec, verified)
	if verifiedRec.Code != http.StatusNoContent {
		t.Fatalf("verified TLS authorization = %d, body = %s", verifiedRec.Code, verifiedRec.Body.String())
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
