# API Reference

This document specifies the complete IPC protocol, WebSocket API, message types, and client-server interaction patterns.

---

## Transport Layer

### Single-User Mode

| Port | Protocol | Purpose |
|---|---|---|
| `8080` | HTTP | Serves embedded static UI assets (go:embed). No authentication required. |
| `8374` | WebSocket | IPC bridge for frontend-daemon communication. Token-based authentication. |

### Enterprise Mode

Additional ports/configurations are documented in the Enterprise deployment guide.

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
  grimlocker client unlock
  grimlocker client lock
  grimlocker client status
  grimlocker client create-entry --title "..." --username "..." --password "..."
  grimlocker client list-entries
  grimlocker client get-entry --id "..."
  grimlocker client delete-entry --id "..."
  grimlocker client wipe
  grimlocker client health
```

### Remote Mode (Enterprise)

```
grimlocker client --remote --addr grimlocker.example.com:9443 --cert client.crt --key client.key unlock
```
