# Enterprise Features

This document covers the Enterprise-tier features of Grimlocker: multi-user management, RBAC, rate limiting, intrusion detection, Panic Button, mDNS server discovery, and mTLS.

Enable Enterprise mode with the `-enterprise` build tag:

```bash
go build -tags enterprise ./cmd/daemon/
```

---

## User Management

### Creating users

Admin sends `MsgEnterpriseUserCreate` (0x52):

```json
{ "username": "bob", "roles": ["user"] }
```

The daemon:
1. Generates a one-time password (16 random bytes, base32-encoded)
2. Creates the user record with `status=pending_first_login`
3. Returns the one-time password in `MsgEnterpriseUserResult` (0x56)

On first login, the user must supply the one-time password **and** choose their own passphrase (stored in Rust enclave RAM only, never on disk).

### Roles

| Role | Permissions |
|---|---|
| `admin` | All operations + user management + Panic Button |
| `user` | Vault CRUD within their own namespace |

Roles are enforced server-side in the ACL layer (`api/ipc/protocol.go` + entry handler). Client-claimed roles are ignored.

### Revoking and restoring

```
Admin → 0x54 {user_id}  →  user status = revoked, all sessions invalidated
Admin → 0x55 {user_id}  →  user status = active, new one-time password generated
```

---

## Rate Limiting

Implemented in `security/rate_limiter.go`. Tracks failed authentication attempts per IP and per user.

| Consecutive failures | Lockout duration |
|---|---|
| 5 | 60 seconds |
| 10 | 10 minutes |
| 15 | 1 hour |
| 20 | 24 hours |

After 20+ failures the account is hard-locked and requires admin intervention via `MsgEnterpriseUserRestore` (0x55).

Counter resets on successful authentication.

---

## Intrusion Detection

Implemented in `security/intrusion_detector.go`. Monitors for:

- **Brute-force patterns** — repeated auth failures from the same IP within a rolling window
- **Credential stuffing** — high failure rate across multiple usernames from one IP
- **Session anomalies** — token reuse after explicit logout, geographic impossibility

Detected events are written to the audit log (`security/audit.go`) and trigger `MsgSystemError` (0x30) broadcasts to all admin sessions.

---

## Panic Button

Admin-only, two-step wipe for account compromise scenarios. Sent as `MsgPanicButton` (0x45):

```json
{ "passphrase": "<admin passphrase>" }
```

**Step 1 — Passphrase verification:**
The admin's passphrase is verified against the Rust enclave (same path as normal auth). If wrong, the request is rejected with `MsgError`.

**Step 2 — Noise overwrite:**
If verified, the daemon:
1. Overwrites all vault data with cryptographically random noise (Noise Protocol-style)
2. Invalidates all active sessions
3. Wipes the Rust enclave's in-memory key material
4. Responds with a fake success sequence to maintain plausible deniability
5. Exits

**Account-compromise flow** (3× password + 4× passphrase exhausted):
```
3 failed password attempts  →  lockdown_timestamp set (60s + escalating)
4 failed passphrase attempts →  Panic Button auto-triggered by daemon
```

This is the "duress" scenario: an attacker with physical access forcing credentials. The vault destroys itself rather than revealing data.

---

## mDNS Server Discovery

Enterprise clients on a VPN can discover available vault servers without manual configuration.

Client sends `MsgDiscoverServers` (0x50). The daemon:
1. Scans the local subnet via mDNS (`_grimlocker._tcp.local`)
2. Returns a server list via `MsgServerList` (0x51):

```json
[
  { "name": "vault-primary", "address": "10.8.0.10", "port": 9443, "tls_required": true },
  { "name": "vault-replica", "address": "10.8.0.11", "port": 9443, "tls_required": true }
]
```

Only servers with valid mTLS certificates appear in the list.

---

## mTLS Setup

Enterprise connections require mutual TLS on port `:9443`. Both client and server present certificates signed by your internal CA.

### Generating certificates

```bash
# CA
openssl genrsa -out ca.key 4096
openssl req -new -x509 -days 3650 -key ca.key -out ca.crt -subj "/CN=Grimlocker CA"

# Server cert
openssl genrsa -out server.key 4096
openssl req -new -key server.key -out server.csr -subj "/CN=vault.corp.example"
openssl x509 -req -days 365 -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out server.crt

# Client cert (one per user or per device)
openssl genrsa -out client.key 4096
openssl req -new -key client.key -out client.csr -subj "/CN=alice"
openssl x509 -req -days 365 -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial -out client.crt
```

### Daemon configuration

```yaml
tls:
  cert: /etc/grimlocker/server.crt
  key:  /etc/grimlocker/server.key
  ca:   /etc/grimlocker/ca.crt
  client_auth: required
```

### SDK client configuration

See [SDK_GUIDE.md](SDK_GUIDE.md#enterprise-mtls) for per-language TLS configuration examples.

---

## Deployment

See `deploy/docker-compose.enterprise.yml` for a reference multi-node setup with:
- Primary + replica vault nodes
- Nginx TLS termination
- Prometheus metrics endpoint (`/metrics`)
- Health check endpoint (`/health` on port 9090, no auth)
