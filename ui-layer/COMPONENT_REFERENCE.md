# UI Component Reference

Documentation for the React components and Tauri commands in `ui-layer/src/`.

---

## React Components

### `ConfirmDialog`

**Path:** `src/components/ConfirmDialog.tsx` (or similar)

A modal confirmation dialog used for destructive operations (delete workspace, trigger wipe, panic button).

**Props:**

| Prop | Type | Description |
|---|---|---|
| `open` | `boolean` | Controls visibility |
| `title` | `string` | Dialog heading |
| `message` | `string` | Body text |
| `confirmLabel` | `string` | Confirm button label (default: `"Confirm"`) |
| `destructive` | `boolean` | If true, confirm button renders red |
| `onConfirm` | `() => void` | Called when user clicks confirm |
| `onCancel` | `() => void` | Called when user dismisses |

**Usage:**

```tsx
<ConfirmDialog
  open={showDelete}
  title="Delete Workspace"
  message="All entries in this workspace will be permanently deleted."
  confirmLabel="Delete"
  destructive
  onConfirm={() => deleteWorkspace(id)}
  onCancel={() => setShowDelete(false)}
/>
```

---

### `FileVaultViewer`

**Path:** `src/components/FileVaultViewer.tsx`

Displays file vault entries and handles the encrypted file download → open flow.

**Responsibilities:**
1. Renders file metadata (name, size, MIME type, upload date)
2. Sends `MsgFileDownloadRequest` (0x41) via IPC when user clicks "Open"
3. Receives streamed `MsgFileChunkData` (0x42) frames, assembles in memory
4. Verifies SHA-256 on `MsgFileDownloadEnd` (0x43)
5. Calls Tauri command `save_temp_file` to write to OS temp dir
6. Calls Tauri command `open_with_default_app` to launch the system viewer
7. Schedules `secure_delete_temp` after a configurable delay

**Props:**

| Prop | Type | Description |
|---|---|---|
| `entries` | `FileEntry[]` | List of file vault entries to display |
| `namespace` | `string` | Active workspace namespace |
| `onError` | `(msg: string) => void` | Error callback |

**Download state machine:**

```
idle → downloading → assembling → verifying → opening → cleanup_pending → idle
                                     ↓ (hash mismatch)
                                   error
```

---

### `WorkspaceSwitcher`

**Path:** `src/components/WorkspaceSwitcher.tsx`

Dropdown/sidebar component for listing and switching workspaces.

**Props:**

| Prop | Type | Description |
|---|---|---|
| `workspaces` | `Workspace[]` | All available workspaces |
| `activeId` | `string` | Currently active workspace ID |
| `onSwitch` | `(id: string) => void` | Called when user selects a workspace |
| `onCreate` | `(name: string) => void` | Called when user creates a new workspace |
| `onRename` | `(id: string, name: string) => void` | Called on rename |
| `onDelete` | `(id: string) => void` | Called on delete (shows `ConfirmDialog`) |

**Behavior:**
- The default workspace shows a lock icon and disables the delete button
- Rename is done inline (double-click or edit icon)
- Create opens a popover with a text input

---

### `PanicButton`

**Path:** `src/components/PanicButton.tsx`

Admin-only component. Renders a hidden trigger (e.g., long-press gesture or keyboard shortcut) that opens a two-step confirmation flow before sending `MsgPanicButton` (0x45).

**Step 1:** Confirm intent — "This will destroy all vault data. Are you sure?"

**Step 2:** Passphrase entry — admin must type their passphrase to authorize.

The passphrase is sent to the daemon via IPC and verified server-side. The component never evaluates the passphrase locally.

**Props:**

| Prop | Type | Description |
|---|---|---|
| `onActivated` | `() => void` | Called after the daemon confirms wipe initiated |
| `onError` | `(msg: string) => void` | Error callback |

---

## Tauri Commands

Defined in `ui-layer/src-tauri/src/main.rs`. Invoked from React via `invoke()`.

### `get_session_token`

Returns the current daemon session config.

```ts
const config = await invoke<{ token: string; ipc_port: number }>('get_session_token')
```

**Returns:** `{ token: string, ipc_port: number }` or throws `"daemon_not_ready"` if the daemon hasn't started yet.

---

### `save_temp_file`

Writes binary data to the OS temp directory under a `grimlocker_tmp_` prefix.

```ts
const path = await invoke<string>('save_temp_file', {
  filename: 'report.pdf',
  data: Array.from(fileBytes),  // Uint8Array → number[]
})
```

**Returns:** Absolute path of the created file.

**Security:** Filename is sanitized — path separators (`/`, `\`, `:`) are replaced with `_`.

---

### `open_with_default_app`

Opens a file with the OS default application. Uses `cmd /C start` on Windows, `open` on macOS, `xdg-open` on Linux.

```ts
await invoke('open_with_default_app', { path: tempFilePath })
```

---

### `secure_delete_temp`

3-pass overwrite (`0x00`, `0x55`, `0xAA`) followed by deletion.

```ts
await invoke('secure_delete_temp', { path: tempFilePath })
```

**Safety:** Only deletes files in the OS temp directory. Rejects any path outside `$TEMP`.

---

### `rust_secure_wipe`

Requests a secure wipe of an arbitrary path via the `grimlocker-core` Rust library.

```ts
await invoke<string>('rust_secure_wipe', { path: '/path/to/file' })
```

---

### `rust_get_version`

Returns the `grimlocker-desktop` Cargo package version string.

```ts
const version = await invoke<string>('rust_get_version')
```
