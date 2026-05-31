#!/usr/bin/env bash
set -euo pipefail

# ── OS Detection ─────────────────────────────────────────────────────────────
OS_NAME="$(uname -s)"

case "$OS_NAME" in
  Linux)
    RUST_TARGET="x86_64-unknown-linux-gnu"
    RUST_LIB_NAME="libgrimlocker_core.so"
    RUST_LIB_SRC_DIR="target/${RUST_TARGET}/release"
    GO_BIN_PREFIX="grimdb-daemon"
    GO_BIN_SUFFIX=""
    GO_OS="linux"
    CGO_CC=""
    ;;
  MINGW*|MSYS*|CYGWIN*)
    # Building on Windows with Git Bash / MSYS2 / Cygwin (native)
    RUST_TARGET="x86_64-pc-windows-gnu"
    RUST_LIB_NAME="grimlocker_core.dll"
    RUST_LIB_SRC_DIR="target/${RUST_TARGET}/release"
    GO_BIN_PREFIX="grimdb-daemon"
    GO_BIN_SUFFIX=".exe"
    GO_OS="windows"
    CGO_CC=""
    ;;
  Darwin)
    RUST_TARGET="$(uname -m)-apple-darwin"
    RUST_LIB_NAME="libgrimlocker_core.dylib"
    RUST_LIB_SRC_DIR="target/${RUST_TARGET}/release"
    GO_BIN_PREFIX="grimdb-daemon"
    GO_BIN_SUFFIX=""
    GO_OS="darwin"
    CGO_CC=""
    ;;
  *)
    echo "[Error] Unsupported OS: $OS_NAME"
    exit 1
    ;;
esac

TARGET="${GO_BIN_PREFIX}-${RUST_TARGET}${GO_BIN_SUFFIX}"
BUILD_VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")

echo "[Grimlocker] Building $TARGET (version: $BUILD_VERSION) for $OS_NAME"

# Phase 1: Build Rust library (core-rust)
echo "[RustBuild] Compiling core-rust library for $RUST_TARGET..."
cd ../core-rust

# Ensure the target is installed (no-op if already present)
rustup target add "$RUST_TARGET" 2>/dev/null || true

cargo build --release --target "$RUST_TARGET" 2>&1 | grep -v "Compiling" | grep -v "Finished" || true

RUST_LIB_PATH="${RUST_LIB_SRC_DIR}/${RUST_LIB_NAME}"

if [ ! -f "$RUST_LIB_PATH" ]; then
    echo "[Error] Failed to build ${RUST_LIB_NAME}"
    exit 1
fi

cp "$RUST_LIB_PATH" ../grimdb/
# Copy import library if it exists (Windows-only)
LIB_PATH="${RUST_LIB_SRC_DIR}/grimlocker_core.lib"
if [ -f "$LIB_PATH" ]; then
    cp "$LIB_PATH" ../grimdb/
fi
echo "[RustBuild] Library built and copied to ../grimdb/"

cd ../grimdb

# Phase 2: Build Go daemon
echo "[GoBuild] Compiling Go daemon..."

if [ -n "$CGO_CC" ]; then
    export CC="$CGO_CC"
    export GOOS="$GO_OS"
    export GOARCH=amd64
fi
export CGO_ENABLED=1

# Build with binary hardening: -trimpath removes absolute paths, -s -w strips symbols
go build \
  -trimpath \
  -ldflags="-s -w -X main.buildVersion=${BUILD_VERSION}" \
  -o "${TARGET}" \
  ./cmd/daemon/

BINARY_SIZE=$(du -h "${TARGET}" | cut -f1)
echo "[Grimlocker] Built: ${TARGET} (${BINARY_SIZE})"

# Deploy to Tauri binaries directory
TAURI_BIN_DIR="../ui-layer/src-tauri/binaries"
TAURI_BIN="${TAURI_BIN_DIR}/${TARGET}"
mkdir -p "${TAURI_BIN_DIR}"

cp "${TARGET}" "${TAURI_BIN}"
cp "${RUST_LIB_NAME}" "${TAURI_BIN_DIR}/"
echo "[Grimlocker] Deployed to: ${TAURI_BIN_DIR}/"
