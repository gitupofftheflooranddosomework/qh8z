package storage

import (
	"context"
	"sync"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

type Memory struct {
	mu     sync.RWMutex
	links  map[string]core.Link
	visits map[string][]core.Visit
}

func NewMemory() *Memory {
	return &Memory{
		links:  make(map[string]core.Link),
		visits: make(map[string][]core.Visit),
	}
}

func (m *Memory) CreateLink(_ context.Context, link core.Link) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.links[link.Slug]; exists {
		return core.ErrConflict
	}
	m.links[link.Slug] = link
	return nil
}

func (m *Memory) GetLink(_ context.Context, slug string) (core.Link, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	link, ok := m.links[slug]
	if !ok {
		return core.Link{}, core.ErrNotFound
	}
	return link, nil
}

func (m *Memory) RecordVisit(_ context.Context, visit core.Visit) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	link, ok := m.links[visit.Slug]
	if !ok {
		return 0, core.ErrNotFound
	}
	link.Visits++
	m.links[visit.Slug] = link
	m.visits[visit.Slug] = append(m.visits[visit.Slug], visit)
	return link.Visits, nil
}

func (m *Memory) Stats(_ context.Context, slug string, recentLimit int) (core.Stats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	link, ok := m.links[slug]
	if !ok {
		return core.Stats{}, core.ErrNotFound
	}
	visits := m.visits[slug]
	if recentLimit < 0 {
		recentLimit = 0
	}
	if recentLimit > len(visits) {
		recentLimit = len(visits)
	}
	recent := make([]core.Visit, 0, recentLimit)
	for i := len(visits) - 1; i >= len(visits)-recentLimit; i-- {
		recent = append(recent, visits[i])
	}
	return core.Stats{TotalVisits: link.Visits, Recent: recent}, nil
}

func (m *Memory) Ping(context.Context) error { return nil }
func (m *Memory) Close() error               { return nil }
