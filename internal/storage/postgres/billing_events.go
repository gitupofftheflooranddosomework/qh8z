package postgres

import (
	"context"
	"fmt"
	"time"
)

func (s *Store) ClaimBillingEvent(ctx context.Context, eventID string, processedAt time.Time) (bool, error) {
	result, err := s.db.ExecContext(ctx, `
INSERT INTO billing_webhook_events (event_id, processed_at)
VALUES ($1, $2)
ON CONFLICT (event_id) DO NOTHING`, eventID, processedAt)
	if err != nil {
		return false, fmt.Errorf("claim billing webhook event: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, fmt.Errorf("read billing webhook claim result: %w", err)
	}
	return rows == 1, nil
}
