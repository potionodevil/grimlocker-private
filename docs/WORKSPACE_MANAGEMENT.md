# Workspace Management

Workspaces are isolated namespaces within a single vault. Each workspace has its own encrypted storage partition and can be independently accessed, renamed, or deleted.

---

## Workspace Lifecycle

### Create

```
Client → 0x28 MsgWorkspaceCreate  { "name": "Work" }
Server → 0x08 MsgAck
```

A workspace is created with a randomly generated ID (UUID v4). The name is display-only and may be changed at any time.

### List

```
Client → 0x27 MsgWorkspaceList
Server → 0x2B MsgWorkspacesResult  [{ "id": "...", "name": "...", "is_default": false }, ...]
```

### Switch

```
Client → 0x29 MsgWorkspaceSwitch  { "id": "abc-123" }
Server → 0x08 MsgAck
```

After switching, all GQL queries use the new workspace's namespace unless an explicit `namespace` is provided.

### Rename

```
Client → 0x44 MsgWorkspaceRename  { "id": "abc-123", "name": "Personal" }
Server → 0x08 MsgAck
```

### Delete

```
Client → 0x2A MsgWorkspaceDelete  { "id": "abc-123" }
Server → 0x08 MsgAck   (or MsgError if protected)
```

---

## Default Workspace Protection

The `default` workspace cannot be deleted or renamed to an empty string. Any attempt returns:

```json
{ "error": "default workspace is protected" }
```

This ensures there is always at least one valid namespace for GQL operations.

---

## GQL Namespace Mapping

Workspace IDs map directly to GQL namespace strings. When creating entries:

```go
// SDK — explicit namespace
client.CreatePassword(ctx, "work-abc-123", &sdk.PasswordEntry{...})

// SDK — default namespace
client.CreatePassword(ctx, "default", &sdk.PasswordEntry{...})
```

The GQL validator rejects namespace strings that don't match `[a-zA-Z0-9_\-\.]+`, preventing namespace injection.

---

## Multi-User (Enterprise)

In Enterprise mode, each user has their own namespace derived from their user ID. Admins can access any namespace; regular users are restricted to their own.

ACL check sequence:

```
1. Token → session → user ID + roles
2. GQL query.Namespace resolved against user's allowed namespaces
3. If namespace not in allowed list → GQL error -103 (ACL_DENIED)
```

Shared workspaces (future feature) would be granted via role-based namespace ACL entries.

---

## State Mirror

On reconnect or session resume, the daemon sends `MsgStateMirror` (0x3A) which includes the full workspace list and the currently active workspace ID. The frontend reconstructs its state from this single message.

```json
{
  "unlocked": true,
  "active_workspace": { "id": "default", "name": "Default" },
  "workspaces": [
    { "id": "default", "name": "Default", "is_default": true },
    { "id": "work-abc", "name": "Work", "is_default": false }
  ],
  "ske_handle": "...",
  "entries": [...]
}
```
