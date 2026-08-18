package main

import (
	"crypto/rand"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

type linkResponse struct {
	Slug             string     `json:"slug"`
	URL              string     `json:"url"`
	ShortURL         string     `json:"shortUrl"`
	WorkspaceID      string     `json:"workspaceId"`
	DomainID         string     `json:"domainId,omitempty"`
	DomainHost       string     `json:"domainHost,omitempty"`
	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
	Visits           int64      `json:"visits"`
	DisabledAt       *time.Time `json:"disabledAt,omitempty"`
	SuspendedAt      *time.Time `json:"suspendedAt,omitempty"`
	SuspensionReason string     `json:"suspensionReason,omitempty"`
}

var slugPattern = regexp.MustCompile(`^[a-z0-9_-]{3,64}$`)
var reserved = map[string]bool{
	"api": true, "healthz": true, "readyz": true, "admin": true,
	"login": true, "signup": true, "pricing": true, "verify-email": true,
	"dashboard": true,
}

const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func (a *app) createLink(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "links:write", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	if err := a.ensureLinkCapacity(r, auth.WorkspaceID); err != nil {
		if errors.Is(err, core.ErrLimitExceeded) {
			writeError(w, http.StatusPaymentRequired, "link limit reached; upgrade the workspace plan")
			return
		}
		a.writeStoreError(w, err)
		return
	}
	var req struct {
		URL        string `json:"url"`
		CustomSlug string `json:"customSlug"`
		DomainID   string `json:"domainId"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	target, err := normalizeURL(req.URL)
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
	domainID := strings.TrimSpace(req.DomainID)
	domainHost := ""
	if domainID != "" {
		domain, err := a.domainForLink(r, auth, domainID)
		if err != nil {
			a.writeCommercialError(w, err)
			return
		}
		domainID = domain.ID
		domainHost = domain.Host
	}
	now := time.Now().UTC()
	custom := strings.TrimSpace(req.CustomSlug)
	if custom != "" {
		if !slugPattern.MatchString(custom) || reserved[custom] {
			writeError(w, http.StatusBadRequest, "custom slug must be 3-64 lowercase letters, numbers, hyphens, or underscores and cannot be reserved")
			return
		}
		item := core.Link{
			Slug:            custom,
			URL:             target,
			WorkspaceID:     auth.WorkspaceID,
			CreatedByUserID: auth.UserID,
			DomainID:        domainID,
			DomainHost:      domainHost,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		if err := a.createOwnedLink(r, auth, item); err != nil {
			a.writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, a.response(item))
		return
	}

	for attempts := 0; attempts < 8; attempts++ {
		slug, err := randomSlug(7)
		if err != nil {
			a.internalRandomError(w, err)
			return
		}
		item := core.Link{
			Slug:            slug,
			URL:             target,
			WorkspaceID:     auth.WorkspaceID,
			CreatedByUserID: auth.UserID,
			DomainID:        domainID,
			DomainHost:      domainHost,
			CreatedAt:       now,
			UpdatedAt:       now,
		}
		err = a.createOwnedLink(r, auth, item)
		if errors.Is(err, core.ErrConflict) {
			continue
		}
		if err != nil {
			a.writeStoreError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, a.response(item))
		return
	}
	writeError(w, http.StatusServiceUnavailable, "could not allocate a unique short code")
}

func (a *app) createOwnedLink(r *http.Request, auth core.AuthContext, item core.Link) error {
	audit := actorAudit(auth, "link.created", "link", item.Slug, item.CreatedAt, map[string]string{
		"destination": item.URL,
		"domain_id":   item.DomainID,
	})
	return a.store.CreateOwnedLink(r.Context(), item, audit)
}

func (a *app) getLink(w http.ResponseWriter, r *http.Request) {
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
	writeJSON(w, http.StatusOK, a.response(item))
}

func (a *app) linkStats(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "analytics:read", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	stats, err := a.store.StatsForWorkspace(r.Context(), auth.WorkspaceID, r.PathValue("slug"), 50)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, stats)
}

func (a *app) redirect(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	requestHost := canonicalHost(r.Host)
	base, _ := url.Parse(a.baseURL)
	primaryHost := canonicalHost(base.Host)
	var item core.Link
	var err error
	if requestHost == primaryHost {
		item, err = a.store.GetLink(r.Context(), slug)
		if err == nil && item.DomainID != "" {
			err = core.ErrNotFound
		}
	} else {
		item, err = a.store.GetCustomDomainLink(r.Context(), requestHost, slug)
	}
	if errors.Is(err, core.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.logger.Error("redirect lookup failed", "slug", slug, "host", requestHost, "error", err)
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
		return
	}
	if item.DisabledAt != nil || item.SuspendedAt != nil {
		http.NotFound(w, r)
		return
	}
	visit := core.Visit{
		Slug:      slug,
		VisitedAt: time.Now().UTC(),
		Referer:   sanitizeHeader(r.Referer(), 2048),
		UserAgent: sanitizeHeader(r.UserAgent(), 1024),
	}
	if _, err := a.store.RecordVisit(r.Context(), visit); err != nil {
		a.logger.Error("visit recording failed", "slug", slug, "error", err)
	}
	http.Redirect(w, r, item.URL, http.StatusFound)
}

func (a *app) response(item core.Link) linkResponse {
	return linkResponse{
		Slug:             item.Slug,
		URL:              item.URL,
		ShortURL:         a.shortURL(item),
		WorkspaceID:      item.WorkspaceID,
		DomainID:         item.DomainID,
		DomainHost:       item.DomainHost,
		CreatedAt:        item.CreatedAt,
		UpdatedAt:        item.UpdatedAt,
		Visits:           item.Visits,
		DisabledAt:       item.DisabledAt,
		SuspendedAt:      item.SuspendedAt,
		SuspensionReason: item.SuspensionReason,
	}
}

func normalizeURL(raw string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return "", errors.New("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errors.New("URL must use http or https")
	}
	if u.User != nil {
		return "", errors.New("URLs containing credentials are not allowed")
	}
	if err := validatePublicDestination(u); err != nil {
		return "", err
	}
	return u.String(), nil
}

func randomSlug(n int) (string, error) {
	out := make([]byte, 0, n)
	b := make([]byte, 1)
	for len(out) < n {
		if _, err := rand.Read(b); err != nil {
			return "", err
		}
		if b[0] >= 252 {
			continue
		}
		out = append(out, alphabet[int(b[0])%len(alphabet)])
	}
	return string(out), nil
}

func sanitizeHeader(value string, maxBytes int) string {
	value = strings.ToValidUTF8(value, "")
	if len(value) <= maxBytes {
		return value
	}
	value = value[:maxBytes]
	for !utf8.ValidString(value) {
		value = value[:len(value)-1]
	}
	return value
}
