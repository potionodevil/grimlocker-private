# Deployment Guide

This document covers production deployment of the Grimlocker daemon in both single-user and enterprise configurations.

---

## Single-User Deployment

### Prerequisites

- Linux (x86_64) or macOS (arm64/x86_64)
- Rust 1.75+ (for building core-rust)
- Go 1.21+ (for building grimdb)
- Optional: Node.js 18+ (for building UI from source)

### Build from Source

```bash
# 1. Build Rust crypto core
cd core-rust
cargo build --release --lib
# Output: target/release/libgrimlocker_core.{so,dylib}

# 2. Build Go daemon
cd ../grimdb
go mod tidy
go build -o grimlocker ./cmd/daemon

# 3. (Optional) Build UI
cd ../ui-layer
npm install && npm run build
# Output → ../grimdb/ui-dist/ (auto-embedded in Go binary)
```

### Run

```bash
# Create data directory
mkdir -p ~/.grimlocker

# Set environment
export GRIMLOCKER_APP_DIR=~/.grimlocker
export GRIMLOCKER_TIER=single

# Start daemon
./grimdb/grimlocker

# Output:
# GRIMLOCKER_UI=http://localhost:8080
# GRIMLOCKER_IPC=ws://127.0.0.1:8374/ws
# Token: a1b2c3d4e5f6...
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

### Architecture

```
┌──────────────────────────────────────────────────────────────┐
│                      Enterprise Topology                       │
│                                                              │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                   │
│  │ Client A │  │ Client B │  │ Client C │  (CLI + mTLS)     │
│  └────┬─────┘  └────┬─────┘  └────┬─────┘                   │
│       │             │             │                           │
│       └─────────────┼─────────────┘                           │
│                     │ mTLS (:9443)                            │
│              ┌──────▼──────┐                                  │
│              │ Grimlocker  │                                  │
│              │ Daemon      │                                  │
│              │             │                                  │
│              │ :8080 (UI)  │                                  │
│              │ :8374 (WS)  │                                  │
│              │ :9443 (mTLS)│                                  │
│              └──────┬──────┘                                  │
│                     │ OIDC                                     │
│              ┌──────▼──────┐                                  │
│              │ Keycloak    │                                  │
│              │ Identity    │                                  │
│              │ Provider    │                                  │
│              │ :8443       │                                  │
│              └──────┬──────┘                                  │
│                     │                                          │
│              ┌──────▼──────┐                                  │
│              │ PostgreSQL  │                                  │
│              │ :5432       │                                  │
│              └─────────────┘                                  │
└──────────────────────────────────────────────────────────────┘
```

### Docker Compose

```yaml
# docker-compose.enterprise.yml
version: '3.8'

services:
  grimlocker:
    build:
      context: .
      dockerfile: Dockerfile.enterprise
    ports:
      - "8080:8080"   # UI
      - "8374:8374"   # WebSocket
      - "9443:9443"   # mTLS
    environment:
      - GRIMLOCKER_TIER=enterprise
      - GRIMLOCKER_APP_DIR=/data
      - GRIMLOCKER_KEYCLOAK_URL=https://keycloak:8443
      - GRIMLOCKER_MTLS_CERT=/certs/server.crt
      - GRIMLOCKER_MTLS_KEY=/certs/server.key
    volumes:
      - grimlocker_data:/data
      - ./certs:/certs:ro
    depends_on:
      - keycloak
      - postgres
    restart: unless-stopped

  keycloak:
    image: quay.io/keycloak/keycloak:24.0
    environment:
      - KEYCLOAK_ADMIN=admin
      - KEYCLOAK_ADMIN_PASSWORD=<secure-password>
      - KC_DB=postgres
      - KC_DB_URL=jdbc:postgresql://postgres:5432/keycloak
      - KC_DB_USERNAME=keycloak
      - KC_DB_PASSWORD=<db-password>
    ports:
      - "8443:8443"
    volumes:
      - ./deploy/keycloak/grimlocker-realm.json:/opt/keycloak/data/import/realm.json
    command: start-dev --import-realm
    depends_on:
      - postgres
    restart: unless-stopped

  postgres:
    image: postgres:16-alpine
    environment:
      - POSTGRES_DB=keycloak
      - POSTGRES_USER=keycloak
      - POSTGRES_PASSWORD=<db-password>
    volumes:
      - postgres_data:/var/lib/postgresql/data
    restart: unless-stopped

volumes:
  grimlocker_data:
  postgres_data:
```

### TLS Certificate Setup

```bash
cd grimdb/scripts
./gen-certs.sh
```

This generates:

```
certs/
├── ca.crt               # Certificate Authority
├── server.crt           # Server certificate (signed by CA)
├── server.key           # Server private key
├── client.crt           # Client certificate (signed by CA)
└── client.key           # Client private key
```

### Keycloak Realm Configuration

Import the realm configuration during Keycloak startup:

```bash
# The grimlocker-realm.json is in grimdb/deploy/keycloak/
# It is auto-imported by the Docker Compose config above
```

Manual import:

```bash
docker exec -it keycloak /opt/keycloak/bin/kcadm.sh create realms \
  -f /opt/keycloak/data/import/grimlocker-realm.json
```

### Health Check

```bash
# Daemon health
curl http://localhost:8080/health

# Expected response:
# {"status":"ok","tier":"enterprise","uptime":3600,"modules":["crypto","security","storage","api"]}

# CLI health check
./grimdb/cmd/healthcheck/healthcheck --addr localhost:9443 --cert certs/client.crt --key certs/client.key
```

### Enterprise Configuration Reference

| Variable | Required | Default | Description |
|---|---|---|---|
| `GRIMLOCKER_TIER` | Yes | (none) | Set to `enterprise` |
| `GRIMLOCKER_KEYCLOAK_URL` | Yes | (none) | Keycloak OIDC endpoint |
| `GRIMLOCKER_MTLS_CERT` | Yes | (none) | Path to server TLS certificate |
| `GRIMLOCKER_MTLS_KEY` | Yes | (none) | Path to server TLS private key |
| `GRIMLOCKER_MTLS_CA` | No | (none) | Path to CA certificate for client verification |
| `GRIMLOCKER_CLUSTER_MODE` | No | `standalone` | `standalone` or `replicated` |
| `GRIMLOCKER_CLUSTER_PEERS` | No | (none) | Comma-separated peer addresses |
| `GRIMLOCKER_REPLICATION_FACTOR` | No | `1` | Number of replicas (cluster mode) |

---

## Production Checklist

### Security

- [ ] All files in `GRIMLOCKER_APP_DIR` are mode `0600` or `0700`
- [ ] TLS certificates use at least 2048-bit RSA or ECDSA P-256
- [ ] Keycloak admin password is strong (32+ characters, CSPRNG)
- [ ] PostgreSQL password is strong and unique
- [ ] mTLS client certificates are issued per-user (not shared)
- [ ] Firewall allows only necessary ports (8080, 8443 from trusted networks)
- [ ] Audit logging is enabled and logs are rotated/archived
- [ ] Regular backup of `.gdb` file (encrypted, cannot be decrypted without master key)

### Performance

- [ ] `RLIMIT_MEMLOCK` set appropriately for Argon2id (at least 64 MiB)
- [ ] SSD storage recommended for `.gdb` (better random I/O for anti-forensic shredder)
- [ ] Monitor `~/.grimlocker/` disk usage (vault grows with entries/files)

### Monitoring

- [ ] Health check endpoint monitored (`/health`)
- [ ] Daemon uptime and restart count tracked
- [ ] Audit log alerts for `CRITICAL` events (lockdown, wipe, integrity mismatch)
- [ ] Disk space monitoring for vault directory

### Backup

```bash
# Encrypted backup (safe to store anywhere)
cp ~/.grimlocker/vault.gdb /backup/vault-$(date +%Y%m%d).gdb

# Entropy file backup (REQUIRED for coordinate recovery)
cp ~/.grimlocker/entropy.bin /backup/entropy-$(date +%Y%m%d).bin
```

**WARNING**: Without the entropy file, coordinate-based override is impossible. Store it securely alongside the master password.

---

## Troubleshooting

### Daemon won't start

```
Symptom: "Address already in use"
Fix:      Check if another instance is running
          lsof -i :8080 -i :8374
          kill <pid>
```

### Rust library not found

```
Symptom: Go build fails with "cannot find -lgrimlocker_core"
Fix:      Build Rust core first
          cd core-rust && cargo build --release --lib
          Verify: ls target/release/libgrimlocker_core.*
```

### WebSocket connection refused

```
Symptom: UI shows "Connection lost" on load
Fix:      Check daemon is running
          curl http://localhost:8080/health
          Ensure .grim_token exists and is readable by Tauri
```

### Lockdown state persists after reboot

```
Symptom: Vault still in lockdown after system restart
Status:   Expected behavior. Lockdown state is persisted in .gdb header.
Fix:      Wait the remaining lockdown time, or use coordinate override.
```

### mTLS handshake failure

```
Symptom: "tls: certificate signed by unknown authority"
Fix:      Verify CA certificate is installed in client trust store
          Verify client certificate is signed by the same CA
          Check certificate expiration dates
```
