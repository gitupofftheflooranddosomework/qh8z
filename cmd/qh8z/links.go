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
	Slug        string    `json:"slug"`
	URL         string    `json:"url"`
	ShortURL    string    `json:"shortUrl"`
	WorkspaceID string    `json:"workspaceId"`
	CreatedAt   time.Time `json:"createdAt"`
	Visits      int64     `json:"visits"`
}

var slugPattern = regexp.MustCompile(`^[a-z0-9_-]{3,64}$`)
var reserved = map[string]bool{
	"api": true, "healthz": true, "readyz": true, "admin": true,
	"login": true, "signup": true, "pricing": true, "verify-email": true,
}

const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func (a *app) createLink(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "links:write", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	var req struct {
		URL        string `json:"url"`
		CustomSlug string `json:"customSlug"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	target, err := normalizeURL(req.URL)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	custom := strings.TrimSpace(req.CustomSlug)
	if custom != "" {
		if !slugPattern.MatchString(custom) || reserved[custom] {
			writeError(w, http.StatusBadRequest, "custom slug must be 3-64 lowercase letters, numbers, hyphens, or underscores and cannot be reserved")
			return
		}
		item := core.Link{Slug: custom, URL: target, WorkspaceID: auth.WorkspaceID, CreatedByUserID: auth.UserID, CreatedAt: time.Now().UTC()}
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
		item := core.Link{Slug: slug, URL: target, WorkspaceID: auth.WorkspaceID, CreatedByUserID: auth.UserID, CreatedAt: time.Now().UTC()}
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
	audit := actorAudit(auth, "link.created", "link", item.Slug, item.CreatedAt, map[string]string{"destination": item.URL})
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
	item, err := a.store.GetLink(r.Context(), slug)
	if errors.Is(err, core.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		a.logger.Error("redirect lookup failed", "slug", slug, "error", err)
		http.Error(w, "service unavailable", http.StatusServiceUnavailable)
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
		Slug:        item.Slug,
		URL:         item.URL,
		ShortURL:    a.baseURL + "/" + item.Slug,
		WorkspaceID: item.WorkspaceID,
		CreatedAt:   item.CreatedAt,
		Visits:      item.Visits,
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
