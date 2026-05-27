package storage

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/grimlocker/grimdb/crypto"
)

const ingestChunkSize = 4 * 1024 * 1024 // 4MB chunks

// BlobManifest describes an ingested file's structure, checksums and encoding.
type BlobManifest struct {
	ID         string   `json:"id"`
	FileName   string   `json:"file_name"`
	MIMEType   string   `json:"mime_type"`
	TotalSize  int64    `json:"total_size"`
	ChunkIDs   []string `json:"chunk_ids"`
	SHA256     []byte   `json:"sha256"`     // SHA-256 of full plaintext (before compression)
	Compressed bool     `json:"compressed"` // true → chunks were zstd-compressed before encryption
	Algorithm  string   `json:"algorithm"`  // "zstd" | "none" — compression algorithm used
	CreatedAt  int64    `json:"created_at"`
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
		return BlobManifest{}, fmt.Errorf("mvk not provided")
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
			return BlobManifest{}, fmt.Errorf("ingest cancelled")
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
				return BlobManifest{}, fmt.Errorf("nonce generation: %w", errN)
			}

			chunkID := fmt.Sprintf("blob-%s-chunk-%d", manifestID, chunkNum)
			ciphertext, errC := e.crypto.Encrypt(mvk, nonce[:], chunkToEncrypt, []byte(chunkID))
			if errC != nil {
				e.deleteChunks(chunkIDs)
				return BlobManifest{}, fmt.Errorf("encrypt chunk %d: %w", chunkNum, errC)
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
				return BlobManifest{}, fmt.Errorf("write chunk %d: %w", chunkNum, errW)
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
			return BlobManifest{}, fmt.Errorf("read error: %w", err)
		}
	}

	algorithm := "none"
	if compress {
		algorithm = "zstd"
	}

	// Build and write manifest block.
	manifest := BlobManifest{
		ID:         manifestID,
		FileName:   name,
		MIMEType:   mimeType,
		TotalSize:  totalBytesRead,
		ChunkIDs:   chunkIDs,
		SHA256:     hasher.Sum(nil),
		Compressed: compress,
		Algorithm:  algorithm,
		CreatedAt:  time.Now().UnixNano(),
	}

	manifestJSON, _ := json.Marshal(manifest)
	manifestBlock := Block{
		ID:       fmt.Sprintf("blob-%s-manifest", manifestID),
		Data:     manifestJSON,
		Category: CategoryFileVault,
	}
	if err := e.store.WriteBlock(manifestBlock); err != nil {
		e.deleteChunks(chunkIDs)
		return BlobManifest{}, fmt.Errorf("write manifest: %w", err)
	}

	// Final progress callback.
	if progressFn != nil {
		progressFn(totalBytesRead, totalBytesRead)
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
		return fmt.Errorf("mvk not provided")
	}

	for i, chunkID := range manifest.ChunkIDs {
		select {
		case <-ctx.Done():
			return fmt.Errorf("retrieve cancelled at chunk %d", i)
		default:
		}

		block, err := e.store.ReadBlock(chunkID)
		if err != nil {
			return fmt.Errorf("read chunk %d (%s): %w", i, chunkID, err)
		}

		// Decrypt.
		if len(block.Nonce) < 12 {
			return fmt.Errorf("chunk %d: nonce too short", i)
		}
		plaintext, err := e.crypto.Decrypt(mvk, block.Nonce, block.Data, []byte(chunkID))
		if err != nil {
			return fmt.Errorf("decrypt chunk %d: %w", i, err)
		}

		// Decompress (marker-aware — handles both old and new blocks).
		decompressed, err := Decompress(plaintext)
		if err != nil {
			return fmt.Errorf("decompress chunk %d: %w", i, err)
		}

		if _, err := w.Write(decompressed); err != nil {
			return fmt.Errorf("write chunk %d: %w", i, err)
		}
	}

	return nil
}

// deleteChunks performs rollback by deleting all written chunk blocks.
func (e *IngestEngine) deleteChunks(chunkIDs []string) {
	for _, id := range chunkIDs {
		_ = e.store.DeleteBlock(id)
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
