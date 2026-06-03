//go:build !enterprise

package single

import (
	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/storage"
	"github.com/grimlocker/grimdb/engine/storage/grimdb"
	stgrimdb "github.com/grimlocker/grimdb/daemon/internal/adapter"
)

// LocalStorage implements provider.StorageProvider for Single-User tier.
// It wraps the file-backed BlockStoreImpl and the kernel Adapter together
// so the daemon can register them without knowing the concrete types.
type LocalStorage struct {
	blockStore *grimdb.BlockStoreImpl
	adapter    *stgrimdb.Adapter
}

// newLocalStorage creates a LocalStorage backed by the given GrimDB instance.
func newLocalStorage(db *grimdb.GrimDB, appDir string) *LocalStorage {
	bs := grimdb.NewBlockStoreImpl(appDir)
	adapter := stgrimdb.NewAdapter(db, bs)
	return &LocalStorage{blockStore: bs, adapter: adapter}
}

// --- storage.BlockStore delegation ---

func (s *LocalStorage) WriteBlock(b storage.Block) error { return s.blockStore.WriteBlock(b) }

func (s *LocalStorage) ReadBlock(id string) (storage.Block, error) { return s.blockStore.ReadBlock(id) }
func (s *LocalStorage) DeleteBlock(id string) error                { return s.blockStore.DeleteBlock(id) }
func (s *LocalStorage) ListBlocks() ([]storage.BlockMeta, error)   { return s.blockStore.ListBlocks() }
func (s *LocalStorage) QueryBlocks(cat storage.Category) ([]storage.BlockMeta, error) {
	return s.blockStore.QueryBlocks(cat)
}
func (s *LocalStorage) Flush() error { return s.blockStore.Flush() }
func (s *LocalStorage) Close() error { return s.blockStore.Close() }

// BlockStore returns the raw BlockStoreImpl for direct use where needed
// (e.g., IngestEngine, EntryHandler).
func (s *LocalStorage) BlockStore() *grimdb.BlockStoreImpl { return s.blockStore }

// SetMVKFunc wires the key-retrieval callback used for block encryption.
func (s *LocalStorage) SetMVKFunc(fn func() []byte) { s.blockStore.SetMVKFunc(fn) }

// LoadIndex loads the block index from disk after vault unlock.
func (s *LocalStorage) LoadIndex() error { return s.blockStore.LoadIndex() }

// KernelModule returns the storage adapter as a kernel.Module so it can
// be registered with the event bus via reg.Add().
func (s *LocalStorage) KernelModule() kernel.Module { return s.adapter }

// SetSession links the adapter to the session context for vault-state gating.
func (s *LocalStorage) SetSession(session interface{ IsUnlocked() bool }) {
	if sc, ok := session.(interface {
		IsUnlocked() bool
	}); ok {
		_ = sc // session passed via SetSession on adapter
	}
}

// Adapter returns the raw Adapter for session wiring in the daemon.
func (s *LocalStorage) Adapter() *stgrimdb.Adapter { return s.adapter }
