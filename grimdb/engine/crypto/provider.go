package crypto

import (
	"github.com/grimlocker/grimdb/engine/bridge"
)

// provider ist die konkrete Implementierung von Provider.
// Es delegiert Rust-Enclave-Operationen an die injizierte Bridge.
type provider struct {
	rb bridge.RustBridge
}

// New gibt einen Provider zurück, der von der gegebenen RustBridge backed wird.
// Übergib DefaultBridge für puren Go-Fallback (keine Rust-Enclave).
func New(rb bridge.RustBridge) Provider {
	return &provider{rb: rb}
}

// Bridge gibt die darunterliegende RustBridge-Instanz zurück.
// Von Consumern genutzt, die direkten Enclave-Zugriff brauchen.
func (p *provider) Bridge() bridge.RustBridge {
	return p.rb
}

// SecureZero überschreibt b mit Nullen.
func (p *provider) SecureZero(b []byte) {
	p.rb.SecureZero(b)
}

// DerivateCoordinate delegiert an die Bridge für den BLAKE3-accelerated Pfad.
func (p *provider) DeriveCoordinate(entropyData []byte, offsets []int64) ([]byte, error) {
	return p.rb.DeriveCoordinate(entropyData, offsets)
}

// DeriveWorkspaceKey delegiert an die Bridge für Enclave-accelerated Derivation.
func (p *provider) DeriveWorkspaceKey(masterKey []byte, workspaceID string) ([32]byte, error) {
	return p.rb.DeriveWorkspaceKey(masterKey, workspaceID)
}
