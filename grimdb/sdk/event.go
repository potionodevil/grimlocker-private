// Package sdk is the public plugin surface for Grimlocker Omega extensions.
// It provides a strictly restricted subset of the kernel interfaces so that
// plugins cannot reach AUTH, SECURITY, or CRYPTO internals.
package sdk

import "time"

// EventType is a subset of kernel.EventType exposed to plugins.
// Plugins may only publish/subscribe to SYNC.* and custom channels.
type EventType string

const (
	EvSyncBegin    EventType = "SYNC.BEGIN"
	EvSyncComplete EventType = "SYNC.COMPLETE"
)

// Event is the plugin-facing event struct. Layout mirrors kernel.Event but
// only SYNC-channel events flow through the SDK dispatcher.
type Event struct {
	ID        string    `json:"id"`
	Type      EventType `json:"type"`
	Payload   []byte    `json:"payload,omitempty"`
	ReplyTo   string    `json:"reply_to,omitempty"`
	Origin    string    `json:"origin,omitempty"`
	Timestamp int64     `json:"timestamp"`
}

// NewEvent creates a plugin event with the current timestamp.
func NewEvent(origin string, t EventType, payload []byte) Event {
	return Event{
		ID:        randomID(),
		Type:      t,
		Payload:   payload,
		Origin:    origin,
		Timestamp: time.Now().UnixNano(),
	}
}
