package store

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// Memory es un almacén en memoria para pruebas y demostraciones rápidas (STORE=memory).
// Los datos se pierden al reiniciar. No usar en producción.
type Memory struct {
	mu       sync.RWMutex
	users    map[string]User
	sessions map[string]Session
	audit    []AuditEntry
}

func NewMemory() *Memory {
	return &Memory{users: map[string]User{}, sessions: map[string]Session{}}
}

func (m *Memory) CountUsers(context.Context) (int64, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return int64(len(m.users)), nil
}

func (m *Memory) CreateUser(_ context.Context, u *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, x := range m.users {
		if strings.EqualFold(x.Email, u.Email) {
			return ErrDuplicate
		}
	}
	m.users[u.ID] = *u
	return nil
}

func (m *Memory) GetUserByID(_ context.Context, id string) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[id]
	if !ok {
		return nil, ErrNotFound
	}
	return &u, nil
}

func (m *Memory) GetUserByEmail(_ context.Context, email string) (*User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if strings.EqualFold(u.Email, email) {
			return &u, nil
		}
	}
	return nil, ErrNotFound
}

func (m *Memory) ListUsers(context.Context) ([]User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]User, 0, len(m.users))
	for _, u := range m.users {
		out = append(out, u)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Email < out[j].Email })
	return out, nil
}

func (m *Memory) UpdateUser(_ context.Context, u *User) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.users[u.ID]; !ok {
		return ErrNotFound
	}
	m.users[u.ID] = *u
	return nil
}

func (m *Memory) CreateSession(_ context.Context, s *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sessions[s.TokenHash] = *s
	return nil
}

func (m *Memory) GetSession(_ context.Context, h string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.sessions[h]
	if !ok {
		return nil, ErrNotFound
	}
	return &s, nil
}

func (m *Memory) TouchSession(_ context.Context, h string, at time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.sessions[h]
	if !ok {
		return ErrNotFound
	}
	s.LastSeenAt = at
	m.sessions[h] = s
	return nil
}

func (m *Memory) DeleteSession(_ context.Context, h string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.sessions, h)
	return nil
}

func (m *Memory) DeleteUserSessions(_ context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, s := range m.sessions {
		if s.UserID == userID {
			delete(m.sessions, k)
		}
	}
	return nil
}

func (m *Memory) AddAudit(_ context.Context, e *AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audit = append(m.audit, *e)
	if len(m.audit) > 5000 {
		m.audit = m.audit[len(m.audit)-5000:]
	}
	return nil
}

func (m *Memory) ListAudit(_ context.Context, limit int) ([]AuditEntry, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []AuditEntry{}
	for i := len(m.audit) - 1; i >= 0 && len(out) < limit; i-- {
		out = append(out, m.audit[i])
	}
	return out, nil
}

func (m *Memory) Ping(context.Context) error  { return nil }
func (m *Memory) Close(context.Context) error { return nil }
