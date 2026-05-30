# Security Model

This document describes the complete security model of the Grimlocker system — every mechanism, assumption, and countermeasure.

---

## Core Principle

**Plaintext data never touches Go's garbage collector or disk.**

All sensitive key material lives exclusively in Rust's memory space, protected by `mlock`/`VirtualLock`, guard pages, and automatic zeroization on deallocation. Go handles orchestration, networking, and storage I/O — but never sees plaintext keys or decrypted payloads.

---

## Key Hierarchy

```
┌─────────────────────────────────────────────────────────────────────┐
│                         KEY DERIVATION TREE                          │
│                                                                     │
│  User Password (human-memorable)                                    │
│       │                                                             │
│       │ + 32-byte CSPRNG salt                                       │
│       ▼                                                             │
│  Argon2id(password, salt, 32MiB, 3, 4)                              │
│       │                                                             │
│       ▼                                                             │
│  Master Key (32 bytes) ────────► BLAKE3(MK) ──► HKDF-SHA256 ──►    │
│       │                         Workspace Keys (32 bytes each)      │
│       │                                                             │
│       ├──────────► BLAKE3(MK) ──► HKDF-SHA256 ──►                   │
│       │             Session Keys (32 bytes, ephemeral)              │
│       │                                                             │
│       └──────────► Entropy File (200+ chars)                        │
│                     │                                               │
│                     │ Coordinate Extraction (user-chosen positions) │
│                     ▼                                               │
│                    BLAKE3(extracted bytes) ──► HKDF-SHA256 ──►      │
│                    Coordinate Override Key (32 bytes)               │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Argon2id Parameters

| Parameter | Value | Rationale |
|---|---|---|
| Memory | 32 MiB | Prevents GPU/ASIC brute-force acceleration |
| Iterations | 3 | Balanced against user experience (login time) |
| Parallelism | 4 | Utilizes multi-core CPUs for attack resistance |
| Salt | 32 bytes CSPRNG | Unique per vault, prevents precomputation |
| Output | 32 bytes | Directly usable as a symmetric key |

### BLAKE3 + HKDF-SHA256

BLAKE3 provides fast, collision-resistant key derivation from the master key. HKDF-SHA256 (RFC 5869) expands the BLAKE3 output into a 32-byte key suitable for ChaCha20-Poly1305, binding the derived key to a specific purpose via the `info` parameter (workspace UUID, session ID, etc.).

### Coordinate Key Extraction

The entropy file contains 200+ bytes of CSPRNG output formatted as a human-readable character matrix. The user provides coordinate positions (e.g., `A3,F12,B7`). The system extracts bytes at those positions, hashes them with BLAKE3, and derives a key via HKDF.

**Panic-key**: The special coordinate `0,0,0` is detected and triggers a disguised vault destruction instead of actual key derivation.

---

## Threat Model

### Adversary Model

Grimlocker defends against:

| Adversary | Capability | Countermeasure |
|---|---|---|
| **Cloud/network surveillance** | Intercept network traffic | 100% local operation, no network dependency, localhost-only ports |
| **Forensic disk analyst** | Recover deleted files, scan for plaintext | No plaintext on disk; 7-pass shredder on wipe; only encrypted `.gdb` file |
| **RAM extraction (cold boot, DMA)** | Read physical memory | mlock/VirtualLock prevents swap; Zeroize on every key use path; guard pages |
| **Compromised OS / root access** | Read process memory, manipulate clocks | Go never sees plaintext; monotonic clock prevents time attacks; lockdown state persisted in encrypted header |
| **Physical coercion (rubber-hose)** | Force user to reveal password | Plausible deniability via panic-key coordinates; deniable volumes; honeypot vaults |
| **Side-channel attacker** | Measure timing, power, EM | Constant-time comparisons; no early exit on password mismatch; ChaCha20 is naturally constant-time |
| **Reboot attacker** | Restart system to bypass lockdown | Lockdown state and boot-ID persisted in `.gdb` header; monotonic ticks cross-checked after reboot |

### Out of Scope (MVP)

| Threat | Status | Plans |
|---|---|---|
| **DMA attacks (Thunderbolt/PCIe)** | Partial | mlock + Zeroize baseline; SGX/SEV enclave planned for enterprise |
| **Hardware keyloggers** | Not addressed | Requires external hardware token (YubiKey integration is SDK-ready) |
| **Supply chain attacks** | Not addressed | Will be addressed via reproducible builds and binary attestation |
| **Spectre/Meltdown class attacks** | Not addressed | Requires hardware mitigation; beyond scope of userspace application |

---

## Lockdown System

### State Machine

```
                      ┌─────────────┐
                      │  UNLOCKED   │
                      └──────┬──────┘
                             │
                      ┌──────▼──────┐
                      │   LOGIN     │
                      │  ATTEMPT    │
                      └──┬──────┬──┘
                         │      │
                    success  fail
                         │      │
              ┌──────────▼──┐   │
              │  UNLOCKED   │   │
              └─────────────┘   │
                                │
                    ┌───────────▼───────────┐
                    │  failed_attempts < 3  │──── retry permitted
                    └───────────────────────┘
                                │
                    failed_attempts >= 3
                                │
              ┌─────────────────▼─────────────────┐
              │          LOCKDOWN MODE             │
              │                                    │
              │  lockdown_timestamp = now()        │
              │  All session keys zeroized         │
              │  Vault unmounted                   │
              │  Only coordinate override available│
              │                                    │
              │  200-minute window                 │
              └────────────────┬──────────────────┘
                               │
                    ┌──────────▼──────────┐
                    │  Coordinate          │
                    │  Override Attempt    │
                    │  (4 attempts max)    │
                    └──┬────────┬─────────┘
                       │        │
                  correct   wrong ×4
                  coords    or timeout
                       │        │
              ┌────────▼──┐  ┌──▼──────┐
              │  UNLOCKED  │  │  WIPE   │
              │  + reset   │  │         │
              │  counters  │  │ 7-pass  │
              └───────────┘  │ shred   │
                             │ unlink  │
                             └─────────┘
```

### Lockdown State Persistence

Lockdown state is stored in the **unencrypted 26-byte header** of the `.gdb` file:

| Field | Purpose |
|---|---|
| `failed_attempts` | Current count (0-3). Reset to 0 on successful login. |
| `lockdown_timestamp` | Unix timestamp when lockdown began. 0 if not in lockdown. |
| `override_attempts_left` | Remaining coordinate override attempts (4-0). |
| `monotonic_boot_ticks` | Monotonic clock value at last header write. Cross-checked after reboot. |
| `wallclock_last_seen` | Last seen wall-clock time. Prevents time rollback attacks. |

### Dual-Clock Integrity (Time-Thief Protection)

The 200-minute lockdown timer is protected against system clock manipulation:

```
┌──────────────────────────────────────────────────────────────┐
│                    TIME INTEGRITY CHECKS                      │
│                                                              │
│  Monotonic Clock (Instant)    Wall Clock (SystemTime)        │
│  │                            │                              │
│  │  Cannot be manipulated     │  Can be set by root          │
│  │  by user or OS time        │                              │
│  │                            │                              │
│  ▼                            ▼                              │
│  elapsed = now - start        if wall_now < wall_last:       │
│  if elapsed < 0:              │                              │
│      → WIPE (regression)      ├──► WIPE (rollback detected)  │
│                               │                              │
│  if elapsed >= 200min:        if wall_now - wall_last > 1yr: │
│      → WIPE (timeout)         ├──► WIPE (anomalous jump)     │
│                               │                              │
│                               Normal case:                   │
│                               └──► Use monotonic elapsed     │
│                                    for lockdown timer        │
└──────────────────────────────────────────────────────────────┘
```

---

## Memory Protection

### Rust Enclave (`core-rust/`)

```
┌──────────────────────────────────────────────────┐
│              MEMORY PROTECTION LAYERS             │
│                                                  │
│  ┌─────┐  Guard Page (PROT_NONE)                │
│  │     │  → SIGSEGV on buffer overflow           │
│  ├─────┤                                        │
│  │ KEY │  Sensitive Buffer                      │
│  │ MAT │  → mlock'd (never swapped)              │
│  │     │  → Zeroize on drop                      │
│  ├─────┤                                        │
│  │     │  Guard Page (PROT_NONE)                │
│  └─────┘  → SIGSEGV on buffer underflow         │
│                                                  │
└──────────────────────────────────────────────────┘
```

**Components:**

| Mechanism | File | Description |
|---|---|---|
| **mlock/VirtualLock** | `crypto.rs` | Locks pages containing key material into physical RAM. Prevents swap. |
| **Zeroize** | `crypto.rs`, `lib.rs`, `wipe.rs` | `zeroize` crate ensures key bytes are overwritten on `Drop`. Compiler barrier prevents optimization. |
| **Guard Pages** | `crypto.rs`, `enclave.rs` | `mmap(PROT_NONE)` pages before/after sensitive allocations. Catches buffer over/under-flows. |
| **Enclave** | `enclave.rs` | Manages allocation/deallocation lifecycle with automatic zeroization. |

### Go Memory Isolation

Go never stores plaintext key material:
- All keys are generated and held in Rust via CGO FFI
- Go receives opaque handles (integers) to reference Rust-held keys
- When Go needs cryptographic operations, it passes data to Rust via CGO
- Go buffers containing encrypted data are zeroized after use

### JavaScript Memory (UI)

The browser/WebView runtime cannot guarantee secure memory:

- **30-second auto-zeroize**: After key display (Single Glance), the buffer is overwritten
- **No copy/paste**: CSS and event handlers block selecting or copying key material
- **GC hint**: Explicit `null` assignment and GC pressure after sensitive data usage
- **Best-effort only**: Warning displayed that JS memory is not cryptographically secure

---

## Anti-Forensic Shredder

When a wipe is triggered (4 failed overrides, lockdown timeout, or panic-key), the vault undergoes:

```
┌────────────────────────────────────────────────────┐
│             7-PASS SHREDDER SEQUENCE                │
│                                                    │
│  1. Open vault.gdb with write access               │
│                                                    │
│  2. Pass 1: Write CSPRNG bytes (file_size)         │
│     → fsync()                                      │
│  3. Pass 2: Write CSPRNG bytes (file_size)         │
│     → fsync()                                      │
│  4. Pass 3: Write CSPRNG bytes (file_size)         │
│     → fsync()                                      │
│  5. Pass 4: Write CSPRNG bytes (file_size)         │
│     → fsync()                                      │
│  6. Pass 5: Write CSPRNG bytes (file_size)         │
│     → fsync()                                      │
│  7. Pass 6: Write CSPRNG bytes (file_size)         │
│     → fsync()                                      │
│  8. Pass 7: Write CSPRNG bytes (file_size)         │
│     → fsync()                                      │
│                                                    │
│  9. Truncate to 0 bytes                            │
│     → fsync()                                      │
│                                                    │
│  10. unlink(vault.gdb)                             │
│                                                    │
│  Each pass uses fresh CSPRNG data — no pattern     │
│  detectable by forensic tools.                     │
└────────────────────────────────────────────────────┘
```

**SSD/NVMe Caveat**: Wear leveling and FTL remapping mean overwrites may not target the same physical NAND cells. This is documented as "best effort" on modern SSDs. For guaranteed physical destruction, use HDDs with direct I/O or FDE with key destruction.

---

## Audit Log

### Immutable Chain

Every security event produces an audit entry with cryptographic chaining:

```
Entry(N) = {
    prevHash:  SHA-256(Entry(N-1))
    timestamp: wall_clock_time
    level:     INFO | WARNING | CRITICAL
    module:    "security" | "policy" | "storage" | "api"
    message:   human_readable_event_description
    subjectID: user | actor_identifier
    hash:      SHA-256(prevHash || timestamp || level || module || message || subjectID)
}
```

Any tampering with an entry breaks the chain because:
1. Entry(N).hash depends on Entry(N-1).hash
2. If Entry(N-1) is modified, Entry(N).hash no longer matches
3. The chain validation will detect the mismatch

### Logged Events

| Event | Level | Module | Trigger |
|---|---|---|---|
| Login attempt | INFO | security | Password submitted |
| Login success | INFO | security | Correct password |
| Login failure | WARNING | security | Wrong password |
| Lockdown triggered | CRITICAL | security | 3 failed attempts |
| Coordinate override | WARNING | security | Lockdown override attempt |
| Override success | INFO | security | Correct coordinates |
| Vault wipe | CRITICAL | security | Wipe triggered |
| Vault create | INFO | storage | New vault created |
| Vault destroy | CRITICAL | storage | Vault destroyed |
| Entry create | INFO | storage | New entry added |
| Entry read | INFO | storage | Entry accessed |
| Entry update | INFO | storage | Entry modified |
| Entry delete | WARNING | storage | Entry removed |
| Unauthorized access | CRITICAL | policy | Permission denied |
| Integrity mismatch | CRITICAL | integrity | Binary hash changed |
| Kernel restart | WARNING | kernel | Watchdog triggered restart |
| Module loaded | INFO | kernel | Module registered |
| Module failed | CRITICAL | kernel | Module startup/operation failed |

---

## Panic-Key / Plausible Deniability

### Panic-Key Coordinates

The coordinate `0,0,0` is a special panic-key that triggers a **disguised wipe**:

1. User enters `0,0,0` as their coordinate override
2. System displays normal verification messages: "Verifying... OK", "Decrypting... OK", "Loading entries... Done."
3. In the background, the vault is being shredded (7-pass wipe)
4. The UI transitions normally, then reveals an empty or honeypot vault
5. An attacker cannot distinguish this from a successful unlock

### Deniable Volumes

Workspaces can be configured as deniable volumes:
- Hidden within the encrypted payload's free space
- Not distinguishable from random padding
- Only accessible with a separate password/coordinate set
- If the user reveals their main password, the deniable volume remains hidden

### Honeypot Vaults

Decoy vaults that:
- Appear to contain real secrets
- Are accessible with a "distress password"
- Log all access for detection of coercion
- Automatically trigger alerts or wipe on specific access patterns

---

## Session Security

### Session Lifecycle

```
┌──────────┐    Login     ┌─────────────┐    Expiry/Timer    ┌──────────┐
│ LOGGED   │ ───────────► │ SESSION     │ ─────────────────► │ LOGGED   │
│ OUT      │              │ ACTIVE      │                    │ OUT      │
│          │              │             │                    │          │
│          │              │ Session Key │                    │ All keys │
│          │              │ derived     │                    │ zeroized │
│          │              │ mounted VFS │                    │ VFS      │
│          │              │             │                    │ unmounted│
└──────────┘              └─────────────┘                    └──────────┘
                                │
                                │ Lockdown / Panic / Manual Lock
                                │
                                ▼
                           ┌──────────┐
                           │ LOCKED   │
                           │          │
                           │ All keys │
                           │ zeroized │
                           └──────────┘
```

### Session Key Management

- Session keys are ephemeral — re-derived from the master key for each session
- Session timeout is configurable (default: 30 minutes idle)
- On timeout, lock, or lockdown: all session keys immediately zeroized
- No session keys persist across daemon restarts
- VFS is unmounted on session end, requiring re-derivation on next login

---

## Platform-Specific Considerations

### Linux

- `mlock` via `mlockall`/`mlock` syscalls
- `RLIMIT_MEMLOCK` may need configuration for large locked allocations
- Transparent Huge Pages (THP) may reduce mlock effectiveness (documented tradeoff)

### macOS

- `mlock` available but limited; works best for small allocations (<64 KB)
- Guard pages via `mmap(PROT_NONE)`
- Sandbox considerations: Tauri app may need entitlements

### Windows

- `VirtualLock` via `VirtualLock` Win32 API
- Requires `SeLockMemoryPrivilege` for the process
- Guard pages via `VirtualAlloc(PAGE_NOACCESS)`
- Full support requires WSL2

---

## Security Assumptions

1. **Operating system provides functional `mlock`/`VirtualLock`.** If the OS swaps locked pages, memory protection fails.
2. **CPU provides constant-time instructions for comparison operations.** Compiler optimizations must be verified via disassembly for critical paths.
3. **CSPRNG (`/dev/urandom`, `getrandom`) is not compromised.** All key material depends on this entropy source.
4. **Rust compiler optimizations respect `zeroize` crate barriers.** Verified via `cargo-asm` on critical functions.
5. **Go CGO boundary crossing does not leak stack/heap data.** Go's runtime does not copy CGO buffers unexpectedly.
6. **File system honors `fsync`.** On some configurations, `fsync` may be a no-op (documented as a risk).
