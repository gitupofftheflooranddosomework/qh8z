package storage

import (
	"context"
	"sync"
	"time"
)

var memoryBillingEventClaims sync.Map

type memoryBillingEventKey struct {
	store   *Memory
	eventID string
}

func (m *Memory) ClaimBillingEvent(_ context.Context, eventID string, _ time.Time) (bool, error) {
	key := memoryBillingEventKey{store: m, eventID: eventID}
	_, loaded := memoryBillingEventClaims.LoadOrStore(key, struct{}{})
	return !loaded, nil
}
