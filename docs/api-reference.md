# API Reference

This document specifies the complete IPC protocol, WebSocket API, REST API, message types, and client-server interaction patterns.

---

## Transport Layer

### Single-User Mode

Ports are **dynamically allocated** on startup. The daemon prints them to stdout:

```
GRIMLOCKER_IPC=ws://127.0.0.1:<ipc-port>/ws
GRIMLOCKER_UI=http://127.0.0.1:<ui-port>
```

| Route | Protocol | Purpose |
|---|---|---|
| `127.0.0.1:<ipc-port>/ws` | WebSocket | IPC bridge — binary protocol |
| `127.0.0.1:<ipc-port>/api/v1` | HTTP POST JSON | REST event dispatch |
| `127.0.0.1:<ipc-port>/health` | HTTP GET | Daemon health + vault status |
| `127.0.0.1:<ipc-port>/init` | HTTP POST JSON | One-shot vault initialization |
| `127.0.0.1:<ipc-port>/shutdown` | HTTP POST | Request graceful daemon shutdown |
| `127.0.0.1:<ui-port>/` | HTTP GET | Embedded React UI (go:embed) |

### Enterprise Mode

| Port | Protocol | Purpose |
|---|---|---|
| `:9443` | TCP/TLS (mTLS) | Client connections — requires mutual TLS |
| `:9090` | HTTP plaintext | Liveness probe only (`/health`) |

---

## REST API (`/api/v1`)

The REST API accepts `POST` requests with JSON body `{"action":"<name>","payload":{...}}` and returns `{"ok":bool,"payload":{...},"error":"..."}`.

### Available Actions

| Action | Event | Description |
|---|---|---|
| `vault.unlock` | `AUTH.UNLOCK` | Authenticate and unlock vault |
| `vault.logout` | `AUTH.LOGOUT` | Lock vault (clear session) |
| `vault.status` | `AUTH.STATUS` | Get lockdown / attempt state |
| `vault.init` | `AUTH.SETUP` | Check vault initialization state |
| `entry.create` | `ENTRY.CREATE` | Create a vault entry |
| `entry.read` | `ENTRY.READ` | Read entry by ID |
| `entry.update` | `ENTRY.UPDATE` | Update entry by ID |
| `entry.delete` | `ENTRY.DELETE` | Delete entry by ID |
| `entry.query` | `ENTRY.QUERY` | List entries by category |
| `storage.write` | `STORAGE.WRITE` | Write raw encrypted block |
| `storage.read` | `STORAGE.READ` | Read raw block by ID |
| `storage.delete` | `STORAGE.DELETE` | Delete raw block |
| `storage.list` | `STORAGE.LIST` | List all block metadata |

### `POST /api/v1` — vault.unlock

```json
// Request
{"action":"vault.unlock","payload":{"password":"MyMasterPassword"}}

// Response (success)
{"ok":true,"payload":{"success":true,"mvk_handle":"abc123","session_key":"<base64>"}}

// Response (failure)
{"ok":true,"payload":{"success":false,"reason":"invalid password"}}
```

### `POST /api/v1` — entry.create

```json
// Request
{
  "action": "entry.create",
  "payload": {
    "id": "github/myuser",
    "title": "github/myuser",
    "category": "password",
    "fields": {"value": "ghp_secret123"}
  }
}

// Response
{"ok":true,"payload":{"id":"<uuid>","title":"github/myuser","category":"password",...}}
```

### `POST /init` — Vault Initialization

Initialize a new vault (first-run only). Returns an error if vault already exists.

```json
// Request
POST /init
{"password":"MyMasterPassword"}

// Response (success)
{"recovery_phrase":"<base64-phrase>"}

// Response (vault already initialized)
HTTP 409 {"error":"vault already initialized"}
```

⚠️ **Store the recovery phrase securely.** It cannot be retrieved again.

### `POST /shutdown` — Graceful Shutdown

Requests a clean daemon shutdown. The daemon flushes storage, revokes cryptographic handles, and exits within ~5 seconds.

```json
// Request
POST /shutdown   (no body required)

// Response (immediate)
{"status":"shutting_down"}
```

### `GET /health` — Daemon Health

Returns current daemon state.

```json
{
  "status": "ready",
  "tier": "single",
  "version": "omega-2026-05-30-v1",
  "ipc_port": 12345,
  "ui_port": 12346,
  "vault_unlocked": false,
  "pid": 1234
}
```

`vault_unlocked: true` is sent to reconnecting WebSocket clients via `HandshakeStatus` so the UI can re-attach to an already-unlocked vault without prompting for the password again.

---

## WebSocket Authentication

### Token Flow

```
Daemon Startup:
  1. Generate 32-byte random token (CSPRNG)
  2. Write to ~/.grimlocker/.grim_token (mode 0600)
  3. Token file deleted on daemon shutdown

Client Connection:
  1. Tauri reads .grim_token via native fs.readTextFile()
  2. WebSocket handshake: ws://127.0.0.1:8374/ws?token=<token>
  3. Daemon validates token on upgrade
  4. Rejected connections get HTTP 401 / WebSocket close 4001
```

### WebSocket Close Codes

| Code | Meaning |
|---|---|
| `1000` | Normal closure (client disconnect) |
| `4001` | Invalid or missing token |
| `4002` | Protocol error (malformed message) |
| `4003` | Unauthorized action |
| `4004` | Internal server error |

---

## Message Wire Format

```
┌──────────────────┬─────────────┬──────────────────────┐
│  4 bytes         │  1 byte     │  N bytes             │
│  Payload Length  │  Msg Type   │  Payload             │
│  (big-endian)    │             │                      │
└──────────────────┴─────────────┴──────────────────────┘
```

### Encoding Rules

- **Length**: uint32, network byte order (big-endian)
- **Message Type**: uint8 (0x00-0xFF)
- **Payload**: Type-dependent; JSON strings or raw binary

### Examples

```
Request vault header:
  [0x00, 0x00, 0x00, 0x00] [0x01] []
  (length=0, type=MSG_GET_HEADER, no payload)

Vault header response:
  [0x00, 0x00, 0x00, 0x1A] [0x02] [26 bytes: .gdb header]
  (length=26, type=MSG_HEADER)

Error response:
  [0x00, 0x00, 0x00, 0x12] [0x09] [UTF-8: "Incorrect password"]
  (length=18, type=MSG_ERROR)
```

---

## Message Types Reference

### `0x01` — `MSG_GET_HEADER`

| Property | Value |
|---|---|
| Direction | Client → Server |
| Payload | None |
| Purpose | Request the 26-byte vault header |

No parameters. Server responds with `MSG_HEADER` (0x02) on success or `MSG_ERROR` (0x09) if no vault exists.

---

### `0x02` — `MSG_HEADER`

| Property | Value |
|---|---|
| Direction | Server → Client |
| Payload | 26 bytes (raw binary) |
| Purpose | Vault header containing lockdown state |

Payload is the raw 26-byte `.gdb` header:

```
[0]     failed_attempts        (uint8)
[1-8]   lockdown_timestamp     (int64, big-endian)
[9]     override_attempts_left (uint8)
[10-17] monotonic_boot_ticks   (uint64, big-endian)
[18-25] wallclock_last_seen    (int64, big-endian)
```

---

### `0x03` — `MSG_GET_CIPHERTEXT`

| Property | Value |
|---|---|
| Direction | Client → Server |
| Payload | None |
| Purpose | Request encrypted vault payload |

Server responds with `MSG_CIPHERTEXT` (0x04) or `MSG_ERROR` (0x09).

---

### `0x04` — `MSG_CIPHERTEXT`

| Property | Value |
|---|---|
| Direction | Server → Client |
| Payload | Raw ciphertext bytes |
| Purpose | Encrypted vault payload |

The ciphertext is ChaCha20-Poly1305 encrypted. Format: `[12-byte nonce][ciphertext + 16-byte tag]`.

---

### `0x05` — `MSG_UPDATE_HEADER`

| Property | Value |
|---|---|
| Direction | Client → Server |
| Payload | 26 bytes (raw binary) |
| Purpose | Update vault header |

Server responds with `MSG_ACK` (0x08) or `MSG_ERROR` (0x09).

---

### `0x06` — `MSG_UPDATE_CIPHERTEXT`

| Property | Value |
|---|---|
| Direction | Client → Server |
| Payload | Re-encrypted ciphertext bytes |
| Purpose | Store new/updated encrypted vault data |

Server responds with `MSG_ACK` (0x08) or `MSG_ERROR` (0x09).

---

### `0x07` — `MSG_TRIGGER_WIPE`

| Property | Value |
|---|---|
| Direction | Client → Server |
| Payload | None |
| Purpose | Trigger full vault self-destruct (confirmed user action) |

Initiates the 7-pass anti-forensic shredder. Server responds with `MSG_ACK` (0x08) before beginning the wipe, then closes the WebSocket.

---

### `0x08` — `MSG_ACK`

| Property | Value |
|---|---|
| Direction | Bidirectional |
| Payload | None |
| Purpose | Acknowledge successful operation |

Empty acknowledgment. No payload.

---

### `0x09` — `MSG_ERROR`

| Property | Value |
|---|---|
| Direction | Server → Client |
| Payload | UTF-8 string |
| Purpose | Error response |

Payload is a human-readable (but sanitized) error message. Never contains plaintext keys, raw memory addresses, or stack traces.

Common error messages:
- `"Vault not found"`
- `"Invalid token"`
- `"Vault locked"`
- `"Incorrect password"`
- `"Lockdown active — override required"`
- `"Override attempts exhausted"`
- `"Internal error"`

---

### `0x0A` — `MSG_PANIC_WIPE`

| Property | Value |
|---|---|
| Direction | Client → Server |
| Payload | None |
| Purpose | Panic-key triggered disguised self-destruct |

Behaves identically to `MSG_TRIGGER_WIPE` (0x07) but the server responds with a fake success sequence to maintain the deception.

---

### `0x0B` — `MSG_GENERATE_MATRIX`

| Property | Value |
|---|---|
| Direction | Client → Server |
| Payload | JSON object |
| Purpose | Trigger entropy file generation |

Payload format:

```json
{
  "line_count": 20,
  "entropy_path": "/home/user/.grimlocker/entropy.bin"
}
```

Progress is streamed via `MSG_PROGRESS` (0x0C). Final result via `MSG_GENERATION_RESULT` (0x0D).

---

### `0x0C` — `MSG_PROGRESS`

| Property | Value |
|---|---|
| Direction | Server → Client |
| Payload | JSON object |
| Purpose | Streaming progress updates during long operations |

Payload format:

```json
{
  "progress": 45,
  "stage": "generating_entropy",
  "message": "Generating cryptographically secure random bytes..."
}
```

| Field | Type | Description |
|---|---|---|
| `progress` | int | 0-100 percentage |
| `stage` | string | Current operation stage |
| `message` | string | Human-readable status |

Stage values: `"generating_entropy"`, `"encoding_matrix"`, `"deriving_keys"`, `"encrypting_vault"`, `"finalizing"`.

---

### `0x0D` — `MSG_GENERATION_RESULT`

| Property | Value |
|---|---|
| Direction | Server → Client |
| Payload | JSON object |
| Purpose | Entropy generation complete |

Payload format:

```json
{
  "key_hex": "a1b2c3d4...",
  "coordinates": ["A3", "F12", "B7", "C5"],
  "entropy_size": 200
}
```

`key_hex` is the 200-character entropy key. `coordinates` are the extracted coordinate positions.

---

### `0x0E` — `MSG_ZEROIZE_CONFIRM`

| Property | Value |
|---|---|
| Direction | Client → Server |
| Payload | None |
| Purpose | Client confirms JavaScript state has been zeroized |

Sent after the Single Glance screen's 30-second timer expires and the UI has overwritten the entropy key buffer. Server acknowledges with `MSG_ACK` (0x08).

---

## Request/Response Patterns

### Vault Status Check

```
Client → Server:  MSG_GET_HEADER (0x01)
Server → Client:  MSG_HEADER (0x02)   → success
                  MSG_ERROR  (0x09)   → no vault yet
```

### Vault Unlock

```
Client → Server:  MSG_GET_HEADER (0x01)
Server → Client:  MSG_HEADER (0x02)

Client → Server:  MSG_GET_CIPHERTEXT (0x03)
Server → Client:  MSG_CIPHERTEXT (0x04)

[Client uses Rust via CGO to decrypt ciphertext with derived key]
[Client re-encrypts with session key]

Client → Server:  MSG_UPDATE_HEADER (0x05)     → reset failed_attempts
Server → Client:  MSG_ACK (0x08)
```

### Vault Save

```
Client → Server:  MSG_UPDATE_CIPHERTEXT (0x06)  → encrypted payload
Server → Client:  MSG_ACK (0x08)
```

### Entropy Generation (Onboarding)

```
Client → Server:  MSG_GENERATE_MATRIX (0x0B)    → {line_count, entropy_path}
Server → Client:  MSG_PROGRESS (0x0C)           → {progress: 15, ...}
Server → Client:  MSG_PROGRESS (0x0C)           → {progress: 45, ...}
Server → Client:  MSG_PROGRESS (0x0C)           → {progress: 75, ...}
Server → Client:  MSG_PROGRESS (0x0C)           → {progress: 95, ...}
Server → Client:  MSG_GENERATION_RESULT (0x0D)  → {key_hex, coordinates, ...}

[User views Single Glance screen (30-second timer)]
[Timer expires, UI zeroizes]

Client → Server:  MSG_ZEROIZE_CONFIRM (0x0E)
Server → Client:  MSG_ACK (0x08)
```

### Lockdown Flow

```
[3 failed login attempts detected by client]

Client → Server:  MSG_UPDATE_HEADER (0x05)     → lockdown_timestamp = now()
Server → Client:  MSG_ACK (0x08)

[User enters coordinate override]

Client → Server:  MSG_GET_HEADER (0x01)        → check override_attempts_left
Server → Client:  MSG_HEADER (0x02)

[If override attempts > 0 and coordinates correct]
Client → Server:  MSG_GET_CIPHERTEXT (0x03)
Server → Client:  MSG_CIPHERTEXT (0x04)
[Client decrypts, re-encrypts]
Client → Server:  MSG_UPDATE_HEADER (0x05)     → reset all counters
Server → Client:  MSG_ACK (0x08)

[If override attempts exhausted or wrong]
Client → Server:  MSG_TRIGGER_WIPE (0x07)       → or MSG_PANIC_WIPE (0x0A)
Server → Client:  MSG_ACK (0x08)
[Server shreds vault, closes WebSocket]
```

---

### `0x0F` — `MSG_INITIALIZE_VAULT` through `0x40` — `MSG_AUTH_LOGOUT_ACK`

For the complete listing of message types `0x0F`–`0x40`, see [`grimdb/docs/IPC_MESSAGE_TYPES.md`](../grimdb/docs/IPC_MESSAGE_TYPES.md).

---

### `0x41` — `MSG_FILE_DOWNLOAD_REQUEST`

| Property | Value |
|---|---|
| Direction | Client → Server |
| Payload | JSON object |
| Purpose | Begin streaming download of an encrypted vault file |

```json
{ "manifest_block_id": "abc123..." }
```

Server streams `MSG_FILE_CHUNK_DATA` (0x42) frames then sends `MSG_FILE_DOWNLOAD_END` (0x43).

---

### `0x42` — `MSG_FILE_CHUNK_DATA`

| Property | Value |
|---|---|
| Direction | Server → Client |
| Payload | Binary (decrypted + decompressed) |
| Purpose | One chunk of the downloaded file |

Raw binary frames. Collect all chunks until `0x43` arrives, then verify SHA-256.

---

### `0x43` — `MSG_FILE_DOWNLOAD_END`

| Property | Value |
|---|---|
| Direction | Server → Client |
| Payload | JSON object |
| Purpose | Download complete — integrity metadata |

```json
{
  "sha256": "e3b0c44298fc1c...",
  "total_size": 204800,
  "file_name": "report.pdf",
  "mime_type": "application/pdf"
}
```

Verify the assembled bytes match `sha256` before opening the file.

---

### `0x44` — `MSG_WORKSPACE_RENAME`

| Property | Value |
|---|---|
| Direction | Client → Server |
| Payload | JSON object |
| Purpose | Rename a workspace |

```json
{ "id": "abc-123", "name": "Work Projects" }
```

Server responds with `MSG_ACK` (0x08) or `MSG_ERROR` (0x09).

---

### `0x45` — `MSG_PANIC_BUTTON`

| Property | Value |
|---|---|
| Direction | Client → Server |
| Payload | JSON object |
| Purpose | Admin-only: initiate account-compromise vault destruction |

```json
{ "passphrase": "<admin passphrase>" }
```

Passphrase verified in Rust enclave. On success: noise-overwrites all vault data, invalidates all sessions, responds with fake success, exits. See `docs/MODERN_SECURITY_ARCHITECTURE.md`.

---

### `0x50` — `MSG_DISCOVER_SERVERS`

| Property | Value |
|---|---|
| Direction | Client → Server |
| Payload | `{}` |
| Purpose | Scan local network for Enterprise vault servers (mDNS) |

Server responds with `MSG_SERVER_LIST` (0x51).

---

### `0x51` — `MSG_SERVER_LIST`

| Property | Value |
|---|---|
| Direction | Server → Client |
| Payload | JSON array |
| Purpose | List of discovered Enterprise servers |

```json
[
  { "name": "vault-primary", "address": "10.8.0.10", "port": 9443, "tls_required": true },
  { "name": "vault-replica", "address": "10.8.0.11", "port": 9443, "tls_required": true }
]
```

---

### `0x52`–`0x56` — Enterprise User Management

Admin-only operations. All require `roles: ["admin"]` in the session.

| Code | Name | Payload |
|---|---|---|
| `0x52` | `MSG_ENTERPRISE_USER_CREATE` | `{username, roles[]}` → daemon returns one-time password |
| `0x53` | `MSG_ENTERPRISE_USER_LIST` | `{}` → server returns user list via 0x56 |
| `0x54` | `MSG_ENTERPRISE_USER_REVOKE` | `{user_id}` → soft-delete, sessions invalidated |
| `0x55` | `MSG_ENTERPRISE_USER_RESTORE` | `{user_id}` → re-enable, new one-time password issued |
| `0x56` | `MSG_ENTERPRISE_USER_RESULT` | Server response — user object or array |

See [`grimdb/docs/ENTERPRISE_FEATURES.md`](../grimdb/docs/ENTERPRISE_FEATURES.md) for the full user lifecycle.

---

## Error Handling

### Client Error Handling

| Scenario | Action |
|---|---|
| WebSocket connection refused | Display "Daemon not running" |
| Token rejected (close 4001) | Display "Authentication failed" |
| Protocol error (close 4002) | Display "Communication error — restart required" |
| `MSG_ERROR`: "Vault locked" | Show login screen |
| `MSG_ERROR`: "Lockdown active" | Show LockdownScreen with timer |
| `MSG_ERROR`: "Internal error" | Display generic error (no details exposed) |
| Unexpected message type | Log warning, ignore |
| Timeout (no response 10s) | Display "Connection lost — retrying" |

### Server Error Handling

| Scenario | Action |
|---|---|
| Malformed message | Send `MSG_ERROR`, close WebSocket (4002) |
| Invalid token | Close WebSocket (4001) |
| Request before vault exists | Send `MSG_ERROR: "Vault not found"` |
| `MSG_UPDATE_*` without lock check | Send `MSG_ERROR: "Vault locked"` |
| Internal Go panic | Recover, send `MSG_ERROR: "Internal error"`, log audit |
| CGO/Rust panic | Zeroize keys, send `MSG_ERROR: "Internal error"`, initiate lockdown |

---

## CLI Client API

The Go CLI client (`cmd/client/`) uses Unix domain sockets (single-user) or mTLS TCP (enterprise) instead of WebSocket. The message format is identical.

### Local Mode

```
Socket path:
  Linux/macOS: /tmp/grimlocker.sock
  Windows:     \\.\pipe\grimlocker

Commands:
  grimlocker init   <password>               Initialize a new vault (first run)
  grimlocker unlock <password|oidc-token>    Authenticate and unlock
  grimlocker get    <title-or-uuid>          Retrieve entry (search by title)
  grimlocker set    <id> <value> [category]  Create entry
  grimlocker update <id> <value> [category]  Update entry
  grimlocker delete <title-or-uuid>          Delete entry (search by title)
  grimlocker list   [category]               List entries
  grimlocker lock                            Lock vault
  grimlocker status                          Vault status (locked / lockdown state)
  grimlocker health                          Daemon health JSON
  grimlocker audit  [count]                  Recent audit log (default: 20)
  grimlocker version                         Print version (no daemon needed)
  grimlocker help                            Show usage
```

### Tier Detection (Environment)

The CLI auto-detects the tier from environment variables:

```bash
# Single-user (local IPC)
export GRIMLOCKER_IPC=ws://127.0.0.1:<port>/ws
export GRIMLOCKER_TOKEN=<token>

# Enterprise (remote mTLS)
export GRIMLOCKER_DAEMON_ADDR=grimlocker.example.com:9443
export GRIMLOCKER_CLIENT_CERT=deploy/tls/client.crt
export GRIMLOCKER_CLIENT_KEY=deploy/tls/client.key
export GRIMLOCKER_CA_CERT=deploy/tls/ca.crt
```

---

## File Upload Protocol (WebSocket Binary)

File ingestion uses a three-phase streaming protocol over the binary WebSocket:

```
Phase 1 — BEGIN:
  Client → Server: 0x20 MSG_FILE_INGEST_BEGIN
    Payload: {"file_name":"doc.pdf","mime_type":"application/pdf","total_size":102400}
  Server → Client: 0x08 MSG_ACK
    Payload: {"status":"ready"}

Phase 2 — CHUNKS (repeated):
  Client → Server: 0x21 MSG_FILE_CHUNK
    Payload: <64 KB raw binary data>
  Server → Client: 0x23 MSG_INGEST_PROGRESS (optional)
    Payload: {"progress":0.42,"stage":"ingesting","message":"43008 / 102400 bytes"}

Phase 3 — END:
  Client → Server: 0x22 MSG_FILE_INGEST_END
    Payload: (empty)
  Server → Client: 0x1D MSG_ENTRY_RESULT
    Payload: {"id":"<uuid>","file_name":"doc.pdf","mime_type":"...","sha256":"<hex>",
              "chunk_ids":["..."],"compressed":true,"algorithm":"zstd","total_size":102400}
    OR
  Server → Client: 0x09 MSG_ERROR
    Payload: "mvk not available" | "ingest timeout" | ...
```

The ingest engine pipeline per chunk: **Read → SHA-256 hash → zstd compress → ChaCha20-Poly1305 encrypt → Write block**

On any error, all written chunks are automatically rolled back (atomic ingest).

---

## Reconnect & Vault State Persistence

The daemon does **not** reset the vault state when the WebSocket disconnects. The session remains unlocked across reconnects (e.g. Tauri window navigation, browser refresh).

On every new WebSocket connection, `HandshakeStatus` sends:

```json
// MSG_ACK (0x08) payload
{
  "status": "Online",
  "initialized": true,
  "unlocked": true
}
```

The UI checks `unlocked` to decide whether to show the vault dashboard directly (re-attach) or the unlock screen (fresh session).
