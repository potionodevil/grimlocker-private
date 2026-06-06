package strategies

import (
	"fmt"
	"log"

	"github.com/grimlocker/grimdb/engine/storage"
)

// HoneypotStrategy feuert einen Alarm, wenn ein designierter Bait-Block gelesen wird,
// und ruft dann einen optionalen Callback auf (z.B. Keys zeroisieren, Operator alarmieren).
// Wird beim Wire-up in den BlockStore injiziert.
type HoneypotStrategy struct {
	baitIDs  map[string]bool
	onTrigger func(baitID string)
}

// NewHoneypotStrategy erzeugt eine HoneypotStrategy.
// onTrigger wird in der gleichen Goroutine aufgerufen, die den Zugriff erkannt hat —
// also schnell sein (Flag setzen und returnen) — kein Locking im Callback.
func NewHoneypotStrategy(baitIDs []string, onTrigger func(baitID string)) *HoneypotStrategy {
	ids := make(map[string]bool, len(baitIDs))
	for _, id := range baitIDs {
		ids[id] = true
	}
	return &HoneypotStrategy{baitIDs: ids, onTrigger: onTrigger}
}

func (h *HoneypotStrategy) Name() string { return "honeypot" }

// OnWrite ist ein reiner Passthrough — Honeypot überwacht nur Reads.
func (h *HoneypotStrategy) OnWrite(b storage.Block) (storage.Block, error) {
	return b, nil
}

// OnRead erkennt Bait-Zugriffe und feuert den Alarm.
func (h *HoneypotStrategy) OnRead(b storage.Block) (storage.Block, error) {
	if h.baitIDs[b.ID] {
		log.Printf("[HONEYPOT] CRITICAL: bait block %s was accessed — intruder detected", b.ID)
		if h.onTrigger != nil {
			h.onTrigger(b.ID)
		}
	}
	return b, nil
}

// OnTrigger registriert eine neue Bait-ID ("bait:<id>") oder entfernt eine ("unbait:<id>").
func (h *HoneypotStrategy) OnTrigger(key string) error {
	if len(key) > 5 && key[:5] == "bait:" {
		h.baitIDs[key[5:]] = true
		return nil
	}
	if len(key) > 7 && key[:7] == "unbait:" {
		delete(h.baitIDs, key[7:])
		return nil
	}
	return fmt.Errorf("honeypot: unknown trigger format %q (use 'bait:<id>' or 'unbait:<id>')", key)
}
