package storage

import (
	"context"
	"encoding/hex"
	"sort"
	"sync"
	"time"

	"github.com/gitupofftheflooranddosomework/qh8z/internal/core"
)

type Memory struct {
	mu            sync.RWMutex
	links         map[string]core.Link
	visits        map[string][]core.Visit
	users         map[string]core.User
	usersByEmail  map[string]string
	workspaces    map[string]core.Workspace
	memberships   map[string]map[string]core.Membership
	sessions      map[string]core.Session
	verifications map[string]core.EmailVerification
	apiKeys       map[string]core.APIKey
	audits        []core.AuditEntry
}

func NewMemory() *Memory {
	return &Memory{
		links:         make(map[string]core.Link),
		visits:        make(map[string][]core.Visit),
		users:         make(map[string]core.User),
		usersByEmail:  make(map[string]string),
		workspaces:    make(map[string]core.Workspace),
		memberships:   make(map[string]map[string]core.Membership),
		sessions:      make(map[string]core.Session),
		verifications: make(map[string]core.EmailVerification),
		apiKeys:       make(map[string]core.APIKey),
	}
}

func hashKey(hash []byte) string { return hex.EncodeToString(hash) }

func (m *Memory) CreateLink(_ context.Context, link core.Link) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.links[link.Slug]; exists {
		return core.ErrConflict
	}
	if link.WorkspaceID != "" {
		if _, ok := m.workspaces[link.WorkspaceID]; !ok {
			return core.ErrNotFound
		}
	}
	m.links[link.Slug] = link
	return nil
}

func (m *Memory) CreateOwnedLink(_ context.Context, link core.Link, audit core.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.links[link.Slug]; exists {
		return core.ErrConflict
	}
	if link.WorkspaceID == "" {
		return core.ErrForbidden
	}
	if _, ok := m.workspaces[link.WorkspaceID]; !ok {
		return core.ErrNotFound
	}
	m.links[link.Slug] = link
	m.appendAuditLocked(audit)
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

func (m *Memory) GetWorkspaceLink(_ context.Context, workspaceID, slug string) (core.Link, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	link, ok := m.links[slug]
	if !ok || link.WorkspaceID != workspaceID {
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
	return m.statsLocked(slug, recentLimit)
}

func (m *Memory) StatsForWorkspace(_ context.Context, workspaceID, slug string, recentLimit int) (core.Stats, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	link, ok := m.links[slug]
	if !ok || link.WorkspaceID != workspaceID {
		return core.Stats{}, core.ErrNotFound
	}
	return m.statsLocked(slug, recentLimit)
}

func (m *Memory) statsLocked(slug string, recentLimit int) (core.Stats, error) {
	link, ok := m.links[slug]
	if !ok {
		return core.Stats{}, core.ErrNotFound
	}
	if recentLimit < 0 {
		recentLimit = 0
	}
	if recentLimit > 100 {
		recentLimit = 100
	}
	visits := m.visits[slug]
	if recentLimit > len(visits) {
		recentLimit = len(visits)
	}
	recent := make([]core.Visit, 0, recentLimit)
	for i := len(visits) - 1; i >= len(visits)-recentLimit; i-- {
		recent = append(recent, visits[i])
	}
	return core.Stats{TotalVisits: link.Visits, Recent: recent}, nil
}

func (m *Memory) Register(_ context.Context, reg core.Registration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, exists := m.usersByEmail[reg.User.Email]; exists {
		return core.ErrConflict
	}
	if _, exists := m.workspaces[reg.Workspace.ID]; exists {
		return core.ErrConflict
	}
	m.users[reg.User.ID] = reg.User
	m.usersByEmail[reg.User.Email] = reg.User.ID
	m.workspaces[reg.Workspace.ID] = reg.Workspace
	m.ensureMembershipMap(reg.Workspace.ID)[reg.User.ID] = reg.Membership
	m.sessions[hashKey(reg.Session.TokenHash)] = reg.Session
	m.verifications[hashKey(reg.Verification.TokenHash)] = reg.Verification
	for _, entry := range reg.Audit {
		m.appendAuditLocked(entry)
	}
	return nil
}

func (m *Memory) UserByEmail(_ context.Context, email string) (core.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	id, ok := m.usersByEmail[email]
	if !ok {
		return core.User{}, core.ErrNotFound
	}
	return m.users[id], nil
}

func (m *Memory) CreateSession(_ context.Context, session core.Session, audit core.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[session.UserID]; !ok {
		return core.ErrNotFound
	}
	m.sessions[hashKey(session.TokenHash)] = session
	m.appendAuditLocked(audit)
	return nil
}

func (m *Memory) DeleteSession(_ context.Context, tokenHash []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, hashKey(tokenHash))
	return nil
}

func (m *Memory) ResolveSession(_ context.Context, tokenHash []byte, workspaceHint string, now time.Time) (core.AuthContext, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hashKey(tokenHash)
	session, ok := m.sessions[key]
	if !ok || !session.ExpiresAt.After(now) {
		return core.AuthContext{}, core.ErrUnauthorized
	}
	user, ok := m.users[session.UserID]
	if !ok {
		return core.AuthContext{}, core.ErrUnauthorized
	}
	membership, ok := m.resolveMembershipLocked(user.ID, workspaceHint)
	if !ok {
		return core.AuthContext{}, core.ErrForbidden
	}
	session.LastSeenAt = now
	m.sessions[key] = session
	return core.AuthContext{
		UserID:        user.ID,
		Email:         user.Email,
		EmailVerified: user.EmailVerifiedAt != nil,
		WorkspaceID:   membership.WorkspaceID,
		Role:          membership.Role,
		Credential:    "session",
	}, nil
}

func (m *Memory) resolveMembershipLocked(userID, workspaceHint string) (core.Membership, bool) {
	if workspaceHint != "" {
		membership, ok := m.memberships[workspaceHint][userID]
		return membership, ok
	}
	var candidates []core.Membership
	for _, members := range m.memberships {
		if membership, ok := members[userID]; ok {
			candidates = append(candidates, membership)
		}
	}
	if len(candidates) == 0 {
		return core.Membership{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		wi := m.workspaces[candidates[i].WorkspaceID]
		wj := m.workspaces[candidates[j].WorkspaceID]
		if wi.CreatedAt.Equal(wj.CreatedAt) {
			return wi.ID < wj.ID
		}
		return wi.CreatedAt.Before(wj.CreatedAt)
	})
	return candidates[0], true
}

func (m *Memory) CreateEmailVerification(_ context.Context, verification core.EmailVerification) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[verification.UserID]; !ok {
		return core.ErrNotFound
	}
	m.verifications[hashKey(verification.TokenHash)] = verification
	return nil
}

func (m *Memory) ConsumeEmailVerification(_ context.Context, tokenHash []byte, now time.Time) (core.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := hashKey(tokenHash)
	verification, ok := m.verifications[key]
	if !ok || verification.UsedAt != nil {
		return core.User{}, core.ErrUnauthorized
	}
	if !verification.ExpiresAt.After(now) {
		return core.User{}, core.ErrExpired
	}
	user, ok := m.users[verification.UserID]
	if !ok {
		return core.User{}, core.ErrNotFound
	}
	verification.UsedAt = &now
	m.verifications[key] = verification
	if user.EmailVerifiedAt == nil {
		user.EmailVerifiedAt = &now
		m.users[user.ID] = user
	}
	m.appendAuditLocked(core.AuditEntry{
		ActorUserID:  user.ID,
		Action:       "user.email_verified",
		ResourceType: "user",
		ResourceID:   user.ID,
		CreatedAt:    now,
	})
	return user, nil
}

func (m *Memory) CreateWorkspace(_ context.Context, workspace core.Workspace, ownerID string, audit core.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[ownerID]; !ok {
		return core.ErrNotFound
	}
	if _, exists := m.workspaces[workspace.ID]; exists {
		return core.ErrConflict
	}
	m.workspaces[workspace.ID] = workspace
	m.ensureMembershipMap(workspace.ID)[ownerID] = core.Membership{
		WorkspaceID: workspace.ID,
		UserID:      ownerID,
		Role:        core.RoleOwner,
		CreatedAt:   workspace.CreatedAt,
	}
	m.appendAuditLocked(audit)
	return nil
}

func (m *Memory) ListWorkspaces(_ context.Context, userID string) ([]core.Workspace, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var result []core.Workspace
	for workspaceID, members := range m.memberships {
		if _, ok := members[userID]; ok {
			result = append(result, m.workspaces[workspaceID])
		}
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].ID < result[j].ID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (m *Memory) AddWorkspaceMember(_ context.Context, membership core.Membership, audit core.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.workspaces[membership.WorkspaceID]; !ok {
		return core.ErrNotFound
	}
	user, ok := m.users[membership.UserID]
	if !ok {
		return core.ErrNotFound
	}
	members := m.ensureMembershipMap(membership.WorkspaceID)
	if _, exists := members[membership.UserID]; exists {
		return core.ErrConflict
	}
	membership.Email = user.Email
	members[membership.UserID] = membership
	m.appendAuditLocked(audit)
	return nil
}

func (m *Memory) ListWorkspaceMembers(_ context.Context, workspaceID string) ([]core.Membership, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.workspaces[workspaceID]; !ok {
		return nil, core.ErrNotFound
	}
	var result []core.Membership
	for _, membership := range m.memberships[workspaceID] {
		membership.Email = m.users[membership.UserID].Email
		result = append(result, membership)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].CreatedAt.Equal(result[j].CreatedAt) {
			return result[i].UserID < result[j].UserID
		}
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (m *Memory) CreateAPIKey(_ context.Context, key core.APIKey, audit core.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.workspaces[key.WorkspaceID]; !ok {
		return core.ErrNotFound
	}
	hash := hashKey(key.KeyHash)
	if _, exists := m.apiKeys[hash]; exists {
		return core.ErrConflict
	}
	m.apiKeys[hash] = key
	m.appendAuditLocked(audit)
	return nil
}

func (m *Memory) ResolveAPIKey(_ context.Context, keyHash []byte, now time.Time) (core.AuthContext, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	hash := hashKey(keyHash)
	key, ok := m.apiKeys[hash]
	if !ok || key.RevokedAt != nil {
		return core.AuthContext{}, core.ErrUnauthorized
	}
	key.LastUsedAt = &now
	m.apiKeys[hash] = key
	user := m.users[key.CreatedByUserID]
	membership, _ := m.memberships[key.WorkspaceID][key.CreatedByUserID]
	return core.AuthContext{
		UserID:        key.CreatedByUserID,
		Email:         user.Email,
		EmailVerified: true,
		WorkspaceID:   key.WorkspaceID,
		Role:          membership.Role,
		APIKeyID:      key.ID,
		Scopes:        append([]string(nil), key.Scopes...),
		Credential:    "api_key",
	}, nil
}

func (m *Memory) WriteAudit(_ context.Context, entry core.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.appendAuditLocked(entry)
	return nil
}

func (m *Memory) ListAudit(_ context.Context, workspaceID string, limit int) ([]core.AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if _, ok := m.workspaces[workspaceID]; !ok {
		return nil, core.ErrNotFound
	}
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	result := make([]core.AuditEntry, 0, limit)
	for i := len(m.audits) - 1; i >= 0 && len(result) < limit; i-- {
		if m.audits[i].WorkspaceID == workspaceID {
			result = append(result, m.audits[i])
		}
	}
	return result, nil
}

func (m *Memory) ensureMembershipMap(workspaceID string) map[string]core.Membership {
	members := m.memberships[workspaceID]
	if members == nil {
		members = make(map[string]core.Membership)
		m.memberships[workspaceID] = members
	}
	return members
}

func (m *Memory) appendAuditLocked(entry core.AuditEntry) {
	entry.ID = int64(len(m.audits) + 1)
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = time.Now().UTC()
	}
	m.audits = append(m.audits, entry)
}

func (m *Memory) Ping(context.Context) error { return nil }
func (m *Memory) Close() error               { return nil }
