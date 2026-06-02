# Grimlocker Java SDK

High-level Java client for the Grimlocker vault daemon. Connects via WebSocket using the GQL binary protocol — injection-immune by design.

## Quick Start (5 minutes)

**1. Add to `pom.xml`:**

```xml
<dependency>
  <groupId>com.grimlocker</groupId>
  <artifactId>grimlocker-sdk</artifactId>
  <version>1.0.0</version>
</dependency>
```

**2. Connect and list entries:**

```java
try (GrimlockerClient client = GrimlockerClient.connect("127.0.0.1", 41753, token)) {

    List<Entry> entries = client.listEntries()
        .namespace("default")
        .limit(20)
        .execute();

    for (Entry e : entries) {
        System.out.println(e.title + " [" + e.category + "]");
    }
}
```

**3. Create a password entry:**

```java
Entry created = client.createEntry()
    .namespace("default")
    .title("GitHub")
    .category("PASSWORD")
    .field("username", "alice")
    .field("password", "s3cr3t")
    .field("url", "https://github.com")
    .executeOne();

System.out.println("Created: " + created.id);
```

**4. Get the token:** The daemon prints `GRIMLOCKER_TOKEN=<token>` to stdout on startup. Read it from there or use the Tauri IPC `get_session_token` command.

## Operations

| Method | Description |
|---|---|
| `client.listEntries()` | List all entries (paginated) |
| `client.getEntry(id)` | Fetch one entry by ID |
| `client.queryEntries(category)` | Filter by category (PASSWORD, SSH_KEY, …) |
| `client.createEntry()` | Create a new entry |
| `client.updateEntry(id)` | Update title/fields of an existing entry |
| `client.deleteEntry(ns, id)` | Delete an entry |

## Builder Methods

All query builders support:

| Method | Default | Description |
|---|---|---|
| `.namespace("x")` | `"default"` | Target workspace namespace |
| `.limit(n)` | `50` | Max results |
| `.offset(n)` | `0` | Pagination offset |
| `.category("x")` | `""` | Category filter |
| `.field("k","v")` | — | Add a single field |
| `.fields(map)` | — | Set all fields at once |
| `.execute()` | — | Run and return `List<Entry>` |
| `.executeOne()` | — | Run and return first `Entry` or `null` |

## Error Handling

All errors throw `GrimlockerException` (unchecked). It carries `getErrorCode()` for programmatic handling:

```java
try {
    Entry e = client.getEntry("missing-id").namespace("default").executeOne();
} catch (GrimlockerException ex) {
    if (ex.getErrorCode() == -11) {
        System.out.println("Entry not found");
    }
}
```

## Security Notes

- The binary wire protocol is identical to the Go and Python SDKs.
- No SQL, no JSON injection — every field is length-prefixed binary.
- TLS/mTLS is enforced in Enterprise mode; use `wss://` endpoints for production.
- Never log the session token.
