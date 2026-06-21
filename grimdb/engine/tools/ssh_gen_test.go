package tools

import (
	"bytes"
	"crypto/ed25519"
	"strings"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestGenerateEd25519Pair_BasicShape(t *testing.T) {
	pair, err := GenerateEd25519Pair("test@grimlocker", "")
	if err != nil {
		t.Fatalf("GenerateEd25519Pair: %v", err)
	}

	// Public-Key-Zeile muss mit dem OpenSSH-Type-Präfix starten.
	if !strings.HasPrefix(pair.PublicKey, "ssh-ed25519 ") {
		t.Errorf("public key does not start with 'ssh-ed25519 ': %q", pair.PublicKey[:min(50, len(pair.PublicKey))])
	}

	// Public-Key-Zeile muss den Kommentar enthalten.
	if !strings.Contains(pair.PublicKey, "test@grimlocker") {
		t.Errorf("public key line missing comment: %q", pair.PublicKey)
	}

	// Private-Key-PEM muss ein gültiger OpenSSH-Key-Block sein.
	if !bytes.Contains(pair.PrivateKeyPEM, []byte("OPENSSH PRIVATE KEY")) {
		t.Errorf("private key PEM missing OPENSSH PRIVATE KEY header")
	}

	// Fingerprint muss mit "SHA256:" beginnen.
	if !strings.HasPrefix(pair.Fingerprint, "SHA256:") {
		t.Errorf("fingerprint unexpected format: %q", pair.Fingerprint)
	}
}

func TestGenerateEd25519Pair_UniqueEachCall(t *testing.T) {
	p1, err := GenerateEd25519Pair("user1", "")
	if err != nil {
		t.Fatalf("GenerateEd25519Pair 1: %v", err)
	}
	p2, err := GenerateEd25519Pair("user2", "")
	if err != nil {
		t.Fatalf("GenerateEd25519Pair 2: %v", err)
	}

	if p1.PublicKey == p2.PublicKey {
		t.Error("two generated key pairs have identical public keys — CSPRNG failure?")
	}
	if p1.Fingerprint == p2.Fingerprint {
		t.Error("two generated key pairs have identical fingerprints")
	}
}

func TestGenerateEd25519Pair_PublicKeyValid(t *testing.T) {
	pair, err := GenerateEd25519Pair("verify@grimlocker", "")
	if err != nil {
		t.Fatalf("GenerateEd25519Pair: %v", err)
	}

	// Parse die OpenSSH authorized_keys-Zeile und stell sicher, dass es ein gültiger Ed25519-Key ist.
	parsed, comment, options, rest, parseErr := ssh.ParseAuthorizedKey([]byte(pair.PublicKey))
	if parseErr != nil {
		t.Fatalf("ssh.ParseAuthorizedKey: %v", parseErr)
	}
	_ = options
	_ = rest

	if parsed.Type() != ssh.KeyAlgoED25519 {
		t.Errorf("expected Ed25519 key type, got %s", parsed.Type())
	}

	if comment != "verify@grimlocker" {
		t.Errorf("expected comment %q, got %q", "verify@grimlocker", comment)
	}

	// Ed25519 Public Key ist 32 Bytes — verifizieren via CryptoPublicKey-Interface.
	if cryptoPub, ok := parsed.(ssh.CryptoPublicKey); ok {
		edPub, ok := cryptoPub.CryptoPublicKey().(ed25519.PublicKey)
		if !ok {
			t.Fatal("CryptoPublicKey is not ed25519.PublicKey")
		}
		if len(edPub) != ed25519.PublicKeySize {
			t.Errorf("Ed25519 public key size: got %d, want %d", len(edPub), ed25519.PublicKeySize)
		}
	} else {
		t.Fatal("parsed key does not implement ssh.CryptoPublicKey")
	}
}

func TestGenerateEd25519Pair_EmptyComment(t *testing.T) {
	pair, err := GenerateEd25519Pair("", "")
	if err != nil {
		t.Fatalf("GenerateEd25519Pair with empty comment: %v", err)
	}
	// Auch ohne Comment muss ein gültiger Public Key rauskommen.
	if !strings.HasPrefix(pair.PublicKey, "ssh-ed25519 ") {
		t.Errorf("unexpected public key format: %q", pair.PublicKey)
	}
}

func TestGenerateEd25519Pair_PrivateKeyParseable(t *testing.T) {
	pair, err := GenerateEd25519Pair("parse@test", "")
	if err != nil {
		t.Fatalf("GenerateEd25519Pair: %v", err)
	}

	// PEM-Block muss von golang.org/x/crypto/ssh parsbars sein.
	privKey, err := ssh.ParseRawPrivateKey(pair.PrivateKeyPEM)
	if err != nil {
		t.Fatalf("ssh.ParseRawPrivateKey: %v", err)
	}

	ed, ok := privKey.(*ed25519.PrivateKey)
	if !ok {
		t.Fatalf("expected *ed25519.PrivateKey, got %T", privKey)
	}

	if len(*ed) != ed25519.PrivateKeySize {
		t.Errorf("private key size: got %d, want %d", len(*ed), ed25519.PrivateKeySize)
	}
}

func TestGenerateEd25519Pair_WithPassphrase(t *testing.T) {
	pair, err := GenerateEd25519Pair("passphrase@test", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("GenerateEd25519Pair with passphrase: %v", err)
	}

	// PEM muss trotz Passphrase ein gültiger OpenSSH-Block sein.
	if !bytes.Contains(pair.PrivateKeyPEM, []byte("OPENSSH PRIVATE KEY")) {
		t.Errorf("encrypted private key PEM missing OPENSSH PRIVATE KEY header")
	}

	// Ohne Passphrase muss das Parsen fehlschlagen.
	_, err = ssh.ParseRawPrivateKey(pair.PrivateKeyPEM)
	if err == nil {
		t.Fatal("expected error when parsing passphrase-protected key without passphrase, got nil")
	}

	// Mit der richtigen Passphrase muss es klappen.
	_, err = ssh.ParseRawPrivateKeyWithPassphrase(pair.PrivateKeyPEM, []byte("correct-horse-battery-staple"))
	if err != nil {
		t.Fatalf("ssh.ParseRawPrivateKeyWithPassphrase: %v", err)
	}
}

func TestGenerateSecurePassphrase(t *testing.T) {
	p1, err := generateSecurePassphrase(32)
	if err != nil {
		t.Fatalf("generateSecurePassphrase: %v", err)
	}
	if len(p1) != 32 {
		t.Errorf("expected passphrase length 32, got %d", len(p1))
	}
	p2, err := generateSecurePassphrase(32)
	if err != nil {
		t.Fatalf("generateSecurePassphrase 2: %v", err)
	}
	if p1 == p2 {
		t.Error("two generated passphrases are identical — CSPRNG failure?")
	}
}
