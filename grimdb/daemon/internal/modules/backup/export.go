package backup

import (
	"bytes"
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

var buildBlobKeyInfo = []byte("GRIMBAK-EXPORT-KEY-v1")

// deriveBackupKey derives the per-export encryption key from the MVK and timestamp.
// Deterministically reproducible: HKDF-SHA256(MVK, salt=timestamp, info=label).
func deriveBackupKey(cryptoP crypto.Provider, mvk []byte, exportTimestamp int64) ([]byte, error) {
	var tsBuf [8]byte
	binary.BigEndian.PutUint64(tsBuf[:], uint64(exportTimestamp))
	return cryptoP.DeriveHKDF(mvk, tsBuf[:], buildBlobKeyInfo, 32)
}

// buildBlob creates the complete .grimbak file at destPath.
// Returns SHA-256 hex of the written file (post-write checksum) and entry count.
func buildBlob(
	cryptoP   crypto.Provider,
	store     storage.BlockStore,
	mvk       []byte,
	argonSalt []byte,
	destPath  string,
	tether    bool,
	version   string,
) (sha256hex string, entryCount uint32, err error) {
	metas, err := store.ListBlocks()
	if err != nil {
		return "", 0, fmt.Errorf("export: list blocks: %w", err)
	}
	blocks := make([]storage.Block, 0, len(metas))
	for _, m := range metas {
		b, err := store.ReadBlock(m.ID)
		if err != nil {
			return "", 0, fmt.Errorf("export: read block %s: %w", m.ID, err)
		}
		blocks = append(blocks, b)
	}
	entryCount = uint32(len(blocks))

	exportTs := time.Now().Unix()
	meta := engbackup.VaultMetaSnapshot{
		ArgonSalt:  argonSalt,
		ExportedAt: exportTs,
		Version:    version,
	}

	plainPayload, err := engbackup.EncodePayload(blocks, meta)
	if err != nil {
		return "", 0, fmt.Errorf("export: encode payload: %w", err)
	}

	backupKey, err := deriveBackupKey(cryptoP, mvk, exportTs)
	if err != nil {
		return "", 0, fmt.Errorf("export: derive backup key: %w", err)
	}
	nonceArr, err := cryptoP.NewNonce()
	if err != nil {
		return "", 0, fmt.Errorf("export: new nonce: %w", err)
	}
	nonce := nonceArr[:]

	encPayload, err := cryptoP.Encrypt(backupKey, nonce, plainPayload, nil)
	if err != nil {
		return "", 0, fmt.Errorf("export: encrypt payload: %w", err)
	}

	hdr := engbackup.BlobHeader{
		FormatVersion:     engbackup.FormatVersionV1,
		ExportTimestamp:   exportTs,
		GrimlockerVersion: version,
		EntryCount:        entryCount,
		HardwareTethered:  tether,
	}
	if tether {
		hdr.Flags |= engbackup.FlagHardwareTethered
		vaultID, err := deriveVaultID(cryptoP, mvk, argonSalt)
		if err != nil {
			return "", 0, fmt.Errorf("export: derive vault id: %w", err)
		}
		hdr.HardwareID = computeCommitment(vaultID, exportTs)
	}

	f, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
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
		_, writeErr = f.Write(buf.Bytes())
		if writeErr != nil {
			writeErr = fmt.Errorf("export: write blob: %w", writeErr)
		}
	}()
	if writeErr != nil {
		return "", 0, writeErr
	}

	sha256hex, err = checksumFile(destPath)
	if err != nil {
		return "", 0, fmt.Errorf("export: post-write checksum: %w", err)
	}

	return sha256hex, entryCount, nil
}

// checksumFile computes SHA-256 over the given file and returns the hex string.
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
