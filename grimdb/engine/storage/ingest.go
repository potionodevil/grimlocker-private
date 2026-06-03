package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"time"

	"github.com/grimlocker/grimdb/engine/crypto"
	gerrors "github.com/grimlocker/grimdb/engine/errors"
)

const ingestChunkSize = 4 * 1024 * 1024 // 4MB chunks

// BlobManifest describes an ingested file's structure, checksums and encoding.
type BlobManifest struct {
	ID              string   `json:"id"`               // UUID (internal reference)
	ManifestBlockID string   `json:"manifest_block_id"` // full block store key: "blob-{uuid}-manifest"
	FileName        string   `json:"file_name"`
	MIMEType        string   `json:"mime_type"`
	TotalSize       int64    `json:"total_size"`
	ChunkIDs        []string `json:"chunk_ids"`
	SHA256          []byte   `json:"sha256"`              // SHA-256 of full plaintext (before compression)
	Compressed      bool     `json:"compressed"`          // true → chunks were zstd-compressed before encryption
	Algorithm       string   `json:"algorithm"`           // "zstd" | "none" — compression algorithm used
	FolderID        string   `json:"folder_id,omitempty"` // folder this file belongs to, "" = root
	CreatedAt       int64    `json:"created_at"`
}

// IngestEngine streams file ingestion with atomic transactions and progress reporting.
// Pipeline: Read → Hash → Compress (zstd) → Encrypt (ChaCha20-Poly1305) → Write Block
type IngestEngine struct {
	store  BlockStore
	crypto crypto.Provider
}

// NewIngestEngine creates an IngestEngine.
func NewIngestEngine(store BlockStore, cp crypto.Provider) *IngestEngine {
	return &IngestEngine{
		store:  store,
		crypto: cp,
	}
}

// Ingest streams binary data from r, optionally compresses it, encrypts it in
// 4MB chunks with the given MVK, writes to BlockStore, and returns a signed
// BlobManifest. On error, performs rollback.
// progressFn is called with (bytesRead, totalSize) for progress reporting.
func (e *IngestEngine) Ingest(
	ctx context.Context,
	mvk []byte,
	name, mimeType string,
	r io.Reader,
	progressFn func(bytesRead, totalSize int64),
) (BlobManifest, error) {
	return e.IngestWithOptions(ctx, mvk, name, mimeType, r, true, progressFn)
}

// IngestWithOptions is like Ingest but allows disabling compression
// (useful for already-compressed file types like JPEG, MP4, ZIP).
func (e *IngestEngine) IngestWithOptions(
	ctx context.Context,
	mvk []byte,
	name, mimeType string,
	r io.Reader,
	compress bool,
	progressFn func(bytesRead, totalSize int64),
) (BlobManifest, error) {
	if len(mvk) == 0 {
		return BlobManifest{}, gerrors.NewSecurityMVKMissingError("file_ingest")
	}

	manifestID := generateUUID()
	chunkIDs := []string{}
	hasher := sha256.New()
	totalBytesRead := int64(0)
	chunkNum := 0

	// Read and (optionally compress then) encrypt in 4MB chunks.
	buf := make([]byte, ingestChunkSize)
	for {
		select {
		case <-ctx.Done():
			e.deleteChunks(chunkIDs)
			return BlobManifest{}, gerrors.NewBusTimeoutError("FILE_INGEST")
		default:
		}

		n, err := r.Read(buf)
		if n > 0 {
			chunk := make([]byte, n)
			copy(chunk, buf[:n])

			// 1. Hash the plaintext before any transformation.
			_, _ = hasher.Write(chunk)

			// 2. Compress if enabled — prepends a marker byte.
			chunkToEncrypt := chunk
			if compress {
				chunkToEncrypt = CompressInPlace(chunk)
			} else {
				// Prepend uncompressed marker so Decompress is always idempotent.
				marked := make([]byte, 1+len(chunk))
				marked[0] = markerUncompressed
				copy(marked[1:], chunk)
				chunkToEncrypt = marked
			}

			// 3. Encrypt the (possibly compressed) chunk.
			nonce, errN := e.crypto.NewNonce()
			if errN != nil {
				e.deleteChunks(chunkIDs)
				return BlobManifest{}, gerrors.NewCryptoEncryptionError("nonce_generation_chunk", errN)
			}

			chunkID := fmt.Sprintf("blob-%s-chunk-%d", manifestID, chunkNum)
			ciphertext, errC := e.crypto.Encrypt(mvk, nonce[:], chunkToEncrypt, []byte(chunkID))
			if errC != nil {
				e.deleteChunks(chunkIDs)
				return BlobManifest{}, gerrors.NewCryptoEncryptionError(
					fmt.Sprintf("encrypt_chunk_%d", chunkNum), errC)
			}

			// 4. Write encrypted block.
			block := Block{
				ID:       chunkID,
				Nonce:    nonce[:],
				Data:     ciphertext,
				Category: CategoryFileVault,
			}
			if errW := e.store.WriteBlock(block); errW != nil {
				e.deleteChunks(chunkIDs)
				return BlobManifest{}, gerrors.NewStorageIOError(
					fmt.Sprintf("write_chunk_%d", chunkNum), chunkID, errW)
			}

			chunkIDs = append(chunkIDs, chunkID)
			totalBytesRead += int64(n)
			chunkNum++

			// Report progress.
			if progressFn != nil {
				progressFn(totalBytesRead, totalBytesRead)
			}
		}

		if err != nil {
			if err == io.EOF {
				break
			}
			e.deleteChunks(chunkIDs)
			return BlobManifest{}, gerrors.NewStorageIOError("read_file_data", manifestID, err)
		}
	}

	algorithm := "none"
	if compress {
		algorithm = "zstd"
	}

	// Build and write manifest block.
	manifest := BlobManifest{
		ID:              manifestID,
		ManifestBlockID: fmt.Sprintf("blob-%s-manifest", manifestID),
		FileName:        name,
		MIMEType:        mimeType,
		TotalSize:       totalBytesRead,
		ChunkIDs:        chunkIDs,
		SHA256:          hasher.Sum(nil),
		Compressed:      compress,
		Algorithm:       algorithm,
		CreatedAt:       time.Now().UnixNano(),
	}

	manifestJSON, _ := json.Marshal(manifest)
	manifestBlock := Block{
		ID:       fmt.Sprintf("blob-%s-manifest", manifestID),
		Data:     manifestJSON,
		Category: CategoryFileVault,
	}
	manifestBlockID := fmt.Sprintf("blob-%s-manifest", manifestID)
	if err := e.store.WriteBlock(manifestBlock); err != nil {
		e.deleteChunks(chunkIDs)
		return BlobManifest{}, gerrors.NewStorageIOError("write_manifest", manifestBlockID, err)
	}

	// Critical: flush the index to disk after writing manifest.
	// Without this, the index may not be persisted and data can become
	// inconsistent if the connection closes before the next operation.
	if err := e.store.Flush(); err != nil {
		log.Printf("[IngestEngine] [Code %d] Flush after manifest write failed: %v — rolling back",
			gerrors.ErrCodeStorageIndexFailed, err)
		e.deleteChunks(chunkIDs)
		return BlobManifest{}, gerrors.NewStorageIndexError("flush_after_ingest", err)
	}

	// Final progress callback.
	if progressFn != nil {
		progressFn(totalBytesRead, totalBytesRead)
	}

	return manifest, nil
}

// ReadManifest reads a BlobManifest from the block store by its manifest block ID.
// The manifest block ID is of the form "blob-{uuid}-manifest".
func (e *IngestEngine) ReadManifest(manifestBlockID string) (BlobManifest, error) {
	block, err := e.store.ReadBlock(manifestBlockID)
	if err != nil {
		return BlobManifest{}, err
	}

	var manifest BlobManifest
	if err := json.Unmarshal(block.Data, &manifest); err != nil {
		return BlobManifest{}, gerrors.NewStorageCorruptionError("unmarshal_manifest", manifestBlockID,
			map[string]string{"json_error": err.Error()})
	}
	if manifest.ID == "" {
		return BlobManifest{}, gerrors.NewStorageCorruptionError("invalid_manifest", manifestBlockID,
			map[string]string{"reason": "manifest ID is empty"})
	}

	return manifest, nil
}

// RetrieveBlob decrypts and optionally decompresses all chunks for a given
// BlobManifest and writes the plaintext to w.
func (e *IngestEngine) RetrieveBlob(
	ctx context.Context,
	mvk []byte,
	manifest BlobManifest,
	w io.Writer,
) error {
	if len(mvk) == 0 {
		return gerrors.NewSecurityMVKMissingError("file_retrieve")
	}

	for i, chunkID := range manifest.ChunkIDs {
		select {
		case <-ctx.Done():
			return gerrors.NewBusTimeoutError("FILE_RETRIEVE").
				WithDetails("chunk_index", fmt.Sprintf("%d", i))
		default:
		}

		block, err := e.store.ReadBlock(chunkID)
		if err != nil {
			// Preserve the typed error from ReadBlock (e.g. StorageNotFound, StorageCorruption)
			// but add retrieve context on top if it's a plain error.
			if _, ok := err.(*gerrors.GrimlockError); ok {
				return err
			}
			return gerrors.NewStorageIOError(
				fmt.Sprintf("read_chunk_%d", i), chunkID, err)
		}

		// Decrypt.
		if len(block.Nonce) < 12 {
			return gerrors.NewStorageCorruptionError(
				fmt.Sprintf("nonce_too_short_chunk_%d", i), chunkID,
				map[string]string{"nonce_len": fmt.Sprintf("%d", len(block.Nonce))})
		}
		plaintext, err := e.crypto.Decrypt(mvk, block.Nonce, block.Data, []byte(chunkID))
		if err != nil {
			return gerrors.NewCryptoDecryptionError(chunkID, err)
		}

		// Decompress (marker-aware — handles both old and new blocks).
		decompressed, err := Decompress(plaintext)
		if err != nil {
			return gerrors.NewStorageCorruptionError(
				fmt.Sprintf("decompress_chunk_%d", i), chunkID,
				map[string]string{"decompress_error": err.Error()})
		}

		if _, err := w.Write(decompressed); err != nil {
			return gerrors.NewStorageIOError(
				fmt.Sprintf("write_output_chunk_%d", i), chunkID, err)
		}
	}

	return nil
}

// deleteChunks performs rollback by deleting all written chunk blocks.
// Failures are logged but not returned — rollback is best-effort.
func (e *IngestEngine) deleteChunks(chunkIDs []string) {
	for _, id := range chunkIDs {
		if err := e.store.DeleteBlock(id); err != nil {
			log.Printf("[IngestEngine] [Code %d] rollback: failed to delete chunk %s: %v",
				gerrors.ErrCodeStorageIO, id, err)
		}
	}
}

// generateUUID generates a random UUID v4 string.
func generateUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant bits
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
