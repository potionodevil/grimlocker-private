package security

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"sync"

	"github.com/grimlocker/grimdb/crypto"
	"github.com/grimlocker/grimdb/kernel"
)

const moduleID = "security"

// secHandlerFn is the internal handler function type for the security module registry.
type secHandlerFn func(kernel.Event) error

// Module implements kernel.Module for the SECURITY and AUTH channels.
// It owns the LockdownManager, AuditLog, MemoryGuard, and the in-memory
// MVK handle table. No other module holds actual key material.
type Module struct {
	mu       sync.RWMutex
	lockdown LockdownManager
	audit    AuditLog
	guard    MemoryGuard

	// mvkHandles maps opaque handle string → locked key bytes.
	mvkHandles map[string][]byte

	entropyPath string
	dispatcher  kernel.Dispatcher
	session     *SessionContext
	handlers    map[kernel.EventType]secHandlerFn

	// exitFunc is called at the end of a hard lockdown. Defaults to os.Exit.
	// Override in tests to prevent the process from terminating.
	exitFunc func(code int)
}

// NewModule creates the security module. entropyPath is the path to the
// entropy file that must be overwritten on hard lockdown.
func NewModule(cfg LockdownConfig, entropyPath string) *Module {
	m := &Module{
		audit:       NewAuditLog(1024),
		guard:       NewMemoryGuard(),
		mvkHandles:  make(map[string][]byte),
		entropyPath: entropyPath,
		exitFunc:    os.Exit,
	}
	cfg.OnHard = m.hardLockdownCallback
	m.lockdown = NewLockdownManager(cfg)
	return m
}

// WithExitFunc overrides the exit function called at the end of hard lockdown.
// Use in tests to prevent os.Exit from terminating the test process.
func (m *Module) WithExitFunc(f func(int)) *Module {
	m.exitFunc = f
	return m
}

func (m *Module) ID() string         { return moduleID }
func (m *Module) Channels() []string { return []string{"SECURITY", "AUTH"} }

// SetSession links the module to the global SessionContext.
func (m *Module) SetSession(s *SessionContext) {
	m.session = s
}

func (m *Module) Start(ctx context.Context, d kernel.Dispatcher) error {
	m.dispatcher = d
	if m.session != nil {
		m.session.SetDispatcher(d)
	}
	m.handlers = m.buildHandlers()
	m.audit.Append(SecurityEvent{Level: LevelInfo, Module: moduleID, Message: "security module started"})
	return nil
}

func (m *Module) Stop() error {
	m.mu.Lock()
	for handle, key := range m.mvkHandles {
		m.guard.Zeroize(key)
		_ = m.guard.Unlock(key)
		delete(m.mvkHandles, handle)
	}
	m.mu.Unlock()
	return nil
}

// buildHandlers returns the static handler registry for all SECURITY.* and AUTH.* events.
// No-op cases are registered explicitly so cross-channel events never produce
// spurious error logs — they silently return nil.
func (m *Module) buildHandlers() map[kernel.EventType]secHandlerFn {
	noop := func(kernel.Event) error { return nil }
	return map[kernel.EventType]secHandlerFn{
		// SECURITY channel
		kernel.EvSecAudit: m.handleSecAudit,
		kernel.EvSecPanic: func(e kernel.Event) error {
			m.audit.Append(SecurityEvent{Level: LevelCritical, Module: moduleID, Message: "PANIC event received — triggering hard lockdown"})
			return m.lockdown.TriggerHard()
		},
		kernel.EvSecLockdown: func(e kernel.Event) error { return m.lockdown.TriggerHard() },
		kernel.EvSecMemLock:  noop,
		kernel.EvSecZeroize:  noop,

		// AUTH channel — active handlers
		kernel.EvAuthStatus:    m.handleAuthStatus,
		kernel.EvAuthSetup:     m.handleAuthSetup,
		kernel.EvAuthGetHandle: m.handleAuthGetHandle,
		kernel.EvAuthLockdown:  func(e kernel.Event) error { return m.lockdown.TriggerHard() },

		// AUTH channel — no-ops (consumed by other modules or bus subscriptions)
		kernel.EvAuthResult:    noop, // consumed by entry handler, translator
		kernel.EvAuthInitReady: noop, // watchdog startup check
		kernel.EvAuthUnlock:    noop, // handled by daemon/main.go bus.Subscribe
		kernel.EvAuthReady:     noop, // emitted by SessionContext.Unlock
		kernel.EvAuthLogout:    m.handleAuthLogout,
		kernel.EvAuthKeyReady:  noop, // emitted after unlock+index load
	}
}

// handleSecAudit appends a SecurityEvent to the audit log.
func (m *Module) handleSecAudit(e kernel.Event) error {
	var ev SecurityEvent
	if err := json.Unmarshal(e.Payload, &ev); err != nil {
		// Fallback: treat plain-string payloads as simple info messages.
		m.audit.Append(SecurityEvent{Level: LevelInfo, Module: moduleID, Message: string(e.Payload)})
		return nil
	}
	m.audit.Append(ev)
	return nil
}

// Handle dispatches the event to the registered handler, or logs a structured
// debug message for unknown events instead of returning an error.
func (m *Module) Handle(e kernel.Event) error {
	if h, ok := m.handlers[e.Type]; ok {
		return h(e)
	}
	log.Printf("[bus][DEBUG] module=%s no_handler event=%s origin=%s", moduleID, e.Type, e.Origin)
	return nil
}

// StoreMVK stores key material in locked memory and returns an opaque handle.
func (m *Module) StoreMVK(key []byte) (string, error) {
	locked, err := m.guard.AllocLocked(len(key))
	if err != nil {
		return "", fmt.Errorf("alloc locked: %w", err)
	}
	copy(locked, key)

	handle := randomHandle()
	m.mu.Lock()
	m.mvkHandles[handle] = locked
	m.mu.Unlock()
	return handle, nil
}

// RetrieveMVK returns the key bytes for a handle without copying —
// callers MUST NOT hold the returned slice past the current call frame.
func (m *Module) RetrieveMVK(handle string) ([]byte, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	key, ok := m.mvkHandles[handle]
	return key, ok
}

// RevokeMVK zeroises and removes the key for the given handle.
func (m *Module) RevokeMVK(handle string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if key, ok := m.mvkHandles[handle]; ok {
		m.guard.Zeroize(key)
		_ = m.guard.Unlock(key)
		delete(m.mvkHandles, handle)
	}
}

// Lockdown returns the current LockdownManager for read-only state queries.
func (m *Module) Lockdown() LockdownManager { return m.lockdown }

// Audit returns the AuditLog.
func (m *Module) Audit() AuditLog { return m.audit }

func (m *Module) handleAuthStatus(e kernel.Event) error {
	state := m.lockdown.State()
	payload, _ := json.Marshal(map[string]interface{}{
		"lockdown_state":     state,
		"remaining_attempts": m.lockdown.RemainingAttempts(),
		"lockdown_until":     m.lockdown.LockdownUntil().Unix(),
	})
	reply := kernel.ReplyEvent(moduleID, kernel.EvAuthResult, e, payload)
	return m.dispatcher.Dispatch(reply)
}

func (m *Module) handleAuthLogout(e kernel.Event) error {
	// Send ACK reply first so REST API callers don't time out.
	// The STORAGE gate is closed by the subscription in main.go.
	payload, _ := json.Marshal(map[string]interface{}{"locked": true})
	reply := kernel.ReplyEvent(moduleID, kernel.EvAuthResult, e, payload)
	return m.dispatcher.Dispatch(reply)
}

func (m *Module) handleAuthSetup(e kernel.Event) error {
	// AUTH.SETUP is a handshake request from the UI to trigger vault initialization.
	// Reply with auth readiness status.
	payload, _ := json.Marshal(map[string]interface{}{
		"ready": true,
	})
	reply := kernel.ReplyEvent(moduleID, kernel.EvAuthResult, e, payload)
	return m.dispatcher.Dispatch(reply)
}

func (m *Module) handleAuthGetHandle(e kernel.Event) error {
	if m.session == nil {
		reply := kernel.ReplyEvent(moduleID, kernel.EvAuthResult, e, []byte(`{"error":"vault locked"}`))
		return m.dispatcher.Dispatch(reply)
	}
	handle := m.session.ActiveHandle()
	if handle == "" {
		reply := kernel.ReplyEvent(moduleID, kernel.EvAuthResult, e, []byte(`{"error":"vault locked"}`))
		return m.dispatcher.Dispatch(reply)
	}
	payload, _ := json.Marshal(map[string]interface{}{
		"handle": handle,
	})
	reply := kernel.ReplyEvent(moduleID, kernel.EvAuthResult, e, payload)
	return m.dispatcher.Dispatch(reply)
}

func (m *Module) hardLockdownCallback() {
	log.Printf("[security] HARD LOCKDOWN: zeroising all key material")
	m.mu.Lock()
	for handle, key := range m.mvkHandles {
		m.guard.Zeroize(key)
		_ = m.guard.Unlock(key)
		delete(m.mvkHandles, handle)
	}
	m.mu.Unlock()

	overwriteEntropy(m.entropyPath)

	if m.dispatcher != nil {
		ev := kernel.NewEvent(moduleID, kernel.EvSecPanic, []byte(`{"reason":"hard_lockdown"}`))
		_ = m.dispatcher.Dispatch(ev)
	}

	log.Printf("[security] HARD LOCKDOWN: exiting process")
	m.exitFunc(1)
}

func overwriteEntropy(path string) {
	if path == "" {
		return
	}

	fi, err := os.Stat(path)
	if err != nil {
		return
	}
	fileSize := fi.Size()

	f, err := os.OpenFile(path, os.O_WRONLY, 0600)
	if err != nil {
		return
	}
	defer f.Close()

	if err := crypto.Shred(f, fileSize); err != nil {
		log.Printf("[security] entropy shred failed: %v — falling back to zero overwrite", err)
		if _, err := f.Seek(0, io.SeekStart); err != nil {
			return
		}
		buf := make([]byte, 4096)
		written := 0
		total := int(fileSize)
		for written < total {
			n, writeErr := f.Write(buf[:min(len(buf), total-written)])
			if writeErr != nil {
				break
			}
			written += n
		}
	}
	_ = f.Sync()
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func randomHandle() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(fmt.Sprintf("security: CSPRNG failure during handle generation: %v", err))
	}
	return fmt.Sprintf("%x", b)
}
