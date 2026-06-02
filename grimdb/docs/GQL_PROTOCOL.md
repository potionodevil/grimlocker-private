# GrimQueryLanguage (GQL) Protocol Specification

## Version 1.0 — Binary Frame Protocol

GQL is Grimlocker's binary query protocol designed for **Total Injection Immunity**.
No text parsing occurs at any point in the query pipeline. Every field is
length-prefixed binary, every frame is schema-validated, and every operation
is ACL-checked before reaching the dispatcher.

---

## 1. Frame Format

```
┌──────────────────────────────────────────────────────────────────┐
│ Offset  │ Size   │ Field        │ Type    │ Description          │
├─────────┼────────┼──────────────┼─────────┼──────────────────────┤
│ 0       │ 1      │ Version      │ uint8   │ Always 1             │
│ 1       │ 1      │ Opcode       │ uint8   │ 1=Query,2=Mutate,... │
│ 2-3     │ 2      │ Flags        │ uint16  │ Bitmask (BE)         │
│ 4-7     │ 4      │ PayloadSize  │ uint32  │ Payload length (BE)  │
│ 8+      │ N      │ Payload      │ []byte  │ Binary-encoded query │
└──────────────────────────────────────────────────────────────────┘
```

### Opcodes

| Value | Name    | Direction        | Description                         |
|-------|---------|------------------|-------------------------------------|
| 0x01  | Query   | Client → Server  | Read-only operation                 |
| 0x02  | Mutate  | Client → Server  | Write/create/update/delete          |
| 0x03  | Result  | Server → Client  | Successful response (JSON payload)  |
| 0x04  | Error   | Server → Client  | Error response (JSON payload)       |

### Flags

| Bit   | Flag        | Description                |
|-------|-------------|----------------------------|
| 0x01  | Compressed  | Payload is zstd-compressed |
| 0x02  | Encrypted   | Payload is SKE-encrypted   |

---

## 2. Query Payload (Binary Encoding)

The payload is a length-prefixed binary structure. All multi-byte integers
are big-endian. All strings are `length(uint16, BE) + data(bytes)`.

```
┌──────────────┬──────────┬──────────────────────────────┐
│ Offset       │ Size     │ Field                        │
├──────────────┼──────────┼──────────────────────────────┤
│ 0            │ 1        │ field_count (uint8)          │
│ 1            │ 2+N      │ namespace (length-prefixed)  │
│ ...          │ 2+N      │ entry_id (length-prefixed)   │
│ ...          │ 2+N      │ category (length-prefixed)   │
│ ...          │ 2+N      │ title (length-prefixed)      │
│ ...          │ varies   │ fields (field_count pairs)   │
│              │          │   key: length(2)+data        │
│              │          │   value: length(2)+data      │
│ ...          │ 4        │ limit (uint32)               │
│ ...          │ 4        │ offset (uint32)              │
│ ...          │ 2+N      │ credentials (length-prefixed)│
└──────────────┴──────────┴──────────────────────────────┘
```

### Length Limits

| Field           | Max Length | Description                  |
|-----------------|------------|------------------------------|
| namespace       | 128 bytes  | Workspace or user identifier |
| entry_id        | 64 bytes   | Entry unique identifier      |
| category        | 32 bytes   | Entry type filter            |
| title           | 8192 bytes | Human-readable title         |
| field key       | 64 bytes   | Single field name            |
| field value     | 8192 bytes | Single field value           |
| fields count    | 100        | Maximum fields per entry     |
| total payload   | 16 MiB     | Max frame payload size       |

---

## 3. Operations

### Query Operations (Opcode 0x01)

| Operation      | Description                                     |
|----------------|-------------------------------------------------|
| `list_entries` | List all entries in namespace                   |
| `get_entry`    | Retrieve a single entry by ID                   |
| `query_entries`| Filter entries by category                      |

### Mutate Operations (Opcode 0x02)

| Operation      | Description                                     |
|----------------|-------------------------------------------------|
| `create_entry` | Create a new vault entry                        |
| `update_entry` | Modify an existing entry                        |
| `delete_entry` | Remove an entry                                 |

---

## 4. Validation Pipeline

Every frame passes through a **two-stage validator** before reaching the
dispatcher:

### Stage 1: Syntactic Validation

- Version must be `1`
- Opcode must be known (1-4)
- Payload must decode into valid binary structure
- All string fields must be within length limits
- Identifiers (namespace, entry_id, category, field keys) must contain only `a-z`, `A-Z`, `0-9`, `_`, `-`, `.`
- Text fields (title, field values) must contain only printable characters (no control chars, no DEL)
- No null bytes in any string field
- Field count must not exceed `MaxFieldsCount` (100)
- Opcode and operation must match (Query ↔ read op, Mutate ↔ write op)

### Stage 2: Semantic (ACL) Validation

- Session must be unlocked
- Active MVK handle must exist
- Write operations require non-empty credentials
- Namespace must match `session.UserID()` (unless admin role)
- Admin role bypasses namespace restriction

### Error Codes

| Code  | Type      | Description                           |
|-------|-----------|---------------------------------------|
| -100  | Internal  | GQL dispatcher unavailable            |
| -101  | Frame     | Invalid frame (decode error)          |
| -102  | Syntactic | Schema validation failure             |
| -103  | Semantic  | ACL / authorization failure           |
| -104  | Frame     | Not a query frame (result/error only) |
| -105  | Dispatch  | Operation dispatch error              |
| -10   | Input     | entry_id required                     |
| -11   | Storage   | Entry not found                       |
| -20   | Storage   | Category query failed                 |
| -30   | Entry     | Create failed                         |
| -31   | Entry     | Update failed                         |
| -32   | Entry     | Delete failed                         |

---

## 5. Security Properties

### Total Injection Immunity

Because GQL is **binary-only** with strict field validation, there is no
injection surface:

- **No SQL injection**: No SQL parser exists in the query path
- **No JSON injection**: Queries are binary, not JSON
- **No command injection**: No shell or command execution is triggered
- **No path traversal**: Identifiers reject `.` and `/` characters
- **No null byte injection**: Null bytes rejected at the syntactic level
- **No control character injection**: Characters below 0x20 (except tab) are rejected

### What IS Allowed (User Data Only)

- Printable characters in title and field values (including `<`, `>`, `"`, etc.)
- These are treated as opaque user data — the security boundary for display
  is at the rendering layer, not the query layer

### Defense in Depth

1. **Binary frame**: No text parser, no grammar, no ambiguous encoding
2. **Length-prefixed fields**: No delimiters to smuggle, no escape sequences
3. **Identifier validation**: Only `[a-zA-Z0-9_.-]` for structural fields
4. **Printable-only text**: No control characters in user-facing fields
5. **Length limits**: Every field has a hard maximum, preventing buffer attacks
6. **ACL check**: Namespace and role validated against session context
7. **No reflection**: Queries don't echo back to the client unmodified — results are always server-generated JSON

---

## 6. SDK Usage (Go)

```go
import "github.com/grimlocker/grimdb/gql"

// Build a query
query := &gql.GQLQuery{
    Namespace: "default",
    Operation: gql.OpListEntries,
    Limit:     50,
}

// Encode to binary frame
frame := gql.NewQueryFrame(query)
data := frame.Encode()

// Send over WebSocket as BinaryMessage
conn.WriteMessage(websocket.BinaryMessage, data)

// Receive and decode response
_, respData, _ := conn.ReadMessage()
respFrame, _ := gql.DecodeFrame(respData)

// Handle result
var result gql.GQLResult
json.Unmarshal(respFrame.Payload, &result)
```

### High-Level SDK Client

```go
import "github.com/grimlocker/grimdb/sdk"

client, _ := sdk.DialGQL(ctx, "ws://127.0.0.1:11003/ws?token=...")
defer client.Close()

entries, _ := client.ListEntries(ctx, "default")
entry, _ := client.CreateEntry(ctx, "default", "My Key", "SSH_KEY", map[string]string{
    "publicKey": "ssh-ed25519 AAAAC3...",
})
```

---

## 7. Testing

### Validator Unit Tests

```bash
go test ./gql/ -v -run TestValidate
```

### Fuzz Testing

```bash
go test ./gql/ -v -run TestFuzz
```

Benchmark:

```bash
go test ./gql/ -bench=. -benchmem
```

### Standalone Tester CLI

```bash
go run ./gql/cli/ --verbose --fuzz 10000
```

---

## Appendix: Comparison with Traditional Protocols

| Property            | SQL       | REST/JSON  | GraphQL    | GQL         |
|---------------------|-----------|------------|------------|-------------|
| Injection surface   | Very high | Medium     | Medium     | **None**    |
| Query format        | Text      | Text/JSON  | Text/JSON  | **Binary**  |
| Schema validation   | None      | Optional   | Optional   | **Mandatory**|
| ACL enforcement     | External  | External   | External   | **Built-in**|
| Parse complexity    | O(n²)     | O(n)       | O(n)       | **O(1)**    |
| Zero-copy possible  | No        | No         | No         | **Yes**     |

---

## 8. Extended Operations (File Vault, Workspace, Sync, Audit, Tools)

The following operations were added in Grimlocker Omega v2. They use the same
binary frame format and two-stage validation pipeline as the core operations.

### File Vault Operations

| Operation          | Opcode | Description                              |
|-------------------|--------|------------------------------------------|
| `file.list_folder`   | Query   | List contents of a FileVault folder       |
| `file.create_folder` | Mutate  | Create a new folder                      |
| `file.rename_folder` | Mutate  | Rename a folder                          |
| `file.delete_folder` | Mutate  | Delete a folder (must be empty)          |
| `file.move`          | Mutate  | Move a file between folders             |
| `file.ingest`        | Mutate  | Upload a file (base64-encoded)          |
| `file.download`      | Query   | Download a file (returns base64 data)   |

### Workspace Operations

| Operation             | Opcode | Description                           |
|-----------------------|--------|---------------------------------------|
| `workspace.list`      | Query   | List all workspaces                  |
| `workspace.create`    | Mutate  | Create a new workspace               |
| `workspace.switch`    | Mutate  | Switch to a different workspace      |
| `workspace.rename`    | Mutate  | Rename a workspace                   |
| `workspace.delete`    | Mutate  | Delete a workspace                   |

### Sync Operations

| Operation            | Opcode | Description                           |
|----------------------|--------|---------------------------------------|
| `sync.list_peers`    | Query   | List discovered LAN sync peers        |
| `sync.trigger`       | Mutate  | Trigger an immediate sync cycle       |

### Audit Operations

| Operation            | Opcode | Description                           |
|----------------------|--------|---------------------------------------|
| `audit.list`         | Query   | Fetch recent security audit events    |

### Tool Operations

| Operation            | Opcode | Description                           |
|----------------------|--------|---------------------------------------|
| `tool.ssh_keygen`    | Mutate  | Generate an Ed25519 SSH key pair       |
| `vault.recovery_phrase` | Query | Retrieve recovery phrase (requires password) |
| `vault.status`       | Query   | Health check (initialized, unlocked)  |

### Search Operation

| Operation            | Opcode | Description                           |
|----------------------|--------|---------------------------------------|
| `search_entries`     | Query   | Full-text search across entry titles  |

---

## 9. IPC Message Type Extensions (0x60–0x74)

These IPC message types were added to support FileVault folders, LAN Sync,
and the Security Audit Log:

| Type    | Name                  | Direction        | Description                    |
|---------|-----------------------|------------------|--------------------------------|
| `0x60`  | MsgFolderCreate       | Client → Server  | Create FileVault folder        |
| `0x61`  | MsgFolderList         | Client → Server  | List folder contents           |
| `0x62`  | MsgFolderRename       | Client → Server  | Rename folder                  |
| `0x63`  | MsgFolderDelete       | Client → Server  | Delete folder                  |
| `0x64`  | MsgFileMoveToFolder   | Client → Server  | Move file between folders      |
| `0x65`  | MsgFolderResult       | Server → Client  | Folder operation result        |
| `0x70`  | MsgSyncListPeers      | Client → Server  | Request sync peer list         |
| `0x71`  | MsgSyncTrigger        | Client → Server  | Trigger immediate sync         |
| `0x72`  | MsgSyncResult         | Server → Client  | Sync operation result          |
| `0x73`  | MsgAuditList          | Client → Server  | Request audit log entries      |
| `0x74`  | MsgAuditResult        | Server → Client  | Audit log entries (SKE)        |

---

## Changelog

### v1.1 (Omega v2)
- Added File Vault operations (upload, download, folder CRUD, move)
- Added Workspace operations (list, create, switch, rename, delete)
- Added Sync operations (list peers, trigger)
- Added Audit operations (list events with SKE encryption)
- Added SSH key generation tool operation
- Added recovery phrase retrieval
- Added full-text entry search
- Extended IPC message types 0x60–0x74
- SDK coverage expanded to 12 languages (Go, Python, Java, TypeScript, Rust, C#, C++, Ruby, PHP, Swift, Kotlin, Dart)
- Argon2id parameters hardened to 128MB/4 iterations (OWASP 2023+ compliant)
- Sync protocol encryption upgraded from custom XOR to ChaCha20-Poly1305
- Session auto-lock timer added (default 15-minute inactivity timeout)
- Panic button requires non-empty passphrase confirmation
- Cryptographic error messages no longer leak key/nonce length information
- MVK handles redacted from all log output
