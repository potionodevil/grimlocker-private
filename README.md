# Grimlocker — Private Edition

> **Complete zero-trust password manager.** Hybrid Go/Rust architecture with Tauri desktop frontend. Unencrypted data never touches disk or Go's garbage collector.

---

## Deployment Tiers

Grimlocker ships in two tiers from the **same codebase**, separated by Go build tags:

| | Single-User | Enterprise |
|---|---|---|
| **Build** | `go build ./cmd/daemon` | `go build -tags enterprise ./cmd/daemon` |
| **Auth** | Argon2id master password | OIDC JWT (Keycloak / Azure AD / Okta) |
| **Storage** | Local `vault.gdb` file | S3 / MinIO object store |
| **Transport** | Local IPC (127.0.0.1) | Mutual TLS on port 9443 |
| **Client** | Tauri desktop app | `grimlocker` CLI (+ Tauri UI) |
| **Distribution** | Windows/macOS/Linux EXE | Docker image (`distroless/static`, ~15 MB) |
| **Shutdown** | SIGTERM → graceful | `POST /shutdown` → graceful |

### Quick Start — Single-User (Tauri)
```bash
cd grimdb && go build -o grimlocker ./cmd/daemon
./grimlocker
# Open Tauri app — vault setup wizard starts automatically
```

### Quick Start — Enterprise (Docker + CLI)
```bash
cd grimdb
bash scripts/gen-certs.sh                              # generate mTLS certs
docker-compose -f docker-compose.enterprise.yml up -d  # Keycloak + MinIO + Daemon

export GRIMLOCKER_DAEMON_ADDR=localhost:9443
export GRIMLOCKER_CLIENT_CERT=deploy/tls/client.crt
export GRIMLOCKER_CLIENT_KEY=deploy/tls/client.key
export GRIMLOCKER_CA_CERT=deploy/tls/ca.crt

go build -o grimlocker ./cmd/client
TOKEN=$(bash scripts/get-token.sh)
./grimlocker unlock "$TOKEN"
./grimlocker set "github/token" "ghp_secret123"
./grimlocker health
```

---

## Architecture Overview

```
┌──────────────────────────────────────────────────────────────────────────┐
│                        GRIMLOCKER PRIVATE EDITION                         │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  ┌─────────────────────┐     ┌──────────────────────────────────────┐   │
│  │  UI-LAYER            │     │  GRIMDB-GO DAEMON                    │   │
│  │  (Tauri + React)     │     │                                      │   │
│  │                      │     │  Port :8080 → UI Assets (go:embed)   │   │
│  │  Onboarding:         │     │  Port :8374 → IPC WebSocket Bridge   │   │
│  │  WELCOME →           │     │                                      │   │
│  │  GENERATING →        │◄───►│  ┌────────────────────────────────┐ │   │
│  │  SINGLE_GLANCE →     │ WS  │  │  KERNEL (Event Bus)            │ │   │
│  │  DASHBOARD           │     │  │  bus → dispatcher → registry   │ │   │
│  │                      │     │  │  uuid → watchdog → events       │ │   │
│  │  Components:         │     │  └───────────┬────────────────────┘ │   │
│  │  - auth/             │     │              │                      │   │
│  │  - dashboard/        │     │  ┌───────────▼────────────────────┐ │   │
│  │  - vault/            │     │  │  MODULES (Event-Driven)        │ │   │
│  │  - onboarding/       │     │  │  crypto │ security │ storage   │ │   │
│  │  - workspace/        │     │  │  api    │ tools    │ config    │ │   │
│  │  - admin/            │     │  │  sdk    │ deploy   │ provider  │ │   │
│  │  - preferences/      │     │  └───────────┬────────────────────┘ │   │
│  │  - shared/           │     │              │                      │   │
│  │                      │     │  ┌───────────▼────────────────────┐ │   │
│  │  Services:           │     │  │  STORAGE LAYER                 │ │   │
│  │  crypto.js           │     │  │  block → compression → ingest  │ │   │
│  │  tauriBridge.js      │     │  │  entry → vfs_adapter → grimdb  │ │   │
│  │  ipc.js              │     │  │  strategies (honeypot/deniable)│ │   │
│  │                      │     │  └───────────┬────────────────────┘ │   │
│  │  State: Zustand      │     │              │                      │   │
│  │  Styles: Tailwind    │     │  ┌───────────▼────────────────────┐ │   │
│  └─────────────────────┘     │  │  CGO → CORE-RUST (cdylib)      │ │   │
│                               │  │  rustbridge.go                 │ │   │
│                               │  │  libgrimlocker_core.{so,dylib} │ │   │
│                               │  │                                │ │   │
│                               │  │  crypto.rs    (ChaCha+BLAKE3)  │ │   │
│                               │  │  enclave.rs   (mlock+guard)    │ │   │
│                               │  │  coordinates.rs (key extract)  │ │   │
│                               │  │  wipe.rs      (7-pass shred)   │ │   │
│                               │  │  time_guard.rs (dual-clock)    │ │   │
│                               │  └────────────────────────────────┘ │   │
│                               └──────────────────────────────────────┘   │
│                                                                          │
│  .gdb FILE FORMAT: [failed][lockdown_ts][override][ticks][wall]...[data] │
│                                                                          │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Component Reference

### core-rust/ — Rust Crypto Enclave

The cryptographic heart of the system. Compiled as a `cdylib` shared library and linked into the Go daemon via CGO.

| File | Lines (approx) | Purpose |
|---|---|---|
| `src/crypto.rs` | ~200 | ChaCha20-Poly1305 AEAD, BLAKE3 hashing, mlock/VirtualLock, zeroize, guard pages |
| `src/enclave.rs` | ~180 | Secure memory enclave: alloc/dealloc with automatic zeroization, platform-specific page protection |
| `src/coordinates.rs` | ~120 | 200-character coordinate parser, BLAKE3+HKDF key extraction, panic-key (`0,0,0`) detection |
| `src/lib.rs` | ~800 | C-ABI entry points (`extern "C"`) for all FFI exports: generate_entropy, extract_key, secure_zero, encrypt/decrypt |
| `src/main.rs` | ~500 | CLI state machine, IPC client for direct vault operations, entropy file management |
| `src/time_guard.rs` | ~80 | Dual-clock integrity: monotonic (`Instant`) + wall-clock cross-check, anomaly detection, wipe triggers |
| `src/wipe.rs` | ~100 | 7-pass anti-forensic shredder with fsync verification, file truncation, unlink |

**Dependencies** (`Cargo.toml`): `chacha20poly1305`, `blake3`, `zeroize`, `rand`, `subtle`, `libc` (Unix), `winapi` (Windows).

---

### grimdb/ — Go Daemon

#### kernel/ — Event-Driven Kernel

The central nervous system of the daemon. All inter-module communication flows through the event bus.

| File | Purpose |
|---|---|
| `bus.go` | Thread-safe event bus. Publish/subscribe pattern with typed event channels. |
| `dispatcher.go` | Routes events to registered module handlers. Ensures in-order delivery within subscription scopes. |
| `event.go` | Event type definitions, constants (`AUTH.UNLOCK`, `VAULT.CREATE`, `SECURITY.LOCKDOWN`, etc.), event struct. |
| `registry.go` | Module registry: tracks all loaded modules, their dependencies, and lifecycle state. |
| `uuid.go` | UUID generation for events, entries, sessions, and audit log entries. |
| `watchdog.go` | 30-second heartbeat monitor. On module timeout, triggers `KERNEL_RESTART` via registry. |

#### crypto/ — Go Crypto Engine

Go-side cryptographic operations, orchestrated by a central engine that delegates to Rust for all key-sensitive paths.

| File | Purpose |
|---|---|
| `argon.go` | Argon2id password hashing (32 MiB, 3 iterations, 4 lanes). Produces 32-byte hashes. |
| `chacha.go` | ChaCha20-Poly1305 AEAD via `golang.org/x/crypto`. Nonce management with CSPRNG. |
| `coordinate.go` | Coordinate-based key derivation. Parses coordinate sets, uses BLAKE3+HKDF for key extraction. |
| `engine.go` | Central coordinator. Manages the entire crypto pipeline: key generation, derivation, encryption, zeroization. |
| `entropy.go` | Secure entropy generation via `crypto/rand`. Creates and validates entropy files. |
| `hkdf.go` | HKDF-SHA256 (RFC 5869). Extract+expand for deriving child keys from master key. |
| `interface.go` | Swappable provider interfaces (`Engine`, `PasswordHasher`, `Encryptor`, `KeyDeriver`). |
| `module.go` | Kernel module registration. Handles lifecycle and event subscriptions for the crypto module. |
| `provider.go` | Default provider binding all Go primitives with the Rust FFI bridge. |
| `shredder.go` | Secure memory/disk deletion with verification. Uses compiler barriers to prevent optimization. |

#### security/ — Security Module

Handles authentication, authorization, auditing, memory protection, and the lockdown state machine.

| File | Purpose |
|---|---|
| `audit.go` | Immutable audit log with SHA-256 hash chaining. Every security event is cryptographically linked. |
| `constant_time.go` | Constant-time byte and string comparisons. Prevents timing side-channels in password verification. |
| `integrity.go` | Binary hash verification at startup and periodic intervals. Watchdog integration for tamper detection. |
| `lockdown.go` | Hard/soft lockdown state machine: 3 fails → 200-min window → 4 coordinate attempts → wipe. |
| `memlock.go` | Cross-platform memory locking interface. Abstracts mlock (Unix) and VirtualLock (Windows). |
| `memlock_unix.go` | Unix mlock implementation via `golang.org/x/sys/unix`. Locks buffer pages into RAM. |
| `memlock_windows.go` | Windows VirtualLock implementation via `golang.org/x/sys/windows`. |
| `module.go` | Kernel module registration for the security subsystem. Handles `AUTH.*`, `SECURITY.*` events. |
| `session.go` | Session and key lifecycle management. Ephemeral session keys, auto-expiry, zeroization on logout/lockdown. |
| `mtls/bridge.go` | mTLS bridge for enterprise-tier deployment. Mutual TLS between daemon and external services. |
| `mtls/certmanager.go` | Certificate management: generation, rotation, validation for mTLS connections. |

#### storage/ — Storage Layer

Manages the encrypted vault file (`.gdb`), block-level storage, compression, file ingestion, and workspace organization.

| File | Purpose |
|---|---|
| `block.go` | Block-level read/write operations on the encrypted vault file. Atomic write semantics. |
| `compression.go` | Data compression/decompression before encryption. Reduces plaintext size and adds entropy. |
| `compression_test.go` | Unit tests for compression correctness and edge cases. |
| `entry.go` | Entry data model: fields, metadata, timestamps, encryption state. |
| `entry_module.go` | Kernel module for entry CRUD operations. Handles `ENTRY.CREATE`, `ENTRY.UPDATE`, `ENTRY.DELETE` events. |
| `ingest.go` | io.Pipe-based streaming file ingestion. Chunked encryption with progress reporting. |
| `interface.go` | Storage provider interfaces allowing swappable backends (local, remote). |
| `vfs_adapter.go` | Virtual filesystem adapter. Maps vault entries to a mountable filesystem interface. |
| `workspace.go` | Multi-workspace management. Isolated workspaces with independent keys and policies. |
| `grimdb/adapter.go` | GrimDB-native storage adapter. Bridges the storage layer with Go's file I/O. |
| `grimdb/blockstore.go` | Block store implementation for `.gdb` file operations. Handles the 26-byte header. |
| `grimdb/metadata.go` | Metadata management: creation times, modification timestamps, size tracking. |
| `grimdb/store.go` | High-level store interface for vault read/write operations. |
| `grimdb/vault.go` | Vault lifecycle: create, open, close, lock, unlock, destroy. |
| `remote/adapter.go` | Remote storage adapter for enterprise distributed deployments. |
| `remote/cache.go` | Client-side cache for remote storage with consistency guarantees. |
| `remote/index.go` | Remote index management for distributed vault entries. |
| `remote/vault.go` | Remote vault implementation communicating with a GrimDB server. |
| `strategies/deniable.go` | Deniable encryption strategy. Provides plausible deniability with hidden volumes. |
| `strategies/honeypot.go` | Honeypot vault strategy. Decoy vaults that appear real but contain no actual secrets. |

#### api/ — API & Communication

Handles all external communication: IPC protocol, WebSocket bridge, message translation, and policy enforcement.

| File | Purpose |
|---|---|
| `ipc_handler.go` | IPC message handler. Dispatches incoming messages to the appropriate module handler. |
| `translator.go` | Message translator. Converts IPC messages to/from kernel events and vice versa. |
| `handlers/entry_handler.go` | Entry-specific request handler. Processes `MsgEntryCreate`, `MsgEntryUpdate`, `MsgEntryDelete`, `MsgFileIngest*`. |
| `handlers/policy.go` | Policy validation handler. Checks permissions before allowing CRUD operations. |
| `ipc/protocol.go` | IPC protocol definition: message types, wire format, framing (4-byte length + 1-byte type + payload). |
| `ipc/server.go` | IPC server implementation. Accepts connections, parses frames, routes to handler. |
| `mtls/protocol.go` | mTLS protocol handler. TLS handshake, certificate validation, secure channel management. |
| `websocket/bridge.go` | WebSocket bridge. Serves the Tauri frontend on port 8374 with token-based authentication. |

#### sdk/ — Plugin SDK

Interfaces and base implementations for building GrimDB plugins.

| File | Purpose |
|---|---|
| `biometric_interface.go` | Biometric authentication interface for hardware token integration (YubiKey, fingerprint, etc.). |
| `dispatcher.go` | Plugin dispatcher interface. Allows plugins to publish and subscribe to events. |
| `event.go` | Plugin event definitions and serialization. |
| `plugin.go` | Base plugin struct. Lifecycle hooks: `Init()`, `Start()`, `Stop()`, `Name()`, `Version()`. |
| `registry.go` | Plugin registry. Manages plugin loading, dependency resolution, and lifecycle. |
| `storage.go` | Plugin storage interface. Allows plugins to persist data in their own encrypted namespace. |

#### config/ — Configuration System

Tier-based configuration for single-user and enterprise deployments.

| File | Purpose |
|---|---|
| `factory.go` | Configuration factory. Selects and instantiates the appropriate tier config based on environment. |
| `tier_enterprise.go` | Enterprise tier configuration: multi-user, mTLS, Keycloak, distributed storage. |
| `tier_single.go` | Single-user tier configuration: local-only, token auth, local storage. |
| `enterprise/auth.go` | Enterprise authentication: OIDC, SAML, Keycloak integration. |
| `enterprise/config.go` | Enterprise-specific configuration: cluster mode, replication, failover. |
| `enterprise/provider.go` | Enterprise provider implementations for storage, auth, and networking. |
| `single/auth.go` | Single-user authentication: password + coordinate system. |
| `single/provider.go` | Single-user provider: local file storage, token auth, no network. |
| `single/storage.go` | Single-user storage configuration: local `.gdb` path, entropy location. |

#### cmd/ — Entry Points

| File | Purpose |
|---|---|
| `daemon/main.go` | Main daemon entry point. Dual-port server (8080 UI + 8374 IPC), token generation, module initialization. |
| `daemon/listener_enterprise.go` | Enterprise listener setup: mTLS, OIDC endpoints, cluster health checks. |
| `daemon/listener_single.go` | Single-user listener setup: local token auth, no external network ports. |
| `client/main.go` | CLI client for interacting with a running GrimDB daemon. |
| `client/commands.go` | CLI command definitions: unlock, lock, create-entry, list-entries, health, wipe. |
| `client/local.go` | Local client implementation connecting via Unix socket / named pipe. |
| `client/remote.go` | Remote client implementation for enterprise cluster connections. |
| `healthcheck/main.go` | Health check binary for Docker/Kubernetes readiness and liveness probes. |

#### tools/ — Utility Tools

| File | Purpose |
|---|---|
| `module.go` | Module utility functions: validation, dependency checking, configuration parsing. |
| `ssh_gen.go` | SSH key pair generator. Creates and securely stores SSH keys within the vault. |
| `ssh_gen_test.go` | Unit tests for SSH key generation. |

#### deploy/ — Deployment Resources

| File | Purpose |
|---|---|
| `keycloak/grimlocker-realm.json` | Keycloak realm configuration for enterprise OIDC authentication. |
| `tls/.gitkeep` | Placeholder for generated TLS certificates in deployment. |

#### scripts/ — Operational Scripts

| File | Purpose |
|---|---|
| `gen-certs.sh` | TLS certificate generation script. Creates CA, server cert, client cert for mTLS. |
| `get-token.sh` | Token retrieval script for authenticating external clients with the daemon. |

#### tests/ — Integration Tests

| File | Purpose |
|---|---|
| `lockdown_test.go` | Integration test for the lockdown state machine: failed attempts, timeout, coordinate override, wipe. |

#### docs/ — Documentation

| File | Purpose |
|---|---|
| `api_flow.md` | Mermaid diagrams of API flows: Vault Unlock, File Ingest, CRUD, Watchdog Heartbeat, Audit Log. |

#### provider/ — Provider Interfaces

| File | Purpose |
|---|---|
| `interfaces.go` | Top-level provider interface definitions consumed by the kernel to initialize subsystems. |

---

### ui-layer/ — Tauri Desktop Frontend

React-based cyberpunk-themed UI wrapped in a Tauri native shell (no Electron). State managed by Zustand, animations via GSAP and Framer Motion, 3D elements via Three.js.

#### Core Files

| File | Purpose |
|---|---|
| `src/main.jsx` | React entry point. Mounts the App component and initializes global styles. |
| `src/App.jsx` | Root component. Onboarding state machine: WELCOME → GENERATING → SINGLE_GLANCE → DASHBOARD. GSAP page transitions. |
| `index.html` | HTML shell. Meta tags, font loading, Tauri CSP configuration. |
| `package.json` | Node dependencies: React 18, Vite 5, TailwindCSS 3, Three.js, GSAP, Framer Motion, Zustand. |
| `vite.config.js` | Vite build configuration with Tauri plugin and dev server proxy. |
| `tailwind.config.js` | Tailwind configuration with cyberpunk color palette and custom animations. |

#### Components

| Directory / File | Purpose |
|---|---|
| **auth/** | Authentication UI |
| `LoginScreen.jsx` | Master password entry with visual feedback and coordinate prompt. |
| `SetupScreen.jsx` | Initial vault creation: password, confirmation, entropy file path. |
| `LockdownScreen.jsx` | 200-minute lockdown display with countdown and coordinate override option. |
| `CoordinateInput.jsx` | Coordinate entry field with panic-key support (`0,0,0`). |
| `CountdownTimer.jsx` | Visual countdown timer during lockdown period. |
| `EntropyDisplay.jsx` | 200-character entropy key display during Single Glance phase. |
| **onboarding/** | Onboarding Flow |
| `WelcomeScreen.jsx` | Introduction screen with feature highlights and initialization button. |
| `GeneratingScreen.jsx` | Progress display with GSAP particle animations during entropy generation. |
| `SingleGlanceScreen.jsx` | 30-second entropy display with copy/paste blocking and auto-zeroize. |
| **dashboard/** | Main Dashboard |
| `BentoGrid.jsx` | Bento-style grid layout for the dashboard. Modular, responsive tile arrangement. |
| `CoreNodeOrb.jsx` | Interactive 3D orb (Three.js / react-three-fiber) representing vault state. |
| `CryptoGenerator.jsx` | Password and key generator with strength indicator. |
| `EntropyIntegrity.jsx` | Entropy file health display with integrity verification results. |
| `OperationsLog.jsx` | Real-time operation log with event streaming from the daemon. |
| `SecretsVault.jsx` | Secrets overview with categorization and quick-access cards. |
| `TerminalPanel.jsx` | Terminal-style command panel for power users. |
| `ThroughputPanel.jsx` | Performance metrics: encryption throughput, ingest speed, I/O stats. |
| `VaultDashboard.jsx` | Top-level vault management: health, size, workspace overview. |
| **vault/** | Vault Entry Management |
| `VaultGrid.jsx` | Grid view of all vault entries with search and filtering. |
| `EntryCard.jsx` | Individual entry display with icon, title, strength indicator, and actions. |
| `EntryContextMenu.jsx` | Right-click context menu: edit, copy, delete, move to workspace. |
| `AddEntryModal.jsx` | Modal for creating new entries: title, fields, tags, workspace assignment. |
| `SearchBar.jsx` | Full-text search across all vault entries with real-time filtering. |
| `FileVaultUpload.jsx` | File upload interface with drag-and-drop and progress tracking. |
| `StrengthDot.jsx` | Visual password strength indicator (colored dot with tooltip). |
| **workspace/** | Workspace Management |
| `WorkspaceSwitcher.jsx` | Dropdown to switch between active workspaces with key separation. |
| **admin/** | Administration (Enterprise) |
| `AuditLog.jsx` | Immutable audit log viewer with cryptographic chain verification. |
| `HealthCards.jsx` | System health dashboard: daemon status, memory, disk, uptime. |
| `PolicyEditor.jsx` | Access policy editor for enterprise deployments. |
| **preferences/** | User Preferences |
| `PreferencesPanel.jsx` | Settings: auto-lock timeout, theme, entropy path, security hardening. |
| **layout/** | App Layout |
| `AppShell.jsx` | Main application shell: sidebar + topbar + content area. |
| `Sidebar.jsx` | Navigation sidebar: dashboard, vault, admin, preferences, workspace. |
| `Topbar.jsx` | Top bar: lock button, health indicator, connection status. |
| `DetailPanel.jsx` | Slide-out detail panel for entry inspection and editing. |
| **shared/** | Shared/Utility Components |
| `ScanLine.jsx` | Animated scan-line effect for the cyberpunk aesthetic. |
| `ZeroizeBar.jsx` | Visual indicator showing auto-zeroize countdown (30s). |
| `TerminalError.jsx` | Terminal-style error display with redacted sensitive information. |
| **debug/** | Debug Utilities |
| `DebugPanel.jsx` | Developer debug panel: event bus monitor, module state, performance tracing. |
| **ui/** | UI Primitives |
| `Badge.jsx` | Status badge component. |
| `Button.jsx` | Themed button with loading, disabled, and icon states. |
| `Card.jsx` | Reusable card container with header, body, and footer slots. |
| `Input.jsx` | Themed input with label, error, and icon support. |
| `Select.jsx` | Themed dropdown select. |
| `Toggle.jsx` | Toggle switch for boolean settings. |

#### Services

| File | Purpose |
|---|---|
| `crypto.js` | Browser-side cryptographic utilities for non-sensitive operations. |
| `ipc.js` | Legacy IPC client for communicating with the Go daemon. |
| `tauriBridge.js` | Tauri-native WebSocket bridge. Reads `.grim_token`, connects to port 8374, handles message framing. |

#### State Management

| File | Purpose |
|---|---|
| `store/useGrimStore.js` | Zustand store. Manages global state: vault status, active workspace, onboarding phase, auth state. |
| `store/preferencesSlice.js` | Zustand slice for user preferences. |

#### Hooks

| File | Purpose |
|---|---|
| `hooks/useAutoLock.jsx` | Auto-lock hook: triggers vault lock after idle timeout. |
| `hooks/useCountdown.js` | Countdown hook for lockdown timer display. |

#### Context

| File | Purpose |
|---|---|
| `context/AuthContext.jsx` | React context for authentication state propagation. |

#### Utils

| File | Purpose |
|---|---|
| `utils/devMode.js` | Development mode utilities: mock data, debug logging, hot-reload helpers. |

#### Styles

| File | Purpose |
|---|---|
| `styles/globals.css` | Global styles: Tailwind directives, cyberpunk theme variables, font configuration. |
| `styles/onboarding.css` | Onboarding-specific styles: security hardening animations, copy/paste blocking. |
| `styles/tokens.css` | Design tokens: colors, spacing, shadows, typography scale. |

---

## Build & Setup

### Prerequisites

| Component | Version | Notes |
|---|---|---|
| **Rust** | 1.75+ | Required for `core-rust` (cdylib) |
| **Go** | 1.21+ | Required for `grimdb` daemon |
| **Node.js** | 18+ | Required for `ui-layer` development/build |
| **OS** | Linux / macOS | Full native support |
| **OS (Windows)** | WSL2 | Windows requires WSL2 for full functionality |

### Build Steps

```bash
# 1. Build Rust crypto core as shared library
cd core-rust
cargo build --release --lib
# Output: target/release/libgrimlocker_core.so (Linux)
#         target/release/libgrimlocker_core.dylib (macOS)
#         target/release/grimlocker_core.dll (Windows)

# 2. Build Go daemon (embeds UI + links Rust library)
cd ../grimdb
go mod tidy
go build -o grimlocker ./cmd/daemon

# 3. Build UI for production (optional, for development)
cd ../ui-layer
npm install
npm run build          # Production build → ../grimdb/ui-dist/
```

### Run

```bash
# Start the daemon
export GRIMLOCKER_APP_DIR=~/.grimlocker
./grimdb/grimlocker

# Output:
# GRIMLOCKER_UI=http://localhost:8080
# GRIMLOCKER_IPC=ws://127.0.0.1:8374/ws
# Token: <32-byte hex token>

# Open browser → http://localhost:8080
# → Onboarding flow: WELCOME → GENERATING → SINGLE_GLANCE → DASHBOARD
```

### Environment Variables

| Variable | Default | Description |
|---|---|---|
| `GRIMLOCKER_APP_DIR` | `~/.grimlocker` | App data directory (token, vault, entropy) |
| `GRIMLOCKER_DB_PATH` | `$APP_DIR/vault.gdb` | Path to the encrypted vault file |
| `GRIMLOCKER_TIER` | `single` | Deployment tier: `single` or `enterprise` |
| `GRIMLOCKER_IPC_PORT` | `8374` | WebSocket IPC port |
| `GRIMLOCKER_UI_PORT` | `8080` | UI HTTP port |
| `GRIMLOCKER_KEYCLOAK_URL` | (none) | Keycloak OIDC URL (enterprise only) |
| `GRIMLOCKER_MTLS_CERT` | (none) | mTLS certificate path (enterprise only) |
| `GRIMLOCKER_MTLS_KEY` | (none) | mTLS private key path (enterprise only) |

---

## Security Model

### Encryption Layers

```
Password → Argon2id(password, salt) → Master Key (32 bytes)
                                              │
                    ┌─────────────────────────┼─────────────────────────┐
                    │                         │                         │
                    ▼                         ▼                         ▼
           BLAKE3(MK) → HKDF          BLAKE3(MK) → HKDF          BLAKE3(MK) → HKDF
                    │                         │                         │
                    ▼                         ▼                         ▼
          Workspace Key 1 (32)        Workspace Key N (32)       Session Key (32)
                    │                         │                         │
                    ▼                         ▼                         ▼
          ChaCha20-Poly1305           ChaCha20-Poly1305           ChaCha20-Poly1305
          encrypts workspace          encrypts workspace          encrypts IPC messages
```

### Key Hierarchy

1. **Master Key**: Derived via Argon2id from the user's password + salt. Never leaves locked memory.
2. **Workspace Keys**: Derived via BLAKE3 + HKDF-SHA256 from the master key. One per workspace.
3. **Session Keys**: Ephemeral ChaCha20 keys derived per session. Rotated on timeout.
4. **Coordinate Keys**: Extracted from the entropy file at randomized coordinate positions. Used for override.

### Lockdown State Machine

```
                    ┌─────────────┐
                    │   UNLOCKED  │
                    └──────┬──────┘
                           │ Master Password
                    ┌──────▼──────┐
               ┌────│  ATTEMPT    │────┐
               │    └──────┬──────┘    │
               │           │ success   │ fail <3 times
               │    ┌──────▼──────┐    │
               │    │  UNLOCKED   │    │ (retry)
               │    └─────────────┘    │
               │                       │
          fail >= 3 times              │
               │                       │
    ┌──────────▼──────────┐            │
    │  LOCKDOWN MODE       │            │
    │  (200-minute window) │◄───────────┘
    └──────────┬──────────┘
               │ Coordinate Override (4 attempts max)
    ┌──────────▼──────────┐
    │  OVERRIDE ATTEMPTS   │
    └──────────┬──────────┘
               │
    ┌──────────┼──────────┐
    │          │          │
correct    wrong x4   timeout
coords     or panic    expired
    │          │          │
    ▼          ▼          ▼
 UNLOCKED    WIPE       WIPE
```

### Panic-Key Coordinates

Entering `0,0,0` at the coordinate prompt triggers a **disguised wipe**:

```
Verifying coordinates... OK      ← System lies
Decrypting vault... OK           ← System lies
Loading entries... Done.         ← System lies
```

The vault is silently destroyed in the background while the UI displays normal behavior.

### Anti-Forensic Shredder

On wipe trigger:
1. Open `.gdb` with write access
2. **7 passes** of cryptographic random data matching exact file size
3. `fsync` after each pass
4. Truncate to 0 bytes
5. `fsync` again
6. `unlink` the file

### Memory Protection

- **mlock** (Unix) / **VirtualLock** (Windows): Locks sensitive buffers into physical RAM
- **Zeroize on drop**: Rust `zeroize` crate ensures key material is overwritten at deallocation
- **Guard pages**: Protected pages before and after sensitive buffers
- **Go GC isolation**: Sensitive data never stored in Go-managed memory

### Time-Thief Protection

- **Monotonic clock** (`std::time::Instant`): Cannot be manipulated by user or OS
- **Wall-clock cross-check**: Regression triggers immediate wipe
- **Anomaly detection**: Jumps > 1 year or monotonic regression = wipe

---

## IPC Protocol

### Message Format

```
[4 bytes: big-endian payload length][1 byte: message type][N bytes: payload]
```

### Message Types

| Code | Name | Direction | Payload | Purpose |
|---|---|---|---|---|
| `0x01` | `MSG_GET_HEADER` | Client → Server | None | Request vault header |
| `0x02` | `MSG_HEADER` | Server → Client | 26 bytes | Vault header (lockdown state) |
| `0x03` | `MSG_GET_CIPHERTEXT` | Client → Server | None | Request encrypted secrets |
| `0x04` | `MSG_CIPHERTEXT` | Server → Client | Raw ciphertext | Encrypted vault payload |
| `0x05` | `MSG_UPDATE_HEADER` | Client → Server | 26 bytes | Update vault header |
| `0x06` | `MSG_UPDATE_CIPHERTEXT` | Client → Server | Re-encrypted payload | Store new secrets |
| `0x07` | `MSG_TRIGGER_WIPE` | Client → Server | None | Trigger self-destruct |
| `0x08` | `MSG_ACK` | Bidirectional | None | Acknowledgment |
| `0x09` | `MSG_ERROR` | Server → Client | UTF-8 string | Error message |
| `0x0A` | `MSG_PANIC_WIPE` | Client → Server | None | Panic-key self-destruct |
| `0x0B` | `MSG_GENERATE_MATRIX` | Client → Server | JSON `{line_count, entropy_path}` | Trigger entropy generation |
| `0x0C` | `MSG_PROGRESS` | Server → Client | JSON `{progress, stage, message}` | Streaming progress (1-100%) |
| `0x0D` | `MSG_GENERATION_RESULT` | Server → Client | JSON `{key_hex, coordinates, entropy_size}` | Generation complete |
| `0x0E` | `MSG_ZEROIZE_CONFIRM` | Client → Server | None | Confirm JS state nuked |

### WebSocket Authentication

1. Go generates 32-byte random token at startup
2. Token written to `~/.grimlocker/.grim_token` (mode 0600)
3. Tauri frontend reads token via native `fs.readTextFile()`
4. WebSocket handshake: `ws://127.0.0.1:8374/ws?token=XYZ`
5. Go validates token; rejects on mismatch
6. Token file deleted on daemon shutdown

---

## .gdb Binary Format

| Offset | Size | Field | Type | Endian |
|---|---|---|---|---|
| 0 | 1 | `failed_attempts` | uint8 | — |
| 1 | 8 | `lockdown_timestamp` | int64 | Big |
| 9 | 1 | `override_attempts_left` | uint8 | — |
| 10 | 8 | `monotonic_boot_ticks` | uint64 | Big |
| 18 | 8 | `wallclock_last_seen` | int64 | Big |
| 26+ | var | `ciphertext_payload` | []byte | — |

The 26-byte header is read unencrypted. The ciphertext payload is ChaCha20-Poly1305 encrypted with the master-derived workspace key.

---

## Deployment

### Docker (Enterprise Tier)

```bash
cd grimdb
docker-compose -f docker-compose.enterprise.yml up -d
```

Services:
- **grimlocker**: GrimDB daemon (ports 8080, 8374)
- **keycloak**: OIDC identity provider (port 8443)
- **postgres**: Keycloak database

### Bare Metal (Single User)

```bash
./grimdb/grimlocker
# Serves UI on :8080, IPC on :8374
# All data in ~/.grimlocker/
```

### TLS Setup

```bash
cd grimdb/scripts
./gen-certs.sh   # Generates CA, server cert, client cert
```

---

## Testing

```bash
# Go unit tests
cd grimdb
go test ./...

# Rust unit tests
cd ../core-rust
cargo test --release

# Rust linting
cargo clippy --release -- -D warnings

# Go linting
cd ../grimdb
golangci-lint run ./...

# UI tests (requires Node)
cd ../ui-layer
npm run test
```

---

## Development

See [docs/development.md](docs/development.md) for the full development guide including:
- Development environment setup
- Module development workflow
- Event bus integration
- Plugin SDK usage
- Debugging and profiling

Additional documentation:
- [docs/architecture.md](docs/architecture.md) — Complete architecture reference
- [docs/security-model.md](docs/security-model.md) — Security model deep dive
- [docs/crypto-spec.md](docs/crypto-spec.md) — Cryptographic specification
- [docs/api-reference.md](docs/api-reference.md) — Full API reference
- [docs/deployment.md](docs/deployment.md) — Deployment guide
- [docs/threat-model.md](docs/threat-model.md) — Formal threat model

---

## Known Limitations

| Limitation | Impact | Mitigation |
|---|---|---|
| SSD wear leveling | Wipe may not destroy all physical copies | Use HDD for sensitive data; documented as best-effort |
| No SGX/SEV in MVP | DMA attacks possible on consumer hardware | mlock + Zeroize baseline; enclave planned for enterprise |
| BLAKE3 is fast hash | Not memory-hard; brute-force feasible with low-entropy coordinates | Use high-entropy entropy file; consider Argon2id upgrade |
| System clock trust | Wall-clock can be manipulated | Monotonic clock cross-check; anomaly detection triggers wipe |
| Browser JS memory | Zeroize not guaranteed in JS runtime | 30s auto-zeroize + GC hint + blocked copy/paste |
| CGO build coupling | Rust cdylib must be built before Go | Separate `cargo build --release --lib` step required |

---

## Related Repositories

- **[grimlocker-public](../grimlocker-public)** — Security audit edition with only the crypto and security code. Suitable for community review.

---

## License

Proprietary. All rights reserved.
