// Package crypto (handler_registry.go) provides HandlerRegistry and
// PayloadValidator — the typed dispatch layer for the crypto module.
//
// HandlerRegistry replaces the raw map[EventType]handlerFn pattern with
// an API that enforces invariants at registration time:
//   - Duplicate event types are detected immediately (panic via MustRegister).
//   - Each handler can declare a PayloadValidator that runs before the handler,
//     rejecting malformed payloads without entering business logic.
//
// JSONSchemaValidator is a generic helper that uses Go generics to unmarshal
// the payload into a typed struct and run an optional check function:
//
//	r.MustRegister(kernel.EvCryptoEncrypt,
//	    JSONSchemaValidator(func(p *encryptPayload) error {
//	        if p.KeyHandle == "" { return errors.New("key_handle required") }
//	        return nil
//	    }),
//	    m.handleEncrypt,
//	)
package crypto

import (
	"encoding/json"
	"fmt"

	"github.com/grimlocker/grimdb/kernel"
)

// ─── PayloadValidator ─────────────────────────────────────────────────────────

// PayloadValidator validates the raw JSON payload of an event before the handler
// runs. Validators are registered alongside handlers in HandlerRegistry.
// Return a non-nil error to reject the event before the handler is called.
type PayloadValidator interface {
	Validate(payload []byte) error
}

// ValidatorFunc adapts a plain function as a PayloadValidator.
type ValidatorFunc func([]byte) error

func (f ValidatorFunc) Validate(payload []byte) error { return f(payload) }

// JSONSchemaValidator is a generic validator that unmarshals JSON into a value
// of type T and calls the user-provided check function.
// T must be a pointer type (e.g. *encryptPayload).
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

// ─── HandlerRegistry ─────────────────────────────────────────────────────────

// HandlerEntry pairs a handler with its optional validator.
type HandlerEntry struct {
	Validator PayloadValidator   // may be nil (no pre-validation)
	Handler   eventHandlerFn     // always non-nil
}

// HandlerRegistry maps EventTypes to validated handler entries.
// It replaces raw map[EventType]eventHandlerFn in module buildHandlers() calls,
// adding pre-validation and an explicit registration API that enforces invariants.
type HandlerRegistry struct {
	entries map[kernel.EventType]HandlerEntry
}

// NewHandlerRegistry creates an empty registry.
func NewHandlerRegistry() *HandlerRegistry {
	return &HandlerRegistry{entries: make(map[kernel.EventType]HandlerEntry)}
}

// Register adds a handler (with optional validator) for the given event type.
// Returns an error if the event type was already registered (prevents accidental
// double-registration which would silently shadow the first handler).
func (r *HandlerRegistry) Register(
	eventType kernel.EventType,
	validator PayloadValidator,
	handler eventHandlerFn,
) error {
	if handler == nil {
		return fmt.Errorf("handler_registry: Register called with nil handler for %s", eventType)
	}
	if _, exists := r.entries[eventType]; exists {
		return fmt.Errorf("handler_registry: event type %s already registered", eventType)
	}
	r.entries[eventType] = HandlerEntry{Validator: validator, Handler: handler}
	return nil
}

// MustRegister is like Register but panics on error.
// Use during module initialization where a duplicate registration is a
// programming error that should fail fast (not be silently swallowed).
func (r *HandlerRegistry) MustRegister(
	eventType kernel.EventType,
	validator PayloadValidator,
	handler eventHandlerFn,
) {
	if err := r.Register(eventType, validator, handler); err != nil {
		panic(err)
	}
}

// Noop registers a no-op handler for the given event type.
// Use this to explicitly silence "no_handler" debug logs for events that this
// module receives but intentionally ignores (cross-channel passthrough events).
func (r *HandlerRegistry) Noop(eventTypes ...kernel.EventType) {
	noop := func(kernel.Event) error { return nil }
	for _, et := range eventTypes {
		// Ignore error — duplicate noop is fine (idempotent).
		_ = r.Register(et, nil, noop)
	}
}

// Dispatch looks up and calls the handler for the given event.
// If a validator is registered, it runs first — a validation error is returned
// directly without calling the handler.
// Returns nil for unknown event types (matches the existing no-op pattern).
func (r *HandlerRegistry) Dispatch(e kernel.Event) error {
	entry, ok := r.entries[e.Type]
	if !ok {
		return nil // unknown event — silently ignore
	}
	if entry.Validator != nil {
		if err := entry.Validator.Validate(e.Payload); err != nil {
			return fmt.Errorf("payload validation for %s: %w", e.Type, err)
		}
	}
	return entry.Handler(e)
}

// EventTypes returns all registered event types (for introspection / testing).
func (r *HandlerRegistry) EventTypes() []kernel.EventType {
	out := make([]kernel.EventType, 0, len(r.entries))
	for et := range r.entries {
		out = append(out, et)
	}
	return out
}
