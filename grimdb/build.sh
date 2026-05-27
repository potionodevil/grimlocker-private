#!/usr/bin/env bash
set -euo pipefail

TARGET="grimlocker-go-x86_64-pc-windows-msvc.exe"
BUILD_VERSION=$(git describe --tags --always 2>/dev/null || echo "dev")

echo "[Grimlocker] Building $TARGET (version: $BUILD_VERSION)"

# Phase 1: Build Rust DLL (core-rust)
echo "[RustBuild] Compiling core-rust library..."
cd ../core-rust
cargo build --release --target x86_64-pc-windows-gnu 2>&1 | grep -v "Compiling" | grep -v "Finished" || true

# Copy the DLL and import library to the grimdb directory
DLL_PATH="target/x86_64-pc-windows-gnu/release/grimlocker_core.dll"
LIB_PATH="target/x86_64-pc-windows-gnu/release/grimlocker_core.lib"

if [ ! -f "$DLL_PATH" ]; then
    echo "[Error] Failed to build grimlocker_core.dll"
    exit 1
fi

cp "$DLL_PATH" ../grimdb/
cp "$LIB_PATH" ../grimdb/
echo "[RustBuild] DLL built and copied to ../grimdb/"

cd ../grimdb

# Phase 2: Build Go daemon with CGO enabled
echo "[GoBuild] Compiling Go daemon with CGO..."

# Set up CGO environment for Windows MSVC cross-compilation
export CGO_ENABLED=1
export CC=x86_64-w64-mingw32-gcc
export GOOS=windows
export GOARCH=amd64

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
cp "grimlocker_core.dll" "${TAURI_BIN_DIR}/"
echo "[Grimlocker] Deployed to: ${TAURI_BIN_DIR}/"
