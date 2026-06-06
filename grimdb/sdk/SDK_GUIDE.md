# Grimlocker SDK Guide

Grimlocker provides native SDKs for **12 languages**, all communicating
with the Grimlocker local daemon. Every SDK abstracts the wire protocol
(HTTP JSON or GQL Binary over WebSocket) behind a typed, idiomatic API.

> **What's New (v2.0)**
> - **Resilience**: Every SDK now has automatic retry (3x, exponential backoff)
>   and circuit breaker (5 failures → 30s open circuit) built-in.
> - **Batch Operations**: `createEntriesBatch()` and `deleteEntriesBatch()`
>   across all 12 SDKs for efficient bulk workloads.
> - **Streaming Events**: WebSocket SDKs support live event streaming
>   (`entry_changed`, `sync_complete`, `connected`, `disconnected`).
> - **Full API Parity**: All SDKs expose the same ~34 operations — no
>   second-class citizens.

---

## Quick Comparison Table

| Language | Package | Protocol | Transport | Best For |
|---|---|---|---|---|
| Go | `github.com/grimlocker/grimdb/sdk` | GQL Binary | WebSocket | Server-side, plugins |
| Python | `pip install grimlocker` | GQL Binary | WebSocket | Scripts, automation |
| Java | `com.grimlocker:grimlocker-sdk` | GQL Binary | WebSocket | Enterprise JVM |
| TypeScript | `npm i @grimlocker/sdk` | HTTP JSON | Fetch API | Web, Node.js |
| Rust | `grimlocker-sdk` (crates.io) | GQL Binary | WebSocket | Performance, WASM |
| C#/.NET | `Grimlocker.SDK` (NuGet) | HTTP JSON | HttpClient | .NET, Azure |
| C++ | header-only (CMake / vcpkg) | HTTP JSON | libcurl | Native, embedded |
| Ruby | `gem install grimlocker` | HTTP JSON | net/http | Rails, scripts |
| PHP | `composer require grimlocker/sdk` | HTTP JSON | cURL | Web backends |
| Swift | `grimlocker-sdk` (SwiftPM) | HTTP JSON | URLSession | iOS / macOS |
| Kotlin | `com.grimlocker:grimlocker-sdk-kotlin` | HTTP JSON | HttpURLConnection | Android, JVM |
| Dart | `grimlocker_sdk` (pub.dev) | HTTP JSON | http package | Flutter |

---

## Quick Start Examples

### Go

```go
package main

import (
    "context"
    "fmt"
    "github.com/grimlocker/grimdb/sdk"
)

func main() {
    client, _ := sdk.DialGQL(context.Background(), "ws://127.0.0.1:11003/ws?token=my-token")
    defer client.Close()
    entries, _ := client.ListEntries(context.Background(), "default")
    fmt.Println(len(entries))
}
```

### Python

```python
from grimlocker import Client

with Client.connect("127.0.0.1", 41753, token="my-token") as client:
    entries = client.list_entries()
    print(len(entries))
```

### Java

```java
import com.grimlocker.sdk.*;

var client = GrimlockerClient.connect("127.0.0.1", 41753, "my-token");
var entries = client.listEntries().namespace("default").execute();
System.out.println(entries.size());
client.close();
```

### TypeScript

```typescript
import { GrimlockerClient } from "@grimlocker/sdk";

const client = new GrimlockerClient("http://127.0.0.1:36353", "my-token");
const entries = await client.listEntries();
console.log(entries.length);
```

### Rust

```rust
use grimlocker_sdk::GrimlockerClient;

#[tokio::main]
async fn main() -> Result<(), grimlocker_sdk::Error> {
    let mut client = GrimlockerClient::connect("ws://127.0.0.1:36352/ws", "my-token").await?;
    let entries = client.list_entries(None).await?;
    println!("{}", entries.len());
    Ok(())
}
```

### C# / .NET

```csharp
using Grimlocker;

await using var client = new GrimlockerClient("http://127.0.0.1:36353", "my-token");
var entries = await client.ListEntriesAsync();
Console.WriteLine(entries.Count);
```

### C++

```cpp
#include <grimlocker/client.hpp>

int main() {
    grimlocker::Client client("http://127.0.0.1:36353", "my-token");
    auto entries = client.list_entries();
    std::cout << entries.size() << std::endl;
}
```

### Ruby

```ruby
require "grimlocker"

client = Grimlocker::Client.new(base_url: "http://127.0.0.1:36353", token: "my-token")
entries = client.list_entries
puts entries.length
```

### PHP

```php
<?php
require 'vendor/autoload.php';

$client = new Grimlocker\Client('http://127.0.0.1:36353', 'my-token');
$entries = $client->listEntries();
echo count($entries);
```

### Swift

```swift
import GrimlockerSDK

let client = GrimlockerClient(baseURL: URL(string: "http://127.0.0.1:36353")!, token: "my-token")
let entries = try await client.listEntries()
print(entries.count)
```

### Kotlin

```kotlin
import com.grimlocker.sdk.*

val client = GrimlockerClient("http://127.0.0.1:36353", "my-token")
val entries = client.listEntries()
println(entries.size)
client.close()
```

### Dart

```dart
import 'package:grimlocker_sdk/grimlocker_sdk.dart';

void main() async {
  final client = GrimlockerClient('http://127.0.0.1:36353', 'my-token');
  final entries = await client.listEntries();
  print(entries.length);
  client.close();
}
```

---

## Full API Surface

Every SDK exposes the same logical operations. The exact method name varies by
language convention (camelCase, snake_case, PascalCase).

### Auth

| Operation | Parameters | Returns | Description |
|---|---|---|---|
| `unlockVault` / `unlock_vault` | `password` | void | Unlock the vault with the master password. |
| `lockVault` / `lock_vault` | — | void | Lock the vault (logout). |
| `vaultStatus` / `vault_status` | — | `VaultStatus` | Returns `{ initialized, unlocked, status }`. |
| `getRecoveryPhrase` / `recovery_phrase` | `password` | `string` | Retrieve the BIP39 recovery mnemonic. |

### Entries

| Operation | Parameters | Returns | Description |
|---|---|---|---|
| `listEntries` / `list_entries` | `category?` | `VaultEntry[]` | List all entries. Pass `category` to filter. |
| `getEntry` / `get_entry` | `id` | `VaultEntry` | Read a single entry by ID. |
| `createEntry` / `create_entry` | `title`, `category`, `fields` | `VaultEntry` | Create a new generic entry. |
| `updateEntry` / `update_entry` | `id`, `fields` | void | Update an existing entry. |
| `deleteEntry` / `delete_entry` | `id` | void | Permanently delete an entry. |
| `searchEntries` / `search_entries` | `query`, `category?` | `VaultEntry[]` | Full-text search across entries. |

### Typed Helpers

| Operation | Returns | Description |
|---|---|---|
| `listPasswords` | `PasswordEntry[]` | Shorthand for `listEntries(category: "PASSWORD")`. |
| `createPassword` | `string` (ID) | Create a password entry from a `PasswordEntry` object. |
| `listSshKeys` | `SshKeyEntry[]` | Shorthand for `listEntries(category: "SSH_KEY")`. |
| `createSshKey` | `string` (ID) | Create an SSH key entry from an `SshKeyEntry` object. |
| `listCertificates` | `CertificateEntry[]` | Shorthand for `listEntries(category: "CERTIFICATE")`. |
| `createCertificate` | `string` (ID) | Create a certificate entry from a `CertificateEntry` object. |

### File Vault

| Operation | Parameters | Returns | Description |
|---|---|---|---|
| `listFolder` / `list_folder` | `folder_id?` | `FolderListing` | List files and subfolders. Empty ID = root. |
| `createFolder` / `create_folder` | `name`, `parent_id?` | `FolderItem` | Create a new folder. |
| `renameFolder` / `rename_folder` | `id`, `name` | void | Rename a folder. |
| `deleteFolder` / `delete_folder` | `id` | void | Delete a folder. |
| `moveFile` / `move_file` | `manifest_block_id`, `folder_id` | void | Move a file to a different folder. |
| `uploadFile` / `upload_file` | `data`, `file_name`, `mime_type?`, `folder_id?`, `on_progress?` | `FileEntry` | Upload a file (Base64-encoded for HTTP). |
| `downloadFile` / `download_file` | `manifest_block_id` | `bytes` | Download and decode a file. |

### Workspaces

| Operation | Parameters | Returns | Description |
|---|---|---|---|
| `listWorkspaces` / `list_workspaces` | — | `Workspace[]` | List all workspaces. |
| `createWorkspace` / `create_workspace` | `name` | `Workspace` | Create a new workspace. |
| `switchWorkspace` / `switch_workspace` | `id` | void | Set the active workspace. |
| `renameWorkspace` / `rename_workspace` | `id`, `name` | void | Rename a workspace. |
| `deleteWorkspace` / `delete_workspace` | `id` | void | Delete a workspace. |

### Sync

| Operation | Parameters | Returns | Description |
|---|---|---|---|
| `listSyncPeers` / `list_sync_peers` | — | `SyncStatus` | List peers and sync metadata. |
| `triggerSync` / `trigger_sync` | — | void | Trigger an immediate sync. |

### Audit

| Operation | Parameters | Returns | Description |
|---|---|---|---|
| `listAuditEvents` / `list_audit_events` | `n=50` | `AuditEvent[]` | Last n audit events. |

### Tools

| Operation | Parameters | Returns | Description |
|---|---|---|---|
| `generateSSHKey` / `generate_ssh_key` | `comment?`, `save_to_vault?` | `{public_key, fingerprint, entry_id?}` | Generate an SSH key pair. |
| `getRecoveryPhrase` / `recovery_phrase` | `password` | `string` | Retrieve the BIP39 recovery phrase. |

### Health

| Operation | Returns | Description |
|---|---|
| `healthCheck` / `health_check` | `VaultStatus` | Alias for `vaultStatus()`. |

---

## Resilience Features

All SDKs ship with **Retry + Circuit Breaker** built into the transport layer.

### Retry Behaviour

| Parameter | Value |
|---|---|
| Max retries | 3 (4 total attempts) |
| Base delay | 100 ms |
| Backoff | Exponential (100 → 200 → 400 ms) |
| Cap | 2 seconds |
| Retryable errors | 5xx, network timeouts, connection drops |
| Non-retryable | 4xx client errors (immediately thrown) |

### Circuit Breaker

| Parameter | Value |
|---|---|
| Failure threshold | 5 consecutive retryable errors |
| Open duration | 30 seconds |
| Half-open | One probe request allowed after timeout |
| On success | Circuit closes immediately |

> **Usage:** Zero configuration required. The SDKs handle retries and circuit
> breaking transparently on every request.

### Batch Operations

All SDKs support efficient bulk creation and deletion:

| Operation | Input | Output |
|---|---|---|
| `createEntriesBatch` | `List<{title, category, fields}>` | `List<entry_id>` |
| `deleteEntriesBatch` | `List<entry_id>` | void / error |

**Go:**
```go
ids, err := client.CreateEntriesBatch(ctx, "default", []sdk.BatchEntry{
    {Title: "GitHub", Category: "PASSWORD", Fields: map[string]string{"username": "alice"}},
    {Title: "AWS", Category: "PASSWORD", Fields: map[string]string{"username": "bob"}},
})
```

**TypeScript:**
```typescript
const ids = await client.createEntriesBatch([
    { title: "GitHub", category: "PASSWORD", fields: { username: "alice" } },
    { title: "AWS", category: "PASSWORD", fields: { username: "bob" } },
]);
```

### Streaming Events (WebSocket SDKs)

Go, Python, TypeScript, Java, Rust, and C# support live daemon event streaming:

| Event | Description |
|---|---|
| `connected` | WebSocket handshake completed. |
| `disconnected` | Connection lost or closed. |
| `entry_changed` | An entry was created, updated, or deleted. |
| `sync_complete` | A sync cycle finished successfully. |

**Go:**
```go
events, _ := client.SubscribeEvents(ctx)
for evt := range events {
    switch evt.Type {
    case sdk.EventEntryChanged:
        fmt.Println("Entry changed:", evt.EntryID)
    case sdk.EventSyncComplete:
        fmt.Println("Sync done")
    }
}
```

**TypeScript:**
```typescript
for await (const evt of client.events()) {
    if (evt.event === "entry_changed") {
        console.log("Entry changed:", evt.data.entry_id);
    }
}
```

---

## Error Handling

All SDKs expose a typed error class. Every error carries a **machine-readable**
`error_code` and a **human-readable** `message`.

### GrimlockerError

| Field | Type | Description |
|---|---|---|
| `code` | string | Machine-readable error code (see table below). |
| `message` | string | Human-readable description. |
| `status_code` | int | HTTP status code (available on HTTP SDKs; GQL SDKs set this to `0`). |
| `cause` | Exception | The underlying transport exception, if any (language dependent). |

### Error Codes

| Code | HTTP | Meaning |
|---|---|---|
| `INVALID_PAYLOAD` | 400 | Required fields missing or invalid types. |
| `UNAUTHORIZED` | 401 | Token missing, expired, or invalid. |
| `FORBIDDEN` | 403 | Token lacks required scope for the operation. |
| `NOT_FOUND` | 404 | Vault, entry, file, or workspace not found. |
| `CONFLICT` | 409 | Resource already exists or state conflict (e.g. duplicate folder name). |
| `VAULT_LOCKED` | 400 | Operation requires the vault to be unlocked first. |
| `VAULT_ALREADY_UNLOCKED` | 400 | Unlock called on an already-unlocked vault. |
| `INSUFFICIENT_STORAGE` | 413 | File ingest would exceed vault storage quota. |
| `RATE_LIMITED` | 429 | Too many requests; retry after the `Retry-After` header duration. |
| `SYNC_CONFLICT` | 409 | Merge conflict during sync. |
| `BAD_FILE_PATH` | 400 | Path contains invalid characters or escapes vault root. |
| `INTERNAL_ERROR` | 500 | Unexpected daemon error. |

### Per-Language Error Handling Patterns

**Go:**
```go
entries, err := client.ListEntries(ctx, "default")
if err != nil {
    if strings.Contains(err.Error(), "circuit breaker open") {
        log.Println("circuit breaker is open — backing off")
    } else {
        log.Printf("error: %v", err)
    }
}
```

**Python:**
```python
from grimlocker import GrimlockerError, CircuitBreakerOpenError

try:
    entries = client.list_entries()
except CircuitBreakerOpenError:
    print("circuit breaker open — backing off")
except GrimlockerError as e:
    print(f"code={e.error_code} msg={e}")
```

**Java:**
```java
try {
    var entries = client.listEntries().namespace("default").execute();
} catch (GrimlockerException e) {
    System.err.printf("code=%d msg=%s%n", e.getErrorCode(), e.getMessage());
}
```

**TypeScript:**
```typescript
try {
    const entries = await client.listEntries();
} catch (e: any) {
    if (e instanceof CircuitBreakerOpenError) {
        console.error("circuit breaker open — backing off");
    } else {
        console.error(`code=${e.status_code} msg=${e.message}`);
    }
}
```

**Rust:**
```rust
match client.list_entries(None).await {
    Ok(entries) => println!("{}", entries.len()),
    Err(grimlocker_sdk::Error::WebSocket(ref msg)) if msg.contains("circuit breaker") => {
        eprintln!("circuit breaker open");
    }
    Err(e) => eprintln!("error: {e}"),
}
```

**C#:**
```csharp
try {
    var entries = await client.ListEntriesAsync();
} catch (CircuitBreakerOpenException) {
    Console.WriteLine("circuit breaker open — backing off");
} catch (GrimlockerException e) {
    Console.WriteLine($"code={e.Code} msg={e.Message}");
}
```

**C++:**
```cpp
try {
    auto entries = client.list_entries();
} catch (const grimlocker::CircuitBreakerOpenException& e) {
    std::cerr << "circuit breaker open" << std::endl;
} catch (const grimlocker::GrimlockerError& e) {
    std::cerr << "code=" << e.code() << " msg=" << e.what() << std::endl;
}
```

**Ruby:**
```ruby
begin
  entries = client.list_entries
rescue Grimlocker::CircuitBreakerOpenError => e
  puts "circuit breaker open"
rescue Grimlocker::GrimlockerError => e
  puts "code=#{e.code} msg=#{e.message}"
end
```

**PHP:**
```php
try {
    $entries = $client->listEntries();
} catch (\Grimlocker\CircuitBreakerOpenException $e) {
    fprintf(STDERR, "circuit breaker open\n");
} catch (\Grimlocker\GrimlockerException $e) {
    fprintf(STDERR, "code=%d msg=%s\n", $e->getCode(), $e->getMessage());
}
```

**Swift:**
```swift
do {
    let entries = try await client.listEntries()
} catch let error as GrimlockerError {
    if case .circuitBreakerOpen = error {
        print("circuit breaker open")
    } else {
        print("code=\(error.errorDescription ?? "")")
    }
}
```

**Kotlin:**
```kotlin
try {
    val entries = client.listEntries()
} catch (e: CircuitBreakerOpenException) {
    System.err.println("circuit breaker open")
} catch (e: GrimlockerException) {
    System.err.println("code=${e.code} msg=${e.message}")
}
```

**Dart:**
```dart
try {
    final entries = await client.listEntries();
} on CircuitBreakerOpenException catch (e) {
    print('circuit breaker open');
} on GrimlockerException catch (e) {
    print('code=${e.code} msg=${e.message}');
}
```

---

## Security

### Token Management

- Each SDK requires an **X-Grimlocker-Token** (API key) passed at construction time.
- Tokens are generated by the Grimlocker CLI (`grimlocker token create --scope <scope>`) or
  retrieved from the daemon config file (`~/.grimlocker/daemon.toml`).
- Tokens support **scoped permissions**:
  - `vault:read` — list/read entries and files; read audit logs.
  - `vault:write` — create, update, delete entries; ingest files.
  - `vault:admin` — lock/unlock vaults; manage workspaces.
  - `sync:*` — list peers and trigger syncs.
  - `tools:*` — run tool operations (e.g. ssh_keygen).
  - `*:*` — full access (daemon bootstrap token only).
- The token is sent in the `X-Grimlocker-Token` header on every HTTP request or
  as the first WebSocket handshake header in GQL Binary SDKs.
- Tokens are **never logged**. All SDKs redact the token from debug output,
  exception messages, and stack traces.

### Transport Layer Security (TLS)

- By default the daemon listens on `127.0.0.1:9707` (loopback only) — traffic
  never leaves the host.
- For remote daemon access, enable TLS by setting `tls: true` in the client config
  and provide the CA certificate path if using self-signed certs:

  ```go
  client, _ := sdk.NewClient(sdk.Config{
      Addr:   "daemon.example.com:9707",
      Token:  "my-token",
      TLS:    true,
      CAFile: "/path/to/ca.pem",
  })
  ```

- All 12 SDKs support TLS. The configuration property name varies:
  | Language | Config field |
  |---|---|
  | Go | `TLS` (bool), `CAFile` (string) |
  | Python | `tls` (bool), `ca_file` (str) |
  | Java | `tls()` builder, `caFile()` builder |
  | TypeScript | `tls: boolean`, `caFile?: string` |
  | Rust | `.tls(true)`, `.ca_file(path)` |
  | C# | `Tls = true`, `CaFile = "..."` |
  | C++ | `set_tls(true)`, `set_ca_file(path)` |
  | Ruby | `tls: true`, `ca_file: "..."` |
  | PHP | `->setTls(true)`, `->setCaFile('...')` |
  | Swift | `tls: true`, `caFile: "..."` |
  | Kotlin | `.tls(true)`, `.caFile("...")` |
  | Dart | `tls: true`, `caFile: '...'` |

### Symmetric Key Encryption (SKE)

- All data (entries, files) is encrypted on the client side using the vault's
  SKE key before transmission. The daemon never sees plaintext.
- The SKE key is derived from the vault passphrase via **Argon2id** and
  encrypted at rest with the user's master key.
- SDKs handle SKE transparently. No additional configuration is required.

### Injection Immunity

- All GQL Binary SDKs (Go, Python, Java, Rust) use a length-prefixed binary
  protocol with statically defined message schemas — **no query language
  injection surface**.
- HTTP JSON SDKs use structured JSON dispatch. Action names are validated
  against a whitelist; user-supplied values (titles, URLs, notes) are treated
  as opaque blobs and never interpolated into commands or queries.
- File paths are validated server-side against path-traversal attacks
  (e.g. `../../../etc/shadow` is rejected with `BAD_FILE_PATH`).

---

## Testing

Every SDK ships with integration tests that require a running daemon instance.
Unit tests use mocked transports and do not require the daemon.

### Prerequisites for Integration Tests

1. Start the Grimlocker daemon: `grimlocker daemon start`
2. Create a test vault: `grimlocker vault create --name test-vault`
3. Generate a test token: `grimlocker token create --scope '*:*'`
4. Set environment variables (see table below).

### Environment Variables

All SDKs read the same environment variables for integration test configuration:

| Variable | Required | Default | Description |
|---|---|---|---|
| `GRIMLOCKER_HOST` | No | `localhost` | Daemon hostname |
| `GRIMLOCKER_PORT` | No | `9707` | Daemon port |
| `GRIMLOCKER_TOKEN` | **Yes** | — | Authentication token |
| `GRIMLOCKER_VAULT_ID` | **Yes** | — | Test vault UUID |
| `GRIMLOCKER_TLS` | No | `false` | Enable TLS |

### Running Tests

| Language | Command |
|---|---|
| Go | `go test ./sdk/... -tags=integration` |
| Python | `pytest tests/integration/ -v` |
| Java | `mvn verify -Pintegration` |
| TypeScript | `npm run test:integration` |
| Rust | `cargo test --features integration` |
| C# | `dotnet test --filter Category=Integration` |
| C++ | `ctest -C Release -L integration` |
| Ruby | `bundle exec rake test:integration` |
| PHP | `vendor/bin/phpunit --testsuite integration` |
| Swift | `xcodebuild test -scheme GrimlockerIntegrationTests` |
| Kotlin | `./gradlew integrationTest` |
| Dart | `dart test --tags integration` |

### Writing Tests (Quick Example)

**Go:**
```go
func TestEntryCRUD(t *testing.T) {
    client := testutil.NewClient(t)
    defer client.Close()
    vaultID := os.Getenv("GRIMLOCKER_VAULT_ID")

    entry, err := client.Entries.Create(context.Background(), vaultID,
        sdk.EntryCreate{Title: "test", Username: "u", Password: "p"})
    require.NoError(t, err)
    require.Equal(t, "test", entry.Title)

    got, err := client.Entries.Read(context.Background(), vaultID, entry.ID)
    require.NoError(t, err)
    require.Equal(t, "u", got.Username)

    err = client.Entries.Delete(context.Background(), vaultID, entry.ID)
    require.NoError(t, err)
}
```

**Python:**
```python
def test_entry_crud():
    client = grimlocker.Client.connect_from_env()
    vault_id = os.environ["GRIMLOCKER_VAULT_ID"]

    entry = client.entries.create(vault_id, title="test", username="u", password="p")
    assert entry.title == "test"

    got = client.entries.read(vault_id, entry.id)
    assert got.username == "u"

    client.entries.delete(vault_id, entry.id)
```

**TypeScript:**
```typescript
test("entry CRUD", async () => {
    const client = GrimlockerClient.fromEnv();
    const vaultId = process.env.GRIMLOCKER_VAULT_ID!;

    const entry = await client.entries.create(vaultId, { title: "test", username: "u", password: "p" });
    expect(entry.title).toBe("test");

    const got = await client.entries.read(vaultId, entry.id);
    expect(got.username).toBe("u");

    await client.entries.delete(vaultId, entry.id);
});
```

**Rust:**
```rust
#[tokio::test]
#[cfg(feature = "integration")]
async fn test_entry_crud() {
    let mut client = grimlocker_sdk::Client::from_env().await.unwrap();
    let vault_id = std::env::var("GRIMLOCKER_VAULT_ID").unwrap();

    let entry = client.entries()
        .create(&vault_id)
        .title("test").username("u").password("p")
        .send().await.unwrap();
    assert_eq!(entry.title, "test");

    let got = client.entries().read(&vault_id, &entry.id).send().await.unwrap();
    assert_eq!(got.username, "u");

    client.entries().delete(&vault_id, &entry.id).send().await.unwrap();
}
```

### Mocking / In-Memory Backend

All SDKs support a `MockTransport` or `InMemoryClient` for unit tests that
do not require a live daemon:

```go
// Go
mock := sdk.NewMockTransport()
mock.StubVaultStatus("vault-id", sdk.VaultStatus{Unlocked: true})
client := sdk.NewClientWithTransport(mock)
```

```python
# Python
from grimlocker.testing import MockClient

client = MockClient()
client.stub_vault_status("vault-id", unlocked=True)
```

```typescript
// TypeScript
import { MockGrimlockerClient } from "@grimlocker/sdk/testing";

const client = new MockGrimlockerClient();
client.stubVaultStatus("vault-id", { unlocked: true });
```

Mock transports are available in all 12 SDKs and support the full API surface
for deterministic, fast unit testing.
