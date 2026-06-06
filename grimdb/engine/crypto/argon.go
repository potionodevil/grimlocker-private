package crypto

import (
	"crypto/hmac"
	"crypto/sha256"
	"fmt"

	"golang.org/x/crypto/argon2"
)

// DeriveArgon2id wandelt ein Passwort in einen sicheren Schlüssel um — mit Argon2id,
// dem Goldstandard für Memory-Hard-KDFs.
func (p *provider) DeriveArgon2id(password []byte, opts KDFOptions) ([]byte, error) {
	if len(opts.Salt) == 0 {
		return nil, fmt.Errorf("argon2id: salt is required")
	}
	if opts.KeyLen == 0 {
		opts.KeyLen = 32
	}
	if opts.Time == 0 {
		opts.Time = DefaultKDFOptions.Time
	}
	if opts.Memory == 0 {
		opts.Memory = DefaultKDFOptions.Memory
	}
	if opts.Threads == 0 {
		opts.Threads = DefaultKDFOptions.Threads
	}

	key := argon2.IDKey(password, opts.Salt, opts.Time, opts.Memory, opts.Threads, opts.KeyLen)
	return key, nil
}

// HMACKey leitet einen 32-Byte-HMAC-Key aus dem MVK via HMAC-SHA256 ab.
func (p *provider) HMACKey(mvk []byte) [32]byte {
	h := hmac.New(sha256.New, mvk)
	h.Write([]byte("grimlocker-hmac-v1"))
	result := h.Sum(nil)
	var key [32]byte
	copy(key[:], result)
	return key
}
