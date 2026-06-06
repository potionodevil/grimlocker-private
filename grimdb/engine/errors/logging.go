// Package errors (logging.go) definiert StructuredLogger — das Logging-Interface
// für alle Grimlocker-Module — und StdLogger, seine Standard-Bibliotheks-Implementierung.
//
// Module sollten ein StructuredLogger als Dependency akzeptieren, statt log.Printf
// direkt aufzurufen. Das entkoppelt Business-Logik vom Log-Formatting und macht
// Log-Output testbar.
//
// Um einen GrimlockError mit vollem Kontext (Code, Operation, Block-ID, Stacktrace)
// an einen Log-Eintrag anzuhängen, nutze die Log-Methode am Error selbst:
//
//	if ge, ok := err.(*errors.GrimlockError); ok {
//	    ge.WithModule("storage").Log(logger)
//	}
//
// Um StdLogger mit zerolog, zap oder slog zu ersetzen, implementiere das
// Drei-Methoden-StructuredLogger-Interface und injiziere es in die Module-Constructors.
package errors

import (
	"fmt"
	"log"
	"strings"
)

// ErrorLevel klassifiziert die Schwere eines GrimlockError für die Terminal-Anzeige.
type ErrorLevel int

const (
	LevelInfo     ErrorLevel = iota // Information — kein Handlungsbedarf
	LevelWarn                       // Warnung — degradierter Betrieb
	LevelError                      // Fehler — Operation fehlgeschlagen
	LevelCritical                   // Kritisch — Systemstabilität gefährdet
	LevelSecurity                   // Security-Event — Auth/Policy/Lockdown
)

// levelLabel gibt das Terminal-Label für ein Error-Level zurück.
func levelLabel(level ErrorLevel) string {
	switch level {
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelCritical:
		return "CRITICAL"
	case LevelSecurity:
		return "SECURITY"
	default:
		return "ERROR"
	}
}

// ErrorRemediation mappt Error-Codes auf kurze Recovery-Hinweise fürs Terminal.
// Operatoren nutzen das, um Probleme ohne Source-Lektüre zu diagnostizieren.
var ErrorRemediation = map[int]string{
	ErrCodeVaultLocked:         "Unlock the vault first (send AUTH.UNLOCK)",
	ErrCodeVaultNotInitialized: "Run vault setup (send VAULT.INIT)",
	ErrCodeAuthInvalid:         "Check your password and try again",
	ErrCodeAuthLockdown:        "Too many failed attempts — wait for lockout to expire",
	ErrCodeStorageIO:           "Check vault file permissions and available disk space",
	ErrCodeStorageCorruption:   "Vault data may be corrupted — check HMAC and backup",
	ErrCodeStorageNotFound:     "Block does not exist — it may have been deleted",
	ErrCodeStorageIndexFailed:  "Index persist failed — check disk space and permissions",
	ErrCodeCryptoDecryption:    "Decryption failed — wrong key or tampered data",
	ErrCodeCryptoInvalidKey:    "Key material is nil or wrong length (need 32 bytes)",
	ErrCodeSecurityMemlock:     "Cannot lock memory — check OS limits (ulimit -l)",
	ErrCodeSecurityLockdown:    "Hard lockdown — restart daemon and re-enter password",
	ErrCodeSecurityMVKMissing:  "Vault is locked — unlock before this operation",
	ErrCodeBusTimeout:          "Request timed out — daemon may be overloaded",
	ErrCodeProtocolInvalid:     "Binary frame is malformed — check client version",
}

// Remediation gibt einen lesbaren Recovery-Hinweis für diesen Error zurück.
// Generische Message, wenn kein spezifischer Hint registriert ist.
func (e *GrimlockError) Remediation() string {
	if hint, ok := ErrorRemediation[e.Code]; ok {
		return hint
	}
	return "Check daemon logs for details"
}

// ConsoleFormat formatiert den Error für lesbare Terminal-Ausgabe.
// Zeigt Code, Message, Operation, Cause und Remediation-Hint.
// Enthält KEINEN Stacktrace (dafür Log verwenden).
func (e *GrimlockError) ConsoleFormat() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[Code %d] %s", e.Code, e.Message))
	if e.Ctx.Operation != "" {
		b.WriteString(fmt.Sprintf("\n        Operation:   %s", e.Ctx.Operation))
	}
	if e.Ctx.BlockID != "" {
		b.WriteString(fmt.Sprintf("\n        BlockID:     %s", e.Ctx.BlockID))
	}
	if e.Cause != nil {
		b.WriteString(fmt.Sprintf("\n        Cause:       %s", e.Cause.Error()))
	}
	for k, v := range e.Ctx.Details {
		b.WriteString(fmt.Sprintf("\n        %s: %s", k, v))
	}
	b.WriteString(fmt.Sprintf("\n        Remediation: %s", e.Remediation()))
	return b.String()
}

// ─── StructuredLogger-Interface ───────────────────────────────────────────────

// StructuredLogger ist das Logging-Interface für alle Grimlocker-Module.
// Als Dependency injizieren — niemals log.Printf direkt aus Module-Code aufrufen.
type StructuredLogger interface {
	Debug(msg string, fields map[string]any)
	Info(msg string, fields map[string]any)
	Warn(msg string, fields map[string]any)
	Error(msg string, err error, fields map[string]any)
	Fatal(msg string, err error, fields map[string]any)
}

// ─── Log-Methode auf GrimlockError ─────────────────────────────────────────────

// Log schreibt eine strukturierte Repräsentation dieses Errors in den übergebenen Logger.
// Rufe das oben im Handler auf, nachdem du einen GrimlockError erhalten hast.
func (e *GrimlockError) Log(logger StructuredLogger) {
	fields := map[string]any{
		"error_code": e.Code,
		"module":     e.ModuleID,
		"operation":  e.Ctx.Operation,
	}
	if e.Ctx.BlockID != "" {
		fields["block_id"] = e.Ctx.BlockID
	}
	for k, v := range e.Ctx.Details {
		fields["detail_"+k] = v
	}
	if e.EventType != "" {
		fields["event_type"] = e.EventType
	}
	if len(e.Stack) > 0 {
		frames := make([]string, 0, len(e.Stack))
		for _, f := range e.Stack {
			frames = append(frames, f.String())
		}
		fields["stacktrace"] = frames
	}

	logger.Error(e.Message, e.Cause, fields)
}

// ─── StdLogger — wrappt das Standard-Log-Package ──────────────────────────────

// StdLogger wrappt das Standard-Log-Package als StructuredLogger.
// Jedes Feld wird als key=value an die Log-Zeile angehängt.
// In Produktion durch ein strukturiertes Logging-Library (zerolog, zap, slog) ersetzen.
type StdLogger struct {
	Prefix       string
	DebugEnabled bool
}

func (l *StdLogger) format(level, msg string, fields map[string]any) string {
	var b strings.Builder
	if l.Prefix != "" {
		b.WriteString(l.Prefix)
		b.WriteString(" ")
	}
	b.WriteString(fmt.Sprintf("[%s] %s", level, msg))
	for k, v := range fields {
		b.WriteString(fmt.Sprintf(" %s=%v", k, v))
	}
	return b.String()
}

func (l *StdLogger) Debug(msg string, fields map[string]any) {
	if !l.DebugEnabled {
		return
	}
	log.Print(l.format("DEBUG", msg, fields))
}

func (l *StdLogger) Info(msg string, fields map[string]any) {
	log.Print(l.format("INFO", msg, fields))
}

func (l *StdLogger) Warn(msg string, fields map[string]any) {
	log.Print(l.format("WARN", msg, fields))
}

func (l *StdLogger) Error(msg string, err error, fields map[string]any) {
	if err != nil {
		if fields == nil {
			fields = make(map[string]any)
		}
		if ge, ok := err.(*GrimlockError); ok {
			fields["grimlock_error"] = ge.ConsoleFormat()
		} else {
			fields["error"] = err.Error()
		}
	}
	log.Print(l.format("ERROR", msg, fields))
}

func (l *StdLogger) Fatal(msg string, err error, fields map[string]any) {
	if err != nil {
		if fields == nil {
			fields = make(map[string]any)
		}
		fields["error"] = err.Error()
	}
	log.Fatal(l.format("FATAL", msg, fields))
}

// NewStdLogger erzeugt einen StdLogger für das gegebene Module-Prefix.
func NewStdLogger(modulePrefix string) *StdLogger {
	return &StdLogger{Prefix: modulePrefix}
}

// NewDebugLogger erzeugt einen StdLogger mit aktiviertem Debug-Output.
func NewDebugLogger(modulePrefix string) *StdLogger {
	return &StdLogger{Prefix: modulePrefix, DebugEnabled: true}
}
