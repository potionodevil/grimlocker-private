// Package backup (handlers.go) — HandlerRegistry und PayloadValidator für das Backup-Modul.
// Identisches Muster wie daemon/internal/modules/crypto/handlers.go.
package backup

import (
	"encoding/json"
	"fmt"

	"github.com/grimlocker/grimdb/engine/kernel"
)

// eventHandlerFn ist der interne Handler-Funktionstyp.
type eventHandlerFn func(kernel.Event) error

// PayloadValidator validiert den rohen JSON-Payload eines Events vor dem Handler.
type PayloadValidator interface {
	Validate(payload []byte) error
}

// ValidatorFunc adaptiert eine Funktion als PayloadValidator.
type ValidatorFunc func([]byte) error

func (f ValidatorFunc) Validate(payload []byte) error { return f(payload) }

// JSONSchemaValidator ist ein generischer Validator, der JSON in T unmarshalt und check aufruft.
func JSONSchemaValidator[T any](check func(*T) error) PayloadValidator {
	return ValidatorFunc(func(payload []byte) error {
		var v T
		if err := json.Unmarshal(payload, &v); err != nil {
			return fmt.Errorf("payload unmarshal: %w", err)
		}
		if check != nil {
			return check(&v)
		}
		return nil
	})
}

// HandlerEntry paart einen Handler mit seinem optionalen Validator.
type HandlerEntry struct {
	Validator PayloadValidator
	Handler   eventHandlerFn
}

// HandlerRegistry mappt EventTypes auf validierte Handler-Entries.
type HandlerRegistry struct {
	entries map[kernel.EventType]HandlerEntry
}

func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{entries: make(map[kernel.EventType]HandlerEntry)}
}

func (r *HandlerRegistry) MustRegister(et kernel.EventType, v PayloadValidator, h eventHandlerFn) {
	if _, exists := r.entries[et]; exists {
		panic(fmt.Sprintf("handler_registry: event type %s already registered", et))
	}
	r.entries[et] = HandlerEntry{Validator: v, Handler: h}
}

func (r *HandlerRegistry) Dispatch(e kernel.Event) error {
	entry, ok := r.entries[e.Type]
	if !ok {
		return nil
	}
	if entry.Validator != nil {
		if err := entry.Validator.Validate(e.Payload); err != nil {
			return fmt.Errorf("payload validation for %s: %w", e.Type, err)
		}
	}
	return entry.Handler(e)
}
