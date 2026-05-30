//go:build enterprise

// Package mtls provides mutual TLS infrastructure for the Enterprise tier.
package mtls

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"os"
	"time"
)

const certExpiryWarningDays = 30

// CertManager loads and validates TLS certificates for the enterprise daemon.
// It builds a tls.Config suitable for use as an mTLS server.
type CertManager struct {
	certPath string
	keyPath  string
	caPath   string
	spkiPin  string // optional SHA-256 hex of expected client cert SPKI
}

// NewCertManager creates a CertManager from the given file paths.
func NewCertManager(certPath, keyPath, caPath, spkiPin string) *CertManager {
	return &CertManager{
		certPath: certPath,
		keyPath:  keyPath,
		caPath:   caPath,
		spkiPin:  spkiPin,
	}
}

// TLSConfig builds a tls.Config for an mTLS server.
// Both client and server certificates are validated against their respective CAs.
func (m *CertManager) TLSConfig() (*tls.Config, error) {
	// Load server certificate and private key.
	serverCert, err := tls.LoadX509KeyPair(m.certPath, m.keyPath)
	if err != nil {
		return nil, fmt.Errorf("load server cert/key: %w", err)
	}

	// Check server cert expiry.
	if err := m.checkExpiry(serverCert); err != nil {
		log.Printf("[mtls] WARNING: %v", err)
	}

	// Build client CA pool.
	caPool, err := m.buildCAPool()
	if err != nil {
		return nil, fmt.Errorf("build CA pool: %w", err)
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert,
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS13,
	}

	// If SPKI pinning is configured, add a VerifyPeerCertificate hook.
	if m.spkiPin != "" {
		cfg.VerifyPeerCertificate = m.makeSPKIPinVerifier(m.spkiPin)
	}

	return cfg, nil
}

// ClientTLSConfig builds a tls.Config for an mTLS client (used by CLI remote.go).
func (m *CertManager) ClientTLSConfig(serverName string) (*tls.Config, error) {
	clientCert, err := tls.LoadX509KeyPair(m.certPath, m.keyPath)
	if err != nil {
		return nil, fmt.Errorf("load client cert/key: %w", err)
	}

	caPool, err := m.buildCAPool()
	if err != nil {
		return nil, fmt.Errorf("build CA pool: %w", err)
	}

	return &tls.Config{
		Certificates: []tls.Certificate{clientCert},
		RootCAs:      caPool,
		ServerName:   serverName,
		MinVersion:   tls.VersionTLS13,
	}, nil
}

func (m *CertManager) buildCAPool() (*x509.CertPool, error) {
	caPEM, err := os.ReadFile(m.caPath)
	if err != nil {
		return nil, fmt.Errorf("read CA file %s: %w", m.caPath, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(caPEM) {
		return nil, fmt.Errorf("no valid CA certificates found in %s", m.caPath)
	}
	return pool, nil
}

func (m *CertManager) checkExpiry(cert tls.Certificate) error {
	if len(cert.Certificate) == 0 {
		return nil
	}
	x509Cert, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		return fmt.Errorf("parse certificate: %w", err)
	}
	daysLeft := int(time.Until(x509Cert.NotAfter).Hours() / 24)
	if daysLeft < certExpiryWarningDays {
		return fmt.Errorf("certificate expires in %d days (%s): %s",
			daysLeft, x509Cert.NotAfter.Format(time.RFC3339), x509Cert.Subject.CommonName)
	}
	return nil
}

// makeSPKIPinVerifier returns a VerifyPeerCertificate function that checks
// whether the client certificate's SPKI SHA-256 matches the expected pin.
func (m *CertManager) makeSPKIPinVerifier(expectedPin string) func([][]byte, [][]*x509.Certificate) error {
	return func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
		if len(rawCerts) == 0 {
			return fmt.Errorf("no client certificate provided")
		}
		cert, err := x509.ParseCertificate(rawCerts[0])
		if err != nil {
			return fmt.Errorf("parse client cert: %w", err)
		}
		spki, err := spkiHash(cert)
		if err != nil {
			return fmt.Errorf("compute SPKI hash: %w", err)
		}
		if spki != expectedPin {
			return fmt.Errorf("certificate pinning verification failed: got %s want %s", spki, expectedPin)
		}
		return nil
	}
}

// spkiHash returns the SHA-256 hex hash of the certificate's SubjectPublicKeyInfo.
func spkiHash(cert *x509.Certificate) (string, error) {
	spki, err := x509.MarshalPKIXPublicKey(cert.PublicKey)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(spki)
	return fmt.Sprintf("%x", sum), nil
}

// ExtractCertPEM reads a PEM-encoded certificate file and returns the first cert.
func ExtractCertPEM(path string) (*x509.Certificate, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in %s", path)
	}
	return x509.ParseCertificate(block.Bytes)
}

// ClientIdentity extracts the common name (CN) from a TLS connection's
// peer certificate. Returns empty string if no client cert is present.
func ClientIdentity(state *tls.ConnectionState) string {
	if state == nil || len(state.PeerCertificates) == 0 {
		return ""
	}
	cert := state.PeerCertificates[0]
	if len(cert.DNSNames) > 0 {
		return cert.DNSNames[0] // Prefer SAN DNS name.
	}
	return cert.Subject.CommonName
}
