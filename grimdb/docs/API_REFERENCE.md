# Grimlocker Omega+ — API Reference

> Complete reference for all exported types, interfaces, structs, and their methods.  
> Use `Ctrl+F` / `Cmd+F` to search by type name or method name.  
> For error codes: see [ERROR_CODES.md](ERROR_CODES.md)  
> For architecture diagrams: see [ARCHITECTURE.md](ARCHITECTURE.md)

---

## Table of Contents

- [Package: kernel](#package-kernel)
- [Package: security](#package-security)
- [Package: crypto](#package-crypto)
- [Package: storage](#package-storage)
- [Package: errors](#package-errors)
- [Package: provider](#package-provider)
- [Package: config/single](#package-configsingle)
- [Package: sdk](#package-sdk)

---

## Package: kernel

**Path:** `github.com/grimlocker/grimdb/kernel`  
**Purpose:** Event bus, module contracts, handler patterns.  
No module may import another module — all communication goes through the Dispatcher.

---

### `type Event struct`

The unit of communication between all modules. Payloads are JSON.

```go
type Event struct {
    ID        string    // UUID v4 — correlates requests and responses
    Type      EventType // e.g. "CRYPTO.ENCRYPT", "AUTH.UNLOCK"
    Payload   []byte    // JSON-encoded data (schema per EventType)
    ReplyTo   string    // set on response events: the originating Event.ID
    Origin    string    // module ID that dispatched this event
    Timestamp int64     // Unix nanoseconds
    TTL       int       // hop count; bus drops events at 0 (default: 8)
}
```

**Methods:**

| Method | Description |
|--------|-------------|
| *(none — plain struct)* | Use `NewEvent()` / `ReplyEvent()` to construct |

**Constructor functions:**

| Function | Signature | Description |
|----------|-----------|-------------|
| `NewEvent` | `(origin string, t EventType, payload []byte) Event` | Creates event with fresh UUID + current timestamp |
| `ReplyEvent` | `(origin string, t EventType, req Event, payload []byte) Event` | Creates response with ReplyTo = req.ID |

---

### `type EventType string`

A typed string constant identifying the channel and action.  
Format: `CHANNEL.ACTION` (e.g. `"CRYPTO.ENCRYPT"`).  
The prefix before `.` is the routing channel used by the bus.

**Method:**

| Method | Signature | Description |
|--------|-----------|-------------|
| `Channel` | `() string` | Returns channel prefix, e.g. `"CRYPTO"` from `"CRYPTO.ENCRYPT"` |

**All EventType constants** (defined in `kernel/event.go`):

| Constant | Value | Owner |
|----------|-------|-------|
| `EvAuthSetup` | `AUTH.SETUP` | security.Module |
| `EvAuthUnlock` | `AUTH.UNLOCK` | security.Module |
| `EvAuthResult` | `AUTH.RESULT` | security.Module |
| `EvAuthLockdown` | `AUTH.LOCKDOWN` | security.Module |
| `EvAuthLogout` | `AUTH.LOGOUT` | security.Module |
| `EvAuthStatus` | `AUTH.STATUS` | security.Module |
| `EvAuthKeyReady` | `AUTH.KEY_READY` | security.Module → bus.OpenGate |
| `EvAuthReady` | `AUTH.READY` | SessionContext |
| `EvAuthGetHandle` | `AUTH.GET_HANDLE` | security.Module |
| `EvCryptoEncrypt` | `CRYPTO.ENCRYPT` | crypto.Module |
| `EvCryptoDecrypt` | `CRYPTO.DECRYPT` | crypto.Module |
| `EvCryptoDerive` | `CRYPTO.DERIVE_KEY` | crypto.Module |
| `EvCryptoResult` | `CRYPTO.RESULT` | crypto.Module |
| `EvStorageWrite` | `STORAGE.WRITE` | grimdb.Adapter |
| `EvStorageRead` | `STORAGE.READ` | grimdb.Adapter |
| `EvStorageDelete` | `STORAGE.DELETE` | grimdb.Adapter |
| `EvStorageList` | `STORAGE.LIST` | grimdb.Adapter |
| `EvStorageResult` | `STORAGE.RESULT` | grimdb.Adapter |
| `EvEntryCreate` | `ENTRY.CREATE` | storage.EntryHandler |
| `EvEntryRead` | `ENTRY.READ` | storage.EntryHandler |
| `EvEntryUpdate` | `ENTRY.UPDATE` | storage.EntryHandler |
| `EvEntryDelete` | `ENTRY.DELETE` | storage.EntryHandler |
| `EvEntryQuery` | `ENTRY.QUERY` | storage.EntryHandler |
| `EvEntryResult` | `ENTRY.RESULT` | storage.EntryHandler |
| `EvToolSSHGen` | `TOOL.SSH_GEN` | tools.Module |
| `EvToolResult` | `TOOL.RESULT` | tools.Module |
| `EvSecAudit` | `SECURITY.AUDIT` | security.Module |
| `EvSecPanic` | `SECURITY.PANIC` | security.Module |
| `EvSecLockdown` | `SECURITY.LOCKDOWN` | security.Module |
| `EvWorkspaceCreate` | `WORKSPACE.CREATE` | — |
| `EvWorkspaceSwitch` | `WORKSPACE.SWITCH` | — |
| `EvWorkspaceDelete` | `WORKSPACE.DELETE` | — |

---

### `type Handler func(Event) error`

The function signature for all bus handlers.  
A non-nil error is logged by the bus; it does not stop event delivery.  
Use `HandlerBuilder` to add panic recovery and logging as decorators.

---

### `type Dispatcher interface`

**File:** `kernel/dispatcher.go`

The sole communication interface between modules. Every module receives a
Dispatcher in its `Start(ctx, d)` call and uses it to send events.

| Method | Signature | Description |
|--------|-----------|-------------|
| `Dispatch` | `(e Event) error` | Async — handlers run in goroutines. Returns immediately. |
| `Request` | `(ctx context.Context, e Event) (Event, error)` | Sync — blocks until a reply arrives or ctx is cancelled |
| `Subscribe` | `(eventType EventType, handler Handler) (unsubscribe func())` | Adds a type-specific handler. Returns an unsubscribe closure. |
| `Register` | `(m Module) error` | Adds a Module to the bus (subscribes to its channels) |
| `Unregister` | `(moduleID string)` | Removes a module and all its subscriptions |
| `Shutdown` | `(ctx context.Context) error` | Stops all modules; drains in-flight events |

**Gate methods** (available via type assertion on the concrete `*bus`):

| Method | Description |
|--------|-------------|
| `OpenGate()` | Allows gated channels (STORAGE) to flow — called on AUTH.KEY_READY |
| `CloseGate()` | Blocks gated channels again — called on AUTH.LOGOUT |

---

### `type Module interface`

**File:** `kernel/dispatcher.go`

The contract every module must satisfy to plug into the kernel.

| Method | Signature | Description |
|--------|-----------|-------------|
| `ID` | `() string` | Unique module identifier (e.g. `"crypto"`, `"storage"`) |
| `Channels` | `() []string` | Channel prefixes this module owns (e.g. `["CRYPTO"]`) |
| `Handle` | `(Event) error` | Processes a delivered event. Called per-goroutine. |
| `Start` | `(ctx context.Context, d Dispatcher) error` | Called once before first event delivery |
| `Stop` | `() error` | Called during bus Shutdown |

---

### `type HandlerBuilder struct`

**File:** `kernel/handler.go`

Fluent API for composing Handlers with cross-cutting decorators.

**Constructor:**

| Function | Description |
|----------|-------------|
| `NewHandlerBuilder(h Handler) *HandlerBuilder` | Wraps a base handler |

**Methods:**

| Method | Signature | Description |
|--------|-----------|-------------|
| `WithRecovery` | `(modulePrefix string) *HandlerBuilder` | Catches panics; converts to non-nil error return |
| `WithLogging` | `(modulePrefix string) *HandlerBuilder` | Logs timing and errors on each invocation |
| `WithMetrics` | `(modulePrefix, eventLabel string) *HandlerBuilder` | Emits timing metrics to the standard logger |
| `Build` | `() Handler` | Returns the composed Handler |

**Standalone decorator functions:**

| Function | Description |
|----------|-------------|
| `WithRecovery(prefix string, h Handler) Handler` | One-shot recovery wrapper |
| `WithLogging(prefix string, h Handler) Handler` | One-shot logging wrapper |

**Example:**
```go
h := kernel.NewHandlerBuilder(myFunc).
    WithRecovery("[mymodule]").
    WithLogging("[mymodule]").
    Build()
bus.Subscribe(kernel.EvMyEvent, h)
```

---

### `type ModuleConfig struct`

**File:** `kernel/module_factory.go`

Standard configuration bag passed to every module constructor.

| Field | Type | Description |
|-------|------|-------------|
| `ID` | `string` | Unique module ID (must not be empty) |
| `Channels` | `[]string` | Channel prefixes this module owns |
| `Context` | `context.Context` | Parent context for module lifetime |
| `DebugLogging` | `bool` | Enables verbose handler entry/exit logs |

---

### `type ModuleFactory interface`

**File:** `kernel/module_factory.go`

Generic interface for creating Module instances from a ModuleConfig.

| Method | Signature | Description |
|--------|-----------|-------------|
| `Create` | `(cfg ModuleConfig) (Module, error)` | Constructs and returns a ready-to-register module |

**Adapter:**

| Type | Description |
|------|-------------|
| `FactoryFunc func(ModuleConfig) (Module, error)` | Converts a plain constructor function to ModuleFactory |

---

### `type BaseModule struct`

**File:** `kernel/module_factory.go`

Embed in your module struct to get `ID()` and `Channels()` for free.

| Method | Description |
|--------|-------------|
| `ID() string` | Returns cfg.ID |
| `Channels() []string` | Returns cfg.Channels |

**Constructor:** `NewBaseModule(cfg ModuleConfig) BaseModule`

---

### `type Registry struct`

**File:** `kernel/registry.go`

Ordered module startup helper. Wraps a Dispatcher.

| Method | Signature | Description |
|--------|-----------|-------------|
| `NewRegistry` | `(d Dispatcher) *Registry` | Constructor |
| `Add` | `(m Module) error` | Registers module on the bus; records startup order |
| `StartAll` | `(ctx context.Context) error` | Calls Start on each module in registration order |
| `Bus` | `() Dispatcher` | Returns the underlying Dispatcher |
| `Modules` | `() []Module` | Returns a copy of the registered module list |

---

## Package: security

**Path:** `github.com/grimlocker/grimdb/security`  
**Purpose:** MVK storage in locked memory, lockdown state machine, audit log, session state.

---

### `type Module struct`

**File:** `security/module.go`

The kernel.Module for the `SECURITY` and `AUTH` channels.  
Owns the LockdownManager, AuditLog, MemoryGuard, and the in-memory MVK handle table.  
No other module holds actual key material.

| Method | Signature | Description |
|--------|-----------|-------------|
| `NewModule` | `(cfg LockdownConfig, entropyPath string) *Module` | Constructor |
| `WithExitFunc` | `(f func(int)) *Module` | Override os.Exit for tests |
| `ID` | `() string` | Returns `"security"` |
| `Channels` | `() []string` | Returns `["SECURITY", "AUTH"]` |
| `Start` | `(ctx context.Context, d Dispatcher) error` | Wires dispatcher, builds handler registry |
| `Stop` | `() error` | Zeroises all MVK handles in locked memory |
| `Handle` | `(Event) error` | Dispatches to internal handler registry |
| `SetSession` | `(s *SessionContext)` | Links the module to the global session state |
| `StoreMVK` | `(key []byte) (string, error)` | Allocates locked memory, copies key, returns opaque handle |
| `RetrieveMVK` | `(handle string) ([]byte, bool)` | Returns key bytes for handle — do NOT hold past current frame |
| `RevokeMVK` | `(handle string)` | Zeroes and frees the key for the given handle |
| `Lockdown` | `() LockdownManager` | Returns the current lockdown manager |
| `Audit` | `() AuditLog` | Returns the audit log |

**Handled Events:**

| EventType | Action |
|-----------|--------|
| `AUTH.UNLOCK` | no-op (handled by daemon subscription) |
| `AUTH.STATUS` | replies with lockdown state + remaining attempts |
| `AUTH.LOGOUT` | replies locked=true; gate closure handled by daemon |
| `AUTH.SETUP` | replies ready=true |
| `AUTH.GET_HANDLE` | replies with current MVK handle |
| `AUTH.LOCKDOWN` | triggers hard lockdown |
| `SECURITY.AUDIT` | appends SecurityEvent to audit log |
| `SECURITY.PANIC` | triggers hard lockdown |
| `SECURITY.LOCKDOWN` | triggers hard lockdown |

---

### `type MVKStore interface`

**File:** `security/mvk_store.go`

Contract for secure key storage in locked (non-swappable) memory.

| Method | Signature | Description |
|--------|-----------|-------------|
| `Store` | `(key []byte) (handle string, err error)` | Allocate locked memory, copy key, return opaque handle |
| `Retrieve` | `(handle string) (key []byte, ok bool)` | Return key pointer — must not be held past current frame |
| `Revoke` | `(handle string)` | Zeroise and free key for handle |
| `RevokeAll` | `()` | Zeroise and free ALL handles — called on shutdown / lockdown |
| `Handles` | `() []string` | Returns active handle list (for audit; never returns key bytes) |

**Constructor:** `NewLockedMVKStore() MVKStore`

---

### `type LockdownManager interface`

**File:** `security/lockdown.go`

State machine for progressive authentication lockout.

| Method | Signature | Description |
|--------|-----------|-------------|
| `RecordFailure` | `() (LockdownState, error)` | Records a failed attempt; may transition to Soft/Hard |
| `RecordSuccess` | `()` | Resets failure counter and lockdown state |
| `State` | `() LockdownState` | Returns current state (auto-expires Soft lockdown) |
| `RemainingAttempts` | `() int` | Attempts left before next state transition |
| `LockdownUntil` | `() time.Time` | When Soft lockdown expires (zero if None/Hard) |
| `TriggerHard` | `() error` | Immediately transitions to Hard; invokes OnHard callback |

**States:**

| Constant | Value | Meaning |
|----------|-------|---------|
| `LockdownNone` | 0 | Normal operation |
| `LockdownSoft` | 1 | Threshold exceeded; timed lockout, limited overrides |
| `LockdownHard` | 2 | All keys zeroed; daemon exits |

**Constructor:** `NewLockdownManager(cfg LockdownConfig) LockdownManager`

**`type LockdownConfig struct`:**

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `Threshold` | `int` | 3 | Failed attempts before soft lockdown |
| `MaxOverrides` | `int` | 4 | Override attempts during soft lockdown |
| `LockdownMinutes` | `int` | 200 | Soft lockdown duration in minutes |
| `OnHard` | `func()` | — | Callback invoked on hard lockdown (zeroise + exit) |

---

### `type SessionContext struct`

**File:** `security/session.go`

Global vault-unlock state. Thread-safe via RWMutex.

| Method | Signature | Description |
|--------|-----------|-------------|
| `NewSessionContext` | `() *SessionContext` | Constructor — starts locked |
| `SetDispatcher` | `(d kernel.Dispatcher)` | Injects bus for emitting AUTH.READY / AUTH.LOGOUT events |
| `Unlock` | `(mvkHandle string)` | Marks session active; emits AUTH.READY |
| `Lock` | `()` | Clears session; emits AUTH.LOGOUT |
| `IsUnlocked` | `() bool` | True if vault is currently unlocked |
| `MVKHandle` | `() string` | Returns active handle or `""` |
| `ActiveHandle` | `() string` | Returns handle only if active (atomic read) |
| `RequireUnlocked` | `() error` | Returns nil if unlocked, error if locked |
| `SessionDestroy` | `()` | Called during graceful shutdown to zero session state |
| `Health` | `() map[string]interface{}` | Returns JSON-serialisable health check |

---

### `type AuditLog interface`

**File:** `security/audit.go`

Append-only, hash-chained in-memory log of security events.

| Method | Signature | Description |
|--------|-----------|-------------|
| `Append` | `(e SecurityEvent)` | Appends an event; updates SHA-256 chain |
| `Entries` | `() []SecurityEvent` | Returns a snapshot of all entries |
| `Len` | `() int` | Returns current entry count |

**`type SecurityEvent struct`:**

| Field | Type | Description |
|-------|------|-------------|
| `Level` | `string` | `"info"`, `"warn"`, or `"critical"` |
| `Module` | `string` | Originating module ID |
| `Message` | `string` | Human-readable description |
| `SubjectID` | `string` | User/session identifier (optional) |
| `Timestamp` | `int64` | Unix nanoseconds |
| `ChainHash` | `[32]byte` | SHA-256 of previous entry + this entry |

**Level constants:** `LevelInfo`, `LevelWarn`, `LevelCritical`

**Constructor:** `NewAuditLog(capacity int) AuditLog`

---

### `type MemoryGuard interface`

**File:** `security/memlock.go`

OS-level memory locking to prevent key material from being swapped to disk.

| Method | Signature | Description |
|--------|-----------|-------------|
| `AllocLocked` | `(n int) ([]byte, error)` | Allocates n bytes of locked (mlock'd) memory |
| `Unlock` | `(b []byte) error` | Releases the OS lock on the slice |
| `Zeroize` | `(b []byte)` | Overwrites the slice with zeros |

**Platform implementations:** `security/memlock_unix.go`, `security/memlock_windows.go`

---

## Package: crypto

**Path:** `github.com/grimlocker/grimdb/crypto`  
**Purpose:** Owns the CRYPTO event channel; all cryptographic operations route through here.

---

### `type Module struct`

**File:** `crypto/module.go`

Kernel.Module for `CRYPTO.*` events. Holds no key material — fetches keys via KeyResolver.

| Method | Signature | Description |
|--------|-----------|-------------|
| `NewModule` | `(p Provider, kr KeyResolver) *Module` | Constructor |
| `ID` | `() string` | Returns `"crypto"` |
| `Channels` | `() []string` | Returns `["CRYPTO"]` |
| `Start` | `(ctx context.Context, d Dispatcher) error` | Wires dispatcher + builds HandlerRegistry |
| `Stop` | `() error` | No-op |
| `Handle` | `(Event) error` | Dispatches via registry; logs unknown events as DEBUG |

**`type KeyResolver func(handle string) ([]byte, bool)`**  
Injected function that fetches raw key bytes from security.Module.  
Provided during daemon startup: `secMod.RetrieveMVK`.

**Handled Events:**

| EventType | Payload Schema | Result |
|-----------|---------------|--------|
| `CRYPTO.ENCRYPT` | `{key_handle, plaintext, aad?}` | `CRYPTO.RESULT{data: nonce+ciphertext}` |
| `CRYPTO.DECRYPT` | `{key_handle, ciphertext, nonce, aad?}` | `CRYPTO.RESULT{data: plaintext}` |
| `CRYPTO.DERIVE_KEY` | `{password, salt, opts}` | `CRYPTO.RESULT{data: derived_key}` |

All results include `error_code` (from `*errors.GrimlockError`) on failure.

---

### `type HandlerRegistry struct`

**File:** `crypto/handler_registry.go`

Type-safe event handler registry with pre-validation.

| Method | Signature | Description |
|--------|-----------|-------------|
| `NewHandlerRegistry` | `() *HandlerRegistry` | Constructor |
| `Register` | `(eventType EventType, validator PayloadValidator, handler eventHandlerFn) error` | Adds handler; errors on duplicate |
| `MustRegister` | `(eventType EventType, validator PayloadValidator, handler eventHandlerFn)` | Like Register but panics on error |
| `Noop` | `(eventTypes ...EventType)` | Registers no-op handlers to silence debug logs |
| `Dispatch` | `(e Event) error` | Validates payload then calls handler; nil for unknown events |
| `EventTypes` | `() []EventType` | Returns all registered event types (for testing) |

---

### `type PayloadValidator interface`

**File:** `crypto/handler_registry.go`

Pre-validates event payloads before the handler runs.

| Method | Signature | Description |
|--------|-----------|-------------|
| `Validate` | `(payload []byte) error` | Returns error to reject the event; nil to proceed |

**Implementations:**

| Type | Description |
|------|-------------|
| `ValidatorFunc func([]byte) error` | Function adapter |
| `JSONSchemaValidator[T any](check func(*T) error)` | Generic: unmarshal + check |

---

### `type Provider interface`

**File:** `crypto/interface.go`

Core cryptographic operations (implemented by pure-Go provider or Rust enclave).

| Method | Signature | Description |
|--------|-----------|-------------|
| `NewNonce` | `() ([12]byte, error)` | Generate random 12-byte nonce for ChaCha20-Poly1305 |
| `Encrypt` | `(key, nonce, plaintext, aad []byte) ([]byte, error)` | ChaCha20-Poly1305 Seal |
| `Decrypt` | `(key, nonce, ciphertext, aad []byte) ([]byte, error)` | ChaCha20-Poly1305 Open |
| `DeriveArgon2id` | `(password []byte, opts KDFOptions) ([]byte, error)` | Argon2id key derivation |
| `GenerateEntropyFileWithProgress` | `(path string, progress func(float64, string)) error` | Generate entropy file |

---

## Package: storage

**Path:** `github.com/grimlocker/grimdb/storage`  
**Purpose:** Block-level storage interfaces, VaultEntry types, entry CRUD handler.

---

### `type BlockStore interface`

**File:** `storage/interface.go`

The interface every storage backend must implement. Backends never decrypt — they store opaque Blocks.

| Method | Signature | Description |
|--------|-----------|-------------|
| `WriteBlock` | `(b Block) error` | Persist block (append-only semantics) |
| `ReadBlock` | `(id string) (Block, error)` | Retrieve block by ID |
| `DeleteBlock` | `(id string) error` | Secure-delete block (zero on disk + remove from index) |
| `ListBlocks` | `() ([]BlockMeta, error)` | All block metadata from in-memory index |
| `QueryBlocks` | `(category Category) ([]BlockMeta, error)` | Filtered by category (empty = all) |
| `Flush` | `() error` | Atomically persist the in-memory index |
| `Close` | `() error` | Flush + cleanup |

---

### `type BlockStoreV2 interface`

**File:** `storage/blockstore_v2.go`

Extends BlockStore with transaction support.

| Method | Signature | Description |
|--------|-----------|-------------|
| `BeginWrite` | `() (WriteTransaction, error)` | Start a buffered write transaction |
| `BeginRead` | `() (ReadTransaction, error)` | Start a consistent read snapshot |

---

### `type WriteTransaction interface`

**File:** `storage/blockstore_v2.go`

Buffers writes and applies them atomically on Commit.

| Method | Signature | Description |
|--------|-----------|-------------|
| `WriteBlock` | `(b Block) error` | Stage a block write |
| `DeleteBlock` | `(id string) error` | Stage a block deletion |
| `Commit` | `() error` | Apply all staged operations + Flush |
| `Rollback` | `()` | Discard all staged operations |

**Compatibility shim:** `NewInMemoryWriteTransaction(store BlockStore) *InMemoryWriteTransaction`  
Wraps any BlockStore to provide WriteTransaction semantics without requiring BlockStoreV2.

---

### `type ReadTransaction interface`

**File:** `storage/blockstore_v2.go`

Consistent snapshot view of the store.

| Method | Signature | Description |
|--------|-----------|-------------|
| `ReadBlock` | `(id string) (Block, error)` | Read from snapshot |
| `ListBlocks` | `() ([]BlockMeta, error)` | All metadata from snapshot |
| `QueryBlocks` | `(category Category) ([]BlockMeta, error)` | Filtered from snapshot |
| `Close` | `()` | Release the snapshot |

---

### `type Block struct`

**File:** `storage/block.go`

The unit of storage. Data is always ciphertext on disk; the entry module stores plaintext VaultEntry JSON.

| Field | Type | Description |
|-------|------|-------------|
| `ID` | `string` | UUID v4 identifier |
| `Nonce` | `[]byte` | 12-byte ChaCha20-Poly1305 nonce |
| `HMAC` | `[]byte` | 32-byte MAC over (id ‖ nonce ‖ data) |
| `Data` | `[]byte` | Ciphertext or plaintext (strategy-dependent) |
| `Category` | `Category` | Routing category (PASSWORD, SSH_KEY, etc.) |
| `CreatedAt` | `int64` | Unix nanoseconds |
| `UpdatedAt` | `int64` | Unix nanoseconds |

---

### `type BlockMeta struct`

**File:** `storage/block.go`

Lightweight metadata returned by ListBlocks/QueryBlocks (no data bytes).

| Field | Type | Description |
|-------|------|-------------|
| `ID` | `string` | Block identifier |
| `Size` | `int64` | Data length in bytes |
| `Category` | `Category` | Entry category |
| `CreatedAt` | `int64` | Unix nanoseconds |
| `UpdatedAt` | `int64` | Unix nanoseconds |

---

### `type VaultEntry struct`

**File:** `storage/entry.go`

The structured payload stored in Block.Data by the EntryHandler.

| Field | Type | Description |
|-------|------|-------------|
| `ID` | `string` | UUID — same as the parent Block.ID |
| `Title` | `string` | Human-readable title |
| `Category` | `Category` | Entry category (PASSWORD, SSH_KEY, …) |
| `Type` | `string` | Legacy type string (maps to Category) |
| `Fields` | `map[string]string` | Key-value pairs (e.g. username, password) |
| `SubjectID` | `string` | Owner identifier (defaults to `"default"`) |
| `CreatedAt` | `int64` | Unix nanoseconds |
| `UpdatedAt` | `int64` | Unix nanoseconds |

**Category constants:** `CategoryPassword`, `CategorySSHKey`, `CategoryCertificate`, `CategoryFileVault`

---

### `type EntryHandler struct`

**File:** `storage/entry_module.go`

High-level CRUD handler for `ENTRY.*` events. Wired as a direct subscription, not a Module.

| Method | Signature | Description |
|--------|-----------|-------------|
| `NewEntryHandler` | `(bs BlockStore) *EntryHandler` | Constructor |
| `SetDispatcher` | `(d kernel.Dispatcher)` | Must be called before first event |
| `Handle` | `(e kernel.Event) error` | Dispatches to internal handler registry |

**Error replies** include `error_code` (GrimlockError code) for structured client handling.

---

### `type StorageStrategy interface`

**File:** `storage/interface.go`

Pluggable interceptor for block read/write (e.g. honeypot, deniable encryption).

| Method | Signature | Description |
|--------|-----------|-------------|
| `Name` | `() string` | Strategy identifier |
| `OnWrite` | `(b Block) (Block, error)` | Transform block before writing |
| `OnRead` | `(b Block) (Block, error)` | Transform block after reading |
| `OnTrigger` | `(key string) error` | Handle a strategy trigger event |

**Built-in:** `NopStrategy{}` — passes blocks through unchanged.

---

## Package: errors

**Path:** `github.com/grimlocker/grimdb/errors`  
**Purpose:** Unified typed error system with codes, context, stacktraces, and HTTP mapping.

---

### `type GrimlockError struct`

**File:** `errors/types.go`

The single error type returned by all Grimlocker modules.

| Field | Type | Description |
|-------|------|-------------|
| `Code` | `int` | Numeric error code (see ranges below) |
| `Message` | `string` | Short human-readable description |
| `Ctx` | `ErrorContext` | Structured diagnostic context |
| `Stack` | `[]StackFrame` | Call stack at creation (only for critical errors) |
| `Cause` | `error` | Wrapped underlying error (accessible via errors.Unwrap) |
| `Timestamp` | `int64` | Unix nanoseconds at creation |
| `ModuleID` | `string` | Which module produced the error |
| `EventType` | `string` | Bus event type during which error occurred |

**Methods:**

| Method | Signature | Description |
|--------|-----------|-------------|
| `Error` | `() string` | Implements error interface: `[code] message: cause` |
| `Unwrap` | `() error` | Returns Cause for errors.Is / errors.As |
| `Is` | `(target error) bool` | Matches GrimlockErrors with the same Code |
| `HTTPStatus` | `() int` | Maps error code to HTTP status (400, 401, 422, 423, 500…) |
| `MarshalJSON` | `() ([]byte, error)` | JSON without exposing raw cause chain |
| `WithDetails` | `(key string, value interface{}) *GrimlockError` | Add context detail (chainable) |
| `WithModule` | `(moduleID string) *GrimlockError` | Set ModuleID (chainable) |
| `WithEvent` | `(eventType string) *GrimlockError` | Set EventType (chainable) |
| `Log` | `(logger StructuredLogger)` | Write to logger with all context fields |

**`type ErrorContext struct`:**

| Field | Type | Description |
|-------|------|-------------|
| `BlockID` | `string` | Affected GrimDB block ID |
| `Operation` | `string` | Low-level operation (e.g. `"read_block_data"`) |
| `Details` | `map[string]string` | Additional key-value diagnostic info |

**Constructor functions** (create and return *GrimlockError):

| Function | Code | Stack? |
|----------|------|--------|
| `NewVaultLockedError()` | 1001 | No |
| `NewVaultNotInitializedError()` | 1002 | No |
| `NewAuthInvalidError(op string, cause error)` | 1003 | Yes |
| `NewAuthLockdownError(remaining int)` | 1005 | No |
| `NewStorageIOError(op, blockID string, cause error)` | 2001 | Yes |
| `NewStorageCorruptionError(op, blockID string, details map[string]string)` | 2002 | Yes |
| `NewStorageNotFoundError(blockID string)` | 2003 | No |
| `NewStorageIndexError(op string, cause error)` | 2005 | Yes |
| `NewCryptoDecryptionError(blockID string, cause error)` | 3003 | Yes |
| `NewCryptoEncryptionError(op string, cause error)` | 3002 | Yes |
| `NewCryptoKeyDerivationError(op string, cause error)` | 3001 | Yes |
| `NewCryptoInvalidKeyError(gotBytes int)` | 3004 | Yes |
| `NewCryptoHandleUnknownError(handle string)` | 3006 | No |
| `NewSecurityMemlockError(cause error)` | 4001 | Yes |
| `NewSecurityLockdownError(reason string, details map[string]string)` | 4002 | Yes |
| `NewSecurityMVKMissingError(op string)` | 4005 | No |
| `NewBusTimeoutError(eventType string)` | 5002 | No |
| `NewBusShutdownError()` | 5001 | No |
| `NewBusGatedError(eventType, channel string)` | 5003 | No |
| `NewProtocolError(op string, cause error)` | 6001 | Yes |

**Utility functions:**

| Function | Description |
|----------|-------------|
| `Wrap(code int, msg string, err error) *GrimlockError` | Wraps plain error; no-op if already GrimlockError |
| `Sentinel(code int, msg string) *GrimlockError` | Value-type for errors.Is comparisons |

---

### `type StructuredLogger interface`

**File:** `errors/logging.go`

Standard logging contract used by all modules.

| Method | Signature | Description |
|--------|-----------|-------------|
| `Debug` | `(msg string, fields map[string]any)` | Debug-level (disabled by default in StdLogger) |
| `Info` | `(msg string, fields map[string]any)` | Informational |
| `Warn` | `(msg string, fields map[string]any)` | Warning |
| `Error` | `(msg string, err error, fields map[string]any)` | Error with optional cause |
| `Fatal` | `(msg string, err error, fields map[string]any)` | Logs and calls log.Fatal |

**`type StdLogger struct`** (implements StructuredLogger):

| Constructor | Description |
|-------------|-------------|
| `NewStdLogger(prefix string) *StdLogger` | Standard logger (debug disabled) |
| `NewDebugLogger(prefix string) *StdLogger` | Standard logger with debug enabled |

---

### `type StackFrame struct`

**File:** `errors/stacktrace.go`

A single call-stack frame captured by CaptureStacktrace.

| Field | Type | Description |
|-------|------|-------------|
| `File` | `string` | Source file path |
| `Line` | `int` | Line number |
| `Function` | `string` | Fully qualified function name |

**Function:** `CaptureStacktrace(skip int) []StackFrame`  
Skips `skip` additional frames above the caller (`skip=1` from error constructors).

---

## Package: provider

**Path:** `github.com/grimlocker/grimdb/provider`  
**Purpose:** Tier-agnostic interfaces that decouple the daemon from concrete implementations.

---

### `type VaultProvider interface`

**File:** `provider/interfaces.go`

Single entry-point the daemon receives at startup. The daemon must not import `config/single` or `config/enterprise` directly.

| Method | Signature | Description |
|--------|-----------|-------------|
| `Auth` | `() AuthProvider` | Authentication handler for this tier |
| `Storage` | `() StorageProvider` | Storage backend for this tier |
| `Crypto` | `() crypto.Provider` | Crypto implementation for this tier |
| `Tier` | `() string` | `"single"` or `"enterprise"` |
| `KernelModules` | `() []kernel.Module` | Modules to register: [security, crypto, storage adapter] |

---

### `type AuthProvider interface`

**File:** `provider/interfaces.go`

Authentication logic for a specific tier.

| Method | Signature | Description |
|--------|-----------|-------------|
| `HandleUnlockEvent` | `(bus, sessionCtx, onSessionKey) kernel.Handler` | Returns the 7-step unlock handler |
| `StoreMVK` | `(key []byte) (string, error)` | Delegates to security.Module |
| `RetrieveMVK` | `(handle string) ([]byte, bool)` | Delegates to security.Module |
| `RevokeMVK` | `(handle string)` | Delegates to security.Module |
| `Lockdown` | `() security.LockdownManager` | Access to lockdown state |
| `AuditLog` | `() security.AuditLog` | Access to audit log |
| `Tier` | `() string` | `"local-argon2id"` or `"oidc-jwt"` |

**Implementations:**

| Type | File | Description |
|------|------|-------------|
| `single.LocalAuth` | `config/single/auth.go` | Argon2id password → MVK |
| `enterprise.OIDCProvider` | `config/enterprise/auth.go` | JWT RS256 → MVK (build tag: enterprise) |

---

### `type StorageProvider interface`

**File:** `provider/interfaces.go`

Storage backend for a specific tier. Embeds BlockStore.

| Method | Signature | Description |
|--------|-----------|-------------|
| `SetMVKFunc` | `(fn func() []byte)` | Wire key retrieval after unlock |
| `LoadIndex` | `() error` | Load block index after unlock |
| `KernelModule` | `() kernel.Module` | The storage adapter (for bus registration) |
| *(+ all BlockStore methods)* | | WriteBlock, ReadBlock, DeleteBlock, ListBlocks, Flush, Close |

**Implementations:**

| Type | File | Description |
|------|------|-------------|
| `grimdb.BlockStoreImpl` | `storage/grimdb/blockstore.go` | File-backed encrypted store |
| `remote.RemoteVault` | `storage/remote/vault.go` | S3/MinIO backend (enterprise only) |

---

## Package: config/single

**Path:** `github.com/grimlocker/grimdb/config/single`  
**Build tag:** `!enterprise` (excluded from enterprise build)  
**Purpose:** Single-user tier — Argon2id password auth + local file storage.

---

### `type LocalAuth struct`

**File:** `config/single/auth.go`

Implements `provider.AuthProvider` for single-user tier.

| Method | Description |
|--------|-------------|
| `NewLocalAuth(secMod, blockStore, appDir)` | Constructor |
| `HandleUnlockEvent(...)` | 7-step Argon2id unlock flow |
| `StoreMVK / RetrieveMVK / RevokeMVK` | Delegate to security.Module |
| `Lockdown() / AuditLog()` | Delegate to security.Module |
| `Tier() string` | Returns `"local-argon2id"` |

---

## Package: sdk

**Path:** `github.com/grimlocker/grimdb/sdk`  
**Purpose:** Plugin interfaces for extending Grimlocker with external modules.

---

### `type Plugin interface`

**File:** `sdk/plugin.go`

Base interface for all SDK plugins.

| Method | Signature | Description |
|--------|-----------|-------------|
| `Name` | `() string` | Plugin identifier |
| `Version` | `() string` | Semantic version |
| `Init` | `(dispatcher sdk.Dispatcher) error` | Called once at plugin startup |
| `Shutdown` | `() error` | Called during daemon shutdown |

---

### `type BiometricPlugin interface`

**File:** `sdk/biometric_interface.go`

Hardware biometric sensor integration.

| Method | Signature | Description |
|--------|-----------|-------------|
| `Authenticate` | `(ctx context.Context) (subjectID string, err error)` | Perform biometric authentication |
| `IsAvailable` | `() bool` | True if hardware is present and ready |

---

*API Reference generated from codebase — update when adding new exported types or changing method signatures.*
