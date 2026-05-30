#!/usr/bin/env bash
# gen-certs.sh — Generate self-signed mTLS certificates for Grimlocker Enterprise.
#
# Creates:
#   deploy/tls/ca.crt / ca.key        — Root CA (sign server + client certs)
#   deploy/tls/server.crt / server.key — Daemon TLS certificate
#   deploy/tls/client.crt / client.key — CLI client certificate
#
# Usage:
#   ./scripts/gen-certs.sh [SERVER_NAME]
#
# SERVER_NAME defaults to "localhost" — set to the daemon's hostname/IP
# for production deployments (e.g. "grimlocker.company.internal").
set -euo pipefail

OUTDIR="$(dirname "$0")/../deploy/tls"
SERVER_NAME="${1:-localhost}"
DAYS=825  # ~2 years (Apple/Chrome max is 825 days)

mkdir -p "$OUTDIR"
cd "$OUTDIR"

echo "==> Generating Grimlocker Enterprise TLS certificates"
echo "    Server name : $SERVER_NAME"
echo "    Output dir  : $OUTDIR"
echo "    Validity    : $DAYS days"
echo ""

# ── 1. Root CA ────────────────────────────────────────────────────────────────
echo "[1/3] Generating CA key and certificate..."
openssl genrsa -out ca.key 4096 2>/dev/null
openssl req -new -x509 -key ca.key -out ca.crt -days "$DAYS" \
  -subj "/CN=Grimlocker Enterprise CA/O=Grimlocker/OU=Security" \
  -extensions v3_ca \
  -addext "keyUsage=critical,keyCertSign,cRLSign" \
  -addext "basicConstraints=critical,CA:TRUE,pathlen:0"
echo "    CA certificate: $OUTDIR/ca.crt"

# ── 2. Server certificate (daemon) ───────────────────────────────────────────
echo "[2/3] Generating server key and certificate (CN=$SERVER_NAME)..."
openssl genrsa -out server.key 4096 2>/dev/null
openssl req -new -key server.key -out server.csr \
  -subj "/CN=$SERVER_NAME/O=Grimlocker/OU=Daemon"

# SAN extension for the server cert.
cat > server_ext.cnf <<EOF
[SAN]
subjectAltName=DNS:$SERVER_NAME,DNS:localhost,IP:127.0.0.1
keyUsage=critical,digitalSignature,keyEncipherment
extendedKeyUsage=serverAuth
EOF

openssl x509 -req -in server.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out server.crt -days "$DAYS" \
  -extfile server_ext.cnf -extensions SAN 2>/dev/null
rm -f server.csr server_ext.cnf
echo "    Server certificate: $OUTDIR/server.crt"

# ── 3. Client certificate (CLI) ───────────────────────────────────────────────
echo "[3/3] Generating client key and certificate..."
openssl genrsa -out client.key 4096 2>/dev/null
openssl req -new -key client.key -out client.csr \
  -subj "/CN=grimlocker-cli/O=Grimlocker/OU=Client"

cat > client_ext.cnf <<EOF
[ext]
keyUsage=critical,digitalSignature
extendedKeyUsage=clientAuth
EOF

openssl x509 -req -in client.csr -CA ca.crt -CAkey ca.key -CAcreateserial \
  -out client.crt -days "$DAYS" \
  -extfile client_ext.cnf -extensions ext 2>/dev/null
rm -f client.csr client_ext.cnf ca.srl
echo "    Client certificate: $OUTDIR/client.crt"

# ── Summary ────────────────────────────────────────────────────────────────────
echo ""
echo "==> Done! Files generated in $OUTDIR:"
ls -lh "$OUTDIR"
echo ""
echo "==> Next steps:"
echo "    1. Start the stack:  docker-compose -f docker-compose.enterprise.yml up -d"
echo "    2. Set client env vars:"
echo "       export GRIMLOCKER_DAEMON_ADDR=localhost:9443"
echo "       export GRIMLOCKER_CLIENT_CERT=$OUTDIR/client.crt"
echo "       export GRIMLOCKER_CLIENT_KEY=$OUTDIR/client.key"
echo "       export GRIMLOCKER_CA_CERT=$OUTDIR/ca.crt"
echo "    3. Get OIDC token and unlock:"
echo "       TOKEN=\$(./scripts/get-token.sh)"
echo "       ./grimlocker-client unlock \$TOKEN"
