package storage

import "time"

// CategoryFolder is the block category for encrypted folder entries in the FileVault.
const CategoryFolder Category = "FOLDER"

// FolderEntry describes a folder in the vault file system hierarchy.
type FolderEntry struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ParentID  string `json:"parent_id"` // "" = root level
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// FolderBlockID returns the block store key for this folder.
func (f *FolderEntry) FolderBlockID() string {
	return "folder-" + f.ID
}

// NewFolderEntry creates a new FolderEntry with a generated UUID.
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
