package storage

import (
	"bytes"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
)

// compressionMarker is the first byte of every Block.Data when compression is active.
// 0x01 → zstd-compressed; 0x00 → uncompressed (backward-compatible default).
const (
	markerUncompressed byte = 0x00
	markerZstd         byte = 0x01
)

// zstdEncoder is a package-level encoder reused across calls (thread-safe).
var zstdEncoder, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))

// zstdDecoder is a package-level decoder reused across calls (thread-safe).
var zstdDecoder, _ = zstd.NewReader(nil)

// Compress compresses data with zstd and prepends a marker byte.
// The marker byte ensures Decompress can detect the encoding even without
// out-of-band metadata. Compression happens BEFORE ChaCha20-Poly1305 encryption.
//
// On error the original data is returned as-is (graceful degradation).
func Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	compressed := zstdEncoder.EncodeAll(data, make([]byte, 0, len(data)/2+1))

	// If compression did not reduce size, return the plaintext path.
	// Still prepend the marker so Decompress knows what to expect.
	if len(compressed) >= len(data) {
		out := make([]byte, 1+len(data))
		out[0] = markerUncompressed
		copy(out[1:], data)
		return out, nil
	}

	out := make([]byte, 1+len(compressed))
	out[0] = markerZstd
	copy(out[1:], compressed)
	return out, nil
}

// Decompress detects the marker byte and decompresses accordingly.
// Calling Decompress on data that was NOT produced by Compress (i.e. legacy
// blocks without a marker byte) is safe — if the first byte is neither 0x00
// nor 0x01, the data is returned unchanged (backward-compatible).
func Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	marker := data[0]
	payload := data[1:]

	switch marker {
	case markerUncompressed:
		// Stored uncompressed with a marker — return payload as-is.
		return payload, nil

	case markerZstd:
		// zstd-compressed path.
		out, err := zstdDecoder.DecodeAll(payload, nil)
		if err != nil {
			return nil, fmt.Errorf("zstd decompress: %w", err)
		}
		return out, nil

	default:
		// No marker byte — legacy block, return the whole slice unchanged.
		return data, nil
	}
}

// CompressStream reads from r, compresses with zstd and writes to w.
// Used by IngestEngine for streaming large files. The stream output does NOT
// include the marker byte — callers that need the marker should prepend it.
func CompressStream(w io.Writer, r io.Reader) error {
	enc, err := zstd.NewWriter(w, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return fmt.Errorf("zstd stream encoder: %w", err)
	}
	if _, err := io.Copy(enc, r); err != nil {
		enc.Close()
		return fmt.Errorf("zstd stream compress: %w", err)
	}
	return enc.Close()
}

// DecompressStream decompresses a zstd stream from r into w.
func DecompressStream(w io.Writer, r io.Reader) error {
	dec, err := zstd.NewReader(r)
	if err != nil {
		return fmt.Errorf("zstd stream decoder: %w", err)
	}
	defer dec.Close()
	if _, err := io.Copy(w, dec); err != nil {
		return fmt.Errorf("zstd stream decompress: %w", err)
	}
	return nil
}

// CompressInPlace is a helper that calls Compress and returns the compressed
// bytes, or the original bytes if an error occurs (graceful degradation).
// Use this when you cannot afford to fail on compression (e.g. path where
// data integrity matters more than compression ratio).
func CompressInPlace(data []byte) []byte {
	out, err := Compress(data)
	if err != nil {
		// Gracefully prepend the uncompressed marker so Decompress still works.
		safe := make([]byte, 1+len(data))
		safe[0] = markerUncompressed
		copy(safe[1:], data)
		return safe
	}
	return out
}

// compressReader wraps an io.Reader and compresses its output to a buffer.
// Returns the compressed bytes and the number of uncompressed bytes read.
func compressReader(r io.Reader) ([]byte, int64, error) {
	var buf bytes.Buffer
	enc, err := zstd.NewWriter(&buf, zstd.WithEncoderLevel(zstd.SpeedDefault))
	if err != nil {
		return nil, 0, err
	}
	n, err := io.Copy(enc, r)
	if err != nil {
		enc.Close()
		return nil, n, err
	}
	if err := enc.Close(); err != nil {
		return nil, n, err
	}
	return buf.Bytes(), n, nil
}
