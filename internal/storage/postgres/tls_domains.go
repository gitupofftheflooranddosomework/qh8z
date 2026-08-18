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
    FROM custom_domains
    WHERE host = $1 AND verified_at IS NOT NULL
)`, host).Scan(&allowed); err != nil {
		return false, fmt.Errorf("check verified custom domain: %w", err)
	}
	return allowed, nil
}
