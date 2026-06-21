package strategies

import (
	"fmt"
	"sync"

	"github.com/grimlocker/grimdb/engine/storage"
)

// DeniableStrategy implementiert Plausible Deniability durch einen parallelen Satz
// von Decoy-Blöcken. Bei Aktivierung ("decoy") geben Reads den Decoy-Inhalt zurück
// statt den echten. Wird beim Wire-up in den BlockStore injiziert.
//
// Security-Tradeoff: Ein Angreifer mit physikalischem Zugriff kann die Block-Liste
// sehen und die Anzahl der Blöcke vergleichen. Wer viele Decoys braucht, sollte
// die Block-Anzahl ebenfalls verschleiern (Padding-Blöcke).
type DeniableStrategy struct {
	mu          sync.RWMutex
	decoyBlocks map[string]storage.Block
	active      bool // true = decoy mode aktiv
}

// NewDeniableStrategy erzeugt eine DeniableStrategy mit leerem Decoy-Store.
func NewDeniableStrategy() *DeniableStrategy {
	return &DeniableStrategy{
		decoyBlocks: make(map[string]storage.Block),
	}
}

func (d *DeniableStrategy) Name() string { return "deniable" }

// SetDecoy registriert einen Decoy-Block für die gegebene ID. Der Decoy-Block muss
// bereits verschlüsselten Ciphertext enthalten — nicht vom echten Block unterscheidbar.
func (d *DeniableStrategy) SetDecoy(b storage.Block) {
	d.mu.Lock()
	d.decoyBlocks[b.ID] = b
	d.mu.Unlock()
}

// OnWrite ist ein reiner Passthrough — Decoys werden via SetDecoy registriert, nicht via Write.
func (d *DeniableStrategy) OnWrite(b storage.Block) (storage.Block, error) {
	return b, nil
}

// OnRead gibt den Decoy-Block zurück, wenn Deniable Mode aktiv ist und ein Decoy
// existiert. Sonst den echten Block.
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

// OnTrigger aktiviert ("decoy") oder deaktiviert ("real") den Deniable Mode.
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
