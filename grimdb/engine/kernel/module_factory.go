// Package kernel (module_factory.go) stellt ModuleConfig, ModuleFactory und BaseModule
// bereit — die Standard-Bausteine für kernel.Module-Implementierungen.
//
// Jedes Module sollte:
//  1. BaseModule embeden, um ID()/Channels() gratis zu bekommen.
//  2. Einen ModuleConfig im Constructor akzeptieren statt positional params.
//  3. Optional ModuleFactory implementieren für generische Registration.
//
// Example:
//
//	type MyModule struct {
//	    kernel.BaseModule
//	}
//	func NewMyModule(cfg kernel.ModuleConfig) *MyModule {
//	    return &MyModule{BaseModule: kernel.NewBaseModule(cfg)}
//	}
package kernel

import "context"

// ─── ModuleConfig ─────────────────────────────────────────────────────────────

// ModuleConfig ist der kanonische Parametersatz für jeden Module-Constructor.
// Ein Config-Struct statt positional params macht es einfach, neue optionale Felder
// hinzuzufügen, ohne existierende Call-Sites zu brechen.
//
// Module, die nicht alle Felder brauchen, ignorieren die ungenutzten einfach.
type ModuleConfig struct {
	// ID ist der eindeutige Module-Identifier (z.B. "crypto", "storage").
	// Darf nicht leer sein. Muss im Bus eindeutig sein.
	ID string

	// Channels listet die Channel-Präfixe auf, die dieses Module besitzt.
	// Example: []string{"CRYPTO"} → Module empfängt alle CRYPTO.*-Events.
	Channels []string

	// Context ist der Parent-Context für die Module-Lebensdauer.
	// Wenn nil, wird context.Background() verwendet.
	Context context.Context

	// DebugLogging aktiviert verbose Handler-Entry/Exit-Logs.
	DebugLogging bool
}

// ─── ModuleFactory ────────────────────────────────────────────────────────────

// ModuleFactory ist das Interface zum Erzeugen von kernel.Module-Instanzen aus
// einem ModuleConfig. Dieses Interface zu implementieren erlaubt es, Module generisch
// zu konstruieren und zu registrieren, ohne ihren konkreten Typ zu kennen.
type ModuleFactory interface {
	Create(cfg ModuleConfig) (Module, error)
}

// FactoryFunc ist ein Function-Adapter, der ModuleFactory implementiert.
// Verwende das, um eine einfache Constructor-Funktion in eine ModuleFactory
// zu verwandeln.
type FactoryFunc func(cfg ModuleConfig) (Module, error)

func (f FactoryFunc) Create(cfg ModuleConfig) (Module, error) {
	return f(cfg)
}

// ─── BaseModule ───────────────────────────────────────────────────────────────

// BaseModule ist eine Default-Implementierung der ID()- und Channels()-Methoden
// des kernel.Module-Interfaces. Embed das in dein Module-Struct, um Boilerplate
// zu vermeiden und konsistentes ID/Channels-Verhalten zu garantieren.
//
// Module MÜSSEN Handle(), Start() und Stop() trotzdem selbst implementieren.
type BaseModule struct {
	id       string
	channels []string
}

// NewBaseModule erzeugt ein BaseModule aus einem ModuleConfig.
func NewBaseModule(cfg ModuleConfig) BaseModule {
	return BaseModule{id: cfg.ID, channels: cfg.Channels}
}

func (b *BaseModule) ID() string { return b.id }

func (b *BaseModule) Channels() []string { return b.channels }
