# Modern Security Architecture

This document describes the advanced security subsystems introduced in Phase 6+: memory protection, post-quantum readiness, zero-knowledge proofs, and binary integrity.

---

## Memory Protection — SecretGuard

**File:** `grimdb/security/secret_guard.go`

`SecretGuard` is a wrapper around sensitive byte slices that:

1. Calls `mlock`/`VirtualLock` to prevent the OS from swapping the pages to disk
2. Places guard pages before and after the allocation to detect buffer overruns
3. Automatically zeroizes memory (`crypto/subtle.ConstantTimeCopy` to zero buffer) on `Close()`
4. Panics with a structured log entry (no sensitive data) if the guard pages are touched

Usage pattern:

```go
guard, err := security.NewSecretGuard(32)
defer guard.Close()    // zeroizes on exit regardless of error path

copy(guard.Bytes(), sensitiveKey)
// ... use guard.Bytes() ...
// guard.Close() wipes automatically
```

Key material lifecycle:
```
Allocation → mlock → use → explicit zeroize → munlock → free
                                    ↑
                             guard.Close() / defer
```

SecretGuard is used by: vault unlock path, SKE session key, master vault key (MVK) handle.

---

## Post-Quantum Crypto Readiness

**File:** `grimdb/crypto/pqc_ready.go`

Grimlocker's current symmetric primitives (ChaCha20-Poly1305, BLAKE3) are already quantum-resistant — symmetric keys ≥ 256 bits survive Grover's algorithm with 128-bit post-quantum security.

`pqc_ready.go` prepares the key exchange layer for a future hybrid KEM (Key Encapsulation Mechanism):

```
Current:  ECDH (X25519)  →  session key
Future:   X25519 + Kyber-768  →  XOR-combined session key
```

The module provides:
- `IsPQCEnabled() bool` — feature flag (off by default, enabled via build tag `+pqc`)
- `HybridKeyExchange(peerPub []byte) ([]byte, error)` — placeholder returning X25519 output today, hybrid output when `+pqc` is active
- Migration path: all call sites use `HybridKeyExchange` already; flipping the build tag is the only change needed

---

## Zero-Knowledge Proof

**File:** `grimdb/security/zkp.go`

Used in the vault unlock path to prove knowledge of the passphrase without transmitting it to the Go layer.

Protocol (simplified Schnorr-style):

```
1. Client derives  k = BLAKE3(passphrase || salt)
2. Client computes commitment  C = H(k || nonce)
3. Client sends C to daemon
4. Daemon challenges with random r
5. Client responds with  s = k XOR H(r)
6. Daemon verifies  H(s XOR H(r)) == C
```

The passphrase-derived key `k` never crosses the Go/Rust boundary in plaintext. The daemon verifies the proof in the Rust enclave via CGO.

This eliminates one attack surface: a compromised Go heap cannot extract the vault key even if the unlock flow is instrumented.

---

## Rust Enclave Integration

**Files:** `core-rust/src/enclave.rs`, `grimdb/cgo/rustbridge.go`

The Rust enclave is a `cdylib` that exports a C ABI:

| Function | Purpose |
|---|---|
| `grimlocker_derive_key(passphrase, salt, out_key)` | BLAKE3-derive 256-bit vault key in locked memory |
| `grimlocker_encrypt(key_handle, plaintext, nonce, out_ct)` | ChaCha20-Poly1305 encrypt |
| `grimlocker_decrypt(key_handle, ct, nonce, out_pt)` | ChaCha20-Poly1305 decrypt |
| `grimlocker_wipe(path)` | 7-pass anti-forensic file shred |
| `grimlocker_zeroize(ptr, len)` | Constant-time memory zeroing |
| `grimlocker_mlock(ptr, len)` | Lock memory pages against swap |

Go calls these via CGO (`grimdb/cgo/rustbridge.go`). The key material lives in Rust's heap — Go only holds opaque handles.

Memory protection guarantees from Rust:
- `mlock`/`VirtualLock` on all key allocations
- Guard pages via `mprotect` (`PROT_NONE`) around sensitive buffers
- Automatic `zeroize` on `Drop` (Rust's ownership model enforces this statically)

---

## Binary Integrity Check

**File:** `grimdb/security/` (startup check)

On startup, the daemon verifies its own binary and the Rust cdylib:

1. Reads the embedded SHA-256 hash from a `.hashes` file baked in at build time
2. Computes the runtime SHA-256 of `grimdb-daemon` and `grimlocker_core.dll`/`.so`
3. If they diverge → logs `INTEGRITY_VIOLATION`, refuses to start, and writes an audit event

This detects:
- Tampered daemon binary (supply-chain attack)
- Replaced crypto library
- Accidental deployment of a debug build in production

The check is bypassed in `debug_assertions` mode to allow development iteration.

---

## Defense-in-Depth Summary

```
Network layer:      mTLS (Enterprise) / token auth (Single-User)
                         ↓
IPC layer:          Binary GQL protocol, injection-immune
                         ↓
Application layer:  Two-stage validator → ACL check → dispatcher
                         ↓
Memory layer:       SecretGuard (mlock + guard pages + auto-zeroize)
                         ↓
Crypto layer:       Rust enclave (ChaCha20-Poly1305, BLAKE3, X25519)
                         ↓
Storage layer:      All data encrypted at rest; plaintext never hits disk
                         ↓
Physical layer:     7-pass wipe, Panic Button, panic-key deception path
                         ↓
LAN Sync layer:     Ed25519 device identity + PIN pairing + ChaCha20-Poly1305 sessions
                    (see security-model.md: Local Network Sync)
```
