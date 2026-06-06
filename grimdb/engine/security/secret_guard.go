// Package security (secret_guard.go) implementiert SecretGuard — einen thread-safeen
// Store für kurzlebige Secrets (Passwörter, Keys, Tokens), die sofort aus dem Memory
// gezeroized werden müssen, sobald sie nicht mehr gebraucht werden.
//
// Jedes Secret wird in einem benannten Slot gespeichert. Wipe oder WipeAll überschreiben
// das zugrundeliegende Byte-Slice mit Nullen, bevor die Referenz freigegeben wird.
// Das begrenzt das Zeitfenster, in dem ein Memory-Dump oder GC-Scan das Secret finden könnte.
//
// SecretGuard ist kein Ersatz für mlock'd Memory (siehe MemoryGuard) — es lebt im Go-Heap
// und kann theoretisch ausgelagert werden. Nutze es für kurzlebige Transients (z.B. ein
// Passwort zwischen Empfang und Derivation). Für persistentes Key-Material (MVK, Session-Keys)
// MemoryGuard.AllocLocked verwenden.
package security

import (
	"crypto/rand"
	"encoding/hex"
	"log"
	"sync"
)

// SecretGuard ist ein thread-safeer Store für kurzlebige Secret-Byte-Slices.
// Jedes Secret wird durch einen opaque Nonce-Token identifiziert, den Store zurückgibt.
// Secrets werden bei Wipe oder WipeAll gezeroized.
type SecretGuard struct {
	mu    sync.Mutex
	slots map[string][]byte
}

// NewSecretGuard erzeugt einen leeren SecretGuard.
func NewSecretGuard() *SecretGuard {
	return &SecretGuard{
		slots: make(map[string][]byte),
	}
}

// Store speichert eine Kopie von secret unter einem frisch generierten Nonce und
// gibt den Nonce-Token zurück. Der Caller muss Wipe(token) aufrufen, wenn er fertig ist.
// Das originale Secret-Slice wird NICHT von Store gezeroized — das ist Caller-Verantwortung.
func (g *SecretGuard) Store(secret []byte) (token string, err error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token = hex.EncodeToString(raw)

	// Kopie speichern, damit wir das Memory kontrollieren.
	buf := make([]byte, len(secret))
	copy(buf, secret)

	g.mu.Lock()
	g.slots[token] = buf
	g.mu.Unlock()
	return token, nil
}

// Retrieve gibt das Secret für den gegebenen Token (als Kopie) zurück und entfernt den Slot.
// Das zurückgegebene Slice sollte nach Gebrauch vom Caller gezeroized werden.
// Gibt (nil, false) zurück, wenn der Token unbekannt oder bereits gewiped ist.
func (g *SecretGuard) Retrieve(token string) ([]byte, bool) {
	g.mu.Lock()
	defer g.mu.Unlock()

	buf, ok := g.slots[token]
	if !ok {
		return nil, false
	}
	// Kopie zurückgeben und die gespeicherte Kopie sofort wipen.
	out := make([]byte, len(buf))
	copy(out, buf)
	zeroize(buf)
	delete(g.slots, token)
	return out, true
}

// Wipe zeroized und entfernt den Slot für den gegebenen Token.
// Kann mehrfach aufgerufen werden — Folgeaufrufe sind No-Ops.
func (g *SecretGuard) Wipe(token string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	if buf, ok := g.slots[token]; ok {
		zeroize(buf)
		delete(g.slots, token)
	}
}

// WipeAll zeroized und entfernt ALLE gespeicherten Secrets.
// Aufruf bei Graceful-Shutdown oder Lockdown.
func (g *SecretGuard) WipeAll() {
	g.mu.Lock()
	defer g.mu.Unlock()

	count := len(g.slots)
	for token, buf := range g.slots {
		zeroize(buf)
		delete(g.slots, token)
	}
	if count > 0 {
		log.Printf("[SecretGuard] WipeAll — zeroed %d secret slots", count)
	}
}

// Count gibt die Anzahl der aktuell gespeicherten Secret-Slots zurück (für Diagnostics).
func (g *SecretGuard) Count() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.slots)
}
