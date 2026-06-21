package errors

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ─── Error-Code-Bereiche ──────────────────────────────────────────────────────
//
//	1000-1999  Vault / Auth
//	2000-2999  Storage / GrimDB
//	3000-3999  Crypto / Key-Material
//	4000-4999  Security / Lockdown / Memory
//	5000-5999  Kernel / Bus / Event
//	6000-6999  API / Protocol / Transport
//	7000-7099  Backup / Air-Gap Export-Import

// ─── Auth Errors (1000-1999) ──────────────────────────────────────────────────

const (
	ErrCodeVaultLocked         = 1001 // Vault muss erst unlocked werden
	ErrCodeVaultNotInitialized = 1002 // Vault wurde noch nicht initialisiert
	ErrCodeAuthInvalid         = 1003 // Passwort oder Token ist falsch
	ErrCodeAuthTimeout         = 1004 // Auth-Operation timed out
	ErrCodeAuthLockdown        = 1005 // Zu viele Fehlversuche; Vault ist ausgesperrt
	ErrCodeAuthSetupFailed     = 1006 // Vault-Initialisierung fehlgeschlagen
	ErrCodeAuthTokenExpired    = 1007 // OIDC/JWT-Token abgelaufen (Enterprise)
)

// ─── Storage Errors (2000-2999) ───────────────────────────────────────────────

const (
	ErrCodeStorageIO           = 2001 // Disk-Read/Write-Fehler
	ErrCodeStorageCorruption   = 2002 // HMAC-Mismatch oder JSON kaputt — Daten evtl. manipuliert
	ErrCodeStorageNotFound     = 2003 // Block-ID existiert nicht im Index
	ErrCodeStorageQuota        = 2004 // Storage-Quota überschritten
	ErrCodeStorageIndexFailed  = 2005 // Index-Persist oder -Load fehlgeschlagen
	ErrCodeStorageNonceFailed  = 2006 // Nonce-Generierung fehlgeschlagen (CSPRNG)
)

// ─── Crypto Errors (3000-3999) ────────────────────────────────────────────────

const (
	ErrCodeCryptoKeyDerivation = 3001 // Argon2id / HKDF-Derivation fehlgeschlagen
	ErrCodeCryptoEncryption    = 3002 // ChaCha20-Poly1305 Seal fehlgeschlagen
	ErrCodeCryptoDecryption    = 3003 // ChaCha20-Poly1305 Open fehlgeschlagen (falscher Key oder manipuliert)
	ErrCodeCryptoInvalidKey    = 3004 // Key-Material nil oder falsche Länge (muss 32 Bytes sein)
	ErrCodeCryptoEntropyFailed = 3005 // CSPRNG / Entropy-Quelle fehlgeschlagen
	ErrCodeCryptoHandleUnknown = 3006 // Key-Handle nicht im Security-Modul gefunden
)

// ─── Security Errors (4000-4999) ──────────────────────────────────────────────

const (
	ErrCodeSecurityMemlock      = 4001 // mlock / VirtualLock fehlgeschlagen — Key im Memory nicht geschützt
	ErrCodeSecurityLockdown     = 4002 // Hard-Lockdown ausgelöst; Key-Material gezeroized
	ErrCodeSecurityIntegrity    = 4003 // Binary-Integrity-Check fehlgeschlagen
	ErrCodeSecurityUnauthorized = 4004 // Operation von Security-Policy verboten
	ErrCodeSecurityMVKMissing   = 4005 // MVK-Handle fehlt oder wurde revoked
)

// ─── Kernel / Bus Errors (5000-5999) ─────────────────────────────────────────

const (
	ErrCodeBusShutdown        = 5001 // Bus fährt runter; Dispatch nicht möglich
	ErrCodeBusTimeout         = 5002 // Request timed out auf Reply wartend
	ErrCodeBusGated           = 5003 // Event gedroppt: Channel ist gated (Vault locked)
	ErrCodeBusTTL             = 5004 // Event gedroppt: TTL erschöpft
	ErrCodeBusModuleDuplicate = 5005 // Module mit dieser ID bereits registriert
	ErrCodeBusNilHandler      = 5006 // Subscribe mit nil-Handler aufgerufen
)

// ─── API / Protocol Errors (6000-6999) ───────────────────────────────────────

const (
	ErrCodeProtocolInvalid   = 6001 // Binary-Frame kaputt oder unbekannter Message-Type
	ErrCodeProtocolTimeout   = 6002 // Client-Request timed out
	ErrCodeProtocolUnhandled = 6003 // Kein Handler für die Action registriert
	ErrCodeProtocolAuth      = 6004 // WebSocket / IPC-Authentifizierung fehlgeschlagen
)

// ─── Backup Errors (7000-7099) ────────────────────────────────────────────────

const (
	ErrCodeBackupInvalidMagic    = 7001 // Datei beginnt nicht mit GRIMBAK-Magic
	ErrCodeBackupVersionMismatch = 7002 // FormatVersion nicht unterstützt
	ErrCodeBackupTetherMismatch  = 7003 // HardwareID passt nicht zur aktuellen Vault
	ErrCodeBackupSessionNotFound = 7004 // session_id unbekannt oder abgelaufen
	ErrCodeBackupSessionExpired  = 7005 // Session-TTL überschritten
	ErrCodeBackupDecryptFailed   = 7006 // Payload-Entschlüsselung fehlgeschlagen
	ErrCodeBackupChecksumFailed  = 7007 // Post-Write-SHA256-Verifikation fehlgeschlagen
	ErrCodeBackupHeaderTampered  = 7008 // HeaderHMAC-Mismatch — Header manipuliert
)

// ─── Core Error Type ──────────────────────────────────────────────────────────

// ErrorContext trägt strukturierte Diagnose-Daten, die an jeden GrimlockError angehängt werden.
// Felder sind optional — nur füllen, was für die Error-Stelle relevant ist.
type ErrorContext struct {
	BlockID   string            `json:"block_id,omitempty"`
	Operation string            `json:"operation,omitempty"`
	Details   map[string]string `json:"details,omitempty"`
}

// GrimlockError ist der einheitliche Error-Type für alle Grimlocker-Module.
// Wrappt eine Cause, hat einen numerischen Error-Code und optional einen Stacktrace.
type GrimlockError struct {
	Code      int          `json:"code"`
	Message   string       `json:"message"`
	Ctx       ErrorContext `json:"context,omitempty"`
	Stack     []StackFrame `json:"stacktrace,omitempty"`
	Cause     error        `json:"-"`
	Timestamp int64        `json:"timestamp"`
	ModuleID  string       `json:"module_id,omitempty"`
	EventType string       `json:"event_type,omitempty"`
}

func (e *GrimlockError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Message)
}

// Unwrap implementiert errors.Unwrap — erlaubt errors.Is / errors.As die Chain zu traversieren.
func (e *GrimlockError) Unwrap() error { return e.Cause }

// Is gibt true zurück, wenn target ein *GrimlockError mit demselben Code ist.
func (e *GrimlockError) Is(target error) bool {
	t, ok := target.(*GrimlockError)
	if !ok {
		return false
	}
	return e.Code == t.Code
}

// HTTPStatus mapped einen Error-Code auf einen passenden HTTP-Status-Code.
func (e *GrimlockError) HTTPStatus() int {
	switch {
	case e.Code == ErrCodeVaultLocked:
		return http.StatusLocked // 423
	case e.Code == ErrCodeVaultNotInitialized:
		return http.StatusNotFound // 404
	case e.Code == ErrCodeAuthInvalid, e.Code == ErrCodeAuthTokenExpired:
		return http.StatusUnauthorized // 401
	case e.Code == ErrCodeAuthLockdown:
		return http.StatusTooManyRequests // 429
	case e.Code == ErrCodeStorageNotFound:
		return http.StatusNotFound // 404
	case e.Code == ErrCodeStorageCorruption, e.Code == ErrCodeCryptoDecryption:
		return http.StatusUnprocessableEntity // 422
	case e.Code == ErrCodeSecurityUnauthorized, e.Code == ErrCodeSecurityLockdown:
		return http.StatusForbidden // 403
	case e.Code == ErrCodeProtocolInvalid:
		return http.StatusBadRequest // 400
	case e.Code == ErrCodeBusTimeout, e.Code == ErrCodeProtocolTimeout:
		return http.StatusGatewayTimeout // 504
	default:
		return http.StatusInternalServerError // 500
	}
}

// MarshalJSON produziert eine JSON-Repräsentation, die sicher zum Client gesendet werden kann.
// Die Cause-Chain wird nicht serialisiert — nur Message + Code + Context.
func (e *GrimlockError) MarshalJSON() ([]byte, error) {
	type wire struct {
		Code      int          `json:"code"`
		Message   string       `json:"message"`
		Ctx       ErrorContext `json:"context,omitempty"`
		Stack     []StackFrame `json:"stacktrace,omitempty"`
		Timestamp int64        `json:"timestamp"`
		ModuleID  string       `json:"module_id,omitempty"`
		EventType string       `json:"event_type,omitempty"`
	}
	return json.Marshal(wire{
		Code:      e.Code,
		Message:   e.Message,
		Ctx:       e.Ctx,
		Stack:     e.Stack,
		Timestamp: e.Timestamp,
		ModuleID:  e.ModuleID,
		EventType: e.EventType,
	})
}

// ─── Constructor-Helper ────────────────────────────────────────────────────────

func newError(code int, msg string, cause error, ctx ErrorContext, captureStack bool) *GrimlockError {
	var stack []StackFrame
	if captureStack {
		stack = CaptureStacktrace(2)
	}
	return &GrimlockError{
		Code:      code,
		Message:   msg,
		Cause:     cause,
		Ctx:       ctx,
		Stack:     stack,
		Timestamp: time.Now().UnixNano(),
	}
}

// ─── Auth-Constructors ─────────────────────────────────────────────────────────

// NewVaultLockedError sagt: "Vault ist locked — bitte erst entsperren."
func NewVaultLockedError() *GrimlockError {
	return newError(ErrCodeVaultLocked, "vault is locked", nil,
		ErrorContext{Operation: "vault_access"}, false)
}

// NewAuthInvalidError sagt: "Passwort oder Token ist falsch."
func NewAuthInvalidError(operation string, cause error) *GrimlockError {
	return newError(ErrCodeAuthInvalid, "authentication failed", cause,
		ErrorContext{Operation: operation}, true)
}

// NewAuthLockdownError sagt: "Zu viele Fehlversuche — Vault ist ausgesperrt."
func NewAuthLockdownError(attemptsRemaining int) *GrimlockError {
	return newError(ErrCodeAuthLockdown, "vault locked out after too many failed attempts", nil,
		ErrorContext{
			Operation: "auth_lockdown",
			Details:   map[string]string{"remaining_attempts": fmt.Sprintf("%d", attemptsRemaining)},
		}, false)
}

// NewVaultNotInitializedError sagt: "Vault noch nicht initialisiert — erst setup laufen lassen."
func NewVaultNotInitializedError() *GrimlockError {
	return newError(ErrCodeVaultNotInitialized, "vault not initialized — run setup first", nil,
		ErrorContext{Operation: "vault_init_check"}, false)
}

// ─── Storage-Constructors ─────────────────────────────────────────────────────

// NewStorageIOError wrappt einen Low-Level-I/O-Fehler mit Block- und Operations-Kontext.
func NewStorageIOError(operation, blockID string, cause error) *GrimlockError {
	return newError(ErrCodeStorageIO, "storage I/O failure", cause,
		ErrorContext{Operation: operation, BlockID: blockID}, true)
}

// NewStorageCorruptionError sagt: "Daten auf der Platte sind korrupt — HMAC-Check oder JSON-Parse fehlgeschlagen."
func NewStorageCorruptionError(operation, blockID string, details map[string]string) *GrimlockError {
	return newError(ErrCodeStorageCorruption, "storage data corruption detected", nil,
		ErrorContext{Operation: operation, BlockID: blockID, Details: details}, true)
}

// NewStorageNotFoundError sagt: "Block-ID existiert nicht im Index."
func NewStorageNotFoundError(blockID string) *GrimlockError {
	return newError(ErrCodeStorageNotFound, fmt.Sprintf("block not found: %s", blockID), nil,
		ErrorContext{Operation: "block_lookup", BlockID: blockID}, false)
}

// NewStorageIndexError wrappt einen Fehler beim Index-Persist oder -Load.
func NewStorageIndexError(operation string, cause error) *GrimlockError {
	return newError(ErrCodeStorageIndexFailed, "vault index operation failed", cause,
		ErrorContext{Operation: operation}, true)
}

// ─── Crypto-Constructors ───────────────────────────────────────────────────────

// NewCryptoDecryptionError sagt: "ChaCha20-Poly1305 Open fehlgeschlagen — entweder falscher Key oder manipulierte Daten."
func NewCryptoDecryptionError(blockID string, cause error) *GrimlockError {
	return newError(ErrCodeCryptoDecryption, "decryption failed — wrong key or data tampered", cause,
		ErrorContext{Operation: "chacha20poly1305_open", BlockID: blockID}, true)
}

// NewCryptoEncryptionError sagt: "Seal fehlgeschlagen."
func NewCryptoEncryptionError(operation string, cause error) *GrimlockError {
	return newError(ErrCodeCryptoEncryption, "encryption failed", cause,
		ErrorContext{Operation: operation}, true)
}

// NewCryptoKeyDerivationError sagt: "Argon2id oder HKDF fehlgeschlagen."
func NewCryptoKeyDerivationError(operation string, cause error) *GrimlockError {
	return newError(ErrCodeCryptoKeyDerivation, "key derivation failed", cause,
		ErrorContext{Operation: operation}, true)
}

// NewCryptoInvalidKeyError sagt: "Key-Material ist nil oder falsche Länge (brauchen 32 Bytes)."
func NewCryptoInvalidKeyError(got int) *GrimlockError {
	return newError(ErrCodeCryptoInvalidKey,
		fmt.Sprintf("invalid key length: got %d bytes, need 32", got), nil,
		ErrorContext{Operation: "key_validation",
			Details: map[string]string{"got_bytes": fmt.Sprintf("%d", got)}}, true)
}

// NewCryptoHandleUnknownError sagt: "Key-Handle wurde nicht im Security-Modul gefunden."
func NewCryptoHandleUnknownError(handle string) *GrimlockError {
	return newError(ErrCodeCryptoHandleUnknown, "key handle not found", nil,
		ErrorContext{Operation: "key_resolve",
			Details: map[string]string{"handle_prefix": safePrefix(handle, 8)}}, false)
}

// ─── Security-Constructors ─────────────────────────────────────────────────────

// NewSecurityMemlockError sagt: "mlock oder VirtualLock fehlgeschlagen — Key-Material könnte ausgelagert werden."
func NewSecurityMemlockError(cause error) *GrimlockError {
	return newError(ErrCodeSecurityMemlock, "cannot lock memory — key material may be swappable", cause,
		ErrorContext{Operation: "memlock_alloc"}, true)
}

// NewSecurityLockdownError sagt: "Hard-Lockdown ausgelöst — alles Key-Material wurde gezeroized."
func NewSecurityLockdownError(reason string, details map[string]string) *GrimlockError {
	return newError(ErrCodeSecurityLockdown, "hard lockdown triggered — all key material zeroed", nil,
		ErrorContext{Operation: "hard_lockdown", Details: details}, true)
}

// NewSecurityMVKMissingError sagt: "MVK-Handle nicht gefunden — ist der Vault unlocked?"
func NewSecurityMVKMissingError(operation string) *GrimlockError {
	return newError(ErrCodeSecurityMVKMissing, "master vault key not available — vault locked?", nil,
		ErrorContext{Operation: operation}, false)
}

// ─── Kernel / Bus-Constructors ─────────────────────────────────────────────────

// NewBusTimeoutError sagt: "Request hat kein Reply bekommen — Timeout."
func NewBusTimeoutError(eventType string) *GrimlockError {
	return newError(ErrCodeBusTimeout, "event request timed out", nil,
		ErrorContext{Operation: "bus_request",
			Details: map[string]string{"event_type": eventType}}, false)
}

// NewBusShutdownError sagt: "Bus fährt runter — Dispatch nicht möglich."
func NewBusShutdownError() *GrimlockError {
	return newError(ErrCodeBusShutdown, "bus is shutting down", nil,
		ErrorContext{Operation: "bus_dispatch"}, false)
}

// NewBusGatedError sagt: "Event gedroppt, weil das STORAGE-Gate geschlossen ist (Vault locked)."
func NewBusGatedError(eventType, channel string) *GrimlockError {
	return newError(ErrCodeBusGated, "event dropped: vault not unlocked", nil,
		ErrorContext{Operation: "gate_check",
			Details: map[string]string{
				"event_type": eventType,
				"channel":    channel,
			}}, false)
}

// ─── API / Protocol-Constructors ─────────────────────────────────────────────

// NewProtocolError sagt: "Binary-Frame oder JSON-Payload ist kaputt."
func NewProtocolError(operation string, cause error) *GrimlockError {
	return newError(ErrCodeProtocolInvalid, "protocol error — invalid message format", cause,
		ErrorContext{Operation: operation}, true)
}

// ─── Utility ──────────────────────────────────────────────────────────────────

// safePrefix gibt die ersten n Zeichen von s zurück, oder s wenn kürzer.
// Verhindert das Leaken voller Handles in Error-Messages.
func safePrefix(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// Wrap wandelt einen plain error in einen GrimlockError mit dem gegebenen Code um.
// Wenn err bereits ein *GrimlockError ist, wird er unverändert zurückgegeben.
// Nützlich zum Wrappen von Drittanbieter-Fehlern an Module-Grenzen.
func Wrap(code int, msg string, err error) *GrimlockError {
	if err == nil {
		return nil
	}
	if ge, ok := err.(*GrimlockError); ok {
		return ge
	}
	return newError(code, msg, err, ErrorContext{}, true)
}

// Sentinel gibt einen GrimlockError zurück, der für errors.Is-Vergleiche verwendet werden kann.
// Sentinels erfassen niemals einen Stacktrace (sind Wert-Types, keine Instanzen).
func Sentinel(code int, msg string) *GrimlockError {
	return &GrimlockError{Code: code, Message: msg}
}

// WithDetails fügt ein Key-Value-Paar zu den Context-Details des Errors hinzu.
// Gibt denselben *GrimlockError für Chaining zurück.
func (e *GrimlockError) WithDetails(key string, value interface{}) *GrimlockError {
	if e.Ctx.Details == nil {
		e.Ctx.Details = make(map[string]string)
	}
	e.Ctx.Details[key] = fmt.Sprintf("%v", value)
	return e
}

// WithModule setzt das ModuleID-Feld und gibt denselben Error für Chaining zurück.
func (e *GrimlockError) WithModule(moduleID string) *GrimlockError {
	e.ModuleID = moduleID
	return e
}

// WithEvent setzt das EventType-Feld und gibt denselben Error für Chaining zurück.
func (e *GrimlockError) WithEvent(eventType string) *GrimlockError {
	e.EventType = eventType
	return e
}

// ─── Backup-Constructors ───────────────────────────────────────────────────────

// NewBackupInvalidMagicError sagt: "Datei startet nicht mit GRIMBAK-Magic — kein gültiges Backup."
func NewBackupInvalidMagicError(path string) *GrimlockError {
	return newError(ErrCodeBackupInvalidMagic, "not a valid GRIMBAK backup file", nil,
		ErrorContext{Operation: "backup_peek", Details: map[string]string{"path": path}}, false)
}

// NewBackupVersionMismatchError sagt: "FormatVersion wird nicht unterstützt."
func NewBackupVersionMismatchError(got uint8) *GrimlockError {
	return newError(ErrCodeBackupVersionMismatch, "unsupported backup format version", nil,
		ErrorContext{Operation: "backup_peek",
			Details: map[string]string{"got": fmt.Sprintf("%d", got), "supported": "1"}}, false)
}

// NewBackupTetherMismatchError sagt: "Backup ist an ein anderes Gerät gebunden."
func NewBackupTetherMismatchError() *GrimlockError {
	return newError(ErrCodeBackupTetherMismatch, "backup is tethered to a different vault — import denied", nil,
		ErrorContext{Operation: "backup_authorize"}, false)
}

// NewBackupSessionNotFoundError sagt: "session_id existiert nicht oder ist abgelaufen."
func NewBackupSessionNotFoundError(sessionID string) *GrimlockError {
	return newError(ErrCodeBackupSessionNotFound, "backup import session not found or expired", nil,
		ErrorContext{Operation: "backup_authorize",
			Details: map[string]string{"session_id": safePrefix(sessionID, 8)}}, false)
}

// NewBackupDecryptFailedError sagt: "Backup-Payload konnte nicht entschlüsselt werden."
func NewBackupDecryptFailedError(cause error) *GrimlockError {
	return newError(ErrCodeBackupDecryptFailed, "backup payload decryption failed — wrong key or corrupted file", cause,
		ErrorContext{Operation: "backup_authorize"}, true)
}

// NewBackupChecksumFailedError sagt: "Post-Write-Checksum weicht vom In-Memory-Hash ab — Bit-Flip beim Schreiben."
func NewBackupChecksumFailedError(path string) *GrimlockError {
	return newError(ErrCodeBackupChecksumFailed, "post-write checksum mismatch — possible bit-flip during write", nil,
		ErrorContext{Operation: "backup_export", Details: map[string]string{"path": path}}, false)
}

// NewBackupHeaderTamperedError sagt: "HeaderHMAC stimmt nicht — Header wurde manipuliert."
func NewBackupHeaderTamperedError(path string) *GrimlockError {
	return newError(ErrCodeBackupHeaderTampered, "backup header integrity check failed — file may be tampered", nil,
		ErrorContext{Operation: "backup_peek", Details: map[string]string{"path": path}}, false)
}
