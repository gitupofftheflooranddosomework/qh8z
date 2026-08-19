package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicAbuseReportPage(t *testing.T) {
	a, _ := testApp()
	req := httptest.NewRequest(http.MethodGet, "/report-abuse", nil)
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"Report an abusive link", "id=\"abuse-form\"", "/assets/abuse.js", "abuse@qh8z.com"} {
		if !strings.Contains(body, want) {
			t.Fatalf("page missing %q", want)
		}
	}
}

func TestPublicAbuseScriptUsesExistingAPI(t *testing.T) {
	a, _ := testApp()
	req := httptest.NewRequest(http.MethodGet, "/assets/abuse.js", nil)
	rec := httptest.NewRecorder()
	a.routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "/api/v1/abuse-reports") {
		t.Fatalf("abuse script does not use abuse API")
	}
}
