package sdk

// Logger is a minimal structured logger provided to plugins.
type Logger interface {
	Info(msg string, args ...interface{})
	Warn(msg string, args ...interface{})
	Error(msg string, args ...interface{})
}

// Plugin is the interface every Grimlocker Omega plugin must implement.
type Plugin interface {
	// ID returns a unique, stable identifier (e.g. "com.example.my-plugin").
	ID() string

	// Name returns a human-readable display name.
	Name() string

	// Version returns the plugin version string (semver recommended).
	Version() string

	// Init is called once during daemon startup.
	// d is the restricted SDK dispatcher; log is the provided logger.
	Init(d Dispatcher, log Logger) error

	// StorageStrategy returns a custom storage strategy to inject into the
	// BlockStore. Return nil if the plugin does not provide one.
	StorageStrategy() StorageStrategy

	// Shutdown is called during graceful daemon shutdown.
	Shutdown() error
}
