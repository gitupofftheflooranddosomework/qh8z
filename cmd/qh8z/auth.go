package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
	"github.com/gitupofftheflooranddosomework/qh8z/internal/password"
)

const (
	sessionCookieName     = "qh8z_session"
	sessionLifetime       = 30 * 24 * time.Hour
	verificationLifetime  = 24 * time.Hour
	maxWorkspaceNameBytes = 80
	maxAPIKeyNameBytes    = 80
)

var allowedAPIKeyScopes = map[string]bool{
	"links:read":      true,
	"links:write":     true,
	"analytics:read":  true,
	"workspace:admin": true,
}

var loginDummyHash = func() string {
	hash, err := password.Hash("qh8z-dummy-password-never-used")
	if err != nil {
		panic(err)
	}
	return hash
}()

func (a *app) register(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email         string `json:"email"`
		Password      string `json:"password"`
		WorkspaceName string `json:"workspaceName"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	passwordHash, err := password.Hash(req.Password)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	workspaceName, err := normalizeWorkspaceName(req.WorkspaceName)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().UTC()
	userID, err := randomID("usr_", 16)
	if err != nil {
		a.internalRandomError(w, err)
		return
	}
	workspaceID, err := randomID("ws_", 16)
	if err != nil {
		a.internalRandomError(w, err)
		return
	}
	sessionSecret, sessionHash, err := randomToken("qh8z_ss_")
	if err != nil {
		a.internalRandomError(w, err)
		return
	}
	verificationSecret, verificationHash, err := randomToken("qh8z_ev_")
	if err != nil {
		a.internalRandomError(w, err)
		return
	}

	user := core.User{ID: userID, Email: email, PasswordHash: passwordHash, CreatedAt: now}
	workspace := core.Workspace{ID: workspaceID, Name: workspaceName, CreatedAt: now}
	reg := core.Registration{
		User:      user,
		Workspace: workspace,
		Membership: core.Membership{
			WorkspaceID: workspaceID,
			UserID:      userID,
			Role:        core.RoleOwner,
			CreatedAt:   now,
		},
		Verification: core.EmailVerification{
			TokenHash: verificationHash,
			UserID:    userID,
			CreatedAt: now,
			ExpiresAt: now.Add(verificationLifetime),
		},
		Session: core.Session{
			TokenHash:  sessionHash,
			UserID:     userID,
			CreatedAt:  now,
			ExpiresAt:  now.Add(sessionLifetime),
			LastSeenAt: now,
		},
		Audit: []core.AuditEntry{
			{ActorUserID: userID, Action: "user.registered", ResourceType: "user", ResourceID: userID, CreatedAt: now},
			{WorkspaceID: workspaceID, ActorUserID: userID, Action: "workspace.created", ResourceType: "workspace", ResourceID: workspaceID, CreatedAt: now},
		},
	}
	if err := a.store.Register(r.Context(), reg); err != nil {
		if errors.Is(err, core.ErrConflict) {
			writeError(w, http.StatusConflict, "an account with that email already exists")
			return
		}
		a.writeStoreError(w, err)
		return
	}
	a.setSessionCookie(w, sessionSecret, reg.Session.ExpiresAt)

	verificationURL := a.verificationURL(verificationSecret)
	emailSent := true
	if err := a.mailer.SendVerification(r.Context(), email, verificationURL); err != nil {
		emailSent = false
		a.logger.Error("verification email failed", "user_id", userID, "error", err)
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"user":                      user,
		"workspace":                 workspace,
		"emailVerificationSent":     emailSent,
		"emailVerificationRequired": true,
	})
}

func (a *app) login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	user, err := a.store.UserByEmail(r.Context(), email)
	if errors.Is(err, core.ErrNotFound) {
		_ = password.Verify(loginDummyHash, req.Password)
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	if !password.Verify(user.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid email or password")
		return
	}
	now := time.Now().UTC()
	secret, hash, err := randomToken("qh8z_ss_")
	if err != nil {
		a.internalRandomError(w, err)
		return
	}
	workspaces, err := a.store.ListWorkspaces(r.Context(), user.ID)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	workspaceID := ""
	if len(workspaces) > 0 {
		workspaceID = workspaces[0].ID
	}
	session := core.Session{TokenHash: hash, UserID: user.ID, CreatedAt: now, ExpiresAt: now.Add(sessionLifetime), LastSeenAt: now}
	audit := core.AuditEntry{WorkspaceID: workspaceID, ActorUserID: user.ID, Action: "user.login", ResourceType: "user", ResourceID: user.ID, CreatedAt: now}
	if err := a.store.CreateSession(r.Context(), session, audit); err != nil {
		a.writeStoreError(w, err)
		return
	}
	a.setSessionCookie(w, secret, session.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{
		"user":       user,
		"workspaces": workspaces,
	})
}

func (a *app) logout(w http.ResponseWriter, r *http.Request) {
	secret, ok := sessionSecretFromRequest(r)
	if !ok {
		writeError(w, http.StatusUnauthorized, "session required")
		return
	}
	if err := a.store.DeleteSession(r.Context(), secretHash(secret)); err != nil {
		a.writeStoreError(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
	})
	w.WriteHeader(http.StatusNoContent)
}

func (a *app) verifyEmail(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token string `json:"token"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	user, err := a.consumeVerification(r, req.Token)
	if err != nil {
		a.writeVerificationError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"user": user, "verified": true})
}

func (a *app) verifyEmailPage(w http.ResponseWriter, r *http.Request) {
	user, err := a.consumeVerification(r, r.URL.Query().Get("token"))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`<!doctype html><html><body><h1>Verification failed</h1><p>The verification link is invalid, expired, or already used.</p></body></html>`))
		return
	}
	_, _ = w.Write([]byte(`<!doctype html><html><body><h1>Email verified</h1><p>Your qh8z account is ready.</p><p>` + htmlEscape(user.Email) + `</p></body></html>`))
}

func (a *app) consumeVerification(r *http.Request, token string) (core.User, error) {
	if !strings.HasPrefix(token, "qh8z_ev_") || len(token) < 40 {
		return core.User{}, core.ErrUnauthorized
	}
	return a.store.ConsumeEmailVerification(r.Context(), secretHash(token), time.Now().UTC())
}

func (a *app) resendVerification(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authenticate(r)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	if auth.Credential != "session" {
		writeError(w, http.StatusForbidden, "session authentication required")
		return
	}
	if auth.EmailVerified {
		writeJSON(w, http.StatusOK, map[string]bool{"alreadyVerified": true})
		return
	}
	now := time.Now().UTC()
	secret, hash, err := randomToken("qh8z_ev_")
	if err != nil {
		a.internalRandomError(w, err)
		return
	}
	verification := core.EmailVerification{TokenHash: hash, UserID: auth.UserID, CreatedAt: now, ExpiresAt: now.Add(verificationLifetime)}
	if err := a.store.CreateEmailVerification(r.Context(), verification); err != nil {
		a.writeStoreError(w, err)
		return
	}
	if err := a.mailer.SendVerification(r.Context(), auth.Email, a.verificationURL(secret)); err != nil {
		a.logger.Error("verification email failed", "user_id", auth.UserID, "error", err)
		writeError(w, http.StatusBadGateway, "verification email could not be sent")
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"sent": true})
}

func (a *app) me(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authenticate(r)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	workspaces, err := a.store.ListWorkspaces(r.Context(), auth.UserID)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"principal": auth, "workspaces": workspaces})
}

func (a *app) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authenticate(r)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	workspaces, err := a.store.ListWorkspaces(r.Context(), auth.UserID)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"workspaces": workspaces})
}

func (a *app) createWorkspace(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorize(r, "", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	if auth.Credential != "session" {
		writeError(w, http.StatusForbidden, "session authentication required")
		return
	}
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	name, err := normalizeWorkspaceName(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := randomID("ws_", 16)
	if err != nil {
		a.internalRandomError(w, err)
		return
	}
	now := time.Now().UTC()
	workspace := core.Workspace{ID: id, Name: name, CreatedAt: now}
	audit := core.AuditEntry{WorkspaceID: id, ActorUserID: auth.UserID, Action: "workspace.created", ResourceType: "workspace", ResourceID: id, CreatedAt: now}
	if err := a.store.CreateWorkspace(r.Context(), workspace, auth.UserID, audit); err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, workspace)
}

func (a *app) listWorkspaceMembers(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorizeWorkspace(r, r.PathValue("workspace"), "", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	members, err := a.store.ListWorkspaceMembers(r.Context(), auth.WorkspaceID)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": members})
}

func (a *app) addWorkspaceMember(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorizeWorkspace(r, r.PathValue("workspace"), "", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	if !isWorkspaceAdmin(auth) {
		writeError(w, http.StatusForbidden, "workspace admin permission required")
		return
	}
	var req struct {
		Email string `json:"email"`
		Role  string `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	email, err := normalizeEmail(req.Email)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	role := strings.ToLower(strings.TrimSpace(req.Role))
	if role != core.RoleMember && role != core.RoleAdmin {
		writeError(w, http.StatusBadRequest, "role must be member or admin")
		return
	}
	user, err := a.store.UserByEmail(r.Context(), email)
	if errors.Is(err, core.ErrNotFound) {
		writeError(w, http.StatusNotFound, "user not found")
		return
	}
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	now := time.Now().UTC()
	membership := core.Membership{WorkspaceID: auth.WorkspaceID, UserID: user.ID, Email: user.Email, Role: role, CreatedAt: now}
	audit := actorAudit(auth, "workspace.member_added", "user", user.ID, now, map[string]string{"role": role})
	if err := a.store.AddWorkspaceMember(r.Context(), membership, audit); err != nil {
		if errors.Is(err, core.ErrConflict) {
			writeError(w, http.StatusConflict, "user is already a workspace member")
			return
		}
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, membership)
}

func (a *app) createAPIKey(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorizeWorkspace(r, r.PathValue("workspace"), "", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	if auth.Credential != "session" || !isWorkspaceAdmin(auth) {
		writeError(w, http.StatusForbidden, "verified workspace owner or admin session required")
		return
	}
	var req struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	name := strings.TrimSpace(req.Name)
	if len(name) < 1 || len(name) > maxAPIKeyNameBytes {
		writeError(w, http.StatusBadRequest, "API key name must be 1-80 bytes")
		return
	}
	scopes, err := normalizeScopes(req.Scopes)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	id, err := randomID("key_", 16)
	if err != nil {
		a.internalRandomError(w, err)
		return
	}
	secret, hash, err := randomToken("qh8z_sk_")
	if err != nil {
		a.internalRandomError(w, err)
		return
	}
	now := time.Now().UTC()
	key := core.APIKey{ID: id, WorkspaceID: auth.WorkspaceID, Name: name, KeyHash: hash, Scopes: scopes, CreatedByUserID: auth.UserID, CreatedAt: now}
	audit := actorAudit(auth, "api_key.created", "api_key", id, now, map[string]string{"name": name})
	if err := a.store.CreateAPIKey(r.Context(), key, audit); err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"apiKey": key, "secret": secret})
}

func (a *app) auditLog(w http.ResponseWriter, r *http.Request) {
	auth, err := a.authorizeWorkspace(r, r.PathValue("workspace"), "workspace:admin", true)
	if err != nil {
		a.writeAuthError(w, err)
		return
	}
	if auth.Credential == "session" && !isWorkspaceAdmin(auth) {
		writeError(w, http.StatusForbidden, "workspace admin permission required")
		return
	}
	entries, err := a.store.ListAudit(r.Context(), auth.WorkspaceID, 100)
	if err != nil {
		a.writeStoreError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"audit": entries})
}

func (a *app) authenticate(r *http.Request) (core.AuthContext, error) {
	workspaceHint := strings.TrimSpace(r.Header.Get("X-QH8Z-Workspace"))
	if authorization := strings.TrimSpace(r.Header.Get("Authorization")); authorization != "" {
		const prefix = "Bearer "
		if !strings.HasPrefix(authorization, prefix) {
			return core.AuthContext{}, core.ErrUnauthorized
		}
		secret := strings.TrimSpace(strings.TrimPrefix(authorization, prefix))
		switch {
		case strings.HasPrefix(secret, "qh8z_sk_"):
			auth, err := a.store.ResolveAPIKey(r.Context(), secretHash(secret), time.Now().UTC())
			if err != nil {
				return core.AuthContext{}, err
			}
			if workspaceHint != "" && workspaceHint != auth.WorkspaceID {
				return core.AuthContext{}, core.ErrForbidden
			}
			return auth, nil
		case strings.HasPrefix(secret, "qh8z_ss_"):
			return a.store.ResolveSession(r.Context(), secretHash(secret), workspaceHint, time.Now().UTC())
		default:
			return core.AuthContext{}, core.ErrUnauthorized
		}
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil || !strings.HasPrefix(cookie.Value, "qh8z_ss_") {
		return core.AuthContext{}, core.ErrUnauthorized
	}
	if isUnsafeMethod(r.Method) && !sameSiteRequest(r, a.baseURL) {
		return core.AuthContext{}, core.ErrForbidden
	}
	return a.store.ResolveSession(r.Context(), secretHash(cookie.Value), workspaceHint, time.Now().UTC())
}

func (a *app) authorize(r *http.Request, scope string, requireVerified bool) (core.AuthContext, error) {
	auth, err := a.authenticate(r)
	if err != nil {
		return core.AuthContext{}, err
	}
	if requireVerified && !auth.EmailVerified {
		return core.AuthContext{}, core.ErrEmailUnverified
	}
	if scope != "" && auth.Credential == "api_key" && !hasScope(auth.Scopes, scope) {
		return core.AuthContext{}, core.ErrForbidden
	}
	return auth, nil
}

func (a *app) authorizeWorkspace(r *http.Request, workspaceID, scope string, requireVerified bool) (core.AuthContext, error) {
	clone := r.Clone(r.Context())
	clone.Header = r.Header.Clone()
	clone.Header.Set("X-QH8Z-Workspace", workspaceID)
	auth, err := a.authorize(clone, scope, requireVerified)
	if err != nil {
		return core.AuthContext{}, err
	}
	if auth.WorkspaceID != workspaceID {
		return core.AuthContext{}, core.ErrForbidden
	}
	return auth, nil
}

func isWorkspaceAdmin(auth core.AuthContext) bool {
	if auth.Credential == "api_key" {
		return hasScope(auth.Scopes, "workspace:admin")
	}
	return auth.Role == core.RoleOwner || auth.Role == core.RoleAdmin
}

func hasScope(scopes []string, wanted string) bool {
	for _, scope := range scopes {
		if scope == wanted {
			return true
		}
	}
	return false
}

func normalizeScopes(scopes []string) ([]string, error) {
	if len(scopes) == 0 {
		return nil, errors.New("at least one API key scope is required")
	}
	seen := make(map[string]bool)
	result := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		scope = strings.TrimSpace(scope)
		if !allowedAPIKeyScopes[scope] {
			return nil, fmt.Errorf("unsupported API key scope %q", scope)
		}
		if !seen[scope] {
			seen[scope] = true
			result = append(result, scope)
		}
	}
	sort.Strings(result)
	return result, nil
}

func actorAudit(auth core.AuthContext, action, resourceType, resourceID string, now time.Time, metadata map[string]string) core.AuditEntry {
	entry := core.AuditEntry{
		WorkspaceID:  auth.WorkspaceID,
		ActorUserID:  auth.UserID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		Metadata:     metadata,
		CreatedAt:    now,
	}
	if auth.APIKeyID != "" {
		entry.ActorAPIKeyID = auth.APIKeyID
	}
	return entry
}

func sessionSecretFromRequest(r *http.Request) (string, bool) {
	if authorization := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(authorization, "Bearer ") {
		secret := strings.TrimSpace(strings.TrimPrefix(authorization, "Bearer "))
		if strings.HasPrefix(secret, "qh8z_ss_") {
			return secret, true
		}
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err == nil && strings.HasPrefix(cookie.Value, "qh8z_ss_") {
		return cookie.Value, true
	}
	return "", false
}

func (a *app) setSessionCookie(w http.ResponseWriter, secret string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    secret,
		Path:     "/",
		HttpOnly: true,
		Secure:   a.secureCookies,
		SameSite: http.SameSiteLaxMode,
		Expires:  expires,
		MaxAge:   int(time.Until(expires).Seconds()),
	})
}

func normalizeEmail(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > 254 || strings.ContainsAny(raw, "\r\n") {
		return "", errors.New("invalid email address")
	}
	parsed, err := mail.ParseAddress(raw)
	if err != nil || parsed.Address != raw || strings.Count(raw, "@") != 1 {
		return "", errors.New("invalid email address")
	}
	parts := strings.SplitN(raw, "@", 2)
	if parts[0] == "" || parts[1] == "" {
		return "", errors.New("invalid email address")
	}
	return strings.ToLower(raw), nil
}

func normalizeWorkspaceName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		name = "My Workspace"
	}
	if len(name) < 2 || len(name) > maxWorkspaceNameBytes || strings.ContainsAny(name, "\r\n\x00") {
		return "", errors.New("workspace name must be 2-80 bytes")
	}
	return name, nil
}

func randomID(prefix string, n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(buf), nil
}

func randomToken(prefix string) (string, []byte, error) {
	secret, err := randomID(prefix, 32)
	if err != nil {
		return "", nil, err
	}
	return secret, secretHash(secret), nil
}

func secretHash(secret string) []byte {
	sum := sha256.Sum256([]byte(secret))
	return sum[:]
}

func (a *app) verificationURL(secret string) string {
	return a.baseURL + "/verify-email?token=" + url.QueryEscape(secret)
}

func (a *app) internalRandomError(w http.ResponseWriter, err error) {
	a.logger.Error("secure random generation failed", "error", err)
	writeError(w, http.StatusInternalServerError, "secure token generation failed")
}

func (a *app) writeAuthError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrUnauthorized):
		writeError(w, http.StatusUnauthorized, "authentication required")
	case errors.Is(err, core.ErrEmailUnverified):
		writeError(w, http.StatusForbidden, "verified email required")
	case errors.Is(err, core.ErrForbidden):
		writeError(w, http.StatusForbidden, "permission denied")
	default:
		a.writeStoreError(w, err)
	}
}

func (a *app) writeVerificationError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrExpired):
		writeError(w, http.StatusBadRequest, "verification token expired")
	case errors.Is(err, core.ErrUnauthorized):
		writeError(w, http.StatusBadRequest, "verification token invalid or already used")
	default:
		a.writeStoreError(w, err)
	}
}

func (a *app) writeStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, core.ErrNotFound):
		writeError(w, http.StatusNotFound, "resource not found")
	case errors.Is(err, core.ErrConflict):
		writeError(w, http.StatusConflict, "resource already exists")
	default:
		a.logger.Error("storage request failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "storage unavailable")
	}
}

func isUnsafeMethod(method string) bool {
	return method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions
}

func sameSiteRequest(r *http.Request, baseURL string) bool {
	if site := strings.ToLower(r.Header.Get("Sec-Fetch-Site")); site == "cross-site" {
		return false
	}
	origin := strings.TrimRight(r.Header.Get("Origin"), "/")
	if origin == "" {
		return true
	}
	return origin == strings.TrimRight(baseURL, "/")
}

func htmlEscape(value string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&#39;")
	return replacer.Replace(value)
}
