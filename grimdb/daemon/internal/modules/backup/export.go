package backup

import (
	"bytes"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"time"

	engbackup "github.com/grimlocker/grimdb/engine/backup"
	"github.com/grimlocker/grimdb/engine/crypto"
	"github.com/grimlocker/grimdb/engine/storage"
)

// buildBlobKeyInfo ist das HKDF-Info-Label für den per-Export-Backup-Key.
var buildBlobKeyInfo = []byte("GRIMBAK-EXPORT-KEY-v1")

// deriveBackupKey leitet den per-Export-Verschlüsselungskey aus dem MVK und dem Export-Timestamp ab.
// Der Key ist deterministisch reproduzierbar: HKDF-SHA256(MVK, salt=timestamp, info=label).
func deriveBackupKey(cryptoP crypto.Provider, mvk []byte, exportTimestamp int64) ([]byte, error) {
	var tsBuf [8]byte
	binary.BigEndian.PutUint64(tsBuf[:], uint64(exportTimestamp))
	return cryptoP.DeriveHKDF(mvk, tsBuf[:], buildBlobKeyInfo, 32)
}

// buildBlob erstellt die vollständige .grimbak-Datei und schreibt sie nach destPath.
// Gibt den SHA-256-Hex-String des geschriebenen Files zurück (Post-Write-Checksum).
func buildBlob(
	cryptoP   crypto.Provider,
	store     storage.BlockStore,
	mvk       []byte,
	argonSalt []byte,
	req       engbackup.ExportRequest,
	version   string,
	sequence  uint32,
) (sha256hex string, entryCount uint32, err error) {
	// Alle Blöcke aus dem Store lesen
	metas, listErr := store.ListBlocks()
	if listErr != nil {
		return "", 0, fmt.Errorf("export: list blocks: %w", listErr)
	}
	blocks := make([]storage.Block, 0, len(metas))
	for _, m := range metas {
		// Delta-Export: nur Blöcke die neuer als BaseExportTimestamp sind
		if req.Delta && req.BaseExportTimestamp > 0 && m.UpdatedAt < req.BaseExportTimestamp {
			continue
		}
		b, readErr := store.ReadBlock(m.ID)
		if readErr != nil {
			return "", 0, fmt.Errorf("export: read block %s: %w", m.ID, readErr)
		}
		blocks = append(blocks, b)
	}
	entryCount = uint32(len(blocks))

	// VaultMetaSnapshot bauen
	exportTs := time.Now().Unix()
	meta := engbackup.VaultMetaSnapshot{
		ArgonSalt:      argonSalt,
		ExportedAt:     exportTs,
		Version:        version,
		BackupSequence: sequence,
	}

	// Payload serialisieren
	plainPayload, encErr := engbackup.EncodePayload(blocks, meta)
	if encErr != nil {
		return "", 0, fmt.Errorf("export: encode payload: %w", encErr)
	}

	// Backup-Key ableiten + Payload verschlüsseln
	backupKey, keyErr := deriveBackupKey(cryptoP, mvk, exportTs)
	if keyErr != nil {
		return "", 0, fmt.Errorf("export: derive backup key: %w", keyErr)
	}
	nonceArr, nonceErr := cryptoP.NewNonce()
	if nonceErr != nil {
		return "", 0, fmt.Errorf("export: new nonce: %w", nonceErr)
	}
	nonce := nonceArr[:]

	encPayload, encryptErr := cryptoP.Encrypt(backupKey, nonce, plainPayload, nil)
	if encryptErr != nil {
		return "", 0, fmt.Errorf("export: encrypt payload: %w", encryptErr)
	}

	// Header zusammenbauen — V2 wenn neue Features genutzt werden
	fmtVersion := engbackup.FormatVersionV1
	if sequence > 0 || req.TTLDays > 0 || req.Delta || req.Sign {
		fmtVersion = engbackup.FormatVersionV2
	}
	hdr := engbackup.BlobHeader{
		FormatVersion:       fmtVersion,
		ExportTimestamp:     exportTs,
		GrimlockerVersion:   version,
		EntryCount:          entryCount,
		HardwareTethered:    req.HardwareTether,
		BackupSequence:      sequence,
		IsDelta:             req.Delta,
		BaseExportTimestamp: req.BaseExportTimestamp,
	}

	// Ed25519-Signing vorbereiten
	var sigPrivKey ed25519.PrivateKey
	if req.Sign {
		var sigPubKey ed25519.PublicKey
		var keyErr error
		sigPrivKey, sigPubKey, keyErr = engbackup.DeriveSigningKey(mvk, exportTs)
		if keyErr != nil {
			return "", 0, fmt.Errorf("export: derive signing key: %w", keyErr)
		}
		hdr.Flags |= engbackup.FlagSigned
		copy(hdr.SignaturePublicKey[:], sigPubKey)
	}
	if req.HardwareTether {
		hdr.Flags |= engbackup.FlagHardwareTethered
		vaultID, vaultErr := deriveVaultID(cryptoP, mvk, argonSalt)
		if vaultErr != nil {
			return "", 0, fmt.Errorf("export: derive vault id: %w", vaultErr)
		}
		hdr.HardwareID = computeCommitment(vaultID, exportTs)
	}
	if req.Delta {
		hdr.Flags |= engbackup.FlagDelta
	}
	if req.TTLDays > 0 {
		hdr.Flags |= engbackup.FlagHasTTL
		hdr.ExpiresAt = exportTs + int64(req.TTLDays)*86400
	}

	// Datei schreiben
	f, err := os.OpenFile(req.DestPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return "", 0, fmt.Errorf("export: open dest file: %w", err)
	}

	var writeErr error
	func() {
		defer f.Close()
		var buf bytes.Buffer
		if writeErr = engbackup.EncodeHeader(&buf, hdr, uint32(len(encPayload)), nonce); writeErr != nil {
			return
		}
		buf.Write(encPayload)

		// Ed25519-Signatur über (Header + Payload) anhängen, bevor File geschrieben wird.
		if sigPrivKey != nil {
			sig := engbackup.SignBlob(buf.Bytes(), sigPrivKey)
			buf.Write(sig)
		}

		_, writeErr = f.Write(buf.Bytes())
		if writeErr != nil {
			writeErr = fmt.Errorf("export: write blob: %w", writeErr)
		}
	}()
	if writeErr != nil {
		return "", 0, writeErr
	}

	// Post-Write-Checksum: Re-Read der Datei um Bit-Flips zu erkennen
	sha256hex, err = checksumFile(req.DestPath)
	if err != nil {
		return "", 0, fmt.Errorf("export: post-write checksum: %w", err)
	}

	return sha256hex, entryCount, nil
}

// checksumFile berechnet SHA-256 über die gegebene Datei und gibt den Hex-String zurück.
func checksumFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
