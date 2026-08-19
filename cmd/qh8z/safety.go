package main

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
	"github.com/gitupofftheflooranddosomework/qh8z/internal/reputation"
)

type safetyConfig struct {
	adminToken     string
	rateLimitSalt  string
	trustedProxies []netip.Prefix
}

type rateLimitError struct {
	Result core.RateLimitResult
}

func (e *rateLimitError) Error() string { return "rate limit exceeded" }

type destinationRejectedError struct {
	reason string
}

func (e *destinationRejectedError) Error() string { return e.reason }

func openSafety(environment string) (reputation.Checker, safetyConfig, error) {
	adminToken, err := secretValue("QH8Z_ADMIN_TOKEN")
	if err != nil {
		return nil, safetyConfig{}, err
	}
	rateLimitSalt, err := secretValue("QH8Z_RATE_LIMIT_SALT")
	if err != nil {
		return nil, safetyConfig{}, err
	}
	cfg := safetyConfig{adminToken: adminToken, rateLimitSalt: rateLimitSalt}
	for _, raw := range strings.Split(envOr("QH8Z_TRUSTED_PROXIES", ""), ",") {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(raw)
		if err != nil {
			return nil, safetyConfig{}, fmt.Errorf("invalid QH8Z_TRUSTED_PROXIES entry %q: %w", raw, err)
		}
		cfg.trustedProxies = append(cfg.trustedProxies, prefix)
	}

	mode := strings.ToLower(envOr("QH8Z_REPUTATION_MODE", "disabled"))
	if environment == "production" {
		if len(cfg.adminToken) < 32 {
			return nil, safetyConfig{}, errors.New("QH8Z_ADMIN_TOKEN must be at least 32 bytes in production")
		}
		if len(cfg.rateLimitSalt) < 32 {
			return nil, safetyConfig{}, errors.New("QH8Z_RATE_LIMIT_SALT must be at least 32 bytes in production")
		}
		if mode != "webrisk" {
			return nil, safetyConfig{}, errors.New("QH8Z_REPUTATION_MODE must be webrisk in production")
		}
	}
	if cfg.rateLimitSalt == "" {
		cfg.rateLimitSalt = "qh8z-development-rate-limit-salt"
	}

	switch mode {
	case "disabled":
		return reputation.AllowAll{}, cfg, nil
	case "webrisk":
		apiKey, err := secretValue("WEBRISK_API_KEY")
		if err != nil {
			return nil, safetyConfig{}, err
		}
		extended := strings.EqualFold(envOr("WEBRISK_EXTENDED_COVERAGE", "false"), "true")
		checker, err := reputation.NewWebRisk(apiKey, extended)
		if err != nil {
			return nil, safetyConfig{}, err
		}
		return checker, cfg, nil
	default:
		return nil, safetyConfig{}, errors.New("QH8Z_REPUTATION_MODE must be disabled or webrisk")
	}
}

func (a *app) apiRateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		if err := a.enforceIPLimit(r, "api", 300, time.Minute); err != nil {
			a.writeRateLimitOrStoreError(w, err)
			return
		}
		if err := a.enforceCredentialLimit(r); err != nil {
			a.writeRateLimitOrStoreError(w, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) limitIPHandler(class string, limit int, window time.Duration, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := a.enforceIPLimit(r, class, limit, window); err != nil {
			a.writeRateLimitOrStoreError(w, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *app) enforceCredentialLimit(r *http.Request) error {
	workspaceHint := strings.TrimSpace(r.Header.Get("X-QH8Z-Workspace"))
	if authorization := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(authorization, "Bearer ") {
		secret := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		now := time.Now().UTC()
		if strings.HasPrefix(secret, "qh8z_sk_") {
			auth, err := a.store.ResolveAPIKey(r.Context(), secretHash(secret), now)
			if err == nil {
				return a.enforcePrincipalLimit(r, auth)
			}
			return nil
		}
		if strings.HasPrefix(secret, "qh8z_ss_") {
			auth, err := a.store.ResolveSession(r.Context(), secretHash(secret), workspaceHint, now)
			if err == nil {
				return a.enforcePrincipalLimit(r, auth)
			}
			return nil
		}
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || !strings.HasPrefix(cookie.Value, "qh8z_ss_") {
		return nil
	}
	auth, err := a.store.ResolveSession(r.Context(), secretHash(cookie.Value), workspaceHint, time.Now().UTC())
	if err != nil {
		return nil
	}
	return a.enforcePrincipalLimit(r, auth)
}

func (a *app) enforceIPLimit(r *http.Request, class string, limit int, window time.Duration) error {
	ip := a.clientIP(r)
	key := "ip:" + a.privateIPKey(ip) + ":" + class
	return a.enforceBucket(r.Context(), key, limit, window)
}

func (a *app) enforcePrincipalLimit(r *http.Request, auth core.AuthContext) error {
	if auth.APIKeyID != "" {
		return a.enforceBucket(r.Context(), "api_key:"+auth.APIKeyID, 600, time.Minute)
	}
	if auth.UserID != "" {
		return a.enforceBucket(r.Context(), "user:"+auth.UserID, 300, time.Minute)
	}
	return nil
}

func (a *app) enforceBucket(ctx context.Context, key string, limit int, window time.Duration) error {
	now := time.Now().UTC()
	windowStart := now.Truncate(window)
	resetAt := windowStart.Add(window)
	result, err := a.store.CheckRateLimit(ctx, key, windowStart, resetAt, limit)
	if err != nil {
		return err
	}
	if !result.Allowed {
		return &rateLimitError{Result: result}
	}
	return nil
}

func (a *app) writeRateLimitOrStoreError(w http.ResponseWriter, err error) {
	var limited *rateLimitError
	if errors.As(err, &limited) {
		retry := time.Until(limited.Result.ResetAt)
		seconds := int(retry.Seconds())
		if seconds < 1 {
			seconds = 1
		}
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(limited.Result.Limit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(limited.Result.Remaining))
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}
	a.writeStoreError(w, err)
}

func (a *app) clientIP(r *http.Request) netip.Addr {
	peer := remoteIP(r.RemoteAddr)
	if !peer.IsValid() || !a.isTrustedProxy(peer) {
		return peer
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		candidate, err := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if err != nil {
			continue
		}
		candidate = candidate.Unmap()
		if !a.isTrustedProxy(candidate) {
			return candidate
		}
	}
	return peer
}

func remoteIP(remoteAddr string) netip.Addr {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		host = remoteAddr
	}
	ip, err := netip.ParseAddr(strings.TrimSpace(host))
	if err != nil {
		return netip.Addr{}
	}
	return ip.Unmap()
}

func (a *app) isTrustedProxy(ip netip.Addr) bool {
	for _, prefix := range a.safety.trustedProxies {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

func (a *app) privateIPKey(ip netip.Addr) string {
	value := "unknown"
	if ip.IsValid() {
		value = ip.String()
	}
	sum := sha256.Sum256([]byte(a.safety.rateLimitSalt + "\x00" + value))
	return hex.EncodeToString(sum[:])
}

func (a *app) adminAuthorized(r *http.Request) bool {
	if a.safety.adminToken == "" {
		return false
	}
	const prefix = "Bearer "
	authorization := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(authorization, prefix) {
		return false
	}
	provided := strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
	if len(provided) != len(a.safety.adminToken) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(a.safety.adminToken)) == 1
}

func (a *app) requireAdmin(w http.ResponseWriter, r *http.Request) bool {
	if !a.adminAuthorized(r) {
		writeError(w, http.StatusUnauthorized, "admin authentication required")
		return false
	}
	return true
}

func validatePublicDestination(u *url.URL) error {
	host := strings.ToLower(strings.TrimSuffix(u.Hostname(), "."))
	if host == "" || strings.ContainsAny(host, "\x00\r\n\t ") {
		return errors.New("destination host is invalid")
	}
	if port := u.Port(); port != "" {
		n, err := strconv.Atoi(port)
		if err != nil || n < 1 || n > 65535 {
			return errors.New("destination port is invalid")
		}
	}
	for _, suffix := range []string{"localhost", ".localhost", ".local", ".internal", ".home.arpa", ".test", ".invalid", ".example"} {
		if host == strings.TrimPrefix(suffix, ".") || strings.HasSuffix(host, suffix) {
			return errors.New("local or reserved destinations are not allowed")
		}
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if nonPublicIP(ip.Unmap()) {
			return errors.New("non-public IP destinations are not allowed")
		}
		return nil
	}
	if legacyNumericHost(host) {
		return errors.New("numeric IP-like destinations are not allowed")
	}
	if !strings.Contains(host, ".") {
		return errors.New("single-label destination hosts are not allowed")
	}
	return nil
}

func nonPublicIP(ip netip.Addr) bool {
	if !ip.IsValid() || ip.IsPrivate() || ip.IsLoopback() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
		return true
	}
	blocked := []netip.Prefix{
		netip.MustParsePrefix("100.64.0.0/10"),
		netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("192.0.2.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"),
		netip.MustParsePrefix("198.51.100.0/24"),
		netip.MustParsePrefix("203.0.113.0/24"),
		netip.MustParsePrefix("2001:db8::/32"),
	}
	for _, prefix := range blocked {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

func legacyNumericHost(host string) bool {
	parts := strings.Split(host, ".")
	if len(parts) == 0 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		for _, r := range part {
			if r < '0' || r > '9' {
				return false
			}
		}
	}
	return true
}

func (a *app) checkDestination(ctx context.Context, target string, auth core.AuthContext) error {
	parsed, err := url.Parse(target)
	if err != nil {
		return &destinationRejectedError{reason: "destination is invalid"}
	}
	host := strings.ToLower(strings.TrimSuffix(parsed.Hostname(), "."))
	rule, err := a.store.MatchURLRule(ctx, host)
	if err != nil && !errors.Is(err, core.ErrNotFound) {
		return err
	}
	if err == nil {
		if rule.Action == core.URLRuleBlock {
			_ = a.store.WriteAudit(ctx, actorAudit(auth, "link.blocked_by_rule", "url_rule", strconv.FormatInt(rule.ID, 10), time.Now().UTC(), map[string]string{"host": host}))
			return &destinationRejectedError{reason: "destination is blocked by safety policy"}
		}
		if rule.Action == core.URLRuleAllow {
			return nil
		}
	}

	result, err := a.reputation.Check(ctx, target)
	if err != nil {
		return fmt.Errorf("destination reputation check: %w", err)
	}
	if result.Unsafe {
		_ = a.store.WriteAudit(ctx, actorAudit(auth, "link.blocked_by_reputation", "destination", host, time.Now().UTC(), map[string]string{"threats": strings.Join(result.ThreatTypes, ",")}))
		return &destinationRejectedError{reason: "destination is blocked by safety policy"}
	}
	return nil
}

func normalizeRulePattern(matchType, raw string) (string, error) {
	pattern := strings.ToLower(strings.TrimSpace(strings.TrimSuffix(raw, ".")))
	if matchType == core.URLRuleDomain {
		pattern = strings.TrimPrefix(pattern, "*.")
	}
	if matchType != core.URLRuleHost && matchType != core.URLRuleDomain {
		return "", errors.New("matchType must be host or domain")
	}
	if pattern == "" || strings.ContainsAny(pattern, "/:@?#\x00\r\n\t ") {
		return "", errors.New("rule pattern must be a hostname")
	}
	return pattern, nil
}
