package main

import (
	"net"
	"net/http"
	"strings"
)

func (a *app) tlsAllow(w http.ResponseWriter, r *http.Request) {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.URL.Query().Get("domain")), "."))
	if host == "" || len(host) > 253 || net.ParseIP(host) != nil || strings.ContainsAny(host, "/:@?#\x00\r\n\t ") {
		http.Error(w, "denied", http.StatusForbidden)
		return
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		http.Error(w, "denied", http.StatusForbidden)
		return
	}
	for _, label := range labels {
		if !domainLabelPattern.MatchString(label) {
			http.Error(w, "denied", http.StatusForbidden)
			return
		}
	}
	allowed, err := a.store.IsVerifiedCustomDomain(r.Context(), host)
	if err != nil {
		a.logger.Error("TLS domain authorization failed", "host", host, "error", err)
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
		return
	}
	if !allowed {
		http.Error(w, "denied", http.StatusForbidden)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
