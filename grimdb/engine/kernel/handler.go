// Package kernel (handler.go) stellt den HandlerBuilder bereit — eine fluide API
// zum Komponieren von bus.Handler-Funktionen mit Cross-Cutting-Concerns.
//
// Statt rohe Handler-Funktionen zu schreiben, die Business-Logik mit Error-Recovery
// und Logging vermischen, nutze HandlerBuilder, um Decorator zu schichten:
//
//	h := kernel.NewHandlerBuilder(myHandlerFunc).
//	    WithRecovery("[mymodule]"). // fängt Panics → wandelt in Error um
//	    WithLogging("[mymodule]").  // loggt Timing + Errors
//	    Build()
//	bus.Subscribe(kernel.EvMyEvent, h)
//
// Decorators werden outermost-first angewandt, also wird der erste hinzugefügte
// Decorator zur äußersten Schicht (z.B. Recovery umschließt Logging umschließt Base).
package kernel

import (
	"log"
	"runtime/debug"
	"time"
)

// ─── HandlerBuilder ───────────────────────────────────────────────────────────

// HandlerBuilder konstruiert einen Handler, indem er Decorator über einer
// Base-Funktion schichtet. Build() gibt den finalen komponierten Handler zurück.
type HandlerBuilder struct {
	base       Handler
	decorators []func(Handler) Handler
}

// NewHandlerBuilder erzeugt einen HandlerBuilder um den gegebenen Base-Handler.
func NewHandlerBuilder(h Handler) *HandlerBuilder {
	return &HandlerBuilder{base: h}
}

// WithRecovery fügt eine Panic-Recovery-Schicht hinzu. Jeder Panic im inneren
// Handler wird gefangen, mit Stacktrace geloggt und in einen non-nil Error
// umgewandelt. Das verhindert, dass ein fehlerhafter Handler die Goroutine killt.
func (b *HandlerBuilder) WithRecovery(modulePrefix string) *HandlerBuilder {
	b.decorators = append(b.decorators, func(next Handler) Handler {
		return func(e Event) (retErr error) {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("%s PANIC in handler for %s: %v\nStack:\n%s",
						modulePrefix, e.Type, r, debug.Stack())
					retErr = &handlerPanicError{event: string(e.Type), value: r}
				}
			}()
			return next(e)
		}
	})
	return b
}

// WithLogging fügt strukturiertes Logging vor und nach dem Handler hinzu.
// Auf DEBUG-Level: Entry + Exit-Timing.
// Bei Error: loggt den Error mit Event-Type.
func (b *HandlerBuilder) WithLogging(modulePrefix string) *HandlerBuilder {
	b.decorators = append(b.decorators, func(next Handler) Handler {
		return func(e Event) error {
			start := time.Now()
			err := next(e)
			if err != nil {
				log.Printf("%s handler error event=%s elapsed=%s err=%v",
					modulePrefix, e.Type, time.Since(start).Round(time.Microsecond), err)
			} else {
				log.Printf("%s [DEBUG] handler ok event=%s elapsed=%s",
					modulePrefix, e.Type, time.Since(start).Round(time.Microsecond))
			}
			return err
		}
	})
	return b
}

// WithMetrics fügt einfache Timing-Metriken hinzu. Loggt aktuell auf stdout;
// ersetze die Implementierung mit deinem Metrics-Backend (Prometheus, etc.).
func (b *HandlerBuilder) WithMetrics(modulePrefix, eventLabel string) *HandlerBuilder {
	b.decorators = append(b.decorators, func(next Handler) Handler {
		return func(e Event) error {
			start := time.Now()
			err := next(e)
			status := "ok"
			if err != nil {
				status = "error"
			}
			log.Printf("%s [METRIC] event=%s status=%s duration_us=%d",
				modulePrefix, eventLabel, status, time.Since(start).Microseconds())
			return err
		}
	})
	return b
}

// Build wendet alle registrierten Decorators an (outermost-first) und gibt den
// finalen komponierten Handler zurück, bereit für die Bus-Registration.
func (b *HandlerBuilder) Build() Handler {
	h := b.base
	for i := len(b.decorators) - 1; i >= 0; i-- {
		h = b.decorators[i](h)
	}
	return h
}

// ─── Standalone Decorator Functions ──────────────────────────────────────────

// WithRecovery wrappt einen einzelnen Handler mit Panic Recovery.
// Bevorzuge HandlerBuilder zum Verketten mehrerer Decorator; nutze das für
// einmalige Subscriptions.
func WithRecovery(modulePrefix string, h Handler) Handler {
	return NewHandlerBuilder(h).WithRecovery(modulePrefix).Build()
}

// WithLogging wrappt einen einzelnen Handler mit Entry/Exit-Logging.
func WithLogging(modulePrefix string, h Handler) Handler {
	return NewHandlerBuilder(h).WithLogging(modulePrefix).Build()
}

// ─── Internal types ───────────────────────────────────────────────────────────

// handlerPanicError ist der synthetische Error, den WithRecovery produziert.
type handlerPanicError struct {
	event string
	value interface{}
}

func (e *handlerPanicError) Error() string {
	return "panic in handler for " + e.event + ": recovered"
}
