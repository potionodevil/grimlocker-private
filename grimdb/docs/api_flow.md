# Grimlocker Omega+ — API Flow Diagrams

## 1. Vault Unlock & VFS Auto-Mount

```mermaid
sequenceDiagram
    UI->>Translator: MsgUnlockVault (password)
    Translator->>Bus: AUTH.UNLOCK event
    Bus->>Security: Handle AUTH.UNLOCK
    Security->>Security: Verify password, derive MVK
    Security->>Storage: StoreMVK(handle)
    Security->>Bus: AUTH.RESULT {success: true, handle}
    Bus->>Watchdog: AUTH.RESULT received
    Watchdog->>Bus: STORAGE.VFS_MOUNT
    Bus->>Storage: Handle mount
    Storage->>Storage: LoadIndex()
    Watchdog->>Bus: STORAGE.READY
    Bus->>Translator: STORAGE.READY event
    Translator->>UI: MsgLogBroadcast "STORAGE.READY"
    UI->>UI: Navigate to Dashboard
```

## 2. File Ingest via io.Pipe Streaming

```mermaid
sequenceDiagram
    UI->>Translator: MsgFileIngestBegin {name, mime, size}
    Translator->>EntryHandler: HandleIngestBegin()
    EntryHandler->>EntryHandler: Create io.Pipe()
    EntryHandler->>IngestEngine: Start Ingest(pr, name, mime, progressFn)
    IngestEngine->>IngestEngine: Read chunks, encrypt, write blocks
    EntryHandler->>UI: MsgAck "ready"
    UI->>Translator: MsgFileChunk[0] (4MB data)
    Translator->>EntryHandler: HandleChunk()
    EntryHandler->>EntryHandler: pw.Write(chunk)
    IngestEngine->>IngestEngine: Decrypt fails; re-encrypt with MVK
    Translator->>UI: MsgIngestProgress {pct: 0.25}
    UI->>Translator: MsgFileChunk[1]
    Translator->>EntryHandler: HandleChunk()
    Translator->>UI: MsgIngestProgress {pct: 0.50}
    ... more chunks ...
    UI->>Translator: MsgFileIngestEnd
    Translator->>EntryHandler: HandleIngestEnd()
    EntryHandler->>EntryHandler: pw.Close() → EOF
    IngestEngine->>IngestEngine: Finalize, write BlobManifest
    Translator->>UI: MsgEntryResult {manifest}
    UI->>UI: Display file in vault
```

## 3. CRUD with Policy Validation

```mermaid
sequenceDiagram
    UI->>Translator: MsgEntryCreate {title, fields}
    Translator->>EntryHandler: HandleCreate()
    EntryHandler->>PolicyManager: CheckWrite(subjectID)
    PolicyManager->>PolicyManager: Verify permission
    alt Permission Denied
        PolicyManager->>AuditLog: Append {UNAUTHORIZED_ACCESS}
        EntryHandler->>UI: MsgError "unauthorized"
    else Permission Granted
        EntryHandler->>Bus: ENTRY.CREATE event
        Bus->>EntryModule: Handle ENTRY.CREATE
        EntryModule->>BlockStore: WriteBlock()
        EntryModule->>Bus: ENTRY.RESULT {id, title}
        Translator->>UI: MsgEntryResult
    end
```

## 4. Watchdog Heartbeat & Recovery

```mermaid
sequenceDiagram
    Watchdog->>Bus: INTEGRITY.CHECK (every 30s)
    Bus->>IntegrityMonitor: Handle check
    IntegrityMonitor->>IntegrityMonitor: Hash binary
    IntegrityMonitor->>Bus: INTEGRITY.RESULT {match: true}
    Watchdog->>Watchdog: Heartbeat OK
    rect Timeout Scenario
        Watchdog->>Bus: INTEGRITY.CHECK (no response for 5s)
        Watchdog->>Bus: SECURITY.AUDIT {KERNEL_RESTART}
        Watchdog->>Registry: StartAll(ctx) — restart modules
    end
```

## 5. Audit Log Cryptographic Chaining

Each SecurityEvent includes:
- `hash = SHA-256(prevHash || timestamp || level || module || message || subjectID)`
- `prevHash` references the previous entry's hash
- Forms an immutable cryptographic chain

Example:
```
Entry 1: hash = SHA-256(0...0 || 1234 || INFO || policy || "LOGIN" || "user1")
Entry 2: hash = SHA-256(Entry1.hash || 5678 || CRITICAL || policy || "UNAUTHORIZED_ACCESS" || "hacker")
Entry 3: hash = SHA-256(Entry2.hash || 9012 || INFO || storage || "FILE_WRITTEN" || "user1")
```

If Entry 2 is tampered with, Entry 3's prevHash no longer matches its recomputed value.

## 6. Graceful Shutdown Flow (Tauri → Daemon)

```mermaid
sequenceDiagram
    participant T as Tauri (main.rs)
    participant D as Daemon (main.go)
    participant B as BlockStore
    participant S as Security Module
    participant R as Rust Enclave
    participant K as Kernel Bus

    T->>D: POST /shutdown
    D->>T: 200 {"status":"shutting_down"}
    Note over D: 100ms delay (flush HTTP response)
    D->>B: Flush() — complete in-flight writes
    D->>B: Close()
    D->>S: sessionCtx.Lock() — revoke MVK
    D->>R: SessionDestroy(sessionKeyHandle)
    D->>K: bus.Shutdown(5s timeout)
    K->>K: Stop() all modules
    D->>D: log "Shutdown complete"
    D->>D: os.Exit(0)

    Note over T: Polls child.try_wait() every 100ms (3s timeout)
    T->>T: Process exited → "Daemon shut down gracefully"
    Note over T: Fallback: child.kill() after 3s timeout
```

## 7. WebSocket Reconnect — Vault State Re-Attach

The daemon persists vault unlock state across WebSocket disconnects. The Tauri app briefly disconnects during page navigation.

```mermaid
sequenceDiagram
    participant UI as Tauri UI
    participant B as WS Bridge
    participant T as Translator
    participant S as SessionContext

    UI->>B: connect (new WebSocket)
    B->>T: handshake callback
    T->>S: IsUnlocked() → true
    T->>B: MsgAck {status:"Online", initialized:true, unlocked:true}
    B->>UI: MsgAck payload
    Note over UI: unlocked:true → skip login screen,\ngo directly to Dashboard
```

## Message Flow Summary

| Message | Byte | Direction | Handler | Purpose |
|---------|------|-----------|---------|---------|
| `MsgFileIngestBegin` | `0x20` | Client→Server | `EntryHandler.HandleIngestBegin` | Start file upload |
| `MsgFileChunk` | `0x21` | Client→Server | `EntryHandler.HandleChunk` | Stream file data |
| `MsgFileIngestEnd` | `0x22` | Client→Server | `EntryHandler.HandleIngestEnd` | Complete upload |
| `MsgIngestProgress` | `0x23` | Server→Client | `EntryHandler` (pushed) | Report progress |
| `MsgEntryCreate` | `0x18` | Client→Server | `EntryHandler.HandleCreate` | New entry |
| `MsgEntryUpdate` | `0x19` | Client→Server | `EntryHandler.HandleUpdate` | Modify entry |
| `MsgEntryDelete` | `0x1A` | Client→Server | `EntryHandler.HandleDelete` | Remove entry |
| `MsgEntryResult` | `0x1D` | Server→Client | `Translator` | CRUD / ingest result |
| `MsgAck` | `0x08` | Server→Client | Bridge handshake | `{status,initialized,unlocked}` |
| `MsgLogBroadcast` | — | Server→Client | `Translator` (pushed to all) | Security/lifecycle events |

## REST API Action Map (`/api/v1`)

| Action | Event | Vault Required |
|--------|-------|---------------|
| `vault.unlock` | `AUTH.UNLOCK` | No (unlocks it) |
| `vault.logout` | `AUTH.LOGOUT` | Yes |
| `vault.status` | `AUTH.STATUS` | No |
| `entry.create` | `ENTRY.CREATE` | Yes |
| `entry.read` | `ENTRY.READ` | Yes |
| `entry.update` | `ENTRY.UPDATE` | Yes |
| `entry.delete` | `ENTRY.DELETE` | Yes |
| `entry.query` | `ENTRY.QUERY` | Yes |
| `storage.write` | `STORAGE.WRITE` | Yes |
| `storage.read` | `STORAGE.READ` | Yes |
| `storage.list` | `STORAGE.LIST` | Yes |
