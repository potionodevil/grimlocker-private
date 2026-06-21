// Package sdk — Tools client built on top of GQLClient.
// Provides typed methods for SSH key generation and recovery phrase retrieval.
package sdk

import (
	"context"

	"github.com/grimlocker/grimdb/engine/gql"
)

// SSHKeyResult holds the result of an Ed25519 SSH key pair generation.
type SSHKeyResult struct {
	PublicKey   string `json:"public_key"`
	Fingerprint string `json:"fingerprint"`
	EntryID     string `json:"entry_id"`
}

// GenerateSSHKey generates a new Ed25519 SSH key pair.
// If saveToVault is true, the private key is stored as a vault entry.
func (c *GQLClient) GenerateSSHKey(ctx context.Context, comment string, saveToVault bool) (*SSHKeyResult, error) {
	payload := map[string]interface{}{
		"comment":       comment,
		"save_to_vault": saveToVault,
	}
	raw, err := c.sendCommand(ctx, "default", gql.OpToolSSHGen, payload)
	if err != nil {
		return nil, err
	}
	var result SSHKeyResult
	if err := unmarshalResponse(raw, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// GetRecoveryPhrase retrieves the BIP39 recovery phrase used to recreate the MVK.
// The password parameter must match the current vault password for verification.
func (c *GQLClient) GetRecoveryPhrase(ctx context.Context, password string) (string, error) {
	payload := map[string]string{"password": password}
	raw, err := c.sendCommand(ctx, "default", gql.OpToolRecoveryPhrase, payload)
	if err != nil {
		return "", err
	}
	var result struct {
		Phrase string `json:"phrase"`
	}
	if err := unmarshalResponse(raw, &result); err != nil {
		return "", err
	}
	return result.Phrase, nil
}
