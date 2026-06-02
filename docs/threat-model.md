# Threat Model

This document formally models the threats Grimlocker defends against, using a structured approach based on the STRIDE methodology with attack trees for critical paths.

---

## Adversary Model

### Adversary Tiers

| Tier | Name | Capabilities | Resources |
|---|---|---|---|
| **T1** | Casual Attacker | Access to the machine after the user has walked away; basic forensic tools | Individual, free tools |
| **T2** | Technical Attacker | Root/administrator access to the OS; RAM imaging tools; disk imaging; packet capture | Individual, professional tools |
| **T3** | Organized Attacker | Physical access; hardware implants; DMA attacks; side-channel equipment; coercive interrogation | Organization, dedicated hardware |
| **T4** | Nation-State | Supply chain compromise; zero-day exploits; custom silicon; legal compulsion; unlimited surveillance | Unlimited |

### Scope

Grimlocker **defends** against T1, T2, and partially T3. T4 is acknowledged as out of scope for the current MVP.

---

## STRIDE Analysis

### Spoofing

| Threat | Attack Vector | Defense | Tier |
|---|---|---|---|
| Impersonate the daemon to capture passwords | Replace or MITM the WebSocket connection | Token authentication (32-byte CSPRNG, 0600 file); localhost-only binding | T2 |
| Impersonate a valid client to access the vault | Steal the `.grim_token` file | Token is 0600 and file is deleted on daemon exit; short-lived (process lifetime) | T2 |
| Spoof integrity hash to hide tampering | Modify the known-good hash in the vault header | Hash is encrypted with the master key; requires successful unlock first | T3 |
| Impersonate a trusted sync peer on LAN | Spoof mDNS response or hijack peer IP | Ed25519 challenge/response; public key pinned after 6-digit PIN pairing | T2 |
| Replay captured sync traffic | Capture and re-inject valid sync frames | Monotonic nonce per session; replayed frames fail Poly1305 tag verification | T2 |

### Tampering

| Threat | Attack Vector | Defense | Tier |
|---|---|---|---|
| Modify `.gdb` ciphertext to inject data | Direct file write | Poly1305 tag verification fails on modified ciphertext; tampering is detected | T2 |
| Modify `.gdb` header to reset lockdown counter | Direct file write of header bytes | Monotonic ticks cross-check detects time reversion; integrity hash includes header | T2 |
| Modify the binary to skip password verification | Replace the grimlocker binary on disk | Binary integrity hash checked at startup and every 30s; mismatch triggers watchdog restart | T3 |
| Modify audit log entries | Direct file write | Each entry is cryptographically chained with SHA-256; any modification breaks the chain | T2 |
| Modify sync data in transit | MITM between two syncing devices | ChaCha20-Poly1305 AEAD encrypts all sync traffic; Poly1305 tag verification on every block | T2 |
| Inject malicious entry via sync | Send crafted entry blocks to peer | Poly1305 tag verified before writing to local vault; entry schema validated post-decrypt | T2 |

### Repudiation

| Threat | Attack Vector | Defense | Tier |
|---|---|---|---|
| Deny that a vault was accessed | No audit trail | All security events are logged in the immutable audit chain | T1 |
| Delete audit log to hide access | Direct file deletion or truncation | Audit log is append-only and chained; missing entries break the chain | T2 |

### Information Disclosure

| Threat | Attack Vector | Defense | Tier |
|---|---|---|---|
| Read plaintext keys from swap | OS paged locked memory to disk | mlock/VirtualLock prevents sensitive pages from being swapped | T2 |
| Read plaintext keys from process memory | Read /proc/pid/mem or kernel dump | Keys only in Rust heap (not Go GC); zeroized after each use; guard pages | T2 |
| RAM extraction via cold boot or DMA | Physical RAM imaging | mlock keeps keys in RAM (not swap); zeroization minimizes window; SGX planned for T3+ | T3 |
| Intercept plaintext during network transfer | Packet capture on localhost | No network communication; localhost-only; WebSocket is local (127.0.0.1) | T1 |
| Recover plaintext from deleted files | Forensic disk recovery | No plaintext on disk; 7-pass CSPRNG overwrite on wipe; only encrypted `.gdb` exists | T2 |
| Read entropy file to derive keys without password | File read of `entropy.bin` | Entropy file is 0600; requires both password and entropy for key derivation | T2 |
| Eavesdrop on LAN sync traffic | Packet capture on local network | All sync traffic is ChaCha20-Poly1305 encrypted; only ciphertext on wire — **Active** | T2 |
| Extract device identity key from disk | Read `~/.grimlocker/device.key` | File is 0600; requires user or root access to the device | T2 |
| Fingerprint sync devices via mDNS | Passive mDNS monitoring on LAN | mDNS device ID is a truncated hash, not the public key; version vectors reveal only version numbers — **Active** | T1 |
| Clipboard history reveals copied passwords | System clipboard history (Windows, KDE, etc.) | Auto-clear erases clipboard after configurable timeout (default 30s) — **Implemented** | T1 |
| App persists after user believes it is closed | Window hidden via minimize-to-tray | System tray icon always visible; explicit Quit required to terminate — **Implemented** | T1 |

### Denial of Service

| Threat | Attack Vector | Defense | Tier |
|---|---|---|---|
| Lock the user out permanently | Repeated failed login attempts | Lockdown is 200-min window, not permanent; coordinate override available | T1 |
| Destroy vault data | Trigger wipe through failed overrides | 4-attempt limit ensures legitimate user can always recover; backup recommended | T2 |
| Fill disk to prevent vault writes | Write large files to the vault directory | File size tracking; disk space checks before block writes | T2 |
| Sync flood (DoS) | Rapid repeated sync requests from peer | Rate limiting: max 1 sync session per 60s per peer; connection timeout 30s | T2 |

### Elevation of Privilege

| Threat | Attack Vector | Defense | Tier |
|---|---|---|---|
| Skip authorization and access all entries | Directly read block store without policy check | Block store is encrypted; decryption requires valid session key; policy is enforced at API layer | T2 |
| Escalate to enterprise admin privileges | Modify config to change tier | Tier is set at compile time in the Go binary; enterprise features require valid Keycloak token | T3 |
| Access another user's workspace | Read workspace blocks directly | Each workspace has independent encryption key derived from master + workspace UUID | T3 |
| Access entries via sync without vault unlock | Trigger sync on a locked vault | Sync scheduler is gated by session context; sync only runs when vault is unlocked | T2 |

---

## Attack Trees

### Attack: Extract Plaintext Master Key

```
Goal: Obtain the master key in plaintext form
├── AND Attack vector 1: Compromise running process
│   ├── Read Go heap        → FAIL: Keys never in Go-managed memory
│   ├── Read Rust heap      → Requires process memory access (root/T2)
│   │   ├── /proc/pid/mem   → MITIGATED: mlock + guard pages
│   │   ├── ptrace attach   → MITIGATED: Process hardening planned
│   │   └── kernel module   → ACCEPTED: T4 adversary
│   └── Read swap           → MITIGATED: mlock prevents paging
│
├── AND Attack vector 2: Brute-force password
│   ├── Online attack       → MITIGATED: Lockdown after 3 attempts
│   └── Offline attack      → MITIGATED: Argon2id (32 MiB, 3 iterations) limits GPU speed
│       └── Low-entropy pwd → MITIGATED: Password strength requirements + coordinate layer
│
├── AND Attack vector 3: Side-channel
│   ├── Timing              → MITIGATED: Constant-time comparisons
│   ├── Power analysis      → ACCEPTED: Requires physical access (T3)
│   └── Cache timing        → ACCEPTED: No Spectre-class mitigation in MVP
│
└── AND Attack vector 4: Coercion (rubber-hose)
    ├── Force password reveal → MITIGATED: Panic-key coordinates, deniable volumes
    └── Force coordinate reveal → MITIGATED: Panic-key (0,0,0) triggers disguised wipe
```

### Attack: Bypass Lockdown Timer

```
Goal: Unlock vault during 200-minute lockdown period
├── AND Attack vector 1: Manipulate system clock
│   ├── Set clock forward 200+ minutes    → MITIGATED: Monotonic clock used for timer
│   ├── Set clock backward                → DETECTED: Wall-clock cross-check triggers wipe
│   └── Freeze clock                      → MITIGATED: Monotonic clock always advances
│
├── AND Attack vector 2: Manipulate .gdb header
│   ├── Reset lockdown_timestamp          → MITIGATED: Monotonic ticks cross-check
│   ├── Reset failed_attempts             → MITIGATED: Integrity hash & audit log chain
│   └── Replace entire header             → MITIGATED: Monotonic tick + wallclock correlation
│
├── AND Attack vector 3: Reboot to reset state
│   ├── Cold restart                      → MITIGATED: Lockdown state persisted in .gdb header
│   ├── Boot different OS                 → MITIGATED: Vault file still encrypted; header still shows lockdown
│   └── Boot-ID tracking                  → MITIGATED: Monotonic boot ticks detect new boot
│
└── AND Attack vector 4: Exploit implementation
    ├── Integer overflow in timer         → REVIEW: lockdown_timestamp is int64, checked
    ├── Race condition in counter update  → REVIEW: Atomic writes to .gdb header
    └── CGO memory corruption             → REVIEW: All FFI functions validate buffer lengths
```

### Attack: Recover Deleted Vault Data

```
Goal: Recover plaintext data after a vault wipe
├── AND Attack vector 1: Undelete vault file
│   ├── File recovery tools              → MITIGATED: 7-pass CSPRNG overwrite before unlink
│   ├── Journal recovery (ext4, NTFS)    → MITIGATED: Data overwritten, not just metadata
│   └── Backup/snapshot recovery         → MITIGATED: User responsible for backup hygiene
│
├── AND Attack vector 2: Read physical disk
│   ├── Magnetic force microscopy (HDD)  → PARTIAL: 7-pass overwrite sufficient for HDDs
│   └── NAND cell recovery (SSD)         → PARTIAL: FTL remapping may preserve old data
│                                          MITIGATION: Document SSD caveat; recommend FDE
│
├── AND Attack vector 3: Memory forensics
│   ├── RAM dumps from live system       → MITIGATED: Zeroization on wipe; process likely exited
│   └── Cold boot attack                 → ACCEPTED: Keys in RAM at wipe time (quick zeroize)
│
└── AND Attack vector 4: Clipboard/screen capture
    ├── Clipboard history                 → MITIGATED: Auto-clear after configurable timeout (default 30s)
    └── Screenshot of Single Glance       → MITIGATED: 30-second timer; blocked screenshots (Tauri config)
```

### Attack: Intercept LAN Sync Traffic

```
Goal: Read or inject data during a sync session between two devices
├── AND Attack vector 1: Passive eavesdropping
│   ├── Capture sync packets on LAN       → MITIGATED: ChaCha20-Poly1305 encrypts all traffic
│   └── Traffic analysis (size, timing)    → ACCEPTED: Limited info leak (entry count, sync frequency)
│
├── AND Attack vector 2: Rogue device impersonation
│   ├── Spoof mDNS advertisement           → MITIGATED: Ed25519 challenge/response verifies identity
│   ├── Guess/Crack 6-digit PIN            → MITIGATED: PIN is one-time use; after pairing, pubkey is pinned
│   └── Steal device.key from disk         → MITIGATED: 0600 permissions; requires root (T2)
│
├── AND Attack vector 3: Active MITM
│   ├── ARP spoofing + intercept TCP       → MITIGATED: Ed25519 signatures prevent impersonation
│   ├── TLS downgrade (if TLS wrapped)     → MITIGATED: Native ChaCha20-Poly1305, no TLS dependency
│   └── Forward secrecy compromise         → MITIGATED: Ephemeral session keys via X25519 ECDH per session
│
├── AND Attack vector 4: Sync state manipulation
│   ├── Modify local sync_state.json       → MITIGATED: Version vector verified against encrypted blocks
│   ├── Version downgrade (send old v)     → MITIGATED: Version only increases; lower version ignored
│   └── Clock skew exploitation            → PARTIAL: version is primary comparator; timestamp is fallback
│
└── AND Attack vector 5: Replay attack
    ├── Capture valid sync frames           → MITIGATED: Monotonic nonce; replayed frames fail Poly1305 tag
    └── Force re-sync of stale data         → MITIGATED: Version comparison ensures only newer data is accepted
```

---

## Risk Assessment Matrix

| Threat | Likelihood | Impact | Risk Level | Mitigation Status |
|---|---|---|---|---|
| Password brute-force (offline) | Medium | Critical | **High** | Argon2id makes it infeasible |
| RAM extraction (cold boot) | Low | Critical | **Medium** | mlock + zeroize minimize window |
| SSD forensic recovery after wipe | Medium | Critical | **High** | 7-pass best effort; SSD caveat documented |
| System clock manipulation | Low | High | **Low** | Dual-clock integrity |
| Coercion (physical threat) | Low | Critical | **Medium** | Panic-key + deniable volumes |
| Side-channel (timing) | Medium | High | **Medium** | Constant-time comparisons |
| Side-channel (power/EM) | Very Low | Critical | **Low** | Requires physical access (T3) |
| Supply chain (binary tampering) | Low | Critical | **Medium** | Integrity verification at startup |
| Clipboard history capture | Medium | Medium | **Low** | Auto-clear after configurable timeout (default 30s) — **Implemented** |
| DMA attack (Thunderbolt/PCIe) | Low | Critical | **Medium** | mlock baseline; SGX planned |
| LAN sync eavesdropping | Medium | High | **Low** | ChaCha20-Poly1305 + X25519 ECDH — **Active (daemon live)** |
| Sync peer impersonation | Low | Critical | **Medium** | Ed25519 challenge/response + pinned public keys — **Active** |
| Sync replay attack | Low | High | **Low** | Monotonic nonce + Poly1305 tag verification — **Active** |
| Sync version downgrade | Low | Medium | **Low** | Monotonic version numbers; lower version ignored — **Active** |
| Password-group metadata in localStorage | Low | Low | **Low** | Group names (no secrets) in plaintext localStorage; same trust domain as session |
| Window hidden (minimize-to-tray); process persists | Medium | Medium | **Low** | System tray icon always visible; auto-lock fires; explicit Quit action |
| Extended unlock window during active sync | Low | Medium | **Low** | Sync gated by sessionCtx; auto-lock still applies during sync sessions |

---

## Defense-in-Depth Layers

```
Layer 1: Physical
  └── Machine under user's physical control
  └── No network exposure (localhost only)

Layer 2: OS / Platform
  └── File permissions (0600)
  └── mlock / VirtualLock (kernel protects memory)
  └── Process isolation (Tauri sandbox)

Layer 3: Cryptographic
  └── Argon2id password hashing (memory-hard)
  └── ChaCha20-Poly1305 AEAD (authenticated encryption)
  └── Poly1305 tag (tamper detection)

Layer 4: Application
  └── Lockdown state machine (brute-force prevention)
  └── Dual-clock integrity (time attack prevention)
  └── Constant-time comparisons (side-channel prevention)
  └── Audit log with cryptographic chaining

Layer 5: Operational
  └── 30-second auto-zeroize (UI)
  └── 7-pass anti-forensic shredder
  └── Binary integrity verification
  └── Watchdog with automatic restart

Layer 6: Network Sync (Single User)
  └── Ed25519 device identity with PIN pairing
  └── ChaCha20-Poly1305 encrypted sync sessions (X25519 ECDH)
  └── Monotonic version vectors (conflict detection)
  └── Rate-limited auto-pull scheduler (vault-unlocked gated)
  └── mDNS discovery (local-network only, TTL=1)
```

---

## Assumptions & Limitations

### Assumptions That Must Hold

1. **CSPRNG is trustworthy.** All key material derives from `getrandom`/`/dev/urandom`. If this is compromised, all keys are compromised.
2. **Argon2id is cryptographically sound.** No known preimage attacks exist. Parameter choices (32 MiB, 3 iterations) assumed sufficient for T1-T3 adversaries.
3. **ChaCha20-Poly1305 is secure.** As specified in RFC 8439. No known attacks against the AEAD construction.
4. **Constant-time code is actually constant-time.** Verified via `cargo-asm` and manual review. Compiler optimizations must be monitored across versions.
5. **OS honors mlock/VirtualLock.** The kernel prevents locked pages from being swapped. Some configurations (THP, overcommit) may weaken this.
6. **File system honors fsync.** Data is actually flushed to disk after each overwrite pass. Some hardware/FS configurations may defer writes.

### Known Limitations

| Limitation | Description | Impact |
|---|---|---|
| **SSD FTL remapping** | NAND flash translation layer may preserve old data after overwrite | Physical destruction of SSD may recover data after wipe |
| **No TEE/SGX in MVP** | Enclave runs in userspace, not hardware-protected | Kernel-level attacker can read Rust memory directly |
| **BLAKE3 is fast** | Not memory-hard for key derivation step | Brute-force of coordinate keys faster than Argon2id |
| **JavaScript memory** | Browser/WebView cannot guarantee memory zeroization | Key may linger in JS heap after Single Glance |
| **CGO build coupling** | Rust must be built before Go | Dependency chain can fail in CI without proper ordering |
| **Watchdog restarts** | After restart, modules reload from scratch | Brief window where security is in initialization state |
| **JS localStorage not encrypted** | Password groups and UI preferences stored in plaintext localStorage | No vault secrets involved — only metadata; same trust domain as the session |

### Future Enhancements

| Enhancement | Would Address | Priority | Status |
|---|---|---|---|
| **Clipboard auto-clear** | Clipboard history capture | **Medium** | ✅ Implemented (configurable timeout) |
| **Auto-lock wired to preferences** | Inactivity-based key exposure | **Medium** | ✅ Implemented (0 = disabled) |
| **System tray minimize-to-tray** | Process visibility / accidental close | **Medium** | ✅ Implemented |
| **LAN Sync** | Encrypted cross-device vault sync | **High** | ✅ Active (daemon live, ChaCha20+Ed25519) |
| **SGX/SEV enclave** | Kernel-level memory access, DMA attacks | **High** | Planned (Enterprise) |
| **Reproducible builds** | Supply chain trust | **High** | Planned |
| **Argon2id for coordinate keys** | Memory-hardness for override path | **Medium** | Planned |
| **YubiKey/SoloKey integration** | Phishing resistance, hardware-bound keys | **Medium** | Planned (SDK ready) |
| **FDE key destruction** | Guaranteed data destruction on SSDs | **Medium** | Planned |
| **Enhanced eBPF/seccomp filter** | Reduce kernel attack surface | **Low** | Planned |
| **LAN Sync: Forward secrecy hardening** | Long-term session key compromise | **Medium** | Planned |
| **LAN Sync: Entry-level Ed25519 signatures** | Tamper-proof sync provenance per entry | **Low** | Planned |
| **LAN Sync: Encrypted sync state** | Metadata protection for sync_state.json | **Low** | Planned |
