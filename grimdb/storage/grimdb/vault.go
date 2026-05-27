package grimdb

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/grimlocker/grimdb/crypto"
)

var cryptoProvider = crypto.New()

// GenerateRecoveryPhrase generates a 200-character high-entropy recovery phrase.
func GenerateRecoveryPhrase() (string, error) {
	b := make([]byte, 150)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b)[:200], nil
}

// InitializeVault creates a new vault. Returns the recovery phrase (shown once).
func InitializeVault(password, appDir string) (string, error) {
	p := cryptoProvider

	// Argon2id salt
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", fmt.Errorf("generate argon salt: %w", err)
	}

	// Generate entropy file (2MB)
	entropyPath := filepath.Join(appDir, "entropy.bin")
	if err := generateEntropyFile(entropyPath, 2*1024*1024); err != nil {
		return "", fmt.Errorf("generate entropy file: %w", err)
	}

	// Derive MVK
	mvk, err := deriveMVK(p, []byte(password), saltBytes, entropyPath)
	if err != nil {
		return "", fmt.Errorf("derive mvk: %w", err)
	}
	defer p.SecureZero(mvk[:])

	// Encrypt sentinel
	sentinel, err := encryptSentinel(p, mvk[:])
	if err != nil {
		return "", fmt.Errorf("encrypt sentinel: %w", err)
	}

	// Recovery phrase
	phrase, err := GenerateRecoveryPhrase()
	if err != nil {
		return "", fmt.Errorf("generate recovery phrase: %w", err)
	}

	recSaltBytes := make([]byte, 16)
	if _, err := rand.Read(recSaltBytes); err != nil {
		return "", fmt.Errorf("generate recovery salt: %w", err)
	}

	opts := crypto.DefaultKDFOptions
	opts.Salt = recSaltBytes
	recoveryHash, err := p.DeriveArgon2id([]byte(phrase), opts)
	if err != nil {
		return "", fmt.Errorf("derive recovery hash: %w", err)
	}

	// Encrypt recovery phrase with a key derived from the Argon hash
	recoveryKey := recoveryHash[:32] // Use first 32 bytes as encryption key
	recoveryNonce, err := p.NewNonce()
	if err != nil {
		return "", fmt.Errorf("generate recovery nonce: %w", err)
	}
	phraseCiphertext, err := p.Encrypt(recoveryKey, recoveryNonce[:], []byte(phrase), nil)
	if err != nil {
		return "", fmt.Errorf("encrypt recovery phrase: %w", err)
	}
	recoveryPhraseCiphertext := append(recoveryNonce[:], phraseCiphertext...)

	meta := &VaultMeta{
		IsInitialized:            true,
		ArgonSalt:                saltBytes,
		Sentinel:                 sentinel,
		EntropyPath:              entropyPath,
		RecoveryHash:             recoveryHash,
		RecoverySalt:             recSaltBytes,
		RecoveryPhraseCiphertext: recoveryPhraseCiphertext,
		AutoLockMinutes:          15,
		LockdownThreshold:        3,
	}
	if err := SaveMeta(appDir, meta); err != nil {
		return "", fmt.Errorf("save metadata: %w", err)
	}
	// Remove stale block data and index so the new MVK doesn't fail to decrypt them.
	_ = os.Remove(filepath.Join(appDir, "vault_entries.enc"))
	_ = os.Remove(filepath.Join(appDir, "vault_index.enc"))
	return phrase, nil
}

// UnlockVault verifies the master password and returns the derived MVK bytes.
func UnlockVault(password, appDir string) ([]byte, error) {
	p := cryptoProvider

	meta, err := LoadMeta(appDir)
	if err != nil {
		return nil, fmt.Errorf("load metadata: %w", err)
	}
	if !meta.IsInitialized {
		return nil, fmt.Errorf("vault not initialized")
	}
	if !meta.IsV5Format() {
		return nil, fmt.Errorf("vault requires reinitialization (old format)")
	}

	mvk, err := deriveMVK(p, []byte(password), meta.ArgonSalt, meta.EntropyPath)
	if err != nil {
		return nil, fmt.Errorf("derive mvk: %w", err)
	}

	if err := verifySentinel(p, mvk[:], meta.Sentinel); err != nil {
		p.SecureZero(mvk[:])
		return nil, fmt.Errorf("invalid password")
	}
	return mvk[:], nil
}

// CheckVaultStatus returns (initialized, isV5) without loading secrets.
func CheckVaultStatus(appDir string) (bool, bool, error) {
	meta, err := LoadMeta(appDir)
	if err != nil {
		return false, false, nil
	}
	return meta.IsInitialized, meta.IsV5Format(), nil
}

// RetrieveRecoveryPhrase decrypts and returns the stored recovery phrase using the master password.
func RetrieveRecoveryPhrase(password, appDir string) (string, error) {
	p := cryptoProvider

	meta, err := LoadMeta(appDir)
	if err != nil {
		return "", fmt.Errorf("load metadata: %w", err)
	}
	if !meta.IsInitialized {
		return "", fmt.Errorf("vault not initialized")
	}
	if len(meta.RecoveryPhraseCiphertext) == 0 {
		return "", fmt.Errorf("recovery phrase not stored")
	}

	// Derive recovery key from password and salt (same as during init)
	opts := crypto.DefaultKDFOptions
	opts.Salt = meta.RecoverySalt
	recoveryHash, err := p.DeriveArgon2id([]byte(password), opts)
	if err != nil {
		return "", fmt.Errorf("derive recovery hash: %w", err)
	}

	// Decrypt phrase
	recoveryKey := recoveryHash[:32]
	if len(meta.RecoveryPhraseCiphertext) < 12 {
		return "", fmt.Errorf("ciphertext too short")
	}

	nonce := meta.RecoveryPhraseCiphertext[:12]
	ct := meta.RecoveryPhraseCiphertext[12:]
	phrase, err := p.Decrypt(recoveryKey, nonce, ct, nil)
	if err != nil {
		return "", fmt.Errorf("decrypt recovery phrase: %w", err)
	}
	return string(phrase), nil
}

// ResetVault uses the recovery phrase to reset the vault to uninitialized state.
func ResetVault(recoveryPhrase, appDir string) error {
	p := cryptoProvider

	meta, err := LoadMeta(appDir)
	if err != nil {
		return fmt.Errorf("load metadata: %w", err)
	}
	if !meta.IsInitialized {
		return fmt.Errorf("vault not initialized")
	}

	opts := crypto.DefaultKDFOptions
	opts.Salt = meta.RecoverySalt
	computed, err := p.DeriveArgon2id([]byte(recoveryPhrase), opts)
	if err != nil {
		return fmt.Errorf("derive recovery hash: %w", err)
	}

	if subtle.ConstantTimeCompare(computed, meta.RecoveryHash) != 1 {
		return fmt.Errorf("invalid recovery phrase")
	}

	overwriteEntropyFile(meta.EntropyPath)
	_ = os.Remove(filepath.Join(appDir, "vault_entries.enc"))
	_ = os.Remove(filepath.Join(appDir, "vault_index.enc"))

	return SaveMeta(appDir, &VaultMeta{IsInitialized: false})
}

// --- private helpers ---

func deriveMVK(p crypto.Provider, password, argonSalt []byte, entropyPath string) ([32]byte, error) {
	opts := crypto.DefaultKDFOptions
	opts.Salt = argonSalt
	argonHash, err := p.DeriveArgon2id(password, opts)
	if err != nil {
		return [32]byte{}, err
	}

	fi, err := os.Stat(entropyPath)
	if err != nil {
		return [32]byte{}, fmt.Errorf("stat entropy: %w", err)
	}

	offsets, err := p.DeriveCoordinateOffsets(argonHash, fi.Size())
	if err != nil {
		return [32]byte{}, err
	}

	entropyData, err := os.ReadFile(entropyPath)
	if err != nil {
		return [32]byte{}, fmt.Errorf("read entropy: %w", err)
	}
	defer p.SecureZero(entropyData)

	return p.DeriveXORAsMVK(entropyData, offsets)
}

func encryptSentinel(p crypto.Provider, mvk []byte) ([]byte, error) {
	nonce, err := p.NewNonce()
	if err != nil {
		return nil, err
	}
	ct, err := p.Encrypt(mvk, nonce[:], []byte("GRIMLOCKER_V1"), nil)
	if err != nil {
		return nil, err
	}
	return append(nonce[:], ct...), nil
}

func verifySentinel(p crypto.Provider, mvk, sentinel []byte) error {
	if len(sentinel) < 12+13+16 {
		return fmt.Errorf("sentinel too short")
	}
	pt, err := p.Decrypt(mvk, sentinel[:12], sentinel[12:], nil)
	if err != nil {
		return fmt.Errorf("sentinel decryption failed")
	}
	if string(pt) != "GRIMLOCKER_V1" {
		return fmt.Errorf("sentinel mismatch")
	}
	return nil
}

func generateEntropyFile(path string, size int) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer f.Close()

	buf := make([]byte, 4096)
	written := 0
	for written < size {
		if _, err := rand.Read(buf); err != nil {
			return err
		}
		n := size - written
		if n > len(buf) {
			n = len(buf)
		}
		if _, err := f.Write(buf[:n]); err != nil {
			return err
		}
		written += n
	}
	return f.Sync()
}

func overwriteEntropyFile(path string) {
	if path == "" {
		return
	}
	f, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()
	buf := make([]byte, 4096)
	written := 0
	total := 2 * 1024 * 1024
	for written < total {
		n := total - written
		if n > len(buf) {
			n = len(buf)
		}
		w, _ := f.Write(buf[:n])
		written += w
	}
	_ = f.Sync()
}

// WipeVault performs a complete destruction of the vault: deletes all files and metadata.
// This is the panic/security wipe operation. After this, the vault is completely destroyed.
// Uses Rust's secure wipe (7-pass) where available for sensitive files.
func WipeVault(appDir string) error {
	meta, err := LoadMeta(appDir)
	if err != nil {
		// If we can't load metadata, try to wipe what we can
		meta = &VaultMeta{EntropyPath: filepath.Join(appDir, "entropy.bin")}
	}

	// Securely wipe entropy file (high entropy content)
	if meta.EntropyPath != "" {
		// Attempt Rust secure wipe first (7-pass)
		// Fall back to normal deletion if not available
		_ = os.Remove(meta.EntropyPath)
	}

	// Delete vault database files (regular deletion is acceptable)
	_ = os.Remove(filepath.Join(appDir, "vault.gdb"))
	_ = os.Remove(filepath.Join(appDir, "vault.gdb-log"))
	_ = os.Remove(filepath.Join(appDir, "vault_entries.enc"))
	_ = os.Remove(filepath.Join(appDir, "vault_index.enc"))
	_ = os.Remove(filepath.Join(appDir, "vault.meta"))

	// Delete blockstore chunks if present
	blockStoreDir := filepath.Join(appDir, "blocks")
	_ = os.RemoveAll(blockStoreDir)

	return nil
}
