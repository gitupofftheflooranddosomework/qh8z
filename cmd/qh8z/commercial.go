package main

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
	qrcode "github.com/skip2/go-qrcode"
)

type planLimits struct {
	Code              string `json:"code"`
	LinkLimit         int64  `json:"linkLimit"`
	CustomDomainLimit int64  `json:"customDomainLimit"`
	AnalyticsDays     int    `json:"analyticsDays"`
	QREnabled         bool   `json:"qrEnabled"`
}

var plans = map[string]planLimits{
	core.PlanFree: {
		Code:              core.PlanFree,
		LinkLimit:         100,
		CustomDomainLimit: 0,
		AnalyticsDays:     7,
		QREnabled:         true,
	},
	core.PlanPro: {
		Code:              core.PlanPro,
		LinkLimit:         10000,
		CustomDomainLimit: 10,
		AnalyticsDays:     90,
		QREnabled:         true,
	},
}

var domainLabelPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

func (a *app) listLinks(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "links:read", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	limit := queryInt(r, "limit", 50, 1, 200)
	offset := queryInt(r, "offset", 0, 0, 1000000)
	links, err := a.store.ListWorkspaceLinks(r.Context(), auth.WorkspaceID, limit, offset)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	items := make([]linkResponse, 0, len(links))
	for _, link := range links {
		items = append(items, a.response(link))
	}
	writeJSON(w, http.StatusOK, map[string]any{"links": items, "limit": limit, "offset": offset})
}

func (a *app) updateLink(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "links:write", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	current, err := a.store.GetWorkspaceLink(r.Context(), auth.WorkspaceID, r.PathValue("slug"))
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	var req struct {
		URL      *string `json:"url"`
		DomainID *string `json:"domainId"`
		Disabled *bool   `json:"disabled"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.URL == nil && req.DomainID == nil && req.Disabled == nil {
		writeError(w, http.StatusBadRequest, "at least one of url, domainId, or disabled is required")
		return
	}
	now := time.Now().UTC()
	updated := current
	needsContentUpdate := false
	if req.URL != nil {
		target, err := normalizeURL(*req.URL)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := a.checkDestination(r.Context(), target, auth); err != nil {
			var rejected *destinationRejectedError
			if errors.As(err, &rejected) {
				writeError(w, http.StatusUnprocessableEntity, rejected.Error())
				return
			}
			a.logger.Error("destination safety check failed", "error", err)
			writeError(w, http.StatusServiceUnavailable, "destination safety check unavailable")
			return
		}
		updated.URL = target
		needsContentUpdate = true
	}
	if req.DomainID != nil {
		domainID := strings.TrimSpace(*req.DomainID)
		if domainID != "" {
			domain, err := a.domainForLink(r, auth, domainID)
			if err != nil {
				a.writeCommercialError(w, err)
				return
			}
			updated.DomainID = domain.ID
			updated.DomainHost = domain.Host
		} else {
			updated.DomainID = ""
			updated.DomainHost = ""
		}
		needsContentUpdate = true
	}
	if needsContentUpdate {
		audit := actorAudit(auth, "link.updated", "link", updated.Slug, now, map[string]string{"destination": updated.URL, "domain_id": updated.DomainID})
		updated, err = a.store.UpdateWorkspaceLink(r.Context(), auth.WorkspaceID, updated.Slug, updated.URL, updated.DomainID, now, audit)
		if err != nil {
			a.writeStoreError(w, err)
			return
		}
	}
	if req.Disabled != nil {
		action := "link.enabled"
		if *req.Disabled {
			action = "link.disabled"
		}
		audit := actorAudit(auth, action, "link", updated.Slug, now, nil)
		updated, err = a.store.SetWorkspaceLinkDisabled(r.Context(), auth.WorkspaceID, updated.Slug, *req.Disabled, now, audit)
		if err != nil {
			a.writeStoreError(w, err)
			return
		}
	}
	writeJSON(w, http.StatusOK, a.response(updated))
}

func (a *app) deleteLink(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "links:write", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	slug := r.PathValue("slug")
	now := time.Now().UTC()
	audit := actorAudit(auth, "link.deleted", "link", slug, now, nil)
	if err := a.store.DeleteWorkspaceLink(r.Context(), auth.WorkspaceID, slug, audit); err != nil {
		a.writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) linkQRCode(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "links:read", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	item, err := a.store.GetWorkspaceLink(r.Context(), auth.WorkspaceID, r.PathValue("slug"))
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	limits, err := a.workspacePlan(r, auth.WorkspaceID)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	if !limits.QREnabled {
		writeError(w, http.StatusPaymentRequired, "QR codes are not included in this plan")
		return
	}
	size := queryInt(r, "size", 256, 128, 1024)
	png, err := qrcode.Encode(a.shortURL(item), qrcode.Medium, size)
	if err != nil {
		a.logger.Error("QR generation failed", "slug", item.Slug, "error", err)
		writeError(w, http.StatusInternalServerError, "could not generate QR code")
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Header().Set("Cache-Control", "private, max-age=300")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}

func (a *app) analyticsDashboard(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "analytics:read", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	limits, err := a.workspacePlan(r, auth.WorkspaceID)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	days := queryInt(r, "days", minInt(30, limits.AnalyticsDays), 1, limits.AnalyticsDays)
	to := time.Now().UTC()
	from := to.AddDate(0, 0, -days)
	analytics, err := a.store.WorkspaceAnalytics(r.Context(), auth.WorkspaceID, from, to, 10)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"analytics": analytics, "retentionDays": limits.AnalyticsDays})
}

func (a *app) usage(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "links:read", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	monthStart := startOfUTCMonth(time.Now().UTC())
	usage, err := a.store.WorkspaceUsage(r.Context(), auth.WorkspaceID, monthStart)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	limits, err := a.workspacePlan(r, auth.WorkspaceID)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"usage": usage, "limits": limits})
}

func (a *app) listPlans(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"plans": []planLimits{plans[core.PlanFree], plans[core.PlanPro]}})
}

func (a *app) createCustomDomain(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "workspace:admin", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	if !isWorkspaceAdmin(auth) {
		writeError(w, http.StatusForbidden, "workspace admin permission required")
		return
	}
	var req struct {
		Host string `json:"host"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	host, err := a.normalizeCustomDomain(req.Host)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	limits, err := a.workspacePlan(r, auth.WorkspaceID)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	usage, err := a.store.WorkspaceUsage(r.Context(), auth.WorkspaceID, startOfUTCMonth(time.Now().UTC()))
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	if usage.CustomDomains >= limits.CustomDomainLimit {
		writeError(w, http.StatusPaymentRequired, "custom-domain limit reached; upgrade the workspace plan")
		return
	}
	id, err := randomID("dom_", 16)
	if err != nil {
		a.internalRandomError(w, err)
		return
	}
	token, _, err := randomToken("qh8z_dns_")
	if err != nil {
		a.internalRandomError(w, err)
		return
	}
	now := time.Now().UTC()
	domain := core.CustomDomain{ID: id, WorkspaceID: auth.WorkspaceID, Host: host, VerificationToken: token, CreatedAt: now}
	audit := actorAudit(auth, "custom_domain.created", "custom_domain", id, now, map[string]string{"host": host})
	if err := a.store.CreateCustomDomain(r.Context(), domain, audit); err != nil {
		if errors.Is(err, core.ErrConflict) {
			writeError(w, http.StatusConflict, "that custom domain is already registered")
			return
		}
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, domainResponse(domain))
}

func (a *app) listCustomDomains(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "links:read", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	domains, err := a.store.ListCustomDomains(r.Context(), auth.WorkspaceID)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	items := make([]map[string]any, 0, len(domains))
	for _, domain := range domains {
		items = append(items, domainResponse(domain))
	}
	writeJSON(w, http.StatusOK, map[string]any{"domains": items})
}

func (a *app) verifyCustomDomain(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "workspace:admin", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	if !isWorkspaceAdmin(auth) {
		writeError(w, http.StatusForbidden, "workspace admin permission required")
		return
	}
	domain, err := a.store.GetCustomDomain(r.Context(), auth.WorkspaceID, r.PathValue("id"))
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	verified, err := a.dnsVerifier.Verify(r.Context(), domain.Host, domain.VerificationToken)
	if err != nil {
		a.logger.Error("custom domain DNS verification failed", "domain_id", domain.ID, "host", domain.Host, "error", err)
		writeError(w, http.StatusBadGateway, "DNS verification could not be completed")
		return
	}
	if !verified {
		writeJSON(w, http.StatusOK, map[string]any{"verified": false, "domain": domainResponse(domain)})
		return
	}
	now := time.Now().UTC()
	audit := actorAudit(auth, "custom_domain.verified", "custom_domain", domain.ID, now, map[string]string{"host": domain.Host})
	domain, err = a.store.SetCustomDomainVerified(r.Context(), auth.WorkspaceID, domain.ID, now, audit)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"verified": true, "domain": domainResponse(domain)})
}

func (a *app) deleteCustomDomain(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "workspace:admin", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	if !isWorkspaceAdmin(auth) {
		writeError(w, http.StatusForbidden, "workspace admin permission required")
		return
	}
	id := r.PathValue("id")
	now := time.Now().UTC()
	audit := actorAudit(auth, "custom_domain.deleted", "custom_domain", id, now, nil)
	if err := a.store.DeleteCustomDomain(r.Context(), auth.WorkspaceID, id, audit); err != nil {
		a.writeStoreError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) ensureLinkCapacity(r *http.Request, workspaceID string) error {
	limits, err := a.workspacePlan(r, workspaceID)
	if err != nil {
		return err
	}
	usage, err := a.store.WorkspaceUsage(r.Context(), workspaceID, startOfUTCMonth(time.Now().UTC()))
	if err != nil {
		return err
	}
	if usage.Links >= limits.LinkLimit {
		return core.ErrLimitExceeded
	}
	return nil
}

func (a *app) domainForLink(r *http.Request, auth core.AuthContext, domainID string) (core.CustomDomain, error) {
	limits, err := a.workspacePlan(r, auth.WorkspaceID)
	if err != nil {
		return core.CustomDomain{}, err
	}
	if limits.CustomDomainLimit == 0 {
		return core.CustomDomain{}, core.ErrLimitExceeded
	}
	domain, err := a.store.GetCustomDomain(r.Context(), auth.WorkspaceID, domainID)
	if err != nil {
		return core.CustomDomain{}, err
	}
	if domain.VerifiedAt == nil {
		return core.CustomDomain{}, fmt.Errorf("custom domain is not verified: %w", core.ErrForbidden)
	}
	return domain, nil
}

func (a *app) workspacePlan(r *http.Request, workspaceID string) (planLimits, error) {
	state, err := a.store.GetBillingState(r.Context(), workspaceID)
	if err != nil {
		return planLimits{}, err
	}
	if state.PlanCode == core.PlanPro && state.Status != core.BillingStatusCanceled {
		return plans[core.PlanPro], nil
	}
	return plans[core.PlanFree], nil
}

func (a *app) writeCommercialError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrLimitExceeded):
		writeError(w, http.StatusPaymentRequired, "this feature or resource limit requires a paid plan")
	case errors.Is(err, core.ErrNotFound):
		writeError(w, http.StatusNotFound, "custom domain not found")
	case errors.Is(err, core.ErrForbidden):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	default:
		a.writeStoreError(w, err)
	}
}

func (a *app) shortURL(item core.Link) string {
	if item.DomainHost != "" {
		return "https://" + item.DomainHost + "/" + item.Slug
	}
	return a.baseURL + "/" + item.Slug
}

func (a *app) normalizeCustomDomain(raw string) (string, error) {
	host := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	if host == "" || len(host) > 253 {
		return "", errors.New("invalid custom domain")
	}
	if net.ParseIP(host) != nil || strings.Contains(host, ":") {
		return "", errors.New("custom domain must be a DNS hostname, not an IP address or host:port")
	}
	labels := strings.Split(host, ".")
	if len(labels) < 2 {
		return "", errors.New("custom domain must contain a public suffix")
	}
	for _, label := range labels {
		if !domainLabelPattern.MatchString(label) {
			return "", errors.New("custom domain contains an invalid DNS label")
		}
	}
	base, err := url.Parse(a.baseURL)
	if err == nil && canonicalHost(base.Host) == host {
		return "", errors.New("custom domain cannot be the primary qh8z hostname")
	}
	return host, nil
}

func domainResponse(domain core.CustomDomain) map[string]any {
	result := map[string]any{
		"id":          domain.ID,
		"workspaceId": domain.WorkspaceID,
		"host":        domain.Host,
		"verifiedAt":  domain.VerifiedAt,
		"createdAt":   domain.CreatedAt,
	}
	if domain.VerifiedAt == nil {
		result["verification"] = map[string]string{
			"type":  "TXT",
			"name":  "_qh8z." + domain.Host,
			"value": "qh8z-verification=" + domain.VerificationToken,
		}
	}
	return result
}

func canonicalHost(hostport string) string {
	host := strings.ToLower(strings.TrimSpace(hostport))
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		return strings.TrimSuffix(parsed, ".")
	}
	return strings.TrimSuffix(host, ".")
}

func queryInt(r *http.Request, key string, fallback, minValue, maxValue int) int {
	raw := strings.TrimSpace(r.URL.Query().Get(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil || value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func startOfUTCMonth(now time.Time) time.Time {
	now = now.UTC()
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
