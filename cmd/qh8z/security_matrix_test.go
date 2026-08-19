package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func createTestAPIKey(t *testing.T, a *app, cookie *http.Cookie, workspaceID, name string, scopes []string) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"name": name, "scopes": scopes})
	if err != nil {
		t.Fatalf("marshal API key request: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/"+workspaceID+"/api-keys", strings.NewReader(string(payload)))
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create API key %q status = %d, body = %s", name, rec.Code, rec.Body.String())
	}
	var result struct {
		Secret string `json:"secret"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&result); err != nil || !strings.HasPrefix(result.Secret, "qh8z_sk_") {
		t.Fatalf("decode API key %q: secret=%q err=%v", name, result.Secret, err)
	}
	return result.Secret
}

func serveWithSession(a *app, method, target, body string, cookie *http.Cookie, workspaceID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.AddCookie(cookie)
	if workspaceID != "" {
		req.Header.Set("X-QH8Z-Workspace", workspaceID)
	}
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	return rec
}

func serveWithAPIKey(a *app, method, target, body, secret, workspaceID string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+secret)
	if workspaceID != "" {
		req.Header.Set("X-QH8Z-Workspace", workspaceID)
	}
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	return rec
}

func TestAuthorizationSecurityMatrix(t *testing.T) {
	a, fm := testApp()
	ownerA, cookieA := registerAndVerify(t, a, fm, "security-owner-a@example.com")
	ownerB, cookieB := registerAndVerify(t, a, fm, "security-owner-b@example.com")

	created := serveWithSession(a, http.MethodPost, "/api/v1/links", `{"url":"https://example.com/private-a","customSlug":"private-a"}`, cookieA, ownerA.Workspace.ID)
	if created.Code != http.StatusCreated {
		t.Fatalf("create owner A link status = %d, body = %s", created.Code, created.Body.String())
	}

	crossWorkspace := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{name: "links", method: http.MethodGet, target: "/api/v1/links"},
		{name: "domains", method: http.MethodGet, target: "/api/v1/domains"},
		{name: "analytics", method: http.MethodGet, target: "/api/v1/analytics"},
		{name: "billing", method: http.MethodGet, target: "/api/v1/billing"},
	}
	for _, test := range crossWorkspace {
		t.Run("cross-workspace-"+test.name, func(t *testing.T) {
			rec := serveWithSession(a, test.method, test.target, test.body, cookieB, ownerA.Workspace.ID)
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}

	for _, test := range []struct {
		name   string
		method string
		target string
		body   string
	}{
		{name: "members", method: http.MethodGet, target: "/api/v1/workspaces/" + ownerA.Workspace.ID + "/members"},
		{name: "api-keys", method: http.MethodPost, target: "/api/v1/workspaces/" + ownerA.Workspace.ID + "/api-keys", body: `{"name":"forbidden","scopes":["links:read"]}`},
		{name: "audit", method: http.MethodGet, target: "/api/v1/workspaces/" + ownerA.Workspace.ID + "/audit"},
	} {
		t.Run("cross-workspace-"+test.name, func(t *testing.T) {
			rec := serveWithSession(a, test.method, test.target, test.body, cookieB, "")
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}

	objectRead := serveWithSession(a, http.MethodGet, "/api/v1/links/private-a", "", cookieB, ownerB.Workspace.ID)
	if objectRead.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace object read status = %d, body = %s", objectRead.Code, objectRead.Body.String())
	}
	objectWrite := serveWithSession(a, http.MethodPatch, "/api/v1/links/private-a", `{"disabled":true}`, cookieB, ownerB.Workspace.ID)
	if objectWrite.Code != http.StatusNotFound {
		t.Fatalf("cross-workspace object write status = %d, body = %s", objectWrite.Code, objectWrite.Body.String())
	}

	readOnly := createTestAPIKey(t, a, cookieA, ownerA.Workspace.ID, "read-only", []string{"links:read"})
	writeOnly := createTestAPIKey(t, a, cookieA, ownerA.Workspace.ID, "write-only", []string{"links:write"})

	for _, test := range []struct {
		name   string
		method string
		target string
		body   string
		secret string
	}{
		{name: "missing-links-write", method: http.MethodPost, target: "/api/v1/links", body: `{"url":"https://example.com/denied","customSlug":"scope-denied"}`, secret: readOnly},
		{name: "missing-links-read", method: http.MethodGet, target: "/api/v1/links", secret: writeOnly},
		{name: "missing-analytics-read", method: http.MethodGet, target: "/api/v1/analytics", secret: readOnly},
		{name: "missing-workspace-admin", method: http.MethodGet, target: "/api/v1/workspaces/" + ownerA.Workspace.ID + "/audit", secret: readOnly},
	} {
		t.Run(test.name, func(t *testing.T) {
			rec := serveWithAPIKey(a, test.method, test.target, test.body, test.secret, "")
			if rec.Code != http.StatusForbidden {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
			}
		})
	}

	workspaceMismatch := serveWithAPIKey(a, http.MethodGet, "/api/v1/links", "", readOnly, ownerB.Workspace.ID)
	if workspaceMismatch.Code != http.StatusForbidden {
		t.Fatalf("API-key workspace mismatch status = %d, body = %s", workspaceMismatch.Code, workspaceMismatch.Body.String())
	}

	logout := serveWithSession(a, http.MethodPost, "/api/v1/auth/logout", "", cookieA, "")
	if logout.Code != http.StatusNoContent {
		t.Fatalf("logout status = %d, body = %s", logout.Code, logout.Body.String())
	}
	me := serveWithSession(a, http.MethodGet, "/api/v1/me", "", cookieA, "")
	if me.Code != http.StatusUnauthorized {
		t.Fatalf("old session after logout status = %d, body = %s", me.Code, me.Body.String())
	}
}
