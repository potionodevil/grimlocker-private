"""
Tests for Grimlocker Python SDK — synchronous and async clients.
Uses pytest with unittest.mock to patch WebSocket communication.
"""
import base64
import json
import struct
from unittest.mock import MagicMock, patch

import pytest

from grimlocker import Client, GrimlockerError
from grimlocker.entries import CertificateEntry, Entry, PasswordEntry, SSHKeyEntry
from grimlocker.files import FileEntry, FolderItem, FolderListing, UploadProgress
from grimlocker._internal import frame as _frame


def _make_result_frame(payload: dict, opcode: int = _frame.OPCODE_RESULT) -> bytes:
    data = json.dumps(payload).encode("utf-8")
    header = struct.pack(">IB", 1 + len(data), opcode)
    return header + data


def _make_ok_result(entries_data=None, **kwargs) -> bytes:
    payload = {"success": True, **kwargs}
    if entries_data is not None:
        payload["entries"] = entries_data
    return _make_result_frame(payload)


def _sample_entry_dict(
    id="e1",
    title="Test Entry",
    category="PASSWORD",
    fields=None,
):
    return {
        "id": id,
        "title": title,
        "category": category,
        "fields": fields or {"username": "alice", "password": "s3cret"},
        "created_at": 1,
        "updated_at": 2,
    }


# ── Sync Client Tests ────────────────────────────────────────────────────────


class TestSyncClient:
    @patch("grimlocker.client._ws.connect")
    def test_connect(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.return_value = _make_result_frame({"success": True})
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        assert client is not None
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_connect_failure(self, mock_connect):
        mock_connect.side_effect = OSError("refused")

        with pytest.raises(GrimlockerError, match="connect failed"):
            Client.connect("127.0.0.1", 41753, "token")

    @patch("grimlocker.client._ws.connect")
    def test_list_entries(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(
                entries_data=[
                    _sample_entry_dict("e1", "First"),
                    _sample_entry_dict("e2", "Second", "SSH_KEY"),
                ]
            ),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        entries = client.list_entries("default", limit=10)
        assert len(entries) == 2
        assert entries[0].id == "e1"
        assert entries[1].category == "SSH_KEY"
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_get_entry(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(entries_data=[_sample_entry_dict("e99")]),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        entry = client.get_entry("e99")
        assert entry.id == "e99"
        assert entry.title == "Test Entry"
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_get_entry_not_found(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(entries_data=[]),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        with pytest.raises(GrimlockerError):
            client.get_entry("nonexistent")
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_create_entry(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(
                entries_data=[_sample_entry_dict("new1", "New Entry")]
            ),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        entry = client.create_entry("New Entry", "PASSWORD", {"username": "bob"})
        assert entry.id == "new1"
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_update_entry(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        client.update_entry("e1", "Updated", {"notes": "new"})
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_delete_entry(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        client.delete_entry("e1")
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_create_password(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(
                entries_data=[
                    _sample_entry_dict(
                        "p1",
                        "GitHub",
                        "PASSWORD",
                        {"username": "alice", "password": "sec", "url": "", "notes": ""},
                    )
                ]
            ),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        p = PasswordEntry(title="GitHub", username="alice", password="sec")
        entry_id = client.create_password(p)
        assert entry_id == "p1"
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_list_passwords(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(
                entries_data=[
                    _sample_entry_dict("p1", "GitHub", "PASSWORD", {"username": "a", "password": "b", "url": "", "notes": ""}),
                    _sample_entry_dict("p2", "GitLab", "PASSWORD", {"username": "c", "password": "d", "url": "", "notes": ""}),
                ]
            ),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        passwords = client.list_passwords()
        assert len(passwords) == 2
        assert isinstance(passwords[0], PasswordEntry)
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_create_ssh_key(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(
                entries_data=[
                    _sample_entry_dict(
                        "sk1", "Key", "SSH_KEY",
                        {"public_key": "pk", "private_key": "prk", "username": "u", "comment": "c"},
                    )
                ]
            ),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        k = SSHKeyEntry(title="Key", public_key="pk", private_key="prk", username="u")
        entry_id = client.create_ssh_key(k)
        assert entry_id == "sk1"
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_list_ssh_keys(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(
                entries_data=[
                    _sample_entry_dict("sk1", "Key", "SSH_KEY", {"public_key": "pk", "private_key": "", "username": "", "comment": ""}),
                ]
            ),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        keys = client.list_ssh_keys()
        assert len(keys) == 1
        assert isinstance(keys[0], SSHKeyEntry)
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_create_certificate(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(
                entries_data=[
                    _sample_entry_dict(
                        "c1", "Cert", "CERTIFICATE",
                        {"domain": "ex.com", "certificate": "crt", "private_key": "key"},
                    )
                ]
            ),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        c = CertificateEntry(title="Cert", domain="ex.com", certificate="crt", private_key="key")
        entry_id = client.create_certificate(c)
        assert entry_id == "c1"
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_list_certificates(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(
                entries_data=[
                    _sample_entry_dict("c1", "Cert", "CERTIFICATE", {"domain": "x", "certificate": "c", "private_key": "k"}),
                ]
            ),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        certs = client.list_certificates()
        assert len(certs) == 1
        assert isinstance(certs[0], CertificateEntry)
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_search_entries(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(
                entries_data=[_sample_entry_dict("e1", "GitHub")],
            ),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        results = client.search_entries("git")
        assert len(results) == 1
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_list_folder(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(folders=[{"id": "d1", "name": "sub", "type": "folder"}], files=[]),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        listing = client.list_folder("")
        assert len(listing.folders) == 1
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_create_folder(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(id="f1", name="Notes"),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        folder = client.create_folder("Notes", "parent1")
        assert folder.name == "Notes"
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_upload_file(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(
                id="f1", file_name="doc.txt", mime_type="text/plain",
                total_size=12, manifest_block_id="mb1", folder_id="",
            ),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        data = b"hello world!"
        result = client.upload_file(data, "doc.txt", folder_id="f1")
        assert result.file_name == "doc.txt"
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_upload_file_with_progress(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(
                id="f1", file_name="doc.txt", mime_type="text/plain",
                total_size=5, manifest_block_id="mb1", folder_id="",
            ),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        progress_calls = []

        def on_progress(p):
            progress_calls.append(p)

        result = client.upload_file(b"hello", "doc.txt", on_progress=on_progress)
        assert len(progress_calls) == 2
        assert progress_calls[0].bytes_sent == 0
        assert progress_calls[1].bytes_sent == 5
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_download_file(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(data_b64=base64.b64encode(b"hello").decode()),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        data = client.download_file("mb1")
        assert data == b"hello"
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_list_workspaces(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(
                workspaces=[{"id": "ws1", "name": "Personal", "is_default": True}],
            ),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        workspaces = client.list_workspaces()
        assert len(workspaces) == 1
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_create_workspace(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(id="ws2", name="Work"),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        ws = client.create_workspace("Work")
        assert ws["name"] == "Work"
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_switch_workspace(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        client.switch_workspace("ws2")
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_list_sync_peers(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(peers=[{"device_id": "d1", "host": "192.168.1.5"}], last_sync_at=0),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        status = client.list_sync_peers()
        assert "peers" in status
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_trigger_sync(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        client.trigger_sync()
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_list_audit_events(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(
                events=[{"timestamp": 1, "level": "INFO", "module": "auth", "message": "unlock"}],
            ),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        events = client.list_audit_events(10)
        assert len(events) == 1
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_health_check(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(status="ok", daemon_version="1.0.0"),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        health = client.health_check()
        assert health["status"] == "ok"
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_generate_ssh_key(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(public_key="ssh-ed25519 AAA", fingerprint="SHA256:abc", entry_id="e1"),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        result = client.generate_ssh_key("test", True)
        assert "public_key" in result
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_get_recovery_phrase(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(recovery_phrase="abandon ability able about above absent..."),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        phrase = client.get_recovery_phrase("master")
        assert len(phrase) > 0
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_error_handling(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_result_frame(
                {"success": False, "error_code": -101, "error_msg": "vault locked"},
                opcode=_frame.OPCODE_ERROR,
            ),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        with pytest.raises(GrimlockerError, match="vault locked"):
            client.list_entries()
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_context_manager(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.return_value = _make_result_frame({"success": True})
        mock_connect.return_value = mock_conn

        with Client.connect("127.0.0.1", 41753, "token") as client:
            assert client is not None
        mock_conn.close.assert_called_once()

    @patch("grimlocker.client._ws.connect")
    def test_move_file(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        client.move_file("mb1", "folder1")
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_rename_folder(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        client.rename_folder("f1", "NewName")
        client.close()

    @patch("grimlocker.client._ws.connect")
    def test_delete_folder(self, mock_connect):
        mock_conn = MagicMock()
        mock_conn.recv.side_effect = [
            _make_result_frame({"success": True}),
            _make_ok_result(),
        ]
        mock_connect.return_value = mock_conn

        client = Client.connect("127.0.0.1", 41753, "token")
        client.delete_folder("f1")
        client.close()


# ── Model Serialization Tests ─────────────────────────────────────────────────


class TestEntryModels:
    def test_entry_from_dict(self):
        d = {"id": "e1", "category": "PASSWORD", "title": "T", "fields": {"k": "v"}, "created_at": 1, "updated_at": 2}
        e = Entry.from_dict(d)
        assert e.id == "e1"
        assert e.fields == {"k": "v"}

    def test_password_entry_roundtrip(self):
        p = PasswordEntry(title="GitHub", username="alice", password="sec", url="https://gh.com", notes="main")
        fields = p.to_fields()
        assert fields["username"] == "alice"

    def test_certificate_entry_from_entry(self):
        e = Entry(id="c1", title="C", category="CERTIFICATE", fields={
            "domain": "ex.com", "certificate": "crt", "private_key": "key",
        })
        c = CertificateEntry.from_entry(e)
        assert c.domain == "ex.com"
        assert c.certificate == "crt"

    def test_file_entry_defaults(self):
        fe = FileEntry(
            id="f1", file_name="a.txt", mime_type="text/plain",
            total_size=100, manifest_block_id="mb1", folder_id="",
        )
        assert fe.file_name == "a.txt"
        assert fe.total_size == 100

    def test_folder_listing(self):
        fl = FolderListing(
            folders=[FolderItem(id="d1", name="sub", type="folder")],
            files=[FileEntry(id="f1", file_name="a.txt", mime_type="text/plain", total_size=0, manifest_block_id="mb1", folder_id="")],
        )
        assert len(fl.folders) == 1
        assert len(fl.files) == 1

    def test_upload_progress(self):
        p = UploadProgress(bytes_sent=50, total_bytes=100)
        assert p.bytes_sent == 50


# ── Async Client Tests ────────────────────────────────────────────────────────


@pytest.mark.asyncio
class TestAsyncClient:
    async def test_async_connect(self):
        from grimlocker.async_client import AsyncClient

        with patch("grimlocker.async_client.websockets.connect") as mock_connect:
            mock_ws = MagicMock()
            mock_ws.recv = MagicMock()
            mock_ws.recv.return_value = _make_result_frame({"success": True})
            mock_ws.send = MagicMock()
            mock_ws.close = MagicMock()
            mock_connect.return_value.__aenter__.return_value = mock_ws

            client = await AsyncClient.connect("127.0.0.1", 41753, "token")
            assert client is not None
            await client.close()

    async def test_async_list_entries(self):
        from grimlocker.async_client import AsyncClient

        with patch("grimlocker.async_client.websockets.connect") as mock_connect:
            mock_ws = MagicMock()
            resp_buf = [
                _make_result_frame({"success": True}),
                _make_ok_result(entries_data=[_sample_entry_dict("e1", "Async")]),
            ]

            async def recv_side():
                return resp_buf.pop(0)

            mock_ws.recv = recv_side
            mock_ws.send = MagicMock()
            mock_ws.close = MagicMock()
            mock_connect.return_value.__aenter__.return_value = mock_ws

            client = await AsyncClient.connect("127.0.0.1", 41753, "token")
            entries = await client.list_entries("default")
            assert len(entries) == 1
            assert entries[0].title == "Async"
            await client.close()

    async def test_async_error_handling(self):
        from grimlocker.async_client import AsyncClient

        with patch("grimlocker.async_client.websockets.connect") as mock_connect:
            mock_ws = MagicMock()

            async def recv_side():
                return _make_result_frame(
                    {"success": False, "error_code": -101, "error_msg": "vault locked"},
                    opcode=_frame.OPCODE_ERROR,
                )

            mock_ws.recv = recv_side
            mock_ws.send = MagicMock()
            mock_ws.close = MagicMock()
            mock_connect.return_value.__aenter__.return_value = mock_ws

            client = await AsyncClient.connect("127.0.0.1", 41753, "token")
            with pytest.raises(GrimlockerError):
                await client.list_entries()
            await client.close()
