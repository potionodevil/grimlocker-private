// Package backup (sign.go) implementiert die Ed25519-Signatur über den gesamten .grimbak-Blob.
//
// Warum Ed25519 statt dem konstant-key HMAC im Header?
// Der HeaderHMAC (Key = zero-Bytes) prüft nur Korruption — wer die Datei hat,
// kann einen gültigen HMAC erzeugen. Ed25519 beweist, dass der Blob vom echten Vault stammt.
//
// Signatur-Layout:
//
//	[Magic … EncryptedPayload]  ← wird über diesen gesamten Bereich signiert
//	[64 Bytes Ed25519-Signatur] ← angehängt am Ende der Datei (nach Payload)
//
// Der Public Key (32 Bytes) wird im Header gespeichert (FlagSigned → V2-Extension-Block),
// so dass die Signatur ohne MVK geprüft werden kann — nur mit dem eingebetteten Public Key.
//
// Key-Ableitung (deterministisch aus MVK):
//
//	sigSeed = HKDF-SHA256(MVK, salt=exportTimestamp(8 bytes), info="GRIMBAK-SIGN-KEY-v1", len=32)
//	privKey = ed25519.NewKeyFromSeed(sigSeed)
//	pubKey  = privKey.Public().(ed25519.PublicKey)
package backup

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
)

// signKeyInfo ist das HKDF-Info-Label für die Ed25519-Signing-Key-Ableitung.
var signKeyInfo = []byte("GRIMBAK-SIGN-KEY-v1")

// DeriveSigningKey leitet das Ed25519-Schlüsselpaar aus dem MVK und dem Export-Timestamp ab.
// Das Schlüsselpaar ist deterministisch reproduzierbar: HKDF-SHA256(MVK, salt=ts, info=label).
func DeriveSigningKey(mvk []byte, exportTimestamp int64) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	var tsBuf [8]byte
	binary.BigEndian.PutUint64(tsBuf[:], uint64(exportTimestamp))

	// HKDF-SHA256 über HMAC — identische Implementierung wie crypto.Provider.DeriveHKDF
	// aber ohne Abhängigkeit auf das crypto-Package.
	seed := hkdfSHA256(mvk, tsBuf[:], signKeyInfo, 32)

	privKey := ed25519.NewKeyFromSeed(seed)
	pubKey := privKey.Public().(ed25519.PublicKey)
	return privKey, pubKey, nil
}

// SignBlob signiert blob (Header + EncryptedPayload) mit privKey.
// Gibt die 64-Byte-Signatur zurück.
func SignBlob(blob []byte, privKey ed25519.PrivateKey) []byte {
	return ed25519.Sign(privKey, blob)
}

// VerifyBlobSignature prüft die Signatur über blob mit pubKey.
// blob = alles außer den letzten 64 Bytes (der Signatur selbst).
// sig = die 64 Bytes am Ende der Datei.
func VerifyBlobSignature(blob []byte, sig []byte, pubKey ed25519.PublicKey) bool {
	if len(sig) != ed25519.SignatureSize {
		return false
	}
	return ed25519.Verify(pubKey, blob, sig)
}

// hkdfSHA256 implementiert RFC 5869 HKDF-SHA256 ohne externe Abhängigkeit.
// Identisch mit crypto.Provider.DeriveHKDF — dupliziert um import-cycle zu vermeiden.
func hkdfSHA256(secret, salt, info []byte, keyLen int) []byte {
	// Extract
	if len(salt) == 0 {
		salt = make([]byte, sha256.Size)
	}
	prk := func() []byte {
		mac := hmac.New(sha256.New, salt)
		mac.Write(secret)
		return mac.Sum(nil)
	}()

	// Expand
	result := make([]byte, 0, keyLen)
	prev := []byte{}
	counter := byte(1)
	for len(result) < keyLen {
		mac := hmac.New(sha256.New, prk)
		mac.Write(prev)
		mac.Write(info)
		mac.Write([]byte{counter})
		block := mac.Sum(nil)
		result = append(result, block...)
		prev = block
		counter++
	}
	if len(result) < keyLen {
		panic(fmt.Sprintf("sign: hkdf output too short: %d < %d", len(result), keyLen))
	}
	return result[:keyLen]
}
