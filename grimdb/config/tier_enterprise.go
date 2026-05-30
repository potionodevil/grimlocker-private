//go:build enterprise

package config

import (
	"fmt"

	"github.com/grimlocker/grimdb/config/enterprise"
	"github.com/grimlocker/grimdb/storage/grimdb"
)

// NewSingleUserProvider is the enterprise build entry-point.
// The function signature is identical to the single-user variant so
// main.go can call it without build-tag guards.
// In the enterprise tier the GrimDB file parameter is ignored —
// storage is handled by RemoteVault (S3/MinIO).
func NewSingleUserProvider(cfg TierConfig, _ *grimdb.GrimDB) *enterprise.Provider {
	p, err := enterprise.NewProvider(cfg.AppDir, cfg.EntropyPath)
	if err != nil {
		// Panic on startup config failure — invalid enterprise config should halt the daemon.
		panic(fmt.Sprintf("enterprise provider init failed: %v", err))
	}
	return p
}
