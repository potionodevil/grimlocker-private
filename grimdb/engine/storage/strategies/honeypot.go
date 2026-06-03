package strategies

import (
	"fmt"
	"log"

	"github.com/grimlocker/grimdb/engine/storage"
)

// HoneypotStrategy fires an alarm when a designated bait block is read,
// then invokes an optional callback (e.g. zeroize secrets, alert operator).
// It is injected into BlockStore at wire-up time.
type HoneypotStrategy struct {
	baitIDs  map[string]bool
	onTrigger func(baitID string)
}

// NewHoneypotStrategy creates a HoneypotStrategy.
// onTrigger is called in the goroutine that detected access; it should be fast
// (e.g. set a flag and return) — do not lock from within it.
func NewHoneypotStrategy(baitIDs []string, onTrigger func(baitID string)) *HoneypotStrategy {
	ids := make(map[string]bool, len(baitIDs))
	for _, id := range baitIDs {
		ids[id] = true
	}
	return &HoneypotStrategy{baitIDs: ids, onTrigger: onTrigger}
}

func (h *HoneypotStrategy) Name() string { return "honeypot" }

// OnWrite is a pass-through — honeypot only watches reads.
func (h *HoneypotStrategy) OnWrite(b storage.Block) (storage.Block, error) {
	return b, nil
}

// OnRead detects bait access and fires the alarm callback.
func (h *HoneypotStrategy) OnRead(b storage.Block) (storage.Block, error) {
	if h.baitIDs[b.ID] {
		log.Printf("[HONEYPOT] CRITICAL: bait block %s was accessed — intruder detected", b.ID)
		if h.onTrigger != nil {
			h.onTrigger(b.ID)
		}
	}
	return b, nil
}

// OnTrigger registers a new bait ID ("bait:<id>") or removes one ("unbait:<id>").
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
