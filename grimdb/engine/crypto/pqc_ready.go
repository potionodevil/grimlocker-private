// Package crypto (pqc_ready.go) stellt das Post-Quantum-Kryptografie (PQC)-Framework
// und die Migrationsinfrastruktur für Grimlocker Omega+ bereit.
//
// Aktueller Stand: AES-256-GCM und ChaCha20-Poly1305 gelten beide als quantensicher
// für symmetrische Verschlüsselung (Grover's Algorithmus reduziert die effektive
// Key-Stärke von 256 auf 128 Bit — immer noch sicher).
//
// Zukünftiger Migrationspfad:
//   - Key Encapsulation: CRYSTALS-Kyber (NIST PQC-Standard, ML-KEM)
//   - Signatures:        CRYSTALS-Dilithium (NIST PQC-Standard, ML-DSA)
//   - Hybrid-Mode:       Classic ECDH + Kyber KEM (belt-and-suspenders während
//     der Transition; schützt sowohl gegen klassische als auch Quanten-Angriffe)
//
// Diese Datei definiert das PQCProvider-Interface und einen PQCStatus-Report,
// damit das System zur Laufzeit über seinen Post-Quantum-Readiness befragt werden
// kann. Wenn PQC-Libraries im Go-Ökosystem verfügbar sind, implementier das
// Interface und tausche es via PQCProvider-Injection aus — keine anderen Code-Änderungen nötig.
//
// References:
//   - NIST FIPS 203 (ML-KEM / CRYSTALS-Kyber)
//   - NIST FIPS 204 (ML-DSA / CRYSTALS-Dilithium)
//   - NIST SP 800-227 (Empfehlungen für Key-Establishment)
package crypto

import "fmt"

// PQCAlgorithm identifiziert einen Post-Quantum-Kryptografie-Algorithmus.
type PQCAlgorithm string

const (
	// AlgoMLKEM768 ist CRYSTALS-Kyber auf 192-Bit-Sicherheitsniveau (NIST ML-KEM-768).
	AlgoMLKEM768 PQCAlgorithm = "ML-KEM-768"

	// AlgoMLDSA65 ist CRYSTALS-Dilithium auf 192-Bit-Sicherheitsniveau (NIST ML-DSA-65).
	AlgoMLDSA65 PQCAlgorithm = "ML-DSA-65"

	// AlgoHybridX25519MLKEM ist der hybride X25519 + ML-KEM-Key-Agreement,
	// empfohlen während der klassisch→post-quanten-Migrationsphase.
	AlgoHybridX25519MLKEM PQCAlgorithm = "X25519+ML-KEM-768"

	// AlgoChaCha20Poly1305 ist der aktuelle symmetrische Algorithmus (quantensicher bei 256 Bit).
	AlgoChaCha20Poly1305 PQCAlgorithm = "ChaCha20-Poly1305"
)

// PQCSafetyLevel beschreibt, wie gut ein Algorithmus Quantenangriffen widersteht.
type PQCSafetyLevel int

const (
	// PQCSafetyClassical bedeutet: sicher gegen klassische Computer,
	// aber von einem ausreichend großen Quantencomputer brechbar.
	PQCSafetyClassical PQCSafetyLevel = iota

	// PQCSafetyQuantumReduced bedeutet: Sicherheit gegen Quantenangriffe,
	// aber mit reduzierter Bit-Stärke (z.B. AES-256 → 128 Bit gegen Grover).
	PQCSafetyQuantumReduced

	// PQCSafetyQuantumFull bedeutet: speziell entwickelt, um Quantenangriffen
	// auf vollem Niveau zu widerstehen.
	PQCSafetyQuantumFull
)

// PQCStatus beschreibt den Post-Quantum-Readiness der aktuellen Konfiguration.
type PQCStatus struct {
	SymmetricAlgo    PQCAlgorithm   `json:"symmetric_algo"`
	SymmetricSafety  PQCSafetyLevel `json:"symmetric_safety"`
	KEMAvailable     bool           `json:"kem_available"`
	SigAvailable     bool           `json:"sig_available"`
	HybridMode       bool           `json:"hybrid_mode"`
	Notes            []string       `json:"notes"`
}

// CurrentPQCStatus gibt den Post-Quantum-Readiness des aktuellen Builds zurück.
// Version 1 — nur klassische Algorithmen mit quantensicherer symmetrischer Crypto.
func CurrentPQCStatus() PQCStatus {
	return PQCStatus{
		SymmetricAlgo:   AlgoChaCha20Poly1305,
		SymmetricSafety: PQCSafetyQuantumReduced,
		KEMAvailable:    false,
		SigAvailable:    false,
		HybridMode:      false,
		Notes: []string{
			"ChaCha20-Poly1305 mit 256-Bit-Keys ist quantensicher für symmetrische Operationen (Grover reduziert auf 128 Bit — immer noch sicher).",
			"Key Encapsulation (ML-KEM / CRYSTALS-Kyber) ist noch nicht integriert. Wenn verfügbar, HybridMode für X25519 + ML-KEM aktivieren.",
			"Signatur-Scheme (ML-DSA / CRYSTALS-Dilithium) ist noch nicht integriert.",
			"Migrationspfad: PQCProvider-Interface implementieren und via SetPQCProvider() injizieren.",
		},
	}
}

// PQCProvider ist das Interface für Post-Quantum-Krypto-Operationen.
// Dieses Interface implementieren, sobald ML-KEM / ML-DSA Libraries im Go-Ökosystem
// verfügbar sind, dann via SetPQCProvider() injizieren.
type PQCProvider interface {
	GenerateKEMKeyPair() (publicKey, privateKey []byte, err error)
	Encapsulate(publicKey []byte) (ciphertext, sharedSecret []byte, err error)
	Decapsulate(privateKey, ciphertext []byte) (sharedSecret []byte, err error)
	Algorithm() PQCAlgorithm
}

// notImplementedPQCProvider ist ein Platzhalter, der klare Error-Messages zurückgibt.
// Durch echte Implementierung ersetzen, wenn ML-KEM verfügbar ist.
type notImplementedPQCProvider struct{}

func (n *notImplementedPQCProvider) GenerateKEMKeyPair() ([]byte, []byte, error) {
	return nil, nil, fmt.Errorf("PQC not yet implemented — see crypto/pqc_ready.go migration notes")
}

func (n *notImplementedPQCProvider) Encapsulate([]byte) ([]byte, []byte, error) {
	return nil, nil, fmt.Errorf("PQC not yet implemented — see crypto/pqc_ready.go migration notes")
}

func (n *notImplementedPQCProvider) Decapsulate([]byte, []byte) ([]byte, error) {
	return nil, fmt.Errorf("PQC not yet implemented — see crypto/pqc_ready.go migration notes")
}

func (n *notImplementedPQCProvider) Algorithm() PQCAlgorithm { return "" }

// DefaultPQCProvider gibt den aktuellen (noch nicht implementierten) PQC-Provider zurück.
func DefaultPQCProvider() PQCProvider {
	return &notImplementedPQCProvider{}
}
