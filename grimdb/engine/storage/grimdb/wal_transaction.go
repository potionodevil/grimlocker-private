package grimdb

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/grimlocker/grimdb/engine/storage"
)

// WALTransaction implementiert storage.WriteTransaction mit WAL-Backing.
// Writes werden erst gepuffert, dann atomar via WAL committed:
//  1. Daten an vault_entries.enc appenden
//  2. WAL Write-Record schreiben (mit Offset + Metadaten für Recovery)
//  3. WAL Commit schreiben
//  4. In-Memory-Index aktualisieren
//  5. vault_index.enc persistent schreiben (temp+rename)
//  6. WAL truncaten (Checkpoint)
//
// Bei Crash nach Schritt 3 aber vor Schritt 5: LoadIndex + recoverWAL rekonstruiert
// den Index aus dem WAL. Bei Crash vor Schritt 3: Daten bleiben orphaned in
// vault_entries.enc (wasted space, aber kein Datenverlust oder Inkonsistenz).
type WALTransaction struct {
	txID    string
	store   *BlockStoreImpl
	writes  []storage.Block
	deletes []string
	done    bool
}

// newWALTransaction startet eine neue WAL-Transaktion und schreibt sofort TxBegin in das WAL.
func newWALTransaction(store *BlockStoreImpl) (*WALTransaction, error) {
	txID, err := generateTxID()
	if err != nil {
		return nil, fmt.Errorf("generate tx id: %w", err)
	}

	tx := &WALTransaction{txID: txID, store: store}

	if store.wal != nil {
		if err := store.wal.appendRecord(walRecord{
			Type:      walTypeTxBegin,
			TxID:      txID,
			Timestamp: time.Now().UnixNano(),
		}); err != nil {
			return nil, fmt.Errorf("wal tx begin: %w", err)
		}
	}
	return tx, nil
}

func (t *WALTransaction) WriteBlock(b storage.Block) error {
	if t.done {
		return storage.ErrTransactionClosed
	}
	t.writes = append(t.writes, b)
	return nil
}

func (t *WALTransaction) DeleteBlock(id string) error {
	if t.done {
		return storage.ErrTransactionClosed
	}
	t.deletes = append(t.deletes, id)
	return nil
}

// Commit wendet alle gepufferten Writes und Deletes atomar an.
// Hält bs.mu für die gesamte Dauer, damit kein anderer Writer den Zustand ändert.
func (t *WALTransaction) Commit() error {
	if t.done {
		return storage.ErrTransactionClosed
	}
	t.done = true

	bs := t.store
	bs.mu.Lock()
	defer bs.mu.Unlock()

	rollbackWAL := func() {
		if bs.wal != nil {
			_ = bs.wal.appendRecord(walRecord{Type: walTypeRollback, TxID: t.txID})
		}
	}

	// Phase 1: Daten schreiben + WAL Write-Records loggen
	for i := range t.writes {
		b := t.writes[i]

		offset, err := bs.appendBlockDataLocked(&b)
		if err != nil {
			rollbackWAL()
			return err
		}

		// In-Memory-Index aktualisieren
		bs.index[b.ID] = blockRecord{
			Offset:    offset,
			Length:    int64(len(b.Data)),
			Nonce:     b.Nonce,
			HMAC:      b.HMAC,
			Category:  b.Category,
			CreatedAt: b.CreatedAt,
			UpdatedAt: b.UpdatedAt,
		}

		// WAL Write-Record schreiben (mit allen Daten für späteres Recovery)
		if bs.wal != nil {
			if err := bs.wal.appendRecord(walRecord{
				Type:      walTypeWrite,
				TxID:      t.txID,
				BlockID:   b.ID,
				Offset:    offset,
				Length:    int64(len(b.Data)),
				Nonce:     b.Nonce,
				HMAC:      b.HMAC,
				Category:  b.Category,
				CreatedAt: b.CreatedAt,
				UpdatedAt: b.UpdatedAt,
			}); err != nil {
				rollbackWAL()
				return fmt.Errorf("wal write record: %w", err)
			}
		}
	}

	// Phase 2: Delete-Records loggen und Index bereinigen
	for _, id := range t.deletes {
		rec, exists := bs.index[id]
		if !exists {
			continue
		}

		if bs.wal != nil {
			if err := bs.wal.appendRecord(walRecord{
				Type:    walTypeDelete,
				TxID:    t.txID,
				BlockID: id,
			}); err != nil {
				rollbackWAL()
				return fmt.Errorf("wal delete record: %w", err)
			}
		}

		// Ciphertext überschreiben (best-effort)
		if err := bs.zeroBlockDataLocked(rec); err != nil {
			log.Printf("[wal-tx] zero block data für %s fehlgeschlagen: %v", id, err)
		}
		delete(bs.index, id)
	}

	// Phase 3: WAL Commit — ab hier kann Recovery bei Crash replayan
	if bs.wal != nil {
		if err := bs.wal.appendRecord(walRecord{Type: walTypeCommit, TxID: t.txID}); err != nil {
			return fmt.Errorf("wal commit: %w", err)
		}
	}

	// Phase 4: Index persistent schreiben (atomar via temp+rename)
	if err := bs.persistIndexLocked(); err != nil {
		return err
	}

	// Phase 5: WAL truncaten — Index ist jetzt kanonisch, WAL ist redundant
	if bs.wal != nil {
		if err := bs.wal.checkpoint(); err != nil {
			log.Printf("[wal-tx] Checkpoint nach Commit fehlgeschlagen: %v", err)
			// non-fatal — Index ist sicher auf Disk
		}
	}

	return nil
}

func (t *WALTransaction) Rollback() {
	if t.done {
		return
	}
	t.done = true
	if t.store.wal != nil {
		_ = t.store.wal.appendRecord(walRecord{Type: walTypeRollback, TxID: t.txID})
	}
}

func generateTxID() (string, error) {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
