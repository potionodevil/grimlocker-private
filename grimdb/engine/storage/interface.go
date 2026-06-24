package storage

// BlockStore ist das Interface, das jedes Storage-Backend implementieren muss.
// Implementierungen empfangen und geben opaque Blocks zurück — sie entschlüsseln nie selbst.
type BlockStore interface {
	WriteBlock(b Block) error
	ReadBlock(id string) (Block, error)
	DeleteBlock(id string) error
	ListBlocks() ([]BlockMeta, error)
	// QueryBlocks gibt alle BlockMeta zurück, deren Category dem gegebenen Wert entspricht.
	// Arbeitet auf dem In-Memory-Index; Vault muss unlocked sein.
	// Ein leerer String gibt alle Blöcke zurück (identisch zu ListBlocks).
	QueryBlocks(category Category) ([]BlockMeta, error)
	// Flush schreibt den In-Memory-Index atomar auf die Platte.
	Flush() error
	Close() error
}

// BlockStoreTransactional prüft ob ein BlockStore WAL-backed Transaktionen unterstützt.
// Nutze einen Type-Assert auf BlockStoreV2 um BeginWrite/BeginRead zu erhalten.
// BlockStoreImpl implementiert BlockStoreV2 wenn WAL aktiv ist.
type BlockStoreTransactional = BlockStoreV2

// StorageStrategy ist ein pluggbarer Interceptor, der in den BlockStore injiziert wird.
// Der Store ruft OnWrite vor dem Persistieren und OnRead nach dem Abrufen auf.
// OnTrigger wird mit einem Trigger-Key aufgerufen (z.B. "bait" für Honeypot,
// "decoy" für Deniable Encryption).
type StorageStrategy interface {
	Name() string
	OnWrite(b Block) (Block, error)
	OnRead(b Block) (Block, error)
	OnTrigger(key string) error
}

// NopStrategy ist eine No-Op-Strategy, wenn keine Strategy aktiv ist.
type NopStrategy struct{}

func (NopStrategy) Name() string               { return "nop" }
func (NopStrategy) OnWrite(b Block) (Block, error) { return b, nil }
func (NopStrategy) OnRead(b Block) (Block, error)  { return b, nil }
func (NopStrategy) OnTrigger(_ string) error        { return nil }
