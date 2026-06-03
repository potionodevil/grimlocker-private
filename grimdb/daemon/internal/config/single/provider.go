//go:build !enterprise

package single

import (
	"log"
	"path/filepath"
	"time"

	"github.com/grimlocker/grimdb/daemon/internal/bridge"
	engcrypto "github.com/grimlocker/grimdb/engine/crypto"
	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/provider"
	engsec "github.com/grimlocker/grimdb/engine/security"
	"github.com/grimlocker/grimdb/engine/storage"
	"github.com/grimlocker/grimdb/engine/storage/grimdb"
	cryptomod "github.com/grimlocker/grimdb/daemon/internal/modules/crypto"
	secmod "github.com/grimlocker/grimdb/daemon/internal/modules/security"
)

// Provider is the Single-User VaultProvider.
// It wires together LocalAuth, LocalStorage, the crypto.Provider,
// and the Local Network Sync subsystem, exposing them through the
// provider.VaultProvider interface so the daemon never imports
// config/single directly.
type Provider struct {
	secMod     *secmod.Module
	cryptoMod  *cryptomod.Module
	auth       *LocalAuth
	storage    *LocalStorage
	cryptoPrv  engcrypto.Provider
	appDir     string

	// Local Network Sync
	deviceID   *DeviceIdentity
	peerStore  *PeerStore
	syncState  *SyncState
	discovery  *Discovery
	scheduler  *SyncScheduler
}

// NewProvider constructs the full Single-User provider.
// appDir is the vault data directory; entropyPath is the entropy file used
// by the security module for hard-lockdown wiping.
func NewProvider(db *grimdb.GrimDB, appDir string, entropyPath string) *Provider {
	// Crypto provider (stateless, pure functions)
	cryptoPrv := engcrypto.New(rustbridge.EngineBridge())

	// Security module (owns key material, lockdown, audit)
	secMod := secmod.NewModule(engsec.LockdownConfig{
		Threshold:       3,
		MaxOverrides:    4,
		LockdownMinutes: 200,
	}, filepath.Clean(entropyPath))

	// Block store + adapter
	stor := newLocalStorage(db, appDir)

	// Auth handler (wraps makeAuthUnlockHandler logic)
	auth := NewLocalAuth(secMod, stor.BlockStore(), appDir)

	// Crypto kernel module
	cryptoMod := cryptomod.NewModule(cryptoPrv, secMod.RetrieveMVK)

	// Device identity for Local Network Sync
	deviceID, err := LoadOrCreateIdentity(appDir)
	if err != nil {
		log.Printf("[single:provider] device identity: %v (sync disabled)", err)
		deviceID = nil
	} else {
		log.Printf("[single:provider] device identity loaded: %s", deviceID.DeviceID)
	}

	// Peer store for sync
	peerStore, err := NewPeerStore(appDir)
	if err != nil {
		log.Printf("[single:provider] peer store: %v (sync disabled)", err)
		peerStore = nil
	}

	// Sync state
	syncState, err := LoadSyncState(appDir)
	if err != nil {
		log.Printf("[single:provider] sync state: %v (sync disabled)", err)
		syncState = nil
	}

	p := &Provider{
		secMod:    secMod,
		cryptoMod: cryptoMod,
		auth:      auth,
		storage:   stor,
		cryptoPrv: cryptoPrv,
		appDir:    appDir,
		deviceID:  deviceID,
		peerStore: peerStore,
		syncState: syncState,
	}

	// Discovery (mDNS) — created but not started until daemon calls StartSync
	if deviceID != nil && syncState != nil {
		p.discovery = NewDiscovery(deviceID.DeviceID, syncPort, p.getVersionVector)
	}

	return p
}

// InitSync initializes and starts the Local Network Sync subsystem.
// Must be called after the bus, sessionCtx, and blockStore are available.
func (p *Provider) InitSync(bus kernel.Dispatcher, sessionCtx *engsec.SessionContext, blockStore storage.BlockStore) error {
	if p.deviceID == nil || p.peerStore == nil || p.syncState == nil {
		return nil
	}
	if p.discovery == nil {
		return nil
	}

	// Start mDNS discovery
	if err := p.discovery.Start(); err != nil {
		log.Printf("[single:provider] discovery start: %v", err)
		return err
	}

	// Create and start sync scheduler
	interval := defaultSyncInterval
	p.scheduler = NewSyncScheduler(
		p.deviceID,
		p.peerStore,
		p.syncState,
		blockStore,
		p.secMod.Audit(),
		sessionCtx,
		p.discovery,
		bus,
		interval,
	)
	p.scheduler.Start()

	log.Printf("[single:provider] Local Network Sync initialized (device=%s)", p.deviceID.DeviceID)
	return nil
}

// ShutdownSync stops the sync subsystem.
func (p *Provider) ShutdownSync() {
	if p.scheduler != nil {
		p.scheduler.Stop()
	}
	if p.discovery != nil {
		p.discovery.Stop()
	}
	log.Printf("[single:provider] Local Network Sync shut down")
}

// getVersionVector returns the current version vector for mDNS advertisements.
func (p *Provider) getVersionVector() map[string]EntryVersion {
	if p.syncState == nil {
		return nil
	}
	return p.syncState.GetAllVersions()
}

// DeviceID returns the local device ID, or empty string if sync is unavailable.
func (p *Provider) DeviceID() string {
	if p.deviceID == nil {
		return ""
	}
	return p.deviceID.DeviceID
}

// SyncPort returns the TCP port used for sync connections.
func (p *Provider) SyncPort() int {
	return syncPort
}

// SyncIdentity returns the device identity for the sync listener's handshake.
func (p *Provider) SyncIdentity() *DeviceIdentity {
	return p.deviceID
}

// SyncPeerStore returns the peer store for verifying incoming connections.
func (p *Provider) SyncPeerStore() *PeerStore {
	return p.peerStore
}

// SyncState returns the sync state tracker.
func (p *Provider) SyncState() *SyncState {
	return p.syncState
}

// SyncPeers returns the current list of discovered peers from the mDNS cache.
// Returns nil if sync is not initialized.
func (p *Provider) SyncPeers() []DiscoveredPeer {
	if p.discovery == nil {
		return nil
	}
	return p.discovery.GetPeers()
}

// TriggerSync fires an immediate sync cycle outside the regular schedule.
// Non-blocking. Does nothing if sync is not initialized.
func (p *Provider) TriggerSync() {
	if p.scheduler != nil {
		p.scheduler.TriggerNow()
	}
}

// LastSyncAt returns the most recent sync timestamp across all known peers.
func (p *Provider) LastSyncAt() time.Time {
	if p.scheduler == nil {
		return time.Time{}
	}
	return p.scheduler.LastSyncAt()
}

// SetSession links the security module and storage adapter to the session context.
// Must be called before StartAll().
func (p *Provider) SetSession(s *engsec.SessionContext) {
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
func (p *Provider) Crypto() engcrypto.Provider { return p.cryptoPrv }

// Tier returns "single".
func (p *Provider) Tier() string { return "single" }

// KernelModules returns the ordered list of kernel.Module instances to register
// on the event bus: security → crypto → storage adapter.
func (p *Provider) KernelModules() []kernel.Module {
	return []kernel.Module{p.secMod, p.cryptoMod, p.storage.KernelModule()}
}

// SecurityModule returns the raw security.Module for components that require it
// (translator MVK resolver, policy manager audit log, integrity monitor).
func (p *Provider) SecurityModule() *secmod.Module { return p.secMod }

// BlockStore returns the underlying BlockStore as a storage.BlockStore interface.
// This is the tier-agnostic way to get the block store for EntryHandler, IngestEngine, etc.
func (p *Provider) BlockStore() storage.BlockStore { return p.storage.BlockStore() }

// CryptoProvider returns the raw crypto.Provider for components that require it
// (translator, ingest engine).
func (p *Provider) CryptoProvider() engcrypto.Provider { return p.cryptoPrv }
