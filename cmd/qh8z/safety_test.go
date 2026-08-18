package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestValidatePublicDestination(t *testing.T) {
	blocked := []string{
		"http://localhost/",
		"http://127.0.0.1/",
		"http://10.0.0.1/",
		"http://100.64.0.1/",
		"http://192.0.2.1/",
		"http://router.local/",
		"http://service.internal/",
		"http://singlelabel/",
		"http://2130706433/",
		"http://0177.0.0.1/",
		"https://example.com:70000/",
	}
	for _, raw := range blocked {
		u, err := url.Parse(raw)
		if err != nil {
			continue
		}
		if err := validatePublicDestination(u); err == nil {
			t.Fatalf("destination %q unexpectedly accepted", raw)
		}
	}
	for _, raw := range []string{"https://www.cloudflare.com/", "https://8.8.8.8/", "https://example.com:8443/path"} {
		u, err := url.Parse(raw)
		if err != nil {
			t.Fatalf("parse %q: %v", raw, err)
		}
		if err := validatePublicDestination(u); err != nil {
			t.Fatalf("destination %q rejected: %v", raw, err)
		}
	}
}

func TestClientIPUsesOnlyTrustedProxyHeaders(t *testing.T) {
	a, _ := testApp()
	a.safety.trustedProxies = []netip.Prefix{netip.MustParsePrefix("10.0.0.0/8")}

	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.RemoteAddr = "203.0.113.10:1234"
	r.Header.Set("X-Forwarded-For", "198.51.100.7")
	if got := a.clientIP(r); got.String() != "203.0.113.10" {
		t.Fatalf("untrusted peer client IP = %s", got)
	}

	r.RemoteAddr = "10.1.2.3:1234"
	r.Header.Set("X-Forwarded-For", "198.51.100.7, 10.2.3.4")
	if got := a.clientIP(r); got.String() != "198.51.100.7" {
		t.Fatalf("trusted proxy client IP = %s", got)
	}
}

func TestSafetyControlsAbuseRulesAndSuspension(t *testing.T) {
	a, fm := testApp()
	owner, cookie := registerAndVerify(t, a, fm, "safety-owner@example.com")

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(`{"url":"https://safe.example.net/path","customSlug":"safety-link"}`))
	createReq.AddCookie(cookie)
	createReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	createRec := httptest.NewRecorder()
	a.routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	reportReq := httptest.NewRequest(http.MethodPost, "/api/v1/abuse-reports", strings.NewReader(`{"slug":"safety-link","category":"phishing","details":"looks suspicious"}`))
	reportRec := httptest.NewRecorder()
	a.routes().ServeHTTP(reportRec, reportReq)
	if reportRec.Code != http.StatusAccepted {
		t.Fatalf("abuse report status = %d, body = %s", reportRec.Code, reportRec.Body.String())
	}

	adminToken := a.safety.adminToken
	suspendReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/links/safety-link/suspend", strings.NewReader(`{"reason":"abuse review"}`))
	suspendReq.Header.Set("Authorization", "Bearer "+adminToken)
	suspendRec := httptest.NewRecorder()
	a.routes().ServeHTTP(suspendRec, suspendReq)
	if suspendRec.Code != http.StatusOK {
		t.Fatalf("suspend status = %d, body = %s", suspendRec.Code, suspendRec.Body.String())
	}

	redirectReq := httptest.NewRequest(http.MethodGet, "/safety-link", nil)
	redirectRec := httptest.NewRecorder()
	a.routes().ServeHTTP(redirectRec, redirectReq)
	if redirectRec.Code != http.StatusNotFound {
		t.Fatalf("suspended redirect status = %d", redirectRec.Code)
	}

	ruleReq := httptest.NewRequest(http.MethodPost, "/api/v1/admin/url-rules", strings.NewReader(`{"action":"block","matchType":"domain","pattern":"blocked.example.net","reason":"known abuse"}`))
	ruleReq.Header.Set("Authorization", "Bearer "+adminToken)
	ruleRec := httptest.NewRecorder()
	a.routes().ServeHTTP(ruleRec, ruleReq)
	if ruleRec.Code != http.StatusCreated {
		t.Fatalf("rule create status = %d, body = %s", ruleRec.Code, ruleRec.Body.String())
	}

	blockedReq := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(`{"url":"https://sub.blocked.example.net/","customSlug":"blocked-link"}`))
	blockedReq.AddCookie(cookie)
	blockedReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	blockedRec := httptest.NewRecorder()
	a.routes().ServeHTTP(blockedRec, blockedReq)
	if blockedRec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("blocked destination status = %d, body = %s", blockedRec.Code, blockedRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/admin/abuse-reports?status=open", nil)
	listReq.Header.Set("Authorization", "Bearer "+adminToken)
	listRec := httptest.NewRecorder()
	a.routes().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), "safety-link") {
		t.Fatalf("abuse list status = %d, body = %s", listRec.Code, listRec.Body.String())
	}
}

func TestProductionSafetyRequiresConfiguredControls(t *testing.T) {
	t.Setenv("QH8Z_ADMIN_TOKEN", strings.Repeat("a", 32))
	t.Setenv("QH8Z_RATE_LIMIT_SALT", strings.Repeat("b", 32))
	t.Setenv("QH8Z_REPUTATION_MODE", "disabled")
	if _, _, err := openSafety("production"); err == nil {
		t.Fatal("production accepted disabled reputation checks")
	}
	t.Setenv("QH8Z_REPUTATION_MODE", "webrisk")
	t.Setenv("WEBRISK_API_KEY", "test-api-key")
	checker, cfg, err := openSafety("production")
	if err != nil || checker == nil || len(cfg.adminToken) < 32 {
		t.Fatalf("production safety config = %+v, err = %v", cfg, err)
	}
}

func TestMemoryRateLimitBoundary(t *testing.T) {
	a, _ := testApp()
	now := time.Now().UTC()
	windowStart := now.Truncate(time.Minute)
	for i := 0; i < 2; i++ {
		result, err := a.store.CheckRateLimit(context.Background(), "test", windowStart, windowStart.Add(time.Minute), 1)
		if err != nil {
			t.Fatalf("rate limit: %v", err)
		}
		if i == 0 && !result.Allowed {
			t.Fatal("first request unexpectedly blocked")
		}
		if i == 1 && result.Allowed {
			t.Fatal("second request unexpectedly allowed")
		}
	}
}
