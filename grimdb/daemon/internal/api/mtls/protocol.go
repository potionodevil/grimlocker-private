//go:build enterprise

// Package mtls extends the Grimlocker API protocol for Enterprise mTLS connections.
// It adds enterprise-specific message types for audit log access, vault rotation,
// and identity-aware session management.
package mtls

import (
	"encoding/json"
	"net/http"
	"time"
)

// ── Enterprise-specific message types ────────────────────────────────────────

// EnterpriseStatus is the JSON body returned by GET /health in enterprise mode.
type EnterpriseStatus struct {
	Status        string    `json:"status"`
	Tier          string    `json:"tier"`
	Version       string    `json:"version"`
	VaultBackend  string    `json:"vault_backend"`
	VaultUnlocked bool      `json:"vault_unlocked"`
	OIDCProvider  string    `json:"oidc_provider"`
	ClientCN      string    `json:"client_cn,omitempty"` // from mTLS cert
	UptimeSeconds int64     `json:"uptime_seconds"`
	PID           int       `json:"pid"`
	Timestamp     time.Time `json:"timestamp"`
}

// AuditRequest is the JSON body for GET /api/v1/audit.
type AuditRequest struct {
	Count int `json:"count"` // number of recent entries to return (default 20)
}

// AuditEntry is one record from the cryptographically-chained audit log.
type AuditEntry struct {
	Timestamp int64  `json:"timestamp"`
	Level     string `json:"level"`
	Module    string `json:"module"`
	Message   string `json:"message"`
	SubjectID string `json:"subject_id,omitempty"`
	PrevHash  string `json:"prev_hash,omitempty"` // hex-encoded SHA-256
	Hash      string `json:"hash,omitempty"`      // hex-encoded SHA-256
}

// RotateKeyRequest is the JSON body for POST /api/v1/rotate-key.
// Authenticated via mTLS — only vault-admin CN may trigger rotation.
type RotateKeyRequest struct {
	NewOIDCToken string `json:"new_oidc_token"` // JWT for new MVK derivation
}

// ── HTTP handler helpers ──────────────────────────────────────────────────────

// WriteJSON writes v as JSON with the given status code.
func WriteJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// WriteError writes a JSON error body with the given status code.
func WriteError(w http.ResponseWriter, status int, msg string) {
	WriteJSON(w, status, map[string]string{"error": msg})
}
