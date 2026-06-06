package kernel

import (
	"context"
	"fmt"
)

// Registry wrappt einen Dispatcher und bietet geordnetes Module-Startup.
type Registry struct {
	bus     Dispatcher
	started []Module
}

// NewRegistry erzeugt ein Registry mit dem gegebenen Dispatcher.
func NewRegistry(d Dispatcher) *Registry {
	return &Registry{bus: d}
}

// Add registriert ein Module auf dem Bus und merkt es fürs geordnete Startup vor.
func (r *Registry) Add(m Module) error {
	if err := r.bus.Register(m); err != nil {
		return fmt.Errorf("register %s: %w", m.ID(), err)
	}
	r.started = append(r.started, m)
	return nil
}

// StartAll ruft Start auf jedem registrierten Module in Registrierungs-Reihenfolge auf.
func (r *Registry) StartAll(ctx context.Context) error {
	for _, m := range r.started {
		if err := m.Start(ctx, r.bus); err != nil {
			return fmt.Errorf("start %s: %w", m.ID(), err)
		}
	}
	return nil
}

// Bus gibt den darunterliegenden Dispatcher für Non-Module-Code zurück (z.B. API-Layer).
func (r *Registry) Bus() Dispatcher {
	return r.bus
}

// Modules gibt eine Kopie der registrierten Module-Liste zurück.
func (r *Registry) Modules() []Module {
	mods := make([]Module, len(r.started))
	copy(mods, r.started)
	return mods
}
