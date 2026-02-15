package mcp

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Session represents an MCP client session.
type Session struct {
	ID                 string
	ClientCapabilities *ClientCapabilities
	CreatedAt          time.Time
}

// SessionStore manages in-memory MCP sessions.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*Session
}

// NewSessionStore creates a new SessionStore.
func NewSessionStore() *SessionStore {
	return &SessionStore{
		sessions: make(map[string]*Session),
	}
}

// NewSession creates a new session with a cryptographically random ID.
func (s *SessionStore) NewSession(caps *ClientCapabilities) (*Session, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate session ID: %w", err)
	}

	session := &Session{
		ID:                 hex.EncodeToString(b),
		ClientCapabilities: caps,
		CreatedAt:          time.Now(),
	}

	s.mu.Lock()
	s.sessions[session.ID] = session
	s.mu.Unlock()

	return session, nil
}

// GetSession retrieves a session by ID.
func (s *SessionStore) GetSession(id string) (*Session, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	session, ok := s.sessions[id]
	return session, ok
}

// DeleteSession removes a session by ID.
func (s *SessionStore) DeleteSession(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}
