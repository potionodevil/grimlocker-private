// Package config provides the tier-selection factory for Grimlocker.
//
// Usage in cmd/daemon/main.go:
//
//	cfg := config.ConfigFromEnv(appDir)
//	vault := config.NewProviderFromTier(cfg, db)
//
// The concrete provider is selected at runtime via GRIMLOCKER_TIER env var,
// with build-tag gating for enterprise-only packages.
//   - GRIMLOCKER_TIER=single (default): Single-User tier (config/single)
//   - GRIMLOCKER_TIER=enterprise:     Enterprise tier  (config/enterprise — requires -tags enterprise)
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/grimlocker/grimdb/daemon/internal/config/single"
	"github.com/grimlocker/grimdb/engine/provider"
	"github.com/grimlocker/grimdb/engine/storage/grimdb"
)

// TierConfig holds all runtime configuration required to construct a VaultProvider.
type TierConfig struct {
	// AppDir is the vault data directory (default: ~/.grimlocker).
	AppDir string

	// EntropyPath is the entropy file path used by the security module
	// for cryptographic randomness and hard-lockdown wiping.
	EntropyPath string

	// Mode is the tier identifier from GRIMLOCKER_TIER env var.
	Mode string
}

// ConfigFromEnv builds a TierConfig by reading environment variables.
// Fallbacks are applied if variables are not set.
func ConfigFromEnv(appDir string) TierConfig {
	mode := os.Getenv("GRIMLOCKER_TIER")
	if mode == "" {
		mode = "single"
	}
	if envDir := os.Getenv("GRIMLOCKER_APP_DIR"); envDir != "" {
		appDir = envDir
	}
	return TierConfig{
		AppDir:      appDir,
		EntropyPath: filepath.Join(appDir, "entropy.bin"),
		Mode:        mode,
	}
}

// NewProviderFromTier constructs the correct VaultProvider based on the tier
// specified in TierConfig.Mode. Falls back to single-user if the tier is
// unrecognized. Enterprise tier requires build tag `-tags enterprise`.
func NewProviderFromTier(cfg TierConfig, db *grimdb.GrimDB) (provider.VaultProvider, error) {
	switch cfg.Mode {
	case "single":
		return single.NewProvider(db, cfg.AppDir, cfg.EntropyPath), nil
	case "enterprise":
		return newEnterpriseProvider(cfg, db)
	default:
		return nil, fmt.Errorf("config: unknown tier %q (supported: single, enterprise)", cfg.Mode)
	}
}

// newEnterpriseProvider is implemented in tier_enterprise.go (gated by build tag).
// Falls back to single-user with a warning if enterprise build tag is absent.
func newEnterpriseProvider(cfg TierConfig, db *grimdb.GrimDB) (provider.VaultProvider, error) {
	return nil, fmt.Errorf("config: enterprise tier requires build tag `-tags enterprise` — recompile with enterprise features enabled")
}
