# grimlocker-sdk (Rust)

Rust SDK for the Grimlocker Zero-Trust Vault daemon. Uses the GQL binary WebSocket protocol for maximum performance and type safety.

## Installation

```toml
[dependencies]
grimlocker-sdk = "1.0"
tokio = { version = "1", features = ["full"] }
```

## Quick Start

```rust
use grimlocker_sdk::{GrimlockerClient, PasswordEntry};

#[tokio::main]
async fn main() -> Result<(), grimlocker_sdk::Error> {
    let url   = "ws://127.0.0.1:36352/ws?token=TOKEN";
    let mut c = GrimlockerClient::connect(url, "TOKEN").await?;
    c.unlock_vault("master-password").await?;

    // Passwords
    let passwords = c.list_passwords().await?;
    let id = c.create_password(&PasswordEntry {
        title:    "GitHub".into(),
        username: "me@example.com".into(),
        password: "s3cr3t".into(),
        url:      "https://github.com".into(),
        ..Default::default()
    }).await?;

    // File vault
    let listing = c.list_folder(None).await?;
    c.upload_file(b"hello world", "note.txt", "text/plain", None, |p| {
        println!("Upload: {:.0}%", p.percent())
    }).await?;

    // Sync + Audit
    let sync   = c.list_sync_peers().await?;
    let events = c.list_audit_events(20).await?;

    // Workspaces
    let workspaces = c.list_workspaces().await?;

    c.close().await;
    Ok(())
}
```

## Features

| Category | Methods |
|---|---|
| Auth | `unlock_vault`, `lock_vault`, `vault_status` |
| Entries | `list_entries`, `get_entry`, `create_entry`, `update_entry`, `delete_entry`, `search_entries` |
| Passwords | `list_passwords`, `create_password` |
| SSH Keys | `list_ssh_keys`, `create_ssh_key` |
| Certificates | `list_certificates`, `create_certificate` |
| File Vault | `list_folder`, `create_folder`, `rename_folder`, `delete_folder`, `move_file`, `upload_file`, `download_file` |
| Workspaces | `list_workspaces`, `create_workspace`, `switch_workspace`, `rename_workspace`, `delete_workspace` |
| Sync | `list_sync_peers`, `trigger_sync` |
| Audit | `list_audit_events` |
| Health | `health_check` |
