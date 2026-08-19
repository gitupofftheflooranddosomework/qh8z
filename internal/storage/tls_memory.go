package storage

import (
	"context"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

func (m *Memory) IsVerifiedCustomDomain(_ context.Context, host string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := commercialStateFor(m)
	for _, domain := range state.domains {
		if domain.Host != host || domain.VerifiedAt == nil {
			continue
		}
		billing, ok := state.billing[domain.WorkspaceID]
		if ok && billing.PlanCode == core.PlanPro && billing.Status != core.BillingStatusCanceled {
			return true, nil
		}
	}
	return false, nil
}
