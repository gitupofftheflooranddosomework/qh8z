package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

type rowScanner interface {
	Scan(...any) error
}

const fullLinkSelect = `
SELECT l.slug, l.destination_url, COALESCE(l.workspace_id, ''), COALESCE(l.created_by_user_id, ''),
       COALESCE(l.domain_id, ''), COALESCE(d.host, ''), l.created_at, l.updated_at, l.visit_count,
       l.disabled_at, l.suspended_at, l.suspension_reason
FROM links l
LEFT JOIN custom_domains d ON d.id = l.domain_id`

func scanFullLink(row rowScanner) (core.Link, error) {
	var link core.Link
	err := row.Scan(
		&link.Slug,
		&link.URL,
		&link.WorkspaceID,
		&link.CreatedByUserID,
		&link.DomainID,
		&link.DomainHost,
		&link.CreatedAt,
		&link.UpdatedAt,
		&link.Visits,
		&link.DisabledAt,
		&link.SuspendedAt,
		&link.SuspensionReason,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return core.Link{}, core.ErrNotFound
	}
	if err != nil {
		return core.Link{}, fmt.Errorf("scan link: %w", err)
	}
	return link, nil
}

func (s *Store) GetCustomDomainLink(ctx context.Context, host, slug string) (core.Link, error) {
	return scanFullLink(s.db.QueryRowContext(ctx, fullLinkSelect+`
WHERE d.host = $1 AND d.verified_at IS NOT NULL AND l.slug = $2`, host, slug))
}

func (s *Store) ListWorkspaceLinks(ctx context.Context, workspaceID string, limit, offset int) ([]core.Link, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	rows, err := s.db.QueryContext(ctx, fullLinkSelect+`
WHERE l.workspace_id = $1
ORDER BY l.created_at DESC, l.slug DESC
LIMIT $2 OFFSET $3`, workspaceID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list workspace links: %w", err)
	}
	defer rows.Close()
	links := make([]core.Link, 0, limit)
	for rows.Next() {
		link, err := scanFullLink(rows)
		if err != nil {
			return nil, err
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace links: %w", err)
	}
	return links, nil
}

func (s *Store) UpdateWorkspaceLink(ctx context.Context, workspaceID, slug, destinationURL, domainID string, updatedAt time.Time, audit core.AuditEntry) (core.Link, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return core.Link{}, fmt.Errorf("begin link update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	if domainID != "" {
		var exists bool
		if err := tx.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1 FROM custom_domains
    WHERE id = $1 AND workspace_id = $2 AND verified_at IS NOT NULL
)`, domainID, workspaceID).Scan(&exists); err != nil {
			return core.Link{}, fmt.Errorf("validate custom domain: %w", err)
		}
		if !exists {
			return core.Link{}, core.ErrNotFound
		}
	}
	result, err := tx.ExecContext(ctx, `
UPDATE links
SET destination_url = $3, domain_id = NULLIF($4, ''), updated_at = $5
WHERE workspace_id = $1 AND slug = $2`, workspaceID, slug, destinationURL, domainID, updatedAt)
	if err != nil {
		return core.Link{}, fmt.Errorf("update workspace link: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return core.Link{}, core.ErrNotFound
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return core.Link{}, err
	}
	link, err := scanFullLink(tx.QueryRowContext(ctx, fullLinkSelect+`
WHERE l.workspace_id = $1 AND l.slug = $2`, workspaceID, slug))
	if err != nil {
		return core.Link{}, err
	}
	if err := tx.Commit(); err != nil {
		return core.Link{}, fmt.Errorf("commit link update: %w", err)
	}
	return link, nil
}

func (s *Store) SetWorkspaceLinkDisabled(ctx context.Context, workspaceID, slug string, disabled bool, changedAt time.Time, audit core.AuditEntry) (core.Link, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return core.Link{}, fmt.Errorf("begin link state change: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var result sql.Result
	if disabled {
		result, err = tx.ExecContext(ctx, `
UPDATE links SET disabled_at = $3, updated_at = $3
WHERE workspace_id = $1 AND slug = $2`, workspaceID, slug, changedAt)
	} else {
		result, err = tx.ExecContext(ctx, `
UPDATE links SET disabled_at = NULL, updated_at = $3
WHERE workspace_id = $1 AND slug = $2`, workspaceID, slug, changedAt)
	}
	if err != nil {
		return core.Link{}, fmt.Errorf("change link disabled state: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return core.Link{}, core.ErrNotFound
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return core.Link{}, err
	}
	link, err := scanFullLink(tx.QueryRowContext(ctx, fullLinkSelect+`
WHERE l.workspace_id = $1 AND l.slug = $2`, workspaceID, slug))
	if err != nil {
		return core.Link{}, err
	}
	if err := tx.Commit(); err != nil {
		return core.Link{}, fmt.Errorf("commit link state change: %w", err)
	}
	return link, nil
}

func (s *Store) DeleteWorkspaceLink(ctx context.Context, workspaceID, slug string, audit core.AuditEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin link delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM links WHERE workspace_id = $1 AND slug = $2`, workspaceID, slug)
	if err != nil {
		return fmt.Errorf("delete workspace link: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return core.ErrNotFound
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit link delete: %w", err)
	}
	return nil
}

func (s *Store) CreateCustomDomain(ctx context.Context, domain core.CustomDomain, audit core.AuditEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin custom domain create: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
INSERT INTO custom_domains (id, workspace_id, host, verification_token, verified_at, created_at)
VALUES ($1, $2, $3, $4, $5, $6)`, domain.ID, domain.WorkspaceID, domain.Host, domain.VerificationToken, domain.VerifiedAt, domain.CreatedAt)
	if err != nil {
		if isUniqueViolation(err) {
			return core.ErrConflict
		}
		return fmt.Errorf("create custom domain: %w", err)
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit custom domain create: %w", err)
	}
	return nil
}

func (s *Store) ListCustomDomains(ctx context.Context, workspaceID string) ([]core.CustomDomain, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT id, workspace_id, host, verification_token, verified_at, created_at
FROM custom_domains
WHERE workspace_id = $1
ORDER BY created_at DESC, id DESC`, workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list custom domains: %w", err)
	}
	defer rows.Close()
	var domains []core.CustomDomain
	for rows.Next() {
		var domain core.CustomDomain
		if err := rows.Scan(&domain.ID, &domain.WorkspaceID, &domain.Host, &domain.VerificationToken, &domain.VerifiedAt, &domain.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan custom domain: %w", err)
		}
		domains = append(domains, domain)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate custom domains: %w", err)
	}
	return domains, nil
}

func (s *Store) GetCustomDomain(ctx context.Context, workspaceID, domainID string) (core.CustomDomain, error) {
	var domain core.CustomDomain
	err := s.db.QueryRowContext(ctx, `
SELECT id, workspace_id, host, verification_token, verified_at, created_at
FROM custom_domains
WHERE workspace_id = $1 AND id = $2`, workspaceID, domainID).Scan(
		&domain.ID, &domain.WorkspaceID, &domain.Host, &domain.VerificationToken, &domain.VerifiedAt, &domain.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return core.CustomDomain{}, core.ErrNotFound
	}
	if err != nil {
		return core.CustomDomain{}, fmt.Errorf("get custom domain: %w", err)
	}
	return domain, nil
}

func (s *Store) SetCustomDomainVerified(ctx context.Context, workspaceID, domainID string, verifiedAt time.Time, audit core.AuditEntry) (core.CustomDomain, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return core.CustomDomain{}, fmt.Errorf("begin domain verification: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	var domain core.CustomDomain
	err = tx.QueryRowContext(ctx, `
UPDATE custom_domains SET verified_at = $3
WHERE workspace_id = $1 AND id = $2
RETURNING id, workspace_id, host, verification_token, verified_at, created_at`, workspaceID, domainID, verifiedAt).Scan(
		&domain.ID, &domain.WorkspaceID, &domain.Host, &domain.VerificationToken, &domain.VerifiedAt, &domain.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return core.CustomDomain{}, core.ErrNotFound
	}
	if err != nil {
		return core.CustomDomain{}, fmt.Errorf("verify custom domain: %w", err)
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return core.CustomDomain{}, err
	}
	if err := tx.Commit(); err != nil {
		return core.CustomDomain{}, fmt.Errorf("commit domain verification: %w", err)
	}
	return domain, nil
}

func (s *Store) DeleteCustomDomain(ctx context.Context, workspaceID, domainID string, audit core.AuditEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin custom domain delete: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	result, err := tx.ExecContext(ctx, `DELETE FROM custom_domains WHERE workspace_id = $1 AND id = $2`, workspaceID, domainID)
	if err != nil {
		return fmt.Errorf("delete custom domain: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return core.ErrNotFound
	}
	if err := insertAudit(ctx, tx, audit); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit custom domain delete: %w", err)
	}
	return nil
}

func (s *Store) WorkspaceAnalytics(ctx context.Context, workspaceID string, from, to time.Time, topLimit int) (core.WorkspaceAnalytics, error) {
	if topLimit <= 0 || topLimit > 50 {
		topLimit = 10
	}
	analytics := core.WorkspaceAnalytics{From: from, To: to}
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*),
       COUNT(*) FILTER (WHERE disabled_at IS NULL AND suspended_at IS NULL),
       COALESCE(SUM(visit_count), 0)
FROM links WHERE workspace_id = $1`, workspaceID).Scan(&analytics.TotalLinks, &analytics.ActiveLinks, &analytics.TotalVisits); err != nil {
		return core.WorkspaceAnalytics{}, fmt.Errorf("read analytics totals: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `
SELECT COUNT(*)
FROM visits v
JOIN links l ON l.slug = v.slug
WHERE l.workspace_id = $1 AND v.visited_at >= $2 AND v.visited_at < $3`, workspaceID, from, to).Scan(&analytics.PeriodVisits); err != nil {
		return core.WorkspaceAnalytics{}, fmt.Errorf("read period visits: %w", err)
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT TO_CHAR((v.visited_at AT TIME ZONE 'UTC')::date, 'YYYY-MM-DD'), COUNT(*)
FROM visits v
JOIN links l ON l.slug = v.slug
WHERE l.workspace_id = $1 AND v.visited_at >= $2 AND v.visited_at < $3
GROUP BY 1 ORDER BY 1`, workspaceID, from, to)
	if err != nil {
		return core.WorkspaceAnalytics{}, fmt.Errorf("read analytics daily visits: %w", err)
	}
	for rows.Next() {
		var day core.AnalyticsDay
		if err := rows.Scan(&day.Date, &day.Visits); err != nil {
			rows.Close()
			return core.WorkspaceAnalytics{}, fmt.Errorf("scan analytics day: %w", err)
		}
		analytics.Daily = append(analytics.Daily, day)
	}
	if err := rows.Close(); err != nil {
		return core.WorkspaceAnalytics{}, fmt.Errorf("close analytics daily rows: %w", err)
	}
	rows, err = s.db.QueryContext(ctx, `
SELECT l.slug, l.destination_url, COALESCE(d.host, ''), COUNT(v.id)
FROM links l
LEFT JOIN custom_domains d ON d.id = l.domain_id
LEFT JOIN visits v ON v.slug = l.slug AND v.visited_at >= $2 AND v.visited_at < $3
WHERE l.workspace_id = $1
GROUP BY l.slug, l.destination_url, d.host
ORDER BY COUNT(v.id) DESC, l.slug
LIMIT $4`, workspaceID, from, to, topLimit)
	if err != nil {
		return core.WorkspaceAnalytics{}, fmt.Errorf("read top links: %w", err)
	}
	for rows.Next() {
		var item core.AnalyticsLink
		if err := rows.Scan(&item.Slug, &item.URL, &item.DomainHost, &item.Visits); err != nil {
			rows.Close()
			return core.WorkspaceAnalytics{}, fmt.Errorf("scan top link: %w", err)
		}
		analytics.TopLinks = append(analytics.TopLinks, item)
	}
	if err := rows.Close(); err != nil {
		return core.WorkspaceAnalytics{}, fmt.Errorf("close top links rows: %w", err)
	}
	rows, err = s.db.QueryContext(ctx, `
SELECT COALESCE(NULLIF(v.referer, ''), '(direct)'), COUNT(*)
FROM visits v
JOIN links l ON l.slug = v.slug
WHERE l.workspace_id = $1 AND v.visited_at >= $2 AND v.visited_at < $3
GROUP BY 1 ORDER BY COUNT(*) DESC, 1
LIMIT $4`, workspaceID, from, to, topLimit)
	if err != nil {
		return core.WorkspaceAnalytics{}, fmt.Errorf("read analytics referrers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var ref core.AnalyticsReferrer
		if err := rows.Scan(&ref.Referrer, &ref.Visits); err != nil {
			return core.WorkspaceAnalytics{}, fmt.Errorf("scan analytics referrer: %w", err)
		}
		analytics.Referrers = append(analytics.Referrers, ref)
	}
	if err := rows.Err(); err != nil {
		return core.WorkspaceAnalytics{}, fmt.Errorf("iterate analytics referrers: %w", err)
	}
	return analytics, nil
}

func (s *Store) WorkspaceUsage(ctx context.Context, workspaceID string, monthStart time.Time) (core.WorkspaceUsage, error) {
	usage := core.WorkspaceUsage{WorkspaceID: workspaceID, PlanCode: core.PlanFree}
	billing, err := s.GetBillingState(ctx, workspaceID)
	if err != nil {
		return core.WorkspaceUsage{}, err
	}
	usage.PlanCode = billing.PlanCode
	if err := s.db.QueryRowContext(ctx, `
SELECT (SELECT COUNT(*) FROM links WHERE workspace_id = $1),
       (SELECT COUNT(*) FROM custom_domains WHERE workspace_id = $1),
       (SELECT COUNT(*) FROM links WHERE workspace_id = $1 AND created_at >= $2)`, workspaceID, monthStart).Scan(
		&usage.Links, &usage.CustomDomains, &usage.LinksCreatedThisMonth,
	); err != nil {
		return core.WorkspaceUsage{}, fmt.Errorf("read workspace usage: %w", err)
	}
	return usage, nil
}

func (s *Store) GetBillingState(ctx context.Context, workspaceID string) (core.BillingState, error) {
	var state core.BillingState
	err := s.db.QueryRowContext(ctx, `
SELECT workspace_id, plan_code, status, provider_customer_id, provider_subscription_id, current_period_end, updated_at
FROM workspace_billing WHERE workspace_id = $1`, workspaceID).Scan(
		&state.WorkspaceID,
		&state.PlanCode,
		&state.Status,
		&state.ProviderCustomerID,
		&state.ProviderSubscriptionID,
		&state.CurrentPeriodEnd,
		&state.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return core.BillingState{WorkspaceID: workspaceID, PlanCode: core.PlanFree, Status: core.BillingStatusActive}, nil
	}
	if err != nil {
		return core.BillingState{}, fmt.Errorf("get billing state: %w", err)
	}
	return state, nil
}

func (s *Store) UpsertBillingState(ctx context.Context, state core.BillingState, audit core.AuditEntry) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin billing update: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	_, err = tx.ExecContext(ctx, `
INSERT INTO workspace_billing (
    workspace_id, plan_code, status, provider_customer_id, provider_subscription_id, current_period_end, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (workspace_id) DO UPDATE SET
    plan_code = EXCLUDED.plan_code,
    status = EXCLUDED.status,
    provider_customer_id = EXCLUDED.provider_customer_id,
    provider_subscription_id = EXCLUDED.provider_subscription_id,
    current_period_end = EXCLUDED.current_period_end,
    updated_at = EXCLUDED.updated_at`,
		state.WorkspaceID,
		state.PlanCode,
		state.Status,
		state.ProviderCustomerID,
		state.ProviderSubscriptionID,
		state.CurrentPeriodEnd,
		state.UpdatedAt,
	)
	if err != nil {
		return fmt.Errorf("upsert billing state: %w", err)
	}
	if audit.Action != "" {
		if err := insertAudit(ctx, tx, audit); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit billing update: %w", err)
	}
	return nil
}
