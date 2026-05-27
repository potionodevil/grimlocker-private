package sdk

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/grimlocker/grimdb/kernel"
)

// Dispatcher is the restricted plugin view of the kernel bus.
// Plugins may only publish and subscribe to SYNC.* channels.
// Attempts to access AUTH, SECURITY, CRYPTO, or STORAGE are rejected.
type Dispatcher interface {
	// Request dispatches e and blocks until a response arrives or timeout elapses.
	Request(e Event, timeout time.Duration) (Event, error)
	// Publish fires an event without waiting for a response.
	Publish(e Event) error
	// Subscribe registers a handler for the given event type.
	// Returns an unsubscribe function.
	Subscribe(eventType EventType, handler func(Event)) (unsubscribe func(), err error)
}

// allowedChannels lists the channels plugins are permitted to use.
// AUTH, SECURITY, CRYPTO, and STORAGE are never accessible from the SDK.
var allowedChannels = map[string]bool{
	"SYNC":      true,
	"BIOMETRIC": true,
}

// restrictedDispatcher wraps the kernel bus and enforces channel restrictions.
type restrictedDispatcher struct {
	bus kernel.Dispatcher
}

// NewDispatcher creates a Dispatcher that wraps the kernel bus.
func NewDispatcher(bus kernel.Dispatcher) Dispatcher {
	return &restrictedDispatcher{bus: bus}
}

func (d *restrictedDispatcher) allow(t EventType) error {
	ch := channel(string(t))
	if !allowedChannels[ch] {
		return fmt.Errorf("sdk: access to channel %q is not permitted", ch)
	}
	return nil
}

func (d *restrictedDispatcher) Request(e Event, timeout time.Duration) (Event, error) {
	if err := d.allow(e.Type); err != nil {
		return Event{}, err
	}

	ke := kernel.Event{
		ID:        e.ID,
		Type:      kernel.EventType(e.Type),
		Payload:   e.Payload,
		Origin:    e.Origin,
		Timestamp: e.Timestamp,
		TTL:       8,
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	kr, err := d.bus.Request(ctx, ke)
	if err != nil {
		return Event{}, err
	}

	return Event{
		ID:        kr.ID,
		Type:      EventType(kr.Type),
		Payload:   kr.Payload,
		ReplyTo:   kr.ReplyTo,
		Origin:    kr.Origin,
		Timestamp: kr.Timestamp,
	}, nil
}

func (d *restrictedDispatcher) Publish(e Event) error {
	if err := d.allow(e.Type); err != nil {
		return err
	}
	ke := kernel.Event{
		ID:        e.ID,
		Type:      kernel.EventType(e.Type),
		Payload:   e.Payload,
		Origin:    e.Origin,
		Timestamp: e.Timestamp,
		TTL:       8,
	}
	return d.bus.Dispatch(ke)
}

func (d *restrictedDispatcher) Subscribe(eventType EventType, handler func(Event)) (func(), error) {
	if err := d.allow(eventType); err != nil {
		return nil, err
	}
	unsub := d.bus.Subscribe(kernel.EventType(eventType), func(ke kernel.Event) error {
		handler(Event{
			ID:        ke.ID,
			Type:      EventType(ke.Type),
			Payload:   ke.Payload,
			ReplyTo:   ke.ReplyTo,
			Origin:    ke.Origin,
			Timestamp: ke.Timestamp,
		})
		return nil
	})
	return unsub, nil
}

func channel(t string) string {
	if i := strings.Index(t, "."); i >= 0 {
		return t[:i]
	}
	return t
}
