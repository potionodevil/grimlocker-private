// Package security (lockdown.go) implementiert den LockdownManager — eine State
// Machine für progressives Auth-Lockout.
//
// States:
//
//	LockdownNone → normaler Betrieb, bis zu Threshold Fehler erlaubt.
//	LockdownSoft → Threshold überschritten; Soft-Lockout für LockdownMinutes,
//	               bis zu MaxOverrides zusätzliche Versuche erlaubt.
//	LockdownHard → MaxOverrides erschöpft (oder TriggerHard direkt aufgerufen);
//	               der OnHard-Callback wird aufgerufen (Keys zeroisieren + os.Exit).
//
// Thread-safe: alle Methoden locken m.mu vor State-Lese/Schreibzugriffen.
//
// Default-Werte (wenn Config-Felder ≤ 0):
//   - Threshold: 3 Fehler vor Soft-Lockdown
//   - MaxOverrides: 4 zusätzliche Versuche während Soft-Lockdown
//   - LockdownMinutes: 200 Minuten Soft-Lockdown-Dauer
package security

import (
	"sync"
	"time"

	gerrors "github.com/grimlocker/grimdb/engine/errors"
)

// LockdownState beschreibt die aktuelle Lockout-Stufe.
type LockdownState int

const (
	LockdownNone LockdownState = 0 // normaler Betrieb
	LockdownSoft LockdownState = 1 // Limit erreicht, Timer läuft
	LockdownHard LockdownState = 2 // Wipe getriggert, Daemon muss exit
)

// LockdownManager trackt fehlgeschlagene Auth-Versuche und managed die
// progressive Lockdown-State-Machine.
type LockdownManager interface {
	RecordFailure() (LockdownState, error)
	RecordSuccess()
	State() LockdownState
	RemainingAttempts() int
	LockdownUntil() time.Time
	// TriggerHard geht sofort in LockdownHard.
	// Der Caller ist für das Zeroisieren von Secrets und den Exit verantwortlich.
	TriggerHard() error
}

type lockdownManager struct {
	mu              sync.Mutex
	failures        int
	threshold       int
	overridesLeft   int
	maxOverrides    int
	lockdownUntil   time.Time
	lockdownMinutes int
	state           LockdownState
	onHard          func() // callback bei Hard-Lockdown (z.B. zeroize + exit)
}

// LockdownConfig konfiguriert den Manager.
type LockdownConfig struct {
	Threshold       int    // Fehlversuche vor Soft-Lockdown
	MaxOverrides    int    // Override-Versuche während Soft-Lockdown
	LockdownMinutes int    // Soft-Lockdown-Dauer in Minuten
	OnHard          func() // wird bei Hard-Lockdown aufgerufen
}

// NewLockdownManager erzeugt einen LockdownManager aus der Config.
func NewLockdownManager(cfg LockdownConfig) LockdownManager {
	if cfg.Threshold <= 0 {
		cfg.Threshold = 3
	}
	if cfg.MaxOverrides <= 0 {
		cfg.MaxOverrides = 4
	}
	if cfg.LockdownMinutes <= 0 {
		cfg.LockdownMinutes = 200
	}
	return &lockdownManager{
		threshold:       cfg.Threshold,
		overridesLeft:   cfg.MaxOverrides,
		maxOverrides:    cfg.MaxOverrides,
		lockdownMinutes: cfg.LockdownMinutes,
		onHard:          cfg.OnHard,
		state:           LockdownNone,
	}
}

func (m *lockdownManager) RecordFailure() (LockdownState, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch m.state {
	case LockdownHard:
		return LockdownHard, gerrors.NewAuthLockdownError(0)

	case LockdownSoft:
		m.overridesLeft--
		if m.overridesLeft <= 0 {
			m.state = LockdownHard
			if m.onHard != nil {
				m.onHard()
			}
			return LockdownHard, gerrors.NewAuthLockdownError(0)
		}
		return LockdownSoft, gerrors.NewAuthLockdownError(m.overridesLeft)

	default:
		m.failures++
		if m.failures >= m.threshold {
			m.state = LockdownSoft
			m.lockdownUntil = time.Now().Add(time.Duration(m.lockdownMinutes) * time.Minute)
			return LockdownSoft, gerrors.NewAuthLockdownError(0)
		}
		remaining := m.threshold - m.failures
		return LockdownNone, gerrors.NewAuthInvalidError("credentials", nil).
			WithDetails("attempts_remaining", remaining)
	}
}

func (m *lockdownManager) RecordSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.failures = 0
	m.overridesLeft = m.maxOverrides
	m.state = LockdownNone
	m.lockdownUntil = time.Time{}
}

func (m *lockdownManager) State() LockdownState {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Soft-Lockdown nach Timer-Ablauf automatisch auslaufen lassen.
	if m.state == LockdownSoft && time.Now().After(m.lockdownUntil) {
		m.state = LockdownNone
		m.failures = 0
		m.overridesLeft = m.maxOverrides
	}
	return m.state
}

func (m *lockdownManager) RemainingAttempts() int {
	m.mu.Lock()
	defer m.mu.Unlock()

	switch m.state {
	case LockdownSoft:
		return m.overridesLeft
	case LockdownHard:
		return 0
	default:
		return m.threshold - m.failures
	}
}

func (m *lockdownManager) LockdownUntil() time.Time {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lockdownUntil
}

func (m *lockdownManager) TriggerHard() error {
	m.mu.Lock()
	m.state = LockdownHard
	m.mu.Unlock()

	if m.onHard != nil {
		m.onHard()
	}
	return nil
}
