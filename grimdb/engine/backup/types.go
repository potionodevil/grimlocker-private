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
	FlagDelta            BlobFlags = 1 << 2 // Bit 2: Delta-Export (enthält nur geänderte Blöcke seit BaseExportTimestamp)
	FlagHasTTL           BlobFlags = 1 << 3 // Bit 3: ExpiresAt ist gesetzt (Backup läuft ab)
	FlagSigned           BlobFlags = 1 << 4 // Bit 4: Ed25519-Signatur am Ende des Blobs; Public Key im V2-Extension-Block
)

// BlobHeader ist der dekodierte Plaintext-Header einer .grimbak-Datei.
// Kann ohne Key-Material gelesen werden — ermöglicht Phase 1 "Peek".
type BlobHeader struct {
	FormatVersion     uint8
	Flags             BlobFlags
	ExportTimestamp   int64    // Unix-Sekunden
	GrimlockerVersion string
	HardwareID        [32]byte // HMAC-SHA256(vaultID||Magic||timestamp); Nullen wenn kein Tethering
	EntryCount        uint32
	HardwareTethered  bool // aus Flags dekodiert
	HeaderHMACValid   bool // true wenn HeaderHMAC der HKDF-Prüfung standhält
	// v2 fields — present in FormatVersion >= 2
	BackupSequence      uint32   // monoton steigend; 0 = ungesetzt (v1 Backups)
	ExpiresAt           int64    // Unix-Sekunden; 0 = kein Ablaufdatum
	IsDelta             bool     // true = nur geänderte Blöcke seit BaseExportTimestamp
	BaseExportTimestamp int64    // gilt nur wenn IsDelta=true
	// signature fields — present when FlagSigned is set (requires FormatVersion >= 2)
	SignaturePublicKey [32]byte // Ed25519 Public Key (32 Bytes); Nullen wenn kein FlagSigned
	SignatureValid     bool     // true = Signatur am Ende des Blobs ist korrekt (wird bei Peek geprüft)
}

// ExportRequest ist das Payload-Schema für BACKUP.EXPORT-Events.
type ExportRequest struct {
	DestPath            string `json:"dest_path"`
	HardwareTether      bool   `json:"hardware_tether"`
	TTLDays             int    `json:"ttl_days,omitempty"`              // 0 = kein Ablaufdatum
	Delta               bool   `json:"delta,omitempty"`                 // nur geänderte Blöcke exportieren
	BaseExportTimestamp int64  `json:"base_export_timestamp,omitempty"` // Basis-Timestamp für Delta-Export
	Sign                bool   `json:"sign,omitempty"`                  // Ed25519-Signatur anhängen
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
	SessionID            string `json:"session_id"`
	ExportTimestamp      int64  `json:"export_timestamp"`
	GrimlockerVersion    string `json:"grimlocker_version"`
	EntryCount           uint32 `json:"entry_count"`
	HardwareTethered     bool   `json:"hardware_tethered"`
	HardwareIDHex        string `json:"hardware_id_hex"`
	HeaderIntegrityOK    bool   `json:"header_integrity_ok"`
	// v2 fields
	BackupSequence      uint32 `json:"backup_sequence"`
	ExpiresAt           int64  `json:"expires_at,omitempty"`
	IsDelta             bool   `json:"is_delta"`
	BaseExportTimestamp int64  `json:"base_export_timestamp,omitempty"`
	// signature fields
	SignaturePubKeyHex string `json:"signature_pub_key_hex,omitempty"` // hex-kodierter Ed25519 Public Key
	SignatureValid     bool   `json:"signature_valid"`                 // true = Signatur am Ende des Blobs ist korrekt
	Error              string `json:"error,omitempty"`
	ErrorCode          int    `json:"error_code,omitempty"`
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
