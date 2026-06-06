// Package provider definiert die tier-agnostische Abstraktionsschicht für Grimlocker.
//
// Der Kernel (cmd/daemon/main.go) hängt NUR von den Interfaces in diesem Package ab.
// Konkrete Implementierungen leben in config/single (LocalAuth, LocalStorage) und
// config/enterprise (IAMProvider, RemoteVault). Das Event-Bus-Design wird NICHT
// geändert — nur die Handler-Implementierungen sind hier gekapselt.
package provider

import (
	"github.com/grimlocker/grimdb/engine/crypto"
	"github.com/grimlocker/grimdb/engine/kernel"
	"github.com/grimlocker/grimdb/engine/security"
	"github.com/grimlocker/grimdb/engine/storage"
)

// AuthProvider kapselt die gesamte Auth-Logik für einen Tier.
// Der Kernel ruft HandleUnlockEvent auf und subscribed es auf kernel.EvAuthUnlock.
// Konkret: config/single.LocalAuth (Argon2id) oder config/enterprise.OIDCProvider (JWT/OIDC).
type AuthProvider interface {
	// HandleUnlockEvent gibt einen kernel.Handler zurück, der den gesamten
	// Unlock-Flow implementiert (Steps 0–7 aus makeAuthUnlockHandler):
	//   0. Lockdown-Check
	//   1. MVK ableiten & verifizieren
	//   2. Key in locked Memory speichern
	//   3. MVK in den BlockStore einhängen
	//   4. Block-Index laden
	//   5. AUTH.KEY_READY dispatched, um STORAGE-Gate zu öffnen
	//   6. Session als unlocked markieren
	//   7. Session-Key generieren, Success aufzeichnen, AUTH.RESULT emitten
	HandleUnlockEvent(
		bus kernel.Dispatcher,
		sessionCtx *security.SessionContext,
		onSessionKey func(key []byte, handle string),
	) kernel.Handler

	// Key-Material-Zugriff — delegiert an security.Module intern.
	StoreMVK(key []byte) (string, error)
	RetrieveMVK(handle string) ([]byte, bool)
	RevokeMVK(handle string)

	Lockdown() security.LockdownManager
	AuditLog() security.AuditLog

	// Tier gibt den Auth-Mechanismus zurück ("local-argon2id" oder "oidc-jwt").
	Tier() string
}

// IdentityProvider ist ein optionaler Extension-Point für federated Identity.
// Konkret: zukünftige SAML 2.0-, LDAP/AD- oder Multi-Tenant-IAM-Implementierungen.
// Für Phase 1 nicht benötigt (OIDC wird direkt von OIDCProvider gehandled).
type IdentityProvider interface {
	Protocol() string

	// Validate prüft ein Credential (Token, Assertion oder Bind-Result) und
	// gibt die canonical Subject-ID oder einen Error zurück.
	Validate(credential []byte) (subjectID string, err error)
}

// UserAuthenticator managed User-Identitäten und Credentials für RBAC.
// Single-Mode: lokaler DB-Check gegen gespeicherte Argon2id-Hashes.
// Enterprise-Mode: Delegation an OIDC/LDAP-Provider.
type UserAuthenticator interface {
	// Authenticate prüft ein User-Credential und gibt die Subject-ID zurück.
	// Fehler bei ungültigem/abgelaufenem Credential oder nicht-existierendem User.
	Authenticate(credential []byte) (subjectID string, err error)

	// CreateIdentity legt einen neuen User im Identity-Store an.
	// Single-Mode: speichert Argon2id-Hash lokal.
	// Enterprise-Mode: No-Op (wird von externem IAM gemanaged).
	CreateIdentity(subjectID string, credential []byte) error

	RevokeIdentity(subjectID string) error

	ListIdentities() ([]string, error)
}

// AuditLogger zeichnet sicherheitsrelevante Operationen persistent auf.
// Konkret: security/audit.go (In-Memory-Ringbuffer + optionaler File-Sink).
type AuditLogger interface {
	Log(level, module, message string, details map[string]string)

	Query(level string, module string, limit int) []AuditEntry

	Flush() error
}

// AuditEntry ist ein einzelner Eintrag im Audit-Log.
type AuditEntry struct {
	Timestamp int64             `json:"timestamp"`
	Level     string            `json:"level"`
	Module    string            `json:"module"`
	Message   string            `json:"message"`
	Details   map[string]string `json:"details,omitempty"`
}

// StorageProvider kapselt ein Storage-Backend für einen Tier.
// Embeded storage.BlockStore, damit existierende Codepfade unverändert funktionieren.
// Konkret: config/single.LocalStorage (file-backed) oder config/enterprise.RemoteVault (S3/MinIO).
type StorageProvider interface {
	storage.BlockStore

	SetMVKFunc(fn func() []byte)

	LoadIndex() error

	// KernelModule gibt die kernel.Module-Implementierung (der Storage-Adapter) zurück,
	// damit der Daemon sie via reg.Add() auf dem Event-Bus registrieren kann.
	KernelModule() kernel.Module
}

// VaultProvider ist der Single-Entry-Point, den der Kernel beim Startup erhält.
// Er trägt alle Provider für einen bestimmten Tier.
// main.go darf config/single oder config/enterprise nicht direkt importieren —
// nur dieses Interface.
type VaultProvider interface {
	Auth() AuthProvider
	Storage() StorageProvider
	Crypto() crypto.Provider

	// Tier gibt einen lesbaren Tier-Identifier zurück ("single" oder "enterprise").
	Tier() string

	// KernelModules gibt alle kernel.Module-Instanzen zurück, die auf dem Event-Bus
	// registriert werden müssen (security, crypto, storage adapter — in Reihenfolge).
	KernelModules() []kernel.Module
}
