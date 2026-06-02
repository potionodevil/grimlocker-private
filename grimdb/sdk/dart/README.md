# Grimlocker Dart SDK

Dart/Flutter SDK for the [Grimlocker](https://github.com/anomalyco/grimlocker) Zero-Trust Vault daemon.

Wraps the `/api/v1` JSON HTTP endpoint exposed by the daemon on localhost.

## Quick start

```dart
import 'package:grimlocker_sdk/grimlocker_sdk.dart';

Future<void> main() async {
  final client = GrimlockerClient('http://127.0.0.1:36353', token);

  await client.unlockVault('master-password');

  final passwords = await client.listPasswords();
  for (final p in passwords) {
    print('${p.title}: ${p.username}');
  }

  await client.createPassword(PasswordEntry(
    title: 'My App',
    username: 'user',
    password: 'secret',
    url: 'https://example.com',
  ));

  client.close();
}
```

## API

All methods return `Future` — use with `async`/`await`.

### Auth
- `unlockVault(password)` — unlock with master password
- `lockVault()` — lock the vault
- `vaultStatus()` — returns `{initialized, unlocked, status}`

### Entries
- `listEntries({category})` — list entries, optionally filtered by category
- `getEntry(id)` — read a single entry
- `createEntry(title, category, fields)` — create an entry
- `updateEntry(id, fields)` — update fields
- `deleteEntry(id)` — delete an entry
- `searchEntries(query, {category})` — full-text search

### Typed helpers
- `listPasswords()` / `createPassword(PasswordEntry)`
- `listSshKeys()` / `createSshKey(SshKeyEntry)`
- `listCertificates()` / `createCertificate(CertificateEntry)`

### File Vault
- `listFolder({folderId})` — list folder contents
- `createFolder(name, {parentId})` — create a folder
- `renameFolder(id, name)` / `deleteFolder(id)`
- `moveFile(manifestBlockId, folderId)`
- `uploadFile(data, fileName, {mimeType, folderId, onProgress})`
- `downloadFile(manifestBlockId)`

### Workspaces
- `listWorkspaces()` / `createWorkspace(name)`
- `switchWorkspace(id)` / `renameWorkspace(id, name)` / `deleteWorkspace(id)`

### Sync
- `listSyncPeers()` — returns LAN sync peers and sync status
- `triggerSync()` — trigger an immediate sync cycle

### Audit
- `listAuditEvents({n})` — fetch last n audit log entries (default 50)

### Health
- `healthCheck()` — alias for `vaultStatus()`

## Error handling

All methods throw `GrimlockerException` on daemon errors:

```dart
try {
  await client.getEntry('nonexistent');
} on GrimlockerException catch (e) {
  print('${e.code}: ${e.message}');
}
```

Error codes: `BUS_ERROR` (-1), `STORAGE_ERROR` (-2), `NOT_FOUND` (-3),
`ENTRY_NOT_FOUND` (-10), `CREATE_FAILED` (-30), `AUTH_REQUIRED` (-101),
`PERMISSION_DENIED` (-102), `TIMEOUT` (-104), etc.
