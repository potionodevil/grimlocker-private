package tools

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// SSHKeyPair holds a freshly generated Ed25519 key pair in OpenSSH-compatible
// formats. The private key is in PEM-encoded OpenSSH private key format so it
// can be written directly to ~/.ssh/id_ed25519. The public key is in the
// authorized_keys line format ("ssh-ed25519 AAAA… comment").
type SSHKeyPair struct {
	// PublicKey is the OpenSSH authorized_keys format, e.g.
	// "ssh-ed25519 AAAAC3Nza… user@host"
	PublicKey string `json:"public_key"`

	// PrivateKeyPEM is the OpenSSH PEM-encoded private key.
	// This is the sensitive field — it must be stored encrypted in the vault.
	PrivateKeyPEM []byte `json:"-"` // never serialized in JSON responses

	// Fingerprint is the SHA-256 fingerprint in the format "SHA256:…"
	Fingerprint string `json:"fingerprint"`

	// Comment is the key comment appended to the public key line.
	Comment string `json:"comment"`

	// EntryID is populated after the key pair is saved to the vault.
	EntryID string `json:"entry_id,omitempty"`
}

// GenerateEd25519Pair creates a fresh Ed25519 key pair using crypto/rand.
// comment is appended to the public key line (e.g. "user@host" or a label).
// Returns an SSHKeyPair with PrivateKeyPEM encoded in OpenSSH PEM format.
func GenerateEd25519Pair(comment string) (SSHKeyPair, error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return SSHKeyPair{}, fmt.Errorf("generate Ed25519 key: %w", err)
	}

	// Marshal the public key into the OpenSSH authorized_keys format.
	sshPub, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return SSHKeyPair{}, fmt.Errorf("marshal SSH public key: %w", err)
	}

	pubLine := string(ssh.MarshalAuthorizedKey(sshPub))
	// ssh.MarshalAuthorizedKey appends a newline — strip it; the comment is
	// already embedded in the marshalled key. Append the comment manually so
	// the caller can control the label.
	if comment != "" {
		// The authorized_keys format is: type base64key comment\n
		// ssh.MarshalAuthorizedKey already includes the type+key; we need to
		// append the comment before the trailing newline.
		if len(pubLine) > 0 && pubLine[len(pubLine)-1] == '\n' {
			pubLine = pubLine[:len(pubLine)-1]
		}
		pubLine = pubLine + " " + comment + "\n"
	}

	// Compute fingerprint (SHA-256 in base64).
	fingerprint := ssh.FingerprintSHA256(sshPub)

	// Marshal the private key in OpenSSH PEM format (openssh-key-v1 envelope).
	privPEM, err := marshalEd25519PrivateKey(privKey, comment)
	if err != nil {
		return SSHKeyPair{}, fmt.Errorf("marshal private key: %w", err)
	}

	return SSHKeyPair{
		PublicKey:     pubLine,
		PrivateKeyPEM: privPEM,
		Fingerprint:   fingerprint,
		Comment:       comment,
	}, nil
}

// marshalEd25519PrivateKey encodes an Ed25519 private key as an OpenSSH PEM block.
// Uses golang.org/x/crypto/ssh's MarshalPrivateKey function.
func marshalEd25519PrivateKey(key ed25519.PrivateKey, comment string) ([]byte, error) {
	pemBlock, err := ssh.MarshalPrivateKey(key, comment)
	if err != nil {
		return nil, fmt.Errorf("MarshalPrivateKey: %w", err)
	}
	return pem.EncodeToMemory(pemBlock), nil
}
