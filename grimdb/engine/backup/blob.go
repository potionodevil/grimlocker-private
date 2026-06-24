// Package backup (blob.go) implementiert den Plaintext-Header des .grimbak-Formats.
//
// Binäres Layout (Big-Endian):
//
//	Offset   Size     Feld
//	 0        8       Magic: "GRIMBAK\0"
//	 8        1       FormatVersion (uint8, v1=0x01)
//	 9        1       Flags (BlobFlags)
//	10        8       ExportTimestampUnix (int64)
//	18        4       GrimlockerVersionLen (uint32)
//	22        N       GrimlockerVersion (UTF-8)
//	22+N     32       HardwareID ([32]byte)
//	54+N      4       EntryCount (uint32)
//	58+N     32       HeaderHMAC (HMAC-SHA256 über alle vorherigen Bytes mit HKDF-Konstantkey)
//	90+N      4       EncryptedPayloadLen (uint32)
//	94+N     12       PayloadNonce
//	106+N     P       EncryptedPayload (Ciphertext + 16-Byte Poly1305-Tag)
//
// Die Peek-Region endet bei Offset 58+N (exkl. HeaderHMAC).
// Alles bis Offset 90+N (exkl. PayloadNonce) ist ohne Key-Material lesbar.
package backup

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
)

// BlobMagic ist die 8-Byte-Signatur am Anfang jeder .grimbak-Datei.
var BlobMagic = [8]byte{0x47, 0x52, 0x49, 0x4D, 0x42, 0x41, 0x4B, 0x00}

// FormatVersionV1 ist die ursprüngliche Format-Version.
const FormatVersionV1 uint8 = 0x01

// FormatVersionV2 fügt Sequenznummer, ExpiresAt und Delta-Felder zum Header hinzu.
// V2-Extension-Block (nach EntryCount, vor HeaderHMAC):
//   4 bytes  BackupSequence (uint32, big-endian)
//   8 bytes  ExpiresAt (int64, big-endian; 0 = kein TTL)
//   1 byte   IsDelta (0x00=nein, 0x01=ja)
//   8 bytes  BaseExportTimestamp (int64, big-endian; 0 wenn !IsDelta)
const FormatVersionV2 uint8 = 0x02

// headerHMACKey leitet den konstanten HMAC-Key für die Header-Integritätsprüfung ab.
// Nutzt HKDF-SHA256 mit einem Zero-Secret — produziert einen deterministischen Key,
// der ohne Key-Material berechnet werden kann (kein Vault-Unlock nötig für Peek).
// Schützt gegen zufällige Korruption; kein kryptografischer Eigentumsnachweis.
func headerHMACKey() []byte {
	// HKDF-SHA256(secret=0x00*32, salt=Magic, info="GRIMBAK-HEADER-INTEGRITY-v1")
	zeroSecret := make([]byte, 32)
	h := hmac.New(sha256.New, zeroSecret)
	h.Write(BlobMagic[:])
	h.Write([]byte("GRIMBAK-HEADER-INTEGRITY-v1"))
	return h.Sum(nil)
}

// computeHeaderHMAC berechnet HMAC-SHA256 über headerBytes mit dem konstanten HMAC-Key.
func computeHeaderHMAC(headerBytes []byte) []byte {
	key := headerHMACKey()
	mac := hmac.New(sha256.New, key)
	mac.Write(headerBytes)
	return mac.Sum(nil)
}

// EncodeHeader schreibt den Plaintext-Header (inkl. HMAC, exkl. verschlüsselter Payload) in w.
// Bei h.FormatVersion >= 2 wird der V2-Extension-Block vor dem HMAC eingefügt.
func EncodeHeader(w io.Writer, h BlobHeader, encryptedPayloadLen uint32, nonce []byte) error {
	var buf bytes.Buffer

	buf.Write(BlobMagic[:])
	buf.WriteByte(h.FormatVersion)
	buf.WriteByte(byte(h.Flags))
	var ts [8]byte
	binary.BigEndian.PutUint64(ts[:], uint64(h.ExportTimestamp))
	buf.Write(ts[:])
	vb := []byte(h.GrimlockerVersion)
	var vlen [4]byte
	binary.BigEndian.PutUint32(vlen[:], uint32(len(vb)))
	buf.Write(vlen[:])
	buf.Write(vb)
	buf.Write(h.HardwareID[:])
	var ec [4]byte
	binary.BigEndian.PutUint32(ec[:], h.EntryCount)
	buf.Write(ec[:])

	// V2 extension block
	if h.FormatVersion >= FormatVersionV2 {
		var seq [4]byte
		binary.BigEndian.PutUint32(seq[:], h.BackupSequence)
		buf.Write(seq[:])
		var exp [8]byte
		binary.BigEndian.PutUint64(exp[:], uint64(h.ExpiresAt))
		buf.Write(exp[:])
		if h.IsDelta {
			buf.WriteByte(0x01)
		} else {
			buf.WriteByte(0x00)
		}
		var base [8]byte
		binary.BigEndian.PutUint64(base[:], uint64(h.BaseExportTimestamp))
		buf.Write(base[:])
		// Ed25519 public key (32 bytes) — only when FlagSigned is set
		if h.Flags&FlagSigned != 0 {
			buf.Write(h.SignaturePublicKey[:])
		}
	}

	mac := computeHeaderHMAC(buf.Bytes())
	buf.Write(mac)

	var pl [4]byte
	binary.BigEndian.PutUint32(pl[:], encryptedPayloadLen)
	buf.Write(pl[:])
	if len(nonce) != 12 {
		return fmt.Errorf("blob: nonce must be 12 bytes, got %d", len(nonce))
	}
	buf.Write(nonce)

	_, err := w.Write(buf.Bytes())
	return err
}

// DecodeHeader liest und validiert den Plaintext-Header aus r.
// HeaderHMACValid wird gesetzt, aber ein HMAC-Mismatch führt NICHT zu einem Fehler —
// der Aufrufer entscheidet, ob er das ablehnt (Peek gibt es als Info zurück).
// Gibt außerdem encryptedPayloadLen und nonce zurück.
func DecodeHeader(r io.Reader) (h BlobHeader, encryptedPayloadLen uint32, nonce []byte, err error) {
	// Magic
	var magic [8]byte
	if _, err = io.ReadFull(r, magic[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read magic: %w", err)
	}
	if magic != BlobMagic {
		return h, 0, nil, fmt.Errorf("blob: invalid magic — not a GRIMBAK file")
	}

	// FormatVersion
	var version [1]byte
	if _, err = io.ReadFull(r, version[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read version: %w", err)
	}
	h.FormatVersion = version[0]
	if h.FormatVersion != FormatVersionV1 && h.FormatVersion != FormatVersionV2 {
		return h, 0, nil, fmt.Errorf("blob: unsupported format version %d", h.FormatVersion)
	}

	// Flags
	var flags [1]byte
	if _, err = io.ReadFull(r, flags[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read flags: %w", err)
	}
	h.Flags = BlobFlags(flags[0])
	h.HardwareTethered = h.Flags&FlagHardwareTethered != 0

	// ExportTimestampUnix
	var ts [8]byte
	if _, err = io.ReadFull(r, ts[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read timestamp: %w", err)
	}
	h.ExportTimestamp = int64(binary.BigEndian.Uint64(ts[:]))

	// GrimlockerVersionLen + GrimlockerVersion
	var vlen [4]byte
	if _, err = io.ReadFull(r, vlen[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read version_len: %w", err)
	}
	vb := make([]byte, binary.BigEndian.Uint32(vlen[:]))
	if len(vb) > 0 {
		if _, err = io.ReadFull(r, vb); err != nil {
			return h, 0, nil, fmt.Errorf("blob: read version_str: %w", err)
		}
	}
	h.GrimlockerVersion = string(vb)

	// HardwareID
	if _, err = io.ReadFull(r, h.HardwareID[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read hardware_id: %w", err)
	}

	// EntryCount
	var ec [4]byte
	if _, err = io.ReadFull(r, ec[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read entry_count: %w", err)
	}
	h.EntryCount = binary.BigEndian.Uint32(ec[:])

	// ── V2 extension block ────────────────────────────────────────────────────
	var seqB [4]byte
	var expB [8]byte
	var isDeltaByte [1]byte
	var baseB [8]byte
	var pubKeyB [32]byte
	if h.FormatVersion >= FormatVersionV2 {
		if _, err = io.ReadFull(r, seqB[:]); err != nil {
			return h, 0, nil, fmt.Errorf("blob: read backup_sequence: %w", err)
		}
		h.BackupSequence = binary.BigEndian.Uint32(seqB[:])
		if _, err = io.ReadFull(r, expB[:]); err != nil {
			return h, 0, nil, fmt.Errorf("blob: read expires_at: %w", err)
		}
		h.ExpiresAt = int64(binary.BigEndian.Uint64(expB[:]))
		if _, err = io.ReadFull(r, isDeltaByte[:]); err != nil {
			return h, 0, nil, fmt.Errorf("blob: read is_delta: %w", err)
		}
		h.IsDelta = isDeltaByte[0] == 0x01
		h.Flags |= FlagDelta * BlobFlags(isDeltaByte[0]&0x01)
		if _, err = io.ReadFull(r, baseB[:]); err != nil {
			return h, 0, nil, fmt.Errorf("blob: read base_export_timestamp: %w", err)
		}
		h.BaseExportTimestamp = int64(binary.BigEndian.Uint64(baseB[:]))
		// Ed25519 public key — only when FlagSigned
		if h.Flags&FlagSigned != 0 {
			if _, err = io.ReadFull(r, pubKeyB[:]); err != nil {
				return h, 0, nil, fmt.Errorf("blob: read signature_public_key: %w", err)
			}
			h.SignaturePublicKey = pubKeyB
		}
	}

	// Reconstruct the bytes we just read to verify HMAC.
	var headerBuf bytes.Buffer
	headerBuf.Write(magic[:])
	headerBuf.Write(version[:])
	headerBuf.Write(flags[:])
	headerBuf.Write(ts[:])
	headerBuf.Write(vlen[:])
	headerBuf.Write(vb)
	headerBuf.Write(h.HardwareID[:])
	headerBuf.Write(ec[:])
	if h.FormatVersion >= FormatVersionV2 {
		headerBuf.Write(seqB[:])
		headerBuf.Write(expB[:])
		headerBuf.Write(isDeltaByte[:])
		headerBuf.Write(baseB[:])
		if h.Flags&FlagSigned != 0 {
			headerBuf.Write(pubKeyB[:])
		}
	}

	// HeaderHMAC
	var storedMAC [32]byte
	if _, err = io.ReadFull(r, storedMAC[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read header_hmac: %w", err)
	}
	expectedMAC := computeHeaderHMAC(headerBuf.Bytes())
	h.HeaderHMACValid = hmac.Equal(storedMAC[:], expectedMAC)

	// EncryptedPayloadLen
	var pl [4]byte
	if _, err = io.ReadFull(r, pl[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read payload_len: %w", err)
	}
	encryptedPayloadLen = binary.BigEndian.Uint32(pl[:])

	// PayloadNonce
	nonce = make([]byte, 12)
	if _, err = io.ReadFull(r, nonce); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read nonce: %w", err)
	}

	return h, encryptedPayloadLen, nonce, nil
}
