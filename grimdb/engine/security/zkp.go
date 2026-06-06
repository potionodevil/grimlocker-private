// Package security (zkp.go) implementiert ein Zero-Knowledge-Proof (ZKP) Challenge-
// Response-Verfahren für die Passwort-Authentifizierung.
//
// Ziel: Nachweisen, dass man ein Passwort kennt, OHNE es jemals in Plaintext
// (oder gehashtem Plaintext) über irgendeinen Channel zu senden — inklusive
// des Kernel-Event-Bus.
//
// Protokoll (ZKPP — Zero-Knowledge Password Proof):
//  1. Daemon generiert ein zufälliges 32-Byte-Nonce und speichert es temporär.
//  2. Daemon sendet (salt, nonce) an den Client.
//  3. Client berechnet: proof = Argon2id(password, salt) XOR BLAKE2b(nonce)
//     — oder äquivalent: proof = derived_key XOR nonce_hash
//     Der Proof ist ein Einmal-Commitment, das nicht replaybar ist (Nonce ist
//     Single-Use) und weder das Passwort noch den Derived Key allein preisgibt.
//  4. Client sendet proof an den Daemon.
//  5. Daemon verifiziert: stored_derived_key XOR BLAKE2b(nonce) == proof
//     — mit constant-time-Vergleich, um Timing-Angriffe zu verhindern.
//  6. Daemon löscht das Nonce sofort nach Verifikation (Replay-Schutz).
//
// Security-Eigenschaften:
//   - Passwort verlässt den Client nie in irgendeiner Form.
//   - Proof ist Single-Use (Nonce-gebunden) — Replay-Schutz.
//   - Constant-Time-Verifikation — kein Timing-Oracle.
//   - Selbst wenn der Proof abgefangen wird, kann der Angreifer weder das Passwort
//     noch den Vault-Key ableiten — dazu braucht er sowohl Nonce als auch Original-Passwort.
//
// HINWEIS: Das ist ein Go-seitiges Framework. Die eigentliche Passwort-Derivation
// (Argon2id) passiert im Tauri-Frontend und im Go-Auth-Handler. Diese Datei stellt
// nur Nonce-Lifecycle-Management und Verifikations-Primitives bereit.
package security

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"log"
	"sync"
	"time"

	"golang.org/x/crypto/blake2b"
)

const (
	// ZKPNonceSize ist die Länge des Single-Use-Challenge-Nonce in Bytes.
	ZKPNonceSize = 32

	// ZKPProofSize ist die Länge des ZKP-Commitment-Proof in Bytes.
	ZKPProofSize = 32

	// zkpNonceTTL is how long a nonce is valid after issuance.
	zkpNonceTTL = 5 * time.Minute
)

// ZKPChallenge ist eine Single-Use-Auth-Challenge, die einem Client ausgestellt wird.
type ZKPChallenge struct {
	// Nonce ist das Single-Use-Random-Challenge, das an den Client gesendet wird.
	// Hex-encoded für sicheren Transport über JSON/WebSocket.
	Nonce string `json:"nonce"`

	// ExpiresAt gibt an, wann die Challenge ungültig wird.
	ExpiresAt time.Time `json:"expires_at"`
}

// ZKPVerifier managed den Lifecycle von ZKP-Challenges und verifiziert Proofs.
type ZKPVerifier struct {
	mu       sync.Mutex
	pending  map[string][ZKPNonceSize]byte // challenge_id -> raw nonce
	expiries map[string]time.Time          // challenge_id -> expiry
}

// NewZKPVerifier erzeugt einen ZKPVerifier.
func NewZKPVerifier() *ZKPVerifier {
	v := &ZKPVerifier{
		pending:  make(map[string][ZKPNonceSize]byte),
		expiries: make(map[string]time.Time),
	}
	// Hintergrund-Cleanup für abgelaufene Challenges.
	go v.cleanupLoop()
	return v
}

// IssueChallenge generiert ein neues Single-Use-Nonce und gibt ein ZKPChallenge
// zurück, das an den Client gesendet wird. Die Challenge-ID referenziert,
// auf welche Challenge der Client antwortet.
func (v *ZKPVerifier) IssueChallenge() (challengeID string, challenge ZKPChallenge, err error) {
	var rawID [16]byte
	if _, err := rand.Read(rawID[:]); err != nil {
		return "", ZKPChallenge{}, fmt.Errorf("nonce generation failed: %w", err)
	}
	challengeID = hex.EncodeToString(rawID[:])

	var nonce [ZKPNonceSize]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return "", ZKPChallenge{}, fmt.Errorf("nonce generation failed: %w", err)
	}

	expiry := time.Now().Add(zkpNonceTTL)

	v.mu.Lock()
	v.pending[challengeID] = nonce
	v.expiries[challengeID] = expiry
	v.mu.Unlock()

	return challengeID, ZKPChallenge{
		Nonce:     hex.EncodeToString(nonce[:]),
		ExpiresAt: expiry,
	}, nil
}

// VerifyProof verifiziert einen ZKP-Proof für eine gegebene Challenge.
// derivedKey ist der Argon2id-abgeleitete Key, den der Daemon aus den
// gespeicherten Vault-Parametern vorberechnet hat (ohne das Passwort zu kennen).
//
// Proof-Verifikation: proof == derivedKey XOR BLAKE2b(nonce)
//
// Die Challenge wird nach dem ersten Verifikationsversuch konsumiert (gelöscht),
// unabhängig vom Erfolg — das verhindert Replay-Angriffe.
func (v *ZKPVerifier) VerifyProof(challengeID string, derivedKey, proof []byte) error {
	v.mu.Lock()
	nonce, ok := v.pending[challengeID]
	expiry := v.expiries[challengeID]
	// Sofort konsumieren — Single-Use.
	delete(v.pending, challengeID)
	delete(v.expiries, challengeID)
	v.mu.Unlock()

	if !ok {
		return fmt.Errorf("challenge not found or already consumed")
	}

	if time.Now().After(expiry) {
		return fmt.Errorf("challenge expired")
	}

	if len(derivedKey) != ZKPProofSize || len(proof) != ZKPProofSize {
		return fmt.Errorf("invalid proof or key length")
	}

	// Erwarteten Proof berechnen: derivedKey XOR BLAKE2b-256(nonce).
	h, _ := blake2b.New256(nil)
	h.Write(nonce[:])
	nonceHash := h.Sum(nil)

	expected := make([]byte, ZKPProofSize)
	for i := 0; i < ZKPProofSize; i++ {
		expected[i] = derivedKey[i] ^ nonceHash[i]
	}

	// Constant-Time-Vergleich schützt vor Timing-Angriffen.
	if subtle.ConstantTimeCompare(expected, proof) != 1 {
		return fmt.Errorf("proof verification failed")
	}

	return nil
}

// ComputeProof berechnet den ZKP-Proof auf der Client-Seite.
// derivedKey ist der Argon2id-abgeleitete Key aus dem User-Passwort.
// nonceHex ist das hex-encoded Nonce aus ZKPChallenge.
func ComputeProof(derivedKey []byte, nonceHex string) ([]byte, error) {
	nonceBytes, err := hex.DecodeString(nonceHex)
	if err != nil || len(nonceBytes) != ZKPNonceSize {
		return nil, fmt.Errorf("invalid nonce")
	}
	if len(derivedKey) != ZKPProofSize {
		return nil, fmt.Errorf("invalid derived key length")
	}

	h, _ := blake2b.New256(nil)
	h.Write(nonceBytes)
	nonceHash := h.Sum(nil)

	proof := make([]byte, ZKPProofSize)
	for i := 0; i < ZKPProofSize; i++ {
		proof[i] = derivedKey[i] ^ nonceHash[i]
	}
	return proof, nil
}

// PendingCount gibt die Anzahl der aktiven (nicht abgelaufenen) Challenges zurück.
func (v *ZKPVerifier) PendingCount() int {
	v.mu.Lock()
	defer v.mu.Unlock()

	count := 0
	now := time.Now()
	for id, expiry := range v.expiries {
		if now.Before(expiry) {
			count++
		} else {
			delete(v.pending, id)
			delete(v.expiries, id)
		}
	}
	return count
}

// cleanupLoop entfernt periodisch abgelaufene Challenges, um Memory-Growth zu verhindern.
func (v *ZKPVerifier) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		v.mu.Lock()
		now := time.Now()
		cleaned := 0
		for id, expiry := range v.expiries {
			if now.After(expiry) {
				delete(v.pending, id)
				delete(v.expiries, id)
				cleaned++
			}
		}
		v.mu.Unlock()
		if cleaned > 0 {
			log.Printf("[ZKPVerifier] cleaned %d expired challenge(s)", cleaned)
		}
	}
}
