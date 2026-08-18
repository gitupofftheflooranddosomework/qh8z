package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

type execContext interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}

func (s *Store) Register(ctx context.Context, reg core.Registration) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin registration: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `
INSERT INTO users (id, email, password_hash, email_verified_at, created_at)
VALUES ($1, $2, $3, $4, $5)`, reg.User.ID, reg.User.Email, reg.User.PasswordHash, reg.User.EmailVerifiedAt, reg.User.CreatedAt); err != nil {
		if isUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("create user: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspaces (id, name, created_at) VALUES ($1, $2, $3)`,
		reg.Workspace.ID, reg.Workspace.Name, reg.Workspace.CreatedAt); err != nil {
		return fmt.Errorf("create workspace: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
VALUES ($1, $2, $3, $4)`, reg.Membership.WorkspaceID, reg.Membership.UserID, reg.Membership.Role, reg.Membership.CreatedAt); err != nil {
		return fmt.Errorf("create workspace membership: %w", err)
	}
	if err := insertVerification(ctx, tx, reg.Verification); err != nil {
		return err
	}
	if err := insertSession(ctx, tx, reg.Session); err != nil {
		return err
	}
	for _, entry := range reg.Audit {
		if err := insertAudit(ctx, tx, entry); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit registration: %w", err)
	}
	return nil
}

func (s *Store) UserByEmail(ctx context.Context, email string) (core.User, error) {
	var user core.User
	var verified sql.NullTime
	err := s.db.QueryRowContext(ctx, `
SELECT id, email, password_hash, email_verified_at, created_at
FROM users WHERE email = $1`, email).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &verified, &user.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return core.User{}, core.ErrNotFound
	}
	if err != nil {
		return core.User{}, fmt.Errorf("get user by email: %w", err)
	}
	if verified.Valid {
		user.EmailVerifiedAt = &verified.Time
	}
	return user, nil
}

func (s *Store) CreateSession(ctx context.Context, session core.Session, audit core.AuditEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin session: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if err := insertSession(ctx, tx, session); err != nil {
		return err
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit session: %w", err)
	}
	return nil
}

func insertSession(ctx context.Context, db execContext, session core.Session) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO sessions (token_hash, user_id, created_at, expires_at, last_seen_at)
VALUES ($1, $2, $3, $4, $5)`,
		session.TokenHash, session.UserID, session.CreatedAt, session.ExpiresAt, session.LastSeenAt,
	)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash []byte) error {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = $1`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}

func (s *Store) ResolveSession(ctx context.Context, tokenHash []byte, workspaceHint string, now time.Time) (core.AuthContext, error) {
	var auth core.AuthContext
	var verified sql.NullTime
	err := s.db.QueryRowContext(ctx, `
SELECT u.id, u.email, u.email_verified_at, wm.workspace_id, wm.role
FROM sessions s
JOIN users u ON u.id = s.user_id
JOIN workspace_members wm ON wm.user_id = u.id
JOIN workspaces w ON w.id = wm.workspace_id
WHERE s.token_hash = $1
  AND s.expires_at > $2
  AND ($3 = '' OR wm.workspace_id = $3)
ORDER BY w.created_at, w.id
LIMIT 1`, tokenHash, now, workspaceHint).Scan(
		&auth.UserID, &auth.Email, &verified, &auth.WorkspaceID, &auth.Role,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return core.AuthContext{}, core.ErrUnauthorized
	}
	if err != nil {
		return core.AuthContext{}, fmt.Errorf("resolve session: %w", err)
	}
	auth.EmailVerified = verified.Valid
	auth.Credential = "session"
	_, _ = s.db.ExecContext(ctx, `UPDATE sessions SET last_seen_at = $2 WHERE token_hash = $1`, tokenHash, now)
	return auth, nil
}

func (s *Store) CreateEmailVerification(ctx context.Context, verification core.EmailVerification) error {
	return insertVerification(ctx, s.db, verification)
}

func insertVerification(ctx context.Context, db execContext, verification core.EmailVerification) error {
	_, err := db.ExecContext(ctx, `
INSERT INTO email_verification_tokens (token_hash, user_id, created_at, expires_at, used_at)
VALUES ($1, $2, $3, $4, $5)`,
		verification.TokenHash, verification.UserID, verification.CreatedAt, verification.ExpiresAt, verification.UsedAt,
	)
	if err != nil {
		return fmt.Errorf("create email verification token: %w", err)
	}
	return nil
}

func (s *Store) ConsumeEmailVerification(ctx context.Context, tokenHash []byte, now time.Time) (core.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return core.User{}, fmt.Errorf("begin email verification: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var userID string
	var expiresAt time.Time
	var usedAt sql.NullTime
	err = tx.QueryRowContext(ctx, `
SELECT user_id, expires_at, used_at
FROM email_verification_tokens
WHERE token_hash = $1
FOR UPDATE`, tokenHash).Scan(&userID, &expiresAt, &usedAt)
	if errors.Is(err, sql.ErrNoRows) || usedAt.Valid {
		return core.User{}, core.ErrUnauthorized
	}
	if err != nil {
		return core.User{}, fmt.Errorf("read email verification token: %w", err)
	}
	if !expiresAt.After(now) {
		return core.User{}, core.ErrExpired
	}
	if _, err := tx.ExecContext(ctx, `UPDATE email_verification_tokens SET used_at = $2 WHERE token_hash = $1`, tokenHash, now); err != nil {
		return core.User{}, fmt.Errorf("consume email verification token: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `UPDATE users SET email_verified_at = COALESCE(email_verified_at, $2) WHERE id = $1`, userID, now); err != nil {
		return core.User{}, fmt.Errorf("verify user email: %w", err)
	}

	var user core.User
	var verified sql.NullTime
	if err := tx.QueryRowContext(ctx, `SELECT id, email, password_hash, email_verified_at, created_at FROM users WHERE id = $1`, userID).Scan(
		&user.ID, &user.Email, &user.PasswordHash, &verified, &user.CreatedAt,
	); err != nil {
		return core.User{}, fmt.Errorf("read verified user: %w", err)
	}
	if verified.Valid {
		user.EmailVerifiedAt = &verified.Time
	}
	if err := insertAudit(ctx, tx, core.AuditEntry{
		ActorUserID:  user.ID,
		Action:       "user.email_verified",
		ResourceType: "user",
		ResourceID:   user.ID,
		CreatedAt:    now,
	}); err != nil {
		return core.User{}, err
	}
	if err := tx.Commit(); err != nil {
		return core.User{}, fmt.Errorf("commit email verification: %w", err)
	}
	return user, nil
}

func (s *Store) CreateWorkspace(ctx context.Context, workspace core.Workspace, ownerID string, audit core.AuditEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workspace create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `INSERT INTO workspaces (id, name, created_at) VALUES ($1, $2, $3)`,
		workspace.ID, workspace.Name, workspace.CreatedAt); err != nil {
		if isUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("create workspace: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
VALUES ($1, $2, $3, $4)`, workspace.ID, ownerID, core.RoleOwner, workspace.CreatedAt); err != nil {
		return fmt.Errorf("create workspace owner: %w", err)
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workspace create: %w", err)
	}
	return nil
}

func (s *Store) ListWorkspaces(ctx context.Context, userID string) ([]core.Workspace, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT w.id, w.name, w.created_at
FROM workspaces w
JOIN workspace_members wm ON wm.workspace_id = w.id
WHERE wm.user_id = $1
ORDER BY w.created_at, w.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list workspaces: %w", err)
	}
	defer rows.Close()
	var workspaces []core.Workspace
	for rows.Next() {
		var workspace core.Workspace
		if err := rows.Scan(&workspace.ID, &workspace.Name, &workspace.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan workspace: %w", err)
		}
		workspaces = append(workspaces, workspace)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspaces: %w", err)
	}
	return workspaces, nil
}

func (s *Store) AddWorkspaceMember(ctx context.Context, membership core.Membership, audit core.AuditEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin add workspace member: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO workspace_members (workspace_id, user_id, role, created_at)
VALUES ($1, $2, $3, $4)`, membership.WorkspaceID, membership.UserID, membership.Role, membership.CreatedAt); err != nil {
		if isUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("add workspace member: %w", err)
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit add workspace member: %w", err)
	}
	return nil
}

func (s *Store) ListWorkspaceMembers(ctx context.Context, workspaceID string) ([]core.Membership, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT wm.workspace_id, wm.user_id, u.email, wm.role, wm.created_at
FROM workspace_members wm
JOIN users u ON u.id = wm.user_id
WHERE wm.workspace_id = $1
ORDER BY wm.created_at, wm.user_id`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list workspace members: %w", err)
	}
	defer rows.Close()
	var members []core.Membership
	for rows.Next() {
		var membership core.Membership
		if err := rows.Scan(&membership.WorkspaceID, &membership.UserID, &membership.Email, &membership.Role, &membership.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan workspace member: %w", err)
		}
		members = append(members, membership)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace members: %w", err)
	}
	if len(members) == 0 {
		var exists bool
		if err := s.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM workspaces WHERE id = $1)`, workspaceID).Scan(&exists); err != nil {
			return nil, fmt.Errorf("check workspace: %w", err)
		}
		if !exists {
			return nil, core.ErrNotFound
		}
	}
	return members, nil
}

func (s *Store) CreateAPIKey(ctx context.Context, key core.APIKey, audit core.AuditEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin API key create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.ExecContext(ctx, `
INSERT INTO api_keys (id, workspace_id, name, key_hash, scopes, created_by_user_id, created_at, last_used_at, revoked_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		key.ID, key.WorkspaceID, key.Name, key.KeyHash, encodeScopes(key.Scopes), key.CreatedByUserID, key.CreatedAt, key.LastUsedAt, key.RevokedAt,
	); err != nil {
		if isUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("create API key: %w", err)
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit API key create: %w", err)
	}
	return nil
}

func (s *Store) ResolveAPIKey(ctx context.Context, keyHash []byte, now time.Time) (core.AuthContext, error) {
	var auth core.AuthContext
	var scopes string
	err := s.db.QueryRowContext(ctx, `
SELECT k.id, k.workspace_id, k.created_by_user_id, u.email, wm.role, k.scopes
FROM api_keys k
JOIN users u ON u.id = k.created_by_user_id
LEFT JOIN workspace_members wm ON wm.workspace_id = k.workspace_id AND wm.user_id = k.created_by_user_id
WHERE k.key_hash = $1 AND k.revoked_at IS NULL`, keyHash).Scan(
		&auth.APIKeyID, &auth.WorkspaceID, &auth.UserID, &auth.Email, &auth.Role, &scopes,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return core.AuthContext{}, core.ErrUnauthorized
	}
	if err != nil {
		return core.AuthContext{}, fmt.Errorf("resolve API key: %w", err)
	}
	auth.EmailVerified = true
	auth.Credential = "api_key"
	auth.Scopes = decodeScopes(scopes)
	_, _ = s.db.ExecContext(ctx, `UPDATE api_keys SET last_used_at = $2 WHERE key_hash = $1`, keyHash, now)
	return auth, nil
}

func encodeScopes(scopes []string) string {
	return strings.Join(scopes, ",")
}

func decodeScopes(encoded string) []string {
	if encoded == "" {
		return nil
	}
	return strings.Split(encoded, ",")
}

func (s *Store) WriteAudit(ctx context.Context, entry core.AuditEntry) error {
	return insertAudit(ctx, s.db, entry)
}

func insertAudit(ctx context.Context, db execContext, entry core.AuditEntry) error {
	metadata := entry.Metadata
	if metadata == nil {
		metadata = map[string]string{}
	}
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("encode audit metadata: %w", err)
	}
	_, err = db.ExecContext(ctx, `
INSERT INTO audit_log (workspace_id, actor_user_id, actor_api_key_id, action, resource_type, resource_id, metadata, created_at)
VALUES (NULLIF($1, ''), NULLIF($2, ''), NULLIF($3, ''), $4, $5, $6, $7, $8)`,
		entry.WorkspaceID, entry.ActorUserID, entry.ActorAPIKeyID, entry.Action, entry.ResourceType, entry.ResourceID, string(encoded), entry.CreatedAt,
	)
	if err != nil {
		return fmt.Errorf("write audit entry: %w", err)
	}
	return nil
}

func (s *Store) ListAudit(ctx context.Context, workspaceID string, limit int) ([]core.AuditEntry, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, COALESCE(workspace_id, ''), COALESCE(actor_user_id, ''), COALESCE(actor_api_key_id, ''),
       action, resource_type, resource_id, metadata, created_at
FROM audit_log
WHERE workspace_id = $1
ORDER BY created_at DESC, id DESC
LIMIT $2`, workspaceID, limit)
	if err != nil {
		return nil, fmt.Errorf("list audit entries: %w", err)
	}
	defer rows.Close()
	var entries []core.AuditEntry
	for rows.Next() {
		var entry core.AuditEntry
		var metadata []byte
		if err := rows.Scan(&entry.ID, &entry.WorkspaceID, &entry.ActorUserID, &entry.ActorAPIKeyID,
			&entry.Action, &entry.ResourceType, &entry.ResourceID, &metadata, &entry.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan audit entry: %w", err)
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &entry.Metadata); err != nil {
				return nil, fmt.Errorf("decode audit metadata: %w", err)
			}
		}
		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate audit entries: %w", err)
	}
	return entries, nil
}
