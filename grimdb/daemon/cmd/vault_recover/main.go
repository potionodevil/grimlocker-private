// vault_recover — Notfall-Recovery-Tool für vault_entries.enc
//
// Scannt vault_entries.enc und rekonstruiert den vault_index.enc.
// Phase 1: JSON-Scan (findet Plaintext-Einträge)
// Phase 2: AEAD-Scan mit MVK (findet verschlüsselte Einträge, benötigt Passwort)
//
// Verwendung:
//   go run ./daemon/cmd/vault_recover/main.go --password <vault-passwort> [--dry-run]
package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"crypto/subtle"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

// ─── Vault Meta ────────────────────────────────────────────────────────────────

type VaultMeta struct {
	IsInitialized bool   `json:"is_initialized"`
	ArgonSalt     []byte `json:"argon_salt,omitempty"`
	Sentinel      []byte `json:"sentinel,omitempty"`
	EntropyPath   string `json:"entropy_path,omitempty"`
}

// ─── Storage Types ─────────────────────────────────────────────────────────────

type Category string

type VaultEntry struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Category  Category          `json:"category"`
	Type      string            `json:"type,omitempty"`
	Fields    map[string]string `json:"fields,omitempty"`
	SubjectID string            `json:"subject_id,omitempty"`
	CreatedAt int64             `json:"created_at"`
	UpdatedAt int64             `json:"updated_at"`
}

type blockRecord struct {
	Offset    int64    `json:"offset"`
	Length    int64    `json:"length"`
	Nonce     []byte   `json:"nonce"`
	HMAC      []byte   `json:"hmac"`
	Category  Category `json:"category,omitempty"`
	CreatedAt int64    `json:"created_at"`
	UpdatedAt int64    `json:"updated_at"`
}

// ─── Crypto Helpers ───────────────────────────────────────────────────────────

func deriveHMACKey(mvk []byte) [32]byte {
	mac := hmac.New(sha256.New, mvk)
	mac.Write([]byte("grimlocker-hmac-v1"))
	var key [32]byte
	copy(key[:], mac.Sum(nil))
	return key
}

func deriveMVKFromPassword(password, appDir string, meta *VaultMeta) ([]byte, error) {
	// Argon2id: Time=4, Memory=128MB, Threads=2 (DefaultKDFOptions)
	argonHash := argon2.IDKey([]byte(password), meta.ArgonSalt, 4, 131072, 2, 32)

	// Entropy-Datei laden
	entropyPath := meta.EntropyPath
	if entropyPath == "" {
		entropyPath = filepath.Join(appDir, "entropy.bin")
	}
	entropy, err := os.ReadFile(entropyPath)
	if err != nil {
		return nil, fmt.Errorf("entropy.bin lesen: %w", err)
	}
	fileSize := int64(len(entropy))

	// DeriveCoordinateOffsets: HKDF-SHA256(argonHash, nil, "grimlocker-coordinates-v1") → 128 bytes → 32 uint32 Offsets
	hkdfReader := hkdf.New(sha256.New, argonHash, nil, []byte("grimlocker-coordinates-v1"))
	hkdfBuf := make([]byte, 128)
	if _, err := io.ReadFull(hkdfReader, hkdfBuf); err != nil {
		return nil, fmt.Errorf("HKDF für Koordinaten fehlgeschlagen: %w", err)
	}

	var offsets [32]int64
	for i := 0; i < 32; i++ {
		val := binary.BigEndian.Uint32(hkdfBuf[i*4 : i*4+4])
		offsets[i] = int64(uint64(val) % uint64(fileSize))
	}

	// DeriveXORAsMVK: mvk[i] = entropy[offsets[i]] für alle 32 Bytes
	mvk := make([]byte, 32)
	for i, off := range offsets {
		if off < 0 || off >= fileSize {
			return nil, fmt.Errorf("ungültiger Offset %d (Dateigröße %d)", off, fileSize)
		}
		mvk[i] = entropy[off]
	}

	// Zero entropy
	for i := range entropy {
		entropy[i] = 0
	}

	return mvk, nil
}

func verifySentinel(mvk, sentinel []byte) bool {
	if len(sentinel) < 12+13+16 {
		return false
	}
	nonce := sentinel[:12]
	ct := sentinel[12:]
	cipher, err := chacha20poly1305.New(mvk)
	if err != nil {
		return false
	}
	pt, err := cipher.Open(nil, nonce, ct, nil)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare(pt, []byte("GRIMLOCKER_V1")) == 1
}

// ─── Index Persistence ────────────────────────────────────────────────────────

func writeIndex(indexPath string, mvk []byte, index map[string]blockRecord) error {
	indexJSON, err := json.Marshal(index)
	if err != nil {
		return fmt.Errorf("index marshal: %w", err)
	}

	nonce := make([]byte, 12)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("nonce gen: %w", err)
	}

	cipher, err := chacha20poly1305.New(mvk)
	if err != nil {
		return fmt.Errorf("cipher: %w", err)
	}
	ct := cipher.Seal(nil, nonce, indexJSON, nil)

	tmpPath := indexPath + ".tmp"
	f, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("create tmp: %w", err)
	}

	lenBuf := make([]byte, 4)
	binary.BigEndian.PutUint32(lenBuf, uint32(len(ct)))
	if _, err := f.Write(lenBuf); err != nil {
		f.Close(); os.Remove(tmpPath); return err
	}
	if _, err := f.Write(nonce); err != nil {
		f.Close(); os.Remove(tmpPath); return err
	}
	if _, err := f.Write(ct); err != nil {
		f.Close(); os.Remove(tmpPath); return err
	}
	if err := f.Sync(); err != nil {
		f.Close(); os.Remove(tmpPath); return err
	}
	f.Close()

	return os.Rename(tmpPath, indexPath)
}

// ─── Scanning ─────────────────────────────────────────────────────────────────

type FoundBlock struct {
	Offset    int64
	DataLen   int64
	Nonce     []byte
	BlockHMAC []byte
	Data      []byte
	Entry     *VaultEntry
	Source    string // "json" or "aead"
}

func defaultAppDir() string {
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(os.Getenv("USERPROFILE"), "AppData", "Roaming")
		}
		return filepath.Join(appData, "com.grimlocker.desktop")
	default:
		home, _ := os.UserHomeDir()
		return filepath.Join(home, ".config", "com.grimlocker.desktop")
	}
}

// scanJSON scannt vault_entries.enc nach Plaintext-JSON-Einträgen.
func scanJSON(data []byte) []FoundBlock {
	var results []FoundBlock
	offset := 0

	for offset < len(data) {
		if offset+46 > len(data) {
			break
		}
		nonce := make([]byte, 12)
		copy(nonce, data[offset:offset+12])
		blockHMAC := make([]byte, 32)
		copy(blockHMAC, data[offset+12:offset+44])
		jsonStart := offset + 44

		dec := json.NewDecoder(bytes.NewReader(data[jsonStart:]))
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			offset++
			continue
		}

		if len(raw) < 2 || raw[0] != '{' {
			offset++
			continue
		}

		var entry VaultEntry
		if err := json.Unmarshal(raw, &entry); err != nil || entry.ID == "" {
			offset++
			continue
		}

		if strings.HasPrefix(entry.ID, "_hist_") {
			offset += 44 + len(raw)
			continue
		}

		fb := FoundBlock{
			Offset:    int64(offset),
			DataLen:   int64(len(raw)),
			Nonce:     nonce,
			BlockHMAC: blockHMAC,
			Data:      raw,
			Entry:     &entry,
			Source:    "json",
		}
		results = append(results, fb)
		log.Printf("  [JSON] offset=%d id=%s title=%q category=%s", offset, entry.ID, entry.Title, entry.Category)
		offset += 44 + len(raw)
	}
	return results
}

// scanAEAD versucht AEAD-Entschlüsselung für verschlüsselte Einträge.
// Geht davon aus, dass verschlüsselte Daten Format [12 enc-nonce][ciphertext+16 tag] haben.
func scanAEAD(data []byte, mvk []byte, maxDataLen int) []FoundBlock {
	if len(mvk) != 32 {
		return nil
	}
	cipher, err := chacha20poly1305.New(mvk)
	if err != nil {
		return nil
	}

	var results []FoundBlock
	tried := 0
	found := 0

	for offset := 0; offset+44+28 <= len(data); offset++ {
		nonce := data[offset : offset+12]
		blockHMAC := data[offset+12 : offset+44]
		dataStart := offset + 44

		// Versuche enc-nonce bei dataStart
		if dataStart+28 > len(data) {
			continue
		}
		encNonce := data[dataStart : dataStart+12]
		remaining := len(data) - dataStart - 12

		// Maximale Datenlänge begrenzen (Performance)
		maxLen := remaining
		if maxLen > maxDataLen {
			maxLen = maxDataLen
		}

		// Versuche verschiedene Längen der Ciphertext+Tag-Daten
		for ctLen := 16; ctLen <= maxLen; ctLen++ {
			ct := data[dataStart+12 : dataStart+12+ctLen]
			pt, decErr := cipher.Open(nil, encNonce, ct, nil)
			tried++
			if decErr != nil {
				continue
			}

			// Entschlüsselung erfolgreich! Prüfe ob es JSON ist.
			if len(pt) < 2 || pt[0] != '{' {
				continue
			}

			var entry VaultEntry
			if err := json.Unmarshal(pt, &entry); err != nil || entry.ID == "" {
				continue
			}

			totalDataLen := 12 + ctLen // enc-nonce + ct+tag
			fb := FoundBlock{
				Offset:    int64(offset),
				DataLen:   int64(totalDataLen),
				Nonce:     append([]byte{}, nonce...),
				BlockHMAC: append([]byte{}, blockHMAC...),
				Data:      pt,
				Entry:     &entry,
				Source:    "aead",
			}
			results = append(results, fb)
			found++
			log.Printf("  [AEAD] offset=%d id=%s title=%q category=%s", offset, entry.ID, entry.Title, entry.Category)
			// Springe zum nächsten Block
			offset += 44 + totalDataLen - 1 // -1 wegen loop increment
			break
		}
	}
	log.Printf("  [AEAD] Versuche: %d, Gefunden: %d", tried, found)
	return results
}

// ─── Main ─────────────────────────────────────────────────────────────────────

func main() {
	appDirFlag := flag.String("appdir", defaultAppDir(), "Grimlocker AppData-Verzeichnis")
	password := flag.String("password", "", "Vault-Passwort (für MVK-Ableitung und Index-Rebuild)")
	dryRun := flag.Bool("dry-run", false, "Kein Schreiben von vault_index.enc")
	maxAEADLen := flag.Int("max-aead-len", 4096, "Max. Datenlänge für AEAD-Scan (Performance)")
	skipAEAD := flag.Bool("skip-aead", false, "AEAD-Scan überspringen (nur JSON-Scan)")
	outputJSON := flag.String("output", "", "Recovery-Report als JSON speichern")
	flag.Parse()

	log.Printf("vault_recover — Grimlocker Notfall-Recovery-Tool")
	log.Printf("AppDir: %s", *appDirFlag)

	entriesPath := filepath.Join(*appDirFlag, "vault_entries.enc")
	indexPath := filepath.Join(*appDirFlag, "vault_index.enc")
	metaPath := filepath.Join(*appDirFlag, "vault.meta")

	// vault.meta laden
	metaBytes, err := os.ReadFile(metaPath)
	if err != nil {
		log.Fatalf("vault.meta nicht gefunden: %v", err)
	}
	var meta VaultMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		log.Fatalf("vault.meta ungültig: %v", err)
	}
	log.Printf("Vault initialisiert: %v, ArgonSalt: %s", meta.IsInitialized, hex.EncodeToString(meta.ArgonSalt))

	// vault_entries.enc laden
	data, err := os.ReadFile(entriesPath)
	if err != nil {
		log.Fatalf("vault_entries.enc nicht lesbar: %v", err)
	}
	log.Printf("vault_entries.enc: %d bytes (%.1f KB)", len(data), float64(len(data))/1024)

	// MVK ableiten wenn Passwort angegeben
	var mvk []byte
	if *password != "" {
		log.Printf("Leite MVK aus Passwort ab...")
		mvk, err = deriveMVKFromPassword(*password, *appDirFlag, &meta)
		if err != nil {
			log.Fatalf("MVK-Ableitung fehlgeschlagen: %v", err)
		}

		// Sentinel prüfen
		if !verifySentinel(mvk, meta.Sentinel) {
			log.Fatalf("FEHLER: Passwort falsch — Sentinel-Verifikation fehlgeschlagen!")
		}
		log.Printf("MVK-Ableitung OK — Passwort korrekt!")

		hmacKey := deriveHMACKey(mvk)
		log.Printf("HMAC-Key abgeleitet: %s...", hex.EncodeToString(hmacKey[:4]))
	} else {
		log.Printf("Kein Passwort angegeben — nur JSON-Scan (kein Index-Rebuild, kein AEAD-Scan)")
	}

	// ── Phase 1: JSON-Scan ──────────────────────────────────────────────────────
	log.Printf("")
	log.Printf("=== Phase 1: JSON-Scan (Plaintext-Einträge) ===")
	jsonBlocks := scanJSON(data)
	log.Printf("JSON-Scan: %d Einträge gefunden", len(jsonBlocks))

	// ── Phase 2: AEAD-Scan (optional) ──────────────────────────────────────────
	var aeadBlocks []FoundBlock
	if !*skipAEAD && mvk != nil {
		log.Printf("")
		log.Printf("=== Phase 2: AEAD-Scan (verschlüsselte Einträge, bis %d bytes Datenlänge) ===", *maxAEADLen)
		log.Printf("Hinweis: Dieser Scan kann je nach Dateigröße mehrere Minuten dauern...")
		aeadBlocks = scanAEAD(data, mvk, *maxAEADLen)
		log.Printf("AEAD-Scan: %d Einträge gefunden", len(aeadBlocks))
	}

	// ── Alle Blöcke zusammenführen ──────────────────────────────────────────────
	allBlocks := append(jsonBlocks, aeadBlocks...)

	log.Printf("")
	log.Printf("=== Gefundene Vault-Einträge ===")
	for _, b := range allBlocks {
		if b.Entry != nil {
			log.Printf("  [%s] id=%s title=%q category=%s fields=%v",
				b.Source, b.Entry.ID, b.Entry.Title, b.Entry.Category, b.Entry.Fields)
		}
	}

	// ── Index rekonstruieren ────────────────────────────────────────────────────
	if len(allBlocks) == 0 {
		log.Printf("")
		log.Printf("Keine Einträge gefunden — vault_index.enc wird nicht verändert.")
		return
	}

	newIndex := make(map[string]blockRecord)
	for _, b := range allBlocks {
		if b.Entry == nil || b.Entry.ID == "" {
			continue
		}

		// HMAC verifizieren wenn MVK vorhanden
		hmacOK := false
		if mvk != nil {
			hmacKey := deriveHMACKey(mvk)
			mac := hmac.New(sha256.New, hmacKey[:])
			mac.Write([]byte(b.Entry.ID))
			mac.Write(b.Nonce)
			mac.Write(b.Data)
			hmacOK = bytes.Equal(mac.Sum(nil), b.BlockHMAC)
		}

		now := time.Now().UnixNano()
		createdAt := b.Entry.CreatedAt
		updatedAt := b.Entry.UpdatedAt
		if createdAt == 0 {
			createdAt = now
		}
		if updatedAt == 0 {
			updatedAt = now
		}

		rec := blockRecord{
			Offset:    b.Offset + 44, // Datenbeginn (nach nonce+hmac)
			Length:    b.DataLen,
			Nonce:     b.Nonce,
			HMAC:      b.BlockHMAC,
			Category:  b.Entry.Category,
			CreatedAt: createdAt,
			UpdatedAt: updatedAt,
		}
		newIndex[b.Entry.ID] = rec

		hmacStr := ""
		if mvk != nil {
			if hmacOK {
				hmacStr = " [HMAC OK]"
			} else {
				hmacStr = " [HMAC FEHLER — Eintrag wird trotzdem aufgenommen]"
			}
		}
		log.Printf("  Index-Eintrag: id=%s offset=%d len=%d%s",
			b.Entry.ID, rec.Offset, rec.Length, hmacStr)
	}

	// JSON-Report optional speichern
	if *outputJSON != "" {
		type Report struct {
			ScannedAt   string       `json:"scanned_at"`
			FileSize    int64        `json:"file_size"`
			EntriesFound int         `json:"entries_found"`
			Entries     []VaultEntry `json:"entries"`
		}
		report := Report{
			ScannedAt:    time.Now().Format(time.RFC3339),
			FileSize:     int64(len(data)),
			EntriesFound: len(allBlocks),
		}
		for _, b := range allBlocks {
			if b.Entry != nil {
				report.Entries = append(report.Entries, *b.Entry)
			}
		}
		reportJSON, _ := json.MarshalIndent(report, "", "  ")
		if err := os.WriteFile(*outputJSON, reportJSON, 0600); err != nil {
			log.Printf("Konnte Report nicht speichern: %v", err)
		} else {
			log.Printf("Report gespeichert: %s", *outputJSON)
		}
	}

	if *dryRun {
		log.Printf("")
		log.Printf("DRY RUN — vault_index.enc wird NICHT überschrieben.")
		log.Printf("Würde %d Einträge in den Index schreiben.", len(newIndex))
		return
	}

	if mvk == nil {
		log.Printf("")
		log.Printf("Kein Passwort angegeben — vault_index.enc kann nicht geschrieben werden.")
		log.Printf("Starte mit --password <passwort> um den Index zu rekonstruieren.")
		return
	}

	// Backup des bestehenden Index
	backupPath := indexPath + ".backup." + fmt.Sprintf("%d", time.Now().Unix())
	if existing, err := os.ReadFile(indexPath); err == nil {
		if err := os.WriteFile(backupPath, existing, 0600); err != nil {
			log.Printf("Warnung: Backup fehlgeschlagen: %v", err)
		} else {
			log.Printf("Bestehender Index gesichert: %s", backupPath)
		}
	}

	// Neuen Index schreiben
	if err := writeIndex(indexPath, mvk, newIndex); err != nil {
		log.Fatalf("vault_index.enc schreiben fehlgeschlagen: %v", err)
	}

	log.Printf("")
	log.Printf("=== ERFOLG ===")
	log.Printf("vault_index.enc wurde mit %d Einträgen rekonstruiert.", len(newIndex))
	log.Printf("Starte den Grimlocker-Daemon neu um die Einträge zu sehen.")
}

