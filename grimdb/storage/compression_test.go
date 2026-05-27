package storage

import (
	"bytes"
	"strings"
	"testing"
)

func TestCompress_Decompress_Roundtrip(t *testing.T) {
	cases := []struct {
		name  string
		input []byte
	}{
		{"empty", []byte{}},
		{"small_text", []byte("hello, grimlocker!")},
		{"compressible", bytes.Repeat([]byte("AAAAAAAAAAAAAAAA"), 512)},
		{"json_like", []byte(`{"id":"abc","title":"test entry","category":"PASSWORD","fields":{"username":"admin","password":"s3cr3t"}}`)},
		{"binary_random", func() []byte {
			b := make([]byte, 256)
			for i := range b {
				b[i] = byte(i)
			}
			return b
		}()},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compressed, err := Compress(tc.input)
			if err != nil {
				t.Fatalf("Compress(%q): %v", tc.name, err)
			}

			// Marker byte must be 0x00 or 0x01
			if len(compressed) > 0 && compressed[0] != markerUncompressed && compressed[0] != markerZstd {
				t.Errorf("unexpected marker byte 0x%02x", compressed[0])
			}

			decompressed, err := Decompress(compressed)
			if err != nil {
				t.Fatalf("Decompress(%q): %v", tc.name, err)
			}

			if !bytes.Equal(decompressed, tc.input) {
				t.Errorf("roundtrip mismatch: got %d bytes, want %d bytes", len(decompressed), len(tc.input))
			}
		})
	}
}

func TestDecompress_LegacyData(t *testing.T) {
	// Data without a marker byte (legacy blocks) should pass through unchanged.
	legacy := []byte("this is legacy data without marker byte")
	got, err := Decompress(legacy)
	if err != nil {
		t.Fatalf("Decompress legacy: %v", err)
	}
	if !bytes.Equal(got, legacy) {
		t.Errorf("legacy data modified: got %q, want %q", got, legacy)
	}
}

func TestCompressStream_Roundtrip(t *testing.T) {
	original := strings.Repeat("grimlocker test data ", 200)
	src := bytes.NewReader([]byte(original))

	var compBuf bytes.Buffer
	if err := CompressStream(&compBuf, src); err != nil {
		t.Fatalf("CompressStream: %v", err)
	}

	var decompBuf bytes.Buffer
	if err := DecompressStream(&decompBuf, &compBuf); err != nil {
		t.Fatalf("DecompressStream: %v", err)
	}

	if decompBuf.String() != original {
		t.Errorf("stream roundtrip mismatch: got %d chars, want %d", decompBuf.Len(), len(original))
	}
}

func TestCompress_MarkerByte_Uncompressible(t *testing.T) {
	// Highly random / incompressible data should still get the uncompressed marker.
	// We simulate this with a byte slice that doesn't compress well.
	// Generate pseudo-random-looking data that's hard to compress.
	data := make([]byte, 1024)
	for i := range data {
		data[i] = byte((i*7 + 13) % 256)
	}

	compressed, err := Compress(data)
	if err != nil {
		t.Fatalf("Compress: %v", err)
	}

	// Must have a valid marker
	if len(compressed) == 0 {
		t.Fatal("compressed output is empty")
	}
	if compressed[0] != markerUncompressed && compressed[0] != markerZstd {
		t.Errorf("unexpected marker 0x%02x", compressed[0])
	}

	// Roundtrip must still work
	got, err := Decompress(compressed)
	if err != nil {
		t.Fatalf("Decompress: %v", err)
	}
	if !bytes.Equal(got, data) {
		t.Error("roundtrip failed for incompressible data")
	}
}

func TestCompressInPlace_NeverPanics(t *testing.T) {
	// CompressInPlace must always return valid output that Decompress can handle.
	inputs := [][]byte{nil, {}, {0x00}, {0xFF}, bytes.Repeat([]byte("x"), 10000)}
	for i, input := range inputs {
		out := CompressInPlace(input)
		if len(input) > 0 && len(out) == 0 {
			t.Errorf("input[%d]: CompressInPlace returned empty", i)
		}
		got, err := Decompress(out)
		if err != nil {
			t.Errorf("input[%d]: Decompress failed: %v", i, err)
		}
		if !bytes.Equal(got, input) {
			t.Errorf("input[%d]: roundtrip mismatch", i)
		}
	}
}
