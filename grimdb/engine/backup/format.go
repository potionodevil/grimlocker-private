// Package backup (format.go) implementiert die Serialisierung des verschlüsselten Payloads.
//
// Das Payload-Format (nach Entschlüsselung, Big-Endian):
//
//	4 bytes   NumBlocks (uint32)
//	Für jeden Block:
//	  4 bytes  BlockPayloadLen (uint32)
//	  N bytes  JSON-kodierter storage.Block
//	4 bytes   VaultMetaLen (uint32)
//	N bytes   JSON-kodiertes VaultMetaSnapshot
//
// storage.Block enthält die originalen verschlüsselten Daten aus dem Vault.
// Der Backup-Layer verschlüsselt sie ein zweites Mal (Outer Layer via buildBlob).
package backup

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"

	"github.com/grimlocker/grimdb/engine/storage"
)

// VaultMetaSnapshot ist ein sicherer Subset der Vault-Metadaten, der im Backup gespeichert wird.
// RecoveryPhraseCiphertext wird bewusst ausgeschlossen (separater Restore-Pfad).
type VaultMetaSnapshot struct {
	ArgonSalt    []byte `json:"argon_salt"`
	RecoveryHash []byte `json:"recovery_hash,omitempty"`
	ExportedAt   int64  `json:"exported_at"`
	Version      string `json:"version"`
}

// EncodePayload serialisiert Blocks + VaultMetaSnapshot in das Payload-Binärformat.
// Der resultierende Byte-Slice wird anschließend vom Aufrufer verschlüsselt.
func EncodePayload(blocks []storage.Block, meta VaultMetaSnapshot) ([]byte, error) {
	var buf bytes.Buffer

	// NumBlocks
	var nb [4]byte
	binary.BigEndian.PutUint32(nb[:], uint32(len(blocks)))
	buf.Write(nb[:])

	// Jeder Block als length-prefixed JSON
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

	// VaultMetaSnapshot als length-prefixed JSON
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

// DecodePayload deserialisiert den entschlüsselten Payload zurück in Blocks + VaultMetaSnapshot.
func DecodePayload(data []byte) ([]storage.Block, VaultMetaSnapshot, error) {
	r := bytes.NewReader(data)

	// NumBlocks
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

	// VaultMetaSnapshot
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
