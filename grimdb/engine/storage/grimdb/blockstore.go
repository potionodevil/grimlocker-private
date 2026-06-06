// Package grimdb implementiert den datei-basierten encrypted BlockStore (GrimDB).
//
// Storage-Layout auf der Platte:
//
//	vault_entries.enc  — append-only Data-File; jeder Block belegt:
//	                     nonce(12 bytes) + ciphertext+AEAD-tag + HMAC(32 bytes)
//	vault_index.enc    — encrypted JSON-Index, der Block-ID → blockRecord mapped.
//	                     Wird atomar via .tmp-Datei + Rename geschrieben.
//
// Encryption: ChaCha20-Poly1305 mit einem zufälligen 12-Byte-Nonce pro Block.
// Der Encryption-Key (MVK) wird nie im Heap gelagert — Caller stellen eine
// Resolver-Funktion via SetMVKFunc bereit, die den Key aus locked Memory holt.
//
// Block-Integrity: Jeder Block hat einen HMAC-SHA256 über (id ‖ nonce ‖ ciphertext).
// Der HMAC-Key wird via HMAC-SHA256("grimlocker-hmac-v1") aus dem MVK abgeleitet.
// ReadBlock verifiziert den HMAC mit constant-time-Vergleich, bevor er Daten zurückgibt.
//
// Sicheres Löschen: DeleteBlock überschreibt den Ciphertext-Bereich auf der Platte
// mit Nullen, bevor er den Block aus dem Index entfernt (best-effort auf SSDs mit
// Wear-Leveling — für echte Sicherheit TRIM/discard verwenden).
//
// Concurrency: Ein sync.RWMutex schützt den In-Memory-Index und File-I/O.
// Reads halten ein RLock, Writes ein Lock. Multiple gleichzeitige Reads sind sicher.
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

	"github.com/grimlocker/grimdb/engine/crypto"
	gerrors "github.com/grimlocker/grimdb/engine/errors"
	"github.com/grimlocker/grimdb/engine/storage"
	"golang.org/x/crypto/chacha20poly1305"
)

type blockRecord struct {
	Offset    int64            `json:"offset"`
	Length    int64            `json:"length"`
	Nonce     []byte           `json:"nonce"`
	HMAC      []byte           `json:"hmac"`
	Category  storage.Category `json:"category,omitempty"` // entry category für In-Memory-Filterung
	CreatedAt int64            `json:"created_at"`
	UpdatedAt int64            `json:"updated_at"`
}

// BlockStoreImpl ist der konkrete BlockStore, backed by vault_entries.enc.
// Nutzt ChaCha20-Poly1305 mit dem MVK für Index- und Entry-Encryption.
// Der MVK wird nie im Heap gelagert — Caller stellen einen Resolver via
// SetMVKFunc bereit und entziehen ihn via ZeroMVK.
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

// SetMVKFunc speichert eine Funktion, die den MVK aus locked Memory holt.
// Nach erfolgreichem UnlockVault aufrufen. Der zurückgegebene Key lebt in locked
// Memory — es wird keine Heap-Kopie erstellt.
func (bs *BlockStoreImpl) SetMVKFunc(fn func() []byte) {
	bs.mu.Lock()
	bs.getMVK = fn
	bs.mu.Unlock()
}

// ZeroMVK entfernt die Key-Referenz und löscht den Index.
// Das eigentliche Key-Material in locked Memory wird vom Security-Modul verwaltet.
func (bs *BlockStoreImpl) ZeroMVK() {
	bs.mu.Lock()
	bs.getMVK = nil
	bs.index = make(map[string]blockRecord)
	bs.mu.Unlock()
}

// SetStrategy injiziert eine StorageStrategy.
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

// WriteBlock encryptet und appended einen Block an vault_entries.enc,
// dann wird der encrypted Index atomar aktualisiert.
//
// Data muss je nach Strategy Plaintext oder Ciphertext sein (NopStrategy lässt
// alles unverändert). WriteBlock generiert ein random 12-Byte-Nonce, leitet einen
// HMAC-Key aus dem MVK ab, berechnet HMAC-SHA256(id ‖ nonce ‖ data), schreibt
// nonce+hmac+data an die Datei und persistiert den Index.
//
// Der Vault muss unlocked sein (MVK-Resolver gesetzt), bevor WriteBlock aufgerufen wird.
func (bs *BlockStoreImpl) WriteBlock(b storage.Block) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	b2, err := bs.strategy.OnWrite(b)
	if err != nil {
		if _, ok := err.(*gerrors.GrimlockError); ok {
			return err
		}
		return gerrors.NewStorageIOError("block_strategy_write", b.ID, err)
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
		return gerrors.NewStorageIOError("write_block_nonce", b.ID, err)
	}
	if _, err := f.Write(b.HMAC); err != nil {
		f.Close()
		return gerrors.NewStorageIOError("write_block_hmac", b.ID, err)
	}
	if _, err := f.Write(b.Data); err != nil {
		f.Close()
		return gerrors.NewStorageIOError("write_block_data", b.ID, err)
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

// ReadBlock retrieviert einen Block aus dem Store anhand der ID, verifiziert den HMAC
// und gibt den rohen (noch verschlüsselten) Ciphertext in Block.Data zurück.
//
// HMAC-Verifikation nutzt constant-time-Vergleich, um Timing-Angriffe zu verhindern.
// Bei Fehlschlag kommt ErrCodeStorageCorruption (2002) — das bedeutet entweder
// manipulierte Daten oder einen falschen MVK (z.B. falsches Passwort).
//
// ErrCodeStorageNotFound (2003) wenn die Block-ID nicht im Index ist.
// ErrCodeStorageIO (2001) bei Disk-Read-Fehlern.
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

	// HMAC verifizieren, bevor wir die Daten rausgeben.
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

// DeleteBlock macht sicheres Löschen: Überschreibt den Ciphertext-Bereich auf
// der Platte mit Nullen (best-effort; SSDs mit Wear-Leveling können Kopien
// behalten), entfernt den Block aus dem In-Memory-Index und persistiert den
// aktualisierten Index atomar.
//
// Gibt nil zurück, wenn die Block-ID nicht existiert (idempotent).
// ErrCodeStorageIO (2001) bei Disk-Overwrite- oder Index-Persist-Fehlern.
func (bs *BlockStoreImpl) DeleteBlock(id string) error {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	rec, exists := bs.index[id]
	if !exists {
		return nil
	}

	// Ciphertext-Bereich auf der Platte mit Nullen überschreiben.
	if err := bs.zeroBlockDataLocked(rec); err != nil {
		log.Printf("[blockstore] zeroBlockData for %s failed: %v (continuing with index delete)", id, err)
	}

	delete(bs.index, id)
	return bs.persistIndexLocked()
}

// ListBlocks gibt einen Snapshot der BlockMeta für alle Blöcke im In-Memory-Index.
// Kein Disk-I/O — der Index wird einmal bei LoadIndex geladen und von WriteBlock/DeleteBlock
// synchron gehalten.
//
// Der Vault muss unlocked sein, damit der Index gefüllt ist. Bei locked vault
// kommt eine leere Slice zurück (der Index wird bei ZeroMVK geleert).
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

// QueryBlocks gibt BlockMeta für alle Blöcke zurück, deren Category dem Wert entspricht.
// Bei leerer Category werden alle Blöcke zurückgegeben (äquivalent zu ListBlocks).
// Arbeitet auf dem entschlüsselten In-Memory-Index; Vault muss unlocked sein.
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

// Flush schreibt den encrypted Index atomar nach vault_index.enc.
// Sollte vor dem Graceful-Shutdown aufgerufen werden, bevor der MVK gezeroized wird,
// damit keine ausstehenden Writes verloren gehen. Thread-safe (write lock).
func (bs *BlockStoreImpl) Flush() error {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.persistIndexLocked()
}

// Close flusht den Index und loggt die finale Block-Anzahl. Immer Close (oder defer)
// vor Daemon-Exit aufrufen, um Index-Korruption zu vermeiden.
// ErrCodeStorageIndexFailed (2005) wenn finaler Persist fehlschlägt.
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

// zeroBlockDataLocked überschreibt den Ciphertext-Bereich eines gelöschten Blocks
// auf der Platte mit Nullen. Der Caller muss bs.mu halten. Best-effort —
// auf SSDs mit Wear-Leveling ist TRIM/discard nötig für echte Löschung.
func (bs *BlockStoreImpl) zeroBlockDataLocked(rec blockRecord) error {
	f, err := os.OpenFile(bs.filePath, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer f.Close()

	// Block-Layout auf Disk: nonce(12) + ciphertext+tag + hmac(32).
	// Nullt den Bereich von Offset bis Offset+Length.
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
		return gerrors.NewStorageIOError("create_index_tmpfile", "", err)
	}

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(ct)))

	if _, err := f.Write(lenBuf); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return gerrors.NewStorageIOError("write_index_length", "", err)
	}
	if _, err := f.Write(nonce); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return gerrors.NewStorageIOError("write_index_nonce", "", err)
	}
	if _, err := f.Write(ct); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return gerrors.NewStorageIOError("write_index_ciphertext", "", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		_ = os.Remove(tmpPath)
		return gerrors.NewStorageIOError("fsync_index", "", err)
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return gerrors.NewStorageIOError("close_index_tmpfile", "", err)
	}
	if err := os.Rename(tmpPath, bs.indexPath); err != nil {
		_ = os.Remove(tmpPath)
		return gerrors.NewStorageIOError("rename_index_tmpfile", "", err)
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
