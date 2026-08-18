package storage

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

type memoryCommercialState struct {
	domains map[string]core.CustomDomain
	billing map[string]core.BillingState
}

var memoryCommercialStates sync.Map

func commercialStateFor(m *Memory) *memoryCommercialState {
	if state, ok := memoryCommercialStates.Load(m); ok {
		return state.(*memoryCommercialState)
	}
	state := &memoryCommercialState{
		domains: make(map[string]core.CustomDomain),
		billing: make(map[string]core.BillingState),
	}
	actual, _ := memoryCommercialStates.LoadOrStore(m, state)
	return actual.(*memoryCommercialState)
}

func (m *Memory) GetCustomDomainLink(_ context.Context, host, slug string) (core.Link, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := commercialStateFor(m)
	link, ok := m.links[slug]
	if !ok || link.DomainID == "" {
		return core.Link{}, core.ErrNotFound
	}
	domain, ok := state.domains[link.DomainID]
	if !ok || domain.VerifiedAt == nil || domain.Host != host {
		return core.Link{}, core.ErrNotFound
	}
	link.DomainHost = domain.Host
	return link, nil
}

func (m *Memory) ListWorkspaceLinks(_ context.Context, workspaceID string, limit, offset int) ([]core.Link, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	state := commercialStateFor(m)
	var links []core.Link
	for _, link := range m.links {
		if link.WorkspaceID != workspaceID {
			continue
		}
		if domain, ok := state.domains[link.DomainID]; ok {
			link.DomainHost = domain.Host
		}
		links = append(links, link)
	}
	sort.Slice(links, func(i, j int) bool {
		if links[i].CreatedAt.Equal(links[j].CreatedAt) {
			return links[i].Slug > links[j].Slug
		}
		return links[i].CreatedAt.After(links[j].CreatedAt)
	})
	if offset >= len(links) {
		return []core.Link{}, nil
	}
	end := offset + limit
	if end > len(links) {
		end = len(links)
	}
	return append([]core.Link(nil), links[offset:end]...), nil
}

func (m *Memory) UpdateWorkspaceLink(_ context.Context, workspaceID, slug, destinationURL, domainID string, updatedAt time.Time, audit core.AuditEntry) (core.Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := commercialStateFor(m)
	link, ok := m.links[slug]
	if !ok || link.WorkspaceID != workspaceID {
		return core.Link{}, core.ErrNotFound
	}
	link.DomainHost = ""
	if domainID != "" {
		domain, ok := state.domains[domainID]
		if !ok || domain.WorkspaceID != workspaceID || domain.VerifiedAt == nil {
			return core.Link{}, core.ErrNotFound
		}
		link.DomainHost = domain.Host
	}
	link.URL = destinationURL
	link.DomainID = domainID
	link.UpdatedAt = updatedAt
	m.links[slug] = link
	m.appendAuditLocked(audit)
	return link, nil
}

func (m *Memory) SetWorkspaceLinkDisabled(_ context.Context, workspaceID, slug string, disabled bool, changedAt time.Time, audit core.AuditEntry) (core.Link, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	link, ok := m.links[slug]
	if !ok || link.WorkspaceID != workspaceID {
		return core.Link{}, core.ErrNotFound
	}
	if disabled {
		link.DisabledAt = &changedAt
	} else {
		link.DisabledAt = nil
	}
	link.UpdatedAt = changedAt
	m.links[slug] = link
	m.appendAuditLocked(audit)
	return link, nil
}

func (m *Memory) DeleteWorkspaceLink(_ context.Context, workspaceID, slug string, audit core.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	link, ok := m.links[slug]
	if !ok || link.WorkspaceID != workspaceID {
		return core.ErrNotFound
	}
	delete(m.links, slug)
	delete(m.visits, slug)
	m.appendAuditLocked(audit)
	return nil
}

func (m *Memory) CreateCustomDomain(_ context.Context, domain core.CustomDomain, audit core.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.workspaces[domain.WorkspaceID]; !ok {
		return core.ErrNotFound
	}
	state := commercialStateFor(m)
	for _, existing := range state.domains {
		if existing.Host == domain.Host {
			return core.ErrConflict
		}
	}
	if _, exists := state.domains[domain.ID]; exists {
		return core.ErrConflict
	}
	state.domains[domain.ID] = domain
	m.appendAuditLocked(audit)
	return nil
}

func (m *Memory) ListCustomDomains(_ context.Context, workspaceID string) ([]core.CustomDomain, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := commercialStateFor(m)
	var domains []core.CustomDomain
	for _, domain := range state.domains {
		if domain.WorkspaceID == workspaceID {
			domains = append(domains, domain)
		}
	}
	sort.Slice(domains, func(i, j int) bool {
		if domains[i].CreatedAt.Equal(domains[j].CreatedAt) {
			return domains[i].ID > domains[j].ID
		}
		return domains[i].CreatedAt.After(domains[j].CreatedAt)
	})
	return domains, nil
}

func (m *Memory) GetCustomDomain(_ context.Context, workspaceID, domainID string) (core.CustomDomain, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	domain, ok := commercialStateFor(m).domains[domainID]
	if !ok || domain.WorkspaceID != workspaceID {
		return core.CustomDomain{}, core.ErrNotFound
	}
	return domain, nil
}

func (m *Memory) SetCustomDomainVerified(_ context.Context, workspaceID, domainID string, verifiedAt time.Time, audit core.AuditEntry) (core.CustomDomain, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := commercialStateFor(m)
	domain, ok := state.domains[domainID]
	if !ok || domain.WorkspaceID != workspaceID {
		return core.CustomDomain{}, core.ErrNotFound
	}
	domain.VerifiedAt = &verifiedAt
	state.domains[domainID] = domain
	m.appendAuditLocked(audit)
	return domain, nil
}

func (m *Memory) DeleteCustomDomain(_ context.Context, workspaceID, domainID string, audit core.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	state := commercialStateFor(m)
	domain, ok := state.domains[domainID]
	if !ok || domain.WorkspaceID != workspaceID {
		return core.ErrNotFound
	}
	delete(state.domains, domainID)
	for slug, link := range m.links {
		if link.DomainID == domainID {
			link.DomainID = ""
			link.DomainHost = ""
			m.links[slug] = link
		}
	}
	m.appendAuditLocked(audit)
	return nil
}

func (m *Memory) WorkspaceAnalytics(_ context.Context, workspaceID string, from, to time.Time, topLimit int) (core.WorkspaceAnalytics, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if topLimit <= 0 || topLimit > 50 {
		topLimit = 10
	}
	state := commercialStateFor(m)
	analytics := core.WorkspaceAnalytics{From: from, To: to}
	daily := make(map[string]int64)
	referrers := make(map[string]int64)
	var top []core.AnalyticsLink
	for _, link := range m.links {
		if link.WorkspaceID != workspaceID {
			continue
		}
		analytics.TotalLinks++
		analytics.TotalVisits += link.Visits
		if link.DisabledAt == nil && link.SuspendedAt == nil {
			analytics.ActiveLinks++
		}
		item := core.AnalyticsLink{Slug: link.Slug, URL: link.URL}
		if domain, ok := state.domains[link.DomainID]; ok {
			item.DomainHost = domain.Host
		}
		for _, visit := range m.visits[link.Slug] {
			if visit.VisitedAt.Before(from) || !visit.VisitedAt.Before(to) {
				continue
			}
			analytics.PeriodVisits++
			item.Visits++
			date := visit.VisitedAt.UTC().Format("2006-01-02")
			daily[date]++
			referrer := visit.Referer
			if referrer == "" {
				referrer = "(direct)"
			}
			referrers[referrer]++
		}
		top = append(top, item)
	}
	for date, count := range daily {
		analytics.Daily = append(analytics.Daily, core.AnalyticsDay{Date: date, Visits: count})
	}
	sort.Slice(analytics.Daily, func(i, j int) bool { return analytics.Daily[i].Date < analytics.Daily[j].Date })
	sort.Slice(top, func(i, j int) bool {
		if top[i].Visits == top[j].Visits {
			return top[i].Slug < top[j].Slug
		}
		return top[i].Visits > top[j].Visits
	})
	if len(top) > topLimit {
		top = top[:topLimit]
	}
	analytics.TopLinks = top
	for referrer, count := range referrers {
		analytics.Referrers = append(analytics.Referrers, core.AnalyticsReferrer{Referrer: referrer, Visits: count})
	}
	sort.Slice(analytics.Referrers, func(i, j int) bool {
		if analytics.Referrers[i].Visits == analytics.Referrers[j].Visits {
			return analytics.Referrers[i].Referrer < analytics.Referrers[j].Referrer
		}
		return analytics.Referrers[i].Visits > analytics.Referrers[j].Visits
	})
	if len(analytics.Referrers) > topLimit {
		analytics.Referrers = analytics.Referrers[:topLimit]
	}
	return analytics, nil
}

func (m *Memory) WorkspaceUsage(ctx context.Context, workspaceID string, monthStart time.Time) (core.WorkspaceUsage, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	state := commercialStateFor(m)
	usage := core.WorkspaceUsage{WorkspaceID: workspaceID, PlanCode: core.PlanFree}
	if billing, ok := state.billing[workspaceID]; ok {
		usage.PlanCode = billing.PlanCode
	}
	for _, link := range m.links {
		if link.WorkspaceID == workspaceID {
			usage.Links++
			if !link.CreatedAt.Before(monthStart) {
				usage.LinksCreatedThisMonth++
			}
		}
	}
	for _, domain := range state.domains {
		if domain.WorkspaceID == workspaceID {
			usage.CustomDomains++
		}
	}
	return usage, nil
}

func (m *Memory) GetBillingState(_ context.Context, workspaceID string) (core.BillingState, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if state, ok := commercialStateFor(m).billing[workspaceID]; ok {
		return state, nil
	}
	return core.BillingState{WorkspaceID: workspaceID, PlanCode: core.PlanFree, Status: core.BillingStatusActive}, nil
}

func (m *Memory) UpsertBillingState(_ context.Context, state core.BillingState, audit core.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.workspaces[state.WorkspaceID]; !ok {
		return core.ErrNotFound
	}
	commercialStateFor(m).billing[state.WorkspaceID] = state
	if audit.Action != "" {
		m.appendAuditLocked(audit)
	}
	return nil
}
