package storage

import "time"

// Category classifies a VaultEntry for filterable sidebar views.
// The string value is the canonical wire identifier used in JSON and IPC.
type Category string

const (
	CategoryPassword    Category = "PASSWORD"
	CategorySSHKey      Category = "SSH_KEY"
	CategoryCertificate Category = "CERTIFICATE"
	CategoryFileVault   Category = "FILE_VAULT"
)

// CategoryFromType maps the legacy UI "type" string to the formal Category enum.
// Used during ENTRY.CREATE to back-fill the Category field from older-style requests.
func CategoryFromType(t string) Category {
	switch t {
	case "password":
		return CategoryPassword
	case "ssh":
		return CategorySSHKey
	case "certs", "certificate":
		return CategoryCertificate
	case "file_vault":
		return CategoryFileVault
	default:
		return CategoryPassword // safe fallback
	}
}

// VaultEntry is the formal schema for all vault entries.
// Mandatory fields: ID, Title, Category.
// Fields holds the entry-type-specific key/value pairs (e.g. "username", "password").
type VaultEntry struct {
	ID        string            `json:"id"`
	Title     string            `json:"title"`
	Category  Category          `json:"category"`             // mandatory — filterable
	Type      string            `json:"type,omitempty"`       // legacy compat ("password", "ssh", …)
	Fields    map[string]string `json:"fields,omitempty"`
	SubjectID string            `json:"subject_id,omitempty"` // workspace / user scope
	CreatedAt int64             `json:"created_at"`
	UpdatedAt int64             `json:"updated_at"`
}

// NewVaultEntry creates a VaultEntry with mandatory fields pre-filled.
// now is injected by callers (typically time.Now().UnixNano()) so tests
// can pass a deterministic timestamp.
func NewVaultEntry(id, title string, category Category, now int64) VaultEntry {
	if now == 0 {
		now = time.Now().UnixNano()
	}
	return VaultEntry{
		ID:        id,
		Title:     title,
		Category:  category,
		Type:      string(category), // keep legacy compat field in sync
		CreatedAt: now,
		UpdatedAt: now,
	}
}
