package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

func (s *Store) CheckRateLimit(ctx context.Context, bucketKey string, windowStart, resetAt time.Time, limit int) (core.RateLimitResult, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `
INSERT INTO rate_limit_windows (bucket_key, window_start, request_count, reset_at)
VALUES ($1, $2, 1, $3)
ON CONFLICT (bucket_key, window_start)
DO UPDATE SET request_count = rate_limit_windows.request_count + 1, reset_at = EXCLUDED.reset_at
RETURNING request_count`, bucketKey, windowStart, resetAt).Scan(&count)
	if err != nil {
		return core.RateLimitResult{}, fmt.Errorf("check rate limit: %w", err)
	}
	nowUnix := time.Now().Unix()
	lastCleanup := s.lastRateCleanup.Load()
	if nowUnix-lastCleanup >= int64((5*time.Minute)/time.Second) && s.lastRateCleanup.CompareAndSwap(lastCleanup, nowUnix) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM rate_limit_windows WHERE reset_at < $1`, time.Now().UTC().Add(-5*time.Minute))
	}
	remaining := limit - count
	if remaining < 0 {
		remaining = 0
	}
	return core.RateLimitResult{
		Allowed:   count <= limit,
		Limit:     limit,
		Remaining: remaining,
		ResetAt:   resetAt,
	}, nil
}

func (s *Store) MatchURLRule(ctx context.Context, host string) (core.URLRule, error) {
	var rule core.URLRule
	err := s.db.QueryRowContext(ctx, `
SELECT id, action, match_type, pattern, reason, created_at
FROM url_rules
WHERE (match_type = 'host' AND pattern = $1)
   OR (match_type = 'domain' AND ($1 = pattern OR $1 LIKE '%.' || pattern))
ORDER BY length(pattern) DESC, id DESC
LIMIT 1`, host).Scan(&rule.ID, &rule.Action, &rule.MatchType, &rule.Pattern, &rule.Reason, &rule.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return core.URLRule{}, core.ErrNotFound
	}
	if err != nil {
		return core.URLRule{}, fmt.Errorf("match URL rule: %w", err)
	}
	return rule, nil
}

func (s *Store) CreateURLRule(ctx context.Context, rule core.URLRule) (core.URLRule, error) {
	err := s.db.QueryRowContext(ctx, `
INSERT INTO url_rules (action, match_type, pattern, reason, created_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id`, rule.Action, rule.MatchType, rule.Pattern, rule.Reason, rule.CreatedAt).Scan(&rule.ID)
	if err != nil {
		if isUniqueViolation(err) {
			return core.URLRule{}, core.ErrConflict
		}
		return core.URLRule{}, fmt.Errorf("create URL rule: %w", err)
	}
	return rule, nil
}

func (s *Store) ListURLRules(ctx context.Context) ([]core.URLRule, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, action, match_type, pattern, reason, created_at
FROM url_rules
ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list URL rules: %w", err)
	}
	defer rows.Close()
	var rules []core.URLRule
	for rows.Next() {
		var rule core.URLRule
		if err := rows.Scan(&rule.ID, &rule.Action, &rule.MatchType, &rule.Pattern, &rule.Reason, &rule.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan URL rule: %w", err)
		}
		rules = append(rules, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate URL rules: %w", err)
	}
	return rules, nil
}

func (s *Store) DeleteURLRule(ctx context.Context, id int64) error {
	result, err := s.db.ExecContext(ctx, `DELETE FROM url_rules WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete URL rule: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read URL rule delete result: %w", err)
	}
	if rows == 0 {
		return core.ErrNotFound
	}
	return nil
}

func (s *Store) CreateAbuseReport(ctx context.Context, report core.AbuseReport) error {
	_, err := s.db.ExecContext(ctx, `
INSERT INTO abuse_reports (id, slug, destination_url, category, details, reporter_email, status, review_notes, created_at, reviewed_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		report.ID, report.Slug, report.DestinationURL, report.Category, report.Details, report.ReporterEmail,
		report.Status, report.ReviewNotes, report.CreatedAt, report.ReviewedAt,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("create abuse report: %w", err)
	}
	return nil
}

func (s *Store) ListAbuseReports(ctx context.Context, status string, limit int) ([]core.AbuseReport, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT id, slug, destination_url, category, details, reporter_email, status, review_notes, created_at, reviewed_at
FROM abuse_reports
WHERE ($1 = '' OR status = $1)
ORDER BY created_at DESC, id DESC
LIMIT $2`, status, limit)
	if err != nil {
		return nil, fmt.Errorf("list abuse reports: %w", err)
	}
	defer rows.Close()
	var reports []core.AbuseReport
	for rows.Next() {
		var report core.AbuseReport
		if err := rows.Scan(&report.ID, &report.Slug, &report.DestinationURL, &report.Category, &report.Details,
			&report.ReporterEmail, &report.Status, &report.ReviewNotes, &report.CreatedAt, &report.ReviewedAt); err != nil {
			return nil, fmt.Errorf("scan abuse report: %w", err)
		}
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate abuse reports: %w", err)
	}
	return reports, nil
}

func (s *Store) UpdateAbuseReport(ctx context.Context, id, status, notes string, now time.Time) (core.AbuseReport, error) {
	var report core.AbuseReport
	err := s.db.QueryRowContext(ctx, `
UPDATE abuse_reports
SET status = $2, review_notes = $3, reviewed_at = $4
WHERE id = $1
RETURNING id, slug, destination_url, category, details, reporter_email, status, review_notes, created_at, reviewed_at`,
		id, status, notes, now).Scan(&report.ID, &report.Slug, &report.DestinationURL, &report.Category, &report.Details,
		&report.ReporterEmail, &report.Status, &report.ReviewNotes, &report.CreatedAt, &report.ReviewedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return core.AbuseReport{}, core.ErrNotFound
	}
	if err != nil {
		return core.AbuseReport{}, fmt.Errorf("update abuse report: %w", err)
	}
	return report, nil
}

func (s *Store) SetLinkSuspension(ctx context.Context, slug string, suspend bool, reason string, now time.Time, audit core.AuditEntry) (core.Link, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return core.Link{}, fmt.Errorf("begin link suspension: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var suspendedAt any
	if suspend {
		suspendedAt = now
	} else {
		suspendedAt = nil
		reason = ""
	}
	var link core.Link
	err = tx.QueryRowContext(ctx, `
UPDATE links
SET suspended_at = $2, suspension_reason = $3
WHERE slug = $1
RETURNING slug, destination_url, COALESCE(workspace_id, ''), COALESCE(created_by_user_id, ''), created_at, visit_count, suspended_at, suspension_reason`,
		slug, suspendedAt, reason).Scan(&link.Slug, &link.URL, &link.WorkspaceID, &link.CreatedByUserID,
		&link.CreatedAt, &link.Visits, &link.SuspendedAt, &link.SuspensionReason)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Link{}, core.ErrNotFound
	}
	if err != nil {
		return core.Link{}, fmt.Errorf("update link suspension: %w", err)
	}
	if audit.WorkspaceID == "" {
		audit.WorkspaceID = link.WorkspaceID
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return core.Link{}, err
	}
	if err := tx.Commit(); err != nil {
		return core.Link{}, fmt.Errorf("commit link suspension: %w", err)
	}
	return link, nil
}
