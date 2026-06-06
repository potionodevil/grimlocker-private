package storage

import "time"

// Category klassifiziert einen VaultEntry für filterbare Sidebar-Ansichten.
// Der String-Wert ist der kanonische Wire-Identifier in JSON und IPC.
type Category string

const (
	CategoryPassword    Category = "PASSWORD"
	CategorySSHKey      Category = "SSH_KEY"
	CategoryCertificate Category = "CERTIFICATE"
	CategoryFileVault   Category = "FILE_VAULT"
)

// CategoryFromType mapped den legacy UI-"type"-String auf die formale Category-Enum.
// Wird bei ENTRY.CREATE verwendet, um das Category-Feld aus älteren Requests zu befüllen.
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

// VaultEntry ist das formale Schema für alle Vault-Einträge.
// Pflichtfelder: ID, Title, Category.
// Fields hält die Entry-Typ-spezifischen Key/Value-Paare (z.B. "username", "password").
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

// NewVaultEntry erzeugt einen VaultEntry mit vorausgefüllten Pflichtfeldern.
// now wird von Callern injiziert (typischerweise time.Now().UnixNano()), damit Tests
// einen deterministischen Timestamp mitgeben können.
func NewVaultEntry(id, title string, category Category, now int64) VaultEntry {
	if now == 0 {
		now = time.Now().UnixNano()
	}
	return VaultEntry{
		ID:        id,
		Title:     title,
		Category:  category,
		Type:      string(category),
		CreatedAt: now,
		UpdatedAt: now,
	}
}
