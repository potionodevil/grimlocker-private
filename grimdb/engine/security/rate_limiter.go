// Package security (rate_limiter.go) implementiert RateLimiter — einen Exponential-
// Backoff-Rate-Limiter für Authentifizierungsversuche.
//
// Policy (angelehnt an NIST SP 800-63B):
//   - Versuche 1-5:   kein Lockout
//   - Versuch 6:      60 Sekunden Lockout
//   - Versuch 11:     600 Sekunden Lockout (10 Minuten)
//   - Versuch 16:     3600 Sekunden Lockout (1 Stunde)
//   - Versuch 21+:    86400 Sekunden Lockout (24 Stunden)
//
// Der State wird nur in-Memory gehalten. Nach Daemon-Restart ist das Lockout
// zurückgesetzt. Für persistentes Lockout über Restarts hinweg mit dem Vault-Index
// integrieren (siehe TODO unten).
package security

import (
	"log"
	"sync"
	"time"
)

// RateLimiter trackt Auth-Versuche pro Subject und erzwingt Exponential-Backoff-Lockouts.
type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rateLimitEntry
}

type rateLimitEntry struct {
	attempts    int
	lockedUntil time.Time
	lastAttempt time.Time
}

// NewRateLimiter erzeugt einen RateLimiter.
func NewRateLimiter() *RateLimiter {
	return &RateLimiter{
		entries: make(map[string]*rateLimitEntry),
	}
}

// Check gibt zurück, ob das Subject gerade einen Auth-Versuch machen darf.
// Returns (allowed=false, lockoutUntil) wenn gelockt.
// Zeichnet KEINEN neuen Versuch auf — RecordFailure nach fehlgeschlagenem Attempt aufrufen.
func (r *RateLimiter) Check(subject string) (allowed bool, lockedUntil time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.entries[subject]
	if !ok {
		return true, time.Time{}
	}

	if time.Now().Before(entry.lockedUntil) {
		return false, entry.lockedUntil
	}

	return true, time.Time{}
}

// RecordFailure zeichnet einen fehlgeschlagenen Auth-Versuch für das Subject auf.
// Gibt die neue Lockout-Dauer (0 wenn kein Lockout) und die Gesamt-Fehleranzahl zurück.
func (r *RateLimiter) RecordFailure(subject string) (lockoutDuration time.Duration, totalFailures int) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.entries[subject]
	if !ok {
		entry = &rateLimitEntry{}
		r.entries[subject] = entry
	}

	entry.attempts++
	entry.lastAttempt = time.Now()
	totalFailures = entry.attempts

	var lockout time.Duration
	switch {
	case entry.attempts >= 21:
		lockout = 24 * time.Hour
	case entry.attempts >= 16:
		lockout = time.Hour
	case entry.attempts >= 11:
		lockout = 10 * time.Minute
	case entry.attempts >= 6:
		lockout = 60 * time.Second
	default:
		lockout = 0
	}

	if lockout > 0 {
		entry.lockedUntil = time.Now().Add(lockout)
		log.Printf("[RateLimiter] LOCKOUT subject=%q attempts=%d lockout=%s until=%s",
			subject, entry.attempts, lockout, entry.lockedUntil.Format(time.RFC3339))
	} else {
		log.Printf("[RateLimiter] ATTEMPT subject=%q attempts=%d (no lockout yet)",
			subject, entry.attempts)
	}

	return lockout, totalFailures
}

// RecordSuccess resetet den Failure-Counter für das Subject.
// Nach erfolgreichem Login aufrufen.
func (r *RateLimiter) RecordSuccess(subject string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if entry, ok := r.entries[subject]; ok {
		if entry.attempts > 0 {
			log.Printf("[RateLimiter] SUCCESS subject=%q — resetting %d failure(s)",
				subject, entry.attempts)
		}
		delete(r.entries, subject)
	}
}

// RemainingAttempts gibt zurück, wie viele Fehlversuche noch erlaubt sind, bevor
// die nächste Lockout-Stufe greift. Gibt -1 zurück, wenn das Subject gerade gelockt ist.
func (r *RateLimiter) RemainingAttempts(subject string) int {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.entries[subject]
	if !ok {
		return 5
	}

	if time.Now().Before(entry.lockedUntil) {
		return -1
	}

	switch {
	case entry.attempts < 5:
		return 5 - entry.attempts
	case entry.attempts < 10:
		return 10 - entry.attempts
	case entry.attempts < 15:
		return 15 - entry.attempts
	case entry.attempts < 20:
		return 20 - entry.attempts
	default:
		return 0
	}
}

// LockoutStatus gibt zurück, ob das Subject gelockt ist und die Restzeit.
func (r *RateLimiter) LockoutStatus(subject string) (locked bool, remaining time.Duration) {
	r.mu.Lock()
	defer r.mu.Unlock()

	entry, ok := r.entries[subject]
	if !ok {
		return false, 0
	}

	now := time.Now()
	if now.Before(entry.lockedUntil) {
		return true, entry.lockedUntil.Sub(now)
	}
	return false, 0
}
