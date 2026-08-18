package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/storage"
)

func testApp() *app {
	return &app{
		store:   storage.NewMemory(),
		baseURL: "https://qh8z.test",
		logger:  slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
}

func TestCreateRedirectAndStats(t *testing.T) {
	a := testApp()
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(`{"url":"https://example.com/path","customSlug":"launch-test"}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	a.routes().ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}
	var created linkResponse
	if err := json.NewDecoder(createRec.Body).Decode(&created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.ShortURL != "https://qh8z.test/launch-test" {
		t.Fatalf("short URL = %q", created.ShortURL)
	}

	redirectReq := httptest.NewRequest(http.MethodGet, "/launch-test", nil)
	redirectReq.Header.Set("Referer", "https://ref.example/")
	redirectReq.Header.Set("User-Agent", "qh8z-test")
	redirectRec := httptest.NewRecorder()
	a.routes().ServeHTTP(redirectRec, redirectReq)
	if redirectRec.Code != http.StatusFound {
		t.Fatalf("redirect status = %d", redirectRec.Code)
	}
	if location := redirectRec.Header().Get("Location"); location != "https://example.com/path" {
		t.Fatalf("location = %q", location)
	}

	statsReq := httptest.NewRequest(http.MethodGet, "/api/v1/links/launch-test/stats", nil)
	statsRec := httptest.NewRecorder()
	a.routes().ServeHTTP(statsRec, statsReq)
	if statsRec.Code != http.StatusOK {
		t.Fatalf("stats status = %d, body = %s", statsRec.Code, statsRec.Body.String())
	}
	var stats struct {
		TotalVisits int64 `json:"totalVisits"`
	}
	if err := json.NewDecoder(statsRec.Body).Decode(&stats); err != nil {
		t.Fatalf("decode stats: %v", err)
	}
	if stats.TotalVisits != 1 {
		t.Fatalf("total visits = %d, want 1", stats.TotalVisits)
	}
}

func TestProductionRequiresPostgres(t *testing.T) {
	t.Setenv("QH8Z_ENV", "production")
	t.Setenv("QH8Z_STORAGE", "memory")
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	if _, err := openStore(context.Background(), logger); err == nil {
		t.Fatal("expected production memory storage to be rejected")
	}
}

func TestNormalizeURLRejectsUnsafeSchemesAndCredentials(t *testing.T) {
	for _, raw := range []string{"javascript:alert(1)", "ftp://example.com/file", "https://user:pass@example.com/"} {
		if _, err := normalizeURL(raw); err == nil {
			t.Fatalf("normalizeURL(%q) unexpectedly succeeded", raw)
		}
	}
}
