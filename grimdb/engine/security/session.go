// Package security (session.go) implementiert SessionContext — den globalen
// Vault-Unlock-State, der zwischen Security-Modul, Storage-Adapter und API-Translator geteilt wird.
//
// SessionContext beantwortet genau eine Frage: "Ist der Vault gerade unlocked?"
// Es ist die autoritative Quelle für den aktiven MVK-Handle und wird von
// HandshakeStatus konsultiert, damit sich reconnectende WebSocket-Clients wieder
// an einen bereits geöffneten Vault anhängen können — ohne Passwort-Neu eingabe.
//
// Thread-safe: alle exportierten Methoden holen das interne Mutex.
//
// Lifecycle:
//
//	NewSessionContext()          // erzeugen (Vault startet locked)
//	sessionCtx.Unlock(handle)   // aufgerufen nach AUTH.KEY_READY
//	sessionCtx.IsUnlocked()     // vom Storage-Adapter für Gate-Checks gepollt
//	sessionCtx.Lock()           // aufgerufen bei AUTH.LOGOUT
//	sessionCtx.SessionDestroy() // bei Graceful-Shutdown
package security

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/grimlocker/grimdb/engine/kernel"
)

// SessionContext hält den Runtime-Auth-State.
// Speichert NIE Plaintext-Passwörter — nur den abgeleiteten MVK-Handle.
// Thread-safe via RWMutex.
type SessionContext struct {
	mu              sync.RWMutex
	active          bool
	mvkHandle       string
	unlockedAt      time.Time
	dispatcher      kernel.Dispatcher
	autoLockMinutes int
	autoLockTimer   *time.Timer
	lastActivity    time.Time
}

// NewSessionContext erzeugt einen leeren Session-Kontext (Vault locked).
func NewSessionContext() *SessionContext {
	return &SessionContext{
		autoLockMinutes: 15, // default: 15 Minuten
	}
}

// SetAutoLockMinutes konfiguriert das Inactivity-Auto-Lock-Intervall.
// Auf 0 setzen, um Auto-Lock komplett zu deaktivieren.
func (s *SessionContext) SetAutoLockMinutes(minutes int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.autoLockMinutes = minutes
}

// ResetActivity restartet den Auto-Lock-Timer bei API-Aktivität.
// Vom Translator bei jeder Nicht-Heartbeat-Nachricht aufgerufen.
func (s *SessionContext) ResetActivity() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.autoLockTimer != nil {
		s.autoLockTimer.Reset(time.Duration(s.autoLockMinutes) * time.Minute)
	}
}

// SetDispatcher injiziert den Bus-Dispatcher fürs Emittieren von Events.
func (s *SessionContext) SetDispatcher(d kernel.Dispatcher) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.dispatcher = d
}

// Unlock markiert die Session als aktiv mit dem gegebenen MVK-Handle.
// Emittiert AUTH.READY, um zu signalisieren, dass der Vault nutzbar ist.
// Startet den Auto-Lock-Inactivity-Timer, falls konfiguriert.
func (s *SessionContext) Unlock(mvkHandle string) {
	s.mu.Lock()
	s.active = true
	s.mvkHandle = mvkHandle
	s.unlockedAt = time.Now()
	s.lastActivity = time.Now()

	if s.autoLockTimer != nil {
		s.autoLockTimer.Stop()
	}
	if s.autoLockMinutes > 0 {
		s.autoLockTimer = time.AfterFunc(time.Duration(s.autoLockMinutes)*time.Minute, func() {
			log.Printf("[session] Auto-lock: inactivity timeout (%d min) reached", s.autoLockMinutes)
			s.Lock()
		})
	}
	s.mu.Unlock()

	log.Printf("[session] Vault unlocked (handle=<redacted>)")

	if s.dispatcher != nil {
		// mvkHandle darf NICHT im Event-Payload landen — sonst wandert es durch
		// den Event-Bus und könnte in Logs auftauchen.
		payload, _ := json.Marshal(map[string]interface{}{
			"unlocked":    true,
			"unlocked_at": s.unlockedAt.Unix(),
		})
		ev := kernel.NewEvent("session", kernel.EvAuthReady, payload)
		_ = s.dispatcher.Dispatch(ev)
	}
}

// Lock löscht die Session und entzieht den aktiven Handle.
// Emittiert AUTH.LOGOUT fürs Downstream-Cleanup.
// Stoppt den Auto-Lock-Timer.
func (s *SessionContext) Lock() {
	s.mu.Lock()
	wasActive := s.active
	s.active = false
	s.mvkHandle = ""
	s.unlockedAt = time.Time{}
	s.lastActivity = time.Time{}
	if s.autoLockTimer != nil {
		s.autoLockTimer.Stop()
	}
	s.mu.Unlock()

	if wasActive {
		log.Printf("[session] Vault locked")
		if s.dispatcher != nil {
			payload, _ := json.Marshal(map[string]string{"reason": "session_locked"})
			ev := kernel.NewEvent("session", kernel.EvAuthLogout, payload)
			_ = s.dispatcher.Dispatch(ev)
		}
	}
}

// IsUnlocked gibt zurück, ob der Vault gerade unlocked ist.
func (s *SessionContext) IsUnlocked() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

// MVKHandle gibt den aktiven MVK-Handle zurück, oder "" wenn locked.
func (s *SessionContext) MVKHandle() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.mvkHandle
}

// ActiveHandle gibt den MVK-Handle zurück, wenn die Session unlocked ist,
// oder "" wenn locked. Liest beide Felder unter einem Lock — verhindert TOCTOU.
func (s *SessionContext) ActiveHandle() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if !s.active {
		return ""
	}
	return s.mvkHandle
}

// RequireUnlocked gibt nil zurück wenn unlocked, sonst einen Error.
func (s *SessionContext) RequireUnlocked() error {
	if !s.IsUnlocked() {
		return fmt.Errorf("vault locked: no active session")
	}
	return nil
}

// Health gibt ein JSON-serialisierbares Health-Check-Ergebnis zurück.
func (s *SessionContext) Health() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return map[string]interface{}{
		"active":      s.active,
		"mvk_handle":  s.mvkHandle != "",
		"unlocked_at": s.unlockedAt.Unix(),
		"age_seconds": time.Since(s.unlockedAt).Seconds(),
	}
}
