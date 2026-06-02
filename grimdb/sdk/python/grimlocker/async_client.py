"""
Grimlocker Python SDK — asynchronous client for the GQL binary protocol.

Usage:
    import asyncio
    from grimlocker.async_client import AsyncClient

    async def main():
        client = await AsyncClient.connect("127.0.0.1", 41753, token)
        try:
            await client.unlock_vault("master-password")
            entries = await client.list_entries()
            for e in entries:
                print(e.title)
        finally:
            await client.close()

    asyncio.run(main())
"""

import asyncio
import base64
import json
from typing import Callable, Dict, List, Optional

import websockets

from grimlocker._internal import errors as _errors
from grimlocker._internal import frame as _frame
from grimlocker.client import GrimlockerError
from grimlocker.entries import CertificateEntry, Entry, PasswordEntry, SSHKeyEntry
from grimlocker.files import FileEntry, FolderItem, FolderListing, UploadProgress


class AsyncClient:
    """
    Asynchronous Grimlocker GQL client (asyncio / websockets).

    Usage:

        client = await AsyncClient.connect("127.0.0.1", 41753, token)
        try:
            await client.unlock_vault("master-password")
            entries = await client.list_entries()
        finally:
            await client.close()
    """

    def __init__(self, conn, timeout: float = 30.0):
        self._conn = conn
        self._timeout = timeout
        self._lock = asyncio.Lock()

    @classmethod
    async def connect(cls, host: str, port: int, token: str, timeout: float = 30.0) -> "AsyncClient":
        """Connect to the Grimlocker daemon and return a ready-to-use async client."""
        uri = f"ws://{host}:{port}/ws?token={token}"
        try:
            conn = await websockets.connect(uri, open_timeout=10)
            # Consume the INIT.READY handshake frame
            await conn.recv()
            return cls(conn, timeout)
        except Exception as e:
            raise GrimlockerError(f"connect failed: {e}") from e

    async def close(self) -> None:
        """Close the WebSocket connection."""
        try:
            await self._conn.close()
        except Exception:
            pass

    async def __aenter__(self) -> "AsyncClient":
        return self

    async def __aexit__(self, *_) -> None:
        await self.close()

    # --- Auth ---

    async def unlock_vault(self, password: str) -> None:
        await self._call_raw("unlock_vault", fields={"password": password})

    async def lock_vault(self) -> None:
        await self._call_raw("lock_vault")

    async def vault_status(self) -> dict:
        return await self._call_raw("vault_status")

    # --- High-level operations ---

    async def list_entries(self, namespace: str = "default", limit: int = 50, offset: int = 0) -> List[Entry]:
        return await self._execute("list_entries", namespace=namespace, limit=limit, offset=offset)

    async def get_entry(self, entry_id: str, namespace: str = "default") -> Entry:
        results = await self._execute("get_entry", namespace=namespace, entry_id=entry_id)
        if not results:
            raise GrimlockerError(f"entry not found: {entry_id}", error_code=-11)
        return results[0]

    async def query_entries(self, category: str, namespace: str = "default",
                            limit: int = 50, offset: int = 0) -> List[Entry]:
        return await self._execute("query_entries", namespace=namespace,
                                   category=category, limit=limit, offset=offset)

    async def create_entry(self, title: str, category: str, fields: Dict[str, str],
                           namespace: str = "default") -> Entry:
        results = await self._execute("create_entry", namespace=namespace,
                                      title=title, category=category, fields=fields)
        if not results:
            raise GrimlockerError("create returned no entry", error_code=-30)
        return results[0]

    async def update_entry(self, entry_id: str, title: str, fields: Dict[str, str],
                           namespace: str = "default") -> None:
        await self._execute("update_entry", namespace=namespace,
                            entry_id=entry_id, title=title, fields=fields)

    async def delete_entry(self, entry_id: str, namespace: str = "default") -> None:
        await self._execute("delete_entry", namespace=namespace, entry_id=entry_id)

    # --- Semantic helpers ---

    async def list_passwords(self, namespace: str = "default") -> List[PasswordEntry]:
        entries = await self.query_entries("PASSWORD", namespace=namespace)
        return [PasswordEntry.from_entry(e) for e in entries]

    async def get_password(self, entry_id: str, namespace: str = "default") -> PasswordEntry:
        e = await self.get_entry(entry_id, namespace=namespace)
        if e.category != "PASSWORD":
            raise GrimlockerError(f"entry {entry_id} is category {e.category!r}, not PASSWORD")
        return PasswordEntry.from_entry(e)

    async def create_password(self, p: PasswordEntry, namespace: str = "default") -> str:
        entry = await self.create_entry(p.title, "PASSWORD", p.to_fields(), namespace=namespace)
        return entry.id

    async def list_ssh_keys(self, namespace: str = "default") -> List[SSHKeyEntry]:
        entries = await self.query_entries("SSH_KEY", namespace=namespace)
        return [SSHKeyEntry.from_entry(e) for e in entries]

    async def create_ssh_key(self, k: SSHKeyEntry, namespace: str = "default") -> str:
        entry = await self.create_entry(k.title, "SSH_KEY", k.to_fields(), namespace=namespace)
        return entry.id

    # --- Certificates ---

    async def list_certificates(self, namespace: str = "default") -> List[CertificateEntry]:
        entries = await self.query_entries("CERTIFICATE", namespace=namespace)
        return [CertificateEntry.from_entry(e) for e in entries]

    async def create_certificate(self, c: CertificateEntry, namespace: str = "default") -> str:
        entry = await self.create_entry(c.title, "CERTIFICATE", c.to_fields(), namespace=namespace)
        return entry.id

    # --- Search ---

    async def search_entries(self, query: str, category: str = "", namespace: str = "default") -> List[Entry]:
        payload = await self._call_raw("search_entries", namespace=namespace, title=query, category=category)
        return [Entry.from_dict(e) for e in (payload.get("entries") or [])]

    # --- File Vault ---

    async def list_folder(self, folder_id: str = "") -> FolderListing:
        payload = await self._call_raw("list_folder", entry_id=folder_id)
        folders = [FolderItem(id=f.get("id", ""), name=f.get("name", ""), type=f.get("type", ""))
                   for f in (payload.get("folders") or [])]
        files = [FileEntry(id=f.get("id", ""), file_name=f.get("file_name", ""),
                           mime_type=f.get("mime_type", ""), total_size=f.get("total_size", 0),
                           manifest_block_id=f.get("manifest_block_id", ""),
                           folder_id=f.get("folder_id", ""))
                 for f in (payload.get("files") or [])]
        return FolderListing(folders=folders, files=files)

    async def create_folder(self, name: str, parent_id: str = "") -> FolderItem:
        flds = {}
        if parent_id:
            flds["parent_id"] = parent_id
        payload = await self._call_raw("create_folder", title=name, fields=flds)
        return FolderItem(id=payload.get("id", ""), name=payload.get("name", name), type="folder")

    async def rename_folder(self, id: str, name: str) -> None:
        await self._call_raw("rename_folder", entry_id=id, title=name)

    async def delete_folder(self, id: str) -> None:
        await self._call_raw("delete_folder", entry_id=id)

    async def move_file(self, manifest_block_id: str, folder_id: str) -> None:
        await self._call_raw("move_file", entry_id=manifest_block_id, fields={"folder_id": folder_id})

    async def upload_file(self, data: bytes, filename: str, mime_type: str = "application/octet-stream",
                          folder_id: str = "", on_progress: Optional[Callable] = None) -> FileEntry:
        total = len(data)
        if on_progress:
            if asyncio.iscoroutinefunction(on_progress):
                await on_progress(UploadProgress(bytes_sent=0, total_bytes=total))
            else:
                on_progress(UploadProgress(bytes_sent=0, total_bytes=total))
        b64 = base64.b64encode(data).decode()
        flds = {"data_b64": b64, "file_name": filename, "mime_type": mime_type}
        if folder_id:
            flds["folder_id"] = folder_id
        payload = await self._call_raw("file_ingest", fields=flds)
        if on_progress:
            progress = UploadProgress(bytes_sent=total, total_bytes=total)
            if asyncio.iscoroutinefunction(on_progress):
                await on_progress(progress)
            else:
                on_progress(progress)
        return FileEntry(
            id=payload.get("id", ""), file_name=payload.get("file_name", ""),
            mime_type=payload.get("mime_type", ""), total_size=payload.get("total_size", 0),
            manifest_block_id=payload.get("manifest_block_id", ""),
            folder_id=payload.get("folder_id", ""),
        )

    async def download_file(self, manifest_block_id: str) -> bytes:
        payload = await self._call_raw("file_download", entry_id=manifest_block_id)
        return base64.b64decode(payload.get("data_b64", ""))

    # --- Workspaces ---

    async def list_workspaces(self) -> List[dict]:
        payload = await self._call_raw("list_workspaces")
        return payload.get("workspaces") or []

    async def create_workspace(self, name: str) -> dict:
        return await self._call_raw("create_workspace", title=name)

    async def switch_workspace(self, id: str) -> None:
        await self._call_raw("switch_workspace", entry_id=id)

    async def rename_workspace(self, id: str, name: str) -> None:
        await self._call_raw("rename_workspace", entry_id=id, title=name)

    async def delete_workspace(self, id: str) -> None:
        await self._call_raw("delete_workspace", entry_id=id)

    # --- Sync + Audit ---

    async def list_sync_peers(self) -> dict:
        return await self._call_raw("list_sync_peers")

    async def trigger_sync(self) -> None:
        await self._call_raw("trigger_sync")

    async def list_audit_events(self, n: int = 50) -> List[dict]:
        payload = await self._call_raw("list_audit_events", limit=n)
        return payload.get("events") or []

    # --- Health + Tools ---

    async def health_check(self) -> dict:
        return await self._call_raw("health_check")

    async def get_recovery_phrase(self, password: str) -> str:
        payload = await self._call_raw("get_recovery_phrase", fields={"password": password})
        return payload.get("recovery_phrase", "") or payload.get("phrase", "")

    async def generate_ssh_key(self, comment: str = "", save_to_vault: bool = True) -> dict:
        return await self._call_raw("generate_ssh_key", fields={
            "comment": comment,
            "save_to_vault": str(save_to_vault).lower(),
        })

    # --- Internal ---

    async def _call_raw(
        self,
        operation: str,
        namespace: str = "default",
        entry_id: str = "",
        category: str = "",
        title: str = "",
        fields: Optional[Dict[str, str]] = None,
        limit: int = 50,
        offset: int = 0,
    ) -> dict:
        """Send a GQL frame and return the raw JSON result dict."""
        wire = _frame.encode_query(
            operation, namespace, entry_id, category, title,
            fields or {}, limit, offset,
        )
        async with self._lock:
            await self._conn.send(wire)
            raw = await self._conn.recv()

        if isinstance(raw, str):
            data = raw.encode()
        else:
            data = raw

        opcode  = _frame.read_opcode(data)
        payload = json.loads(_frame.read_payload(data))

        if opcode == _frame.OPCODE_ERROR or not payload.get("success", False):
            raise GrimlockerError._from_result(payload)

        return payload

    async def _execute(
        self,
        operation: str,
        namespace: str = "default",
        entry_id: str = "",
        category: str = "",
        title: str = "",
        fields: Optional[Dict[str, str]] = None,
        limit: int = 50,
        offset: int = 0,
    ) -> List[Entry]:
        payload = await self._call_raw(operation, namespace=namespace, entry_id=entry_id,
                                       category=category, title=title, fields=fields,
                                       limit=limit, offset=offset)
        return [Entry.from_dict(e) for e in (payload.get("entries") or [])]
