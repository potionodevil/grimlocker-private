# Grimlocker Architecture

This document describes the complete system architecture of Grimlocker — a zero-trust, enterprise-grade password manager built with a hybrid Go/Rust architecture and a Tauri desktop frontend.

---

## System Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│                        GRIMLOCKER SYSTEM LAYERS                           │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  PRESENTATION LAYER                                              │    │
│  │                                                                  │    │
│  │  ┌──────────────────┐  ┌────────────────────────────────────┐  │    │
│  │  │  Browser / Tauri  │  │  CLI Client (cmd/client/)          │  │    │
│  │  │  window           │  │  - local mode (Unix socket)        │  │    │
│  │  │                   │  │  - remote mode (mTLS)              │  │    │
│  │  │  WebSocket :8374  │  │                                    │  │    │
│  │  │  HTTP      :8080  │  │  Unix socket / TCP                 │  │    │
│  │  └──────────────────┘  └────────────────────────────────────┘  │    │
│  └─────────────────────────────┬────────────────────────────────────┘    │
│                                │                                         │
│  ┌─────────────────────────────▼────────────────────────────────────┐    │
│  │  API LAYER                                                        │    │
│  │                                                                   │    │
│  │  ┌──────────┐  ┌───────────┐  ┌───────────┐  ┌──────────────┐   │    │
│  │  │ IPC      │  │ WebSocket │  │ mTLS      │  │ Translator   │   │    │
│  │  │ Handler  │  │ Bridge    │  │ Bridge    │  │              │   │    │
│  │  │          │  │           │  │           │  │ Events ↔ Msg │   │    │
│  │  └────┬─────┘  └─────┬─────┘  └─────┬─────┘  └──────┬───────┘   │    │
│  └───────┼───────────────┼─────────────┼───────────────┼───────────┘    │
│          │               │             │               │                 │
│  ┌───────▼───────────────▼─────────────▼───────────────▼───────────┐    │
│  │  KERNEL LAYER                                                     │    │
│  │                                                                   │    │
│  │  ┌────────┐  ┌────────────┐  ┌──────────┐  ┌──────────────────┐ │    │
│  │  │ Bus    │  │ Dispatcher │  │ Registry  │  │ Watchdog (30s)   │ │    │
│  │  │        │──│            │──│           │  │                  │ │    │
│  │  │ Pub/Sub│  │ Route      │  │ Module    │  │ Heartbeat +      │ │    │
│  │  │        │  │ events     │  │ lifecycle │  │ restart          │ │    │
│  │  └────────┘  └────────────┘  └──────────┘  └──────────────────┘ │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                │                                         │
│  ┌─────────────────────────────▼────────────────────────────────────┐    │
│  │  MODULE LAYER                                                     │    │
│  │                                                                   │    │
│  │  ┌──────────┐ ┌──────────┐ ┌─────────────┐ ┌───────────────────┐│    │
│  │  │ crypto/  │ │security/ │ │ storage/    │ │  config/          ││    │
│  │  │          │ │          │ │             │ │                   ││    │
│  │  │ argon    │ │ audit    │ │ block       │ │ factory           ││    │
│  │  │ chacha   │ │ lockdown │ │ compression │ │ tier_enterprise   ││    │
│  │  │ engine   │ │ session  │ │ entry       │ │ tier_single       ││    │
│  │  │ hkdf     │ │ memlock  │ │ ingest      │ │ enterprise/       ││    │
│  │  │          │ │          │ │ grimdb/     │ │ single/           ││    │
│  │  └──────────┘ └──────────┘ └─────────────┘ └───────────────────┘│    │
│  │                                                                   │    │
│  │  ┌──────────┐ ┌──────────┐ ┌─────────────┐ ┌───────────────────┐│    │
│  │  │ api/     │ │ sdk/     │ │ tools/      │ │ provider/         ││    │
│  │  │          │ │          │ │             │ │                   ││    │
│  │  │ handlers │ │ plugin   │ │ ssh_gen     │ │ interfaces.go     ││    │
│  │  │  │       │ │ storage  │ │ module      │ │                   ││    │
│  │  └──────────┘ └──────────┘ └─────────────┘ └───────────────────┘│    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                │                                         │
│  ┌─────────────────────────────▼────────────────────────────────────┐    │
│  │  CGO FFI LAYER                                                    │    │
│  │                                                                   │    │
│  │  ┌───────────────────────────────────────────────────────────┐   │    │
│  │  │  cgo/rustbridge.go                                        │   │    │
│  │  │                                                           │   │    │
│  │  │  Go (managed, GC) ← CGO → Rust (unmanaged, mlock'd)      │   │    │
│  │  │                                                           │   │    │
│  │  │  libgrimlocker_core.{so,dylib,dll}                        │   │    │
│  │  └───────────────────────────────────────────────────────────┘   │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                │                                         │
│  ┌─────────────────────────────▼────────────────────────────────────┐    │
│  │  RUST CRYPTO ENCLAVE                                              │    │
│  │                                                                   │    │
│  │  ┌──────────┐ ┌──────────┐ ┌──────────┐ ┌──────────────────────┐│    │
│  │  │ crypto   │ │ enclave  │ │ wipe     │ │ time_guard           ││    │
│  │  │          │ │          │ │          │ │                      ││    │
│  │  │ ChaCha   │ │ alloc()  │ │ 7-pass   │ │ monotonic clock      ││    │
│  │  │ BLAKE3   │ │ dealloc()│ │ fsync    │ │ wall cross-check     ││    │
│  │  │ mlock    │ │ guard pg │ │ unlink   │ │ anomaly detection    ││    │
│  │  │ zeroize  │ │          │ │          │ │                      ││    │
│  │  └──────────┘ └──────────┘ └──────────┘ └──────────────────────┘│    │
│  │                                                                   │    │
│  │  ┌──────────┐ ┌──────────────────────────────────────────────────┐│    │
│  │  │ coordi-  │ │ lib.rs — C-ABI exports                          ││    │
│  │  │ nates    │ │                                                  ││    │
│  │  │          │ │ generate_entropy_file()                          ││    │
│  │  │ key      │ │ extract_key_from_coordinates()                    ││    │
│  │  │ extract  │ │ generate_random_coordinates()                     ││    │
│  │  │ panic    │ │ encrypt_chacha() / decrypt_chacha()               ││    │
│  │  └──────────┘ │ secure_zero()                                    ││    │
│  │               └──────────────────────────────────────────────────┘│    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
│  ┌─────────────────────────────────────────────────────────────────┐    │
│  │  PERSISTENCE LAYER                                                │    │
│  │                                                                   │    │
│  │  ~/.grimlocker/                                                   │    │
│  │  ├── .grim_token         (0600, 32-byte random, deleted on exit) │    │
│  │  ├── vault.gdb           (26B header + ChaCha20-Poly1305 data)   │    │
│  │  └── entropy.bin         (200+ byte CSPRNG entropy source)       │    │
│  └─────────────────────────────────────────────────────────────────┘    │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Data Flow

### Startup Sequence

```
1. Daemon starts (cmd/daemon/main.go)
   │
2. Config factory selects tier (config/factory.go)
   │
3. Kernel initializes (kernel/bus.go, kernel/registry.go)
   │
4. Rust library loaded via CGO (cgo/rustbridge.go)
   │
5. Modules registered:
   ├── Crypto module (crypto/module.go)
   ├── Security module (security/module.go)
   ├── Storage module (storage/entry_module.go)
   └── API module (api/ipc_handler.go)
   │
6. Watchdog starts 30s heartbeat (kernel/watchdog.go)
   │
7. UI served on :8080, IPC on :8374
```

### Vault Unlock Flow

```
UI → WebSocket: MSG_UNLOCK_VAULT {password}
  │
  ▼
WebSocket Bridge (api/websocket/bridge.go)
  │
  ▼
IPC Handler (api/ipc_handler.go)
  │
  ▼
Translator (api/translator.go) → Bus.Publish(AUTH.UNLOCK)
  │
  ▼
Security Module (security/module.go) — Handle AUTH.UNLOCK
  ├── Verify password via crypto engine
  ├── Derive Master Vault Key (MVK) via Argon2id
  ├── Store MVK handle in locked memory
  └── Bus.Publish(AUTH.RESULT {success, handle})
  │
  ▼
Watchdog receives AUTH.RESULT → Bus.Publish(STORAGE.VFS_MOUNT)
  │
  ▼
Storage Module — Handle VFS mount
  ├── Load index from vault
  ├── Mount virtual filesystem
  └── Bus.Publish(STORAGE.READY)
  │
  ▼
Translator → UI: MSG_LOG_BROADCAST "STORAGE.READY"
  │
  ▼
UI navigates to Dashboard
```

### Entry Create Flow

```
UI → WebSocket: MSG_ENTRY_CREATE {title, fields}
  │
  ▼
Entry Handler (api/handlers/entry_handler.go)
  │
  ▼
Policy Manager (api/handlers/policy.go)
  ├── Check write permission
  ├── If denied → AuditLog entry → MSG_ERROR "unauthorized"
  └── If granted → continue
  │
  ▼
Storage Module (storage/entry_module.go) — Handle ENTRY.CREATE
  ├── Compress entry data
  ├── Request encryption via Crypto Engine
  │   └── CGO → Rust: encrypt_chacha(key, nonce, plaintext)
  ├── Write block via Block Store (storage/grimdb/blockstore.go)
  └── Bus.Publish(ENTRY.RESULT {id, title})
  │
  ▼
Translator → UI: MSG_ENTRY_RESULT {id, title}
```

### Lockdown Flow

```
Login attempt fails
  │
  ▼
Security Module (security/lockdown.go)
  ├── Increment failed_attempts in .gdb header
  │
  ├── If failed_attempts < 3:
  │   └── Return error, allow retry
  │
  └── If failed_attempts >= 3:
      ├── Set lockdown_timestamp = current_time
      ├── Persist header to .gdb
      ├── Bus.Publish(SECURITY.LOCKDOWN)
      ├── Zeroize all session keys
      └── UI → LockdownScreen (200-min countdown)
          │
          ▼
      Coordinate override prompt (4 attempts max)
          │
          ├── Correct → Unlock vault
          ├── Wrong ×4 → Wipe
          ├── Timeout (200min) → Wipe
          └── Panic-key ("0,0,0") → Silent wipe
```

---

## Module System

### Kernel Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                        EVENT BUS                              │
│                                                              │
│  Publisher ──► [Event{Type, Payload}] ──► Subscriber         │
│                                                              │
│  ┌──────────┐     ┌──────────┐     ┌──────────────┐        │
│  │ SYNC     │     │ ASYNC    │     │ BUFFERED     │        │
│  │ Channel  │     │ Channel  │     │ Channel      │        │
│  │ (1:1)    │     │ (N:M)    │     │ (ring buf)   │        │
│  └──────────┘     └──────────┘     └──────────────┘        │
└──────────────────────────────────────────────────────────────┘
```

### Module Interface

Every module implements:

```go
type Module interface {
    Name() string
    Version() string
    Init(registry Registry) error
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

### Event Types

Events follow the pattern: `CATEGORY.ACTION`

| Category | Events |
|---|---|
| `AUTH` | `UNLOCK`, `LOCK`, `RESULT`, `LOCKDOWN`, `WIPE` |
| `VAULT` | `CREATE`, `OPEN`, `CLOSE`, `DESTROY` |
| `STORAGE` | `READY`, `VFS_MOUNT`, `BLOCK_WRITTEN`, `INGEST_PROGRESS` |
| `ENTRY` | `CREATE`, `UPDATE`, `DELETE`, `RESULT`, `SEARCH` |
| `SECURITY` | `AUDIT`, `LOCKDOWN`, `WIPE`, `INTEGRITY_CHECK` |
| `INTEGRITY` | `CHECK`, `RESULT`, `MISMATCH` |
| `KERNEL` | `MODULE_LOADED`, `MODULE_FAILED`, `RESTART`, `SHUTDOWN` |

### Lifecycle Hooks

```
Init() → called during module registration
  │
  ▼
Start(ctx) → called when daemon starts
  │
  ├── Subscribe to events
  ├── Initialize subsystems
  └── Return nil on success
  │
  ▼
[Module runs — handles events via bus subscriptions]
  │
  ▼
Stop(ctx) → called during graceful shutdown
  ├── Unsubscribe from events
  ├── Flush pending operations
  ├── Zeroize sensitive state
  └── Return nil on success
```

---

## Storage Architecture

### Vault File Format (.gdb)

```
┌──────────────────────────────────────────────────────┐
│                    HEADER (26 bytes)                   │
│  [0]     failed_attempts        (uint8)               │
│  [1-8]   lockdown_timestamp     (int64, big-endian)   │
│  [9]     override_attempts_left (uint8)               │
│  [10-17] monotonic_boot_ticks   (uint64, big-endian)  │
│  [18-25] wallclock_last_seen    (int64, big-endian)   │
├──────────────────────────────────────────────────────┤
│              CIPHERTEXT (variable)                     │
│  ┌────────────────────────────────────────────────┐   │
│  │  ChaCha20-Poly1305 encrypted payload            │   │
│  │  ┌──────┬──────┬──────┬──────┬──────────┐     │   │
│  │  │Nonce │Block │Block │ ...  │Auth Tag  │     │   │
│  │  │12B   │Meta  │Data  │      │16B       │     │   │
│  │  └──────┴──────┴──────┴──────┴──────────┘     │   │
│  └────────────────────────────────────────────────┘   │
└──────────────────────────────────────────────────────┘
```

### Block Store

The block store provides structured access within the encrypted payload:

```
Block Index (encrypted)
  │
  ├── Block 0: Metadata
  │   ├── vault_name
  │   ├── created_at
  │   ├── modified_at
  │   ├── entry_count
  │   └── workspace_count
  │
  ├── Block 1: Entry Index
  │   ├── entry_id → block_offset mapping
  │   └── entry metadata (title, category, workspace)
  │
  ├── Block 2..N: Workspaces
  │   ├── workspace_id
  │   ├── workspace_key (encrypted with master key)
  │   ├── entry_ids[]
  │   └── access_policies
  │
  └── Block N+1..M: Entries
      ├── entry_id
      ├── encrypted fields (username, password, url, notes)
      ├── file manifest (for file entries)
      └── blob blocks (encrypted file chunks)
```

### Workspace Isolation

Each workspace has:
- Independent encryption key (derived from master + workspace UUID)
- Own entry namespace
- Optional deniable volume (hidden within free space)
- Optional honeypot configuration

---

## Frontend Architecture

### Component Tree

```
<App>
  ├── <AuthContext.Provider>
  │   ├── <OnboardingFlow>          (WELCOME state)
  │   │   ├── <WelcomeScreen>
  │   │   ├── <GeneratingScreen>    (GENERATING state)
  │   │   └── <SingleGlanceScreen>  (SINGLE_GLANCE state)
  │   │
  │   ├── <AuthFlow>                (LOGIN state)
  │   │   ├── <LoginScreen>
  │   │   ├── <CoordinateInput>
  │   │   ├── <LockdownScreen>      (LOCKDOWN state)
  │   │   └── <CountdownTimer>
  │   │
  │   └── <AppShell>                (DASHBOARD state)
  │       ├── <Sidebar>
  │       │   └── <WorkspaceSwitcher>
  │       ├── <Topbar>
  │       ├── <Dashboard>
  │       │   ├── <BentoGrid>
  │       │   │   ├── <CoreNodeOrb>
  │       │   │   ├── <SecretsVault>
  │       │   │   ├── <EntropyIntegrity>
  │       │   │   ├── <TerminalPanel>
  │       │   │   ├── <ThroughputPanel>
  │       │   │   ├── <CryptoGenerator>
  │       │   │   └── <OperationsLog>
  │       │   │
  │       │   └── <VaultDashboard>
  │       ├── <VaultGrid>
  │       │   └── <EntryCard>* → <EntryContextMenu>
  │       ├── <AddEntryModal>
  │       ├── <DetailPanel>
  │       ├── <AdminPanel>           (enterprise only)
  │       │   ├── <AuditLog>
  │       │   ├── <HealthCards>
  │       │   └── <PolicyEditor>
  │       └── <PreferencesPanel>
  └── <ZeroizeBar>
```

### State Machine

```
┌──────────┐    Initialize    ┌─────────────┐    Generation      ┌──────────────────┐
│ WELCOME  │ ────────────────►│ GENERATING  │ ────complete────►  │ SINGLE_GLANCE    │
│          │                  │             │                    │                  │
│ Intro    │                  │ Progress    │                    │ 200-char key     │
│ screen   │                  │ stream      │                    │ 30s timer        │
└──────────┘                  └─────────────┘                    └────────┬─────────┘
                                                                         │
                                                                   "PROCEED"
                                                                         │
                                                                         ▼
                                                                  ┌─────────────┐
                                                                  │ DASHBOARD   │
                                                                  │             │
                                                                  │ Bento Grid  │
                                                                  │ Vault Mgmt  │
                                                                  └──────┬──────┘
                                                                         │
                                                                    Lock / Timeout
                                                                         │
                                                                         ▼
                                                                  ┌─────────────┐
                                                          ┌───────│   LOGIN     │
                                                          │       └──────┬──────┘
                                                          │              │
                                                    fail < 3      fail >= 3
                                                          │              │
                                                          │       ┌──────▼──────┐
                                                          │       │  LOCKDOWN   │
                                                          │       └─────────────┘
                                                          │
                                                     success
                                                          │
                                                          ▼
                                                   DASHBOARD (again)
```

---

## Network Architecture

### Single User Mode

```
┌────────────────────────────────────────┐
│         GrimDB Daemon                   │
│                                         │
│  :8080 ──► go:embed static UI          │
│  :8374 ──► WebSocket (token auth)      │
│                                         │
│  127.0.0.1 only                         │
└────────────────────────────────────────┘
```

### Enterprise Mode

```
                              ┌─────────────────────────────────────┐
                              │         GrimDB Daemon               │
┌─────────┐   mTLS :9443     │                                     │
│ Client  │ ◄──────────────► │  :8080 ──► go:embed static UI       │
│ (CLI)   │                  │  :8374 ──► WebSocket (OIDC auth)    │
└─────────┘                  │                                     │
                             │  ┌──────────────────────────────┐   │
┌─────────┐   OIDC :8443     │  │ Keycloak Integration         │   │
│ Keycloak│ ◄───────────────►│  │ - Token validation           │   │
│ Server  │                  │  │ - Role mapping               │   │
└─────────┘                  │  │ - Policy enforcement         │   │
                             │  └──────────────────────────────┘   │
                             └─────────────────────────────────────┘
```

---

## Build Dependencies Graph

```
┌─────────────────┐
│ core-rust/       │
│ cargo build      │──► libgrimlocker_core.{so,dylib,dll}
└────────┬────────┘
         │ CGO linkage
         ▼
┌─────────────────┐
│ grimdb/          │
│ go:embed ui-dist │──► grimlocker (binary)
└────────┬────────┘
         │
         │ go:embed
         ▼
┌─────────────────┐
│ ui-layer/        │
│ npm run build    │──► ui-dist/ (static assets)
└─────────────────┘
```

**Build order**: Rust → UI → Go (the Go build embeds both the UI output and links the Rust shared library).
