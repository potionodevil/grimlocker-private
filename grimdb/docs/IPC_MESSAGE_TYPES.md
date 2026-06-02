# IPC Message Types Reference

Complete enumeration of all 86 IPC message type constants defined in `api/ipc/protocol.go`.

Wire format: `[4-byte big-endian length][1-byte type][N-byte payload]`

---

## Core Vault Protocol (0x01–0x0E)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x01` | `MsgGetHeader` | C→S | — | Request 26-byte vault header |
| `0x02` | `MsgHeader` | S→C | 26 bytes binary | Vault header (attempts, lockdown, timestamps) |
| `0x03` | `MsgGetCiphertext` | C→S | — | Request encrypted vault payload |
| `0x04` | `MsgCiphertext` | S→C | Raw bytes | ChaCha20-Poly1305 ciphertext: `[12-byte nonce][ct+tag]` |
| `0x05` | `MsgUpdateHeader` | C→S | 26 bytes binary | Write updated vault header |
| `0x06` | `MsgUpdateCiphertext` | C→S | Raw bytes | Store re-encrypted vault payload |
| `0x07` | `MsgTriggerWipe` | C→S | — | Begin 7-pass anti-forensic shred |
| `0x08` | `MsgAck` | Both | — | Success acknowledgement (empty) |
| `0x09` | `MsgError` | S→C | UTF-8 string | Sanitized error message |
| `0x0A` | `MsgPanicWipe` | C→S | — | Deceptive wipe (panic key path) — responds with fake success |
| `0x0B` | `MsgGenerateMatrix` | C→S | JSON `{line_count, entropy_path}` | Trigger entropy file generation |
| `0x0C` | `MsgProgress` | S→C | JSON `{progress, stage, message}` | Streaming progress (0–100%) |
| `0x0D` | `MsgGenerationResult` | S→C | JSON `{key_hex, coordinates, entropy_size}` | Entropy generation complete |
| `0x0E` | `MsgZeroizeConfirm` | C→S | — | Client confirms JS buffer zeroized after Single Glance |

---

## Vault Lifecycle (0x0F–0x1A)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x0F` | `MsgInitializeVault` | C→S | JSON | One-shot vault initialization |
| `0x10` | `MsgUnlockVault` | C→S | JSON `{password}` | Authenticate and unlock |
| `0x11` | `MsgSaveEntry` | C→S | JSON | Legacy entry save (deprecated — use GQL) |
| `0x12` | `MsgRecoveryPhrase` | S→C | JSON | Encrypted recovery phrase delivery |
| `0x13` | `MsgUnlockResult` | S→C | JSON `{success, error?}` | Unlock outcome |
| `0x14` | `MsgCheckVaultStatus` | C→S | — | Query vault state |
| `0x15` | `MsgListEntries` | C→S | JSON | Legacy list (deprecated — use GQL) |
| `0x16` | `MsgGetEntry` | C→S | JSON `{id}` | Legacy get (deprecated — use GQL) |
| `0x17` | `MsgDeleteEntry` | C→S | JSON `{id}` | Legacy delete (deprecated — use GQL) |
| `0x18` | `MsgEntriesResult` | S→C | JSON `[]Entry` | Legacy entry list response |
| `0x19` | `MsgEntryData` | S→C | JSON `Entry` | Legacy single entry response |
| `0x1A` | `MsgResetVault` | C→S | — | Full vault reset (destructive) |

---

## Broadcast & Entry CRUD (0x1B–0x1F)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x1B` | `MsgLogBroadcast` | S→C | UTF-8 string | Daemon log line (debug builds) |
| `0x1C` | `MsgEntryCreate` | C→S | JSON `{title, category, fields}` | Create entry (legacy path) |
| `0x1D` | `MsgEntryResult` | S→C | JSON `{success, entry?, error?}` | Entry operation result |
| `0x1E` | `MsgEntryUpdate` | C→S | JSON `{id, title, fields}` | Update entry (legacy path) |
| `0x1F` | `MsgEntryDelete` | C→S | JSON `{id}` | Delete entry (legacy path) |

---

## File Ingest (0x20–0x23)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x20` | `MsgFileIngestBegin` | C→S | JSON `{file_name, mime_type, size}` | Begin encrypted file upload |
| `0x21` | `MsgFileChunk` | C→S | Binary | Raw file chunk (16 KiB max per frame) |
| `0x22` | `MsgFileIngestEnd` | C→S | JSON `{sha256, total_size}` | Finalize upload, verify integrity |
| `0x23` | `MsgIngestProgress` | S→C | JSON `{progress, bytes_written}` | Upload progress feedback |

---

## Recovery & Workspace (0x24–0x2B)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x24` | `MsgGetRecoveryPhrase` | C→S | — | Request encrypted recovery phrase |
| `0x25` | `MsgRecoveryPhraseData` | S→C | JSON (SKE-encrypted) | Encrypted recovery phrase |
| `0x26` | `MsgPanicWipeRequest` | C→S | — | Destructive full wipe request |
| `0x27` | `MsgWorkspaceList` | C→S | — | List all workspaces |
| `0x28` | `MsgWorkspaceCreate` | C→S | JSON `{name}` | Create a new workspace |
| `0x29` | `MsgWorkspaceSwitch` | C→S | JSON `{id}` | Switch active workspace |
| `0x2A` | `MsgWorkspaceDelete` | C→S | JSON `{id}` | Delete workspace (non-default only) |
| `0x2B` | `MsgWorkspacesResult` | S→C | JSON `[]Workspace` | Workspace list response |

---

## Handshake Protocol (0x2C–0x32)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x2C` | `MsgInitReady` | S→C | JSON `{version, tier}` | INIT.READY — first frame after connect |
| `0x2D` | `MsgAuthTokenSubmit` | C→S | JSON `{token}` | AUTH.TOKEN_SUBMIT |
| `0x2E` | `MsgKernelStateReady` | S→C | JSON (state mirror) | KERNEL.STATE_READY |
| `0x2F` | `MsgSystemHeartbeat` | S→C | JSON `{ts}` | SYSTEM.HEARTBEAT (every 30s) |
| `0x30` | `MsgSystemError` | S→C | JSON `{error}` | SYSTEM.ERROR — daemon-level fault |
| `0x31` | `MsgSystemLog` | S→C | UTF-8 string | SYSTEM.LOG — structured log line |
| `0x32` | `MsgSystemHealthCheck` | Both | — | SYSTEM.HEALTH_CHECK ping/pong |

---

## Session-Key Encrypted Data (0x33–0x34)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x33` | `MsgDecryptEntry` | C→S | JSON `{id}` | Request SKE-decrypted entry |
| `0x34` | `MsgDecryptedData` | S→C | Base64 (SKE-encrypted JSON) | Entry data encrypted with per-session ChaCha20-Poly1305 key |

The frontend holds the SKE key in RAM and decrypts only when the user explicitly clicks "Reveal". The key never leaves the Tauri process.

---

## Entry Queries & SSH Tools (0x35–0x38)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x35` | `MsgEntryQuery` | C→S | JSON `{category}` | Category-filtered entry query |
| `0x36` | `MsgEntryQueryResult` | S→C | JSON `{category, entries[], count}` | Category query results |
| `0x37` | `MsgSSHKeyGen` | C→S | JSON `{comment, save_to_vault}` | Generate ED25519 SSH key pair |
| `0x38` | `MsgSSHKeyResult` | S→C | JSON `{public_key, fingerprint, entry_id?}` | SSH key generation result |

---

## Reconnect / State Mirror (0x39–0x3C)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x39` | `MsgReconnect` | C→S | JSON `{token}` | Resume session without re-auth |
| `0x3A` | `MsgStateMirror` | S→C | JSON (full vault state) | Complete state snapshot: unlocked, gate, workspace, entries, SKE handle |
| `0x3B` | `MsgSessionResumeOK` | S→C | — | Session resume successful |
| `0x3C` | `MsgSessionResumeErr` | S→C | JSON `{reason}` | Resume failed (expired, locked, etc.) |

---

## GQL Binary Protocol (0x3D–0x3E)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x3D` | `MsgGQLQuery` | C→S | GQL binary frame | Injection-immune query (see `GQL_PROTOCOL.md`) |
| `0x3E` | `MsgGQLResult` | S→C | JSON `GQLResult` | Query result |

See [GQL_PROTOCOL.md](GQL_PROTOCOL.md) for the binary frame format and [SDK_GUIDE.md](SDK_GUIDE.md) for the high-level client API.

---

## Auth Lifecycle (0x3F–0x40)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x3F` | `MsgAuthLogout` | C→S | — | Request vault lock (auto-lock or user logout) |
| `0x40` | `MsgAuthLogoutAck` | S→C | — | Vault locked acknowledgement |

---

## FileVault Download (0x41–0x43)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x41` | `MsgFileDownloadRequest` | C→S | JSON `{manifest_block_id: string}` | Begin streaming download of an encrypted file |
| `0x42` | `MsgFileChunkData` | S→C | Binary (decrypted + decompressed) | One chunk of the file; stream ends with `0x43` |
| `0x43` | `MsgFileDownloadEnd` | S→C | JSON `{sha256: hex, total_size: int, file_name: string, mime_type: string}` | Download complete — verify integrity |

Download flow:
```
Client → 0x41 {manifest_block_id}
Server → 0x42 (chunk 1)
Server → 0x42 (chunk 2)
...
Server → 0x43 {sha256, total_size, file_name, mime_type}
```

---

## Workspace Rename (0x44)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x44` | `MsgWorkspaceRename` | C→S | JSON `{id: string, name: string}` | Rename a workspace |

Server responds with `MsgAck` (0x08) or `MsgError` (0x09).

---

## Enterprise Security (0x45)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x45` | `MsgPanicButton` | C→S | JSON `{passphrase: string}` | Admin-only: trigger two-step account compromise wipe |

See [ENTERPRISE_FEATURES.md](ENTERPRISE_FEATURES.md) for the Panic Button flow.

---

## Enterprise Server Discovery (0x50–0x51)

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x50` | `MsgDiscoverServers` | C→S | `{}` | Scan local network via mDNS |
| `0x51` | `MsgServerList` | S→C | JSON `[{name, address, port, tls_required}]` | Available Enterprise servers |

---

## Enterprise User Management (0x52–0x56)

Admin-only operations. Rejected with `MsgError` for non-admin sessions.

| Code | Name | Direction | Payload | Description |
|---|---|---|---|---|
| `0x52` | `MsgEnterpriseUserCreate` | C→S | JSON `{username, roles[]}` | Create user; daemon returns one-time password |
| `0x53` | `MsgEnterpriseUserList` | C→S | `{}` | List all users and their status |
| `0x54` | `MsgEnterpriseUserRevoke` | C→S | JSON `{user_id}` | Revoke user access (soft delete) |
| `0x55` | `MsgEnterpriseUserRestore` | C→S | JSON `{user_id}` | Re-enable a revoked user |
| `0x56` | `MsgEnterpriseUserResult` | S→C | JSON (user object or list) | Response to any user management operation |

---

## Constants

| Name | Value | Description |
|---|---|---|
| `CookieSize` | `32` | WebSocket session cookie length in bytes |
| `UnixSockPath` | `/tmp/grimlocker.sock` | Unix domain socket (Linux/macOS) |
| `WinPipePath` | `\\.\pipe\grimlocker` | Named pipe (Windows) |
