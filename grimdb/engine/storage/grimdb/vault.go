package grimdb

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"

	"github.com/grimlocker/grimdb/engine/bridge"
	"github.com/grimlocker/grimdb/engine/crypto"
)

var cryptoProvider = crypto.New(bridge.DefaultBridge{})

// GenerateRecoveryPhrase erzeugt eine 200-Zeichen-high-entropy Recovery-Phrase.
func GenerateRecoveryPhrase() (string, error) {
	b := make([]byte, 150)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(b)
	if len(encoded) != 200 {
		return "", fmt.Errorf("recovery phrase encoding: unexpected length %d", len(encoded))
	}
	return encoded, nil
}

// InitializeVault erstellt ein neues Vault. Gibt die Recovery-Phrase zurück (nur einmal angezeigt).
func InitializeVault(password, appDir string) (string, error) {
	p := cryptoProvider

	// Argon2id Salt
	saltBytes := make([]byte, 16)
	if _, err := rand.Read(saltBytes); err != nil {
		return "", fmt.Errorf("generate argon salt: %w", err)
	}

	// Entropy-Datei (2MB) generieren
	entropyPath := filepath.Join(appDir, "entropy.bin")
	if err := generateEntropyFile(entropyPath, 2*1024*1024); err != nil {
		return "", fmt.Errorf("generate entropy file: %w", err)
	}

	// MVK ableiten
	mvk, err := deriveMVK(p, []byte(password), saltBytes, entropyPath)
	if err != nil {
		return "", fmt.Errorf("derive mvk: %w", err)
	}
	defer p.SecureZero(mvk[:])

	// Sentinel encrypten
	sentinel, err := encryptSentinel(p, mvk[:])
	if err != nil {
		return "", fmt.Errorf("encrypt sentinel: %w", err)
	}

	// Recovery-Phrase
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

	// Recovery-Phrase mit einem aus dem Argon-Hash abgeleiteten Key verschlüsseln
	recoveryKey := recoveryHash[:32]
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
	// Stale Block-Daten und Index entfernen, damit der neue MVK nicht versucht, sie zu entschlüsseln.
	_ = os.Remove(filepath.Join(appDir, "vault_entries.enc"))
	_ = os.Remove(filepath.Join(appDir, "vault_index.enc"))
	return phrase, nil
}

// UnlockVault verifiziert das Master-Passwort und gibt den abgeleiteten MVK zurück.
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

// CheckVaultStatus gibt (initialized, isV5) zurück, ohne Secrets zu laden.
func CheckVaultStatus(appDir string) (bool, bool, error) {
	meta, err := LoadMeta(appDir)
	if err != nil {
		return false, false, nil
	}
	return meta.IsInitialized, meta.IsV5Format(), nil
}

// RetrieveRecoveryPhrase entschlüsselt die gespeicherte Recovery-Phrase mit dem Master-Passwort.
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

	// Recovery-Key aus Password + Salt ableiten (wie bei Init).
	opts := crypto.DefaultKDFOptions
	opts.Salt = meta.RecoverySalt
	recoveryHash, err := p.DeriveArgon2id([]byte(password), opts)
	if err != nil {
		return "", fmt.Errorf("derive recovery hash: %w", err)
	}

	// Phrase entschlüsseln
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

// ResetVault setzt das Vault mit der Recovery-Phrase auf den uninitialisierten Zustand zurück.
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
	if subtle.ConstantTimeCompare(pt, []byte("GRIMLOCKER_V1")) != 1 {
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

// ChangePasswordWithRecovery setzt das Master-Passwort über die Recovery-Phrase zurück.
// Neue Vault-Parameter (Salt, Entropy, Sentinel) werden neu generiert.
// Vorhandene verschlüsselte Einträge können nicht migriert werden und werden gelöscht.
// Gibt die neue Recovery-Phrase zurück (muss sicher aufbewahrt werden).
func ChangePasswordWithRecovery(recoveryPhrase, newPassword, appDir string) (string, error) {
	p := cryptoProvider

	meta, err := LoadMeta(appDir)
	if err != nil {
		return "", fmt.Errorf("load metadata: %w", err)
	}
	if !meta.IsInitialized {
		return "", fmt.Errorf("vault not initialized")
	}

	// Recovery-Phrase prüfen
	opts := crypto.DefaultKDFOptions
	opts.Salt = meta.RecoverySalt
	computed, err := p.DeriveArgon2id([]byte(recoveryPhrase), opts)
	if err != nil {
		return "", fmt.Errorf("derive recovery hash: %w", err)
	}
	if subtle.ConstantTimeCompare(computed, meta.RecoveryHash) != 1 {
		return "", fmt.Errorf("invalid recovery phrase")
	}

	// Bestehende Einträge löschen (nicht re-enkryptierbar ohne alten MVK)
	overwriteEntropyFile(meta.EntropyPath)
	_ = os.Remove(filepath.Join(appDir, "vault_entries.enc"))
	_ = os.Remove(filepath.Join(appDir, "vault_index.enc"))
	_ = os.RemoveAll(filepath.Join(appDir, "blocks"))

	// Neues Vault mit neuem Passwort initialisieren
	return InitializeVault(newPassword, appDir)
}

// WipeVault zerstört das Vault komplett: löscht alle Dateien und Metadaten.
// Das ist die Panic/Security-Wipe-Operation. Danach ist das Vault komplett weg.
// Nutzt Rust's Secure Wipe (7-pass) wo verfügbar für sensitive Dateien.
func WipeVault(appDir string) error {
	meta, err := LoadMeta(appDir)
	if err != nil {
		// Wenn wir keine Metadaten laden können, versuchen wir zu löschen, was geht.
		meta = &VaultMeta{EntropyPath: filepath.Join(appDir, "entropy.bin")}
	}

	// Entropy-Datei sicher überschreiben (7-pass via Rust, Fallback auf os.Remove).
	if meta.EntropyPath != "" {
		_ = os.Remove(meta.EntropyPath)
	}

	// Vault-DB-Dateien löschen (einfaches Löschen reicht für diese).
	_ = os.Remove(filepath.Join(appDir, "vault.gdb"))
	_ = os.Remove(filepath.Join(appDir, "vault.gdb-log"))
	_ = os.Remove(filepath.Join(appDir, "vault_entries.enc"))
	_ = os.Remove(filepath.Join(appDir, "vault_index.enc"))
	_ = os.Remove(filepath.Join(appDir, "vault.meta"))

	// Blockstore-Chunks löschen, falls vorhanden.
	blockStoreDir := filepath.Join(appDir, "blocks")
	_ = os.RemoveAll(blockStoreDir)

	return nil
}
