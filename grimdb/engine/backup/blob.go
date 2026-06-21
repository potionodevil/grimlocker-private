// Package backup (blob.go) implements the plaintext header of the .grimbak format.
//
// Binary layout (big-endian):
//
//	Offset   Size   Field
//	 0        8     Magic: "GRIMBAK\0"
//	 8        1     FormatVersion (uint8, v1=0x01)
//	 9        1     Flags (BlobFlags)
//	10        8     ExportTimestampUnix (int64)
//	18        4     GrimlockerVersionLen (uint32)
//	22        N     GrimlockerVersion (UTF-8)
//	22+N     32     HardwareID ([32]byte)
//	54+N      4     EntryCount (uint32)
//	58+N     32     HeaderHMAC (HMAC-SHA256 over all preceding bytes)
//	90+N      4     EncryptedPayloadLen (uint32)
//	94+N     12     PayloadNonce
//	106+N     P     EncryptedPayload (ciphertext + 16-byte Poly1305 tag)
//
// The peek region ends at offset 58+N (before HeaderHMAC).
// Everything up to offset 90+N is readable without key material.
package backup

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"
)

// BlobMagic is the 8-byte signature at the start of every .grimbak file.
var BlobMagic = [8]byte{0x47, 0x52, 0x49, 0x4D, 0x42, 0x41, 0x4B, 0x00}

// FormatVersionV1 is the current format version.
const FormatVersionV1 uint8 = 0x01

// headerHMACKey derives the constant HMAC key for header integrity.
// Uses HMAC-SHA256 with a zero secret — produces a deterministic key
// computable without any vault key material (safe for Phase 1 Peek).
// Guards against accidental corruption; does not prove authorship.
func headerHMACKey() []byte {
	zeroSecret := make([]byte, 32)
	h := hmac.New(sha256.New, zeroSecret)
	h.Write(BlobMagic[:])
	h.Write([]byte("GRIMBAK-HEADER-INTEGRITY-v1"))
	return h.Sum(nil)
}

func computeHeaderHMAC(headerBytes []byte) []byte {
	key := headerHMACKey()
	mac := hmac.New(sha256.New, key)
	mac.Write(headerBytes)
	return mac.Sum(nil)
}

// EncodeHeader writes the plaintext header (including HMAC, excluding encrypted payload) to w.
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

// DecodeHeader reads and validates the plaintext header from r.
// HeaderHMACValid is set but a mismatch does NOT return an error —
// the caller decides whether to reject (Peek surfaces it as metadata).
// Returns encryptedPayloadLen and nonce alongside the decoded header.
func DecodeHeader(r io.Reader) (h BlobHeader, encryptedPayloadLen uint32, nonce []byte, err error) {
	var magic [8]byte
	if _, err = io.ReadFull(r, magic[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read magic: %w", err)
	}
	if magic != BlobMagic {
		return h, 0, nil, fmt.Errorf("blob: invalid magic — not a GRIMBAK file")
	}

	var version [1]byte
	if _, err = io.ReadFull(r, version[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read version: %w", err)
	}
	h.FormatVersion = version[0]
	if h.FormatVersion != FormatVersionV1 {
		return h, 0, nil, fmt.Errorf("blob: unsupported format version %d", h.FormatVersion)
	}

	var flags [1]byte
	if _, err = io.ReadFull(r, flags[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read flags: %w", err)
	}
	h.Flags = BlobFlags(flags[0])
	h.HardwareTethered = h.Flags&FlagHardwareTethered != 0

	var ts [8]byte
	if _, err = io.ReadFull(r, ts[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read timestamp: %w", err)
	}
	h.ExportTimestamp = int64(binary.BigEndian.Uint64(ts[:]))

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

	if _, err = io.ReadFull(r, h.HardwareID[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read hardware_id: %w", err)
	}

	var ec [4]byte
	if _, err = io.ReadFull(r, ec[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read entry_count: %w", err)
	}
	h.EntryCount = binary.BigEndian.Uint32(ec[:])

	// Reconstruct header bytes for HMAC verification.
	var headerBuf bytes.Buffer
	headerBuf.Write(magic[:])
	headerBuf.Write(version[:])
	headerBuf.Write(flags[:])
	headerBuf.Write(ts[:])
	headerBuf.Write(vlen[:])
	headerBuf.Write(vb)
	headerBuf.Write(h.HardwareID[:])
	headerBuf.Write(ec[:])

	var storedMAC [32]byte
	if _, err = io.ReadFull(r, storedMAC[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read header_hmac: %w", err)
	}
	expectedMAC := computeHeaderHMAC(headerBuf.Bytes())
	h.HeaderHMACValid = hmac.Equal(storedMAC[:], expectedMAC)

	var pl [4]byte
	if _, err = io.ReadFull(r, pl[:]); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read payload_len: %w", err)
	}
	encryptedPayloadLen = binary.BigEndian.Uint32(pl[:])

	nonce = make([]byte, 12)
	if _, err = io.ReadFull(r, nonce); err != nil {
		return h, 0, nil, fmt.Errorf("blob: read nonce: %w", err)
	}

	return h, encryptedPayloadLen, nonce, nil
}
