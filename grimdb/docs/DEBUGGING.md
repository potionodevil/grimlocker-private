# Grimlocker Omega+ — Debugging Guide

> Use this guide to self-diagnose issues from logs and error codes.  
> Every `GrimlockError` carries a numeric code — look it up in `ERROR_CODES.md`.

---

## Quick Reference: Log Levels

```
[DEBUG]    Handler dispatch, event routing (only with DebugEnabled=true)
[INFO]     Module lifecycle (start, stop), normal successful operations
[WARN]     Recoverable issues — retries, approaching limits
[ERROR]    Operation failed — GrimlockError logged with code + stacktrace
[FATAL]    Process will exit — e.g., Argon2id failure at startup, hard lockdown
```

**How to read a GrimlockError log line:**

```
[ERROR] storage I/O failure error_code=2001 module=storage operation=read_block_data 
        block_id=f3a8c91b detail_reason=disk_read_failed 
        stacktrace=[storage/grimdb/blockstore.go:238 in grimdb.(*BlockStoreImpl).ReadBlock]
```

Key fields:
- `error_code` → look up in `ERROR_CODES.md`
- `operation` → which low-level op failed
- `block_id` → which block is affected (search in vault_index.enc)
- `stacktrace` → first frame = error origin file + line

---

## Diagnostic Flowcharts

### A. "Vault won't unlock"

```
START: Auth.Unlock event sent, but vault stays locked
│
├─ Check logs: does "AUTH.UNLOCK received" appear?
│  ├─ NO → Event never reached daemon
│  │         Check: Is daemon running? (`curl http://localhost:PORT/health`)
│  │         Check: Is WebSocket connected? (look for "ws: upgrade" in logs)
│  │         Check: Is IPC socket alive? (`ls /tmp/grimlocker.sock`)
│  └─ YES → Continue ↓
│
├─ Check logs: error_code present?
│  ├─ 1003 (AuthInvalid) → Wrong password or JWT token
│  │   Fix: Re-enter the correct password
│  │   Debug: Check `lockdown_state` in AUTH.STATUS reply
│  │
│  ├─ 1005 (AuthLockdown) → Too many failed attempts
│  │   Fix: Wait until `lockdown_until` timestamp passes
│  │   Debug: Check `remaining_attempts` in AUTH.STATUS reply
│  │   Note: Hard lockdown → process exits, restart required
│  │
│  ├─ 3001 (CryptoKeyDerivation) → Argon2id failed
│  │   Fix: Check entropy availability (not a wrong password)
│  │   Debug: Look for "csprng failure" in logs
│  │
│  ├─ 3004 (CryptoInvalidKey) → MVK has wrong length
│  │   Fix: Re-derive key — re-enter password
│  │   Debug: Check config/single/auth.go key derivation path
│  │
│  ├─ 4001 (SecurityMemlock) → mlock/VirtualLock failed
│  │   Fix: Increase memlock limit: `ulimit -l unlimited` (Linux)
│  │        Or: Restart daemon as root (Windows: run as Administrator)
│  │   Debug: `security/memlock_unix.go` or `security/memlock_windows.go`
│  │
│  └─ No error code → Continue ↓
│
├─ Check logs: does "AUTH.KEY_READY received" appear?
│  ├─ NO → MVK was derived but StoreMVK() failed
│  │   Debug: Check security/module.go:StoreMVK() — AllocLocked error?
│  │   Check: error_code 4001 (SecurityMemlock)
│  └─ YES → Continue ↓
│
├─ Check logs: does "Gate opened — gated channels now flow" appear?
│  ├─ NO → bus.OpenGate() was not called
│  │   Debug: cmd/daemon/main.go line 126-133 (KEY_READY subscriber)
│  │   Fix: This is a bug — file an issue with the full log
│  └─ YES → Continue ↓
│
└─ Gate is open but STORAGE events not working → Go to Section B
```

---

### B. "Storage / Entry operations fail"

```
START: Entry reads or writes return error, vault is unlocked
│
├─ Check error_code:
│  ├─ 2003 (StorageNotFound) → Block ID not in index
│  │   Cause: Entry was deleted, or index not loaded after unlock
│  │   Fix: Call AUTH.UNLOCK to reload index (LoadIndex is called on unlock)
│  │   Debug: Check blockstore.go:LoadIndex() was called (look for "LoadIndex — N entries")
│  │
│  ├─ 2002 (StorageCorruption) → HMAC mismatch or JSON parse failed
│  │   Cause: vault_entries.enc or vault_index.enc tampered/corrupted
│  │   ⚠ SERIOUS — may indicate data tampering
│  │   Debug: Check `operation` field:
│  │     - "hmac_verify" → block data tampered (vault_entries.enc)
│  │     - "unmarshal_index" → index JSON corrupted (vault_index.enc)
│  │   Fix: Restore from backup. If no backup, data is unrecoverable.
│  │
│  ├─ 2001 (StorageIO) → Disk I/O failure
│  │   Cause: Disk full, permissions, hardware error
│  │   Debug: Check `operation` field (open_data_file, read_block_data, etc.)
│  │   Fix: Check disk space (`df -h`), check permissions (vault dir: 0700)
│  │
│  ├─ 2005 (StorageIndexFailed) → Index persist failed
│  │   Cause: Cannot write vault_index.enc (disk full? permissions?)
│  │   ⚠ Data written to vault_entries.enc but index not saved → orphaned blocks
│  │   Fix: Free disk space, then re-write the entry (it will get a new block ID)
│  │
│  └─ 3003 (CryptoDecryption) → Block decryption failed
│      Cause: Wrong MVK (key was rotated?) or block data tampered
│      Debug: Check `block_id` — is HMAC verified first? (check error_code 2002 above)
│      Fix: If HMAC passed but decrypt failed → key mismatch. Re-unlock vault.
│
└─ No error but operation hangs → Check Section C
```

---

### C. "Operation hangs / times out"

```
START: Request sent but no reply received within timeout
│
├─ Check error_code: 5002 (BusTimeout)
│   Cause: Handler registered but didn't dispatch a reply event
│   Debug: Which channel? 
│     - AUTH → check security/module.go handler dispatches ReplyEvent
│     - STORAGE → is gate open? (check for "Gate opened" in logs)
│     - CRYPTO → check crypto/module.go handler dispatches EvCryptoResult
│
├─ Check logs: "[bus] PANIC in handler" 
│   Cause: Handler panicked — panic was recovered but reply never sent
│   Fix: Look at the stacktrace in the panic log and fix the nil pointer / OOB
│
├─ Check logs: "[bus] event X dropped: gate closed"
│   Cause: STORAGE gate closed (vault not unlocked)
│   Fix: Unlock the vault first (AUTH.UNLOCK)
│
└─ Check: Is the handler goroutine stuck?
    Debug: `kill -SIGQUIT <pid>` (Linux/Mac) to dump goroutine stacks
    Look for: goroutines blocked on `<-replyCh` or `bs.mu.Lock()`
```

---

### D. "Hard Lockdown / Process Exits"

```
START: Daemon exits unexpectedly with "[security] HARD LOCKDOWN"
│
├─ Causes:
│   1. SECURITY.LOCKDOWN event received (AUTH.LOCKDOWN or SECURITY.PANIC)
│   2. Too many failed auth attempts (LockdownManager threshold)
│   3. Binary integrity check failed (INTEGRITY.VIOLATION)
│
├─ What happened:
│   - All MVK handles zeroed (key material gone)
│   - Entropy file overwritten with random bytes
│   - SECURITY.PANIC event dispatched
│   - os.Exit(1) called
│
├─ Recovery:
│   1. Restart daemon
│   2. Unlock with correct password (MVK will be re-derived)
│   3. Vault data is safe — only in-RAM key was zeroed
│
└─ If integrity check failed:
    - Binary may be tampered
    - Verify checksum: `sha256sum grimlocker-daemon`
    - Compare against signed release checksum
```

---

## Per-Module Error Matrix

### SECURITY Module (`security/module.go`)

| Log Pattern | Error Code | Root Cause | Fix |
|-------------|-----------|-----------|-----|
| `alloc locked:` | 4001 | `mlock()` / `VirtualLock()` failed | Increase memlock limit or run as admin |
| `PANIC event received` | 4002 | SECURITY.PANIC dispatched | Check who dispatched it; restart daemon |
| `HARD LOCKDOWN: exiting` | 4002 | Lockdown threshold reached | Restart daemon, re-unlock with correct pwd |
| `entropy shred failed` | 4002 | Entropy file write error | Check file permissions at `entropyPath` |

### CRYPTO Module (`crypto/module.go`)

| Log Pattern | Error Code | Root Cause | Fix |
|-------------|-----------|-----------|-----|
| `key handle not found` | 3006 | Key handle expired / revoked | Re-unlock vault to get fresh handle |
| `decryption failed` | 3003 | Wrong key or tampered ciphertext | Verify HMAC first (2002); re-unlock |
| `key derivation failed` | 3001 | Argon2id / HKDF failure | Check password input; check entropy |
| `encrypt: new_nonce` | 3002 | CSPRNG failure during encryption | Restart daemon; check OS entropy |

### STORAGE Module (`storage/grimdb/blockstore.go`)

| Log Pattern | Error Code | Root Cause | Fix |
|-------------|-----------|-----------|-----|
| `block not found` | 2003 | Block ID missing from index | Entry deleted; LoadIndex not called |
| `HMAC mismatch` | 2002 | Block data tampered | ⚠ Restore from backup |
| `unmarshal_index` | 2002 | Index JSON corrupted | ⚠ Restore from backup |
| `decrypt index` | 3003 | Index decryption failed | MVK mismatch; re-unlock vault |
| `open_data_file` | 2001 | Disk I/O error | Check disk, permissions |
| `marshal index` | 2005 | Index serialize failed | Memory pressure? Check available RAM |

### KERNEL / BUS (`kernel/bus.go`)

| Log Pattern | Error Code | Root Cause | Fix |
|-------------|-----------|-----------|-----|
| `event X dropped: gate closed` | 5003 | STORAGE gate closed | Unlock vault first |
| `event X dropped: TTL exhausted` | 5004 | Event loop / cycle | Check handler not re-dispatching same event |
| `PANIC in handler for X` | — | Handler panicked | Fix nil pointer / OOB in handler |
| `handler error for X` | — | Handler returned error | Check specific module error |

### API / Translator (`api/translator.go`)

| Log Pattern | Error Code | Root Cause | Fix |
|-------------|-----------|-----------|-----|
| `protocol error` | 6001 | Binary frame malformed | Check client version matches daemon |
| `session key not available` | 6004 | SKE not initialized | Reconnect WebSocket after unlock |
| `timeout` | 6002 | Handler didn't reply within limit | Check bus for stuck handler |

---

## Log File Structure

The daemon logs to **stdout/stderr** by default (captured by Tauri or systemd).

**Typical startup log:**
```
[Omega] ===== DAEMON START vomega-2026-05-30 =====
[Omega] Rust secure enclave initialized
[Omega] App directory: /home/user/.grimlocker
[Omega] Database path: /home/user/.grimlocker/vault.gdb
[Omega] Tier: single-user
[kernel] Register module: security
[kernel] Register module: crypto
[kernel] Register module: storage
[kernel] Register module: tools
[security] security module started
[blockstore] LoadIndex — 0 entries loaded: []
[Omega] Daemon listening on :PORT
```

**Successful unlock:**
```
[security] AUTH.UNLOCK: verifying password (step 1/7)
[security] AUTH.UNLOCK: argon2id hash verified (step 3/7)
[security] AUTH.UNLOCK: MVK stored in locked memory (step 5/7)
[blockstore] LoadIndex — 42 entries loaded: [...]
[bus] Gate opened — gated channels now flow
[Omega] KEY_READY subscription triggered
[Omega] AUTH.KEY_READY received — STORAGE gate OPEN
```

**Failed unlock (wrong password):**
```
[ERROR] authentication failed error_code=1003 module=auth operation=password_hash 
        stacktrace=[config/single/auth.go:87 in single.(*LocalAuth).verify]
[security] lockdown state: remaining_attempts=2
```

**Hard lockdown:**
```
[security] HARD LOCKDOWN: zeroising all key material — [4002] hard lockdown triggered
[security] HARD LOCKDOWN: exiting process (code=4002)
```

---

## Useful Debug Commands

```bash
# Check daemon health
curl http://localhost:PORT/health

# Check vault is unlocked
curl http://localhost:PORT/health | jq .vault_unlocked

# Dump goroutine stacks (Linux/Mac)
kill -SIGQUIT $(pgrep grimlocker-daemon)

# Count log lines by level
grep -c '\[ERROR\]' daemon.log
grep -c '\[FATAL\]' daemon.log

# Find all lockdown events
grep 'LOCKDOWN' daemon.log

# Find all storage errors  
grep 'error_code=2' daemon.log

# Find crypto failures
grep 'error_code=3' daemon.log
```

---

## File Locations

| File | Default Path | Purpose |
|------|-------------|---------|
| `vault_entries.enc` | `$APP_DIR/vault_entries.enc` | Encrypted block data |
| `vault_index.enc` | `$APP_DIR/vault_index.enc` | Encrypted block index |
| `entropy.bin` | Configured in `entropyPath` | Entropy seed (overwritten on hard lockdown) |
| IPC socket | `/tmp/grimlocker.sock` | Unix socket for CLI |

`$APP_DIR` defaults to:
- Linux/Mac: `~/.grimlocker/`
- Windows: `%APPDATA%\grimlocker\`
- Override: `GRIMLOCKER_DB_PATH` env var

---

*If none of the above helps: collect the full daemon log and the error code, then open an issue.*
