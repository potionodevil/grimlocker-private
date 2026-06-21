// Package storage implementiert das Virtual Encrypted Filesystem (VFS) auf Basis
// des BlockStore. Anders als bei FUSE braucht's keinen OS-Treiber — das VFS läuft
// auf allen Plattformen, indem es Dateinamen auf verschlüsselte Block-IDs mapped.
package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/grimlocker/grimdb/engine/crypto"
)

// VFSMeta hält die Plaintext-Metadaten, die neben jeder VFS-Datei gespeichert werden.
type VFSMeta struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Mode      uint32 `json:"mode"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// VFS bietet ein dateinamen-basiertes Interface über dem verschlüsselten BlockStore.
//
// File → Block-Mapping:
//   blockID      = hex(HMAC-SHA256(filenameKey, filename))
//   metaBlockID  = blockID + "_meta"
//
// Der filenameKey wird aus dem MVK abgeleitet, sodass Block-IDs deterministisch
// sind und in O(1) aufgelöst werden können — ohne Plaintext-Index.
type VFS struct {
	bs          BlockStore
	provider    crypto.Provider
	filenameKey []byte // 32-Byte-HMAC-Key, abgeleitet vom MVK
	mvk         []byte // nur für Encryption-Vorgänge gehalten
}

// NewVFS erzeugt ein VFS backed by bs. Der MVK wird für filenameKey-Derivation
// und für die Ver-/Entschlüsselung von Datei-Inhalten und Metadaten verwendet.
func NewVFS(bs BlockStore, p crypto.Provider, mvk []byte) (*VFS, error) {
	if len(mvk) != 32 {
		return nil, fmt.Errorf("vfs: mvk must be 32 bytes, got %d", len(mvk))
	}

	// Ein separater filenameKey wird abgeleitet, damit eine bekannte Block-ID
	// nicht den Encryption-Key verrät (und umgekehrt).
	fnKey, err := p.DeriveHKDF(mvk, nil, []byte("grimlocker-vfs-filename-v1"), 32)
	if err != nil {
		return nil, fmt.Errorf("vfs: derive filename key: %w", err)
	}

	return &VFS{
		bs:          bs,
		provider:    p,
		filenameKey: fnKey,
		mvk:         mvk,
	}, nil
}

// Write encryptet data und speichert es als Block plus verschlüsselte Metadaten.
func (v *VFS) Write(name string, data []byte) error {
	if name == "" {
		return fmt.Errorf("vfs: filename must not be empty")
	}

	blockID := v.blockID(name)

	// File-Content encrypten.
	nonce, err := v.provider.NewNonce()
	if err != nil {
		return fmt.Errorf("vfs write %q: nonce: %w", name, err)
	}
	ct, err := v.provider.Encrypt(v.mvk, nonce[:], data, nil)
	if err != nil {
		return fmt.Errorf("vfs write %q: encrypt: %w", name, err)
	}

	now := time.Now().UnixNano()
	existing, readErr := v.bs.ReadBlock(blockID)
	createdAt := now
	if readErr == nil {
		createdAt = existing.CreatedAt
	}

	block := Block{
		ID:        blockID,
		Nonce:     nonce[:],
		Data:      ct,
		CreatedAt: createdAt,
		UpdatedAt: now,
	}
	if err := v.bs.WriteBlock(block); err != nil {
		return fmt.Errorf("vfs write %q: store block: %w", name, err)
	}

	// Metadaten verschlüsselt ablegen.
	meta := VFSMeta{
		Name:      name,
		Size:      int64(len(data)),
		Mode:      0600,
		CreatedAt: createdAt,
		UpdatedAt: now,
	}
	return v.writeMeta(blockID, meta)
}

// Read entschlüsselt und gibt den Inhalt der genannten Datei zurück.
func (v *VFS) Read(name string) ([]byte, error) {
	blockID := v.blockID(name)

	block, err := v.bs.ReadBlock(blockID)
	if err != nil {
		return nil, fmt.Errorf("vfs read %q: %w", name, err)
	}
	if len(block.Nonce) != 12 {
		return nil, fmt.Errorf("vfs read %q: invalid nonce length %d", name, len(block.Nonce))
	}

	pt, err := v.provider.Decrypt(v.mvk, block.Nonce, block.Data, nil)
	if err != nil {
		return nil, fmt.Errorf("vfs read %q: decrypt: %w", name, err)
	}
	return pt, nil
}

// Delete entfernt den Daten-Block und den Meta-Block für name.
func (v *VFS) Delete(name string) error {
	blockID := v.blockID(name)

	if err := v.bs.DeleteBlock(blockID); err != nil {
		return fmt.Errorf("vfs delete %q data: %w", name, err)
	}
	// Metadaten-Löschung ist best-effort; not-found-Fehler werden ignoriert.
	_ = v.bs.DeleteBlock(blockID + "_meta")
	return nil
}

// Stat gibt die entschlüsselten Metadaten für name zurück.
func (v *VFS) Stat(name string) (VFSMeta, error) {
	blockID := v.blockID(name)
	return v.readMeta(blockID)
}

// List gibt die Plaintext-Namen aller Dateien im VFS zurück.
// Iteriert über alle Blöcke, überspringt Meta-Blöcke und entschlüsselt _meta-Blöcke.
func (v *VFS) List() ([]string, error) {
	metas, err := v.bs.ListBlocks()
	if err != nil {
		return nil, fmt.Errorf("vfs list: %w", err)
	}

	var names []string
	for _, bm := range metas {
		// Meta-Blöcke überspringen — die werden via Stat abgefragt, nicht via List.
		if len(bm.ID) > 5 && bm.ID[len(bm.ID)-5:] == "_meta" {
			continue
		}
		meta, err := v.readMeta(bm.ID)
		if err != nil {
			continue
		}
		names = append(names, meta.Name)
	}
	return names, nil
}

// --- private helpers ---

// blockID berechnet die deterministische Block-ID aus dem Dateinamen.
// HMAC-SHA256 verhindert Enumeration der Dateinamen.
func (v *VFS) blockID(name string) string {
	mac := hmac.New(sha256.New, v.filenameKey)
	mac.Write([]byte(name))
	return hex.EncodeToString(mac.Sum(nil))
}

func (v *VFS) writeMeta(blockID string, meta VFSMeta) error {
	metaJSON, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("vfs meta marshal: %w", err)
	}

	nonce, err := v.provider.NewNonce()
	if err != nil {
		return err
	}
	ct, err := v.provider.Encrypt(v.mvk, nonce[:], metaJSON, nil)
	if err != nil {
		return fmt.Errorf("vfs meta encrypt: %w", err)
	}

	return v.bs.WriteBlock(Block{
		ID:        blockID + "_meta",
		Nonce:     nonce[:],
		Data:      ct,
		CreatedAt: meta.CreatedAt,
		UpdatedAt: meta.UpdatedAt,
	})
}

func (v *VFS) readMeta(blockID string) (VFSMeta, error) {
	block, err := v.bs.ReadBlock(blockID + "_meta")
	if err != nil {
		return VFSMeta{}, fmt.Errorf("vfs meta read: %w", err)
	}
	if len(block.Nonce) != 12 {
		return VFSMeta{}, fmt.Errorf("vfs meta: invalid nonce")
	}

	pt, err := v.provider.Decrypt(v.mvk, block.Nonce, block.Data, nil)
	if err != nil {
		return VFSMeta{}, fmt.Errorf("vfs meta decrypt: %w", err)
	}

	var meta VFSMeta
	if err := json.Unmarshal(pt, &meta); err != nil {
		return VFSMeta{}, fmt.Errorf("vfs meta unmarshal: %w", err)
	}
	return meta, nil
}
