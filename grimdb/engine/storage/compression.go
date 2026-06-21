package storage

import (
	"bytes"
	"fmt"
	"io"

	"github.com/klauspost/compress/zstd"
)

// compressionMarker ist das erste Byte jedes Block.Data, wenn Compression aktiv ist.
// 0x01 → zstd-komprimiert; 0x00 → unkomprimiert (abwärtskompatibler Default).
const (
	markerUncompressed byte = 0x00
	markerZstd         byte = 0x01
)

// zstdEncoder ist ein package-level-Encoder, der über Aufrufe hinweg wiederverwendet wird (thread-safe).
var zstdEncoder, _ = zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedDefault))

// zstdDecoder ist ein package-level-Decoder, der wiederverwendet wird (thread-safe).
var zstdDecoder, _ = zstd.NewReader(nil)

// Compress komprimiert Daten mit zstd und stellt ein Marker-Byte voran.
// Der Marker stellt sicher, dass Decompress das Encoding auch ohne Out-of-Band-Metadaten erkennt.
// Compression passiert VOR der ChaCha20-Poly1305-Verschlüsselung.
//
// Bei Fehler werden die Original-Daten unverändert zurückgegeben (Graceful Degradation).
func Compress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	compressed := zstdEncoder.EncodeAll(data, make([]byte, 0, len(data)/2+1))

	// Wenn Compression die Größe nicht reduziert hat, Plaintext-Pfad nehmen.
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

// Decompress erkennt das Marker-Byte und dekomprimiert entsprechend.
// Der Aufruf von Decompress auf Daten, die NICHT von Compress produziert wurden
// (also legacy ohne Marker-Byte), ist sicher — wenn das erste Byte weder 0x00
// noch 0x01 ist, werden die Daten unverändert zurückgegeben (abwärtskompatibel).
func Decompress(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return data, nil
	}

	marker := data[0]
	payload := data[1:]

	switch marker {
	case markerUncompressed:
		return payload, nil

	case markerZstd:
		out, err := zstdDecoder.DecodeAll(payload, nil)
		if err != nil {
			return nil, fmt.Errorf("zstd decompress: %w", err)
		}
		return out, nil

	default:
		// Kein Marker-Byte — legacy Block, ganze Slice unverändert zurückgeben.
		return data, nil
	}
}

// CompressStream liest von r, komprimiert mit zstd und schreibt nach w.
// Von IngestEngine fürs Streaming großer Dateien genutzt. Der Stream-Output
// enthält KEIN Marker-Byte — Caller, die es brauchen, müssen es voranstellen.
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

// DecompressStream dekomprimiert einen zstd-Stream von r in w.
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

// CompressInPlace ist ein Helfer, der Compress aufruft und die komprimierten
// Bytes zurückgibt, oder die Original-Bytes bei Fehler (Graceful Degradation).
// Verwende das, wenn Compression nicht fehlschlagen darf (z.B. wo Datenintegrität
// wichtiger ist als Kompressionsrate).
func CompressInPlace(data []byte) []byte {
	out, err := Compress(data)
	if err != nil {
		safe := make([]byte, 1+len(data))
		safe[0] = markerUncompressed
		copy(safe[1:], data)
		return safe
	}
	return out
}

// compressReader wrappt einen io.Reader und komprimiert dessen Output in einen Buffer.
// Gibt die komprimierten Bytes und die Anzahl der gelesenen unkomprimierten Bytes zurück.
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
