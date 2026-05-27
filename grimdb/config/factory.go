// Package config provides the tier-selection factory for Grimlocker.
//
// Usage in cmd/daemon/main.go:
//
//	cfg := config.ConfigFromEnv(appDir)
//	vault := config.NewSingleUserProvider(cfg, db)  // or enterprise variant
//
// The concrete provider is selected at compile time via Go build tags:
//   - Default (no tags):  Single-User tier (config/single)
//   - -tags enterprise:   Enterprise tier  (config/enterprise)
package config

import (
	"os"
	"path/filepath"
)

// TierConfig holds all runtime configuration required to construct a VaultProvider.
type TierConfig struct {
	// AppDir is the vault data directory (default: ~/.grimlocker).
	AppDir string

	// EntropyPath is the entropy file path used by the security module
	// for cryptographic randomness and hard-lockdown wiping.
	EntropyPath string

	// Mode is the tier identifier from GRIMLOCKER_MODE env var.
	// Informational only — actual tier is selected via build tags.
	Mode string
}

// ConfigFromEnv builds a TierConfig by reading environment variables.
// Fallbacks are applied if variables are not set.
func ConfigFromEnv(appDir string) TierConfig {
	mode := os.Getenv("GRIMLOCKER_MODE")
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
