//go:build !enterprise

package config

import (
	"github.com/grimlocker/grimdb/config/single"
	"github.com/grimlocker/grimdb/storage/grimdb"
)

// NewSingleUserProvider creates the Single-User VaultProvider.
// db must already be opened by the caller (e.g., grimdb.NewGrimDB(dbPath)).
func NewSingleUserProvider(cfg TierConfig, db *grimdb.GrimDB) *single.Provider {
	return single.NewProvider(db, cfg.AppDir, cfg.EntropyPath)
}
