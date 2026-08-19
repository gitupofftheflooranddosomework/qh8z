package main

import "testing"

func TestPublicAndSystemRoutesAreReservedSlugs(t *testing.T) {
	for _, slug := range []string{
		"api", "assets", "internal", "healthz", "readyz", "metrics",
		"admin", "login", "signup", "verify-email", "dashboard", "pricing",
		"terms", "privacy", "acceptable-use", "report-abuse",
	} {
		if !reserved[slug] {
			t.Fatalf("route slug %q is not reserved", slug)
		}
	}
}
