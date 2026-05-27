package sdk

import (
	"crypto/rand"
	"fmt"
	"log"
	"sync"

	"github.com/grimlocker/grimdb/kernel"
)

// Registry manages the lifecycle of loaded plugins.
type Registry struct {
	mu      sync.RWMutex
	plugins map[string]Plugin
	bus     kernel.Dispatcher
}

// NewRegistry creates a Registry backed by the given kernel bus.
func NewRegistry(bus kernel.Dispatcher) *Registry {
	return &Registry{
		plugins: make(map[string]Plugin),
		bus:     bus,
	}
}

// Register loads a plugin, providing it with a restricted Dispatcher and logger.
// If the plugin provides a StorageStrategy it is returned for injection into
// the BlockStore by the caller.
func (r *Registry) Register(p Plugin) (StorageStrategy, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.plugins[p.ID()]; exists {
		return nil, fmt.Errorf("sdk: plugin %q already registered", p.ID())
	}

	d := NewDispatcher(r.bus)
	l := &stdLogger{prefix: fmt.Sprintf("[plugin:%s] ", p.ID())}

	if err := p.Init(d, l); err != nil {
		return nil, fmt.Errorf("sdk: plugin %q init failed: %w", p.ID(), err)
	}

	r.plugins[p.ID()] = p
	log.Printf("[sdk] Plugin loaded: %s v%s", p.Name(), p.Version())

	return p.StorageStrategy(), nil
}

// Unregister shuts down and removes a plugin.
func (r *Registry) Unregister(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	p, exists := r.plugins[id]
	if !exists {
		return fmt.Errorf("sdk: plugin %q not found", id)
	}

	if err := p.Shutdown(); err != nil {
		log.Printf("[sdk] Plugin %s shutdown error: %v", id, err)
	}

	delete(r.plugins, id)
	return nil
}

// List returns all loaded plugins.
func (r *Registry) List() []Plugin {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]Plugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		result = append(result, p)
	}
	return result
}

// ShutdownAll shuts down every loaded plugin in no particular order.
func (r *Registry) ShutdownAll() {
	r.mu.Lock()
	defer r.mu.Unlock()
	for id, p := range r.plugins {
		if err := p.Shutdown(); err != nil {
			log.Printf("[sdk] Plugin %s shutdown error: %v", id, err)
		}
	}
}

// stdLogger is a simple Logger backed by the standard log package.
type stdLogger struct{ prefix string }

func (l *stdLogger) Info(msg string, args ...interface{}) {
	log.Printf(l.prefix+"INFO "+msg, args...)
}
func (l *stdLogger) Warn(msg string, args ...interface{}) {
	log.Printf(l.prefix+"WARN "+msg, args...)
}
func (l *stdLogger) Error(msg string, args ...interface{}) {
	log.Printf(l.prefix+"ERROR "+msg, args...)
}

// randomID generates a random hex string for event IDs (no uuid dependency).
func randomID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return fmt.Sprintf("%x", b)
}
