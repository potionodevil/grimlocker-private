# Grimlocker SDK Guide

Grimlocker provides native SDKs for **12 languages**, all communicating
with the Grimlocker local daemon. Every SDK abstracts the wire protocol
(HTTP JSON or GQL Binary over WebSocket) behind a typed, idiomatic API.

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

import "github.com/grimlocker/grimdb/sdk"

func main() {
    client, _ := sdk.NewClient(sdk.Config{Addr: "localhost:9707", Token: "my-token"})
    defer client.Close()
    entries, _ := client.Entries.List(context.Background(), "vault-id", sdk.ListOpts{Limit: 10})
    fmt.Println(entries.Total)
}
```

### Python

```python
from grimlocker import Client

client = Client("localhost:9707", token="my-token")
entries = client.entries.list("vault-id", limit=10)
print(entries.total)
client.close()
```

### Java

```java
import com.grimlocker.sdk.*;

var client = new GrimlockerClient("localhost", 9707, "my-token");
var result = client.entries().list("vault-id", ListOpts.builder().limit(10).build());
System.out.println(result.total());
client.close();
```

### TypeScript

```typescript
import { GrimlockerClient } from "@grimlocker/sdk";

const client = new GrimlockerClient({ host: "localhost", port: 9707, token: "my-token" });
const entries = await client.entries.list("vault-id", { limit: 10 });
console.log(entries.total);
client.close();
```

### Rust

```rust
use grimlocker_sdk::Client;

#[tokio::main]
async fn main() -> Result<(), Box<dyn std::error::Error>> {
    let mut client = Client::connect("localhost:9707", "my-token").await?;
    let entries = client.entries().list("vault-id").limit(10).send().await?;
    println!("{}", entries.total);
    Ok(())
}
```

### C# / .NET

```csharp
using Grimlocker.SDK;

var client = new GrimlockerClient("localhost", 9707, "my-token");
var entries = await client.Entries.ListAsync("vault-id", new ListOptions { Limit = 10 });
Console.WriteLine(entries.Total);
client.Dispose();
```

### C++

```cpp
#include <grimlocker/client.hpp>

int main() {
    auto client = grimlocker::Client("localhost", 9707, "my-token");
    client.connect();
    auto entries = client.entries().list("vault-id", grimlocker::ListOpts{10, 0});
    std::cout << entries.total << std::endl;
    client.close();
}
```

### Ruby

```ruby
require "grimlocker"

client = Grimlocker::Client.new(host: "localhost", port: 9707, token: "my-token")
entries = client.entries.list("vault-id", limit: 10)
puts entries.total
client.close
```

### PHP

```php
<?php
require 'vendor/autoload.php';

$client = new Grimlocker\Client('localhost', 9707, 'my-token');
$entries = $client->entries()->list('vault-id', ['limit' => 10]);
echo $entries->total;
$client->close();
```

### Swift

```swift
import GrimlockerSDK

let client = GrimlockerClient(host: "localhost", port: 9707, token: "my-token")
let entries = try await client.entries.list(vaultId: "vault-id", limit: 10)
print(entries.total)
client.close()
```

### Kotlin

```kotlin
import com.grimlocker.sdk.*

val client = GrimlockerClient("localhost", 9707, "my-token")
val entries = client.entries.list("vault-id", ListOpts(limit = 10))
println(entries.total)
client.close()
```

### Dart

```dart
import 'package:grimlocker_sdk/grimlocker_sdk.dart';

void main() async {
  final client = GrimlockerClient('localhost', 9707, token: 'my-token');
  final entries = await client.entries.list('vault-id', limit: 10);
  print(entries.total);
  client.close();
}
```

---

## Full API Surface

Every SDK exposes the same logical operations divided into **namespaces**.

### Client Lifecycle

| Operation | Description |
|---|---|
| `connect()` / constructor | Open connection to daemon. For WebSocket SDKs this performs the WebSocket handshake. For HTTP SDKs the connection is lazy (established on first request). |
| `close()` / `dispose()` | Gracefully close the connection and release resources. |
| `ping()` | Health-check. Returns latency in milliseconds. Available in all SDKs. |
| `version()` | Returns daemon version string and protocol version. |

### Vault

| Operation | Parameters | Returns | Description |
|---|---|---|---|
| `vault.unlock` | `vault_id`, `passphrase` | `{}` | Unlock a vault for the session. |
| `vault.lock` | `vault_id` | `{}` | Lock a vault, clearing decrypted keys from memory. |
| `vault.status` | `vault_id` | `VaultStatus` | Get vault metadata: locked state, entry/file counts, storage usage. |
| `vault.recovery_phrase` | `vault_id` | `RecoveryPhrase` | Retrieve the BIP39 recovery mnemonic (requires elevated token scope). |

### Entry

| Operation | Parameters | Returns | Description |
|---|---|---|---|
| `entry.list` | `vault_id`, `limit`, `offset` | `EntryListResult` | Paginated list of entry summaries (excludes secrets). |
| `entry.read` | `vault_id`, `entry_id` | `Entry` | Read full entry including decrypted password. |
| `entry.create` | `vault_id`, `title`, `url?`, `username?`, `password?`, `tags?`, `notes?` | `Entry` | Create a new entry. |
| `entry.update` | `vault_id`, `entry_id`, + optional fields | `Entry` | Update one or more fields of an existing entry. Unset fields are left unchanged. |
| `entry.delete` | `vault_id`, `entry_id` | `{}` | Permanently delete an entry. |
| `entry.query` | `vault_id`, `filter`, `limit`, `offset` | `EntryQueryResult` | Search entries with structured field/operator/value filters. |
| `entry.search` | `vault_id`, `query`, `limit` | `EntrySearchResult` | Full-text search across all entry fields. |

### File

| Operation | Parameters | Returns | Description |
|---|---|---|---|
| `file.list_folder` | `vault_id`, `path` | `FileListResult` | List files and subfolders at a path. |
| `file.create_folder` | `vault_id`, `path` | `{}` | Create a new folder (and any missing ancestors). |
| `file.rename_folder` | `vault_id`, `path`, `new_name` | `{}` | Rename a folder in-place. |
| `file.delete_folder` | `vault_id`, `path`, `recursive?` | `{}` | Delete a folder. Set `recursive: true` to delete non-empty folders. |
| `file.move` | `vault_id`, `source`, `destination` | `{}` | Move or rename a file or folder. |
| `file.ingest` | `vault_id`, `path`, `content` (bytes), `mime_type?` | `{}` | Upload (encrypt and store) a file into the vault. |
| `file.download` | `vault_id`, `path` | `FileDownloadResult` | Download a file (returns decrypted bytes). |

> **Note:** For HTTP JSON SDKs, `content` in ingest/download is Base64-encoded
> in the JSON body. GQL Binary SDKs transmit raw bytes directly over the
> WebSocket message frame.

### Workspace

| Operation | Parameters | Returns | Description |
|---|---|---|---|
| `workspace.list` | _none_ | `WorkspaceListResult` | List all configured workspaces. |
| `workspace.create` | `name`, `vault_id` | `Workspace` | Create a new workspace linked to a vault. |
| `workspace.switch` | `workspace_id` | `{}` | Set the active workspace for the current session. |
| `workspace.rename` | `workspace_id`, `name` | `{}` | Rename a workspace. |
| `workspace.delete` | `workspace_id` | `{}` | Delete a workspace (does not delete the underlying vault). |

### Sync

| Operation | Parameters | Returns | Description |
|---|---|---|---|
| `sync.list_peers` | _none_ | `SyncPeerListResult` | List registered sync peers and their online status. |
| `sync.trigger` | `peer_id` | `SyncTriggerResult` | Trigger an immediate sync with a peer. |

### Audit

| Operation | Parameters | Returns | Description |
|---|---|---|---|
| `audit.list` | `vault_id`, `limit`, `offset` | `AuditListResult` | Paginated audit trail for a vault. |

### Tools

| Operation | Parameters | Returns | Description |
|---|---|---|---|
| `tool.ssh_keygen` | `algorithm`, `bits?`, `comment?`, `passphrase?` | `SSHKeygenResult` | Generate an SSH key pair. The private key never leaves the SDK; only the public key and fingerprint are returned. |

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
entries, err := client.Entries.List(ctx, "vault-id", sdk.ListOpts{})
if err != nil {
    var ge *sdk.GrimlockerError
    if errors.As(err, &ge) {
        fmt.Printf("code=%s msg=%s\n", ge.Code, ge.Message)
    }
}
```

**Python:**
```python
from grimlocker import GrimlockerError

try:
    entries = client.entries.list("vault-id")
except GrimlockerError as e:
    print(f"code={e.code} msg={e.message}")
```

**Java:**
```java
try {
    var result = client.entries().list("vault-id", opts);
} catch (GrimlockerException e) {
    System.err.printf("code=%s msg=%s%n", e.getCode(), e.getMessage());
}
```

**TypeScript:**
```typescript
try {
    const entries = await client.entries.list("vault-id");
} catch (e) {
    if (e instanceof GrimlockerError) {
        console.error(`code=${e.code} msg=${e.message}`);
    }
}
```

**Rust:**
```rust
match client.entries().list("vault-id").send().await {
    Ok(entries) => println!("{}", entries.total),
    Err(GrimlockerError { code, message, .. }) => eprintln!("{code}: {message}"),
    Err(e) => eprintln!("transport error: {e}"),
}
```

**C#:**
```csharp
try {
    var entries = await client.Entries.ListAsync("vault-id");
} catch (GrimlockerException e) {
    Console.WriteLine($"code={e.Code} msg={e.Message}");
}
```

**C++:**
```cpp
try {
    auto entries = client.entries().list("vault-id");
} catch (const grimlocker::Error& e) {
    std::cerr << "code=" << e.code() << " msg=" << e.what() << std::endl;
}
```

**Ruby:**
```ruby
begin
  entries = client.entries.list("vault-id")
rescue Grimlocker::Error => e
  puts "code=#{e.code} msg=#{e.message}"
end
```

**PHP:**
```php
try {
    $entries = $client->entries()->list('vault-id');
} catch (\Grimlocker\Error $e) {
    fprintf(STDERR, "code=%s msg=%s\n", $e->getCode(), $e->getMessage());
}
```

**Swift:**
```swift
do {
    let entries = try await client.entries.list(vaultId: "vault-id")
} catch let error as GrimlockerError {
    print("code=\(error.code) msg=\(error.message)")
}
```

**Kotlin:**
```kotlin
try {
    val entries = client.entries.list("vault-id")
} catch (e: GrimlockerException) {
    System.err.println("code=${e.code} msg=${e.message}")
}
```

**Dart:**
```dart
try {
    final entries = await client.entries.list('vault-id');
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
