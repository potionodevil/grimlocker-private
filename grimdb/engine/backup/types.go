// Package backup defines types and wire schemas for the air-gap backup module.
//
// The backup format has two zones:
//   - Plaintext header (readable without key material — Phase 1 "Peek")
//   - Encrypted payload (ChaCha20-Poly1305, requires MVK — Phase 2 "Authorize")
package backup

// BlobFlags is the flags bitfield in the blob header.
type BlobFlags uint8

const (
	FlagHardwareTethered BlobFlags = 1 << 0 // bit 0: import only works on originating device
	FlagCompressed       BlobFlags = 1 << 1 // bit 1: payload compressed (reserved, always 0 in v1)
)

// BlobHeader is the decoded plaintext header of a .grimbak file.
// Readable without key material — enables Phase 1 "Peek".
type BlobHeader struct {
	FormatVersion     uint8
	Flags             BlobFlags
	ExportTimestamp   int64 // Unix seconds
	GrimlockerVersion string
	HardwareID        [32]byte // HMAC-SHA256(vaultID||Magic||timestamp); zeros if not tethered
	EntryCount        uint32
	HardwareTethered  bool // decoded from Flags
	HeaderHMACValid   bool // true if HeaderHMAC passes integrity check
}

// ExportRequest is the payload schema for BACKUP.EXPORT events.
type ExportRequest struct {
	DestPath       string `json:"dest_path"`
	HardwareTether bool   `json:"hardware_tether"`
}

// ExportResult is the payload schema for BACKUP.RESULT after an export.
type ExportResult struct {
	Path       string `json:"path"`
	SHA256     string `json:"sha256"`
	EntryCount uint32 `json:"entry_count"`
	Error      string `json:"error,omitempty"`
	ErrorCode  int    `json:"error_code,omitempty"`
}

// PeekRequest is the payload schema for BACKUP.PEEK events.
type PeekRequest struct {
	SourcePath string `json:"source_path"`
}

// PeekResult is the payload schema for BACKUP.RESULT after a peek (Phase 1).
type PeekResult struct {
	SessionID         string `json:"session_id"`
	ExportTimestamp   int64  `json:"export_timestamp"`
	GrimlockerVersion string `json:"grimlocker_version"`
	EntryCount        uint32 `json:"entry_count"`
	HardwareTethered  bool   `json:"hardware_tethered"`
	HardwareIDHex     string `json:"hardware_id_hex"`
	HeaderIntegrityOK bool   `json:"header_integrity_ok"`
	Error             string `json:"error,omitempty"`
	ErrorCode         int    `json:"error_code,omitempty"`
}

// AuthorizeRequest is the payload schema for BACKUP.AUTHORIZE events (Phase 2).
type AuthorizeRequest struct {
	SessionID string `json:"session_id"`
	KeyHandle string `json:"key_handle"`
	Merge     bool   `json:"merge"` // true=skip existing IDs; false=overwrite
}

// AuthorizeResult is the payload schema for BACKUP.RESULT after authorization.
type AuthorizeResult struct {
	ImportedCount uint32 `json:"imported_count"`
	SkippedCount  uint32 `json:"skipped_count"`
	Error         string `json:"error,omitempty"`
	ErrorCode     int    `json:"error_code,omitempty"`
}

// ChecksumRequest is the payload schema for BACKUP.CHECKSUM events.
type ChecksumRequest struct {
	Path string `json:"path"`
}

// ChecksumResult is the payload schema for BACKUP.RESULT after a checksum request.
type ChecksumResult struct {
	Path      string `json:"path"`
	SHA256    string `json:"sha256"`
	Error     string `json:"error,omitempty"`
	ErrorCode int    `json:"error_code,omitempty"`
}

// ChecksumCompleteEvent is the payload for BACKUP.CHECKSUM_COMPLETE.
// Emitted after every successful export.
type ChecksumCompleteEvent struct {
	Path            string `json:"path"`
	SHA256          string `json:"sha256"`
	ExportTimestamp int64  `json:"export_timestamp"`
}
