package grimdb

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"
	"time"

	"github.com/grimlocker/grimdb/crypto"
	"github.com/grimlocker/grimdb/storage"
	"golang.org/x/crypto/chacha20poly1305"
)

type blockRecord struct {
	Offset    int64  `json:"offset"`
	Length    int64  `json:"length"`
	Nonce     []byte `json:"nonce"`
	HMAC      []byte `json:"hmac"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
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
		return fmt.Errorf("open index file: %w", err)
	}
	defer f.Close()

	lenBuf := make([]byte, 4)
	if _, err := io.ReadFull(f, lenBuf); err != nil {
		if err == io.EOF {
			return nil
		}
		return fmt.Errorf("read index length: %w", err)
	}

	indexLen := binary.BigEndian.Uint32(lenBuf)
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(f, nonce); err != nil {
		return fmt.Errorf("read index nonce: %w", err)
	}

	ct := make([]byte, indexLen)
	if _, err := io.ReadFull(f, ct); err != nil {
		return fmt.Errorf("read encrypted index: %w", err)
	}

	if err := crypto.ValidateKeyLength(bs.mvk()); err != nil {
		return fmt.Errorf("blockstore LoadIndex: %w", err)
	}
	cipher, err := chacha20poly1305.New(bs.mvk())
	if err != nil {
		return err
	}

	indexJSON, err := cipher.Open(nil, nonce, ct, nil)
	if err != nil {
		return fmt.Errorf("decrypt index: %w", err)
	}

	var idx map[string]blockRecord
	if err := json.Unmarshal(indexJSON, &idx); err != nil {
		return fmt.Errorf("unmarshal index: %w", err)
	}
	bs.index = idx

	stats := make([]string, 0, len(bs.index))
	for id := range bs.index {
		stats = append(stats, id)
	}
	log.Printf("[blockstore] LoadIndex — %d entries loaded: %v", len(bs.index), stats)
	return nil
}

// WriteBlock encrypts and appends a block to the data file, then persists the index.
// The block's Data field must already be encrypted ciphertext.
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
			return fmt.Errorf("nonce generation: %w", err)
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
		return fmt.Errorf("open data file: %w", err)
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
		return fmt.Errorf("close data file: %w", err)
	}

	bs.index[b.ID] = blockRecord{
		Offset:    dataOffset,
		Length:    int64(len(b.Data)),
		Nonce:     b.Nonce,
		HMAC:      b.HMAC,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}

	log.Printf("[blockstore] WriteBlock — id=%s offset=%d dataLen=%d", b.ID, dataOffset, len(b.Data))

	return bs.persistIndexLocked()
}

// ReadBlock retrieves and verifies a block from the store.
func (bs *BlockStoreImpl) ReadBlock(id string) (storage.Block, error) {
	bs.mu.RLock()
	rec, exists := bs.index[id]
	filePath := bs.filePath
	mvk := bs.mvk()
	bs.mu.RUnlock()

	if !exists {
		return storage.Block{}, fmt.Errorf("block not found: %s", id)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return storage.Block{}, err
	}
	defer f.Close()

	ct := make([]byte, rec.Length)
	if _, err := f.ReadAt(ct, rec.Offset); err != nil {
		return storage.Block{}, fmt.Errorf("read block: %w", err)
	}

	// Verify HMAC before returning.
	hmacKey := deriveHMACKey(mvk)
	mac := hmac.New(sha256.New, hmacKey[:])
	mac.Write([]byte(id))
	mac.Write(rec.Nonce)
	mac.Write(ct)
	if subtle.ConstantTimeCompare(mac.Sum(nil), rec.HMAC) != 1 {
		return storage.Block{}, fmt.Errorf("HMAC verification failed for block %s", id)
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

// DeleteBlock removes a block from the index and zeroes the ciphertext
// region on disk before removing from the index (secure delete).
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

// ListBlocks returns metadata for all known blocks.
func (bs *BlockStoreImpl) ListBlocks() ([]storage.BlockMeta, error) {
	bs.mu.RLock()
	defer bs.mu.RUnlock()

	result := make([]storage.BlockMeta, 0, len(bs.index))
	for id, rec := range bs.index {
		result = append(result, storage.BlockMeta{
			ID:        id,
			Size:      rec.Length,
			CreatedAt: rec.CreatedAt,
			UpdatedAt: rec.UpdatedAt,
		})
	}
	return result, nil
}

// Flush atomically rewrites the index.
func (bs *BlockStoreImpl) Flush() error {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.persistIndexLocked()
}

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
		return fmt.Errorf("marshal index: %w", err)
	}

	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return err
	}

	if err := crypto.ValidateKeyLength(bs.mvk()); err != nil {
		return fmt.Errorf("blockstore persistIndex: %w", err)
	}
	cipher, err := chacha20poly1305.New(bs.mvk())
	if err != nil {
		return err
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
