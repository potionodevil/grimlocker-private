package kernel

import (
	"context"
	"encoding/json"
	"log"
	"time"

	egk "github.com/grimlocker/grimdb/engine/kernel"
)

// Watchdog supervises kernel health, auto-initialization, and VFS mounting.
type Watchdog struct {
	bus        egk.Dispatcher
	registry   *egk.Registry
	appDir     string
	sessionCtx interface {
		IsUnlocked() bool
		Health() map[string]interface{}
	}
	heartbeatTick    time.Duration
	heartbeatTimeout time.Duration
}

// NewWatchdog creates a Watchdog supervisor.
func NewWatchdog(bus egk.Dispatcher, registry *egk.Registry, appDir string) *Watchdog {
	return &Watchdog{
		bus:              bus,
		registry:         registry,
		appDir:           appDir,
		heartbeatTick:    30 * time.Second,
		heartbeatTimeout: 5 * time.Second,
	}
}

// SetSession links the watchdog to the global SessionContext for monitoring.
func (w *Watchdog) SetSession(s interface {
	IsUnlocked() bool
	Health() map[string]interface{}
}) {
	w.sessionCtx = s
}

// Start launches watchdog goroutines (heartbeat, vault-status check, VFS mount, health check).
func (w *Watchdog) Start(ctx context.Context) {
	// Goroutine 1: Heartbeat supervisor
	go w.heartbeatLoop(ctx)

	// Goroutine 2: Startup vault-status check (one-shot)
	go w.checkVaultStatus(ctx)

	// Goroutine 3: VFS auto-mount on AUTH.RESULT
	go w.vfsAutoMountLoop(ctx)

	// Goroutine 4: Health check responder
	go w.healthCheckLoop(ctx)
}

// heartbeatLoop sends a liveness ping every 30s.
func (w *Watchdog) heartbeatLoop(ctx context.Context) {
	ticker := time.NewTicker(w.heartbeatTick)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			log.Printf("[Watchdog] Heartbeat tick — bus liveness check")
			_ = w.bus.Dispatch(egk.NewEvent("watchdog", egk.EvSecAudit, []byte(`{"level":"info","module":"watchdog","message":"heartbeat_tick"}`)))
		}
	}
}

// checkVaultStatus checks if vault is initialized and emits INIT_READY if not.
func (w *Watchdog) checkVaultStatus(ctx context.Context) {
	// Placeholder: dispatch AUTH.INIT_READY after short delay to allow modules to stabilize
	time.Sleep(100 * time.Millisecond)
	initPayload, _ := json.Marshal(map[string]string{"reason": "vault_uninitialized_check"})
	_ = w.bus.Dispatch(egk.NewEvent("watchdog", egk.EvAuthInitReady, initPayload))
}

// vfsAutoMountLoop subscribes to AUTH.RESULT and triggers VFS mount on success.
func (w *Watchdog) vfsAutoMountLoop(ctx context.Context) {
	unsubscribe := w.bus.Subscribe(egk.EvAuthResult, func(ev egk.Event) error {
		var payload struct {
			Success bool `json:"success"`
		}
		if err := json.Unmarshal(ev.Payload, &payload); err != nil || !payload.Success {
			return nil // Ignore failures
		}

		// On AUTH.RESULT success: trigger VFS mount
		vfsMountEv := egk.NewEvent("watchdog", egk.EvStorageVFSMount, []byte(`{"action":"mount"}`))
		_ = w.bus.Dispatch(vfsMountEv)

		// After brief delay (for adapter to load index), signal ready
		go func() {
			time.Sleep(100 * time.Millisecond)
			readyPayload, _ := json.Marshal(map[string]bool{"mounted": true})
			readyEv := egk.NewEvent("watchdog", egk.EvStorageReady, readyPayload)
			_ = w.bus.Dispatch(readyEv)
		}()

		return nil
	})

	// Cleanup on context done
	<-ctx.Done()
	unsubscribe()
}

// healthCheckLoop responds to SYSTEM.HEALTH_CHECK with module status.
func (w *Watchdog) healthCheckLoop(ctx context.Context) {
	unsubscribe := w.bus.Subscribe(egk.EvSystemHealthCheck, func(ev egk.Event) error {
		status := map[string]interface{}{
			"watchdog":  "ok",
			"modules":   len(w.registry.Modules()),
			"timestamp": time.Now().Unix(),
		}
		if w.sessionCtx != nil {
			status["session"] = w.sessionCtx.Health()
		}
		payload, _ := json.Marshal(status)
		reply := egk.ReplyEvent("watchdog", egk.EvSystemLog, ev, payload)
		return w.bus.Dispatch(reply)
	})

	<-ctx.Done()
	unsubscribe()
}
