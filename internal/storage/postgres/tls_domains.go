package postgres

import (
	"context"
	"fmt"
)

func (s *Store) IsVerifiedCustomDomain(ctx context.Context, host string) (bool, error) {
	var allowed bool
	if err := s.db.QueryRowContext(ctx, `
SELECT EXISTS (
    SELECT 1
    FROM custom_domains d
    JOIN workspace_billing wb ON wb.workspace_id = d.workspace_id
    WHERE d.host = $1
      AND d.verified_at IS NOT NULL
      AND wb.plan_code = 'pro'
      AND wb.status <> 'canceled'
)`, host).Scan(&allowed); err != nil {
		return false, fmt.Errorf("check verified custom domain: %w", err)
	}
	return allowed, nil
}
