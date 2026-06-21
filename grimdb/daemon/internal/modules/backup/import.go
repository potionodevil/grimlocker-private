package backup

import (
	"encoding/hex"
	"fmt"
	"io"
	"os"

	engbackup "github.com/grimlocker/grimdb/engine/backup"
	"github.com/grimlocker/grimdb/engine/crypto"
	gerrors "github.com/grimlocker/grimdb/engine/errors"
	"github.com/grimlocker/grimdb/engine/storage"
)

// peekBlob liest den Plaintext-Header einer .grimbak-Datei und erzeugt eine ImportSession.
// Kein Key-Material nötig — Vault muss NICHT entsperrt sein.
func peekBlob(sessions *SessionStore, sourcePath string) (engbackup.PeekResult, error) {
	f, err := os.Open(sourcePath)
	if err != nil {
		return engbackup.PeekResult{}, fmt.Errorf("peek: open file: %w", err)
	}
	defer f.Close()

	hdr, _, _, err := engbackup.DecodeHeader(f)
	if err != nil {
		return engbackup.PeekResult{}, err
	}

	peek := engbackup.PeekResult{
		ExportTimestamp:   hdr.ExportTimestamp,
		GrimlockerVersion: hdr.GrimlockerVersion,
		EntryCount:        hdr.EntryCount,
		HardwareTethered:  hdr.HardwareTethered,
		HardwareIDHex:     hex.EncodeToString(hdr.HardwareID[:]),
		HeaderIntegrityOK: hdr.HeaderHMACValid,
	}

	sess := sessions.newSession(hdr, peek, sourcePath)
	peek.SessionID = sess.ID
	return peek, nil
}

// authorizeImport führt Phase 2 durch: Tether-Prüfung, Entschlüsselung, Block-Import.
func authorizeImport(
	sessions    *SessionStore,
	cryptoP     crypto.Provider,
	store       storage.BlockStore,
	sessionID   string,
	mvk         []byte,
	argonSalt   []byte,
	merge        bool,
) (imported, skipped uint32, err error) {
	sess, ok := sessions.lookup(sessionID)
	if !ok {
		return 0, 0, gerrors.NewBackupSessionNotFoundError(sessionID)
	}

	hdr := sess.Header

	// Hardware-Tether-Prüfung
	if hdr.HardwareTethered {
		match, err := tethersMatch(cryptoP, mvk, argonSalt, hdr)
		if err != nil {
			return 0, 0, fmt.Errorf("authorize: tether check: %w", err)
		}
		if !match {
			sessions.delete(sessionID)
			return 0, 0, gerrors.NewBackupTetherMismatchError()
		}
	}

	// Backup-Key ableiten
	backupKey, err := deriveBackupKey(cryptoP, mvk, hdr.ExportTimestamp)
	if err != nil {
		return 0, 0, fmt.Errorf("authorize: derive backup key: %w", err)
	}

	// Verschlüsselten Payload lesen
	f, err := os.Open(sess.BlobPath)
	if err != nil {
		return 0, 0, fmt.Errorf("authorize: open blob: %w", err)
	}
	defer f.Close()

	_, payloadLen, nonce, err := engbackup.DecodeHeader(f)
	if err != nil {
		return 0, 0, fmt.Errorf("authorize: re-read header: %w", err)
	}

	encPayload := make([]byte, payloadLen)
	if _, err = io.ReadFull(f, encPayload); err != nil {
		return 0, 0, fmt.Errorf("authorize: read payload: %w", err)
	}

	// Entschlüsseln
	plainPayload, err := cryptoP.Decrypt(backupKey, nonce, encPayload, nil)
	if err != nil {
		return 0, 0, gerrors.NewBackupDecryptFailedError(err)
	}

	// Payload parsen
	blocks, _, err := engbackup.DecodePayload(plainPayload)
	if err != nil {
		return 0, 0, fmt.Errorf("authorize: decode payload: %w", err)
	}

	// Blocks schreiben
	var existingIDs map[string]bool
	if merge {
		metas, err := store.ListBlocks()
		if err != nil {
			return 0, 0, fmt.Errorf("authorize: list existing blocks: %w", err)
		}
		existingIDs = make(map[string]bool, len(metas))
		for _, m := range metas {
			existingIDs[m.ID] = true
		}
	}

	for _, b := range blocks {
		if merge && existingIDs[b.ID] {
			skipped++
			continue
		}
		if err := store.WriteBlock(b); err != nil {
			return imported, skipped, fmt.Errorf("authorize: write block %s: %w", b.ID, err)
		}
		imported++
	}

	if err := store.Flush(); err != nil {
		return imported, skipped, fmt.Errorf("authorize: flush store: %w", err)
	}

	sessions.delete(sessionID)
	return imported, skipped, nil
}
