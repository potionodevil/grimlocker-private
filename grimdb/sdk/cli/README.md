# grim — Grimlocker CLI

A standalone Go binary that wraps the full Grimlocker API as a CLI.

## Build

```bash
cd sdk/cli
go build -o grim .
```

## Environment

| Variable | Default | Description |
|---|---|---|
| `GRIMLOCKER_URL` | `http://127.0.0.1:36353` | API base URL |
| `GRIMLOCKER_TOKEN` | (none) | Authentication token |

## Global Flags

- `--pretty` — Human-readable formatted output (tables, indentation)
- `--silent` — Suppress all output; only exit code matters

## Usage Examples

### Vault Operations

```bash
# Unlock the vault
grim unlock --password "my-master-password"

# Check vault status
grim status --pretty

# Lock the vault
grim lock
```

### Entry Management

```bash
# List all entries
grim entries list --pretty

# List only passwords
grim entries list --category PASSWORD --pretty

# Get a specific entry
grim entries get abc12345

# Create a password entry
grim entries create password \
  --title "GitHub" \
  --username "alice@example.com" \
  --password "s3cr3t!" \
  --url "https://github.com" \
  --notes "Personal account"

# Create an SSH key entry
grim entries create ssh-key \
  --title "Dev Server" \
  --public-key "ssh-ed25519 AAAAC3NzaC..." \
  --username "admin"

# Create a certificate entry
grim entries create certificate \
  --title "example.com" \
  --domain "example.com" \
  --cert "-----BEGIN CERTIFICATE-----..." \
  --private-key "-----BEGIN PRIVATE KEY-----..."

# Update an entry
grim entries update abc12345 --field username=newuser --field url=https://new.example.com

# Delete an entry
grim entries delete abc12345

# Search entries
grim entries search "github" --category PASSWORD --pretty
```

### File Operations

```bash
# Upload a file
grim files upload ./document.pdf

# Upload to a specific folder
grim files upload ./secret.txt --folder my-folder-id

# List folder contents
grim files list-folder --pretty

# Create a folder
grim files create-folder "Documents"

# Rename a folder
grim files rename-folder my-folder-id "New Name"

# Delete a folder
grim files delete-folder my-folder-id

# Move a file to a folder
grim files move manifest-block-id folder-id

# Download a file
grim files download manifest-block-id --output ./downloaded.pdf
```

### Workspace Management

```bash
# List workspaces
grim workspaces list --pretty

# Create a workspace
grim workspaces create "Work"

# Switch to a workspace
grim workspaces switch workspace-id

# Rename a workspace
grim workspaces rename workspace-id "Personal"

# Delete a workspace
grim workspaces delete workspace-id
```

### Sync

```bash
# Check sync status
grim sync status --pretty

# Trigger a sync
grim sync trigger
```

### Audit & Health

```bash
# View audit log
grim audit --n 100 --pretty

# Check daemon health
grim health --pretty
```

### SSH Key Generation

```bash
# Generate a new Ed25519 key pair
grim ssh-keygen --comment "alice@laptop"

# Generate and save to vault
grim ssh-keygen --comment "server-deploy" --save-to-vault

# Machine-only output
grim ssh-keygen --comment "ci-key" --silent
```

## Exit Codes

- `0` — Success
- `1` — Error (authentication failure, missing argument, API error, etc.)

## Output Formats

- **Default** — Compact JSON (one line)
- **`--pretty`** — Human-readable tables and indented JSON
- **`--silent`** — No output; only exit code matters

## API Protocol

The CLI POSTs JSON to `GRIMLOCKER_URL/api/v1` with:

```json
{
  "action": "entry.create",
  "payload": { "title": "Example", "fields": {} }
}
```

Headers:
- `Content-Type: application/json`
- `X-Grimlocker-Token: <token>`
