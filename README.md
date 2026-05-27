# Grimlocker - Private Edition

**Das vollständige Grimlocker-Sicherheitssystem mit allen Komponenten.**

## 📋 Übersicht

Dies ist die **private Version** von Grimlocker, die das komplette System enthält:

- **Core-Rust**: Sichere Enclave für Crypto-Operationen
- **GrimDB**: Event-basierte Datenbank mit Vault-Management
- **Security Layer**: Lockdown, Audit, Memory-Protection
- **Crypto Engine**: ChaCha20-Poly1305, Argon2id, HKDF
- **UI Layer**: Tauri-basierte Benutzeroberfläche
- **Storage**: Sichere Datenspeicherung mit Encryption
- **API**: IPC/WebSocket für externe Kommunikation

## 🔒 Architektur

```
Grimlocker
├── core-rust/          # Sichere Rust-Enclave
│   └── Crypto-Primitives (ChaCha, Argon2, BLAKE3)
├── grimdb/            # Event-Driven Kernel
│   ├── kernel/        # Event-Bus & Module-System
│   ├── crypto/        # Crypto Engine (Go)
│   ├── security/      # Security Module (Lockdown, Audit)
│   ├── storage/       # Vault Storage & Encryption
│   ├── api/           # IPC/WebSocket Handler
│   └── sdk/           # TypeScript SDK
└── ui-layer/          # Tauri Frontend (React)
```

## 🚀 Setup

### Abhängigkeiten
- **Rust 1.75+** (für core-rust)
- **Go 1.21+** (für grimdb)
- **Node.js 18+** (für ui-layer)

### Build

```bash
# Rust Core
cd core-rust
cargo build --release

# Go Daemon
cd ../grimdb
go build -o grimlocker ./cmd/daemon

# UI (optional)
cd ../ui-layer
npm install && npm run build
```

## 🔑 Wichtige Konzepte

### Vault State Machine
- **Setup**: Initial password → Master Key
- **Login**: Password verification → Session Key
- **Vault**: Encrypted Workspace Management

### Security Model
- Schlüsselmaterial nur in Rust-Enclave oder locked memory
- Hard Lockdown: Sofortiges Zeroize aller Keys + Prozess-Exit
- Timing-Angriff-Schutz: Constant-time Vergleiche
- Memory-Locking: mlock/VirtualLock für sensitive Daten

### Encryption
- **Master Key**: Argon2id(password, salt) + HKDF-SHA256
- **Session Keys**: ChaCha20-Poly1305 (aus Rust Enclave)
- **Workspace Keys**: BLAKE3 + HKDF von Master Key

## 📝 Testing

```bash
# Unit Tests
go test ./...

# Integration Tests
cargo test --release

# Security Audit (siehe grimlocker-public)
```

## 📚 Weitere Infos

Siehe [grimlocker-public](../grimlocker-public) für die **public Security Audit Edition** mit nur Crypto/Security-Layer.

---

**⚠️ SICHERHEITSHINWEIS**: Dies ist die private Version mit potenziellen Credentials/Konfigurationen. Nur für persönliche/interne Nutzung.
