package storage

import "context"

func (m *Memory) IsVerifiedCustomDomain(_ context.Context, host string) (bool, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, domain := range commercialStateFor(m).domains {
		if domain.Host == host && domain.VerifiedAt != nil {
			return true, nil
		}
	}
	return false, nil
}
