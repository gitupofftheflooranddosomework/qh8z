package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicSecurityPolicyPage(t *testing.T) {
	a, _ := testApp()
	req := httptest.NewRequest(http.MethodGet, "/security", nil)
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Security policy", "security@qh8z.com", "Reporting a vulnerability"} {
		if !strings.Contains(body, want) {
			t.Fatalf("security page missing %q", want)
		}
	}
}

func TestWellKnownSecurityText(t *testing.T) {
	a, _ := testApp()
	req := httptest.NewRequest(http.MethodGet, "/.well-known/security.txt", nil)
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("content type = %q", rec.Header().Get("Content-Type"))
	}
	body := rec.Body.String()
	for _, want := range []string{
		"Contact: mailto:security@qh8z.com",
		"Canonical: https://qh8z.com/.well-known/security.txt",
		"Policy: https://qh8z.com/security",
		"Preferred-Languages: en",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("security.txt missing %q", want)
		}
	}
}
