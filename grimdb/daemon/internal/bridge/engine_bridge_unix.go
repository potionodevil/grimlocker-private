//go:build !windows

package rustbridge

import "github.com/grimlocker/grimdb/engine/bridge"

// EngineBridge returns a RustBridge implementation backed by the Unix Rust SO.
func EngineBridge() bridge.RustBridge {
	if err := InitCore(); err != nil {
		return bridge.DefaultBridge{}
	}
	return &unixBridge{}
}

type unixBridge struct{}

func (b *unixBridge) InitCore() error                                              { return InitCore() }
func (b *unixBridge) ShutdownCore()                                               { ShutdownCore() }
func (b *unixBridge) SecureZero(data []byte)                                      { SecureZero(data) }
func (b *unixBridge) DeriveCoordinate(entropyData []byte, offsets []int64) ([]byte, error)     { return DeriveCoordinate(entropyData, offsets) }
func (b *unixBridge) DeriveWorkspaceKey(masterKey []byte, workspaceID string) ([32]byte, error) { return DeriveWorkspaceKey(masterKey, workspaceID) }
func (b *unixBridge) MVKStore(mvk []byte) (string, error)                        { return MVKStore(mvk) }
func (b *unixBridge) MVKRevoke(handle string)                                     { MVKRevoke(handle) }
func (b *unixBridge) EncryptHandle(handle string, plaintext, aad []byte) ([]byte, error)     { return EncryptHandle(handle, plaintext, aad) }
func (b *unixBridge) DecryptHandle(handle string, ciphertext, aad []byte) ([]byte, error)     { return DecryptHandle(handle, ciphertext, aad) }
func (b *unixBridge) GenerateEntropyFile(path string, lineCount int) error        { return GenerateEntropyFile(path, lineCount) }
