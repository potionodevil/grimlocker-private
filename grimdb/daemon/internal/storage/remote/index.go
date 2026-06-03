//go:build enterprise

package remote

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"

	"github.com/grimlocker/grimdb/engine/storage"
)

const indexObjectKey = "_index/manifest.json.enc"

// remoteIndex manages the encrypted block-metadata index on S3.
type remoteIndex struct {
	entries map[string]storage.BlockMeta
}

func newRemoteIndex() *remoteIndex {
	return &remoteIndex{entries: make(map[string]storage.BlockMeta)}
}

// list returns all BlockMeta entries.
func (ri *remoteIndex) list() []storage.BlockMeta {
	out := make([]storage.BlockMeta, 0, len(ri.entries))
	for _, m := range ri.entries {
		out = append(out, m)
	}
	return out
}

// query returns BlockMeta entries matching the given category.
func (ri *remoteIndex) query(cat storage.Category) []storage.BlockMeta {
	if cat == "" {
		return ri.list()
	}
	var out []storage.BlockMeta
	for _, m := range ri.entries {
		if m.Category == cat {
			out = append(out, m)
		}
	}
	return out
}

// set adds or updates a BlockMeta entry.
func (ri *remoteIndex) set(meta storage.BlockMeta) { ri.entries[meta.ID] = meta }

// delete removes an entry by ID.
func (ri *remoteIndex) delete(id string) { delete(ri.entries, id) }

// save encrypts the index with the MVK and uploads it to S3.
func (ri *remoteIndex) save(ctx context.Context, s3c s3Client, bucket string, mvk []byte) error {
	plaintext, err := json.Marshal(ri.entries)
	if err != nil {
		return fmt.Errorf("marshal index: %w", err)
	}
	ct, err := aesgcmEncrypt(mvk, plaintext)
	if err != nil {
		return fmt.Errorf("encrypt index: %w", err)
	}
	return s3c.putObject(ctx, bucket, indexObjectKey, bytes.NewReader(ct), int64(len(ct)))
}

// load downloads and decrypts the index from S3.
func (ri *remoteIndex) load(ctx context.Context, s3c s3Client, bucket string, mvk []byte) error {
	ct, err := s3c.getObject(ctx, bucket, indexObjectKey)
	if err != nil {
		// Index doesn't exist yet (empty vault).
		return nil
	}

	plaintext, err := aesgcmDecrypt(mvk, ct)
	if err != nil {
		return fmt.Errorf("decrypt index: %w", err)
	}

	var entries map[string]storage.BlockMeta
	if err := json.Unmarshal(plaintext, &entries); err != nil {
		return fmt.Errorf("unmarshal index: %w", err)
	}
	ri.entries = entries
	return nil
}

// ── AES-256-GCM helpers ───────────────────────────────────────────────────────

func aesgcmEncrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:32])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nonce, nonce, plaintext, nil)
	return ct, nil
}

func aesgcmDecrypt(key, ct []byte) ([]byte, error) {
	block, err := aes.NewCipher(key[:32])
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(ct) < ns {
		return nil, fmt.Errorf("ciphertext too short")
	}
	return gcm.Open(nil, ct[:ns], ct[ns:], nil)
}
