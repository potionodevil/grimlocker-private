package crypto

import (
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// KeyLen ist die erforderliche Länge für alle ChaCha20-Poly1305-Keys.
const KeyLen = chacha20poly1305.KeySize // 32

// ValidateKeyLength gibt nil zurück, wenn key genau 32 Bytes hat, sonst einen Error.
func ValidateKeyLength(key []byte) error {
	if len(key) == 0 {
		return fmt.Errorf("crypto/engine: key is empty (vault may be locked)")
	}
	if len(key) != KeyLen {
		return fmt.Errorf("crypto/engine: invalid key length %d (want %d)", len(key), KeyLen)
	}
	return nil
}

// ValidateAndNewCipher wrappt chacha20poly1305.New mit striktem Key-Length-Check.
// Gibt einen informativeren Error als die opake Standard-Bibliothek zurück.
func ValidateAndNewCipher(key []byte) (cipher interface {
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}, err error) {
	if err := ValidateKeyLength(key); err != nil {
		return nil, err
	}
	c, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("crypto/engine: cipher creation failed: %w", err)
	}
	return c, nil
}
