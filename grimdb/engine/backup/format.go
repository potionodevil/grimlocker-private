// Package backup (format.go) implements serialization of the encrypted payload.
//
// Payload format (after decryption, big-endian):
//
//	4 bytes   NumBlocks (uint32)
//	For each block:
//	  4 bytes  BlockPayloadLen (uint32)
//	  N bytes  JSON-encoded storage.Block
//	4 bytes   VaultMetaLen (uint32)
//	N bytes   JSON-encoded VaultMetaSnapshot
//
// storage.Block entries carry their original ciphertext from the vault.
// The backup layer adds a second encryption layer (outer key from buildBlob).
package backup

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/grimlocker/grimdb/engine/storage"
)

// VaultMetaSnapshot is the safe subset of vault metadata stored in a backup.
// RecoveryPhraseCiphertext is deliberately excluded (separate restore path).
type VaultMetaSnapshot struct {
	ArgonSalt  []byte `json:"argon_salt"`
	RecoveryHash []byte `json:"recovery_hash,omitempty"`
	ExportedAt int64  `json:"exported_at"`
	Version    string `json:"version"`
}

// EncodePayload serializes blocks + VaultMetaSnapshot into the payload binary format.
// The resulting byte slice is then encrypted by the caller.
func EncodePayload(blocks []storage.Block, meta VaultMetaSnapshot) ([]byte, error) {
	var buf bytes.Buffer

	var nb [4]byte
	binary.BigEndian.PutUint32(nb[:], uint32(len(blocks)))
	buf.Write(nb[:])

	for i, b := range blocks {
		data, err := json.Marshal(b)
		if err != nil {
			return nil, fmt.Errorf("format: marshal block[%d] id=%s: %w", i, b.ID, err)
		}
		var blen [4]byte
		binary.BigEndian.PutUint32(blen[:], uint32(len(data)))
		buf.Write(blen[:])
		buf.Write(data)
	}

	metaData, err := json.Marshal(meta)
	if err != nil {
		return nil, fmt.Errorf("format: marshal vault_meta: %w", err)
	}
	var mlen [4]byte
	binary.BigEndian.PutUint32(mlen[:], uint32(len(metaData)))
	buf.Write(mlen[:])
	buf.Write(metaData)

	return buf.Bytes(), nil
}

// DecodePayload deserializes the decrypted payload back into blocks + VaultMetaSnapshot.
func DecodePayload(data []byte) ([]storage.Block, VaultMetaSnapshot, error) {
	r := bytes.NewReader(data)

	var nb [4]byte
	if _, err := io.ReadFull(r, nb[:]); err != nil {
		return nil, VaultMetaSnapshot{}, fmt.Errorf("format: read num_blocks: %w", err)
	}
	numBlocks := binary.BigEndian.Uint32(nb[:])

	blocks := make([]storage.Block, 0, numBlocks)
	for i := uint32(0); i < numBlocks; i++ {
		var blen [4]byte
		if _, err := io.ReadFull(r, blen[:]); err != nil {
			return nil, VaultMetaSnapshot{}, fmt.Errorf("format: read block[%d] len: %w", i, err)
		}
		blockData := make([]byte, binary.BigEndian.Uint32(blen[:]))
		if _, err := io.ReadFull(r, blockData); err != nil {
			return nil, VaultMetaSnapshot{}, fmt.Errorf("format: read block[%d] data: %w", i, err)
		}
		var b storage.Block
		if err := json.Unmarshal(blockData, &b); err != nil {
			return nil, VaultMetaSnapshot{}, fmt.Errorf("format: unmarshal block[%d]: %w", i, err)
		}
		blocks = append(blocks, b)
	}

	var mlen [4]byte
	if _, err := io.ReadFull(r, mlen[:]); err != nil {
		return nil, VaultMetaSnapshot{}, fmt.Errorf("format: read vault_meta len: %w", err)
	}
	metaData := make([]byte, binary.BigEndian.Uint32(mlen[:]))
	if _, err := io.ReadFull(r, metaData); err != nil {
		return nil, VaultMetaSnapshot{}, fmt.Errorf("format: read vault_meta data: %w", err)
	}
	var meta VaultMetaSnapshot
	if err := json.Unmarshal(metaData, &meta); err != nil {
		return nil, VaultMetaSnapshot{}, fmt.Errorf("format: unmarshal vault_meta: %w", err)
	}

	return blocks, meta, nil
}
