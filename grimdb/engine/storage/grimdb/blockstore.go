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
//
// WAL (Write-Ahead Log): Wenn wal != nil, wird jede Schreiboperation in
// vault_wal.enc protokolliert, bevor der Index aktualisiert wird. Das erlaubt
// crash-sichere Multi-Block-Transaktionen via BeginWrite().
type BlockStoreImpl struct {
	mu        sync.RWMutex
	filePath  string
	indexPath string
	appDir    string
	index     map[string]blockRecord
	getMVK    func() []byte
	strategy  storage.StorageStrategy
	wal       *WALManager // nil bis SetMVKFunc aufgerufen wird
}

func NewBlockStoreImpl(appDir string) *BlockStoreImpl {
	return &BlockStoreImpl{
		filePath:  appDir + "/vault_entries.enc",
		indexPath: appDir + "/vault_index.enc",
		appDir:    appDir,
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
// Öffnet gleichzeitig den WAL-Manager falls noch nicht geschehen.
func (bs *BlockStoreImpl) SetMVKFunc(fn func() []byte) {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	bs.getMVK = fn
	if bs.wal == nil && bs.appDir != "" {
		wal, err := openWALManager(bs.appDir, fn)
		if err != nil {
			log.Printf("[blockstore] WAL konnte nicht geöffnet werden: %v — fahre ohne WAL fort", err)
		} else {
			bs.wal = wal
			log.Printf("[blockstore] WAL aktiviert: %s/%s", bs.appDir, walFileName)
		}
	}
}

// ZeroMVK entfernt die Key-Referenz und löscht den Index.
// Das eigentliche Key-Material in locked Memory wird vom Security-Modul verwaltet.
func (bs *BlockStoreImpl) ZeroMVK() {
	bs.mu.Lock()
	bs.getMVK = nil
	bs.index = make(map[string]blockRecord)
	if bs.wal != nil {
		bs.wal.close()
		bs.wal = nil
	}
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
		} else {
			return gerrors.NewStorageIOError("open_index", "", err)
		}
	} else {
		defer f.Close()

		lenBuf := make([]byte, 4)
		if _, err := io.ReadFull(f, lenBuf); err != nil {
			if err == io.EOF {
				bs.index = make(map[string]blockRecord)
			} else {
				return gerrors.NewStorageIOError("read_index_length", "", err)
			}
		} else {
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
		}
	}

	// WAL-Recovery: Committed Transaktionen replayan, die noch nicht im Index sind.
	if bs.wal != nil {
		recovered, err := recoverWAL(bs.wal, bs.index)
		if err != nil {
			log.Printf("[blockstore] WAL-Recovery Fehler: %v", err)
		} else {
			bs.index = recovered
		}
	}

	log.Printf("[blockstore] LoadIndex — %d entries geladen", len(bs.index))
	return nil
}

// appendBlockDataLocked schreibt nonce+hmac+data an vault_entries.enc und gibt
// den Daten-Offset (nach nonce+hmac) zurück. Der Caller muss bs.mu halten.
// Aktualisiert b.Nonce, b.HMAC, b.CreatedAt, b.UpdatedAt in-place.
func (bs *BlockStoreImpl) appendBlockDataLocked(b *storage.Block) (int64, error) {
	now := time.Now().UnixNano()
	if b.CreatedAt == 0 {
		b.CreatedAt = now
	}
	b.UpdatedAt = now

	if len(b.Nonce) == 0 {
		b.Nonce = make([]byte, 12)
		if _, err := rand.Read(b.Nonce); err != nil {
			return 0, gerrors.NewStorageIOError("nonce_generation", b.ID, err)
		}
	}

	hmacKey := deriveHMACKey(bs.mvk())
	mac := hmac.New(sha256.New, hmacKey[:])
	mac.Write([]byte(b.ID))
	mac.Write(b.Nonce)
	mac.Write(b.Data)
	b.HMAC = mac.Sum(nil)

	f, err := os.OpenFile(bs.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		return 0, gerrors.NewStorageIOError("open_data_file", b.ID, err)
	}

	stat, _ := f.Stat()
	dataOffset := stat.Size() + 12 + 32 // hinter nonce(12) + hmac(32)

	if _, err := f.Write(b.Nonce); err != nil {
		f.Close()
		return 0, gerrors.NewStorageIOError("write_block_nonce", b.ID, err)
	}
	if _, err := f.Write(b.HMAC); err != nil {
		f.Close()
		return 0, gerrors.NewStorageIOError("write_block_hmac", b.ID, err)
	}
	if _, err := f.Write(b.Data); err != nil {
		f.Close()
		return 0, gerrors.NewStorageIOError("write_block_data", b.ID, err)
	}
	if err := f.Close(); err != nil {
		return 0, gerrors.NewStorageIOError("close_data_file", b.ID, err)
	}

	return dataOffset, nil
}

// WriteBlock encryptet und appended einen Block an vault_entries.enc,
// dann wird der encrypted Index atomar aktualisiert.
//
// Wenn WAL aktiv ist, wird der Write als autocommit-Transaktion geloggt:
// Crash nach dem Schreiben aber vor dem Index-Persist → Recovery via WAL.
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

	// WAL autocommit: TxBegin
	txID := ""
	if bs.wal != nil {
		txID, err = generateTxID()
		if err == nil {
			_ = bs.wal.appendRecord(walRecord{
				Type:      walTypeTxBegin,
				TxID:      txID,
				Timestamp: time.Now().UnixNano(),
			})
		}
	}

	offset, err := bs.appendBlockDataLocked(&b)
	if err != nil {
		if bs.wal != nil && txID != "" {
			_ = bs.wal.appendRecord(walRecord{Type: walTypeRollback, TxID: txID})
		}
		return err
	}

	bs.index[b.ID] = blockRecord{
		Offset:    offset,
		Length:    int64(len(b.Data)),
		Nonce:     b.Nonce,
		HMAC:      b.HMAC,
		Category:  b.Category,
		CreatedAt: b.CreatedAt,
		UpdatedAt: b.UpdatedAt,
	}

	// WAL Write-Record + Commit
	if bs.wal != nil && txID != "" {
		_ = bs.wal.appendRecord(walRecord{
			Type:      walTypeWrite,
			TxID:      txID,
			BlockID:   b.ID,
			Offset:    offset,
			Length:    int64(len(b.Data)),
			Nonce:     b.Nonce,
			HMAC:      b.HMAC,
			Category:  b.Category,
			CreatedAt: b.CreatedAt,
			UpdatedAt: b.UpdatedAt,
		})
		_ = bs.wal.appendRecord(walRecord{Type: walTypeCommit, TxID: txID})
	}

	log.Printf("[blockstore] WriteBlock — id=%s offset=%d dataLen=%d", b.ID, offset, len(b.Data))

	if err := bs.persistIndexLocked(); err != nil {
		return err
	}

	// WAL Checkpoint nach erfolgreichem Index-Persist
	if bs.wal != nil {
		_ = bs.wal.checkpoint()
	}

	return nil
}

// BeginWrite startet eine WAL-backed Write-Transaktion für atomare Multi-Block-Writes.
// Implementiert storage.BlockStoreV2.
func (bs *BlockStoreImpl) BeginWrite() (storage.WriteTransaction, error) {
	if bs.wal == nil {
		return storage.NewInMemoryWriteTransaction(bs), nil // Fallback ohne WAL
	}
	return newWALTransaction(bs)
}

// BeginRead startet einen konsistenten Read-Only-Snapshot. Implementiert storage.BlockStoreV2.
func (bs *BlockStoreImpl) BeginRead() (storage.ReadTransaction, error) {
	bs.mu.RLock()
	snapshot := make(map[string]blockRecord, len(bs.index))
	for k, v := range bs.index {
		snapshot[k] = v
	}
	filePath := bs.filePath
	store := bs
	bs.mu.RUnlock()
	return &blockStoreReadTx{index: snapshot, filePath: filePath, store: store}, nil
}

// blockStoreReadTx ist ein Snapshot-Read für BlockStoreV2.BeginRead().
type blockStoreReadTx struct {
	index    map[string]blockRecord
	filePath string
	store    *BlockStoreImpl
}

func (r *blockStoreReadTx) ReadBlock(id string) (storage.Block, error) {
	rec, exists := r.index[id]
	if !exists {
		return storage.Block{}, gerrors.NewStorageNotFoundError(id)
	}
	return r.store.readBlockFromDisk(id, rec)
}

func (r *blockStoreReadTx) ListBlocks() ([]storage.BlockMeta, error) {
	result := make([]storage.BlockMeta, 0, len(r.index))
	for id, rec := range r.index {
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

func (r *blockStoreReadTx) QueryBlocks(category storage.Category) ([]storage.BlockMeta, error) {
	result := make([]storage.BlockMeta, 0)
	for id, rec := range r.index {
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

func (r *blockStoreReadTx) Close() {}

// readBlockFromDisk liest und verifiziert einen Block von Disk ohne Lock.
// Wird von ReadBlock und blockStoreReadTx genutzt.
func (bs *BlockStoreImpl) readBlockFromDisk(id string, rec blockRecord) (storage.Block, error) {
	mvk := bs.mvk()

	f, err := os.Open(bs.filePath)
	if err != nil {
		return storage.Block{}, gerrors.NewStorageIOError("open_data_file", id, err)
	}
	defer f.Close()

	ct := make([]byte, rec.Length)
	if _, err := f.ReadAt(ct, rec.Offset); err != nil {
		return storage.Block{}, gerrors.NewStorageIOError("read_block_data", id, err)
	}

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

// ReadBlock retrieviert einen Block aus dem Store anhand der ID, verifiziert den HMAC
// und gibt den rohen (noch verschlüsselten) Ciphertext in Block.Data zurück.
func (bs *BlockStoreImpl) ReadBlock(id string) (storage.Block, error) {
	bs.mu.RLock()
	rec, exists := bs.index[id]
	bs.mu.RUnlock()

	if !exists {
		return storage.Block{}, gerrors.NewStorageNotFoundError(id)
	}
	return bs.readBlockFromDisk(id, rec)
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

	// WAL Delete-Record schreiben
	if bs.wal != nil {
		txID, err := generateTxID()
		if err == nil {
			_ = bs.wal.appendRecord(walRecord{Type: walTypeTxBegin, TxID: txID, Timestamp: time.Now().UnixNano()})
			_ = bs.wal.appendRecord(walRecord{Type: walTypeDelete, TxID: txID, BlockID: id})
			_ = bs.wal.appendRecord(walRecord{Type: walTypeCommit, TxID: txID})
		}
	}

	if err := bs.zeroBlockDataLocked(rec); err != nil {
		log.Printf("[blockstore] zeroBlockData for %s failed: %v", id, err)
	}

	delete(bs.index, id)
	if err := bs.persistIndexLocked(); err != nil {
		return err
	}

	if bs.wal != nil {
		_ = bs.wal.checkpoint()
	}
	return nil
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
	if bs.wal != nil {
		_ = bs.wal.checkpoint()
		bs.wal.close()
		bs.wal = nil
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
