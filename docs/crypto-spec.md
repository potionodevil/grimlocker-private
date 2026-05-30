# Cryptographic Specification

This document formally specifies every cryptographic primitive, parameter, and operation used in Grimlocker.

---

## Algorithms Summary

| Primitive | Standard | Implementation | Purpose |
|---|---|---|---|
| Argon2id | RFC 9106 | `golang.org/x/crypto/argon2` | Password hashing |
| ChaCha20-Poly1305 | RFC 8439 | `chacha20poly1305` crate + `golang.org/x/crypto/chacha20poly1305` | Authenticated encryption |
| BLAKE3 | — | `blake3` crate | Key derivation hashing |
| HKDF-SHA256 | RFC 5869 | `golang.org/x/crypto/hkdf` | Key derivation expansion |
| CSPRNG | — | `rand::rngs::OsRng` (Rust) / `crypto/rand` (Go) | Random generation |
| SHA-256 | FIPS 180-4 | `crypto/sha256` (Go) | Audit log chaining |

---

## 1. Master Key Derivation

### Inputs

| Parameter | Value | Source |
|---|---|---|
| Password | User-provided string (UTF-8) | User input |
| Salt | 32 bytes | CSPRNG, stored with vault |
| Memory | 32 MiB (33554432 bytes) | Hardcoded |
| Iterations | 3 | Hardcoded |
| Parallelism | 4 | Hardcoded |
| Key length | 32 bytes | Hardcoded |

### Algorithm

```
MasterKey = Argon2id(
    password: []byte(password),
    salt:     salt_bytes,
    memory:   32 * 1024 * 1024,
    time:     3,
    threads:  4,
    keyLen:   32
)
```

### Go Implementation

```go
// grimdb/crypto/argon.go
import "golang.org/x/crypto/argon2"

masterKey := argon2.IDKey(
    password,
    salt,
    3,        // iterations
    32*1024,  // memory in KiB
    4,        // threads
    32,       // key length
)
```

### Security Properties

- **Memory-hardness**: 32 MiB prevents GPU/ASIC acceleration — each brute-force attempt requires 32 MiB of dedicated RAM
- **Salt uniqueness**: Each vault has a unique 32-byte salt, preventing rainbow table attacks
- **Tunable parameters**: Enterprise tier can increase memory/iterations for additional security

---

## 2. ChaCha20-Poly1305 AEAD Encryption

### Algorithm

ChaCha20-Poly1305 as specified in RFC 8439:
- ChaCha20 stream cipher with 256-bit key, 96-bit nonce
- Poly1305 one-time authenticator for ciphertext integrity
- Encrypt-then-MAC construction (inherent to AEAD)

### Key Sizes

| Parameter | Size | Notes |
|---|---|---|
| Key | 256 bits (32 bytes) | ChaCha20 key |
| Nonce | 96 bits (12 bytes) | Must be unique per key; CSPRNG-generated |
| Authentication Tag | 128 bits (16 bytes) | Poly1305 MAC |

### Nonce Management

- Nonces are generated via CSPRNG for each encryption operation
- A 96-bit space provides 2^96 unique nonces per key
- No counter-based nonce generation (avoids state management complexity)
- Nonce is prepended to the ciphertext: `[12-byte nonce][ciphertext + 16-byte tag]`

### Rust Implementation

```rust
// core-rust/src/crypto.rs
use chacha20poly1305::{
    ChaCha20Poly1305,
    Key, Nonce,
    aead::{Aead, KeyInit, OsRng},
};

fn encrypt(key: &[u8; 32], plaintext: &[u8]) -> Vec<u8> {
    let cipher = ChaCha20Poly1305::new(Key::from_slice(key));
    let nonce = ChaCha20Poly1305::generate_nonce(&mut OsRng);
    let ciphertext = cipher.encrypt(&nonce, plaintext).unwrap();

    // Prepend nonce to output
    [nonce.as_slice(), &ciphertext].concat()
}

fn decrypt(key: &[u8; 32], encrypted: &[u8]) -> Option<Vec<u8>> {
    let (nonce_bytes, ciphertext) = encrypted.split_at(12);
    let cipher = ChaCha20Poly1305::new(Key::from_slice(key));
    let nonce = Nonce::from_slice(nonce_bytes);
    cipher.decrypt(nonce, ciphertext).ok()
}
```

### Go Implementation

```go
// grimdb/crypto/chacha.go
import "golang.org/x/crypto/chacha20poly1305"

func Encrypt(key []byte, plaintext []byte) ([]byte, error) {
    aead, _ := chacha20poly1305.New(key)
    nonce := make([]byte, aead.NonceSize())
    _, _ = rand.Read(nonce)
    return aead.Seal(nonce, nonce, plaintext, nil), nil
}

func Decrypt(key []byte, ciphertext []byte) ([]byte, error) {
    aead, _ := chacha20poly1305.New(key)
    nonceSize := aead.NonceSize()
    nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
    return aead.Open(nil, nonce, ct, nil)
}
```

### Security Properties

- **IND-CCA3**: Chosen ciphertext attack resistance via AEAD authentication
- **Nonce-misuse resistance**: Partial — reuse is detectable (identical ciphertexts) but not catastrophic
- **Constant-time**: ChaCha20 stream cipher is naturally constant-time; Poly1305 is constant-time in the `chacha20poly1305` crate
- **Tag verification**: Decryption verifies the Poly1305 tag before returning plaintext

---

## 3. BLAKE3 Key Derivation

### Algorithm

BLAKE3 is a cryptographic hash function optimized for speed and parallelism.

### Usage

```
IntermediateKey = BLAKE3(MasterKey)
WorkspaceKey   = HKDF-SHA256(IntermediateKey, salt="workspace", info=workspaceUUID)
```

### Reason for BLAKE3

- **Performance**: Significant speed advantage over SHA-256 for hashing
- **Collision resistance**: 256-bit output provides 128-bit collision resistance
- **Not used as password hash**: BLAKE3 is used only for key derivation from already-strong keys (master key output from Argon2id)

---

## 4. HKDF-SHA256 Key Expansion

### Algorithm

HKDF as specified in RFC 5869, using SHA-256 as the HMAC hash function.

### Parameters

| Parameter | Value |
|---|---|
| Hash function | SHA-256 |
| Output length | 32 bytes |
| Salt | Application-specific (workspace UUID, session ID, etc.) |
| Info | Purpose-binding string |

### Derivation

```
prk = HMAC-SHA256(salt, IKM)
okm = HMAC-SHA256(prk, info || 0x01)
```

### Implementation

```go
// grimdb/crypto/hkdf.go
import (
    "crypto/sha256"
    "golang.org/x/crypto/hkdf"
)

func deriveKey(ikm []byte, salt []byte, info []byte) ([]byte, error) {
    reader := hkdf.New(sha256.New, ikm, salt, info)
    key := make([]byte, 32)
    _, err := io.ReadFull(reader, key)
    return key, err
}
```

### Usage Contexts

| Context | Salt | Info |
|---|---|---|
| Workspace key | `"grimlocker-workspace"` | Workspace UUID (16 bytes) |
| Session key | `"grimlocker-session"` | Session ID (16 bytes) |
| Audit HMAC key | `"grimlocker-audit"` | Vault UUID (16 bytes) |

---

## 5. Entropy & Coordinate System

### Entropy File

- Generated via CSPRNG (`/dev/urandom` or equivalent)
- Length: 200+ bytes
- Format: base64-encoded, displayed as character matrix
- Never stored as plaintext after key derivation

### Coordinate Extraction

```
1. User provides coordinate set: e.g., "A3,F12,B7,C5"
2. System parses positions: [(row=0, col=3), (row=5, col=12), (row=1, col=7), ...]
3. Extracts bytes at each position from entropy matrix
4. Concatenates extracted bytes
5. BLAKE3(extracted_bytes) → intermediate
6. HKDF-SHA256(intermediate, "coord-override", vault_uuid) → override key
```

### Panic-Key Detection

```
if coordinates == "0,0,0":
    trigger_disguised_wipe()
    return fake_key()  // Return fake key to maintain deception
```

---

## 6. Random Number Generation

### Sources

| Platform | Source |
|---|---|
| Rust | `rand::rngs::OsRng` → `/dev/urandom` (Unix) / `BCryptGenRandom` (Windows) |
| Go | `crypto/rand` → `/dev/urandom` (Unix) / `CNG` (Windows) |

### Usage

| Purpose | Size | Frequency |
|---|---|---|
| Salt for Argon2id | 32 bytes | Once per vault creation |
| ChaCha20 nonce | 12 bytes | Per encryption operation |
| Entropy file | 200+ bytes | Once per vault creation |
| WebSocket auth token | 32 bytes | Per daemon startup |
| Session ID | 16 bytes | Per session |
| UUID generation | 16 bytes | Per entity creation |

---

## 7. Audit Log Hashing

### Chain Construction

```
Entry_0: hash_0 = SHA-256(0x00*32 || timestamp_0 || level_0 || module_0 || message_0 || subjectID_0)
Entry_1: hash_1 = SHA-256(hash_0   || timestamp_1 || level_1 || module_1 || message_1 || subjectID_1)
Entry_N: hash_N = SHA-256(hash_N-1 || timestamp_N || level_N || module_N || message_N || subjectID_N)
```

### Validation

```
for i in 1..N:
    recomputed = SHA-256(entry[i-1].hash || entry[i].timestamp || entry[i].level || entry[i].module || entry[i].message || entry[i].subjectID)
    if recomputed != entry[i].hash:
        return TAMPER_DETECTED
return VALID
```

---

## 8. Constant-Time Operations

### Password/Coordinate Verification

All password and coordinate verifications use constant-time comparison:

```go
// grimdb/security/constant_time.go
import "crypto/subtle"

func ConstantTimeCompare(a, b []byte) bool {
    return subtle.ConstantTimeCompare(a, b) == 1
}
```

```rust
// core-rust/src/lib.rs
use subtle::ConstantTimeEq;

fn constant_time_compare(a: &[u8], b: &[u8]) -> bool {
    a.ct_eq(b).into()
}
```

### Principles

1. No early exit on mismatch — always compare all bytes
2. No branching based on comparison result (use `bool` conversion after)
3. Use `crypto/subtle` (Go) and `subtle` crate (Rust) — well-audited libraries

---

## 9. Parameter Justification

### Why Argon2id (not bcrypt/scrypt/PBKDF2)?

| Algorithm | Memory-hard | GPU-resistant | Standard |
|---|---|---|---|
| PBKDF2 | No | No | NIST SP 800-132 |
| bcrypt | Minimal (4 KiB) | Partial | — |
| scrypt | Yes (configurable) | Yes | RFC 7914 |
| **Argon2id** | **Yes (configurable)** | **Yes** | **RFC 9106** |

Argon2id provides both memory-hardness (side-channel resistance of Argon2i) and time-hardness (GPU resistance of Argon2d).

### Why ChaCha20-Poly1305 (not AES-GCM)?

| Algorithm | Software speed | Constant-time | Hardware acceleration |
|---|---|---|---|
| AES-GCM | Requires AES-NI | Complicated | Common (x86, ARM) |
| **ChaCha20-Poly1305** | **Fast in software** | **Inherently constant-time** | **None required** |

ChaCha20-Poly1305 is naturally constant-time in software, reducing the risk of side-channel vulnerabilities on hardware without AES-NI.

### Why BLAKE3 (not SHA-256) for key derivation?

| Algorithm | Speed (single core) | Parallelism | Output size |
|---|---|---|---|
| SHA-256 | Baseline | No | 256-bit |
| **BLAKE3** | **~10× faster** | **Inherently parallel** | **Arbitrary** |

For key derivation (not password hashing), BLAKE3's speed is an advantage. It is not used where memory-hardness is required.

### Why 32 MiB Argon2id memory?

- 32 MiB × 3 iterations = 96 MiB-seconds per hash
- GPU with 8 GB can run ~85 parallel hashes (vs. millions for non-memory-hard hashes)
- Acceptable login delay (~1-2 seconds on modern hardware)
- Configurable for enterprise tier (up to 256 MiB)
