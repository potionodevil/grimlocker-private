package gql

import (
	"fmt"
	"strings"
)

// SessionInfo stellt den Runtime-Context bereit, der für die semantische (ACL-)Validierung
// gebraucht wird. Die konkrete Implementierung kommt vom security.SessionContext des Daemons.
type SessionInfo interface {
	// IsUnlocked gibt zurück, ob der Vault gerade unlocked ist.
	IsUnlocked() bool

	// ActiveHandle gibt den MVK-Handle zurück, wenn die Session unlocked ist.
	ActiveHandle() string

	// UserID gibt die Subject-ID des authentifizierten Users zurück.
	// Leerer String bedeutet anonym (pre-auth).
	UserID() string

	// HasRole gibt zurück, ob die Session die angegebene RBAC-Rolle hat.
	HasRole(role string) bool
}

// SyntacticError beschreibt ein Frame, das die Schema-Validierung nicht besteht.
type SyntacticError struct {
	Field   string
	Reason  string
	Details string
}

func (e *SyntacticError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("gql: syntactic error in %q: %s (%s)", e.Field, e.Reason, e.Details)
	}
	return fmt.Sprintf("gql: syntactic error in %q: %s", e.Field, e.Reason)
}

// SemanticError beschreibt ein Frame, das die ACL/Authorization-Validierung nicht besteht.
type SemanticError struct {
	Operation Operation
	Reason    string
}

func (e *SemanticError) Error() string {
	return fmt.Sprintf("gql: semantic error for %q: %s", e.Operation, e.Reason)
}

// ValidateFrame führt die vollständige 2-stufige Validierung eines GQL-Frames durch.
//
// Stufe 1 — Syntaktisch:
//   - Version muss Version (1) sein
//   - Opcode muss bekannt sein
//   - Payload muss sich zu einem gültigen GQLQuery dekodieren lassen
//   - Alle String-Felder müssen innerhalb der Längenlimits liegen
//   - Keine Null-Bytes oder Steuerzeichen in String-Feldern
//   - Field-Count muss im Limit sein
//   - Operation muss zum Opcode passen (query vs mutate)
//
// Stufe 2 — Semantisch:
//   - Session muss unlocked sein für jede Operation
//   - Write-Operationen brauchen Credentials
//   - Namespace muss zur Session-UserID passen (RBAC)
//   - Passende Rolle für die Operation erforderlich
//
// Gibt den dekodierten GQLQuery bei Erfolg zurück, oder einen Error.
func ValidateFrame(frame *Frame, session SessionInfo) (*GQLQuery, error) {
	if frame.Version != Version {
		return nil, &SyntacticError{Field: "version", Reason: fmt.Sprintf("unsupported version %d (expected %d)", frame.Version, Version)}
	}

	if !isValidOpcode(frame.Opcode) {
		return nil, &SyntacticError{Field: "opcode", Reason: fmt.Sprintf("unknown opcode 0x%02x", byte(frame.Opcode))}
	}

	// Result- und Error-Frames brauchen keine Query-Level-Validierung.
	if frame.Opcode == OpcodeResult || frame.Opcode == OpcodeError {
		return nil, nil
	}

	if frame.PayloadSize > MaxPayloadSize {
		return nil, &SyntacticError{
			Field:   "payload_size",
			Reason:  "exceeds maximum",
			Details: fmt.Sprintf("%d > %d", frame.PayloadSize, MaxPayloadSize),
		}
	}

	if len(frame.Payload) == 0 {
		return nil, &SyntacticError{Field: "payload", Reason: "empty payload"}
	}

	// Payload-Bytes auf struktureller Ebene validieren, bevor dekodiert wird.
	if err := validatePayloadBytes(frame.Payload); err != nil {
		return nil, err
	}

	var op Operation
	switch frame.Opcode {
	case OpcodeQuery:
	case OpcodeMutate:
	default:
		return nil, &SyntacticError{Field: "opcode", Reason: "opcode not allowed for query validation"}
	}

	query, err := DecodeQuery(frame.Payload, op)
	if err != nil {
		return nil, &SyntacticError{Field: "payload", Reason: "decode failed", Details: err.Error()}
	}

	if err := validateQuery(query, frame.Opcode); err != nil {
		return nil, err
	}

	// -- Stufe 2: Semantische (ACL-)Validierung --
	if err := validateACL(query, session); err != nil {
		return nil, err
	}

	return query, nil
}

// validatePayloadBytes prüft den rohen Payload auf strukturelle Integrität,
// bevor dekodiert wird. Fängt malformed/corrupted Payloads früh.
func validatePayloadBytes(payload []byte) error {
	if len(payload) == 0 {
		return &SyntacticError{Field: "payload", Reason: "zero-length"}
	}

	// Grundlegende Sanity: Field-Count und Längen dürfen keine Endlos-Loops verursachen.
	fieldCount := int(payload[0])
	if fieldCount > MaxFieldsCount {
		return &SyntacticError{
			Field:   "field_count",
			Reason:  "exceeds maximum",
			Details: fmt.Sprintf("%d > %d", fieldCount, MaxFieldsCount),
		}
	}

	return nil
}

// validateQuery führt die Feld-Level-Syntaktische-Validierung auf einem dekodierten GQLQuery durch.
func validateQuery(q *GQLQuery, opcode Opcode) error {
	if err := validateStringField("namespace", q.Namespace, true, MaxNamespaceLen); err != nil {
		return err
	}
	if err := validateIdentifier("namespace", q.Namespace); err != nil {
		return err
	}

	if q.EntryID != "" {
		if err := validateStringField("entry_id", q.EntryID, false, MaxEntryIDLen); err != nil {
			return err
		}
		if err := validateIdentifier("entry_id", q.EntryID); err != nil {
			return err
		}
	}

	if q.Category != "" {
		if err := validateStringField("category", q.Category, false, MaxCategoryLen); err != nil {
			return err
		}
		if err := validateIdentifier("category", q.Category); err != nil {
			return err
		}
	}

	if q.Title != "" {
		if err := validateStringField("title", q.Title, false, MaxFieldValueLen); err != nil {
			return err
		}
		if err := validatePrintable("title", q.Title); err != nil {
			return err
		}
	}

	if len(q.Fields) > MaxFieldsCount {
		return &SyntacticError{
			Field:   "fields",
			Reason:  "too many fields",
			Details: fmt.Sprintf("%d > %d", len(q.Fields), MaxFieldsCount),
		}
	}
	for k, v := range q.Fields {
		if err := validateStringField("fields.key", k, false, MaxFieldKeyLen); err != nil {
			return err
		}
		if err := validateIdentifier("fields.key", k); err != nil {
			return err
		}
		if err := validateStringField("fields.value", v, false, MaxFieldValueLen); err != nil {
			return err
		}
		if err := validatePrintable("fields.value", v); err != nil {
			return err
		}
	}

	if opcode == OpcodeQuery && !isReadOperation(q.Operation) {
		return &SyntacticError{
			Field:   "operation",
			Reason:  "opcode mismatch",
			Details: fmt.Sprintf("opcode QUERY requires a read operation, got %q", q.Operation),
		}
	}
	if opcode == OpcodeMutate && !isWriteOperation(q.Operation) {
		return &SyntacticError{
			Field:   "operation",
			Reason:  "opcode mismatch",
			Details: fmt.Sprintf("opcode MUTATE requires a write operation, got %q", q.Operation),
		}
	}

	return nil
}

// validateACL führt semantische Authorization-Checks durch.
func validateACL(q *GQLQuery, session SessionInfo) error {
	if session == nil {
		return &SemanticError{Operation: q.Operation, Reason: "no active session"}
	}

	if !session.IsUnlocked() {
		return &SemanticError{Operation: q.Operation, Reason: "vault locked"}
	}

	if session.ActiveHandle() == "" {
		return &SemanticError{Operation: q.Operation, Reason: "no active MVK handle"}
	}

	// Write-Operationen brauchen Credentials
	if isWriteOperation(q.Operation) && len(q.Credentials) == 0 {
		return &SemanticError{
			Operation: q.Operation,
			Reason:    "write operation requires credentials",
		}
	}

	// RBAC: Namespace muss mit der Session-UserID übereinstimmen (wenn RBAC aktiv ist).
	userID := session.UserID()
	if userID != "" && q.Namespace != "" && q.Namespace != userID {
		if !session.HasRole("admin") {
			return &SemanticError{
				Operation: q.Operation,
				Reason:    fmt.Sprintf("namespace %q does not match session user %q", q.Namespace, userID),
			}
		}
	}

	return nil
}

// validateStringField prüft Länge und Inhalt eines String-Felds.
func validateStringField(name, value string, required bool, maxLen int) error {
	if required && value == "" {
		return &SyntacticError{Field: name, Reason: "required field is empty"}
	}
	if len(value) > maxLen {
		return &SyntacticError{
			Field:   name,
			Reason:  "exceeds maximum length",
			Details: fmt.Sprintf("%d > %d", len(value), maxLen),
		}
	}
	return nil
}

// validateIdentifier prüft, dass ein String nur safe Identifier-Zeichen enthält.
// Erlaubt: a-z, A-Z, 0-9, _, -, .
// Verhindert Injection-Angriffe durch Identifier-Felder.
func validateIdentifier(name, value string) error {
	if value == "" {
		return nil
	}
	for i, c := range value {
		if !isIdentChar(c) {
			return &SyntacticError{
				Field:   name,
				Reason:  "invalid character in identifier",
				Details: fmt.Sprintf("position %d: character %q not allowed — only alphanumeric, _, -, . permitted", i, string(c)),
			}
		}
	}
	return nil
}

// validatePrintable prüft, dass ein String nur druckbare Zeichen enthält.
// Keine Steuerzeichen (Tab, Newline, etc.), keine Null-Bytes.
func validatePrintable(name, value string) error {
	if value == "" {
		return nil
	}
	for i, c := range value {
		if c < 0x20 && c != '\t' {
			return &SyntacticError{
				Field:   name,
				Reason:  "control character in text field",
				Details: fmt.Sprintf("position %d: 0x%02x", i, c),
			}
		}
		if c == 0x7F {
			return &SyntacticError{
				Field:   name,
				Reason:  "DEL character in text field",
				Details: fmt.Sprintf("position %d", i),
			}
		}
	}
	return nil
}

// isIdentChar gibt true zurück, wenn c ein gültiges Identifier-Zeichen ist.
func isIdentChar(c rune) bool {
	if c >= 'a' && c <= 'z' {
		return true
	}
	if c >= 'A' && c <= 'Z' {
		return true
	}
	if c >= '0' && c <= '9' {
		return true
	}
	if c == '_' || c == '-' || c == '.' {
		return true
	}
	return false
}

// SanitizeFieldKey normalisiert einen Field-Key für die sichere Speicherung.
// Entfernt führende/trailende Leerzeichen und ersetzt invalide Zeichen.
func SanitizeFieldKey(key string) string {
	key = strings.TrimSpace(key)
	var b strings.Builder
	b.Grow(len(key))
	for _, c := range key {
		if isIdentChar(c) {
			b.WriteRune(c)
		}
	}
	return b.String()
}
