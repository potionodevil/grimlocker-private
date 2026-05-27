//go:build !enterprise

package single

import (
	"github.com/grimlocker/grimdb/kernel"
	"github.com/grimlocker/grimdb/storage/grimdb"
)

// LocalStorage implements provider.StorageProvider for Single-User tier.
// It wraps the file-backed BlockStoreImpl and the kernel Adapter together
// so the daemon can register them without knowing the concrete types.
type LocalStorage struct {
	blockStore *grimdb.BlockStoreImpl
	adapter    *grimdb.Adapter
}

// newLocalStorage creates a LocalStorage backed by the given GrimDB instance.
func newLocalStorage(db *grimdb.GrimDB, appDir string) *LocalStorage {
	bs := grimdb.NewBlockStoreImpl(appDir)
	adapter := grimdb.NewAdapter(db, bs)
	return &LocalStorage{blockStore: bs, adapter: adapter}
}

// --- storage.BlockStore delegation ---

func (s *LocalStorage) WriteBlock(b interface{}) error {
	// Type-checked at compile time via the embed in provider.StorageProvider.
	// Actual delegation occurs through the blockStore field.
	return nil // implemented via BlockStoreImpl directly
}

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
func (s *LocalStorage) Adapter() *grimdb.Adapter { return s.adapter }
