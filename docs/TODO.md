# Grimlocker TODO

## Completed

- [x] Core crypto enclave (Rust) — ChaCha20-Poly1305, BLAKE3, mlock, zeroize
- [x] Event-driven kernel — Bus, Dispatcher, Registry, Watchdog (30s)
- [x] Single User vault — Argon2id authentication, local `.gdb` file storage
- [x] Enterprise tier — OIDC/JWT (Keycloak), S3/MinIO storage, mTLS
- [x] Tauri desktop shell — React frontend + Rust binary bundling
- [x] SecretGuard memory protection — mlock + guard pages + auto-zeroize
- [x] Post-quantum crypto readiness — X25519 + Kyber-768 hybrid KEM skeleton
- [x] Zero-knowledge proof vault unlock — Schnorr-style protocol
- [x] Binary integrity check — SHA-256 at startup + periodic re-check
- [x] 7-pass anti-forensic shredder — CSPRNG overwrite + fsync + unlink
- [x] Dual-clock integrity — monotonic + wall-clock cross-check
- [x] Lockdown state machine — 200-min window, 4 override attempts, panic-key
- [x] Panic-key (0,0,0) — disguised wipe, deniable volumes, honeypot vaults
- [x] Audit log — SHA-256 chained, append-only, ring buffer
- [x] Workspace management — multi-tenant vaults with independent keys
- [x] File vault ingestion — io.Pipe streaming, chunked encryption
- [x] GQL binary protocol — injection-immune query language
- [x] IPC (Unix socket / WebSocket) — localhost-only token auth
- [x] Rate limiter + intrusion detector — anomaly-based threat detection
- [x] Python + Java SDKs — pip package + Maven artifact
- [x] **TypeScript/JavaScript SDK** — `@grimlocker/sdk` wrapping HTTP `/api/v1` endpoint
- [x] Security documentation — security-model, threat-model (STRIDE), crypto-spec
- [x] **Session key two-pass zeroization** — random overwrite + zero, prevents V8 dead-store elision
- [x] **CSP hardening** — removed `unsafe-inline`, eliminated Google Fonts external dependency
- [x] **WebSocket zombie cleanup** — broadcast write failures now remove stale connections
- [x] **Auto-lock wired to preferences** — configurable timeout (0 = disabled)
- [x] **Clipboard auto-clear** — configurable timeout after copy
- [x] **System tray** — minimize-to-tray with Show/Quit menu

### Local Network Sync (Single User) — COMPLETE
- [x] Device identity — Ed25519 keypair + PIN pairing
- [x] mDNS discovery — Zeroconf `_grimlocker._tcp`
- [x] Sync protocol — ChaCha20-Poly1305 encrypted, X25519 ECDH session key
- [x] Version vectors — monotonic version + updated_at per entry
- [x] Conflict resolution — last-write-wins (version primary, timestamp fallback)
- [x] Sync scheduler — background auto-pull every 60s, vault-unlocked gated
- [x] Audit integration — sync events in immutable audit chain
- [x] Daemon wiring — IPC bridge (listSyncPeers, triggerSync, listAuditEntries)

---

## Pending

### High Priority
- [ ] **Reproducible builds** — Supply chain trust, binary attestation
- [ ] **SGX/SEV enclave** — Hardware-protected memory for Enterprise tier (T4 defense)

### Medium Priority
- [ ] **YubiKey/SoloKey integration** — Phishing-resistant hardware-bound keys (SDK ready)
- [ ] **FDE key destruction** — Guaranteed data destruction on SSDs via full-disk-encryption key wipe
- [ ] **Argon2id for coordinate keys** — Memory-hardness for the coordinate override path
- [ ] **eBPF/seccomp filter** — Reduce kernel attack surface

### Low Priority
- [ ] **LAN Sync: Entry-level Ed25519 signatures** — Tamper-proof sync provenance per entry
- [ ] **LAN Sync: Encrypted sync state** — Metadata protection for `sync_state.json`
- [ ] **SAML 2.0 / LDAP identity providers** — Extended enterprise federation (IdentityProvider interface exists)
- [ ] **Biometric auth plugin** — WebAuthn/FIDO2 via BiometricAuthenticate interface
- [ ] **Hardware security module (HSM) integration** — PKCS#11 for enterprise key storage
- [ ] **PolicyEditor backend** — RBAC policy persistence + daemon handler (UI skeleton exists)

---

## Known Issues / Technical Debt

| Issue | Impact | Mitigation |
|---|---|---|
| SSD FTL remapping | Wear leveling may preserve old data after 7-pass shredder | Documented caveat; recommend FDE |
| JavaScript GC can't guarantee zeroization | Key material may linger in WebView heap after Single Glance | Two-pass zeroization (random + zero) minimizes window; 30s auto-zeroize |
| CGO build coupling | Rust must be built before Go; fragile CI dependency chain | Documented build order in development.md |
| No TEE/SGX in MVP | Kernel-level attacker can read Rust heap directly | Planned for Enterprise; SecretGuard is userspace best-effort |
| BLAKE3 is not memory-hard | Coordinate override key derivation faster than Argon2id | Medium priority: replace with Argon2id for coordinate path |
| Watchdog restart window | Brief window where security modules are in init state after restart | Sub-second restart; modules validate state on re-init |
| LAN Sync: clock skew >30s | May cause version ordering issues when timestamps are the tiebreaker | Version number is primary comparator; timestamp is fallback |
| JS localStorage not encrypted | Password groups and UI preferences stored in plaintext localStorage | No vault secrets involved; same trust domain as session |

---

## Future Considerations

- Mobile client (iOS/Android) with biometric unlock
- Browser extension for auto-fill (security review required)
- Collaborative vaults with threshold signatures
- Hardware wallet integration (Ledger/Trezor) for master key storage
- Decentralized sync via libp2p / IPFS (research phase)
