# Grimlocker Omega+ — Error Code Reference

> All errors implement `*GrimlockError` from `errors/types.go`.  
> Each error carries: `code`, `message`, `context.operation`, `context.block_id`, `stacktrace`, `timestamp`.

---

## Error Code Ranges

| Range | Category | Package |
|-------|---------|---------|
| 1000–1999 | Vault / Authentication | `errors/types.go` |
| 2000–2999 | Storage / GrimDB | `errors/types.go` |
| 3000–3999 | Cryptography / Key Material | `errors/types.go` |
| 4000–4999 | Security / Lockdown / Memory | `errors/types.go` |
| 5000–5999 | Kernel / Bus / Event Routing | `errors/types.go` |
| 6000–6999 | API / Protocol / Transport | `errors/types.go` |

---

## 1000–1999: Vault / Authentication Errors

### 1001 — ErrCodeVaultLocked

**Meaning:** The operation requires an unlocked vault. The vault has not been unlocked yet (or has been locked since).

**Constructor:** `gerrors.NewVaultLockedError()`  
**HTTP Status:** 423 Locked  
**Stacktrace:** No (hot-path, performance-sensitive)

**Typical log:**
```
[ERROR] vault is locked error_code=1001 operation=vault_access
```

**Recovery:**
1. Send `AUTH.UNLOCK` with the correct password / OIDC token
2. Wait for `AUTH.KEY_READY` event
3. Retry the operation

---

### 1002 — ErrCodeVaultNotInitialized

**Meaning:** The vault has never been set up. `vault_entries.enc` and `vault_index.enc` do not exist yet.

**Constructor:** `gerrors.NewVaultNotInitializedError()`  
**HTTP Status:** 404 Not Found  
**Stacktrace:** No

**Recovery:**
1. Run vault initialization: `POST /init` with password
2. The daemon will create the encrypted files and derive the MVK

---

### 1003 — ErrCodeAuthInvalid

**Meaning:** Authentication failed. Password is wrong, or OIDC JWT signature verification failed.

**Constructor:** `gerrors.NewAuthInvalidError(operation, cause)`  
**HTTP Status:** 401 Unauthorized  
**Stacktrace:** Yes

**Typical log:**
```
[ERROR] authentication failed error_code=1003 operation=password_hash 
        stacktrace=[config/single/auth.go:87]
```

**Context fields:**
- `operation`: `"password_hash"` (Argon2id) or `"jwt_verification"` (OIDC/enterprise)

**Recovery:**
- Re-enter the correct password
- Check remaining attempts: `GET /api/v1` with `AUTH.STATUS` action

---

### 1004 — ErrCodeAuthTimeout

**Meaning:** Auth operation timed out (e.g., OIDC JWKS fetch timed out, or Argon2id took too long).

**HTTP Status:** 504 Gateway Timeout

**Recovery:**
- Retry the unlock operation
- Check network connectivity (enterprise: OIDC provider reachable?)

---

### 1005 — ErrCodeAuthLockdown

**Meaning:** Too many failed attempts. The vault is now in lockout mode until `lockdown_until` timestamp.

**Constructor:** `gerrors.NewAuthLockdownError(attemptsRemaining)`  
**HTTP Status:** 429 Too Many Requests  
**Stacktrace:** No

**Context fields:**
- `remaining_attempts`: always 0 when lockdown is active

**Recovery:**
- Wait until `lockdown_until` timestamp (check via `AUTH.STATUS` reply)
- Hard lockdown → process exits → restart daemon, then unlock with correct password

---

### 1006 — ErrCodeAuthSetupFailed

**Meaning:** Vault initialization failed during `POST /init`.

**HTTP Status:** 500 Internal Server Error

**Recovery:**
- Check disk permissions at `$APP_DIR`
- Check entropy availability (CSPRNG must work)
- Delete partial files and retry init

---

### 1007 — ErrCodeAuthTokenExpired

**Meaning:** OIDC/JWT token has expired (enterprise tier only).

**HTTP Status:** 401 Unauthorized

**Recovery:**
- Re-authenticate with the OIDC provider
- Obtain a fresh JWT token and retry `AUTH.UNLOCK`

---

## 2000–2999: Storage / GrimDB Errors

### 2001 — ErrCodeStorageIO

**Meaning:** A disk I/O operation failed. Could be disk full, permissions, or hardware error.

**Constructor:** `gerrors.NewStorageIOError(operation, blockID, cause)`  
**HTTP Status:** 500 Internal Server Error  
**Stacktrace:** Yes

**Context fields:**
- `operation`: one of `open_data_file`, `read_block_data`, `close_data_file`, `nonce_generation`, `open_index`, `read_index_length`, `read_index_nonce`, `read_encrypted_index`
- `block_id`: affected block (empty for index operations)

**Recovery:**
- Check disk space: `df -h`
- Check permissions: vault dir should be `0700`, files `0600`
- Check hardware: `dmesg | grep -i error`

---

### 2002 — ErrCodeStorageCorruption

**Meaning:** HMAC verification failed or JSON parsing of encrypted data failed. **Data may have been tampered with.**

**Constructor:** `gerrors.NewStorageCorruptionError(operation, blockID, details)`  
**HTTP Status:** 422 Unprocessable Entity  
**Stacktrace:** Yes

**Context fields:**
- `operation`: `"hmac_verify"` or `"unmarshal_index"`
- `detail_reason`: human-readable description

⚠ **This is a serious security event.** The vault may have been tampered with.

**Recovery:**
1. Do NOT overwrite the corrupted files — preserve them for forensics
2. Restore from a verified backup
3. If no backup: data is unrecoverable (by design — ChaCha20-Poly1305 authentication prevents partial decryption)

---

### 2003 — ErrCodeStorageNotFound

**Meaning:** The requested block ID is not in the vault index.

**Constructor:** `gerrors.NewStorageNotFoundError(blockID)`  
**HTTP Status:** 404 Not Found  
**Stacktrace:** No

**Typical causes:**
- Entry was deleted
- Index not loaded yet (vault was just unlocked)
- Bug: wrong block ID passed

**Recovery:**
- If recently unlocked: check that `LoadIndex()` completed (look for "LoadIndex — N entries" in logs)
- If entry should exist: check if it was deleted in audit log

---

### 2004 — ErrCodeStorageQuota

**Meaning:** Storage quota exceeded.

**HTTP Status:** 500 Internal Server Error

**Recovery:**
- Delete unused entries
- Check `GRIMLOCKER_QUOTA` env var

---

### 2005 — ErrCodeStorageIndexFailed

**Meaning:** Index serialization or persistence failed.

**Constructor:** `gerrors.NewStorageIndexError(operation, cause)`  
**HTTP Status:** 500 Internal Server Error  
**Stacktrace:** Yes

**Context fields:**
- `operation`: `"marshal_index"` or `"nonce_generation_index"`

⚠ **Warning:** If this happens during a WriteBlock, the block data was written to `vault_entries.enc` but the index was NOT updated. The block is orphaned (takes space but is unreachable).

**Recovery:**
- Free disk space
- Restart daemon — orphaned blocks will remain but won't cause errors

---

### 2006 — ErrCodeStorageNonceFailed

**Meaning:** CSPRNG failed to generate a random nonce.

**HTTP Status:** 500 Internal Server Error

**Recovery:**
- This is extremely rare; it indicates OS-level entropy failure
- Restart the daemon; if it persists, check OS entropy sources

---

## 3000–3999: Cryptography Errors

### 3001 — ErrCodeCryptoKeyDerivation

**Meaning:** Argon2id (password → MVK) or HKDF derivation failed.

**Constructor:** `gerrors.NewCryptoKeyDerivationError(operation, cause)`  
**HTTP Status:** 500 Internal Server Error  
**Stacktrace:** Yes

**Context fields:**
- `operation`: `"argon2id"` or `"hkdf"`

**Recovery:**
- Usually caused by entropy failure during derivation
- Retry — Argon2id is deterministic given the same password + salt, so if the cause was transient, a retry should succeed

---

### 3002 — ErrCodeCryptoEncryption

**Meaning:** ChaCha20-Poly1305 `Seal()` (encryption) failed.

**Constructor:** `gerrors.NewCryptoEncryptionError(operation, cause)`  
**HTTP Status:** 500 Internal Server Error  
**Stacktrace:** Yes

**Recovery:**
- Check MVK is available (is vault unlocked?)
- Check nonce generation (error_code 3005)

---

### 3003 — ErrCodeCryptoDecryption

**Meaning:** ChaCha20-Poly1305 `Open()` (decryption) failed. This means either the wrong key was used or the ciphertext was tampered with.

**Constructor:** `gerrors.NewCryptoDecryptionError(blockID, cause)`  
**HTTP Status:** 422 Unprocessable Entity  
**Stacktrace:** Yes

**Context fields:**
- `block_id`: affected block (empty for index)
- `operation`: `"chacha20poly1305_open"` or `"vault_index"`

**Disambiguation:**
- If HMAC check (2002) passed but decrypt failed → **wrong key** (re-unlock vault)
- If HMAC check failed before decrypt → **tampered data** (see 2002 recovery)

---

### 3004 — ErrCodeCryptoInvalidKey

**Meaning:** Key material is nil or wrong length. ChaCha20-Poly1305 requires exactly 32 bytes.

**Constructor:** `gerrors.NewCryptoInvalidKeyError(gotBytes)`  
**HTTP Status:** 500 Internal Server Error  
**Stacktrace:** Yes

**Context fields:**
- `detail_got_bytes`: actual key length received

**Recovery:**
- Vault is probably not unlocked (MVK resolver returns nil)
- Ensure AUTH.KEY_READY was received before issuing STORAGE operations

---

### 3005 — ErrCodeCryptoEntropyFailed

**Meaning:** CSPRNG / entropy source failed.

**HTTP Status:** 500 Internal Server Error

**Recovery:**
- OS-level issue — check available entropy: `cat /proc/sys/kernel/random/entropy_avail` (Linux)
- Restart the system if entropy pool is depleted

---

### 3006 — ErrCodeCryptoHandleUnknown

**Meaning:** The key handle passed to `CRYPTO.ENCRYPT` or `CRYPTO.DECRYPT` was not found in `security.Module`'s handle table.

**Constructor:** `gerrors.NewCryptoHandleUnknownError(handle)`  
**HTTP Status:** 500 Internal Server Error  
**Stacktrace:** No

**Context fields:**
- `detail_handle_prefix`: first 8 chars of the handle (never the full handle)

**Recovery:**
- Handle may have been revoked (vault locked, or daemon restarted)
- Re-unlock vault to get a fresh handle via `AUTH.GET_HANDLE`

---

## 4000–4999: Security / Memory / Lockdown Errors

### 4001 — ErrCodeSecurityMemlock

**Meaning:** `mlock()` (Linux/Mac) or `VirtualLock()` (Windows) failed. Key material cannot be locked in RAM and may be swapped to disk.

**Constructor:** `gerrors.NewSecurityMemlockError(cause)`  
**HTTP Status:** 500 Internal Server Error  
**Stacktrace:** Yes

**Recovery (Linux):**
```bash
# Check current limit
ulimit -l
# Increase for session
ulimit -l unlimited
# Or add to /etc/security/limits.conf:
* soft memlock unlimited
* hard memlock unlimited
```

**Recovery (Windows):**
- Run daemon as Administrator
- Or grant "Lock pages in memory" privilege via Local Security Policy

---

### 4002 — ErrCodeSecurityLockdown

**Meaning:** Hard lockdown was triggered. All key material has been zeroed, entropy overwritten, and the process will exit.

**Constructor:** `gerrors.NewSecurityLockdownError(reason, details)`  
**HTTP Status:** 403 Forbidden  
**Stacktrace:** Yes

**What happened:**
1. All `mvkHandles` zeroed via `guard.Zeroize()`
2. Entropy file overwritten with random bytes (`crypto.Shred()`)
3. `SECURITY.PANIC` event dispatched
4. `os.Exit(1)` called

**Recovery:**
1. Restart daemon
2. Re-unlock vault with correct password (data is safe, only in-RAM key was zeroed)
3. If repeated hard lockdowns: investigate who is sending `SECURITY.LOCKDOWN` events

---

### 4003 — ErrCodeSecurityIntegrity

**Meaning:** Binary integrity check failed — the daemon binary may have been tampered with.

**HTTP Status:** 403 Forbidden

**Recovery:**
1. Verify binary: `sha256sum grimlocker-daemon` against published release hash
2. Re-download from official source
3. Do NOT unlock vault with a potentially compromised binary

---

### 4004 — ErrCodeSecurityUnauthorized

**Meaning:** Operation denied by security policy.

**HTTP Status:** 403 Forbidden

---

### 4005 — ErrCodeSecurityMVKMissing

**Meaning:** The Master Vault Key handle is missing or has been revoked.

**Constructor:** `gerrors.NewSecurityMVKMissingError(operation)`  
**HTTP Status:** 500 Internal Server Error  
**Stacktrace:** No

**Recovery:**
- Vault is locked → re-unlock with `AUTH.UNLOCK`
- If vault is unlocked: this is a bug — check security/module.go handle table

---

## 5000–5999: Kernel / Bus Errors

### 5001 — ErrCodeBusShutdown

**Meaning:** A dispatch was attempted while the bus is shutting down.

**Constructor:** `gerrors.NewBusShutdownError()`  
**HTTP Status:** 500 Internal Server Error  
**Stacktrace:** No

**Recovery:**
- Normal during graceful shutdown; ignore
- If not during shutdown: investigate who called `bus.Shutdown()` prematurely

---

### 5002 — ErrCodeBusTimeout

**Meaning:** A `bus.Request()` call timed out waiting for a reply event.

**Constructor:** `gerrors.NewBusTimeoutError(eventType)`  
**HTTP Status:** 504 Gateway Timeout  
**Stacktrace:** No

**Context fields:**
- `detail_event_type`: which event timed out

**Recovery:**
- Check handler registered for the event type
- Check handler dispatches a `ReplyEvent` with `ReplyTo` set
- Check for stuck goroutines: `kill -SIGQUIT <pid>`

---

### 5003 — ErrCodeBusGated

**Meaning:** An event was dropped because the STORAGE gate is closed (vault not unlocked).

**Constructor:** `gerrors.NewBusGatedError(eventType, channel)`  
**HTTP Status:** 423 Locked  
**Stacktrace:** No

**Recovery:**
- Unlock vault first: `AUTH.UNLOCK`

---

### 5004 — ErrCodeBusTTL

**Meaning:** Event dropped because TTL (Time-To-Live hop count) reached 0. Default TTL = 8.

**HTTP Status:** 500 Internal Server Error

**Recovery:**
- Usually indicates an event loop (handler re-dispatches same event)
- Check handler logic for infinite dispatch cycles

---

### 5005 — ErrCodeBusModuleDuplicate

**Meaning:** A module with this ID was already registered.

**HTTP Status:** 500 Internal Server Error

**Recovery:**
- Bug in startup sequence — check `cmd/daemon/main.go` for duplicate `reg.Add()` calls

---

## 6000–6999: API / Protocol Errors

### 6001 — ErrCodeProtocolInvalid

**Meaning:** Binary frame or JSON payload is malformed or uses an unknown message type.

**Constructor:** `gerrors.NewProtocolError(operation, cause)`  
**HTTP Status:** 400 Bad Request  
**Stacktrace:** Yes

**Context fields:**
- `operation`: where parsing failed (e.g., `"encrypt_unmarshal"`, `"decrypt_unmarshal"`)

**Recovery:**
- Check client SDK version matches daemon version
- Check JSON schema for the specific action

---

### 6002 — ErrCodeProtocolTimeout

**Meaning:** Client-side request timed out.

**HTTP Status:** 504 Gateway Timeout

---

### 6003 — ErrCodeProtocolUnhandled

**Meaning:** No handler registered for the requested action.

**HTTP Status:** 500 Internal Server Error

**Recovery:**
- Check event type is defined in `kernel/event.go`
- Check module is registered in `cmd/daemon/main.go`

---

### 6004 — ErrCodeProtocolAuth

**Meaning:** WebSocket or IPC authentication failed (wrong session token or cookie).

**HTTP Status:** 401 Unauthorized

**Recovery:**
- Reconnect WebSocket with correct origin cookie
- Session token is generated fresh on each daemon start — don't cache it

---

## Error JSON Schema

When an error is returned via HTTP or WebSocket, the JSON payload is:

```json
{
  "code": 2001,
  "message": "storage I/O failure",
  "context": {
    "block_id": "f3a8c91b-...",
    "operation": "read_block_data",
    "details": {
      "reason": "file not found"
    }
  },
  "stacktrace": [
    "github.com/grimlocker/grimdb/storage/grimdb/blockstore.go:238 in grimdb.(*BlockStoreImpl).ReadBlock",
    "github.com/grimlocker/grimdb/storage/grimdb/adapter.go:45 in grimdb.handleRead"
  ],
  "timestamp": 1748563200000000000,
  "module_id": "storage",
  "event_type": "STORAGE.READ"
}
```

**Note:** The `stacktrace` field is only present for errors where `CaptureStack: true` in the constructor. `VaultLocked` (1001) and other hot-path errors do NOT include a stacktrace to avoid performance overhead.

---

*See `errors/types.go` for all constructor functions and code constants.*
