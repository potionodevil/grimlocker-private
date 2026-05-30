#!/usr/bin/env bash
# get-token.sh — Fetch a JWT access token from the local Keycloak instance.
# Used for testing the enterprise tier unlock flow.
#
# Usage:
#   TOKEN=$(./scripts/get-token.sh)
#   ./grimlocker-client unlock $TOKEN
#
# Requires: curl, jq
set -euo pipefail

KEYCLOAK_URL="${KEYCLOAK_URL:-http://localhost:8080}"
REALM="${KEYCLOAK_REALM:-grimlocker}"
CLIENT_ID="${GRIMLOCKER_OIDC_CLIENT_ID:-grimlocker-daemon}"
CLIENT_SECRET="${GRIMLOCKER_OIDC_CLIENT_SECRET:-changeme}"
TEST_USER="${GRIMLOCKER_TEST_USER:-admin@grimlocker.local}"
TEST_PASS="${GRIMLOCKER_TEST_PASS:-GrimlockAdmin1!}"

TOKEN_URL="$KEYCLOAK_URL/realms/$REALM/protocol/openid-connect/token"

RESPONSE=$(curl -sf -X POST "$TOKEN_URL" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  -d "grant_type=password" \
  -d "client_id=$CLIENT_ID" \
  -d "client_secret=$CLIENT_SECRET" \
  -d "username=$TEST_USER" \
  -d "password=$TEST_PASS" \
  -d "scope=openid")

echo "$RESPONSE" | jq -r '.access_token'
