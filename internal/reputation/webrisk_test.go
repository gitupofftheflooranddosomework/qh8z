package reputation

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestWebRiskDetectsThreatAndCaches(t *testing.T) {
	var calls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&calls, 1)
		if r.URL.Query().Get("key") != "test-key" || r.URL.Query().Get("uri") != "https://bad.example/" {
			t.Fatalf("unexpected query: %s", r.URL.RawQuery)
		}
		if got := r.URL.Query()["threatTypes"]; len(got) != 4 {
			t.Fatalf("threatTypes = %v", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"threat":{"threatTypes":["MALWARE"],"expireTime":"` + time.Now().Add(time.Hour).UTC().Format(time.RFC3339) + `"}}`))
	}))
	defer server.Close()

	checker, err := NewWebRisk("test-key", true)
	if err != nil {
		t.Fatalf("new checker: %v", err)
	}
	checker.Endpoint = server.URL
	for range 2 {
		result, err := checker.Check(context.Background(), "https://bad.example/")
		if err != nil {
			t.Fatalf("check: %v", err)
		}
		if !result.Unsafe || !strings.Contains(strings.Join(result.ThreatTypes, ","), "MALWARE") {
			t.Fatalf("unexpected result: %+v", result)
		}
	}
	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("calls = %d, want 1 due to cache", got)
	}
}

func TestWebRiskSafeEmptyResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	checker, _ := NewWebRisk("test-key", false)
	checker.Endpoint = server.URL
	result, err := checker.Check(context.Background(), "https://safe.example/")
	if err != nil || result.Unsafe {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
}
