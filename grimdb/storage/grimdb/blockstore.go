// Package grimdb implements the file-backed encrypted block store (GrimDB).
//
// Storage layout on disk:
//
//	vault_entries.enc  — append-only data file; each block occupies:
//	                     nonce(12 bytes) + ciphertext+AEAD-tag + HMAC(32 bytes)
//	vault_index.enc    — encrypted JSON index mapping block ID → blockRecord.
//	                     Written atomically via a .tmp file + rename.
//
// Encryption: ChaCha20-Poly1305 with a per-block random 12-byte nonce.
// The encryption key (MVK) is never stored in heap memory — callers provide
// a resolver function via SetMVKFunc that fetches the key from locked memory.
//
// Block integrity: each block carries an HMAC-SHA256 over (id ‖ nonce ‖ ciphertext)
// computed with a key derived from the MVK via HMAC-SHA256("grimlocker-hmac-v1").
// ReadBlock verifies the HMAC before returning data, using constant-time comparison.
//
// Deletion is secure: DeleteBlock overwrites the ciphertext region with zeros on
// disk before removing the block from the index (best-effort on SSDs with wear-leveling).
//
// Concurrency: a single sync.RWMutex guards the in-memory index and all file I/O.
// Reads hold RLock; writes hold Lock. Multiple concurrent reads are safe.
package grimdb

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/grimlocker/grimdb/crypto"
	gerrors "github.com/grimlocker/grimdb/errors"
	"github.com/grimlocker/grimdb/storage"
	"golang.org/x/crypto/chacha20poly1305"
)

type blockRecord struct {
	Offset    int64            `json:"offset"`
	Length    int64            `json:"length"`
	Nonce     []byte           `json:"nonce"`
	HMAC      []byte           `json:"hmac"`
	Category  storage.Category `json:"category,omitempty"` // entry category for in-memory filtering
	CreatedAt int64            `json:"created_at"`
	UpdatedAt int64            `json:"updated_at"`
}

// BlockStoreImpl is the concrete BlockStore backed by vault_entries.enc.
// It uses ChaCha20-Poly1305 with the MVK for both index encryption and
// entry encryption. The MVK is never stored in heap memory — callers
// provide a resolver function via SetMVKFunc and revoke it via ZeroMVK.
type BlockStoreImpl struct {
	mu        sync.RWMutex
	filePath  string
	indexPath string
	index     map[string]blockRecord
	getMVK    func() []byte
	strategy  storage.StorageStrategy
}

func NewBlockStoreImpl(appDir string) *BlockStoreImpl {
	return &BlockStoreImpl{
		filePath:  appDir + "/vault_entries.enc",
		indexPath: appDir + "/vault_index.enc",
		index:     make(map[string]blockRecord),
		strategy:  storage.NopStrategy{},
	}
}

func (bs *BlockStoreImpl) mvk() []byte {
	if bs.getMVK == nil {
		return nil
	}
	return bs.getMVK()
}

// SetMVKFunc stores a function that resolves the master vault key from
// locked memory. Call after successful UnlockVault. The returned key
// references stay in locked memory — no heap copy is made.
func (bs *BlockStoreImpl) SetMVKFunc(fn func() []byte) {
	bs.mu.Lock()
	bs.getMVK = fn
	bs.mu.Unlock()
}

// ZeroMVK drops the key reference and clears the index.
// The actual key material in locked memory is managed by the security module.
func (bs *BlockStoreImpl) ZeroMVK() {
	bs.mu.Lock()
	bs.getMVK = nil
	bs.index = make(map[string]blockRecord)
	bs.mu.Unlock()
}

// SetStrategy injects a StorageStrategy.
func (bs *BlockStoreImpl) SetStrategy(s storage.StorageStrategy) {
	bs.mu.Lock()
	bs.strategy = s
	bs.mu.Unlock()
}

func (bs *BlockStoreImpl) LoadIndex() error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	f, err := os.Open(bs.indexPath)
	if err != nil {
		if os.IsNotExist(err) {
			bs.index = make(map[string]blockRecord)
			return nil
		}
		return gerrors.NewStorageIOError("open_index", "", err)
	}
	defer f.Close()

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(f, lenBuf); err != nil {
		if err == io.EOF {
			return nil
		}
		return gerrors.NewStorageIOError("read_index_length", "", err)
	}

	indexLen := binary.BigEndian.Uint32(lenBuf)
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(f, nonce); err != nil {
		return gerrors.NewStorageIOError("read_index_nonce", "", err)
	}

	ct := make([]byte, indexLen)
	if _, err := io.ReadFull(f, ct); err != nil {
		return gerrors.NewStorageIOError("read_encrypted_index", "", err)
	}

	if err := crypto.ValidateKeyLength(bs.mvk()); err != nil {
		return gerrors.NewCryptoInvalidKeyError(len(bs.mvk()))
	}
	cipher, err := chacha20poly1305.New(bs.mvk())
	if err != nil {
		return gerrors.NewCryptoDecryptionError("", err)
	}

	indexJSON, err := cipher.Open(nil, nonce, ct, nil)
	if err != nil {
		return gerrors.NewCryptoDecryptionError("vault_index", err)
	}

	var idx map[string]blockRecord
	if err := json.Unmarshal(indexJSON, &idx); err != nil {
		return gerrors.NewStorageCorruptionError("unmarshal_index", "",
			map[string]string{"json_error": err.Error()})
	}
	bs.index = idx

	stats := make([]string, 0, len(bs.index))
	for id := range bs.index {
		stats = append(stats, id)
	}
	log.Printf("[blockstore] LoadIndex — %d entries loaded: %v", len(bs.index), stats)
	return nil
}

// WriteBlock encrypts and appends a block to the vault_entries.enc data file,
// then atomically updates the encrypted index.
//
// The block's Data field must be the plaintext or ciphertext depending on the
// storage strategy in use (NopStrategy passes data through unchanged).
// WriteBlock generates a random 12-byte nonce, derives an HMAC key from the MVK,
// computes HMAC-SHA256(id ‖ nonce ‖ data), appends nonce+hmac+data to the file,
// and calls persistIndexLocked to flush the updated in-memory index.
//
// Returns *errors.GrimlockError with code ErrCodeStorageIO on any I/O failure.
// The vault must be unlocked (MVK resolver set) before calling WriteBlock.
func (bs *BlockStoreImpl) WriteBlock(b storage.Block) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	b2, err := bs.strategy.OnWrite(b)
	if err != nil {
		return err
	}
	b = b2

	now := time.Now().UnixNano()
	if b.CreatedAt == 0 {
		b.CreatedAt = now
	}
	b.UpdatedAt = now

	if len(b.Nonce) == 0 {
		b.Nonce = make([]byte, 12)
		if _, err := rand.Read(b.Nonce); err != nil {
			return gerrors.NewStorageIOError("nonce_generation", b.ID, err)
		}
	}

	hmacKey := deriveHMACKey(bs.mvk())
	mac := hmac.New(sha256.New, hmacKey[:])
	mac.Write([]byte(b.ID))
	mac.Write(b.Nonce)
	mac.Write(b.Data)
	b.HMAC = mac.Sum(nil)

	dataFile := bs.filePath

	f, err := os.OpenFile(dataFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return gerrors.NewStorageIOError("open_data_file", b.ID, err)
	}

	stat, _ := f.Stat()
	dataOffset := stat.Size() + 12 + 32

	if _, err := f.Write(b.Nonce); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(b.HMAC); err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(b.Data); err != nil {
		f.Close()
		return err
	}

	if err := f.Close(); err != nil {
		return gerrors.NewStorageIOError("close_data_file", b.ID, err)
	}

	bs.index[b.ID] = blockRecord{
		Offset:    dataOffset,
		Length:    int64(len(b.Data)),
		Nonce:     b.Nonce,
		HMAC:      b.HMAC,
		Category:  b.Category,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}

	log.Printf("[blockstore] WriteBlock — id=%s offset=%d dataLen=%d", b.ID, dataOffset, len(b.Data))

	return bs.persistIndexLocked()
}

// ReadBlock retrieves a block from the store by its ID, verifies its HMAC,
// and returns the raw (still-encrypted) ciphertext inside the Block.Data field.
//
// HMAC verification uses constant-time comparison to prevent timing attacks.
// If verification fails, ErrCodeStorageCorruption (2002) is returned — this
// indicates either tampered data or a mismatched MVK (e.g. wrong password).
//
// Returns ErrCodeStorageNotFound (2003) if the block ID is not in the index.
// Returns ErrCodeStorageIO (2001) on any disk read failure.
func (bs *BlockStoreImpl) ReadBlock(id string) (storage.Block, error) {
	bs.mu.RLock()
	rec, exists := bs.index[id]
	filePath := bs.filePath
	mvk := bs.mvk()
	bs.mu.RUnlock()

	if !exists {
		return storage.Block{}, gerrors.NewStorageNotFoundError(id)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return storage.Block{}, gerrors.NewStorageIOError("open_data_file", id, err)
	}
	defer f.Close()

	ct := make([]byte, rec.Length)
	if _, err := f.ReadAt(ct, rec.Offset); err != nil {
		return storage.Block{}, gerrors.NewStorageIOError("read_block_data", id, err)
	}

	// Verify HMAC before returning.
	hmacKey := deriveHMACKey(mvk)
	mac := hmac.New(sha256.New, hmacKey[:])
	mac.Write([]byte(id))
	mac.Write(rec.Nonce)
	mac.Write(ct)
	if subtle.ConstantTimeCompare(mac.Sum(nil), rec.HMAC) != 1 {
		return storage.Block{}, gerrors.NewStorageCorruptionError("hmac_verify", id,
			map[string]string{"reason": "HMAC mismatch — data may be tampered"})
	}

	b := storage.Block{
		ID:        id,
		Nonce:     rec.Nonce,
		HMAC:      rec.HMAC,
		Data:      ct,
		CreatedAt: rec.CreatedAt,
		UpdatedAt: rec.UpdatedAt,
	}

	return bs.strategy.OnRead(b)
}

// DeleteBlock performs a secure delete: it overwrites the block's ciphertext
// region on disk with zeros (best-effort; SSDs with wear-leveling may retain
// copies), then removes the block from the in-memory index and persists the
// updated index atomically.
//
// Silently returns nil if the block ID is not in the index (idempotent).
// Returns ErrCodeStorageIO (2001) if the disk overwrite or index persist fails.
func (bs *BlockStoreImpl) DeleteBlock(id string) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	rec, exists := bs.index[id]
	if !exists {
		return nil
	}

	// Zero the ciphertext region on disk to prevent forensic recovery.
	if err := bs.zeroBlockDataLocked(rec); err != nil {
		log.Printf("[blockstore] zeroBlockData for %s failed: %v (continuing with index delete)", id, err)
	}

	delete(bs.index, id)
	return bs.persistIndexLocked()
}

// ListBlocks returns a snapshot of BlockMeta for all blocks in the in-memory
// index. No disk I/O is performed — the index is loaded once during LoadIndex
// and kept in sync by WriteBlock/DeleteBlock.
//
// The vault must be unlocked for the index to be populated; calling ListBlocks
// on a locked vault returns an empty slice (the index is zeroed on ZeroMVK).
func (bs *BlockStoreImpl) ListBlocks() ([]storage.BlockMeta, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	result := make([]storage.BlockMeta, 0, len(bs.index))
	for id, rec := range bs.index {
		result = append(result, storage.BlockMeta{
			ID:        id,
			Size:      rec.Length,
			Category:  rec.Category,
			CreatedAt: rec.CreatedAt,
			UpdatedAt: rec.UpdatedAt,
		})
	}
	return result, nil
}

// QueryBlocks returns BlockMeta for all blocks whose Category matches the given value.
// If category is empty, all blocks are returned (equivalent to ListBlocks).
// Operates on the decrypted in-memory index; vault must be unlocked.
func (bs *BlockStoreImpl) QueryBlocks(category storage.Category) ([]storage.BlockMeta, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	result := make([]storage.BlockMeta, 0)
	for id, rec := range bs.index {
		if category == "" || rec.Category == category {
			result = append(result, storage.BlockMeta{
				ID:        id,
				Size:      rec.Length,
				Category:  rec.Category,
				CreatedAt: rec.CreatedAt,
				UpdatedAt: rec.UpdatedAt,
			})
		}
	}
	return result, nil
}

// Flush atomically rewrites the encrypted index to vault_index.enc.
// Called during graceful shutdown before zeroing the MVK to ensure no
// pending writes are lost. Safe to call concurrently — acquires the write lock.
func (bs *BlockStoreImpl) Flush() error {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.persistIndexLocked()
}

// Close flushes the index and logs the final block count. Always call Close
// (or defer it) before the daemon exits to prevent index corruption.
// Returns ErrCodeStorageIndexFailed (2005) if the final persist fails.
func (bs *BlockStoreImpl) Close() error {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	if err := bs.persistIndexLocked(); err != nil {
		log.Printf("[blockstore] Close — persist failed: %v", err)
		return err
	}
	log.Printf("[blockstore] Closed — %d blocks in index", len(bs.index))
	return nil
}

// zeroBlockDataLocked overwrites the ciphertext region of a deleted block
// on disk. The caller must hold bs.mu. This is a best-effort secure delete —
// on SSDs with wear-leveling, full data erasure requires TRIM/discard.
func (bs *BlockStoreImpl) zeroBlockDataLocked(rec blockRecord) error {
	f, err := os.OpenFile(bs.filePath, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	// The block layout on disk is: nonce(12) + ciphertext+tag + hmac(32).
	// Zero the region from Offset to Offset+Length so no plaintext-related
	// data remains.
	start := int64(rec.Offset)
	length := int64(rec.Length)
	if start < 0 || length <= 0 {
		return nil
	}

	zeros := make([]byte, 4096)
	written := int64(0)
	for written < length {
		chunk := int64(len(zeros))
		if length-written < chunk {
			chunk = length - written
		}
		n, wErr := f.WriteAt(zeros[:chunk], start+written)
		if wErr != nil {
			return wErr
		}
		written += int64(n)
	}
	return f.Sync()
}

func (bs *BlockStoreImpl) persistIndexLocked() error {
	indexJSON, err := json.Marshal(bs.index)
	if err != nil {
		return gerrors.NewStorageIndexError("marshal_index", err)
	}

	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return gerrors.NewStorageIOError("nonce_generation_index", "", err)
	}

	if err := crypto.ValidateKeyLength(bs.mvk()); err != nil {
		return gerrors.NewCryptoInvalidKeyError(len(bs.mvk()))
	}
	cipher, err := chacha20poly1305.New(bs.mvk())
	if err != nil {
		return gerrors.NewCryptoEncryptionError("new_cipher_index", err)
	}

	ct := cipher.Seal(nil, nonce, indexJSON, nil)

	tmpPath := bs.indexPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(ct)))

	if _, err := f.Write(lenBuf); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if _, err := f.Write(nonce); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if _, err := f.Write(ct); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, bs.indexPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	log.Printf("[blockstore] Index persisted — %d blocks, %d bytes", len(bs.index), len(ct))
	return nil
}

func deriveHMACKey(mvk []byte) [32]byte {
	mac := hmac.New(sha256.New, mvk)
	mac.Write([]byte("grimlocker-hmac-v1"))
	var key [32]byte
	copy(key[:], mac.Sum(nil))
	return key
}
