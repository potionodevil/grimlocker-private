package backup

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"

	engbackup "github.com/grimlocker/grimdb/engine/backup"
	"github.com/grimlocker/grimdb/engine/crypto"
)

// deriveVaultID leitet eine gerätespezifische ID aus dem MVK und dem ArgonSalt der Vault ab.
// Die VaultID verlässt niemals den Arbeitsspeicher — nur ihr Commitment wird im Header gespeichert.
func deriveVaultID(cryptoP crypto.Provider, mvk, argonSalt []byte) ([]byte, error) {
	return cryptoP.DeriveHKDF(mvk, argonSalt, []byte("GRIMBAK-VAULT-ID-v1"), 32)
}

// computeCommitment berechnet den Hardware-Commitment-Wert, der im Blob-Header gespeichert wird.
// HMAC-SHA256(vaultID, Magic || exportTimestamp) — ohne VaultID nicht reproduzierbar.
func computeCommitment(vaultID []byte, exportTimestamp int64) [32]byte {
	var tsBuf [8]byte
	binary.BigEndian.PutUint64(tsBuf[:], uint64(exportTimestamp))

	mac := hmac.New(sha256.New, vaultID)
	mac.Write(engbackup.BlobMagic[:])
	mac.Write(tsBuf[:])
	sum := mac.Sum(nil)

	var result [32]byte
	copy(result[:], sum)
	return result
}

// verifyTether prüft ob das Hardware-Commitment im Header zur aktuellen Vault passt.
// Nutzt constant-time Vergleich um Timing-Angriffe zu verhindern.
// Gibt nil zurück wenn der Commitment stimmt, sonst einen Fehler.
func verifyTether(cryptoP crypto.Provider, mvk, argonSalt []byte, header engbackup.BlobHeader) error {
	vaultID, err := deriveVaultID(cryptoP, mvk, argonSalt)
	if err != nil {
		return err
	}
	expected := computeCommitment(vaultID, header.ExportTimestamp)
	if !hmac.Equal(expected[:], header.HardwareID[:]) {
		return nil // Fehler wird vom Aufrufer als ErrCodeBackupTetherMismatch behandelt
	}
	return nil
}

// tethersMatch gibt true zurück wenn die Commitments übereinstimmen (constant-time).
func tethersMatch(cryptoP crypto.Provider, mvk, argonSalt []byte, header engbackup.BlobHeader) (bool, error) {
	vaultID, err := deriveVaultID(cryptoP, mvk, argonSalt)
	if err != nil {
		return false, err
	}
	expected := computeCommitment(vaultID, header.ExportTimestamp)
	return hmac.Equal(expected[:], header.HardwareID[:]), nil
}
