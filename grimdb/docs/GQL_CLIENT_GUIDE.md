# GrimQueryLanguage (GQL) Client Developer Guide

GQL is a binary-only query protocol with **Total Injection Immunity** — no text parsing
occurs at any point. Every field is length-prefixed binary, validated through a two-stage
syntactic + semantic validator before reaching the dispatcher.

> **For most use cases, use the high-level SDK instead of this guide.**
> The SDK handles frame encoding, connection management, and error translation automatically.
> See [SDK_GUIDE.md](SDK_GUIDE.md) for Go, Java, and Python quick-starts.
>
> This guide is for **power users** who need direct binary-frame access: custom client
> implementations, protocol testing, embedded systems, or audit work.

This guide covers the raw wire protocol for **Java** and **Python** for both the
Single-User tier (token-based WebSocket) and the Enterprise tier (mutual TLS WebSocket).

---

## Table of Contents

1. Connection Overview
2. Single-User Connection (Token)
3. Enterprise Connection (mTLS)
4. Frame Format
5. Java Client — Single-User
6. Java Client — Enterprise (mTLS)
7. Python Client — Single-User
8. Python Client — Enterprise (mTLS)
9. Error Codes
10. Security Guidelines

---

## 1. Connection Overview

```
Client              Grimlocker Daemon
  |                       |
  |--- WebSocket open ---> |  (ws:// single-user, wss:// enterprise)
  |<-- INIT.READY (0x2C) --|
  |--- AUTH.TOKEN (0x2D) ->|  (single-user: token in URL query param)
  |<-- KERNEL.STATE (0x2E)-|
  |--- GQL frame (0x3D) -->|  (binary frame, see §4)
  |<-- GQL result (0x3E) --|
```

Grimlocker uses a binary frame protocol on top of WebSocket binary messages.
Each message: `4-byte big-endian length` + `1-byte type` + `N-byte payload`.

GQL messages use types `0x3D` (query) and `0x3E` (result).

---

## 2. Single-User Connection (Token Auth)

```
ws://127.0.0.1:{PORT}/ws?token={TOKEN}
```

The token and port are written to the daemon's stdout on startup:
```
GRIMLOCKER_TOKEN=gqiHnn4Fek98GIz_6WbEpthRMbzy-XEEemuVC3lz8OA=
GRIMLOCKER_IPC=ws://127.0.0.1:41753/ws
```

Tauri reads these and makes them available via the `get_session_token` command.
External Java/Python clients can read them from the daemon stdout if spawning the daemon
directly, or from a shared config file written by the Tauri layer.

---

## 3. Enterprise Connection (mTLS)

```
wss://{server}:9443/ws
```

Mutual TLS is mandatory. The client must present a client certificate signed by the
enterprise CA. The server certificate must also be verified (or pinned via SPKI hash).

After the WebSocket handshake, the enterprise login flow uses IPC messages before GQL:

```
Client                                 Enterprise Daemon
  |--- WS upgrade (mTLS) ------------>|
  |<-- INIT.READY (0x2C) ------------|
  |--- {username, password} (0x10) -->|
  |<-- unlock result (0x13) ----------|  or passphrase challenge
  |--- GQL frame (0x3D) ------------->|  (now authenticated)
  |<-- GQL result (0x3E) -------------|
```

---

## 4. Frame Format

### Outer Wire Frame (all Grimlocker messages)

```
Bytes 0-3 : Total payload length (uint32, big-endian) = 1 + len(gql_frame)
Byte 4    : Message type = 0x3D (GQL query) or 0x3E (GQL result)
Bytes 5+  : GQL frame payload (see below)
```

### GQL Frame (inner, bytes 5+)

```
Byte 0    : Version       (uint8) — always 1
Byte 1    : Opcode        (uint8) — 0x01 Query, 0x02 Mutate
Bytes 2-3 : Flags         (uint16, big-endian) — 0x0000 normally
Bytes 4-7 : PayloadSize   (uint32, big-endian)
Bytes 8+  : Payload       (binary-encoded GQLQuery, see below)
```

### GQLQuery Payload Encoding

```
[0:1]   field_count    uint8   (number of entries in Fields map)
[1:3]   operation_len  uint16  (e.g. "list_entries" = 12)
[3:n]   operation      bytes
[n:n+2] namespace_len  uint16  (your user/workspace ID)
[...]   namespace       bytes
[...]   entry_id_len   uint16  (0 if not needed)
[...]   entry_id        bytes
[...]   category_len   uint16  (e.g. "PASSWORD")
[...]   category        bytes
[...]   title_len       uint16
[...]   title           bytes
[...]   fields...       each: key_len(2) + key + val_len(2) + val
[...]   limit           uint32 (0 = default 50)
[...]   offset          uint32
[...]   credentials_len uint16 (0 if not needed)
[...]   credentials      bytes (SKE-encrypted handle for write ops)
```

All multi-byte integers are **big-endian**.

### GQLResult (JSON, returned as inner payload of 0x3E frame)

```json
{
  "success":     true,
  "entries":     [ { "id": "...", "category": "PASSWORD", "title": "...", "fields": {} } ],
  "total_count": 1,
  "error_code":  0,
  "error_msg":   ""
}
```

---

## 5. Java Client — Single-User

### Maven Dependencies

```xml
<dependency>
    <groupId>com.squareup.okhttp3</groupId>
    <artifactId>okhttp</artifactId>
    <version>4.12.0</version>
</dependency>
<dependency>
    <groupId>com.google.code.gson</groupId>
    <artifactId>gson</artifactId>
    <version>2.10.1</version>
</dependency>
```

### GQLFrame.java

```java
import java.nio.ByteBuffer;
import java.nio.ByteOrder;
import java.nio.charset.StandardCharsets;
import java.util.Map;

public class GQLFrame {

    public static final byte OPCODE_QUERY  = 0x01;
    public static final byte OPCODE_MUTATE = 0x02;
    public static final byte MSG_GQL_QUERY = 0x3D;

    public static byte[] encodeGQLQuery(
        String operation,
        String namespace,
        String entryId,
        String category,
        String title,
        Map<String, String> fields,
        int limit,
        int offset
    ) {
        byte[] opBytes  = operation.getBytes(StandardCharsets.UTF_8);
        byte[] nsBytes  = namespace.getBytes(StandardCharsets.UTF_8);
        byte[] idBytes  = (entryId  != null ? entryId  : "").getBytes(StandardCharsets.UTF_8);
        byte[] catBytes = (category != null ? category : "").getBytes(StandardCharsets.UTF_8);
        byte[] ttlBytes = (title    != null ? title    : "").getBytes(StandardCharsets.UTF_8);

        int size = 1                  // field_count
            + 2 + opBytes.length
            + 2 + nsBytes.length
            + 2 + idBytes.length
            + 2 + catBytes.length
            + 2 + ttlBytes.length;

        byte[][] fk = new byte[fields != null ? fields.size() : 0][];
        byte[][] fv = new byte[fk.length][];
        if (fields != null) {
            int i = 0;
            for (Map.Entry<String, String> e : fields.entrySet()) {
                fk[i] = e.getKey().getBytes(StandardCharsets.UTF_8);
                fv[i] = e.getValue().getBytes(StandardCharsets.UTF_8);
                size += 2 + fk[i].length + 2 + fv[i].length;
                i++;
            }
        }
        size += 4 + 4;   // limit + offset
        size += 2;        // credentials_len (0)

        // GQL frame = 8-byte header + payload
        byte[] payload = new byte[size];
        ByteBuffer pb = ByteBuffer.wrap(payload).order(ByteOrder.BIG_ENDIAN);
        pb.put((byte) (fk.length));
        putField(pb, opBytes);
        putField(pb, nsBytes);
        putField(pb, idBytes);
        putField(pb, catBytes);
        putField(pb, ttlBytes);
        for (int i = 0; i < fk.length; i++) {
            putField(pb, fk[i]);
            putField(pb, fv[i]);
        }
        pb.putInt(limit);
        pb.putInt(offset);
        pb.putShort((short) 0); // no credentials

        // Build full GQL frame (8-byte header + payload)
        byte opcode = "list_entries".equals(operation) || "get_entry".equals(operation)
            || "query_entries".equals(operation) ? OPCODE_QUERY : OPCODE_MUTATE;
        ByteBuffer frame = ByteBuffer.allocate(8 + payload.length).order(ByteOrder.BIG_ENDIAN);
        frame.put((byte) 1);           // Version
        frame.put(opcode);             // Opcode
        frame.putShort((short) 0);     // Flags
        frame.putInt(payload.length);  // PayloadSize
        frame.put(payload);

        // Wrap in outer wire frame: 4-byte len + 1-byte type + gql frame
        byte[] gqlFrame = frame.array();
        int totalLen = 1 + gqlFrame.length;
        ByteBuffer wire = ByteBuffer.allocate(4 + totalLen).order(ByteOrder.BIG_ENDIAN);
        wire.putInt(totalLen);
        wire.put(MSG_GQL_QUERY);
        wire.put(gqlFrame);
        return wire.array();
    }

    private static void putField(ByteBuffer buf, byte[] data) {
        buf.putShort((short) data.length);
        buf.put(data);
    }
}
```

### GQLClient.java

```java
import com.google.gson.*;
import okhttp3.*;
import java.util.*;
import java.util.concurrent.*;

public class GQLClient {

    private final String wsUrl;
    private final OkHttpClient http;
    private WebSocket ws;
    private final BlockingQueue<String> results = new LinkedBlockingQueue<>();

    public GQLClient(String host, int port, String token) {
        this.wsUrl = "ws://" + host + ":" + port + "/ws?token=" + token;
        this.http  = new OkHttpClient.Builder()
            .pingInterval(30, TimeUnit.SECONDS)
            .build();
    }

    public void connect() throws InterruptedException {
        Request req = new Request.Builder().url(wsUrl).build();
        CountDownLatch ready = new CountDownLatch(1);
        ws = http.newWebSocket(req, new WebSocketListener() {
            @Override
            public void onOpen(WebSocket ws, Response r) { ready.countDown(); }
            @Override
            public void onMessage(WebSocket ws, okio.ByteString bytes) {
                byte[] data = bytes.toByteArray();
                if (data.length >= 5 && data[4] == 0x3E) { // MSG_GQL_RESULT
                    // Strip 8-byte GQL header from inner payload
                    int innerOff = 5 + 8;
                    if (data.length > innerOff) {
                        results.offer(new String(data, innerOff, data.length - innerOff));
                    }
                }
            }
        });
        ready.await(5, TimeUnit.SECONDS);
    }

    public List<Map<String, Object>> listEntries(String namespace) throws Exception {
        byte[] frame = GQLFrame.encodeGQLQuery(
            "list_entries", namespace, null, null, null, null, 50, 0);
        ws.send(okio.ByteString.of(frame));
        String json = results.poll(10, TimeUnit.SECONDS);
        return parseEntries(json);
    }

    public List<Map<String, Object>> queryByCategory(String namespace, String category) throws Exception {
        byte[] frame = GQLFrame.encodeGQLQuery(
            "query_entries", namespace, null, category, null, null, 50, 0);
        ws.send(okio.ByteString.of(frame));
        String json = results.poll(10, TimeUnit.SECONDS);
        return parseEntries(json);
    }

    public String createEntry(String namespace, String title, String category,
                              Map<String, String> fields) throws Exception {
        byte[] frame = GQLFrame.encodeGQLQuery(
            "create_entry", namespace, null, category, title, fields, 0, 0);
        ws.send(okio.ByteString.of(frame));
        String json = results.poll(10, TimeUnit.SECONDS);
        JsonObject obj = JsonParser.parseString(json).getAsJsonObject();
        JsonArray entries = obj.getAsJsonArray("entries");
        if (entries != null && entries.size() > 0) {
            return entries.get(0).getAsJsonObject().get("id").getAsString();
        }
        throw new Exception("Create failed: " + obj.get("error_msg").getAsString());
    }

    public void deleteEntry(String namespace, String entryId) throws Exception {
        byte[] frame = GQLFrame.encodeGQLQuery(
            "delete_entry", namespace, entryId, null, null, null, 0, 0);
        ws.send(okio.ByteString.of(frame));
        String json = results.poll(10, TimeUnit.SECONDS);
        JsonObject obj = JsonParser.parseString(json).getAsJsonObject();
        if (!obj.get("success").getAsBoolean()) {
            throw new Exception("Delete failed: " + obj.get("error_msg").getAsString());
        }
    }

    private List<Map<String, Object>> parseEntries(String json) {
        if (json == null) throw new RuntimeException("Timeout waiting for GQL result");
        JsonObject obj = JsonParser.parseString(json).getAsJsonObject();
        List<Map<String, Object>> result = new ArrayList<>();
        JsonArray entries = obj.getAsJsonArray("entries");
        if (entries != null) {
            Gson gson = new Gson();
            for (JsonElement e : entries) {
                result.add(gson.fromJson(e, Map.class));
            }
        }
        return result;
    }

    public void disconnect() { http.dispatcher().executorService().shutdown(); }
}
```

### Usage Example (Single-User)

```java
public class Main {
    public static void main(String[] args) throws Exception {
        // Read from daemon stdout or config file
        String token = System.getenv("GRIMLOCKER_TOKEN");
        int    port  = Integer.parseInt(System.getenv("GRIMLOCKER_PORT"));

        GQLClient client = new GQLClient("127.0.0.1", port, token);
        client.connect();

        // List all entries
        List<Map<String, Object>> entries = client.listEntries("default");
        System.out.println("Found " + entries.size() + " entries");

        // List passwords only
        List<Map<String, Object>> passwords = client.queryByCategory("default", "PASSWORD");
        for (Map<String, Object> p : passwords) {
            System.out.println("  - " + p.get("title"));
        }

        // Create an entry
        Map<String, String> fields = Map.of("username", "alice", "url", "https://example.com");
        String id = client.createEntry("default", "Example Login", "PASSWORD", fields);
        System.out.println("Created: " + id);

        client.disconnect();
    }
}
```

---

## 6. Java Client — Enterprise (mTLS)

```java
import javax.net.ssl.*;
import java.io.*;
import java.security.*;

public class GQLEnterpriseClient extends GQLClient {

    public static OkHttpClient buildMTLSClient(
        String clientCertPath,
        String clientKeyPath,
        String caCertPath,
        String keystorePassword
    ) throws Exception {
        // Load client certificate + private key into KeyStore
        KeyStore ks = KeyStore.getInstance("PKCS12");
        try (InputStream in = new FileInputStream(clientCertPath)) {
            ks.load(in, keystorePassword.toCharArray());
        }

        KeyManagerFactory kmf = KeyManagerFactory.getInstance(KeyManagerFactory.getDefaultAlgorithm());
        kmf.init(ks, keystorePassword.toCharArray());

        // Load CA certificate into TrustStore
        KeyStore ts = KeyStore.getInstance(KeyStore.getDefaultType());
        ts.load(null);
        CertificateFactory cf = CertificateFactory.getInstance("X.509");
        try (InputStream in = new FileInputStream(caCertPath)) {
            ts.setCertificateEntry("ca", cf.generateCertificate(in));
        }
        TrustManagerFactory tmf = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm());
        tmf.init(ts);

        SSLContext sslCtx = SSLContext.getInstance("TLS");
        sslCtx.init(kmf.getKeyManagers(), tmf.getTrustManagers(), null);

        return new OkHttpClient.Builder()
            .sslSocketFactory(sslCtx.getSocketFactory(),
                (X509TrustManager) tmf.getTrustManagers()[0])
            .build();
    }

    // After connecting, send enterprise login before GQL
    public void enterpriseLogin(String username, String password) throws Exception {
        // Send MsgUnlockVault (0x10) with username:password payload
        // Handle passphrase challenge if returned
        // See enterprise login flow in §3
    }
}
```

---

## 7. Python Client — Single-User

```python
pip install websocket-client
```

### grimlocker_client.py

```python
import struct
import json
import threading
import queue
import websocket


MSG_GQL_QUERY  = 0x3D
MSG_GQL_RESULT = 0x3E

GQL_VERSION    = 1
OPCODE_QUERY   = 0x01
OPCODE_MUTATE  = 0x02

READ_OPS  = {"list_entries", "get_entry", "query_entries"}
WRITE_OPS = {"create_entry", "update_entry", "delete_entry"}


def encode_field(s: str) -> bytes:
    b = s.encode("utf-8")
    return struct.pack(">H", len(b)) + b


def build_gql_payload(operation: str, namespace: str, entry_id: str = "",
                      category: str = "", title: str = "",
                      fields: dict = None, limit: int = 50, offset: int = 0) -> bytes:
    fields = fields or {}
    payload = bytes([len(fields)])
    payload += encode_field(operation)
    payload += encode_field(namespace)
    payload += encode_field(entry_id or "")
    payload += encode_field(category or "")
    payload += encode_field(title or "")
    for k, v in fields.items():
        payload += encode_field(k) + encode_field(v)
    payload += struct.pack(">II", limit, offset)
    payload += struct.pack(">H", 0)  # credentials_len = 0
    return payload


def build_gql_frame(operation: str, **kwargs) -> bytes:
    payload = build_gql_payload(operation, **kwargs)
    opcode = OPCODE_QUERY if operation in READ_OPS else OPCODE_MUTATE
    # GQL frame: version(1) + opcode(1) + flags(2) + payload_size(4) + payload
    gql_frame = struct.pack(">BBHI", GQL_VERSION, opcode, 0, len(payload)) + payload
    # Outer wire frame: length(4) + type(1) + gql_frame
    total_len = 1 + len(gql_frame)
    return struct.pack(">IB", total_len, MSG_GQL_QUERY) + gql_frame


class GrimlockerClient:
    def __init__(self, host: str, port: int, token: str):
        self.url = f"ws://{host}:{port}/ws?token={token}"
        self._results: queue.Queue = queue.Queue()
        self._ws = None

    def connect(self):
        self._ws = websocket.WebSocketApp(
            self.url,
            on_message=self._on_message,
            on_error=lambda ws, e: print(f"WS error: {e}"),
        )
        t = threading.Thread(target=self._ws.run_forever, daemon=True)
        t.start()

    def _on_message(self, ws, data: bytes):
        if len(data) < 5:
            return
        msg_type = data[4]
        if msg_type == MSG_GQL_RESULT and len(data) > 5 + 8:
            inner = data[5 + 8:]  # Skip outer header + 8-byte GQL header
            try:
                self._results.put(json.loads(inner))
            except Exception:
                pass

    def _send_gql(self, operation: str, **kwargs) -> dict:
        frame = build_gql_frame(operation, **kwargs)
        self._ws.send(frame, websocket.ABNF.OPCODE_BINARY)
        return self._results.get(timeout=10)

    def list_entries(self, namespace: str = "default") -> list:
        r = self._send_gql("list_entries", namespace=namespace)
        return r.get("entries", [])

    def query_by_category(self, category: str, namespace: str = "default") -> list:
        r = self._send_gql("query_entries", namespace=namespace, category=category)
        return r.get("entries", [])

    def get_entry(self, entry_id: str, namespace: str = "default") -> dict:
        r = self._send_gql("get_entry", namespace=namespace, entry_id=entry_id)
        entries = r.get("entries", [])
        return entries[0] if entries else {}

    def create_entry(self, title: str, category: str, fields: dict,
                     namespace: str = "default") -> str:
        r = self._send_gql("create_entry", namespace=namespace,
                           title=title, category=category, fields=fields)
        if not r.get("success"):
            raise Exception(r.get("error_msg", "Create failed"))
        entries = r.get("entries", [])
        return entries[0]["id"] if entries else ""

    def delete_entry(self, entry_id: str, namespace: str = "default"):
        r = self._send_gql("delete_entry", namespace=namespace, entry_id=entry_id)
        if not r.get("success"):
            raise Exception(r.get("error_msg", "Delete failed"))

    def close(self):
        if self._ws:
            self._ws.close()
```

### Usage Example (Single-User)

```python
import os
from grimlocker_client import GrimlockerClient

token = os.environ["GRIMLOCKER_TOKEN"]
port  = int(os.environ["GRIMLOCKER_PORT"])

client = GrimlockerClient("127.0.0.1", port, token)
client.connect()

# List all entries
entries = client.list_entries()
print(f"Found {len(entries)} entries")

# Query passwords
passwords = client.query_by_category("PASSWORD")
for p in passwords:
    print(f"  - {p['title']}")

# Query SSH keys
ssh_keys = client.query_by_category("SSH_KEY")
for k in ssh_keys:
    print(f"  - {k['title']}")

# Create entry
entry_id = client.create_entry(
    title="My Server",
    category="PASSWORD",
    fields={"username": "admin", "url": "https://myserver.example.com"}
)
print(f"Created: {entry_id}")

# Get single entry
entry = client.get_entry(entry_id)
print(f"Got: {entry['title']}")

client.close()
```

---

## 8. Python Client — Enterprise (mTLS)

```python
import ssl
import websocket

def build_mtls_context(client_cert: str, client_key: str, ca_cert: str) -> ssl.SSLContext:
    ctx = ssl.SSLContext(ssl.PROTOCOL_TLS_CLIENT)
    ctx.load_cert_chain(certfile=client_cert, keyfile=client_key)
    ctx.load_verify_locations(cafile=ca_cert)
    ctx.minimum_version = ssl.TLSVersion.TLSv1_3
    return ctx


class GrimlockerEnterpriseClient(GrimlockerClient):
    def __init__(self, host: str, port: int, client_cert: str, client_key: str, ca_cert: str):
        # Enterprise: no token in URL — mTLS handles auth
        self.url = f"wss://{host}:{port}/ws"
        self._ssl = build_mtls_context(client_cert, client_key, ca_cert)
        self._results = __import__("queue").Queue()
        self._ws = None

    def connect(self):
        self._ws = websocket.WebSocketApp(
            self.url,
            on_message=self._on_message,
        )
        t = __import__("threading").Thread(
            target=self._ws.run_forever,
            kwargs={"sslopt": {"context": self._ssl}},
            daemon=True,
        )
        t.start()

    def enterprise_login(self, username: str, password: str):
        # Send MsgUnlockVault with enterprise credentials
        # Handle passphrase challenge if returned (see §3)
        import struct
        import json
        creds = json.dumps({"username": username, "password": password}).encode()
        total_len = 1 + len(creds)
        frame = struct.pack(">IB", total_len, 0x10) + creds  # 0x10 = MsgUnlockVault
        self._ws.send(frame, websocket.ABNF.OPCODE_BINARY)
        # Wait for MsgUnlockResult (0x13)
        return self._results.get(timeout=15)
```

### Usage Example (Enterprise)

```python
from grimlocker_enterprise import GrimlockerEnterpriseClient

client = GrimlockerEnterpriseClient(
    host="grimlocker.company.internal",
    port=9443,
    client_cert="./certs/client.crt",
    client_key="./certs/client.key",
    ca_cert="./certs/ca.crt"
)
client.connect()
client.enterprise_login("alice", "generated-password-from-admin")

entries = client.list_entries(namespace="alice")
print(f"Alice has {len(entries)} entries")

client.close()
```

---

## 9. Error Codes

| Code | Name | Description |
|------|------|-------------|
| -10 | missing_entry_id | `entry_id` required for this operation |
| -11 | entry_not_found | Entry does not exist in the vault |
| -20 | category_query_failed | Category query returned no results or failed |
| -30 | create_failed | Entry creation failed (storage error) |
| -31 | update_failed | Entry update failed |
| -32 | delete_failed | Entry deletion failed |
| -100 | dispatcher_unavailable | GQL dispatcher not initialised |
| -101 | invalid_frame | Frame decode error (malformed binary) |
| -102 | schema_validation | Syntactic validation failed |
| -103 | acl_denied | Semantic validation failed (ACL / permissions) |
| -104 | not_a_query_frame | Frame opcode is not Query or Mutate |
| -105 | dispatch_error | Internal dispatch to storage failed |

---

## 10. Security Guidelines

### Mandatory

- Enterprise tier: **mTLS is not optional**. Never use wss:// without a valid client certificate.
- Single-user tier: The token is 256-bit (32 random bytes, base64-encoded). Never log or expose it.
- Both tiers: All data is encrypted in transit (TLS 1.3 minimum) and at rest (ChaCha20-Poly1305).
- Never serialize passwords or MVK handles into GQL `fields`. GQL fields are for entry data only.

### Recommended

- Pin the server certificate via SPKI hash if connecting from outside a trusted network:
  ```python
  # Compute SPKI hash of server cert (once):
  openssl x509 -in server.crt -pubkey -noout | \
    openssl pkey -pubin -outform DER | openssl dgst -sha256 -binary | base64
  ```
- Rotate the single-user token periodically by restarting the daemon.
- Use a separate namespace per user/application to isolate entry access.
- Close the WebSocket connection when done — idle connections are terminated after 90s.
