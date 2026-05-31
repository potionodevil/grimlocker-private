//go:build !enterprise

package single

import (
	"path/filepath"

	"github.com/grimlocker/grimdb/crypto"
	"github.com/grimlocker/grimdb/kernel"
	"github.com/grimlocker/grimdb/provider"
	"github.com/grimlocker/grimdb/security"
	"github.com/grimlocker/grimdb/storage"
	"github.com/grimlocker/grimdb/storage/grimdb"
)

// Provider is the Single-User VaultProvider.
// It wires together LocalAuth, LocalStorage, and the crypto.Provider,
// and exposes them through the provider.VaultProvider interface so the
// daemon never imports config/single directly.
type Provider struct {
	secMod    *security.Module
	cryptoMod *crypto.Module
	auth      *LocalAuth
	storage   *LocalStorage
	cryptoPrv crypto.Provider
}

// NewProvider constructs the full Single-User provider.
// appDir is the vault data directory; entropyPath is the entropy file used
// by the security module for hard-lockdown wiping.
func NewProvider(db *grimdb.GrimDB, appDir string, entropyPath string) *Provider {
	// Crypto provider (stateless, pure functions)
	cryptoPrv := crypto.New()

	// Security module (owns key material, lockdown, audit)
	secMod := security.NewModule(security.LockdownConfig{
		Threshold:       3,
		MaxOverrides:    4,
		LockdownMinutes: 200,
	}, filepath.Clean(entropyPath))

	// Block store + adapter
	stor := newLocalStorage(db, appDir)

	// Auth handler (wraps makeAuthUnlockHandler logic)
	auth := NewLocalAuth(secMod, stor.BlockStore(), appDir)

	// Crypto kernel module
	cryptoMod := crypto.NewModule(cryptoPrv, secMod.RetrieveMVK)

	return &Provider{
		secMod:    secMod,
		cryptoMod: cryptoMod,
		auth:      auth,
		storage:   stor,
		cryptoPrv: cryptoPrv,
	}
}

// SetSession links the security module and storage adapter to the session context.
// Must be called before StartAll().
func (p *Provider) SetSession(s *security.SessionContext) {
	p.secMod.SetSession(s)
	p.storage.Adapter().SetSession(s)
}

// Auth returns the AuthProvider for this tier.
func (p *Provider) Auth() provider.AuthProvider { return p.auth }

// Storage returns the StorageProvider for this tier.
func (p *Provider) Storage() provider.StorageProvider         { return p.storage }
func (p *Provider) StorageProvider() provider.StorageProvider { return p.storage }

// RawStorage returns the LocalStorage (needed for daemon wiring of IngestEngine, etc.)
func (p *Provider) RawStorage() *LocalStorage { return p.storage }

// Crypto returns the crypto.Provider.
func (p *Provider) Crypto() crypto.Provider { return p.cryptoPrv }

// Tier returns "single".
func (p *Provider) Tier() string { return "single" }

// KernelModules returns the ordered list of kernel.Module instances to register
// on the event bus: security → crypto → storage adapter.
func (p *Provider) KernelModules() []kernel.Module {
	return []kernel.Module{p.secMod, p.cryptoMod, p.storage.KernelModule()}
}

// SecurityModule returns the raw security.Module for components that require it
// (translator MVK resolver, policy manager audit log, integrity monitor).
func (p *Provider) SecurityModule() *security.Module { return p.secMod }

// BlockStore returns the underlying BlockStore as a storage.BlockStore interface.
// This is the tier-agnostic way to get the block store for EntryHandler, IngestEngine, etc.
func (p *Provider) BlockStore() storage.BlockStore { return p.storage.BlockStore() }

// CryptoProvider returns the raw crypto.Provider for components that require it
// (translator, ingest engine).
func (p *Provider) CryptoProvider() crypto.Provider { return p.cryptoPrv }
