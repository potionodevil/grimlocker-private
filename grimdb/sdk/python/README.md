# Grimlocker Python SDK

High-level Python client for the Grimlocker vault daemon. Connects via WebSocket using the GQL binary protocol — injection-immune by design.

## Installation

```bash
pip install grimlocker
# or from source:
pip install -e grimdb/sdk/python
```

## Quick Start (5 minutes)

**List all entries:**

```python
from grimlocker import Client

with Client.connect("127.0.0.1", 41753, token) as client:
    entries = client.list_entries(namespace="default", limit=20)
    for e in entries:
        print(f"{e.title} [{e.category}]")
```

**Create a password entry:**

```python
from grimlocker import Client, PasswordEntry

with Client.connect("127.0.0.1", 41753, token) as client:
    entry_id = client.create_password(PasswordEntry(
        title="GitHub",
        username="alice",
        password="s3cr3t",
        url="https://github.com",
    ))
    print(f"Created: {entry_id}")
```

**Get the token:** The daemon prints `GRIMLOCKER_TOKEN=<token>` to stdout on startup.

## Operations

| Method | Description |
|---|---|
| `client.list_entries(namespace, limit, offset)` | List all entries |
| `client.get_entry(id, namespace)` | Fetch one entry by ID |
| `client.query_entries(category, namespace)` | Filter by category |
| `client.create_entry(title, category, fields, namespace)` | Create a generic entry |
| `client.update_entry(id, title, fields, namespace)` | Update an entry |
| `client.delete_entry(id, namespace)` | Delete an entry |
| `client.list_passwords(namespace)` | List `PasswordEntry` objects |
| `client.get_password(id, namespace)` | Get one `PasswordEntry` |
| `client.create_password(p, namespace)` | Create a `PasswordEntry`, returns ID |
| `client.list_ssh_keys(namespace)` | List `SSHKeyEntry` objects |
| `client.create_ssh_key(k, namespace)` | Create an `SSHKeyEntry`, returns ID |

## Error Handling

```python
from grimlocker import Client, GrimlockerError

with Client.connect("127.0.0.1", 41753, token) as client:
    try:
        entry = client.get_entry("missing-id")
    except GrimlockerError as e:
        if e.error_code == -11:
            print("Entry not found")
        else:
            raise
```

## Security Notes

- Binary wire protocol — no SQL, no JSON injection possible.
- TLS/mTLS enforced in Enterprise mode; use `wss://` for production.
- Never log the session token.
