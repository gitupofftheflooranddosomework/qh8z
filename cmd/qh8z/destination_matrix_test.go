package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnsafeDestinationMatrixThroughCreateLink(t *testing.T) {
	a, fm := testApp()
	owner, cookie := registerAndVerify(t, a, fm, "destination-matrix@example.com")

	blocked := []string{
		"javascript:alert(1)",
		"ftp://example.net/file",
		"http://localhost/",
		"http://127.0.0.1/",
		"http://127.1/",
		"http://10.0.0.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://100.64.0.1/",
		"http://192.0.2.1/",
		"http://2130706433/",
		"http://0177.0.0.1/",
		"http://0x7f000001/",
		"http://[::1]/",
		"http://[fc00::1]/",
		"http://[fe80::1]/",
		"http://[2001:db8::1]/",
		"http://[::ffff:127.0.0.1]/",
		"http://router.local/",
		"http://service.internal/",
		"http://example.test/",
		"http://singlelabel/",
		"https://user:pass@public.example.net/",
		"https://www.cloudflare.com:70000/",
	}
	for i, target := range blocked {
		t.Run(fmt.Sprintf("blocked-%02d", i), func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{
				"url":        target,
				"customSlug": fmt.Sprintf("unsafe-%02d", i),
			})
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(string(payload)))
			req.AddCookie(cookie)
			req.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
			rec := httptest.NewRecorder()
			a.routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusBadRequest && rec.Code != http.StatusUnprocessableEntity {
				t.Fatalf("target %q status = %d, body = %s", target, rec.Code, rec.Body.String())
			}
		})
	}

	allowed := []string{
		"https://www.cloudflare.com/",
		"https://8.8.8.8/",
		"https://[2606:4700:4700::1111]/",
		"https://example.com:8443/path",
	}
	for i, target := range allowed {
		t.Run(fmt.Sprintf("allowed-%02d", i), func(t *testing.T) {
			payload, err := json.Marshal(map[string]string{
				"url":        target,
				"customSlug": fmt.Sprintf("public-%02d", i),
			})
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			req := httptest.NewRequest(http.MethodPost, "/api/v1/links", strings.NewReader(string(payload)))
			req.AddCookie(cookie)
			req.Header.Set("X-QH8Z-Workspace", owner.Workspace.ID)
			rec := httptest.NewRecorder()
			a.routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusCreated {
				t.Fatalf("target %q status = %d, body = %s", target, rec.Code, rec.Body.String())
			}
		})
	}
}
