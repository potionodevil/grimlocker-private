```
⠀⠀⠀⠀⠀⠀⠀⠀⠀⢀⣤⠞⠛⠛⠶⣄⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀      _____    _       __         __          
⠀⠀⠀⣠⢤⠀⠀⠀⣴⠟⠁⠀⠀⠀⠀⠈⢧⡀⠀⠀⠀⠀⠀⠀⠀⠀⠀    / ___/___(_)_ _  / /__  ____/ /_____ ____
⠀⢠⠞⢁⡞⢀⣰⠞⠁⠀⠀⠀⠀⠀⠀⠀⠀⠻⣄⠀⠀⠀⠀⠀⠀⠀⠀    / (_ / __/ /  ' \/ / _ \/ __/  '_/ -_) __/
⡰⢷⠆⠸⠶⠋⠁⠀⢠⡴⠄⠀⠀⠀⠀⠀⠀⠀⠙⣆⠀⠀⠀⠀⠀⠀⠀    \___/_/ /_/_/_/_/_/\___/\__/_/\_\\__/_/   
⠀⣼⠀⠀⠀⣠⠴⠋⠁⠀⠀⠀⠀⠀⠀⠀⠀⠀⠈⠚⢧⡀⠀⠀⠀⠀⠀                                              
⠀⠹⡤⠖⣻⠃⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠹⣄⠀⠀⠀⠀                                              
⠀⠀⠀⢰⠇⠀⠀⠀⠀⠀⠀⠀⣠⢶⡀⠀⠀⠀⠀⠀⠀⠀⢹⡄⠀⠀⠀                                              
⠀⠀⢀⡏⠀⠀⠀⠀⠀⠀⣠⠞⠁⠀⠙⢦⡀⠀⠀⠀⠀⠀⠀⢳⡀⠀⠀                                              
⠀⢀⡞⠀⠀⠀⠀⠀⣠⠞⠁⠀⠀⠀⠀⠀⠙⠳⣄⠀⠀⠀⠀⠈⢧⠀⠀                                              
⠀⡞⠀⠀⠀⢀⣤⢞⡁⠀⠀⠀⠀⠀⠀⠀⠀⠀⣈⡷⣄⠀⠀⠀⠈⢧⠀                                              
⣸⠁⠀⢀⣴⡟⢹⠀⠉⠓⢤⣀⠀⣄⠀⣀⠴⠊⠁⢀⡏⢷⣄⠀⠀⠘⡆                                              
⡏⠀⢠⢿⡏⠀⠀⠹⢦⣀⠴⠛⠋⣉⠙⠓⠤⣠⠴⠋⠀⠈⢻⣳⡄⠀⣿                                              
⣧⢀⡏⠈⠳⣄⡀⣀⣤⣄⠀⠀⡘⠈⢆⠀⠀⣠⣄⡀⢀⡴⠞⠀⢻⠀⡿                                              
⠹⡌⡇⠀⠀⠀⠉⠉⣆⡏⡀⠰⠅⠤⠬⠀⠘⢩⣼⠉⠉⠀⠀⠀⣸⣰⠃                                              
⠀⠙⢿⣄⠀⠀⠀⠀⡟⣏⡑⠳⠒⠲⠲⠚⢋⡟⣷⠀⠀⠀⠀⣰⡿⠃⠀                                              
⠀⠀⠈⠛⢷⣤⡀⠀⣇⠑⠬⣑⠒⠄⠒⣈⠕⠂⣸⠀⣀⣴⡾⠋⠀⠀⠀                                              
⠀⠀⠀⠀⠀⠙⢿⡳⣌⡳⣄⠀⣉⣚⡉⠀⡠⢊⡤⣺⠟⠋⠀⠀⠀⠀⠀                                              
⠀⠀⠀⠀⠀⠀⠀⠉⢮⡱⣌⠉⠉⠉⠉⠉⡵⢋⠞⠁⠀⠀⠀⠀⠀⠀⠀                                              
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠑⢌⢣⡀⠀⢀⢞⡴⠋⠀⠀⠀⠀⠀⠀⠀⠀⠀                                              
⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠀⠓⢬⣀⠵⠋⠀⠀⠀⠀⠀⠀⠀⠀                                              
```

# Grimlocker — Private Edition

> **Zero-trust, enterprise-grade Password Manager.** Hybrid Go/Rust Architektur mit Tauri-Desktop-Frontend. Unverschlüsselte Daten berühren weder die Festplatte noch den Go-Garbage-Collector.

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

The cryptographic heart of the system. Compiled as a `cdylib` shared library and linked via CGO.

| Module | Key Files | Purpose |
|--------|-----------|---------|
| Crypto | `crypto.rs` | ChaCha20-Poly1305 AEAD, BLAKE3, mlock, zeroize, guard pages |
| Enclave | `enclave.rs` | Secure memory alloc/dealloc with automatic zeroization |
| Coordinates | `coordinates.rs` | 200-char coordinate parser, BLAKE3+HKDF extraction, panic-key detection |
| FFI | `lib.rs` | C-ABI entry points: generate_entropy, extract_key, encrypt/decrypt, secure_zero |
| CLI | `main.rs` | CLI state machine, IPC client, entropy management |
| Time Guard | `time_guard.rs` | Dual-clock integrity (monotonic + wall-clock cross-check) |
| Shredder | `wipe.rs` | 7-pass anti-forensic shredder with fsync verification |

---

### grimdb/ — Go Daemon

Event-driven kernel with modular architecture. All inter-module communication flows through the event bus.

| Module | Purpose | Key Files |
|--------|---------|-----------|
| `kernel/` | Event bus, dispatcher, module registry, watchdog | `bus.go`, `dispatcher.go`, `registry.go`, `watchdog.go` |
| `crypto/` | Argon2id, ChaCha20-Poly1305, HKDF, entropy, coordinate keys | `argon.go`, `engine.go`, `chacha.go`, `coordinate.go`, `hkdf.go` |
| `security/` | Auth, lockdown, audit, mTLS, memlock, session management | `lockdown.go`, `audit.go`, `session.go`, `memlock.go`, `constant_time.go` |
| `storage/` | Block storage, compression, VFS, workspaces, remote | `block.go`, `entry.go`, `workspace.go`, `compression.go`, `vfs_adapter.go` |
| `api/` | IPC protocol, WebSocket bridge, message handlers | `ipc_handler.go`, `protocol.go`, `server.go`, `bridge.go` |
| `sdk/` | Plugin SDK interfaces (biometric, events, storage) | `plugin.go`, `registry.go`, `event.go`, `storage.go` |
| `config/` | Tier-based config (single/enterprise) | `factory.go`, `tier_single.go`, `tier_enterprise.go` |
| `cmd/` | Entry points: daemon, client, healthcheck | `daemon/main.go`, `client/main.go`, `healthcheck/main.go` |
| `tools/` | SSH keygen, module utilities | `ssh_gen.go`, `module.go` |
| `deploy/` | Keycloak realm, TLS templates | `keycloak/`, `tls/` |
| `scripts/` | Cert generation, token retrieval | `gen-certs.sh`, `get-token.sh` |

---

### ui-layer/ — Tauri Desktop Frontend

React-based cyberpunk-themed UI wrapped in a Tauri native shell. State managed by Zustand, animations via GSAP and Framer Motion, 3D via Three.js.

**Core:** `src/main.jsx`, `src/App.jsx`, `index.html`, `vite.config.js`, `tailwind.config.js`

**Component groups:**
| Group | Components |
|-------|-----------|
| `auth/` | LoginScreen, SetupScreen, LockdownScreen, CoordinateInput, CountdownTimer |
| `onboarding/` | WelcomeScreen, GeneratingScreen, SingleGlanceScreen |
| `dashboard/` | BentoGrid, CoreNodeOrb, SecretsVault, TerminalPanel, ThroughputPanel |
| `vault/` | VaultGrid, EntryCard, AddEntryModal, SearchBar, FileVaultUpload |
| `admin/` | AuditLog, HealthCards, PolicyEditor |
| `shared/` | ScanLine, ZeroizeBar, TerminalError |
| `layout/` | AppShell, Sidebar, Topbar, DetailPanel |

**Services:** `crypto.js`, `ipc.js`, `tauriBridge.js`
**State:** `store/useGrimStore.js` (Zustand)

Full component tree: [ui-layer/src/](ui-layer/src/), detailed docs: [docs/](docs/)

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

## Nutzungsbedingungen

- **Der Quellcode und die Projektstruktur dürfen nicht verändert werden.**
- Das **SDK** darf frei verwendet werden, um Plugins und Erweiterungen zu entwickeln.
- Bei Fragen, Hilfe oder Contribution-Anfragen: **info@blackforest-digital.de**

---

## License

Proprietary. All rights reserved.
