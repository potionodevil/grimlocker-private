//go:build enterprise

package config

import (
	"github.com/grimlocker/grimdb/config/enterprise"
	"github.com/grimlocker/grimdb/storage/grimdb"
)

// NewSingleUserProvider is the enterprise build entry-point.
// The function signature is identical to the single-user variant so
// main.go can call it without build-tag guards.
func NewSingleUserProvider(cfg TierConfig, db *grimdb.GrimDB) *enterprise.Provider {
	return enterprise.NewProvider(cfg.AppDir)
}
