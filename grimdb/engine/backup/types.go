// Package backup definiert die Typen und Wire-Schemas für das Air-Gap-Backup-Modul.
//
// Das Backup-Format besteht aus zwei Zonen:
//   - Plaintext-Header (lesbar ohne Key — für Phase 1 "Peek")
//   - Verschlüsselter Payload (ChaCha20-Poly1305, entsperrbar nur mit MVK — Phase 2 "Authorize")
//
// Event-Schemas: alle Request- und Result-Structs sind JSON-serialisierbar.
// Sie werden als Payload in BACKUP.*-Events über den Kernel-Bus übertragen.
package backup

// BlobFlags ist ein Bitfeld im Blob-Header.
type BlobFlags uint8

const (
	FlagHardwareTethered BlobFlags = 1 << 0 // Bit 0: Import nur auf demselben Gerät möglich
	FlagCompressed       BlobFlags = 1 << 1 // Bit 1: Payload komprimiert (reserviert, v1 immer 0)
)

// BlobHeader ist der dekodierte Plaintext-Header einer .grimbak-Datei.
// Kann ohne Key-Material gelesen werden — ermöglicht Phase 1 "Peek".
type BlobHeader struct {
	FormatVersion    uint8
	Flags            BlobFlags
	ExportTimestamp  int64    // Unix-Sekunden
	GrimlockerVersion string
	HardwareID       [32]byte // HMAC-SHA256(vaultID||Magic||timestamp); Nullen wenn kein Tethering
	EntryCount       uint32
	HardwareTethered bool   // aus Flags dekodiert
	HeaderHMACValid  bool   // true wenn HeaderHMAC der HKDF-Prüfung standhält
}

// ExportRequest ist das Payload-Schema für BACKUP.EXPORT-Events.
type ExportRequest struct {
	DestPath       string `json:"dest_path"`
	HardwareTether bool   `json:"hardware_tether"`
}

// ExportResult ist das Payload-Schema für BACKUP.RESULT nach einem Export.
type ExportResult struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`      // Hex des Post-Write-Checksums
	EntryCount uint32 `json:"entry_count"`
	Error      string `json:"error,omitempty"`
	ErrorCode  int    `json:"error_code,omitempty"`
}

// PeekRequest ist das Payload-Schema für BACKUP.PEEK-Events.
type PeekRequest struct {
	SourcePath string `json:"source_path"`
}

// PeekResult ist das Payload-Schema für BACKUP.RESULT nach einem Peek (Phase 1).
type PeekResult struct {
	SessionID         string `json:"session_id"`
	ExportTimestamp   int64  `json:"export_timestamp"`
	GrimlockerVersion string `json:"grimlocker_version"`
	EntryCount        uint32 `json:"entry_count"`
	HardwareTethered  bool   `json:"hardware_tethered"`
	HardwareIDHex     string `json:"hardware_id_hex"` // Hex [32]byte; Nullen wenn kein Tethering
	HeaderIntegrityOK bool   `json:"header_integrity_ok"`
	Error             string `json:"error,omitempty"`
	ErrorCode         int    `json:"error_code,omitempty"`
}

// AuthorizeRequest ist das Payload-Schema für BACKUP.AUTHORIZE-Events (Phase 2).
type AuthorizeRequest struct {
	SessionID string `json:"session_id"`
	KeyHandle string `json:"key_handle"` // MVK-Handle vom Security-Modul
	Merge     bool   `json:"merge"`      // true=merge (überspringt existierende IDs); false=überschreiben
}

// AuthorizeResult ist das Payload-Schema für BACKUP.RESULT nach einer Autorisierung.
type AuthorizeResult struct {
	ImportedCount uint32 `json:"imported_count"`
	SkippedCount  uint32 `json:"skipped_count"`
	Error         string `json:"error,omitempty"`
	ErrorCode     int    `json:"error_code,omitempty"`
}

// ChecksumRequest ist das Payload-Schema für BACKUP.CHECKSUM-Events.
type ChecksumRequest struct {
	Path string `json:"path"`
}

// ChecksumResult ist das Payload-Schema für BACKUP.RESULT nach einem Checksum-Request.
type ChecksumResult struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"` // Hex
	Error     string `json:"error,omitempty"`
	ErrorCode int    `json:"error_code,omitempty"`
}

// ChecksumCompleteEvent ist das Payload für BACKUP.CHECKSUM_COMPLETE.
// Wird nach jedem erfolgreichen Export emittiert.
type ChecksumCompleteEvent struct {
	Path            string `json:"path"`
	SHA256          string `json:"sha256"`
	ExportTimestamp int64  `json:"export_timestamp"`
}
