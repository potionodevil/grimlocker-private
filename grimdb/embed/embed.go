package embed

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/grimlocker/grimdb/engine/bridge"
	"github.com/grimlocker/grimdb/engine/crypto"
	"github.com/grimlocker/grimdb/engine/storage"
	"github.com/grimlocker/grimdb/engine/storage/grimdb"
)

// DB ist eine geöffnete GrimDB-Instanz. Thread-safe — alle Methoden sind via
// den internen BlockStore-Mutex geschützt.
//
// Nutzung:
//
//	db, err := embed.Open(embed.Config{Dir: "/path/to/vault", Password: "secret", CreateIfMissing: true})
//	if err != nil { ... }
//	defer db.Close()
//
//	id, err := db.Put("", embed.CategoryPassword, data)
//	data, err := db.Get(id)
//	list, err := db.List(embed.CategoryPassword)
//	err = db.Delete(id)
type DB struct {
	store  *grimdb.BlockStoreImpl
	mvk    []byte
	crypto crypto.Provider
	appDir string
}

// Open öffnet eine GrimDB-Instanz im gegebenen Verzeichnis.
//
// Wenn vault.meta fehlt und CreateIfMissing=true, wird ein neues Vault
// initialisiert. Die Recovery-Phrase wird nach stderr geschrieben.
//
// Die Vault-Daten bleiben auf Disk; der MVK lebt nur im RAM dieser DB-Instanz.
func Open(cfg Config) (*DB, error) {
	if cfg.Dir == "" {
		return nil, fmt.Errorf("embed.Open: Dir darf nicht leer sein")
	}
	if cfg.Password == "" {
		return nil, fmt.Errorf("embed.Open: Password darf nicht leer sein")
	}

	if err := os.MkdirAll(cfg.Dir, 0700); err != nil {
		return nil, fmt.Errorf("embed.Open: Verzeichnis anlegen: %w", err)
	}

	initialized, _, _ := grimdb.CheckVaultStatus(cfg.Dir)

	if !initialized {
		if !cfg.CreateIfMissing {
			return nil, fmt.Errorf("embed.Open: Vault nicht initialisiert — CreateIfMissing=true setzen um ein neues Vault anzulegen")
		}

		phrase, err := grimdb.InitializeVault(cfg.Password, cfg.Dir)
		if err != nil {
			return nil, fmt.Errorf("embed.Open: InitializeVault: %w", err)
		}
		fmt.Fprintf(os.Stderr, "[grimdb/embed] Neues Vault angelegt.\n")
		fmt.Fprintf(os.Stderr, "[grimdb/embed] RECOVERY-PHRASE (sicher aufbewahren!):\n%s\n", phrase)
	}

	mvk, err := grimdb.UnlockVault(cfg.Password, cfg.Dir)
	if err != nil {
		return nil, fmt.Errorf("embed.Open: falsches Passwort oder vault.meta beschädigt: %w", err)
	}

	cp := crypto.New(bridge.DefaultBridge{})

	store := grimdb.NewBlockStoreImpl(cfg.Dir)
	mvkCopy := make([]byte, len(mvk))
	copy(mvkCopy, mvk)
	store.SetMVKFunc(func() []byte { return mvkCopy })

	if err := store.LoadIndex(); err != nil {
		return nil, fmt.Errorf("embed.Open: LoadIndex: %w", err)
	}

	return &DB{
		store:  store,
		mvk:    mvkCopy,
		crypto: cp,
		appDir: cfg.Dir,
	}, nil
}

// Put speichert einen Datensatz. Wenn id leer ist, wird eine UUID generiert.
// Gibt die Block-ID zurück.
func (db *DB) Put(id string, category Category, data []byte) (string, error) {
	if id == "" {
		id = generateID()
	}
	// Daten mit MVK verschlüsseln (ChaCha20-Poly1305)
	nonce, err := db.crypto.NewNonce()
	if err != nil {
		return "", fmt.Errorf("Put: nonce: %w", err)
	}
	ct, err := db.crypto.Encrypt(db.mvk, nonce[:], data, []byte(id))
	if err != nil {
		return "", fmt.Errorf("Put: encrypt: %w", err)
	}

	b := storage.Block{
		ID:       id,
		Nonce:    nonce[:],
		Data:     ct,
		Category: category,
	}
	if err := db.store.WriteBlock(b); err != nil {
		return "", fmt.Errorf("Put: WriteBlock: %w", err)
	}
	return id, nil
}

// Get liest und entschlüsselt einen Datensatz anhand der ID.
func (db *DB) Get(id string) ([]byte, error) {
	b, err := db.store.ReadBlock(id)
	if err != nil {
		return nil, fmt.Errorf("Get: %w", err)
	}
	if len(b.Nonce) < 12 {
		return nil, fmt.Errorf("Get: ungültiger Nonce im Block")
	}
	pt, err := db.crypto.Decrypt(db.mvk, b.Nonce, b.Data, []byte(id))
	if err != nil {
		return nil, fmt.Errorf("Get: decrypt: %w", err)
	}
	return pt, nil
}

// Delete löscht einen Block. Idempotent — kein Fehler wenn ID nicht existiert.
func (db *DB) Delete(id string) error {
	return db.store.DeleteBlock(id)
}

// List gibt alle Blöcke der gegebenen Kategorie zurück (nur Metadaten, keine Daten).
// Leere Kategorie gibt alle Blöcke zurück.
func (db *DB) List(category Category) ([]Block, error) {
	metas, err := db.store.QueryBlocks(category)
	if err != nil {
		return nil, err
	}
	result := make([]Block, 0, len(metas))
	for _, m := range metas {
		result = append(result, Block{
			ID:        m.ID,
			Category:  m.Category,
			CreatedAt: m.CreatedAt,
			UpdatedAt: m.UpdatedAt,
		})
	}
	return result, nil
}

// GetJSON ist ein Convenience-Wrapper: Get + json.Unmarshal in v.
func (db *DB) GetJSON(id string, v any) error {
	data, err := db.Get(id)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// PutJSON ist ein Convenience-Wrapper: json.Marshal + Put.
func (db *DB) PutJSON(id string, category Category, v any) (string, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return "", fmt.Errorf("PutJSON: marshal: %w", err)
	}
	return db.Put(id, category, data)
}

// Begin startet eine WAL-backed Write-Transaktion.
// Alle Writes innerhalb der Transaktion werden atomar committed (alles oder nichts).
//
//	tx := db.Begin()
//	tx.Put("key1", embed.CategoryPassword, data1)
//	tx.Put("key2", embed.CategoryNote, data2)
//	if err := tx.Commit(); err != nil { ... }
func (db *DB) Begin() *EmbedTransaction {
	tx, err := db.store.BeginWrite()
	if err != nil {
		// Fallback auf InMemoryWriteTransaction
		tx = storage.NewInMemoryWriteTransaction(db.store)
	}
	return &EmbedTransaction{db: db, tx: tx}
}

// Close flusht den Index und schliesst das Vault. Nach Close darf DB nicht mehr genutzt werden.
func (db *DB) Close() error {
	err := db.store.Close()
	// MVK aus RAM löschen
	for i := range db.mvk {
		db.mvk[i] = 0
	}
	db.mvk = nil
	return err
}

// ── EmbedTransaction ──────────────────────────────────────────────────────────

// EmbedTransaction ist eine WAL-backed Write-Transaktion über einer DB-Instanz.
type EmbedTransaction struct {
	db   *DB
	tx   storage.WriteTransaction
	done bool
}

// Put puffert einen Write innerhalb der Transaktion.
func (t *EmbedTransaction) Put(id string, category Category, data []byte) error {
	if t.done {
		return fmt.Errorf("Transaktion bereits abgeschlossen")
	}
	if id == "" {
		id = generateID()
	}
	nonce, err := t.db.crypto.NewNonce()
	if err != nil {
		return err
	}
	ct, err := t.db.crypto.Encrypt(t.db.mvk, nonce[:], data, []byte(id))
	if err != nil {
		return err
	}
	return t.tx.WriteBlock(storage.Block{
		ID:        id,
		Nonce:     nonce[:],
		Data:      ct,
		Category:  category,
		CreatedAt: time.Now().UnixNano(),
	})
}

// Delete puffert einen Delete innerhalb der Transaktion.
func (t *EmbedTransaction) Delete(id string) error {
	if t.done {
		return fmt.Errorf("Transaktion bereits abgeschlossen")
	}
	return t.tx.DeleteBlock(id)
}

// Commit wendet alle gepufferten Writes/Deletes atomar an.
func (t *EmbedTransaction) Commit() error {
	if t.done {
		return fmt.Errorf("Transaktion bereits abgeschlossen")
	}
	t.done = true
	return t.tx.Commit()
}

// Rollback verwirft alle gepufferten Writes.
func (t *EmbedTransaction) Rollback() {
	if !t.done {
		t.done = true
		t.tx.Rollback()
	}
}

// ── Hilfsfunktionen ───────────────────────────────────────────────────────────

func generateID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}
