package backup

import (
	"crypto/rand"
	"fmt"
	"sync"
	"time"

	engbackup "github.com/grimlocker/grimdb/engine/backup"
)

const sessionTTL = 10 * time.Minute

// ImportSession holds Phase-1 state between Peek and Authorize.
type ImportSession struct {
	ID        string
	PeekResult engbackup.PeekResult
	Header    engbackup.BlobHeader
	BlobPath  string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// SessionStore manages active import sessions with TTL-based expiry.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]*ImportSession
}

func newSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]*ImportSession)}
}

func (s *SessionStore) newSession(header engbackup.BlobHeader, peek engbackup.PeekResult, blobPath string) *ImportSession {
	id := generateSessionID()
	now := time.Now()
	sess := &ImportSession{
		ID:        id,
		PeekResult: peek,
		Header:    header,
		BlobPath:  blobPath,
		CreatedAt: now,
		ExpiresAt: now.Add(sessionTTL),
	}
	s.mu.Lock()
	s.sessions[id] = sess
	s.mu.Unlock()
	return sess
}

func (s *SessionStore) lookup(id string) (*ImportSession, bool) {
	s.mu.RLock()
	sess, ok := s.sessions[id]
	s.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(sess.ExpiresAt) {
		s.delete(id)
		return nil, false
	}
	return sess, true
}

func (s *SessionStore) delete(id string) {
	s.mu.Lock()
	delete(s.sessions, id)
	s.mu.Unlock()
}

func (s *SessionStore) pruneExpired() {
	now := time.Now()
	s.mu.Lock()
	for id, sess := range s.sessions {
		if now.After(sess.ExpiresAt) {
			delete(s.sessions, id)
		}
	}
	s.mu.Unlock()
}

func generateSessionID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
