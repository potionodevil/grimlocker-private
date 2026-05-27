// Package storage provides the virtual encrypted filesystem (VFS) built on top
// of the BlockStore. No OS FUSE driver is required — the VFS works on all
// platforms by mapping file names to encrypted Block IDs.
package storage

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/grimlocker/grimdb/crypto"
)

// VFSMeta holds the plaintext metadata stored alongside every VFS file.
type VFSMeta struct {
	Name      string `json:"name"`
	Size      int64  `json:"size"`
	Mode      uint32 `json:"mode"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// VFS presents a file-named interface over an encrypted BlockStore.
//
// File → Block mapping:
//   blockID      = hex(HMAC-SHA256(filenameKey, filename))
//   metaBlockID  = blockID + "_meta"
//
// The filename key is derived from the MVK so that block IDs are deterministic
// and can be resolved in O(1) without a plaintext index.
type VFS struct {
	bs          BlockStore
	provider    crypto.Provider
	filenameKey []byte // 32-byte HMAC key derived from MVK
	mvk         []byte // held only for encryption operations
}

// NewVFS creates a VFS backed by bs. mvk is used to derive the filename key
// and as the encryption key for file contents and metadata.
func NewVFS(bs BlockStore, p crypto.Provider, mvk []byte) (*VFS, error) {
	if len(mvk) != 32 {
		return nil, fmt.Errorf("vfs: mvk must be 32 bytes, got %d", len(mvk))
	}

	// Derive a separate filename key so that knowing a blockID does not reveal
	// the encryption key (and vice versa).
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

// Write encrypts data and stores it as a Block, then stores encrypted metadata.
func (v *VFS) Write(name string, data []byte) error {
	if name == "" {
		return fmt.Errorf("vfs: filename must not be empty")
	}

	blockID := v.blockID(name)

	// Encrypt the file content.
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

	// Store encrypted metadata.
	meta := VFSMeta{
		Name:      name,
		Size:      int64(len(data)),
		Mode:      0600,
		CreatedAt: createdAt,
		UpdatedAt: now,
	}
	return v.writeMeta(blockID, meta)
}

// Read decrypts and returns the content of the named file.
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

// Delete removes the data block and metadata block for name.
func (v *VFS) Delete(name string) error {
	blockID := v.blockID(name)

	if err := v.bs.DeleteBlock(blockID); err != nil {
		return fmt.Errorf("vfs delete %q data: %w", name, err)
	}
	// Best-effort metadata deletion; ignore not-found errors.
	_ = v.bs.DeleteBlock(blockID + "_meta")
	return nil
}

// Stat returns the decrypted metadata for name.
func (v *VFS) Stat(name string) (VFSMeta, error) {
	blockID := v.blockID(name)
	return v.readMeta(blockID)
}

// List returns the plaintext names of all files in the VFS.
// It iterates all blocks, skips metadata blocks, and decrypts each _meta block.
func (v *VFS) List() ([]string, error) {
	metas, err := v.bs.ListBlocks()
	if err != nil {
		return nil, fmt.Errorf("vfs list: %w", err)
	}

	var names []string
	for _, bm := range metas {
		// Skip metadata blocks — they are surfaced via Stat, not List.
		if len(bm.ID) > 5 && bm.ID[len(bm.ID)-5:] == "_meta" {
			continue
		}
		meta, err := v.readMeta(bm.ID)
		if err != nil {
			// Block has no metadata (e.g. a non-VFS block); skip it.
			continue
		}
		names = append(names, meta.Name)
	}
	return names, nil
}

// --- private helpers ---

// blockID returns the deterministic block ID for the given filename.
// Uses HMAC-SHA256 over the filename to prevent enumeration.
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
