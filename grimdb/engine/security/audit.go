// Package security (audit.go) implementiert den AuditLog — ein append-only,
// hash-gechainedes In-Memory-Log von Security-Events.
//
// Jedes SecurityEvent hat ein Level (Info/Warn/Critical), das Modul, eine
// lesbare Message und optional eine SubjectID (User/Session). Einträge sind
// via SHA-256-Chaining verlinkt — Manipulation an einem Eintrag invalidiert
// alle nachfolgenden.
//
// Der Log ist durch eine konfigurierbare Kapazität begrenzt; wenn voll, wird
// der älteste Eintrag verworfen (Ringbuffer). Nutze AuditLog.Append zum Schreiben
// und AuditLog.Entries zum Lesen des aktuellen Snapshots.
package security

import (
	"crypto/sha256"
	"encoding/binary"
	"sync"
	"time"
)

// Level constants for SecurityEvent.
const (
	LevelInfo     = "INFO"
	LevelWarn     = "WARN"
	LevelCritical = "CRITICAL"
)

// SecurityEvent ist ein immutabler Audit-Eintrag mit kryptografischem Chaining.
type SecurityEvent struct {
	Timestamp int64  `json:"timestamp"`
	Level     string `json:"level"`
	Module    string `json:"module"`
	Message   string `json:"message"`
	SubjectID string `json:"subject_id,omitempty"`   // Wer hat die Aktion ausgelöst
	PrevHash  []byte `json:"prev_hash,omitempty"`    // Hash des vorherigen Eintrags
	Hash      []byte `json:"hash,omitempty"`         // SHA-256 dieses Eintrags
}

// AuditLog ist ein thread-safeer Append-Only-Ringbuffer von SecurityEvents.
type AuditLog interface {
	Append(e SecurityEvent)
	Recent(n int) []SecurityEvent
	Drain() []SecurityEvent
}

type ringAuditLog struct {
	mu       sync.Mutex
	ring     []SecurityEvent
	cap      int
	head     int
	size     int
	lastHash [32]byte  // SHA-256 des aktuellsten Eintrags
}

// NewAuditLog erzeugt einen AuditLog mit der gegebenen Ringbuffer-Kapazität.
func NewAuditLog(capacity int) AuditLog {
	if capacity <= 0 {
		capacity = 1024
	}
	return &ringAuditLog{
		ring: make([]SecurityEvent, capacity),
		cap:  capacity,
	}
}

func (a *ringAuditLog) Append(e SecurityEvent) {
	if e.Timestamp == 0 {
		e.Timestamp = time.Now().UnixNano()
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	// Kryptografisches Chaining: hash = SHA-256(prevHash || timestamp || level || module || message || subjectID)
	e.PrevHash = a.lastHash[:]
	h := sha256.New()
	h.Write(a.lastHash[:])
	_ = binary.Write(h, binary.BigEndian, e.Timestamp)
	h.Write([]byte(e.Level))
	h.Write([]byte(e.Module))
	h.Write([]byte(e.Message))
	h.Write([]byte(e.SubjectID))
	e.Hash = h.Sum(nil)
	copy(a.lastHash[:], e.Hash)

	idx := (a.head + a.size) % a.cap
	a.ring[idx] = e
	if a.size < a.cap {
		a.size++
	} else {
		a.head = (a.head + 1) % a.cap
	}
}

func (a *ringAuditLog) Recent(n int) []SecurityEvent {
	a.mu.Lock()
	defer a.mu.Unlock()

	if n > a.size {
		n = a.size
	}

	result := make([]SecurityEvent, n)
	start := (a.head + a.size - n + a.cap) % a.cap
	for i := 0; i < n; i++ {
		result[i] = a.ring[(start+i)%a.cap]
	}
	return result
}

func (a *ringAuditLog) Drain() []SecurityEvent {
	a.mu.Lock()
	defer a.mu.Unlock()

	result := make([]SecurityEvent, a.size)
	for i := 0; i < a.size; i++ {
		result[i] = a.ring[(a.head+i)%a.cap]
	}
	a.size = 0
	a.head = 0
	return result
}
