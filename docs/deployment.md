# Deployment Guide

This document covers production deployment of Grimlocker in both the **Single-User (desktop EXE)** and **Enterprise (Docker + CLI)** configurations.

---

## Deployment Tiers

| | Single-User | Enterprise |
|---|---|---|
| **Distribution** | Windows/macOS/Linux EXE | Docker image (distroless) |
| **Authentication** | Argon2id master password | OIDC JWT (Keycloak / Azure AD) |
| **Storage** | Local `vault.gdb` file | S3 / MinIO object store |
| **Transport** | Local IPC (127.0.0.1) | mTLS on port 9443 |
| **Client** | Tauri desktop app | `grimlocker` CLI or Tauri UI |
| **Build command** | `go build ./cmd/daemon` | `go build -tags enterprise ./cmd/daemon` |

---

## Single-User Deployment

### Prerequisites

- Go 1.21+
- Rust 1.75+ with `cargo` (for building the crypto core)
- Node.js 18+ (optional — only needed to rebuild the embedded UI)

### Build from Source

```bash
# 1. Build Rust crypto core (outputs grimlocker_core.dll / .so / .dylib)
cd core-rust
cargo build --release --lib

# 2. Build Go daemon (embeds the UI assets at compile time)
cd ../grimdb
go mod tidy
go build -o grimlocker ./cmd/daemon

# 3. Build universal CLI client
go build -o grimlocker-cli ./cmd/client

# 4. (Optional) Rebuild embedded UI from source
cd ../ui-layer
npm install && npm run build
# Output → ../grimdb/ui-dist/ — re-run step 2 to embed updated UI
```

### Run

```bash
# The daemon picks a random port on startup and prints it:
./grimlocker

# Startup output:
# GRIMLOCKER_COOKIE=<base64>
# GRIMLOCKER_TOKEN=<token>
# GRIMLOCKER_UI=http://127.0.0.1:<ui-port>
# GRIMLOCKER_IPC=ws://127.0.0.1:<ipc-port>/ws
# [GRIMLOCKER] ===== DAEMON READY =====
```

### Headless / CLI-only Workflow

```bash
# Start daemon
export GRIMLOCKER_APP_DIR=~/.grimlocker
./grimlocker &

# Capture IPC address from daemon stdout
export GRIMLOCKER_IPC=$(./grimlocker 2>/dev/null | grep GRIMLOCKER_IPC | cut -d= -f2)

# Initialize vault (first run only)
./grimlocker-cli init "MyMasterPassword"
# → prints recovery phrase — store securely!

# Unlock and use
./grimlocker-cli unlock "MyMasterPassword"
./grimlocker-cli set "github/token" "ghp_secret123"
./grimlocker-cli get "github/token"
./grimlocker-cli list
./grimlocker-cli lock
./grimlocker-cli health
```

### Systemd Service (Linux)

```
# /etc/systemd/system/grimlocker.service

[Unit]
Description=Grimlocker Password Manager Daemon
After=network.target

[Service]
Type=simple
User=your-user
Environment=GRIMLOCKER_APP_DIR=/home/your-user/.grimlocker
Environment=GRIMLOCKER_TIER=single
ExecStart=/usr/local/bin/grimlocker
Restart=on-failure
RestartSec=5
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/home/your-user/.grimlocker
NoNewPrivileges=true
MemoryDenyWriteExecute=true

[Install]
WantedBy=multi-user.target
```

Enable and start:

```bash
sudo systemctl daemon-reload
sudo systemctl enable grimlocker
sudo systemctl start grimlocker
```

### Data Directory Layout

```
~/.grimlocker/
├── .grim_token           # 32-byte auth token (0600, deleted on exit)
├── vault.gdb             # Encrypted vault (0600)
├── entropy.bin           # CSPRNG entropy source (0600)
└── audit.log             # Plaintext audit log (for review, 0600)
```

### Security Hardening

Ensure all files in `~/.grimlocker/` have mode `0600`:

```bash
chmod 700 ~/.grimlocker
chmod 600 ~/.grimlocker/*
```

---

## Enterprise Deployment

The Enterprise tier runs the daemon as a **Docker container on a Linux server** and communicates with client workstations via **mutual TLS (mTLS)**. Authentication is handled by an external OIDC provider (Keycloak / Azure AD / Okta). Vault data is stored on S3-compatible object storage (AWS S3 or MinIO).

### Architecture

```
┌────────────────────────────────────────────────────────────────┐
│                       Enterprise Topology                        │
│                                                                │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                     │
│  │ Client A │  │ Client B │  │ Client C │  ← grimlocker CLI   │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘    or Tauri UI     │
│       │             │             │                             │
│       └─────────────┼─────────────┘                             │
│                     │ mTLS :9443 (mutual cert auth)             │
│              ┌──────▼──────┐                                    │
│              │  Grimlocker │ ← distroless/static Docker image  │
│              │   Daemon    │                                    │
│              │             │   :9443 client connections (mTLS)  │
│              │             │   :9090 liveness probe (plaintext) │
│              └──────┬──────┘                                    │
│               OIDC  │  S3 API                                   │
│          ┌──────────┤──────────┐                                │
│          │          │          │                                │
│   ┌──────▼──────┐  ┌▼──────────▼──┐                           │
│   │  Keycloak   │  │  MinIO / S3  │                           │
│   │  OIDC :8080 │  │  :9000 API   │                           │
│   └─────────────┘  └──────────────┘                           │
└────────────────────────────────────────────────────────────────┘
```

### Build the Enterprise Binary

The enterprise tier is compiled with Go build tags. **No CGO required** — the binary is fully static.

```bash
cd grimdb

# Enterprise daemon
CGO_ENABLED=0 go build -tags enterprise \
  -ldflags "-s -w" \
  -o grimlocker-enterprise \
  ./cmd/daemon

# Universal CLI client (no build tag — supports both tiers)
go build -o grimlocker ./cmd/client

# Docker image (distroless, ~15 MB)
BUILD_DATE=$(date +%Y%m%d) docker-compose -f docker-compose.enterprise.yml build
```

### Quick Start (docker-compose)

```bash
# 1. Generate TLS certificates
bash grimdb/scripts/gen-certs.sh          # outputs to grimdb/deploy/tls/

# 2. Start the full stack (Keycloak + MinIO + Grimlocker)
cd grimdb
docker-compose -f docker-compose.enterprise.yml up -d

# 3. Wait for readiness
docker logs grimlocker-daemon | grep "DAEMON READY"

# 4. Configure CLI environment
export GRIMLOCKER_DAEMON_ADDR=localhost:9443
export GRIMLOCKER_CLIENT_CERT=deploy/tls/client.crt
export GRIMLOCKER_CLIENT_KEY=deploy/tls/client.key
export GRIMLOCKER_CA_CERT=deploy/tls/ca.crt

# 5. Obtain OIDC token and unlock
TOKEN=$(bash scripts/get-token.sh)
./grimlocker unlock "$TOKEN"

# 6. Use the vault
./grimlocker set "github/token" "ghp_secret123"
./grimlocker get "github/token"
./grimlocker list
./grimlocker health
```

### TLS Certificate Generation

```bash
# Generate self-signed CA + server + client certificates
bash grimdb/scripts/gen-certs.sh [SERVER_HOSTNAME]
# Default SERVER_HOSTNAME: localhost

# Output:
grimdb/deploy/tls/
├── ca.crt        # Root CA (sign both server and client certs)
├── ca.key        # CA private key  — keep offline in production
├── server.crt    # Daemon server certificate
├── server.key    # Daemon server private key
├── client.crt    # CLI client certificate
└── client.key    # CLI client private key
```

For production: use your PKI infrastructure or a CA like Step-CA / HashiCorp Vault PKI.

### SPKI Certificate Pinning (Optional Hardening)

Pin the server's public key so clients reject certificates signed by a compromised CA:

```bash
# Get server certificate SPKI hash
openssl x509 -in deploy/tls/server.crt -pubkey -noout \
  | openssl pkey -pubin -outform der \
  | openssl dgst -sha256 -hex

# Set pin in environment
export GRIMLOCKER_MTLS_PIN_SPKI=<hash-from-above>
```

### Keycloak Realm Configuration

The Keycloak realm is auto-imported from `grimdb/deploy/keycloak/grimlocker-realm.json` on first startup.

Pre-configured accounts:

| Username | Password | Role |
|---|---|---|
| `admin@grimlocker.local` | `GrimlockAdmin1!` | `vault-admin` |
| `user@grimlocker.local` | `VaultUser1!` | `vault-user` |

For production: change all passwords and disable `directAccessGrantsEnabled` in the Keycloak client.

Obtain a token manually:

```bash
curl -s -X POST \
  http://localhost:8080/realms/grimlocker/protocol/openid-connect/token \
  -d "grant_type=password" \
  -d "client_id=grimlocker-daemon" \
  -d "client_secret=changeme" \
  -d "username=admin@grimlocker.local" \
  -d "password=GrimlockAdmin1!" \
  | jq -r '.access_token'
```

### Enterprise Health Check

```bash
# Daemon health (plaintext probe port — no mTLS required)
curl http://localhost:9090/health

# Expected response:
# {"status":"ready","tier":"enterprise","probe":"ok"}

# Full health via mTLS
curl --cert deploy/tls/client.crt \
     --key  deploy/tls/client.key \
     --cacert deploy/tls/ca.crt \
     https://localhost:9443/health

# Docker HEALTHCHECK uses the static binary /usr/local/bin/grimlocker-healthcheck
# (built from cmd/healthcheck/ — no shell needed in distroless image)
```

### Enterprise Environment Variables

| Variable | Required | Default | Description |
|---|---|---|---|
| `GRIMLOCKER_TIER` | Yes | `single` | Set to `enterprise` |
| `GRIMLOCKER_OIDC_PROVIDER` | Yes | — | OIDC issuer URL (e.g. `http://keycloak:8080/realms/grimlocker`) |
| `GRIMLOCKER_OIDC_CLIENT_ID` | Yes | — | OIDC client ID |
| `GRIMLOCKER_OIDC_CLIENT_SECRET` | No | — | OIDC client secret (if confidential client) |
| `GRIMLOCKER_VAULT_BACKEND` | No | `s3` | `s3` or `minio` |
| `GRIMLOCKER_S3_BUCKET` | Yes | — | S3 bucket name |
| `GRIMLOCKER_S3_REGION` | No | `us-east-1` | AWS region |
| `GRIMLOCKER_S3_ENDPOINT` | No | AWS default | Custom endpoint (for MinIO) |
| `AWS_ACCESS_KEY_ID` | Yes | — | S3 / MinIO access key |
| `AWS_SECRET_ACCESS_KEY` | Yes | — | S3 / MinIO secret key |
| `GRIMLOCKER_MTLS_CERT_PATH` | Yes | — | Server TLS certificate path |
| `GRIMLOCKER_MTLS_KEY_PATH` | Yes | — | Server TLS private key path |
| `GRIMLOCKER_MTLS_CA_PATH` | Yes | — | CA certificate path (for client validation) |
| `GRIMLOCKER_MTLS_PIN_SPKI` | No | — | Optional SHA-256 hex SPKI pin for client cert |
| `GRIMLOCKER_APP_DIR` | No | `/var/lib/grimlocker` | Data directory (entropy.bin location) |
| `GRIMLOCKER_PROBE_PORT` | No | `9090` | Plaintext liveness probe port |

---

## Production Checklist

### Security (Both Tiers)

- [ ] All files in `GRIMLOCKER_APP_DIR` are mode `0600` / `0700`
- [ ] Recovery phrase stored offline in a secure location (printed / hardware key)
- [ ] `entropy.bin` backed up securely — without it, MVK re-derivation is impossible
- [ ] Regular backups of `vault.gdb` (encrypted at rest, safe to store anywhere)
- [ ] `RLIMIT_MEMLOCK` ≥ 64 MiB on Linux (Argon2id key derivation)

### Security (Enterprise)

- [ ] TLS certificates ≥ 2048-bit RSA or ECDSA P-256; validity ≤ 825 days
- [ ] mTLS client certificates issued per-user (never shared)
- [ ] Keycloak admin password rotated from default (32+ chars)
- [ ] `directAccessGrantsEnabled` disabled in Keycloak for production
- [ ] S3 bucket policy restricts access to daemon IAM role only
- [ ] Firewall: port 9443 accessible from client workstations only; 9090 accessible from monitoring only
- [ ] SPKI pinning configured (`GRIMLOCKER_MTLS_PIN_SPKI`) for extra cert chain protection

### Monitoring

- [ ] Health endpoint monitored: `GET /health` (returns `vault_unlocked`, `tier`, `version`)
- [ ] Docker HEALTHCHECK passes (uses `grimlocker-healthcheck` binary on probe port 9090)
- [ ] Audit log alerts for `CRITICAL` events: lockdown, wipe, integrity mismatch, unauthorized access
- [ ] Certificate expiry monitored — daemon logs a warning 30 days before expiry

### Graceful Shutdown

The daemon implements cooperative shutdown via `POST /shutdown`. Tauri calls this endpoint before sending SIGKILL. The daemon:
1. Flushes in-flight storage writes
2. Locks the session (revokes MVK)
3. Destroys Rust enclave session handles
4. Stops all kernel modules (5-second timeout)
5. Exits cleanly

```bash
# Request graceful shutdown (e.g. from a management script)
IPC_PORT=$(cat /run/grimlocker/ipc_port)
curl -s -X POST "http://127.0.0.1:${IPC_PORT}/shutdown"
```

---

## Troubleshooting

### Daemon won't start — "enterprise config validation failed"

```
Symptom: Daemon exits immediately with config validation errors
Fix:      Check all required enterprise env vars are set.
          Required: GRIMLOCKER_OIDC_PROVIDER, GRIMLOCKER_OIDC_CLIENT_ID,
                    GRIMLOCKER_S3_BUCKET, AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY,
                    GRIMLOCKER_MTLS_CERT_PATH, GRIMLOCKER_MTLS_KEY_PATH, GRIMLOCKER_MTLS_CA_PATH
```

### Rust library not found (single-user Windows)

```
Symptom: [rustbridge] DLL not found, using Go fallback
Status:   Non-fatal — daemon continues with pure-Go cryptography
Fix:      Place grimlocker_core.dll alongside the daemon binary
          Build: cd core-rust && cargo build --release
```

### WebSocket connection refused (single-user Tauri)

```
Symptom: UI shows "Connection lost" immediately
Fix:      Check GRIMLOCKER_IPC env var is set and port is correct
          curl http://127.0.0.1:<ipc-port>/health
          Restart Tauri app (it will respawn the daemon automatically)
```

### mTLS handshake failure (enterprise)

```
Symptom: "tls: certificate signed by unknown authority"
Fix:      Verify GRIMLOCKER_MTLS_CA_PATH points to the correct CA cert
          Check client cert is signed by the same CA: openssl verify -CAfile ca.crt client.crt
          Check cert expiry: openssl x509 -in server.crt -noout -dates
```

### File upload returns "unauthorized"

```
Symptom: File uploads fail with error "unauthorized" in the UI
Status:   Fixed in v2026-05-30. Upgrade the daemon binary.
Cause:    Policy check incorrectly required subject_id in MsgFileIngestBegin payload.
```

### Lockdown state persists after reboot

```
Symptom: Vault still in lockdown after system restart
Status:   Expected behavior. Lockdown state persists in the vault.gdb header.
Fix:      Wait the remaining lockdown duration, or use the recovery phrase.
```

### S3 connectivity (enterprise)

```
Symptom: "upload block: S3 PUT: status 403"
Fix:      Verify AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY are correct
          Check bucket exists: aws s3 ls s3://<bucket-name>
          For MinIO: verify GRIMLOCKER_S3_ENDPOINT points to the MinIO API port (9000)
```
