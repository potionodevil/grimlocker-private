# @grimlocker/sdk

TypeScript/JavaScript SDK for the Grimlocker Zero-Trust Vault daemon.

Works in **Node.js**, **browsers** (via Vite/webpack), and any environment with the Fetch API.

## Installation

```bash
npm install @grimlocker/sdk
```

## Quick Start

```typescript
import { GrimlockerClient } from '@grimlocker/sdk'

// The token is printed by the daemon at startup: GRIMLOCKER_TOKEN=...
// The port is printed as: GRIMLOCKER_UI=http://127.0.0.1:PORT
const client = new GrimlockerClient('http://127.0.0.1:36353', process.env.GRIMLOCKER_TOKEN!)

// Unlock the vault
await client.unlockVault('your-master-password')

// List entries
const entries = await client.listEntries()
console.log(entries)

// Create a password
const entry = await client.createPassword(
  'GitHub',
  'me@example.com',
  's3cr3t!',
  'https://github.com'
)

// Audit log
const events = await client.listAuditLog(20)

// LAN sync status
const sync = await client.listSyncPeers()
console.log(`${sync.peers.length} peer(s) found, device: ${sync.device_id}`)

// Lock
await client.lockVault()
```

## Error Handling

```typescript
try {
  await client.unlockVault('wrong-password')
} catch (err) {
  // err.message, err.action, err.status_code
  console.error(err.message)
}
```

## Enterprise (mTLS)

For Enterprise deployments, pass a custom `fetch` implementation configured with
client certificates:

```typescript
import { GrimlockerClient } from '@grimlocker/sdk'
import { createTLSFetch } from './your-tls-fetch-wrapper'

const client = new GrimlockerClient(
  'https://vault.company.internal:9443',
  token,
  { fetch: createTLSFetch({ cert, key, ca }) }
)
```

## Supported Languages

| Language | Package | Status |
|----------|---------|--------|
| TypeScript / JavaScript | `@grimlocker/sdk` | ✅ This package |
| Go | `github.com/grimlocker/grimdb/sdk` | ✅ Available |
| Python | `pip install grimlocker` | ✅ Available |
| Java | `com.grimlocker:grimlocker-sdk` | ✅ Available |
