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
	"api": true, "assets": true, "internal": true,
	"healthz": true, "readyz": true, "metrics": true,
	"admin": true, "login": true, "signup": true, "verify-email": true,
	"dashboard": true, "pricing": true,
	"terms": true, "privacy": true, "acceptable-use": true, "report-abuse": true,
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
		if !validSlug(custom) {
			writeError(w, http.StatusBadRequest, "customSlug must be 3-64 lowercase letters, numbers, underscores, or hyphens")
			return
		}
		if reserved[custom] {
			writeError(w, http.StatusBadRequest, "customSlug is reserved")
			return
		}
		created, err := a.store.CreateLink(r.Context(), core.Link{
			Slug:            custom,
			URL:             target,
			WorkspaceID:     auth.WorkspaceID,
			CreatedByUserID: auth.UserID,
			DomainID:        domainID,
			DomainHost:      domainHost,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
		if err != nil {
			if errors.Is(err, core.ErrConflict) {
				writeError(w, http.StatusConflict, "customSlug already exists")
				return
			}
			a.writeStoreError(w, err)
			return
		}
		_ = a.audit(r, auth, "link.created", "link", created.Slug, map[string]string{"custom": "true"})
		writeJSON(w, http.StatusCreated, a.presentLink(created))
		return
	}
	for range 10 {
		slug, err := randomSlug(7)
		if err != nil {
			a.internalRandomError(w, err)
			return
		}
		created, err := a.store.CreateLink(r.Context(), core.Link{
			Slug:            slug,
			URL:             target,
			WorkspaceID:     auth.WorkspaceID,
			CreatedByUserID: auth.UserID,
			DomainID:        domainID,
			DomainHost:      domainHost,
			CreatedAt:       now,
			UpdatedAt:       now,
		})
		if err == nil {
			_ = a.audit(r, auth, "link.created", "link", created.Slug, map[string]string{"custom": "false"})
			writeJSON(w, http.StatusCreated, a.presentLink(created))
			return
		}
		if !errors.Is(err, core.ErrConflict) {
			a.writeStoreError(w, err)
			return
		}
	}
	writeError(w, http.StatusServiceUnavailable, "could not allocate a unique short code")
}

func (a *app) getLink(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "links:read", false)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	link, err := a.store.GetLink(r.Context(), r.PathValue("slug"))
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	if !a.linkReadableBy(auth, link) {
		writeError(w, http.StatusNotFound, "not found")
		return
	}
	writeJSON(w, http.StatusOK, a.presentLink(link))
}

func (a *app) redirect(w http.ResponseWriter, r *http.Request) {
	slug := r.PathValue("slug")
	host := requestHostname(r)
	var link core.Link
	var err error
	if host != "" && !sameHostname(host, baseHostname(a.baseURL)) {
		link, err = a.store.GetCustomDomainLink(r.Context(), host, slug)
	} else {
		link, err = a.store.GetLink(r.Context(), slug)
	}
	if err != nil || link.DisabledAt != nil || link.SuspendedAt != nil {
		http.NotFound(w, r)
		return
	}
	if err := a.store.IncrementVisit(r.Context(), slug, time.Now().UTC(), r.Referer(), r.UserAgent()); err != nil {
		a.logger.Error("failed to record visit", "slug", slug, "error", err)
	}
	http.Redirect(w, r, link.URL, http.StatusFound)
}

func (a *app) presentLink(link core.Link) linkResponse {
	shortBase := a.baseURL
	if link.DomainHost != "" {
		shortBase = "https://" + link.DomainHost
	}
	return linkResponse{
		Slug:             link.Slug,
		URL:              link.URL,
		ShortURL:         strings.TrimRight(shortBase, "/") + "/" + link.Slug,
		WorkspaceID:      link.WorkspaceID,
		DomainID:         link.DomainID,
		DomainHost:       link.DomainHost,
		CreatedAt:        link.CreatedAt,
		UpdatedAt:        link.UpdatedAt,
		Visits:           link.Visits,
		DisabledAt:       link.DisabledAt,
		SuspendedAt:      link.SuspendedAt,
		SuspensionReason: link.SuspensionReason,
	}
}

func validSlug(s string) bool {
	return utf8.RuneCountInString(s) == len(s) && slugPattern.MatchString(s)
}

func randomSlug(n int) (string, error) {
	out := make([]byte, n)
	for i := range out {
		for {
			var b [1]byte
			if _, err := rand.Read(b[:]); err != nil {
				return "", err
			}
			if b[0] < 252 {
				out[i] = alphabet[int(b[0])%len(alphabet)]
				break
			}
		}
	}
	return string(out), nil
}

func normalizeURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if len(raw) > 2048 {
		return "", errors.New("url is too long")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return "", errors.New("url must be an absolute http or https URL")
	}
	if u.User != nil {
		return "", errors.New("URLs containing credentials are not allowed")
	}
	return u.String(), nil
}
