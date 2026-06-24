// Package grimdb — Write-Ahead Log (WAL) für crash-sichere Multi-Block-Transaktionen.
//
// Das WAL-File vault_wal.enc speichert Intent-Records BEVOR Daten auf Disk geschrieben
// werden. Bei einem Crash kann LoadIndex die uncommitted Transaktionen erkennen und
// den letzten konsistenten Zustand wiederherstellen.
//
// Format: [5-byte header: "GWAL\x01"] + N×Record
// Record: [4-byte CRC32][4-byte ciphertext_len][12-byte nonce][ciphertext]
// Payload (nach Entschlüsselung): JSON-codiertes walRecord
//
// Crash-Szenarien:
//   Crash vor WAL-Commit → Block-Daten in entries-File, aber NICHT im Index (orphaned space)
//   Crash nach WAL-Commit, vor Index-Persist → Recovery replays Write-Records in Index
//   Crash nach Index-Persist → WAL ist redundant, Recovery ist idempotent
package grimdb

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"os"
	"sync"

	"github.com/grimlocker/grimdb/engine/storage"
	"golang.org/x/crypto/chacha20poly1305"
)

const walFileName = "vault_wal.enc"

var walMagic = [4]byte{'G', 'W', 'A', 'L'}

const (
	walTypeTxBegin  byte = 0x01
	walTypeWrite    byte = 0x02
	walTypeDelete   byte = 0x03
	walTypeCommit   byte = 0x04
	walTypeRollback byte = 0x05
)

// walRecord ist das einheitliche Payload-Format für alle WAL-Record-Typen.
// Felder die nicht zum jeweiligen Typ gehören bleiben leer/0.
type walRecord struct {
	Type      byte             `json:"t"`
	TxID      string           `json:"tx,omitempty"`
	Timestamp int64            `json:"ts,omitempty"`
	// Write-Felder (walTypeWrite)
	BlockID   string           `json:"bid,omitempty"`
	Offset    int64            `json:"off,omitempty"`
	Length    int64            `json:"len,omitempty"`
	Nonce     []byte           `json:"n,omitempty"`
	HMAC      []byte           `json:"h,omitempty"`
	Category  storage.Category `json:"cat,omitempty"`
	CreatedAt int64            `json:"ca,omitempty"`
	UpdatedAt int64            `json:"ua,omitempty"`
}

// deriveWALKey leitet einen WAL-spezifischen Schlüssel aus dem MVK ab.
func deriveWALKey(mvk []byte) []byte {
	mac := hmac.New(sha256.New, mvk)
	mac.Write([]byte("grimlocker-wal-key-v1"))
	return mac.Sum(nil) // 32 bytes
}

// WALManager verwaltet das WAL-File. Thread-safe über seinen eigenen Mutex.
// Er hält keinen BlockStore-Mutex und wird nie von BlockStore-Locks aus aufgerufen,
// sodass kein Deadlock möglich ist.
type WALManager struct {
	mu     sync.Mutex
	path   string
	f      *os.File
	getMVK func() []byte
}

// openWALManager öffnet (oder erstellt) das WAL-File und schreibt den Magic-Header
// wenn die Datei neu ist.
func openWALManager(appDir string, getMVK func() []byte) (*WALManager, error) {
	path := appDir + "/" + walFileName

	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return nil, fmt.Errorf("open wal: %w", err)
	}

	stat, _ := f.Stat()
	if stat.Size() == 0 {
		header := append(walMagic[:], 0x01)
		if _, err := f.Write(header); err != nil {
			f.Close()
			return nil, fmt.Errorf("write wal header: %w", err)
		}
		if err := f.Sync(); err != nil {
			f.Close()
			return nil, fmt.Errorf("sync wal header: %w", err)
		}
	}

	return &WALManager{path: path, f: f, getMVK: getMVK}, nil
}

func (w *WALManager) mvk() []byte {
	if w.getMVK == nil {
		return nil
	}
	return w.getMVK()
}

// appendRecord verschlüsselt und appended einen WAL-Record.
// Format: [4-byte CRC32 über Frame][4-byte ct_len][12-byte nonce][ciphertext]
func (w *WALManager) appendRecord(rec walRecord) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	mvk := w.mvk()
	if len(mvk) == 0 {
		return nil // WAL ohne MVK nicht möglich — silent skip (vault nicht unlocked)
	}

	payload, err := json.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshal wal record: %w", err)
	}

	walKey := deriveWALKey(mvk)
	cipher, err := chacha20poly1305.New(walKey)
	if err != nil {
		return fmt.Errorf("new wal cipher: %w", err)
	}

	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("wal nonce gen: %w", err)
	}

	ct := cipher.Seal(nil, nonce, payload, nil)

	// Frame: [4-byte ct_len][12-byte nonce][ciphertext]
	frame := make([]byte, 4+12+len(ct))
	binary.BigEndian.PutUint32(frame[0:4], uint32(len(ct)))
	copy(frame[4:16], nonce)
	copy(frame[16:], ct)

	crc := crc32.ChecksumIEEE(frame)
	crcBuf := [4]byte{}
	binary.BigEndian.PutUint32(crcBuf[:], crc)

	if _, err := w.f.Write(crcBuf[:]); err != nil {
		return fmt.Errorf("write wal crc: %w", err)
	}
	if _, err := w.f.Write(frame); err != nil {
		return fmt.Errorf("write wal frame: %w", err)
	}
	return w.f.Sync()
}

// readAllRecords liest und entschlüsselt alle WAL-Records nach dem 5-Byte-Header.
// Stoppt bei CRC-Fehler oder truncated Record (partial write = safe stop point).
func (w *WALManager) readAllRecords() ([]walRecord, error) {
	f, err := os.Open(w.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open wal for read: %w", err)
	}
	defer f.Close()

	// 5-Byte-Header überspringen
	if _, err := f.Seek(5, io.SeekStart); err != nil {
		return nil, nil
	}

	mvk := w.mvk()
	if len(mvk) == 0 {
		return nil, nil
	}

	walKey := deriveWALKey(mvk)
	cipher, err := chacha20poly1305.New(walKey)
	if err != nil {
		return nil, fmt.Errorf("new wal cipher for read: %w", err)
	}

	var records []walRecord
	for {
		var crcBuf [4]byte
		if _, err := io.ReadFull(f, crcBuf[:]); err != nil {
			break // EOF
		}
		expectedCRC := binary.BigEndian.Uint32(crcBuf[:])

		var lenBuf [4]byte
		if _, err := io.ReadFull(f, lenBuf[:]); err != nil {
			break
		}
		ctLen := binary.BigEndian.Uint32(lenBuf[:])

		nonce := make([]byte, 12)
		if _, err := io.ReadFull(f, nonce); err != nil {
			break
		}

		ct := make([]byte, ctLen)
		if _, err := io.ReadFull(f, ct); err != nil {
			break
		}

		// CRC prüfen
		frame := make([]byte, 4+12+int(ctLen))
		binary.BigEndian.PutUint32(frame[0:4], ctLen)
		copy(frame[4:16], nonce)
		copy(frame[16:], ct)
		if crc32.ChecksumIEEE(frame) != expectedCRC {
			log.Printf("[wal] CRC-Fehler — truncated Record, Recovery stoppt hier")
			break
		}

		payload, err := cipher.Open(nil, nonce, ct, nil)
		if err != nil {
			log.Printf("[wal] Entschlüsselung fehlgeschlagen — truncated Record")
			break
		}

		var rec walRecord
		if err := json.Unmarshal(payload, &rec); err != nil {
			log.Printf("[wal] unmarshal fehlgeschlagen: %v", err)
			break
		}
		records = append(records, rec)
	}
	return records, nil
}

// checkpoint truncated das WAL nach einem erfolgreichen Index-Persist.
// Nach dem Persist ist der Index die kanonische Quelle der Wahrheit — alle
// WAL-Records davor sind redundant.
func (w *WALManager) checkpoint() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.f != nil {
		_ = w.f.Close()
	}

	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		// Re-open für Append damit der WALManager nutzbar bleibt
		w.f, _ = os.OpenFile(w.path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0600)
		return fmt.Errorf("wal checkpoint truncate: %w", err)
	}
	header := append(walMagic[:], 0x01)
	_, _ = f.Write(header)
	_ = f.Sync()
	f.Close()

	w.f, err = os.OpenFile(w.path, os.O_RDWR|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("wal checkpoint reopen: %w", err)
	}
	log.Printf("[wal] Checkpoint geschrieben — WAL truncated")
	return nil
}

// close schliesst das WAL-File.
func (w *WALManager) close() {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.f != nil {
		_ = w.f.Close()
		w.f = nil
	}
}

// recoverWAL liest das WAL und replayed committed Transaktionen, die noch nicht
// im Index sind. Wird von LoadIndex vor dem Einlesen von vault_index.enc aufgerufen.
func recoverWAL(wal *WALManager, index map[string]blockRecord) (map[string]blockRecord, error) {
	records, err := wal.readAllRecords()
	if err != nil {
		log.Printf("[wal] Recovery-Read-Fehler: %v", err)
		return index, nil // non-fatal
	}
	if len(records) == 0 {
		return index, nil
	}

	type txState struct {
		committed  bool
		rolledBack bool
		writes     []walRecord
		deletes    []walRecord
	}

	txs := make(map[string]*txState)
	var txOrder []string

	for _, rec := range records {
		switch rec.Type {
		case walTypeTxBegin:
			if _, exists := txs[rec.TxID]; !exists {
				txOrder = append(txOrder, rec.TxID)
				txs[rec.TxID] = &txState{}
			}
		case walTypeWrite:
			if tx := txs[rec.TxID]; tx != nil {
				tx.writes = append(tx.writes, rec)
			}
		case walTypeDelete:
			if tx := txs[rec.TxID]; tx != nil {
				tx.deletes = append(tx.deletes, rec)
			}
		case walTypeCommit:
			if tx := txs[rec.TxID]; tx != nil {
				tx.committed = true
			}
		case walTypeRollback:
			if tx := txs[rec.TxID]; tx != nil {
				tx.rolledBack = true
			}
		}
	}

	replayed := 0
	for _, txID := range txOrder {
		tx := txs[txID]
		if tx == nil || !tx.committed {
			// Uncommitted = Rollback-Zustand (Daten in entries-File orphaned, aber nicht im Index)
			if tx != nil && !tx.rolledBack && len(tx.writes) > 0 {
				log.Printf("[wal] Recovery: uncommitted tx=%s (%d writes) — orphaned data, kein Replay", txID, len(tx.writes))
			}
			continue
		}

		for _, w := range tx.writes {
			if _, exists := index[w.BlockID]; !exists {
				index[w.BlockID] = blockRecord{
					Offset:    w.Offset,
					Length:    w.Length,
					Nonce:     w.Nonce,
					HMAC:      w.HMAC,
					Category:  w.Category,
					CreatedAt: w.CreatedAt,
					UpdatedAt: w.UpdatedAt,
				}
				log.Printf("[wal] Recovery: Write replayed block_id=%s", w.BlockID)
				replayed++
			}
		}
		for _, d := range tx.deletes {
			if _, exists := index[d.BlockID]; exists {
				delete(index, d.BlockID)
				log.Printf("[wal] Recovery: Delete replayed block_id=%s", d.BlockID)
				replayed++
			}
		}
	}

	if replayed > 0 {
		log.Printf("[wal] Recovery abgeschlossen: %d Operationen replayed", replayed)
	} else {
		log.Printf("[wal] Recovery: WAL konsistent — kein Replay nötig")
	}

	return index, nil
}
