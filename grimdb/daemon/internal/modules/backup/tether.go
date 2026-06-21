package backup

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"

	engbackup "github.com/grimlocker/grimdb/engine/backup"
	"github.com/grimlocker/grimdb/engine/crypto"
)

// deriveVaultID derives a device-specific ID from the MVK and the vault's ArgonSalt.
// The VaultID never leaves memory — only its commitment is stored in the blob header.
func deriveVaultID(cryptoP crypto.Provider, mvk, argonSalt []byte) ([]byte, error) {
	return cryptoP.DeriveHKDF(mvk, argonSalt, []byte("GRIMBAK-VAULT-ID-v1"), 32)
}

// computeCommitment computes the hardware commitment stored in the blob header.
// HMAC-SHA256(vaultID, Magic||exportTimestamp) — unforgeable without the VaultID.
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

// tethersMatch returns true if the commitment in the header matches the current vault.
// Uses constant-time comparison to prevent timing attacks.
func tethersMatch(cryptoP crypto.Provider, mvk, argonSalt []byte, header engbackup.BlobHeader) (bool, error) {
	vaultID, err := deriveVaultID(cryptoP, mvk, argonSalt)
	if err != nil {
		return false, err
	}
	expected := computeCommitment(vaultID, header.ExportTimestamp)
	return hmac.Equal(expected[:], header.HardwareID[:]), nil
}
