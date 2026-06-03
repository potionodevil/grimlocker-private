//go:build windows

package rustbridge

import "github.com/grimlocker/grimdb/engine/bridge"

// EngineBridge returns a RustBridge implementation backed by the Windows Rust DLL.
func EngineBridge() bridge.RustBridge {
	if err := InitCore(); err != nil {
		return bridge.DefaultBridge{}
	}
	return &windowsBridge{}
}

type windowsBridge struct{}

func (b *windowsBridge) InitCore() error                                              { return InitCore() }
func (b *windowsBridge) ShutdownCore()                                               { ShutdownCore() }
func (b *windowsBridge) SecureZero(data []byte)                                      { SecureZero(data) }
func (b *windowsBridge) DeriveCoordinate(entropyData []byte, offsets []int64) ([]byte, error)     { return DeriveCoordinate(entropyData, offsets) }
func (b *windowsBridge) DeriveWorkspaceKey(masterKey []byte, workspaceID string) ([32]byte, error) { return DeriveWorkspaceKey(masterKey, workspaceID) }
func (b *windowsBridge) MVKStore(mvk []byte) (string, error)                        { return MVKStore(mvk) }
func (b *windowsBridge) MVKRevoke(handle string)                                     { MVKRevoke(handle) }
func (b *windowsBridge) EncryptHandle(handle string, plaintext, aad []byte) ([]byte, error)     { return EncryptHandle(handle, plaintext, aad) }
func (b *windowsBridge) DecryptHandle(handle string, ciphertext, aad []byte) ([]byte, error)     { return DecryptHandle(handle, ciphertext, aad) }
func (b *windowsBridge) GenerateEntropyFile(path string, lineCount int) error        { return GenerateEntropyFile(path, lineCount) }
