package kernel

import "context"

// Dispatcher ist das einzige Kommunikations-Interface zwischen Modulen.
// Module DÜRFEN sich nicht gegenseitig importieren — sie interagieren ausschließlich via Dispatch.
type Dispatcher interface {
	// Dispatch sendet ein Event an alle registrierten Handler für seinen Channel.
	// Asynchron — Handler laufen in ihren eigenen Goroutinen.
	Dispatch(e Event) error

	// Request dispatched e und blockt, bis eine Response mit ReplyTo == e.ID
	// eintrifft, oder der Context gecancelled wird.
	Request(ctx context.Context, e Event) (Event, error)

	// Subscribe registriert einen Handler für einen bestimmten Event-Type.
	// Gibt eine Unsubscribe-Funktion zurück.
	Subscribe(eventType EventType, handler Handler) (unsubscribe func())

	// Register fügt ein Module zum Bus hinzu und subscribed es auf seine deklarierten Channels.
	Register(m Module) error

	// Unregister entfernt ein Module und alle seine Subscriptions.
	Unregister(moduleID string)

	// Shutdown drainet In-Flight-Events und stoppt den Bus.
	Shutdown(ctx context.Context) error
}

// Handler ist eine Funktion, die ein zugestelltes Event verarbeitet.
// Ein non-nil Error wird geloggt, stoppt aber nicht den Bus.
type Handler func(Event) error

// Module ist der Vertrag, den jedes Module erfüllen muss, um in den Kernel einzustecken.
type Module interface {
	// ID gibt den eindeutigen Identifier für dieses Module zurück (z.B. "crypto", "storage").
	ID() string

	// Channels gibt die Event-Type-Channel-Präfixe zurück, die dieses Module besitzt.
	// Der Bus routed alle Events mit passendem Prefix an Module.Handle.
	Channels() []string

	// Handle verarbeitet ein geroutetes Event. Wird in einer eigenen Goroutine pro Event aufgerufen.
	Handle(Event) error

	// Start wird einmal nach der Registration aufgerufen, vor dem ersten Event.
	Start(ctx context.Context, d Dispatcher) error

	// Stop wird während des Bus-Shutdowns aufgerufen.
	Stop() error
}
