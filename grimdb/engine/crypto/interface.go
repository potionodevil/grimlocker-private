package crypto

// KDFOptions parametrisiert die Argon2id-Key-Derivation.
type KDFOptions struct {
	Time    uint32
	Memory  uint32
	Threads uint8
	KeyLen  uint32
	Salt    []byte
}

// DefaultKDFOptions liefert gehärtete Argon2id-Parameter.
// 128 MB Memory und 4 Iterationen übertreffen die OWASP 2023-Empfehlungen
// und bieten robusten Widerstand gegen GPU-beschleunigtes Cracking.
var DefaultKDFOptions = KDFOptions{
	Time:    4,
	Memory:  131072, // 128 MB
	Threads: 2,
	KeyLen:  32,
}

// Provider ist das Interface für sämtliche reine Krypto-Operationen.
// Implementierungen MÜSSEN zustandslos sein und DÜRFEN KEIN File-I/O machen.
type Provider interface {
	// Encrypt gibt ChaCha20-Poly1305-Ciphertext zurück.
	Encrypt(key, nonce, plaintext, aad []byte) (ciphertext []byte, err error)

	// Decrypt verifiziert den Tag und gibt Plaintext zurück.
	Decrypt(key, nonce, ciphertext, aad []byte) (plaintext []byte, err error)

	// NewNonce generiert ein kryptografisch sicheres 12-Byte-Nonce.
	NewNonce() ([12]byte, error)

	// DeriveArgon2id wandelt ein Passwort in einen sicheren Schlüssel um — mit Argon2id,
	// dem Goldstandard für Memory-Hard-KDFs.
	DeriveArgon2id(password []byte, opts KDFOptions) ([]byte, error)

	// DeriveHKDF expandiert secret in keyLen Bytes via HKDF-SHA256.
	DeriveHKDF(secret, salt, info []byte, keyLen int) ([]byte, error)

	// DeriveCoordinate extrahiert Bytes an den gegebenen Offsets aus entropyData
	// und durchläuft BLAKE3→HKDF, um einen 32-Byte-Key zu produzieren.
	DeriveCoordinate(entropyData []byte, offsets []int64) ([]byte, error)

	// DeriveCoordinateOffsets konvertiert einen Argon2id-Hash in 32 File-Offsets
	// für DeriveXORAsMVK.
	DeriveCoordinateOffsets(argonHash []byte, fileSize int64) ([32]int64, error)

	// DeriveXORAsMVK XORed Entropy-Bytes an den gegebenen Offsets zu einem 32-Byte-MVK.
	DeriveXORAsMVK(entropyData []byte, offsets [32]int64) ([32]byte, error)

	// HMACKey leitet einen 32-Byte-HMAC-Key aus einem MVK ab.
	HMACKey(mvk []byte) [32]byte

	// SecureZero überschreibt b mit Nullen — compiler-resistent.
	SecureZero(b []byte)

	// GenerateEntropyFileWithProgress generiert eine 2MB-Entropy-Datei mit Progress-Callbacks.
	GenerateEntropyFileWithProgress(path string, progressFn func(pct float64, msg string)) error

	// DeriveWorkspaceKey leitet einen Workspace-spezifischen Encryption-Key aus dem Master-Key ab.
	DeriveWorkspaceKey(masterKey []byte, workspaceID string) ([32]byte, error)
}
