package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/mailer"
	"github.com/gitupofftheflooranddosomework/qh8z/internal/storage"
)

type fakeMailer struct {
	mu   sync.Mutex
	to   string
	link string
}

func (m *fakeMailer) SendVerification(_ context.Context, to, link string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.to = to
	m.link = link
	return nil
}

func (m *fakeMailer) verificationToken(t *testing.T) string {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	u, err := url.Parse(m.link)
	if err != nil {
		t.Fatalf("parse verification URL: %v", err)
	}
	return u.Query().Get("token")
}

var _ mailer.Mailer = (*fakeMailer)(nil)

func testApp() (*app, *fakeMailer) {
	fm := &fakeMailer{}
	return &app{
		store:         storage.NewMemory(),
		mailer:        fm,
		baseURL:       "https://qh8z.test",
		logger:        slog.New(slog.NewTextHandler(io.Discard, nil)),
		secureCookies: true,
	}, fm
}

type registrationResult struct {
	User struct {
		ID    string `json:"id"`
		Email string `json:"email"`
	} `json:"user"`
	Workspace struct {
		ID string `json:"id"`
	} `json:"workspace"`
}

func registerAndVerify(t *testing.T, a *app, fm *fakeMailer, email string) (registrationResult, *http.Cookie) {
	t.Helper()
	body := `{"email":"` + email + `","password":"correct horse battery staple","workspaceName":"Launch Team"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(body))
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var result registrationResult
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
		t.Fatalf("decode registration: %v", err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) == 0 || cookies[0].Name != sessionCookieName || !cookies[0].HttpOnly || !cookies[0].Secure {
		t.Fatalf("unexpected session cookie: %+v", cookies)
	}
	token := fm.verificationToken(t)
	if !strings.HasPrefix(token, "qh8z_ev_") {
		t.Fatalf("verification token = %q", token)
	}
	verifyReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/verify-email", strings.NewReader(`{"token":"`+token+`"}`))
	verifyRec := httptest.NewRecorder()
	a.routes().ServeHTTP(verifyRec, verifyReq)
	if verifyRec.Code != http.StatusOK {
		t.Fatalf("verify status = %d, body = %s", verifyRec.Code, verifyRec.Body.String())
	}
	return result, cookies[0]
}

func TestRegistrationVerificationOwnedLinksAndAPIKeys(t *testing.T) {
	a, fm := testApp()
	owner, cookie := registerAndVerify(t, a, fm, "owner@example.com")

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(`{"url":"https://example.com/path","customSlug":"launch-test"}`))
	createReq.AddCookie(cookie)
	createReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	createRec := httptest.NewRecorder()
	a.routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	redirectReq := httptest.NewRequest(http.MethodGet, "/launch-test", nil)
	redirectReq.Header.Set("Referer", "https://ref.example/")
	redirectReq.Header.Set("User-Agent", "qh8z-test")
	redirectRec := httptest.NewRecorder()
	a.routes().ServeHTTP(redirectRec, redirectReq)
	if redirectRec.Code != http.StatusFound || redirectRec.Header().Get("Location") != "https://example.com/path" {
		t.Fatalf("redirect = %d %q", redirectRec.Code, redirectRec.Header().Get("Location"))
	}

	statsReq := httptest.NewRequest(http.MethodGet, "/api/v1/links/launch-test/stats", nil)
	statsReq.AddCookie(cookie)
	statsReq.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
	statsRec := httptest.NewRecorder()
	a.routes().ServeHTTP(statsRec, statsReq)
	if statsRec.Code != http.StatusOK {
		t.Fatalf("stats status = %d, body = %s", statsRec.Code, statsRec.Body.String())
	}
	var stats struct {
		TotalVisits int64 `json:"totalVisits"`
	}
	if err := json.NewDecoder(statsRec.Body).Decode(&stats); err != nil || stats.TotalVisits != 1 {
		t.Fatalf("stats = %+v, err = %v", stats, err)
	}

	keyReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+owner.Workspace.ID+"/api-keys", strings.NewReader(`{"name":"launch-ci","scopes":["links:write","links:read","analytics:read"]}`))
	keyReq.AddCookie(cookie)
	keyRec := httptest.NewRecorder()
	a.routes().ServeHTTP(keyRec, keyReq)
	if keyRec.Code != http.StatusCreated {
		t.Fatalf("create API key status = %d, body = %s", keyRec.Code, keyRec.Body.String())
	}
	var keyResult struct {
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(keyRec.Body).Decode(&keyResult); err != nil || !strings.HasPrefix(keyResult.Secret, "qh8z_sk_") {
		t.Fatalf("API key result = %+v, err = %v", keyResult, err)
	}

	apiCreateReq := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(`{"url":"https://example.com/api","customSlug":"api-owned"}`))
	apiCreateReq.Header.Set("Authorization", "Bearer "+keyResult.Secret)
	apiCreateRec := httptest.NewRecorder()
	a.routes().ServeHTTP(apiCreateRec, apiCreateReq)
	if apiCreateRec.Code != http.StatusCreated {
		t.Fatalf("API-key link create status = %d, body = %s", apiCreateRec.Code, apiCreateRec.Body.String())
	}

	auditReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+owner.Workspace.ID+"/audit", nil)
	auditReq.AddCookie(cookie)
	auditRec := httptest.NewRecorder()
	a.routes().ServeHTTP(auditRec, auditReq)
	if auditRec.Code != http.StatusOK || !strings.Contains(auditRec.Body.String(), "link.created") || !strings.Contains(auditRec.Body.String(), "api_key.created") {
		t.Fatalf("audit status = %d, body = %s", auditRec.Code, auditRec.Body.String())
	}
}

func TestWorkspaceMembershipAndLogin(t *testing.T) {
	a, fm := testApp()
	owner, ownerCookie := registerAndVerify(t, a, fm, "owner2@example.com")
	_, _ = registerAndVerify(t, a, fm, "member@example.com")

	memberReq := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+owner.Workspace.ID+"/members", strings.NewReader(`{"email":"member@example.com","role":"member"}`))
	memberReq.AddCookie(ownerCookie)
	memberRec := httptest.NewRecorder()
	a.routes().ServeHTTP(memberRec, memberReq)
	if memberRec.Code != http.StatusCreated {
		t.Fatalf("add member status = %d, body = %s", memberRec.Code, memberRec.Body.String())
	}

	loginReq := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(`{"email":"member@example.com","password":"correct horse battery staple"}`))
	loginRec := httptest.NewRecorder()
	a.routes().ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK || len(loginRec.Result().Cookies()) == 0 {
		t.Fatalf("login status = %d, body = %s", loginRec.Code, loginRec.Body.String())
	}
	memberCookie := loginRec.Result().Cookies()[0]

	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/workspaces/"+owner.Workspace.ID+"/members", nil)
	listReq.AddCookie(memberCookie)
	listRec := httptest.NewRecorder()
	a.routes().ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK || !strings.Contains(listRec.Body.String(), "member@example.com") {
		t.Fatalf("member list status = %d, body = %s", listRec.Code, listRec.Body.String())
	}
}

func TestUnverifiedEmailCannotCreateLinks(t *testing.T) {
	a, _ := testApp()
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/register", strings.NewReader(`{"email":"pending@example.com","password":"correct horse battery staple"}`))
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("register status = %d", rec.Code)
	}
	cookie := rec.Result().Cookies()[0]
	linkReq := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(`{"url":"https://example.com"}`))
	linkReq.AddCookie(cookie)
	linkRec := httptest.NewRecorder()
	a.routes().ServeHTTP(linkRec, linkReq)
	if linkRec.Code != http.StatusForbidden || !strings.Contains(linkRec.Body.String(), "verified email") {
		t.Fatalf("unverified create = %d, body = %s", linkRec.Code, linkRec.Body.String())
	}
}

func TestProductionRequiresPostgresAndSMTP(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	t.Setenv("QH8Z_STORAGE", "memory")
	if _, err := openStore(context.Background(), logger, "production"); err == nil {
		t.Fatal("expected production memory storage to be rejected")
	}
	t.Setenv("QH8Z_EMAIL_MODE", "log")
	if _, err := openMailer(logger, "production"); err == nil {
		t.Fatal("expected production log email to be rejected")
	}
}

func TestNormalizeURLRejectsUnsafeSchemesAndCredentials(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "ftp://example.com/file", "https://user:pass@example.com/"} {
		if _, err := normalizeURL(raw); err == nil {
			t.Fatalf("normalizeURL(%q) unexpectedly succeeded", raw)
		}
	}
}
