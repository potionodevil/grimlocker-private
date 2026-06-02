# Grimlocker Enterprise — Local Development Setup

This guide walks through setting up a full local Enterprise stack for development and testing.
The stack consists of: **Keycloak** (OIDC provider), **MinIO** (S3-compatible vault backend),
and the **Grimlocker Enterprise Daemon**.

---

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Docker Desktop | 4.0+ | Runs Keycloak + MinIO containers |
| OpenSSL | 3.0+ | Generates mTLS certificates |
| Go | 1.22+ | Builds the enterprise daemon binary |
| make / bash | any | Runs setup scripts |

---

## Step 1 — Generate mTLS Certificates

```bash
cd grimdb
chmod +x scripts/gen-certs.sh
./scripts/gen-certs.sh
```

This creates `deploy/tls/` with:
- `ca.crt` + `ca.key` — Self-signed CA (90 days)
- `server.crt` + `server.key` — Server certificate (signed by CA)
- `client.crt` + `client.key` — Client certificate (for testing)

---

## Step 2 — Start the Docker Stack

```bash
docker compose -f docker-compose.enterprise.yml up -d
```

Wait ~30 seconds for Keycloak to initialize, then verify:

```bash
# Keycloak health
curl http://localhost:8080/health/ready
# { "status": "UP" }

# MinIO health
curl http://localhost:9000/minio/health/live
# 200 OK

# Grimlocker daemon health
curl http://localhost:9090/health
# {"status":"ready","tier":"enterprise","probe":"ok"}
```

---

## Step 3 — Configure Keycloak

Open `http://localhost:8080` in your browser.

Login with admin / admin.

### Create Grimlocker Realm

1. Click "Create realm"
2. Name: `grimlocker`
3. Enable: ON
4. Save

### Create Client

1. Go to **Clients** → **Create client**
2. Client ID: `grimlocker-daemon`
3. Client authentication: ON (confidential client)
4. Root URL: (leave empty)
5. Save
6. Go to **Credentials** tab → copy the **Client secret**
7. Set in docker-compose: `GRIMLOCKER_OIDC_CLIENT_SECRET=<secret>`

### Create Test User

1. Go to **Users** → **Add user**
2. Username: `testuser`
3. Email verified: ON
4. Save
5. Go to **Credentials** tab → **Set password** → `testpassword` (Temporary: OFF)

### Get OIDC Token (for testing)

```bash
TOKEN=$(curl -s -X POST \
  http://localhost:8080/realms/grimlocker/protocol/openid-connect/token \
  -d "grant_type=password" \
  -d "client_id=grimlocker-daemon" \
  -d "client_secret=<your-client-secret>" \
  -d "username=testuser" \
  -d "password=testpassword" \
  | jq -r .access_token)

echo "Token: ${TOKEN:0:50}..."
```

---

## Step 4 — Build Enterprise Binary

```bash
cd grimdb

# Build enterprise daemon
go build -tags enterprise -o grimdb-daemon-enterprise ./cmd/daemon/

# Verify enterprise features are compiled in
./grimdb-daemon-enterprise --version
# Should show: tier=enterprise
```

Copy to the Tauri binaries directory if testing with the full Tauri app:

```bash
cp grimdb-daemon-enterprise \
   ../ui-layer/src-tauri/binaries/grimdb-daemon-$(go env GOARCH)-$(go env GOOS)
```

---

## Step 5 — Run Daemon Directly (without Docker)

For rapid iteration without rebuilding Docker images:

```bash
export GRIMLOCKER_OIDC_PROVIDER=http://localhost:8080/realms/grimlocker
export GRIMLOCKER_OIDC_CLIENT_ID=grimlocker-daemon
export GRIMLOCKER_OIDC_CLIENT_SECRET=<your-client-secret>
export GRIMLOCKER_VAULT_BACKEND=local   # Use local storage instead of MinIO
export GRIMLOCKER_MTLS_CERT_PATH=deploy/tls/server.crt
export GRIMLOCKER_MTLS_KEY_PATH=deploy/tls/server.key
export GRIMLOCKER_MTLS_CA_PATH=deploy/tls/ca.crt
export GRIMLOCKER_BIND_ADDR=0.0.0.0:9443
export GRIMLOCKER_APP_DIR=/tmp/grimlocker-enterprise-test

./grimdb-daemon-enterprise
```

---

## Step 6 — Connect from Tauri App

Set these environment variables before running the Tauri dev server:

```bash
export GRIMLOCKER_ENTERPRISE_HOST=127.0.0.1:9443
export GRIMLOCKER_ENTERPRISE_CA=deploy/tls/ca.crt
export GRIMLOCKER_CLIENT_CERT=deploy/tls/client.crt
export GRIMLOCKER_CLIENT_KEY=deploy/tls/client.key
```

Or configure via the Admin Debug Panel:
1. Open the app in dev mode
2. Go to **Admin** → **Debug Panel**
3. Toggle **Enterprise Mode Preview**
4. Enter server address: `127.0.0.1:9443`

---

## Troubleshooting

### Daemon cannot connect to Keycloak

```
ERROR: OIDC discovery failed: connection refused
```

Ensure Keycloak is running: `docker compose -f docker-compose.enterprise.yml ps`

### mTLS handshake fails

```
ERROR: tls: certificate signed by unknown authority
```

Check that `GRIMLOCKER_MTLS_CA_PATH` points to the same CA that signed `server.crt`.
Regenerate certificates with `./scripts/gen-certs.sh` if in doubt.

### MinIO bucket not found

```
ERROR: NoSuchBucket
```

The MinIO init container should create the bucket automatically.
If not: `docker compose -f docker-compose.enterprise.yml run minio-init`

### Token validation fails

```
ERROR: auth: JWT validation: token expired
```

Keycloak tokens expire after 5 minutes. Fetch a new token (see Step 3).
In production, use a refresh token or service account.

---

## Default Credentials (Development Only)

| Service | URL | Username | Password |
|---------|-----|----------|---------|
| Keycloak Admin | http://localhost:8080 | admin | admin |
| MinIO Console | http://localhost:9001 | minioadmin | minioadmin123 |
| Test OIDC User | — | testuser | testpassword |

**Do not use these credentials in production.**

---

## Security Notes for Development

- All certificates are self-signed and expire in 90 days
- The MinIO credentials are hardcoded in docker-compose — change before any external exposure
- Keycloak runs in dev mode (no persistent storage) — data is lost on restart
- For production deployment, see `deploy/README.md`
