# Development Guide

This document covers the development workflow, conventions, and debugging practices for the Grimlocker codebase.

---

## Development Environment Setup

### Required Tools

| Tool | Min Version | Purpose |
|---|---|---|
| Rust | 1.75+ | `core-rust` compilation, cdylib linker |
| Go | 1.21+ | `grimdb` compilation, CGO linking |
| Node.js | 18+ | `ui-layer` development server |
| `cargo-watch` | latest | Auto-rebuild on Rust changes |
| `golangci-lint` | latest | Go static analysis |
| `cargo-asm` | latest | Verify compiled assembly (security audits) |

### One-Time Setup

```bash
# Clone the repository
git clone <private-repo-url>
cd grimlocker-private

# Install Rust toolchains
rustup target add x86_64-unknown-linux-gnu
rustup target add aarch64-apple-darwin
rustup component add clippy

# Install Go tools
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest

# Build Rust core (required before Go)
cd core-rust
cargo build --release --lib

# Verify Go compiles
cd ../grimdb
go mod tidy
go build ./cmd/daemon

# Setup UI
cd ../ui-layer
npm install
```

---

## Project Structure

```
grimlocker-private/
├── core-rust/                # Rust crypto enclave (cdylib + staticlib)
│   ├── src/
│   │   ├── crypto.rs         # ChaCha20-Poly1305, BLAKE3, mlock, zeroize
│   │   ├── enclave.rs        # Secure memory enclave
│   │   ├── coordinates.rs    # Coordinate parser, panic-key
│   │   ├── lib.rs            # C-ABI exports (extern "C")
│   │   ├── main.rs           # CLI state machine
│   │   ├── time_guard.rs     # Dual-clock integrity
│   │   └── wipe.rs           # 7-pass anti-forensic shredder
│   ├── Cargo.toml
│   └── target/               # Build output
│
├── grimdb/                   # Go daemon
│   ├── kernel/               # Event bus, dispatcher, registry, watchdog
│   ├── crypto/               # Go crypto engine
│   ├── security/             # Security module (lockdown, audit, memlock)
│   │   ├── secret_guard.go   # mlock + guard pages + auto-zeroize wrapper
│   │   ├── rate_limiter.go   # Auth attempt rate limiting (5/10/15/20 failures)
│   │   ├── intrusion_detector.go  # Brute-force + credential stuffing detection
│   │   └── enterprise/       # Enterprise-tier security (RBAC, user mgmt)
│   ├── storage/              # Storage layer (block, compression, ingest)
│   ├── api/                  # IPC, WebSocket, mTLS, handlers
│   ├── sdk/                  # Plugin SDK + language SDKs
│   │   ├── gql_client.go    # Go GQL client (high-level)
│   │   ├── operations.go    # Typed helpers: PasswordEntry, SSHKeyEntry
│   │   ├── example/         # Runnable SDK usage example
│   │   ├── java/            # Java Maven SDK (com.grimlocker:grimlocker-sdk)
│   │   └── python/          # Python pip package (grimlocker)
│   ├── config/               # Tier configuration (single / enterprise)
│   ├── cgo/                  # Go-Rust FFI bridge
│   ├── cmd/                  # Entry points (daemon, client, healthcheck)
│   ├── tools/                # Utility tools (SSH gen, module utils)
│   ├── scripts/              # Operational scripts
│   ├── deploy/               # Deployment configs
│   ├── tests/                # Integration tests
│   ├── docs/                 # Documentation
│   ├── ui-dist/              # Embedded UI build output
│   └── provider/             # Provider interfaces
│
├── ui-layer/                 # Tauri + React frontend
│   ├── src/
│   │   ├── components/       # React components
│   │   ├── services/         # IPC, crypto, tauri bridge
│   │   ├── store/            # Zustand state management
│   │   ├── hooks/            # Custom React hooks
│   │   └── styles/           # TailwindCSS styles
│   ├── package.json
│   └── vite.config.js
│
└── docs/                     # Architecture & security documentation
```

---

## Development Workflow

### Building

```bash
# Rust — watch mode
cd core-rust
cargo watch -x 'check --release'

# Go — build after Rust changes
cd ../grimdb
go build ./cmd/daemon

# UI — dev server with proxy
cd ../ui-layer
npm run dev
# Vite proxies /ws to localhost:8374
```

### Full Build Cycle

```bash
# 1. Rust crypto core
cd core-rust && cargo build --release --lib

# 2. UI (if modified)
cd ../ui-layer && npm run build

# 3. Go daemon (embeds UI)
cd ../grimdb && go build -o grimlocker ./cmd/daemon

# 4. Run
./grimlocker
```

---

## Module Development

### Creating a New Kernel Module

```go
// mymodule/module.go
package mymodule

import (
    "context"
    "github.com/grimlocker/grimdb-private/kernel"
)

type MyModule struct {
    bus       kernel.Bus
    registry  kernel.Registry
}

func (m *MyModule) Name() string    { return "my-module" }
func (m *MyModule) Version() string { return "1.0.0" }

func (m *MyModule) Init(reg kernel.Registry) error {
    m.registry = reg
    m.bus = reg.Bus()
    return nil
}

func (m *MyModule) Start(ctx context.Context) error {
    // Subscribe to events
    m.bus.Subscribe("VAULT.UNLOCKED", m.onVaultUnlocked)
    return nil
}

func (m *MyModule) Stop(ctx context.Context) error {
    m.bus.Unsubscribe("VAULT.UNLOCKED", m.onVaultUnlocked)
    return nil
}

func (m *MyModule) onVaultUnlocked(event kernel.Event) error {
    // Handle event
    return nil
}
```

### Registering the Module

```go
// In cmd/daemon/main.go or config module loader
reg := kernel.NewRegistry()
mod := &mymodule.MyModule{}
reg.Register(mod)
```

### Module Lifecycle Events

```
Register → Init() → Start() → [handles events] → Stop()

Init: Called once during registration. Set up references.
Start: Called when daemon starts. Subscribe to events.
      Return error to prevent daemon startup.
Stop: Called during graceful shutdown. Unsubscribe, flush, clean up.
```

### Event Bus Usage

```go
// Publish
bus.Publish(kernel.Event{
    Type:    "CUSTOM.ACTION",
    Payload: myData,
})

// Subscribe
bus.Subscribe("CUSTOM.ACTION", func(event kernel.Event) error {
    // Handle event
    return nil
})

// Wildcard subscription
bus.Subscribe("CUSTOM.*", handler)
```

---

## Working with the CGO Bridge

### Adding a New FFI Function

**Rust side** (`core-rust/src/lib.rs`):

```rust
#[no_mangle]
pub extern "C" fn my_new_function(
    input_ptr: *const u8,
    input_len: usize,
    output_ptr: *mut u8,
    output_len: *mut usize,
) -> i32 {
    // Validate pointers
    if input_ptr.is_null() || output_ptr.is_null() {
        return -1;
    }

    let input = unsafe { std::slice::from_raw_parts(input_ptr, input_len) };

    // Do crypto work
    let result = perform_operation(input);

    // Copy to output buffer
    unsafe {
        let output = std::slice::from_raw_parts_mut(output_ptr, *output_len);
        output[..result.len()].copy_from_slice(&result);
        *output_len = result.len();
    }

    // Zeroize intermediates
    // ...

    0 // Success
}
```

**Go side** (`grimdb/cgo/rustbridge.go`):

```go
package cgo

/*
#cgo LDFLAGS: -L${SRCDIR}/../../core-rust/target/release -lgrimlocker_core
#include <stdint.h>
#include <stdlib.h>

extern int32_t my_new_function(
    const uint8_t* input_ptr,
    size_t input_len,
    uint8_t* output_ptr,
    size_t* output_len
);
*/
import "C"
import "unsafe"

func MyNewFunction(input []byte) ([]byte, error) {
    outputLen := C.size_t(1024) // Max expected output
    output := C.malloc(outputLen)
    defer C.free(output)

    ret := C.my_new_function(
        (*C.uint8_t)(unsafe.Pointer(&input[0])),
        C.size_t(len(input)),
        (*C.uint8_t)(output),
        &outputLen,
    )
    if ret != 0 {
        return nil, fmt.Errorf("FFI call failed: %d", ret)
    }

    return C.GoBytes(output, C.int(outputLen)), nil
}
```

**Security rules for FFI:**
1. Always validate pointer nullity in Rust before `unsafe`
2. Always zeroize sensitive data in Rust before returning
3. Go must `C.free()` any buffer it allocated via `C.malloc()`
4. Never pass Go-managed memory pointers across CGO without `runtime.KeepAlive()`
5. Test with `go test -race` and `cargo miri test`

---

## Testing

### Running Tests

```bash
# All tests
cd grimdb && go test ./...
cd ../core-rust && cargo test --release

# Specific packages
go test ./crypto/... -v
go test ./security/... -v
go test ./storage/... -v

# Test with race detector
go test -race ./...

# Test with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Writing Tests

```go
// Example: crypto/chacha_test.go
func TestEncryptDecrypt(t *testing.T) {
    key := make([]byte, 32)
    _, err := rand.Read(key)
    require.NoError(t, err)

    plaintext := []byte("secret data")

    ciphertext, err := Encrypt(key, plaintext)
    require.NoError(t, err)
    require.NotEqual(t, plaintext, ciphertext)

    decrypted, err := Decrypt(key, ciphertext)
    require.NoError(t, err)
    require.Equal(t, plaintext, decrypted)
}

func TestDecryptRejectsTampered(t *testing.T) {
    key := make([]byte, 32)
    rand.Read(key)

    ct, _ := Encrypt(key, []byte("data"))
    ct[len(ct)-1] ^= 0xFF // Flip last byte

    _, err := Decrypt(key, ct)
    require.Error(t, err) // Poly1305 tag verification must fail
}
```

### Integration Tests

```go
// Example: tests/lockdown_test.go
func TestLockdownStateMachine(t *testing.T) {
    // Setup: create vault, set up daemon
    // ...

    // Phase 1: Three failed attempts
    for i := 0; i < 3; i++ {
        err := client.Unlock("wrong-password")
        require.Error(t, err)
    }

    // Phase 2: Verify lockdown
    header := client.GetHeader()
    require.True(t, header.LockdownTimestamp > 0)
    require.Equal(t, 4, header.OverrideAttemptsLeft)

    // Phase 3: Correct override
    err := client.Override("correct-coordinates")
    require.NoError(t, err)

    // Phase 4: Verify unlocked
    header = client.GetHeader()
    require.Equal(t, 0, header.FailedAttempts)
    require.Equal(t, 0, header.LockdownTimestamp)
}
```

---

## Debugging

### Go Daemon

```bash
# Run with debug logging
GRIMLOCKER_LOG_LEVEL=debug ./grimlocker

# Attach debugger
dlv debug ./cmd/daemon -- --log-level debug

# Profile with pprof
# http://localhost:8080/debug/pprof/ (if enabled)
go tool pprof http://localhost:8080/debug/pprof/heap
```

### Rust Core

```bash
# Print assembly for critical function
cargo asm --lib core_rust::encrypt_chacha

# Run with miri (undefined behavior detector)
cargo +nightly miri test

# Debug build with symbols
cargo build
gdb target/debug/libgrimlocker_core.so
```

### UI

```bash
# Development mode with hot reload
cd ui-layer && npm run dev

# React DevTools (install browser extension)
# Tauri DevTools (inspect element in Tauri window)

# Debug services
# In tauriBridge.js, set window.__GRIM_DEBUG = true
# Console will show all WebSocket messages
```

### Common Issues

| Symptom | Cause | Fix |
|---|---|---|
| `SIGSYS: bad system call` | mlock in container without `SYS_MLOCK` capability | Add `--cap-add=IPC_LOCK` to Docker |
| `RLIMIT_MEMLOCK too low` | OS limit on locked memory | `ulimit -l 65536` or extend limits.conf |
| Rust build: `cannot find -lgrimlocker_core` | Rust library not built first | `cd core-rust && cargo build --release --lib` |
| Go build: `cgo: C compiler "gcc" not found` | Missing C compiler | Install `build-essential` / Xcode CLT |
| UI: `WebSocket connection failed` | Daemon not running | Start daemon first, check ports |
| Go test panic in CGO call | Rust library incompatible | Rebuild Rust with `cargo clean && cargo build --release --lib` |

---

## Profiling & Benchmarking

### Go Profiling

```bash
# CPU profile
go test -bench=. -cpuprofile=cpu.prof ./crypto/
go tool pprof cpu.prof

# Memory profile
go test -bench=. -memprofile=mem.prof ./crypto/
go tool pprof mem.prof
```

### Rust Profiling

```bash
# Benchmark with criterion (if integrated)
cargo bench

# Profile with perf (Linux)
perf record --call-graph dwarf ./target/release/grimlocker_core
perf report
```

### UI Profiling

```bash
# Vite build analysis
npm run build -- --report

# React profiler (in React DevTools)
```

---

## Version Compatibility

| Component | Version file | Key constraints |
|---|---|---|
| Rust core | `core-rust/Cargo.toml` | `edition = "2021"`, specific crate versions |
| Go daemon | `grimdb/go.mod` | Go 1.21, module path `github.com/grimlocker/grimdb-private` |
| UI | `ui-layer/package.json` | Node 18+, specific npm package versions |

### Cross-Compilation

```bash
# Linux x86_64 (from any OS)
cd core-rust
cargo build --release --lib --target x86_64-unknown-linux-gnu
cd ../grimdb
GOOS=linux GOARCH=amd64 CGO_ENABLED=1 go build -o grimlocker-linux ./cmd/daemon

# macOS ARM64 (from macOS)
cargo build --release --lib --target aarch64-apple-darwin
GOOS=darwin GOARCH=arm64 CGO_ENABLED=1 go build -o grimlocker-darwin ./cmd/daemon
```
