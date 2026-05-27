package grimdb

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
)

// VaultMeta is the JSON schema for vault.meta. Unchanged from the original
// grimdb-go format to maintain backwards compatibility.
type VaultMeta struct {
	IsInitialized            bool   `json:"is_initialized"`
	ArgonSalt                []byte `json:"argon_salt,omitempty"`
	Sentinel                 []byte `json:"sentinel,omitempty"`
	EntropyPath              string `json:"entropy_path,omitempty"`
	RecoveryHash             []byte `json:"recovery_hash,omitempty"`
	RecoverySalt             []byte `json:"recovery_salt,omitempty"`
	RecoveryPhraseCiphertext []byte `json:"recovery_phrase_ciphertext,omitempty"` // Encrypted recovery phrase
	AutoLockMinutes          int    `json:"auto_lock_minutes,omitempty"`
	LockdownThreshold        int    `json:"lockdown_threshold,omitempty"`
}

// IsV5Format returns true when the metadata uses the V5 Argon2id+coordinate format.
func (m *VaultMeta) IsV5Format() bool {
	return len(m.ArgonSalt) == 16
}

const metaFileName = "vault.meta"

// LoadMeta reads and unmarshals vault.meta from appDir.
func LoadMeta(appDir string) (*VaultMeta, error) {
	data, err := os.ReadFile(filepath.Join(appDir, metaFileName))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("metadata not found")
		}
		return nil, fmt.Errorf("read metadata: %w", err)
	}

	var meta VaultMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal metadata: %w", err)
	}
	return &meta, nil
}

// SaveMeta atomically writes vault.meta to appDir.
func SaveMeta(appDir string, m *VaultMeta) error {
	metaPath := filepath.Join(appDir, metaFileName)
	tmpPath := metaPath + ".tmp"

	data, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal metadata: %w", err)
	}

	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return fmt.Errorf("write metadata tmp: %w", err)
	}

	if err := os.Rename(tmpPath, metaPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("rename metadata: %w", err)
	}
	log.Printf("[vault] vault.meta saved (len=%d Bytes)", len(data))
	return nil
}
