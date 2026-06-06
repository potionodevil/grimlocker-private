package bridge

import (
	"crypto/sha256"
	"fmt"
	"io"
	"unsafe"

	"golang.org/x/crypto/hkdf"
)

// GoSecureZero überschreibt b mit Nullen — der Compiler kann das nicht wegoptimieren.
func GoSecureZero(b []byte) {
	for i := range b {
		b[i] = 0
	}
	_ = *(*byte)(unsafe.Pointer(&b))
}

// GoDeriveCoordinate extrahiert Bytes an den gegebenen Offsets aus EntropyData
// und durchläuft SHA-256→HKDF für einen 32-Byte-Key.
func GoDeriveCoordinate(entropyData []byte, offsets []int64) ([]byte, error) {
	extracted := make([]byte, len(offsets))
	for i, off := range offsets {
		if off < 0 || int(off) >= len(entropyData) {
			return nil, fmt.Errorf("coordinate: offset %d out of range", off)
		}
		extracted[i] = entropyData[off]
	}
	h := sha256.Sum256(extracted)
	r := hkdf.New(sha256.New, h[:], nil, []byte("grimlocker-coordinate-salt-v1"))
	key := make([]byte, 32)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, fmt.Errorf("coordinate: hkdf: %w", err)
	}
	return key, nil
}

// GoDeriveWorkspaceKey leitet einen Workspace-spezifischen Key via HKDF-SHA256 ab.
func GoDeriveWorkspaceKey(masterKey []byte, workspaceID string) ([32]byte, error) {
	var key [32]byte
	r := hkdf.New(sha256.New, masterKey, nil, []byte("grimlocker-workspace-v1:"+workspaceID))
	if _, err := io.ReadFull(r, key[:]); err != nil {
		return key, fmt.Errorf("workspace key: hkdf: %w", err)
	}
	return key, nil
}
