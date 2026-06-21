package security

import (
	"crypto/rand"
	"fmt"
	"sync"

	gerrors "github.com/grimlocker/grimdb/engine/errors"
)

func randomHandle() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("security: CSPRNG failure during handle generation: %v", err))
	}
	return fmt.Sprintf("%x", b)
}

// ─── MVKStore Interface ───────────────────────────────────────────────────────

// MVKStore ist der Vertrag zum Speichern, Abrufen und Entziehen von MVK-Material
// in locked Memory. Kein anderes Modul hält die eigentlichen Key-Bytes —
// sie interagieren mit Key-Material ausschließlich via opaque String-Handles.
//
// Implementoren MÜSSEN:
//   - Backing-Memory via OS-Memory-Locking-API allozieren (mlock / VirtualLock)
//   - Key-Material bei Revoke und Prozess-Exit zeroisieren
//   - Key-Bytes nie auf den Heap kopieren (nur für den aktuellen Call-Frame returnen)
type MVKStore interface {
	// Store alloziert locked Memory, kopiert key hinein und gibt einen opaque Handle zurück.
	// Das originale Key-Slice sollte vom Caller nach Rückkehr gezeroized werden.
	Store(key []byte) (handle string, err error)

	// Retrieve gibt die rohen Key-Bytes für den gegebenen Handle zurück.
	// Gibt (nil, false) zurück, wenn der Handle unbekannt oder revoked ist.
	// WICHTIG: Caller dürfen das zurückgegebene Slice NICHT über den Call-Frame hinaus halten.
	Retrieve(handle string) (key []byte, ok bool)

	// Revoke zeroized und gibt locked Memory für den Handle frei.
	// Ignoriert unbekannte Handles silent.
	Revoke(handle string)

	// RevokeAll zeroized und gibt ALLE Handles frei. Aufruf bei Shutdown oder Lockdown.
	RevokeAll()

	// Handles gibt die Liste der aktiven Handle-Strings zurück (für Audit).
	// Gibt NIE das Key-Material selbst zurück.
	Handles() []string
}

// ─── lockedMVKStore — konkrete Implementierung ────────────────────────────────

// lockedMVKStore ist die MVKStore-Implementierung, die auf MemoryGuard basiert.
// Nutze NewLockedMVKStore für einen Standalone-Store oder security.Module,
// das einen internen einbettet.
type lockedMVKStore struct {
	mu      sync.RWMutex
	guard   MemoryGuard
	handles map[string][]byte // handle → locked memory slice
}

// NewLockedMVKStore erzeugt einen Standalone-MVKStore mit dem gegebenen MemoryGuard.
// Übergib einen plattform-passenden Guard (z.B. NewMemoryGuard() aus daemon/internal/security).
// Der Engine stellt einen Default-Go-Guard via NewGoMemoryGuard() bereit.
func NewLockedMVKStore(guard MemoryGuard) MVKStore {
	return &lockedMVKStore{
		guard:   guard,
		handles: make(map[string][]byte),
	}
}

// NewGoMemoryGuard gibt einen Go-Only-MemoryGuard zurück, der Bytes zeroized ohne
// OS-Level-Memory-Locking. Wird verwendet, wenn der echte MemoryGuard nicht verfügbar ist
// (z.B. im Engine-Package, wo mlock nicht zugänglich ist).
func NewGoMemoryGuard() MemoryGuard {
	return &goMemoryGuard{}
}

type goMemoryGuard struct{}

func (g *goMemoryGuard) Lock(b []byte) error                                            { return nil }
func (g *goMemoryGuard) Unlock(b []byte) error                                          { return nil }
func (g *goMemoryGuard) Zeroize(b []byte)                                               { zeroize(b) }
func (g *goMemoryGuard) CompareConstantTime(a, b []byte) bool                           { return constantTimeEqual(a, b) }
func (g *goMemoryGuard) AllocLocked(size int) ([]byte, error)                           { return make([]byte, size), nil }

func (s *lockedMVKStore) Store(key []byte) (string, error) {
	locked, err := s.guard.AllocLocked(len(key))
	if err != nil {
		return "", gerrors.NewSecurityMemlockError(err)
	}
	copy(locked, key)

	handle := randomHandle()
	s.mu.Lock()
	s.handles[handle] = locked
	s.mu.Unlock()
	return handle, nil
}

func (s *lockedMVKStore) Retrieve(handle string) ([]byte, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	key, ok := s.handles[handle]
	return key, ok
}

func (s *lockedMVKStore) Revoke(handle string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if key, ok := s.handles[handle]; ok {
		s.guard.Zeroize(key)
		_ = s.guard.Unlock(key)
		delete(s.handles, handle)
	}
}

func (s *lockedMVKStore) RevokeAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for handle, key := range s.handles {
		s.guard.Zeroize(key)
		_ = s.guard.Unlock(key)
		delete(s.handles, handle)
	}
}

func (s *lockedMVKStore) Handles() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.handles))
	for h := range s.handles {
		out = append(out, h)
	}
	return out
}
