package storage

import "time"

// CategoryFolder ist die Block-Kategorie für encrypted Folder-Einträge im FileVault.
const CategoryFolder Category = "FOLDER"

// FolderEntry beschreibt einen Ordner in der Vault-Dateisystem-Hierarchie.
type FolderEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ParentID  string `json:"parent_id"` // "" = root level
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// FolderBlockID gibt den Block-Store-Key für diesen Folder zurück.
func (f *FolderEntry) FolderBlockID() string {
	return "folder-" + f.ID
}

// NewFolderEntry erzeugt einen neuen FolderEntry mit generierter UUID.
func NewFolderEntry(name, parentID string) FolderEntry {
	now := time.Now().UnixNano()
	return FolderEntry{
		ID:        generateUUID(),
		Name:      name,
		ParentID:  parentID,
		CreatedAt: now,
		UpdatedAt: now,
	}
}
