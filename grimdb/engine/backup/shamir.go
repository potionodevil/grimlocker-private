// Package backup (shamir.go) implementiert Shamir's Secret Sharing über GF(2^8).
//
// Einsatzzweck: Den Backup-Verschlüsselungskey (32 Bytes) in N Shares aufteilen,
// sodass mindestens K Shares benötigt werden, um den Key wiederherzustellen.
// Jeder Share ist (x, f(x)) — x ist der Share-Index (1..N), f(x) sind die 32 Byte.
//
// Algorithmus:
//   - Split: Erzeuge ein zufälliges Polynom f(x) Grad K-1 über GF(2^8), sodass f(0) = secret.
//     Werte f an den Punkten x=1..N aus — je ein Share.
//   - Combine: Lagrange-Interpolation über GF(2^8) gibt f(0) aus K beliebigen Shares zurück.
//
// GF(2^8)-Arithmetik: irreduzibles Polynom x^8 + x^4 + x^3 + x + 1 (AES-Standard, 0x11b).
//
// Keine externe Abhängigkeit — identischer Algorithmus wie hashicorp/vault/shamir (MIT).
package backup

import (
	"crypto/rand"
	"errors"
	"fmt"
)

// ShamirShare ist ein einzelner Share: (X, Y), wobei Y ein 32-Byte-Vektor ist.
// X ist der Share-Index (1..N). X=0 ist der geheime Wert selbst — wird nie ausgegeben.
type ShamirShare struct {
	X byte   // 1..255
	Y []byte // len(Y) == len(secret)
}

// SplitSecret teilt secret in n Shares auf, von denen mindestens k benötigt werden.
// Bedingungen: 2 ≤ k ≤ n ≤ 255, len(secret) > 0.
func SplitSecret(secret []byte, n, k int) ([]ShamirShare, error) {
	if k < 2 || k > n || n > 255 {
		return nil, fmt.Errorf("shamir: invalid params: n=%d k=%d (need 2 ≤ k ≤ n ≤ 255)", n, k)
	}
	if len(secret) == 0 {
		return nil, errors.New("shamir: empty secret")
	}

	shares := make([]ShamirShare, n)
	for i := range shares {
		shares[i] = ShamirShare{X: byte(i + 1), Y: make([]byte, len(secret))}
	}

	// Für jedes Byte des Secrets ein unabhängiges Polynom erzeugen.
	coeffs := make([]byte, k)
	for byteIdx, s := range secret {
		coeffs[0] = s
		if _, err := rand.Read(coeffs[1:]); err != nil {
			return nil, fmt.Errorf("shamir: rand: %w", err)
		}
		for i, sh := range shares {
			shares[i].Y[byteIdx] = gfPolyEval(coeffs, sh.X)
		}
	}
	return shares, nil
}

// CombineShares stellt das Secret aus mindestens k Shares wieder her.
// Die Shares müssen paarweise verschiedene X-Werte haben. Len(shares) ≥ k.
func CombineShares(shares []ShamirShare) ([]byte, error) {
	if len(shares) < 2 {
		return nil, errors.New("shamir: need at least 2 shares")
	}
	// Alle Y müssen gleich lang sein.
	secretLen := len(shares[0].Y)
	for _, sh := range shares[1:] {
		if len(sh.Y) != secretLen {
			return nil, errors.New("shamir: share Y-lengths differ")
		}
	}
	// X-Werte müssen eindeutig sein.
	seen := make(map[byte]bool)
	xs := make([]byte, len(shares))
	for i, sh := range shares {
		if seen[sh.X] {
			return nil, fmt.Errorf("shamir: duplicate share X=%d", sh.X)
		}
		seen[sh.X] = true
		xs[i] = sh.X
	}

	secret := make([]byte, secretLen)
	for byteIdx := range secret {
		ys := make([]byte, len(shares))
		for i, sh := range shares {
			ys[i] = sh.Y[byteIdx]
		}
		secret[byteIdx] = gfLagrange(xs, ys)
	}
	return secret, nil
}

// ─── GF(2^8) Arithmetik ──────────────────────────────────────────────────────

// gfMul multipliziert a und b in GF(2^8) mit Reduktionspolynom 0x11b.
func gfMul(a, b byte) byte {
	var p byte
	for i := 0; i < 8; i++ {
		if b&1 != 0 {
			p ^= a
		}
		hiSet := a&0x80 != 0
		a <<= 1
		if hiSet {
			a ^= 0x1b // x^8 + x^4 + x^3 + x + 1 mod 0x100
		}
		b >>= 1
	}
	return p
}

// gfDiv dividiert a durch b in GF(2^8).
func gfDiv(a, b byte) byte {
	if b == 0 {
		panic("shamir: gfDiv by zero")
	}
	return gfMul(a, gfInv(b))
}

// gfInv gibt das multiplikative Inverse von a in GF(2^8) zurück (via Fermat: a^(254)).
func gfInv(a byte) byte {
	if a == 0 {
		return 0
	}
	result := byte(1)
	base := a
	exp := 254
	for exp > 0 {
		if exp&1 != 0 {
			result = gfMul(result, base)
		}
		base = gfMul(base, base)
		exp >>= 1
	}
	return result
}

// gfPolyEval wertet das Polynom coeffs an der Stelle x aus.
// coeffs[0] ist der konstante Term (f(0) = secret byte).
func gfPolyEval(coeffs []byte, x byte) byte {
	result := byte(0)
	xPow := byte(1)
	for _, c := range coeffs {
		result ^= gfMul(c, xPow)
		xPow = gfMul(xPow, x)
	}
	return result
}

// gfLagrange interpoliert f(0) aus den Punkten (xs[i], ys[i]) via Lagrange über GF(2^8).
func gfLagrange(xs, ys []byte) byte {
	result := byte(0)
	for i := range xs {
		num := byte(1)
		den := byte(1)
		for j := range xs {
			if i == j {
				continue
			}
			// Nummerator: ∏ (0 - xs[j]) = ∏ xs[j]   (in GF(2^8): -x == x)
			num = gfMul(num, xs[j])
			// Denominator: ∏ (xs[i] - xs[j])
			den = gfMul(den, xs[i]^xs[j])
		}
		result ^= gfMul(ys[i], gfDiv(num, den))
	}
	return result
}
