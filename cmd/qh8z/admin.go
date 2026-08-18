package main

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

var abuseCategories = map[string]bool{
	"malware":  true,
	"phishing": true,
	"scam":     true,
	"spam":     true,
	"other":    true,
}

func (a *app) createAbuseReport(w http.ResponseWriter, r *http.Request) {
	if err := a.enforceIPLimit(r, "abuse-report", 20, time.Hour); err != nil {
		a.writeRateLimitOrStoreError(w, err)
		return
	}
	var req struct {
		Slug          string `json:"slug"`
		Category      string `json:"category"`
		Details       string `json:"details"`
		ReporterEmail string `json:"reporterEmail"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	slug := strings.TrimSpace(req.Slug)
	if !slugPattern.MatchString(slug) {
		writeError(w, http.StatusBadRequest, "invalid short-link slug")
		return
	}
	category := strings.ToLower(strings.TrimSpace(req.Category))
	if !abuseCategories[category] {
		writeError(w, http.StatusBadRequest, "category must be malware, phishing, scam, spam, or other")
		return
	}
	details := strings.TrimSpace(req.Details)
	if len(details) > 2000 {
		writeError(w, http.StatusBadRequest, "details must be at most 2000 bytes")
		return
	}
	reporterEmail := strings.TrimSpace(req.ReporterEmail)
	if reporterEmail != "" {
		var err error
		reporterEmail, err = normalizeEmail(reporterEmail)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
	}
	link, err := a.store.GetLink(r.Context(), slug)
	if errors.Is(err, core.ErrNotFound) {
		// Keep the response generic so this endpoint is not a link-existence oracle.
		writeJSON(w, http.StatusAccepted, map[string]string{"status": "accepted"})
		return
	}
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	id, err := randomID("abr_", 16)
	if err != nil {
		a.internalRandomError(w, err)
		return
	}
	report := core.AbuseReport{
		ID:             id,
		Slug:           slug,
		DestinationURL: link.URL,
		Category:       category,
		Details:        details,
		ReporterEmail:  reporterEmail,
		Status:         core.AbuseStatusOpen,
		CreatedAt:      time.Now().UTC(),
	}
	if err := a.store.CreateAbuseReport(r.Context(), report); err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"id": id, "status": core.AbuseStatusOpen})
}

func (a *app) listAbuseReports(w http.ResponseWriter, r *http.Request) {
	if !a.adminGuard(w, r) {
		return
	}
	status := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("status")))
	if status != "" && status != core.AbuseStatusOpen && status != core.AbuseStatusReviewed && status != core.AbuseStatusResolved {
		writeError(w, http.StatusBadRequest, "invalid abuse-report status")
		return
	}
	reports, err := a.store.ListAbuseReports(r.Context(), status, 100)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
}

func (a *app) reviewAbuseReport(w http.ResponseWriter, r *http.Request) {
	if !a.adminGuard(w, r) {
		return
	}
	var req struct {
		Status string `json:"status"`
		Notes  string `json:"notes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.Status))
	if status != core.AbuseStatusOpen && status != core.AbuseStatusReviewed && status != core.AbuseStatusResolved {
		writeError(w, http.StatusBadRequest, "status must be open, reviewed, or resolved")
		return
	}
	notes := strings.TrimSpace(req.Notes)
	if len(notes) > 2000 {
		writeError(w, http.StatusBadRequest, "notes must be at most 2000 bytes")
		return
	}
	report, err := a.store.UpdateAbuseReport(r.Context(), r.PathValue("id"), status, notes, time.Now().UTC())
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	_ = a.store.WriteAudit(r.Context(), core.AuditEntry{
		Action:       "admin.abuse_report_reviewed",
		ResourceType: "abuse_report",
		ResourceID:   report.ID,
		Metadata:     map[string]string{"status": report.Status},
		CreatedAt:    time.Now().UTC(),
	})
	writeJSON(w, http.StatusOK, report)
}

func (a *app) listURLRules(w http.ResponseWriter, r *http.Request) {
	if !a.adminGuard(w, r) {
		return
	}
	rules, err := a.store.ListURLRules(r.Context())
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"rules": rules})
}

func (a *app) createURLRule(w http.ResponseWriter, r *http.Request) {
	if !a.adminGuard(w, r) {
		return
	}
	var req struct {
		Action    string `json:"action"`
		MatchType string `json:"matchType"`
		Pattern   string `json:"pattern"`
		Reason    string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	action := strings.ToLower(strings.TrimSpace(req.Action))
	if action != core.URLRuleAllow && action != core.URLRuleBlock {
		writeError(w, http.StatusBadRequest, "action must be allow or block")
		return
	}
	matchType := strings.ToLower(strings.TrimSpace(req.MatchType))
	pattern, err := normalizeRulePattern(matchType, req.Pattern)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if len(reason) > 500 {
		writeError(w, http.StatusBadRequest, "reason must be at most 500 bytes")
		return
	}
	rule, err := a.store.CreateURLRule(r.Context(), core.URLRule{
		Action:    action,
		MatchType: matchType,
		Pattern:   pattern,
		Reason:    reason,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		if errors.Is(err, core.ErrConflict) {
			writeError(w, http.StatusConflict, "a rule already exists for that pattern")
			return
		}
		a.writeStoreError(w, err)
		return
	}
	_ = a.store.WriteAudit(r.Context(), core.AuditEntry{
		Action:       "admin.url_rule_created",
		ResourceType: "url_rule",
		ResourceID:   strconv.FormatInt(rule.ID, 10),
		Metadata:     map[string]string{"action": rule.Action, "match_type": rule.MatchType, "pattern": rule.Pattern},
		CreatedAt:    time.Now().UTC(),
	})
	writeJSON(w, http.StatusCreated, rule)
}

func (a *app) deleteURLRule(w http.ResponseWriter, r *http.Request) {
	if !a.adminGuard(w, r) {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		writeError(w, http.StatusBadRequest, "invalid rule ID")
		return
	}
	if err := a.store.DeleteURLRule(r.Context(), id); err != nil {
		a.writeStoreError(w, err)
		return
	}
	_ = a.store.WriteAudit(r.Context(), core.AuditEntry{
		Action:       "admin.url_rule_deleted",
		ResourceType: "url_rule",
		ResourceID:   strconv.FormatInt(id, 10),
		CreatedAt:    time.Now().UTC(),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) suspendLink(w http.ResponseWriter, r *http.Request) {
	if !a.adminGuard(w, r) {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	reason := strings.TrimSpace(req.Reason)
	if reason == "" || len(reason) > 500 {
		writeError(w, http.StatusBadRequest, "reason must be 1-500 bytes")
		return
	}
	now := time.Now().UTC()
	link, err := a.store.SetLinkSuspension(r.Context(), r.PathValue("slug"), true, reason, now, core.AuditEntry{
		Action:       "admin.link_suspended",
		ResourceType: "link",
		ResourceID:   r.PathValue("slug"),
		Metadata:     map[string]string{"reason": reason},
		CreatedAt:    now,
	})
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.response(link))
}

func (a *app) unsuspendLink(w http.ResponseWriter, r *http.Request) {
	if !a.adminGuard(w, r) {
		return
	}
	now := time.Now().UTC()
	link, err := a.store.SetLinkSuspension(r.Context(), r.PathValue("slug"), false, "", now, core.AuditEntry{
		Action:       "admin.link_unsuspended",
		ResourceType: "link",
		ResourceID:   r.PathValue("slug"),
		CreatedAt:    now,
	})
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, a.response(link))
}

func (a *app) adminGuard(w http.ResponseWriter, r *http.Request) bool {
	if err := a.enforceIPLimit(r, "admin", 60, time.Minute); err != nil {
		a.writeRateLimitOrStoreError(w, err)
		return false
	}
	return a.requireAdmin(w, r)
}
