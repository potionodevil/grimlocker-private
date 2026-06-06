// Package kernel implementiert den Event-Bus — das zentrale Nervensystem
// des Grimlocker-Daemons. Die gesamte Inter-Module-Kommunikation MUSS über den
// Dispatcher laufen — Module importieren oder rufen sich nie direkt auf.
//
// Core-Konzepte:
//
//   - Event: die Kommunikationseinheit (getypter JSON-Payload mit TTL und ID).
//   - Dispatcher: routet Events per Channel-Präfix zu registrierten Handlern.
//   - Module: eine zustandsbehaftete Komponente, die einen oder mehrere Channels besitzt.
//   - Handler: eine func(Event) error, die in einer eigenen Goroutine pro Event läuft.
//   - Gate: der STORAGE-Channel ist blockiert, bis AUTH.KEY_READY eintrifft —
//     verhindert Block-Reads/Writes, bevor der Vault unlocked ist.
//
// Bus-Lifecycle:
//
//  1. NewBus(opts...) — Bus erzeugen (mit optionalen Gated-Channels).
//  2. Register(module) — Module auf seine Channels subscriben.
//  3. StartAll(ctx) — Start() auf jedem registrierten Module in Reihenfolge.
//  4. Dispatch(event) / Request(ctx, event) — Events senden.
//  5. Shutdown(ctx) — drainen und alle Module stoppen.
package kernel

import (
	"context"
	"fmt"
	"log"
	"runtime/debug"
	"strings"
	"sync"
	"time"
)

// defaultTTL ist der Start-Hop-Count für jedes Event.
const defaultTTL = 8

// bus ist der konkrete Dispatcher. Er routet Events per Channel-Präfix,
// führt jeden Handler in seiner eigenen Goroutine aus und unterstützt
// synchrones Request/Response via Per-Event-Reply-Channels.
type bus struct {
	mu sync.RWMutex

	modules        map[string]Module
	channelHandlers map[string][]namedHandler
	typeHandlers   map[EventType][]namedHandler

	pending   map[string]chan Event
	pendingMu sync.Mutex

	gateMu        sync.RWMutex
	gatedChannels map[string]bool
	gateOpen      bool

	ctx    context.Context
	cancel context.CancelFunc
}

// BusOption konfiguriert einen Bus.
type BusOption func(*bus)

// WithGatedChannels gibt eine BusOption zurück, die die angegebenen Channel-Präfixe
// als gated markiert. Gated-Events werden silent gedroppt, bis OpenGate() aufgerufen wird.
func WithGatedChannels(channels ...string) BusOption {
	return func(b *bus) {
		b.gatedChannels = make(map[string]bool)
		for _, ch := range channels {
			b.gatedChannels[strings.ToUpper(ch)] = true
		}
	}
}

type namedHandler struct {
	id      string
	handler Handler
}

// NewBus konstruiert und gibt einen fertigen Dispatcher zurück.
func NewBus(opts ...BusOption) Dispatcher {
	ctx, cancel := context.WithCancel(context.Background())
	b := &bus{
		modules:         make(map[string]Module),
		channelHandlers: make(map[string][]namedHandler),
		typeHandlers:    make(map[EventType][]namedHandler),
		pending:         make(map[string]chan Event),
		ctx:             ctx,
		cancel:          cancel,
	}
	for _, opt := range opts {
		opt(b)
	}
	return b
}

// OpenGate hebt das Gate, sodass vorher gated Events jetzt durchfließen.
func (b *bus) OpenGate() {
	b.gateMu.Lock()
	b.gateOpen = true
	b.gateMu.Unlock()
	log.Printf("[bus] Gate opened — gated channels now flow")
}

// CloseGate senkt das Gate und blockiert gated Channels wieder.
func (b *bus) CloseGate() {
	b.gateMu.Lock()
	b.gateOpen = false
	b.gateMu.Unlock()
	log.Printf("[bus] Gate closed — gated channels blocked")
}

func (b *bus) Register(m Module) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, exists := b.modules[m.ID()]; exists {
		return fmt.Errorf("module %q already registered", m.ID())
	}

	b.modules[m.ID()] = m

	for _, ch := range m.Channels() {
		ch = strings.ToUpper(ch)
		b.channelHandlers[ch] = append(b.channelHandlers[ch], namedHandler{
			id:      m.ID(),
			handler: m.Handle,
		})
	}

	return nil
}

func (b *bus) Unregister(moduleID string) {
	b.mu.Lock()
	defer b.mu.Unlock()

	m, exists := b.modules[moduleID]
	if !exists {
		return
	}

	for _, ch := range m.Channels() {
		ch = strings.ToUpper(ch)
		handlers := b.channelHandlers[ch]
		filtered := handlers[:0]
		for _, h := range handlers {
			if h.id != moduleID {
				filtered = append(filtered, h)
			}
		}
		b.channelHandlers[ch] = filtered
	}

	delete(b.modules, moduleID)
}

func (b *bus) Subscribe(eventType EventType, handler Handler) (unsubscribe func()) {
	if handler == nil {
		panic(fmt.Sprintf("bus: Subscribe called with nil handler for %s", eventType))
	}
	id := newID()
	b.mu.Lock()
	b.typeHandlers[eventType] = append(b.typeHandlers[eventType], namedHandler{id: id, handler: handler})
	b.mu.Unlock()

	return func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		handlers := b.typeHandlers[eventType]
		filtered := handlers[:0]
		for _, h := range handlers {
			if h.id != id {
				filtered = append(filtered, h)
			}
		}
		b.typeHandlers[eventType] = filtered
	}
}

func (b *bus) Dispatch(e Event) error {
	if e.TTL <= 0 {
		if e.TTL == 0 {
			e.TTL = defaultTTL
		} else {
			log.Printf("[bus] event %s dropped: TTL exhausted", e.Type)
			return nil
		}
	}
	e.TTL--

	if e.Timestamp == 0 {
		e.Timestamp = time.Now().UnixNano()
	}

	// Wenn das Event eine Response ist (ReplyTo gesetzt), signalisiere den wartenden Request.
	if e.ReplyTo != "" {
		b.pendingMu.Lock()
		ch, found := b.pending[e.ReplyTo]
		b.pendingMu.Unlock()
		if found {
			select {
			case ch <- e:
			default:
			}
		}
	}

	channel := e.Type.Channel()

	// Gate-Check: Events für gated Channels droppen, bis das Gate offen ist.
	b.gateMu.RLock()
	isGated := b.gatedChannels[channel]
	open := b.gateOpen
	b.gateMu.RUnlock()
	if isGated && !open {
		log.Printf("[bus] event %s dropped: gate closed for channel %s", e.Type, channel)
		return nil
	}

	b.mu.RLock()
	chHandlers := make([]Handler, 0)
	for _, h := range b.channelHandlers[channel] {
		chHandlers = append(chHandlers, h.handler)
	}
	for _, h := range b.typeHandlers[e.Type] {
		chHandlers = append(chHandlers, h.handler)
	}
	b.mu.RUnlock()

	for _, h := range chHandlers {
		h := h
		go func() {
			defer func() {
				if r := recover(); r != nil {
					log.Printf("[bus] PANIC in handler for %s: %v\nStack:\n%s",
						e.Type, r, debug.Stack())
				}
			}()
			if err := h(e); err != nil {
				log.Printf("[bus] handler error for %s: %v", e.Type, err)
			}
		}()
	}

	return nil
}

func (b *bus) Request(ctx context.Context, e Event) (Event, error) {
	if e.ID == "" {
		e.ID = newID()
	}

	replyCh := make(chan Event, 1)

	b.pendingMu.Lock()
	b.pending[e.ID] = replyCh
	b.pendingMu.Unlock()

	defer func() {
		b.pendingMu.Lock()
		delete(b.pending, e.ID)
		b.pendingMu.Unlock()
	}()

	if err := b.Dispatch(e); err != nil {
		return Event{}, err
	}

	select {
	case reply := <-replyCh:
		return reply, nil
	case <-ctx.Done():
		return Event{}, ctx.Err()
	case <-b.ctx.Done():
		return Event{}, fmt.Errorf("bus shut down")
	}
}

func (b *bus) Shutdown(ctx context.Context) error {
	b.mu.RLock()
	mods := make([]Module, 0, len(b.modules))
	for _, m := range b.modules {
		mods = append(mods, m)
	}
	b.mu.RUnlock()

	var wg sync.WaitGroup
	for _, m := range mods {
		m := m
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := m.Stop(); err != nil {
				log.Printf("[bus] module %s stop error: %v", m.ID(), err)
			}
		}()
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
	}

	b.cancel()
	return nil
}

// NewEvent erzeugt ein Outbound-Event mit frischer UUID und aktuellem Timestamp.
func NewEvent(origin string, t EventType, payload []byte) Event {
	return Event{
		ID:        newID(),
		Type:      t,
		Payload:   payload,
		Origin:    origin,
		Timestamp: time.Now().UnixNano(),
		TTL:       defaultTTL,
	}
}

// ReplyEvent erzeugt ein Response-Event für ein gegebenes Request-Event.
func ReplyEvent(origin string, t EventType, req Event, payload []byte) Event {
	e := NewEvent(origin, t, payload)
	e.ReplyTo = req.ID
	return e
}
