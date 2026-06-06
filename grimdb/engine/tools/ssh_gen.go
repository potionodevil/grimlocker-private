package tools

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"fmt"

	"golang.org/x/crypto/ssh"
)

// SSHKeyPair hält ein frisch generiertes Ed25519-Keypair in OpenSSH-kompatiblen Formaten.
// Der Private Key liegt als PEM-encoded OpenSSH Private Key vor (direkt in ~/.ssh/id_ed25519 speicherbar).
// Der Public Key ist im authorized_keys-Line-Format (z.B. "ssh-ed25519 AAAA… comment").
type SSHKeyPair struct {
	// PublicKey im OpenSSH authorized_keys-Format, z.B. "ssh-ed25519 AAAAC3Nza… user@host"
	PublicKey string `json:"public_key"`

	// PrivateKeyPEM ist der OpenSSH-PEM-encoded Private Key.
	// Sensibel — dieser Wert muss im Vault verschlüsselt gespeichert werden.
	PrivateKeyPEM []byte `json:"-"` // never serialized in JSON responses

	// Fingerprint im SHA-256-Format: "SHA256:…"
	Fingerprint string `json:"fingerprint"`

	// Comment wird an die Public-Key-Zeile angehängt.
	Comment string `json:"comment"`

	// EntryID wird gesetzt, nachdem der Key im Vault gespeichert wurde.
	EntryID string `json:"entry_id,omitempty"`
}

// GenerateEd25519Pair erzeugt ein Ed25519-Keypair via crypto/rand.
// comment wird an die Public-Key-Zeile gehängt (z.B. "user@host").
// passphrase: wenn leer, bleibt der PEM unverschlüsselt (trotzdem sicher, weil der Vault
// mit MVK verschlüsselt). Wenn nicht leer, wird der Private Key damit verschlüsselt.
func GenerateEd25519Pair(comment string, passphrase string) (SSHKeyPair, error) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return SSHKeyPair{}, fmt.Errorf("generate Ed25519 key: %w", err)
	}

	// Public Key ins OpenSSH authorized_keys-Format marshalen.
	sshPub, err := ssh.NewPublicKey(pubKey)
	if err != nil {
		return SSHKeyPair{}, fmt.Errorf("marshal SSH public key: %w", err)
	}

	pubLine := string(ssh.MarshalAuthorizedKey(sshPub))
	// ssh.MarshalAuthorizedKey hängt einen Newline an — wir strippen ihn und hängen
	// den Caller-Kommentar manuell an, damit der X-Label voll kontrolliert bleibt.
	if comment != "" {
		if len(pubLine) > 0 && pubLine[len(pubLine)-1] == '\n' {
			pubLine = pubLine[:len(pubLine)-1]
		}
		pubLine = pubLine + " " + comment + "\n"
	}

	// Fingerprint (SHA-256 in Base64) berechnen.
	fingerprint := ssh.FingerprintSHA256(sshPub)

	// Private Key ins OpenSSH PEM-Format marshalen.
	var privPEM []byte
	if passphrase != "" {
		privPEM, err = marshalEd25519PrivateKeyWithPassphrase(privKey, comment, passphrase)
	} else {
		privPEM, err = marshalEd25519PrivateKey(privKey, comment)
	}
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

// marshalEd25519PrivateKey encoded den Private Key als unverschlüsselten OpenSSH-PEM-Block.
func marshalEd25519PrivateKey(key ed25519.PrivateKey, comment string) ([]byte, error) {
	pemBlock, err := ssh.MarshalPrivateKey(key, comment)
	if err != nil {
		return nil, fmt.Errorf("MarshalPrivateKey: %w", err)
	}
	return pem.EncodeToMemory(pemBlock), nil
}

// marshalEd25519PrivateKeyWithPassphrase encoded den Private Key mit Passphrase als OpenSSH-PEM.
func marshalEd25519PrivateKeyWithPassphrase(key ed25519.PrivateKey, comment string, passphrase string) ([]byte, error) {
	pemBlock, err := ssh.MarshalPrivateKeyWithPassphrase(key, comment, []byte(passphrase))
	if err != nil {
		return nil, fmt.Errorf("MarshalPrivateKeyWithPassphrase: %w", err)
	}
	return pem.EncodeToMemory(pemBlock), nil
}

// generateSecurePassphrase erzeugt eine kryptografisch sichere Passphrase mit crypto/rand.
func generateSecurePassphrase(length int) (string, error) {
	const chars = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789!@#$%^&*()-_=+"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("passphrase: %w", err)
	}
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b), nil
}
