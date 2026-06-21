package storage

// ─── BlockStoreV2 ─────────────────────────────────────────────────────────────

// BlockStoreV2 erweitert BlockStore um explizite Transaktionen.
// Transaktionen bieten Atomizität: entweder alle Writes in einer Transaktion
// werden committed, oder keine (Rollback).
//
// Nutze BlockStoreV2, wenn du mehrere Blöcke in einer atomaren Operation
// schreiben musst (z.B. Entry + Metadaten-Block anlegen).
//
// Existierender Code mit BlockStore funktioniert unverändert — BlockStoreV2
// ist eine abwärtskompatible Erweiterung, kein Ersatz.
type BlockStoreV2 interface {
	BlockStore

	// BeginWrite startet eine Write-Transaktion.
	// Nur eine Write-Transaktion kann gleichzeitig offen sein.
	// Der Caller muss Commit() oder Rollback() aufrufen.
	BeginWrite() (WriteTransaction, error)

	// BeginRead startet eine Read-Only-Snapshot-Transaktion.
	// Mehrere Read-Transaktionen können gleichzeitig laufen.
	BeginRead() (ReadTransaction, error)
}

// ─── WriteTransaction ─────────────────────────────────────────────────────────

// WriteTransaction batcht Block-Writes und committed sie atomar.
// Die Transaktion puffert alle Writes im Memory, bis Commit aufgerufen wird,
// sodass partielle Fehler den On-Disk-State nicht korrumpieren.
type WriteTransaction interface {
	WriteBlock(b Block) error
	DeleteBlock(id string) error
	Commit() error
	Rollback()
}

// ─── ReadTransaction ──────────────────────────────────────────────────────────

// ReadTransaction bietet einen konsistenten Snapshot-View des Stores.
// Reads innerhalb einer Transaktion sehen den State zum Zeitpunkt von BeginRead.
type ReadTransaction interface {
	ReadBlock(id string) (Block, error)
	ListBlocks() ([]BlockMeta, error)
	QueryBlocks(category Category) ([]BlockMeta, error)
	Close()
}

// ─── InMemoryWriteTransaction ─────────────────────────────────────────────────

// InMemoryWriteTransaction implementiert WriteTransaction mit einem Buffer.
// Der Store flushed den Buffer bei Commit. Geeignet für den aktuellen
// Single-Thread-File-Backed-Store; ein zukünftiger WAL-Store würde es ersetzen.
type InMemoryWriteTransaction struct {
	store    BlockStore
	writes   []Block
	deletes  []string
	done     bool
}

// NewInMemoryWriteTransaction erzeugt eine gepufferte Write-Transaktion über
// jedem BlockStore. Module, die Transaktionen brauchen, aber nur mit einem
// einfachen BlockStore arbeiten, können das als Compatibility-Shim nutzen.
func NewInMemoryWriteTransaction(store BlockStore) *InMemoryWriteTransaction {
	return &InMemoryWriteTransaction{store: store}
}

func (t *InMemoryWriteTransaction) WriteBlock(b Block) error {
	if t.done {
		return ErrTransactionClosed
	}
	t.writes = append(t.writes, b)
	return nil
}

func (t *InMemoryWriteTransaction) DeleteBlock(id string) error {
	if t.done {
		return ErrTransactionClosed
	}
	t.deletes = append(t.deletes, id)
	return nil
}

// Commit wendet alle staged Writes und dann Deletes an.
// Bei partiellem Fehler bleiben die erfolgreich geschriebenen Blöcke erhalten —
// das ist eine "Best-Effort-Atomic"-Implementierung. Echten atomaren Commit
// gibt's nur mit WAL-Backed-Store.
func (t *InMemoryWriteTransaction) Commit() error {
	if t.done {
		return ErrTransactionClosed
	}
	t.done = true

	for _, b := range t.writes {
		if err := t.store.WriteBlock(b); err != nil {
			return err
		}
	}
	for _, id := range t.deletes {
		if err := t.store.DeleteBlock(id); err != nil {
			return err
		}
	}
	return t.store.Flush()
}

func (t *InMemoryWriteTransaction) Rollback() {
	t.done = true
	t.writes = nil
	t.deletes = nil
}

// ─── Sentinel Errors ──────────────────────────────────────────────────────────

// ErrTransactionClosed wird zurückgegeben, wenn eine Methode auf einer bereits
// committed oder rolled-back Transaktion aufgerufen wird.
var ErrTransactionClosed = transactionClosedError{}

type transactionClosedError struct{}

func (transactionClosedError) Error() string {
	return "storage: operation on closed transaction"
}
