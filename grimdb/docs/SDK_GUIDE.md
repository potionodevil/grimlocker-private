# Grimlocker SDK Guide

Get up and running with the Grimlocker vault in 5 minutes. This guide covers the high-level SDK for **Go**, **Java**, and **Python**.

> **Low-level access:** If you need direct control over GQL binary frames (custom clients, protocol testing, embedded systems), see [GQL_CLIENT_GUIDE.md](GQL_CLIENT_GUIDE.md) instead.

---

## How it works

```
Your App
  └─ SDK (Go / Java / Python)
       └─ GQL binary frames (injection-immune)
            └─ WebSocket  →  Grimlocker Daemon
                                  └─ Kernel / Storage / Crypto
```

The SDK encodes every query as a binary frame — no string interpolation, no SQL, no JSON injection possible. The security stack is transparent to your code.

---

## Getting the session token

The daemon prints the token to stdout on startup:

```
GRIMLOCKER_TOKEN=eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...
GRIMLOCKER_IPC=ws://127.0.0.1:41753/ws
```

Read it from the process output, or call the Tauri IPC command `get_session_token` from the desktop app.

---

## Go SDK

### Install

The SDK is in the same module as the daemon:

```go
import "github.com/grimlocker/grimdb/sdk"
```

### Connect

```go
client, err := sdk.DialGQL(ctx, "ws://127.0.0.1:41753/ws?token="+token)
if err != nil { log.Fatal(err) }
defer client.Close()
```

### List entries

```go
entries, err := client.ListEntries(ctx, "default")
for _, e := range entries {
    fmt.Printf("%s [%s]\n", e.Title, e.Category)
}
```

### Password entries

```go
// Create
id, err := client.CreatePassword(ctx, "default", &sdk.PasswordEntry{
    Title:    "GitHub",
    Username: "alice",
    Password: "s3cr3t",
    URL:      "https://github.com",
})

// List
passwords, err := client.ListPasswords(ctx, "default")

// Get one
p, err := client.GetPassword(ctx, "default", id)
fmt.Println(p.Username)
```

### SSH key entries

```go
id, err := client.CreateSSHKey(ctx, "default", &sdk.SSHKeyEntry{
    Title:     "Dev Server",
    PublicKey: "ssh-ed25519 AAAAC3Nz... alice@dev",
    Algorithm: "ed25519",
})

keys, err := client.ListSSHKeys(ctx, "default")
```

### Generic operations

```go
// Get by ID
entry, err := client.GetEntry(ctx, "default", entryID)

// Create generic
entry, err := client.CreateEntry(ctx, "default", "My Secret", "NOTE", map[string]string{
    "body": "top secret text",
})

// Update
err = client.UpdateEntry(ctx, "default", entryID, "New Title", map[string]string{
    "body": "updated text",
})

// Delete
err = client.DeleteEntry(ctx, "default", entryID)
```

### Error handling

```go
entries, err := client.ListEntries(ctx, "default")
if err != nil {
    // err message includes: "ENTRY_NOT_FOUND (-11): ..."
    log.Printf("GQL error: %v", err)
}
```

---

## Java SDK

### Maven dependency

```xml
<dependency>
  <groupId>com.grimlocker</groupId>
  <artifactId>grimlocker-sdk</artifactId>
  <version>1.0.0</version>
</dependency>
```

### Connect and list (3 lines)

```java
try (GrimlockerClient client = GrimlockerClient.connect("127.0.0.1", 41753, token)) {
    List<Entry> entries = client.listEntries().namespace("default").limit(20).execute();
    entries.forEach(e -> System.out.println(e.title + " [" + e.category + "]"));
}
```

### Create a password

```java
Entry created = client.createEntry()
    .namespace("default")
    .title("GitHub")
    .category("PASSWORD")
    .field("username", "alice")
    .field("password", "s3cr3t")
    .field("url", "https://github.com")
    .executeOne();

System.out.println("ID: " + created.id);
```

### Query by category

```java
List<Entry> passwords = client.queryEntries("PASSWORD")
    .namespace("default")
    .execute();
```

### Error handling

```java
try {
    Entry e = client.getEntry("missing-id").namespace("default").executeOne();
} catch (GrimlockerException ex) {
    // ex.getMessage(): "ENTRY_NOT_FOUND (-11): entry not found"
    // ex.getErrorCode(): -11
}
```

Full Java documentation: [java/README.md](../sdk/java/README.md)

---

## Python SDK

### Install

```bash
pip install grimlocker
# or from source:
pip install -e grimdb/sdk/python
```

### Connect and list (4 lines)

```python
from grimlocker import Client

with Client.connect("127.0.0.1", 41753, token) as client:
    entries = client.list_entries(namespace="default", limit=20)
    for e in entries:
        print(f"{e.title} [{e.category}]")
```

### Create a password

```python
from grimlocker import Client, PasswordEntry

with Client.connect("127.0.0.1", 41753, token) as client:
    entry_id = client.create_password(PasswordEntry(
        title="GitHub",
        username="alice",
        password="s3cr3t",
        url="https://github.com",
    ))
```

### SSH keys

```python
from grimlocker import Client, SSHKeyEntry

with Client.connect("127.0.0.1", 41753, token) as client:
    key_id = client.create_ssh_key(SSHKeyEntry(
        title="Dev Server",
        public_key="ssh-ed25519 AAAAC3Nz...",
        algorithm="ed25519",
    ))
    keys = client.list_ssh_keys()
```

### Error handling

```python
from grimlocker import Client, GrimlockerError

with Client.connect("127.0.0.1", 41753, token) as client:
    try:
        entry = client.get_entry("missing-id")
    except GrimlockerError as e:
        if e.error_code == -11:
            print("Not found")
```

Full Python documentation: [python/README.md](../sdk/python/README.md)

---

## Enterprise (mTLS)

For Enterprise deployments, the daemon requires mutual TLS. Pass `wss://` URIs and configure client certificates:

```go
// Go — custom TLS dialer
tlsConfig := &tls.Config{
    Certificates: []tls.Certificate{clientCert},
    RootCAs:      caCertPool,
}
dialer := websocket.Dialer{TLSClientConfig: tlsConfig}
conn, _, err := dialer.DialContext(ctx, "wss://vault.corp.example:9443/ws", nil)
client := sdk.NewGQLClientFromConn(conn)
```

```java
// Java — pass TLS config to GrimlockerClient.connectTLS(...)
GrimlockerClient client = GrimlockerClient.connectTLS("vault.corp.example", 9443, token, tlsConfig);
```

```python
# Python — pass ssl_context
import ssl
ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
ctx.load_cert_chain("client.crt", "client.key")
ctx.load_verify_locations("ca.crt")
client = Client.connect("vault.corp.example", 9443, token, ssl_context=ctx)
```

---

## Error code reference

| Code | Name | Meaning |
|---|---|---|
| `-1` | `BUS_TIMEOUT` | Kernel event bus did not respond in 30s |
| `-2` | `INVALID_STORAGE_RESPONSE` | Storage layer returned unparseable data |
| `-3` | `STORAGE_ERROR` | Storage layer returned an error |
| `-10` | `MISSING_ENTRY_ID` | `entry_id` required but not provided |
| `-11` | `ENTRY_NOT_FOUND` | No entry with the given ID exists |
| `-20` | `CATEGORY_QUERY_FAILED` | Category filter query failed |
| `-30` | `CREATE_FAILED` | Entry creation failed |
| `-31` | `UPDATE_FAILED` | Entry update failed |
| `-32` | `DELETE_FAILED` | Entry deletion failed |
| `-100` | `DISPATCHER_UNAVAILABLE` | GQL dispatcher not running |
| `-101` | `INVALID_FRAME` | Binary frame malformed |
| `-102` | `SCHEMA_VALIDATION` | Query failed validator |
| `-103` | `ACL_DENIED` | Namespace access denied |
| `-104` | `NOT_A_QUERY_FRAME` | Wrong opcode |
| `-105` | `DISPATCH_ERROR` | Dispatcher internal error |
