// Package backup (handlers.go) — HandlerRegistry and PayloadValidator.
// Identical pattern to daemon/internal/modules/crypto/handlers.go.
package backup

import (
	"encoding/json"
	"fmt"

	"github.com/grimlocker/grimdb/engine/kernel"
)

type eventHandlerFn func(kernel.Event) error

// PayloadValidator validates the raw JSON payload of an event before the handler runs.
type PayloadValidator interface {
	Validate(payload []byte) error
}

// ValidatorFunc adapts a function as a PayloadValidator.
type ValidatorFunc func([]byte) error

func (f ValidatorFunc) Validate(payload []byte) error { return f(payload) }

// JSONSchemaValidator is a generic validator that unmarshals JSON into T and calls check.
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

// HandlerEntry pairs a handler with its optional validator.
type HandlerEntry struct {
	Validator PayloadValidator
	Handler   eventHandlerFn
}

// HandlerRegistry maps EventTypes to validated handler entries.
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
