package main

import (
	"net/http"

	project "github.com/gitupofftheflooranddosomework/qh8z"
)

func (a *app) securityPage(w http.ResponseWriter, r *http.Request) {
	a.policyPage(w, r, "Security policy", project.SecurityPolicy)
}

func (a *app) securityText(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "public, max-age=3600")
	_, _ = w.Write([]byte("Contact: mailto:security@qh8z.com\nCanonical: https://qh8z.com/.well-known/security.txt\nPolicy: https://qh8z.com/security\nExpires: 2027-08-19T00:00:00Z\nPreferred-Languages: en\n"))
}
