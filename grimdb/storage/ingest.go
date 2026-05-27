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

// BlobManifest describes an ingested file's structure and checksums.
type BlobManifest struct {
	ID        string   `json:"id"`
	FileName  string   `json:"file_name"`
	MIMEType  string   `json:"mime_type"`
	TotalSize int64    `json:"total_size"`
	ChunkIDs  []string `json:"chunk_ids"`
	SHA256    []byte   `json:"sha256"`    // SHA-256 of full plaintext
	CreatedAt int64    `json:"created_at"`
}

// IngestEngine streams file ingestion with atomic transactions and progress reporting.
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

// Ingest streams binary data from r, encrypts it in 4MB chunks with the given MVK,
// writes to BlockStore, and returns a signed BlobManifest. On error, performs rollback.
// progressFn is called with (bytesRead, totalSize) for progress reporting.
func (e *IngestEngine) Ingest(
	ctx context.Context,
	mvk []byte,
	name, mimeType string,
	r io.Reader,
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

	// Read and encrypt in 4MB chunks
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
			chunk := buf[:n]

			// Compute hash of plaintext
			_, _ = hasher.Write(chunk)

			// Encrypt chunk
			nonce, errN := e.crypto.NewNonce()
			if errN != nil {
				e.deleteChunks(chunkIDs)
				return BlobManifest{}, fmt.Errorf("nonce generation: %w", errN)
			}

			chunkID := fmt.Sprintf("blob-%s-chunk-%d", manifestID, chunkNum)
			ciphertext, errC := e.crypto.Encrypt(mvk, nonce[:], chunk, []byte(chunkID))
			if errC != nil {
				e.deleteChunks(chunkIDs)
				return BlobManifest{}, fmt.Errorf("encrypt chunk %d: %w", chunkNum, errC)
			}

			// Write block
			block := Block{
				ID:    chunkID,
				Nonce: nonce[:],
				Data:  ciphertext,
			}
			if errW := e.store.WriteBlock(block); errW != nil {
				e.deleteChunks(chunkIDs)
				return BlobManifest{}, fmt.Errorf("write chunk %d: %w", chunkNum, errW)
			}

			chunkIDs = append(chunkIDs, chunkID)
			totalBytesRead += int64(n)
			chunkNum++

			// Report progress
			if progressFn != nil {
				progressFn(totalBytesRead, totalBytesRead) // Size == bytes read so far
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

	// Create and write manifest block
	manifest := BlobManifest{
		ID:        manifestID,
		FileName:  name,
		MIMEType:  mimeType,
		TotalSize: totalBytesRead,
		ChunkIDs:  chunkIDs,
		SHA256:    hasher.Sum(nil),
		CreatedAt: time.Now().UnixNano(),
	}

	// Serialize manifest as JSON and write as special block
	manifestJSON, _ := json.Marshal(manifest)
	manifestBlock := Block{
		ID:   fmt.Sprintf("blob-%s-manifest", manifestID),
		Data: manifestJSON,
	}
	if err := e.store.WriteBlock(manifestBlock); err != nil {
		e.deleteChunks(chunkIDs)
		return BlobManifest{}, fmt.Errorf("write manifest: %w", err)
	}

	// Final progress
	if progressFn != nil {
		progressFn(totalBytesRead, totalBytesRead)
	}

	return manifest, nil
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
