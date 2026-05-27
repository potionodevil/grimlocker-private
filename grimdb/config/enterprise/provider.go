//go:build enterprise

// Package enterprise provides the Enterprise tier implementation of provider interfaces.
//
// Current status: STUBS — all methods return ErrNotImplemented.
//
// Planned implementation:
//   - IAMProvider: SAML 2.0 / LDAP / SSO authentication instead of local Argon2id
//   - RemoteVault: S3-compatible or MinIO/Azure Blob backend instead of local files
//   - RemoteStorage: Encrypted remote block store with TLS transport
package enterprise

import (
	"errors"

	"github.com/grimlocker/grimdb/crypto"
	"github.com/grimlocker/grimdb/kernel"
	"github.com/grimlocker/grimdb/provider"
	"github.com/grimlocker/grimdb/security"
	"github.com/grimlocker/grimdb/storage"
	"github.com/grimlocker/grimdb/storage/grimdb"
)

// ErrNotImplemented is returned by all enterprise stubs until the feature is built.
var ErrNotImplemented = errors.New("enterprise: not implemented")

// ─── Provider ────────────────────────────────────────────────────────────────

// Provider is the Enterprise VaultProvider stub.
type Provider struct {
	appDir string
}

// NewProvider creates an Enterprise Provider. Currently returns a stub.
func NewProvider(appDir string) *Provider { return &Provider{appDir: appDir} }

func (p *Provider) Auth() provider.AuthProvider               { return &iamProvider{} }
func (p *Provider) StorageProvider() provider.StorageProvider { return &remoteStorage{} }
func (p *Provider) Crypto() crypto.Provider                   { return crypto.New() }
func (p *Provider) Tier() string                              { return "enterprise" }
func (p *Provider) KernelModules() []kernel.Module            { return nil }

// SetSession is a no-op for the enterprise stub (IAMProvider manages sessions).
func (p *Provider) SetSession(s *security.SessionContext) {}

// SecurityModule returns nil for the enterprise stub.
// The IAMProvider will own its own security module when implemented.
func (p *Provider) SecurityModule() *security.Module { return nil }

// CryptoProvider returns a local crypto provider (same for all tiers).
func (p *Provider) CryptoProvider() crypto.Provider { return crypto.New() }

// RawStorage returns a stub remote storage.
func (p *Provider) RawStorage() *remoteStorage { return &remoteStorage{} }

// ─── IAMProvider (AuthProvider stub) ─────────────────────────────────────────

// iamProvider is a stub that will implement SAML/LDAP/SSO auth.
type iamProvider struct{}

func (i *iamProvider) HandleUnlockEvent(
	bus kernel.Dispatcher,
	sessionCtx *security.SessionContext,
	onSessionKey func([]byte, string),
) kernel.Handler {
	return func(e kernel.Event) error {
		return ErrNotImplemented
	}
}

func (i *iamProvider) StoreMVK(key []byte) (string, error)      { return "", ErrNotImplemented }
func (i *iamProvider) RetrieveMVK(handle string) ([]byte, bool) { return nil, false }
func (i *iamProvider) RevokeMVK(handle string)                  {}
func (i *iamProvider) Lockdown() security.LockdownManager       { return nil }
func (i *iamProvider) AuditLog() security.AuditLog              { return nil }

// ─── RemoteStorage (StorageProvider stub) ─────────────────────────────────────

// remoteStorage is a stub that will implement S3/MinIO/Azure block storage.
type remoteStorage struct{}

func (r *remoteStorage) WriteBlock(b storage.Block) error          { return ErrNotImplemented }
func (r *remoteStorage) ReadBlock(id string) (storage.Block, error) {
	return storage.Block{}, ErrNotImplemented
}
func (r *remoteStorage) DeleteBlock(id string) error                              { return ErrNotImplemented }
func (r *remoteStorage) ListBlocks() ([]storage.BlockMeta, error)                 { return nil, ErrNotImplemented }
func (r *remoteStorage) QueryBlocks(_ storage.Category) ([]storage.BlockMeta, error) { return nil, ErrNotImplemented }
func (r *remoteStorage) Flush() error                                              { return ErrNotImplemented }
func (r *remoteStorage) Close() error                                              { return nil }
func (r *remoteStorage) SetMVKFunc(fn func() []byte) {}
func (r *remoteStorage) LoadIndex() error            { return ErrNotImplemented }
func (r *remoteStorage) KernelModule() kernel.Module {
	// Enterprise storage adapter will be wired here when implemented.
	db, _ := grimdb.NewGrimDB(":memory:")
	bs := grimdb.NewBlockStoreImpl("/tmp/enterprise-stub")
	return grimdb.NewAdapter(db, bs)
}

// BlockStore returns a stub BlockStoreImpl for enterprise tier.
// When IAMProvider and RemoteVault are implemented, this will return
// an encrypted remote block store.
func (r *remoteStorage) BlockStore() *grimdb.BlockStoreImpl {
	return grimdb.NewBlockStoreImpl("/tmp/enterprise-stub")
}

// Adapter returns a stub storage Adapter for enterprise tier.
func (r *remoteStorage) Adapter() *grimdb.Adapter {
	db, _ := grimdb.NewGrimDB(":memory:")
	bs := grimdb.NewBlockStoreImpl("/tmp/enterprise-stub")
	return grimdb.NewAdapter(db, bs)
}
