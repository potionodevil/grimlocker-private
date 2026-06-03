//go:build enterprise

package enterprise

// Package enterprise provides the Enterprise tier implementation of provider interfaces.
//
// Enterprise tier features:
//   - OIDCProvider: JWT access token validation (Keycloak / Azure AD / Okta)
//   - RemoteVault: AES-256-GCM encrypted block storage on S3 / MinIO
//   - mTLS: Mutual TLS between client EXE and daemon (see security/mtls)
package enterprise

import (
	"path/filepath"

	"github.com/grimlocker/grimdb/daemon/internal/bridge"
	"github.com/grimlocker/grimdb/engine/crypto"
	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/provider"
	"github.com/grimlocker/grimdb/engine/security"
	"github.com/grimlocker/grimdb/engine/storage"
	"github.com/grimlocker/grimdb/daemon/internal/storage/remote"
)

// Provider is the Enterprise VaultProvider.
type Provider struct {
	cfg         *EnterpriseConfig
	secMod      *security.Module
	cryptoMod   *crypto.Module
	cryptoPrv   crypto.Provider
	auth        *OIDCProvider
	remoteVault *remote.RemoteVault
	adapter     *remote.BlockStoreAdapter
}

// NewProvider constructs the full Enterprise provider.
func NewProvider(appDir, entropyPath string) (*Provider, error) {
	cfg, err := LoadEnterpriseConfig(appDir, entropyPath)
	if err != nil {
		return nil, err
	}
	return newProviderFromConfig(cfg)
}

func newProviderFromConfig(cfg *EnterpriseConfig) (*Provider, error) {
	cryptoPrv := crypto.New(bridge.EngineBridge())

	secMod := security.NewModule(security.LockdownConfig{
		Threshold:       3,
		MaxOverrides:    4,
		LockdownMinutes: 200,
	}, filepath.Clean(cfg.EntropyPath))

	vault, err := remote.NewRemoteVault(remote.RemoteVaultConfig{
		Endpoint:  cfg.S3Endpoint,
		Region:    cfg.S3Region,
		Bucket:    cfg.S3Bucket,
		AccessKey: cfg.S3AccessKey,
		SecretKey: cfg.S3SecretKey,
	})
	if err != nil {
		return nil, err
	}

	adapter := remote.NewBlockStoreAdapter(vault)
	oidcAuth := NewOIDCProvider(cfg, secMod, vault)
	cryptoMod := crypto.NewModule(cryptoPrv, secMod.RetrieveMVK)

	return &Provider{
		cfg:         cfg,
		secMod:      secMod,
		cryptoMod:   cryptoMod,
		cryptoPrv:   cryptoPrv,
		auth:        oidcAuth,
		remoteVault: vault,
		adapter:     adapter,
	}, nil
}

func (p *Provider) SetSession(s *security.SessionContext) {
	p.secMod.SetSession(s)
	p.adapter.SetSession(s)
}

func (p *Provider) Auth() provider.AuthProvider               { return p.auth }
func (p *Provider) Storage() provider.StorageProvider         { return &enterpriseStorage{vault: p.remoteVault, adapter: p.adapter} }
func (p *Provider) Crypto() crypto.Provider                   { return p.cryptoPrv }
func (p *Provider) Tier() string                              { return "enterprise" }
func (p *Provider) SecurityModule() *security.Module          { return p.secMod }
func (p *Provider) CryptoProvider() crypto.Provider           { return p.cryptoPrv }
func (p *Provider) RawVault() *remote.RemoteVault             { return p.remoteVault }

// BlockStore returns the RemoteVault as a storage.BlockStore interface.
// Mirrors single.Provider.BlockStore() so main.go can call it without build-tag guards.
func (p *Provider) BlockStore() storage.BlockStore            { return p.remoteVault }
func (p *Provider) EnterpriseConfig() *EnterpriseConfig       { return p.cfg }
func (p *Provider) TestConnectivity() error                   { return p.remoteVault.TestConnectivity() }

func (p *Provider) KernelModules() []kernel.Module {
	return []kernel.Module{p.secMod, p.cryptoMod, p.adapter}
}

// ── enterpriseStorage implements provider.StorageProvider ────────────────────

type enterpriseStorage struct {
	vault   *remote.RemoteVault
	adapter *remote.BlockStoreAdapter
}

// storage.BlockStore delegation to RemoteVault.
func (s *enterpriseStorage) WriteBlock(b storage.Block) error                    { return s.vault.WriteBlock(b) }
func (s *enterpriseStorage) ReadBlock(id string) (storage.Block, error)          { return s.vault.ReadBlock(id) }
func (s *enterpriseStorage) DeleteBlock(id string) error                         { return s.vault.DeleteBlock(id) }
func (s *enterpriseStorage) ListBlocks() ([]storage.BlockMeta, error)            { return s.vault.ListBlocks() }
func (s *enterpriseStorage) QueryBlocks(c storage.Category) ([]storage.BlockMeta, error) {
	return s.vault.QueryBlocks(c)
}
func (s *enterpriseStorage) Flush() error { return s.vault.Flush() }
func (s *enterpriseStorage) Close() error { return s.vault.Close() }

// provider.StorageProvider extension methods.
func (s *enterpriseStorage) SetMVKFunc(fn func() []byte) { s.vault.SetMVKFunc(fn) }
func (s *enterpriseStorage) LoadIndex() error             { return s.vault.LoadIndex() }
func (s *enterpriseStorage) KernelModule() kernel.Module  { return s.adapter }
