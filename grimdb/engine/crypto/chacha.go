package crypto

import (
	"crypto/rand"
	"fmt"

	"golang.org/x/crypto/chacha20poly1305"
)

// Encrypt liefert ChaCha20-Poly1305-AEAD-Ciphertext.
// key muss 32 Bytes sein, nonce 12 Bytes.
func (p *provider) Encrypt(key, nonce, plaintext, aad []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("chacha: invalid key")
	}
	if len(nonce) != 12 {
		return nil, fmt.Errorf("chacha: invalid nonce")
	}

	cipher, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("chacha: create cipher: %w", err)
	}

	return cipher.Seal(nil, nonce, plaintext, aad), nil
}

// Decrypt verifiziert den Auth-Tag und gibt den Plaintext zurück.
func (p *provider) Decrypt(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, fmt.Errorf("chacha: invalid key")
	}
	if len(nonce) != 12 {
		return nil, fmt.Errorf("chacha: invalid nonce")
	}

	cipher, err := chacha20poly1305.New(key)
	if err != nil {
		return nil, fmt.Errorf("chacha: create cipher: %w", err)
	}

	plaintext, err := cipher.Open(nil, nonce, ciphertext, aad)
	if err != nil {
		return nil, fmt.Errorf("chacha: authentication failed")
	}
	return plaintext, nil
}

// NewNonce generiert ein kryptografisch sicheres 12-Byte-Nonce.
func (p *provider) NewNonce() ([12]byte, error) {
	var nonce [12]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nonce, fmt.Errorf("nonce: %w", err)
	}
	return nonce, nil
}
