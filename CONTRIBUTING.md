# Contributing to Grimlocker

## Development Philosophy

Grimlocker is a security-first project. Every change must be evaluated through the lens of:

1. **Does this weaken any security property?** If yes, it needs explicit justification and compensating controls.
2. **Does plaintext key material ever touch Go's GC?** Key operations must occur in Rust's locked memory or through CGO FFI.
3. **Are all code paths constant-time where sensitive comparisons occur?**
4. **Is every exit path covered by zeroization?**

---

## Getting Started

### Prerequisites

- Rust 1.75+ with `wasm32-unknown-unknown` and target platform toolchains
- Go 1.21+ with CGO enabled
- Node.js 18+ (for UI development)
- `golangci-lint` (for Go linting)
- `clippy` (included with Rust)

### Repository Setup

```bash
# Clone the private repository
git clone <private-repo-url>
cd grimlocker-private

# Build Rust core
cd core-rust
cargo build --release --lib

# Build Go daemon
cd ../grimdb
go mod tidy
go build ./cmd/daemon

# Setup UI
cd ../ui-layer
npm install
```

### Development Loop

| Component | Watch Command | Tests |
|---|---|---|
| Rust core | `cargo watch -x check` | `cargo test --release` |
| Go daemon | `go run ./cmd/daemon` | `go test ./...` |
| UI layer | `npm run dev` | `npm run test` |

---

## Code Standards

### Rust (`core-rust/`)

- All `unsafe` blocks must have a `// SAFETY:` comment explaining why the invariants hold
- All FFI functions must be `extern "C"` with `#[no_mangle]`
- `zeroize` on every path that touches key material (including error paths)
- Clippy must pass with `-D warnings` on all code
- Public functions must be documented with `///` doc comments
- `#[must_use]` on functions where ignoring the return value is dangerous

### Go (`grimdb/`)

- Follow standard Go conventions (`gofmt`, `go vet`)
- No `panic()` in production code paths (use error returns)
- Sensitive data in `[]byte` must be zeroized with `runtime.KeepAlive`
- Kernel modules must implement `Module` interface from `kernel/event.go`
- Provider interfaces from `provider/interfaces.go` are the extension points
- Use `crypto/rand` only — never `math/rand` for security-sensitive operations

### UI (`ui-layer/`)

- React functional components with hooks only (no class components)
- Zustand for global state, React context only for auth propagation
- TailwindCSS utility classes (no inline styles)
- All text that touches keys/passwords must use `ScanLine` or `ZeroizeBar` for visual obfuscation
- Tauri native APIs only for file system access (token reading)

---

## Branching & Commits

### Branch Naming

```
feature/<description>     # New features
fix/<description>         # Bug fixes
security/<description>    # Security patches
refactor/<description>    # Code refactoring
docs/<description>        # Documentation changes
```

### Commit Messages

```
<type>(<scope>): <description>

[optional body]

[optional footer]
```

Types: `feat`, `fix`, `security`, `refactor`, `docs`, `test`, `chore`

Scopes: `core-rust`, `grimdb`, `ui-layer`, `crypto`, `security`, `storage`, `api`, `kernel`, `config`, `docs`

Example: `security(grimdb): harden constant-time comparison against compiler optimization`

---

## Review Checklist

Before submitting a pull request:

### Security

- [ ] No plaintext keys in logs, errors, or debug output
- [ ] All sensitive buffers zeroized before return
- [ ] Constant-time operations used for all sensitive comparisons
- [ ] No timing differences between success and failure paths

### Rust

- [ ] `cargo clippy --release -- -D warnings` passes
- [ ] `cargo test --release` passes
- [ ] No new `unsafe` without `// SAFETY:` comment
- [ ] All FFI functions properly handle null pointers and invalid input

### Go

- [ ] `go vet ./...` passes
- [ ] `golangci-lint run ./...` passes
- [ ] `go test ./...` passes
- [ ] Error handling covers all failure modes

### UI

- [ ] `npm run build` succeeds
- [ ] No hardcoded keys or credentials
- [ ] No key material exposed in React DevTools
- [ ] Auto-zeroize behavior works correctly

---

## Security Vulnerability Handling

If you discover a security vulnerability:

1. **Do NOT open a public issue.** Vulnerabilities in the private edition must be handled confidentially.
2. Contact the maintainer directly through secure channels.
3. Provide: affected file(s), line numbers, vulnerability type, reproduction steps, suggested fix.

---

## Plugin SDK

To develop a plugin using the GrimDB SDK:

```go
package myplugin

import "github.com/grimlocker/grimdb-private/sdk"

type MyPlugin struct {
    sdk.Plugin
}

func (p *MyPlugin) Name() string { return "my-plugin" }
func (p *MyPlugin) Version() string { return "1.0.0" }

func (p *MyPlugin) Init(registry sdk.Registry) error {
    registry.Subscribe("VAULT.UNLOCKED", p.onUnlock)
    return nil
}

func (p *MyPlugin) onUnlock(event sdk.Event) error {
    // Plugin logic here
    return nil
}
```
