package strategies

import (
	"fmt"
	"sync"

	"github.com/grimlocker/grimdb/storage"
)

// DeniableStrategy implements plausible-deniability by maintaining a parallel
// set of decoy blocks. When triggered with "decoy", subsequent reads return
// decoy content instead of real content. This is injected into BlockStore
// at wire-up time.
type DeniableStrategy struct {
	mu          sync.RWMutex
	decoyBlocks map[string]storage.Block
	active      bool // true when decoy mode is active
}

// NewDeniableStrategy creates a DeniableStrategy with an empty decoy store.
func NewDeniableStrategy() *DeniableStrategy {
	return &DeniableStrategy{
		decoyBlocks: make(map[string]storage.Block),
	}
}

func (d *DeniableStrategy) Name() string { return "deniable" }

// SetDecoy registers a decoy block for the given ID. The decoy block's Data
// must already be encrypted ciphertext indistinguishable from real data.
func (d *DeniableStrategy) SetDecoy(b storage.Block) {
	d.mu.Lock()
	d.decoyBlocks[b.ID] = b
	d.mu.Unlock()
}

// OnWrite is a pass-through — decoy blocks are registered via SetDecoy, not Write.
func (d *DeniableStrategy) OnWrite(b storage.Block) (storage.Block, error) {
	return b, nil
}

// OnRead returns the decoy block when deniable mode is active and a decoy
// exists for the requested ID; otherwise returns the real block.
func (d *DeniableStrategy) OnRead(b storage.Block) (storage.Block, error) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if d.active {
		if decoy, ok := d.decoyBlocks[b.ID]; ok {
			return decoy, nil
		}
	}
	return b, nil
}

// OnTrigger activates ("decoy") or deactivates ("real") deniable mode.
func (d *DeniableStrategy) OnTrigger(key string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	switch key {
	case "decoy":
		d.active = true
	case "real":
		d.active = false
	default:
		return fmt.Errorf("deniable: unknown trigger %q", key)
	}
	return nil
}
