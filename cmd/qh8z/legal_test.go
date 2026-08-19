package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicLegalAndPricingPages(t *testing.T) {
	a, _ := testApp()
	for _, test := range []struct {
		path string
		want string
	}{
		{path: "/pricing", want: "Pro — C$12.00 per month"},
		{path: "/terms", want: "qh8z Terms of Service"},
		{path: "/privacy", want: "qh8z Privacy Policy"},
		{path: "/acceptable-use", want: "qh8z Acceptable Use Policy"},
	} {
		t.Run(test.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, test.path, nil)
			rec := httptest.NewRecorder()
			a.routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d", rec.Code)
			}
			if !strings.Contains(rec.Header().Get("Content-Type"), "text/html") {
				t.Fatalf("content type = %q", rec.Header().Get("Content-Type"))
			}
			if !strings.Contains(rec.Body.String(), test.want) {
				t.Fatalf("page missing %q", test.want)
			}
		})
	}
}

func TestPolicyRendererEscapesHTML(t *testing.T) {
	got := policyInline(`<script>alert("x")</script> **safe** ` + "`code`")
	if strings.Contains(got, "<script>") {
		t.Fatalf("renderer emitted raw script: %s", got)
	}
	if !strings.Contains(got, "&lt;script&gt;") || !strings.Contains(got, "<strong>safe</strong>") || !strings.Contains(got, "<code>code</code>") {
		t.Fatalf("unexpected rendered inline content: %s", got)
	}
}
