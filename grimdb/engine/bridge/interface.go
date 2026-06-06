// Package bridge stellt das Interface zur Rust Secure Enclave bereit.
//
// Das Engine nutzt dieses Interface für optionale Hardware-beschleunigte Krypto.
// Wenn die Rust-Enclave nicht verfügbar ist, stellt DefaultBridge pure Go-Fallback-
// Implementierungen bereit.
package bridge

// RustBridge abstrahiert die Rust-Secure-Enclave-Operationen.
type RustBridge interface {
	InitCore() error
	ShutdownCore()
	// SecureZero überschreibt b mit Nullen — compiler-resistent.
	// Wenn die Rust-Enclave verfügbar ist, nutzt sie 7-pass Secure Wipe.
	SecureZero(b []byte)

	// DeriveCoordinate führt BLAKE3→HKDF auf Entropy-Daten via Enclave aus.
	// Fällt auf SHA-256→HKDF zurück, wenn die Enclave nicht verfügbar ist.
	DeriveCoordinate(entropyData []byte, offsets []int64) ([]byte, error)

	// DeriveWorkspaceKey leitet einen Workspace-spezifischen Key vom Master-Key ab.
	DeriveWorkspaceKey(masterKey []byte, workspaceID string) ([32]byte, error)

	// MVKStore speichert einen MVK in der Enclave und gibt einen Handle zurück.
	MVKStore(mvk []byte) (string, error)

	// MVKRevoke entzieht einen MVK-Handle in der Enclave.
	MVKRevoke(handle string)

	// EncryptHandle encryptet Daten mit einem Session-Key-Handle in der Enclave.
	EncryptHandle(handle string, plaintext, aad []byte) ([]byte, error)

	// DecryptHandle decryptet Daten mit einem Session-Key-Handle in der Enclave.
	DecryptHandle(handle string, ciphertext, aad []byte) ([]byte, error)

	// GenerateEntropyFile generiert eine Entropy-Datei via Enclave.
	GenerateEntropyFile(path string, lineCount int) error
}

// DefaultBridge stellt pure Go-Fallback-Implementierungen für das RustBridge-Interface bereit.
type DefaultBridge struct{}

func (DefaultBridge) InitCore() error                                         { return nil }
func (DefaultBridge) ShutdownCore()                                           {}
func (DefaultBridge) SecureZero(b []byte)                                     { GoSecureZero(b) }
func (DefaultBridge) DeriveCoordinate(entropyData []byte, offsets []int64) ([]byte, error)     { return GoDeriveCoordinate(entropyData, offsets) }
func (DefaultBridge) DeriveWorkspaceKey(masterKey []byte, workspaceID string) ([32]byte, error) { return GoDeriveWorkspaceKey(masterKey, workspaceID) }
func (DefaultBridge) MVKStore(mvk []byte) (string, error)                    { return "", nil }
func (DefaultBridge) MVKRevoke(handle string)                                 {}
func (DefaultBridge) EncryptHandle(handle string, plaintext, aad []byte) ([]byte, error) { return nil, nil }
func (DefaultBridge) DecryptHandle(handle string, ciphertext, aad []byte) ([]byte, error) { return nil, nil }
func (DefaultBridge) GenerateEntropyFile(path string, lineCount int) error    { return nil }
