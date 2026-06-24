// Package tools/totp implements RFC 6238 TOTP (Time-Based One-Time Password).
// Supported algorithms: SHA1, SHA256, SHA512.
// Digits: 6 or 8.  Period: any positive integer (default 30s).
package tools

import (
	"crypto/hmac"
	"crypto/sha1"  //nolint:gosec // SHA1 required by RFC 4226 / RFC 6238
	"crypto/sha256"
	"crypto/sha512"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"hash"
	"strings"
	"time"
)

// TOTPAlgorithm is the HMAC hash function used to generate the OTP.
type TOTPAlgorithm string

const (
	AlgoSHA1   TOTPAlgorithm = "SHA1"
	AlgoSHA256 TOTPAlgorithm = "SHA256"
	AlgoSHA512 TOTPAlgorithm = "SHA512"
)

// TOTPParams holds all parameters required to generate a TOTP code.
type TOTPParams struct {
	Secret    string        // Base32-encoded shared secret (case-insensitive, padding optional)
	Algorithm TOTPAlgorithm // SHA1 | SHA256 | SHA512
	Digits    int           // 6 or 8
	Period    int           // counter period in seconds (typically 30)
}

// TOTPResult is returned by GenerateTOTP.
type TOTPResult struct {
	Code      string // zero-padded OTP string (e.g. "048192")
	ExpiresIn int    // seconds until this code expires
	Counter   uint64 // HOTP counter value used (= Unix / period)
}

// GenerateTOTP generates a TOTP code at the given reference time.
// Pass time.Now() for production use; pass a fixed time in tests.
func GenerateTOTP(p TOTPParams, now time.Time) (TOTPResult, error) {
	if p.Secret == "" {
		return TOTPResult{}, fmt.Errorf("totp: secret must not be empty")
	}
	if p.Digits != 6 && p.Digits != 8 {
		return TOTPResult{}, fmt.Errorf("totp: digits must be 6 or 8, got %d", p.Digits)
	}
	if p.Period <= 0 {
		return TOTPResult{}, fmt.Errorf("totp: period must be > 0, got %d", p.Period)
	}

	// Normalise and decode secret.
	secret := strings.ToUpper(strings.ReplaceAll(p.Secret, " ", ""))
	// Add padding if missing.
	if rem := len(secret) % 8; rem != 0 {
		secret += strings.Repeat("=", 8-rem)
	}
	key, err := base32.StdEncoding.DecodeString(secret)
	if err != nil {
		return TOTPResult{}, fmt.Errorf("totp: base32 decode failed: %w", err)
	}

	unixSec := now.Unix()
	counter := uint64(unixSec) / uint64(p.Period)
	expiresIn := p.Period - int(unixSec)%p.Period

	code, err := hotp(key, counter, p.Digits, p.Algorithm)
	if err != nil {
		return TOTPResult{}, err
	}

	return TOTPResult{
		Code:      code,
		ExpiresIn: expiresIn,
		Counter:   counter,
	}, nil
}

// hotp computes an HMAC-based OTP per RFC 4226.
func hotp(key []byte, counter uint64, digits int, algo TOTPAlgorithm) (string, error) {
	var h func() hash.Hash
	switch algo {
	case AlgoSHA1, "":
		h = sha1.New
	case AlgoSHA256:
		h = sha256.New
	case AlgoSHA512:
		h = sha512.New
	default:
		return "", fmt.Errorf("totp: unsupported algorithm %q", algo)
	}

	mac := hmac.New(h, key)
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], counter)
	mac.Write(msg[:])
	sum := mac.Sum(nil)

	// Dynamic truncation per RFC 4226 §5.4.
	offset := sum[len(sum)-1] & 0x0f
	binCode := (uint32(sum[offset])&0x7f)<<24 |
		uint32(sum[offset+1])<<16 |
		uint32(sum[offset+2])<<8 |
		uint32(sum[offset+3])

	mod := uint32(1)
	for i := 0; i < digits; i++ {
		mod *= 10
	}
	otp := binCode % mod

	return fmt.Sprintf("%0*d", digits, otp), nil
}
