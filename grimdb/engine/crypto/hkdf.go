package crypto

import (
	"crypto/sha256"
	"fmt"
	"io"

	"golang.org/x/crypto/hkdf"
)

// DeriveHKDF expandiert ein Secret via HKDF-SHA256 auf keyLen Bytes.
func (p *provider) DeriveHKDF(secret, salt, info []byte, keyLen int) ([]byte, error) {
	if keyLen <= 0 {
		return nil, fmt.Errorf("hkdf: keyLen must be > 0")
	}
	r := hkdf.New(sha256.New, secret, salt, info)
	key := make([]byte, keyLen)
	if _, err := io.ReadFull(r, key); err != nil {
		return nil, fmt.Errorf("hkdf: expand: %w", err)
	}
	return key, nil
}
