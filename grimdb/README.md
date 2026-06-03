# Grimlocker — Zero-Trust Vault Daemon

[![Go 1.25+](https://img.shields.io/badge/Go-1.25+-00ADD8)](https://go.dev)
[![Security Tier: T3](https://img.shields.io/badge/Security-T3_(Advanced)-brightgreen)](docs/ARCHITECTURE.md)
[![SDKs](https://img.shields.io/badge/SDKs-12-orange)](sdk/SDK_GUIDE.md)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue)](LICENSE)

> **Zero-plaintext in heap. Binary, injection-immune query protocol.  
> 12 SDKs. Secure enclave. LAN sync.**

Grimlocker is a next-generation password vault daemon. It stores all secrets
encrypted at rest with ChaCha20-Poly1305, holds keys exclusively in locked
memory via OS `mlock`/`VirtualLock`, and serves data through a binary GQL
protocol with **total injection immunity** — no text parsing, no SQL,
no JSON injection at any point.

---

## Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        /daemon/                                  │
│  ┌──────────┐  ┌────────────┐  ┌────────────┐  ┌────────────┐  │
│  │ Tauri UI │  │ WebSocket  │  │ REST /api  │  │ IPC (UDS)  │  │
│  │ Browser  │  │ Bridge     │  │ Handler    │  │ Server     │  │
│  └──────────┘  └─────┬──────┘  └─────┬──────┘  └──────┬─────┘  │
│                      │               │                │         │
│                 ┌────▼───────────────▼────────────────▼────┐    │
│                 │            Translator                    │    │
│                 │  (api/translator.go)                    │    │
│                 └────┬───────────────┬────────────────────┘    │
│                      │               │                         │
│                      ▼               ▼                         │
│              ┌────────────┐  ┌──────────────┐                  │
│              │GQL Dispatch│  │Config/Tiers  │                  │
│              │Entry CRUD  │  │Single/Enterpr│                  │
│              └────────────┘  └──────────────┘                  │
├─────────────────────────────────────────────────────────────────┤
│                        /engine/                                  │
│  ┌──────┐ ┌──────────┐ ┌────────┐ ┌──────┐ ┌────────┐ ┌────┐  │
│  │Crypto│ │  Storage │ │Security│ │ GQL  │ │ Kernel │ │Tools│  │
│  │Provider│ │BlockStore│ │Session │ │Frame │ │ Bus    │ │SSH  │  │
│  │ChaCha │ │ Vault    │ │Lockdown│ │Valid  │ │Events  │ │Gen  │  │
│  │Argon2 │ │ Compress │ │Audit   │ │Ops    │ │Modules │ │     │  │
│  └──────┘ └──────────┘ └────────┘ └──────┘ └────────┘ └────┘  │
├─────────────────────────────────────────────────────────────────┤
│                   /core-rust/  (Rust Enclave)                    │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │ SecureZero (7-pass), BLAKE3→HKDF, ChaCha20-Poly1305     │    │
│  │ MLocked handles, session key management, entropy gen    │    │
│  └─────────────────────────────────────────────────────────┘    │
└─────────────────────────────────────────────────────────────────┘
```

### Hexagonal (Ports & Adapters)

The codebase follows **hexagonal architecture**:

| Layer | Path | Responsibility |
|-------|------|---------------|
| **Engine** | `engine/` | Pure data logic — crypto, storage, security primitives, GQL protocol, kernel event bus. ZERO knowledge of HTTP, WebSocket, passwords, or OS. |
| **Daemon** | `daemon/` | Adapters — HTTP/WS/IPC listeners, Tauri UI embed, config wiring, LAN sync, CLI tools. Depends on `engine/` interfaces. |
| **Enclave** | `core-rust/` | Rust shared library for hardened crypto (mlocked key store, BLAKE3, secure zeroization). Optional — pure Go fallback works without it. |

The engine never sees passwords — only pre-hashed `[]byte`. It never opens files
— only abstract `FileSystem` handles. This guarantees **zero plaintext in heap**.

---

## Quick Start

### Build the daemon

```bash
go build -o grimlocker-daemon ./daemon/cmd/daemon/
```

### Run

```bash
./grimlocker-daemon
# Output:
# GRIMLOCKER_TOKEN=<your-session-token>
# GRIMLOCKER_UI=http://127.0.0.1:<port>
```

### Use a language SDK

```typescript
// TypeScript
import { GrimlockerClient } from 'grimlocker-sdk';
const client = new GrimlockerClient('http://127.0.0.1:36353', token);
await client.unlockVault('master-password');
const passwords = await client.listPasswords();
```

All 12 SDKs support the same full API surface.

---

## Security Properties

| Property | Detail |
|----------|--------|
| **Zero plaintext in heap** | Keys held in `mlock`/`VirtualLock`-ed memory. Password strings are `[]byte` that get zeroed after use. |
| **Injection immunity** | Binary GQL protocol with two-stage validation (syntactic + ACL). No text parsing at any point. |
| **At-rest encryption** | ChaCha20-Poly1305 per block. HMAC-SHA256 integrity. Index separately encrypted. |
| **Key derivation** | Argon2id (128MB, 4 iterations — OWASP 2023+) + entropy file XOR (defense-in-depth). |
| **Sync encryption** | Ed25519 mutual auth + ChaCha20-Poly1305 session encryption (no custom crypto). |
| **Session auto-lock** | 15-minute inactivity timeout. Configurable. |
| **Secure deletion** | 7-pass random shred on all key material. SSD-aware overwrite for block data. |
| **Hard lockdown** | After configurable auth failures: key material destroyed, entropy file shredded, process exits. |
| **Audit log** | Append-only ring buffer with SHA-256 chained hashes (tamper-evident). |

---

## SDKs (12 languages)

| Language | Package | Install | Protocol |
|----------|---------|---------|----------|
| Go | `github.com/grimlocker/grimdb/sdk` | `go get` | GQL Binary WS |
| TypeScript | `grimlocker-sdk` | `npm i` | HTTP JSON |
| Python | `grimlocker` | `pip install` | GQL Binary WS |
| Java | `com.grimlocker:grimlocker-sdk` | `mvn install` | GQL Binary WS |
| Rust | `grimlocker-sdk` | `cargo add` | GQL Binary WS |
| C# | `Grimlocker.SDK` | `dotnet add` | HTTP JSON |
| C++ | `grimlocker` | header-only | HTTP JSON |
| Ruby | `grimlocker` | `gem install` | HTTP JSON |
| PHP | `grimlocker/sdk` | `composer require` | HTTP JSON |
| Swift | `grimlocker-sdk` | SwiftPM | HTTP JSON |
| Kotlin | `com.grimlocker:grimlocker-sdk-kotlin` | Gradle | HTTP JSON |
| Dart | `grimlocker_sdk` | `dart pub add` | HTTP JSON |

See [SDK_GUIDE.md](sdk/SDK_GUIDE.md) for full documentation.

---

## Project Structure

```
grimdb/
├── engine/                          # Domain core (zero OS, network, or password knowledge)
│   ├── crypto/                      Provider interface, ChaCha20-Poly1305, Argon2id, HKDF
│   ├── storage/                     BlockStore interface, vault, ingest, compression
│   │   ├── grimdb/                  File-backed BlockStore implementation
│   │   └── strategies/              Pluggable storage strategies (deniable, honeypot)
│   ├── security/                    Session, lockdown, audit log, MVK store, ZKP
│   ├── gql/                         Binary frame protocol, opcodes, validator
│   ├── kernel/                      Event bus, dispatcher, module registry
│   ├── tools/                       SSH key generation (pure crypto)
│   ├── errors/                      GrimlockError, structured logger, stack traces
│   ├── provider/                    Hexagonal port interfaces (VaultProvider, AuthProvider, etc.)
│   └── bridge/                      RustBridge interface (abstraction over enclave)
│
├── daemon/                          # Appliance adapters (HTTP, WS, OS hooks)
│   ├── cmd/                         Entrypoints
│   │   ├── daemon/                  main.go, listeners
│   │   ├── client/                  CLI client tool
│   │   └── healthcheck/            Health check utility
│   └── internal/                    Private adapters
│       ├── api/                     IPC handler, JSON/REST endpoint
│       ├── ws/                      WebSocket bridge (gorilla/websocket)
│       ├── config/                  Tier wiring: single-user, enterprise
│       ├── modules/                 Kernel.WS adapters (crypto, security, tools)
│       ├── security/                Platform-specific mlock/VirtualLock
│       ├── sync/                    LAN sync, mDNS discovery
│       ├── bridge/                  CGO/windows DLL Rust bridge
│       └── mtls/                   Enterprise mTLS
│
├── core-rust/                       Rust secure enclave (DLL/SO)
├── sdk/                             12 language SDKs
├── docs/                            Architecture, GQL protocol, API reference
├── embed.go                         Embedded Tauri UI (go:embed)
├── go.mod
└── README.md
```

---

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture](docs/ARCHITECTURE.md) | System overview, module dependency graph, startup sequence, security properties |
| [GQL Protocol](docs/GQL_PROTOCOL.md) | Binary frame spec, opcodes, validation pipeline, injection immunity |
| [SDK Guide](sdk/SDK_GUIDE.md) | 12-language comparison, quickstarts, package names |
| [API Reference](docs/API_REFERENCE.md) | All exported types, interfaces, methods |
| [Error Codes](docs/ERROR_CODES.md) | All error codes with HTTP status, recovery steps |
| [IPC Message Types](docs/IPC_MESSAGE_TYPES.md) | All 86+ IPC message type constants |
| [Enterprise Features](docs/ENTERPRISE_FEATURES.md) | RBAC, OIDC, mTLS, user management |

---

## License

MIT — see [LICENSE](LICENSE).
