//! Main `GrimlockerClient` — async WebSocket client using the GQL binary protocol.

use std::collections::HashMap;
use std::sync::Arc;
use tokio::sync::{Mutex, broadcast};
use futures_util::{SinkExt, StreamExt};
use tokio_tungstenite::{connect_async, tungstenite::Message};
use serde_json::Value;

use crate::entries::*;
use crate::error::Error;
use crate::files::{FolderListing, UploadProgress};
use crate::workspaces::Workspace;
use crate::sync::SyncStatus;
use crate::audit::AuditEvent;
use crate::protocol::{self, GqlResponse};

type WsStream = tokio_tungstenite::WebSocketStream<
    tokio_tungstenite::MaybeTlsStream<tokio::net::TcpStream>
>;

#[derive(Debug, Clone)]
pub enum ClientEvent {
    Connected,
    Disconnected,
    EntryChanged { entry_id: String },
    Error(String),
}

/// Async GQL WebSocket client.
///
/// # Example
/// ```no_run
/// # use grimlocker_sdk::GrimlockerClient;
/// # #[tokio::main] async fn main() -> Result<(), grimlocker_sdk::Error> {
/// let mut c = GrimlockerClient::connect("ws://127.0.0.1:36352/ws", "TOKEN").await?;
/// c.unlock_vault("password").await?;
/// let entries = c.list_entries(None).await?;
/// # Ok(()) }
/// ```
pub struct GrimlockerClient {
    ws:       Arc<Mutex<WsStream>>,
    event_tx: broadcast::Sender<ClientEvent>,
}

impl GrimlockerClient {
    /// Connect to the daemon and consume the initial handshake frame.
    pub async fn connect(url: &str, _token: &str) -> Result<Self, Error> {
        let (ws, _) = connect_async(url).await
            .map_err(|e| Error::Connect(e.to_string()))?;
        let ws = Arc::new(Mutex::new(ws));

        // Consume the INIT.READY handshake frame
        {
            let mut lock = ws.lock().await;
            lock.next().await;
        }

        let (event_tx, _) = broadcast::channel::<ClientEvent>(64);
        Ok(Self { ws, event_tx })
    }

    /// Disconnect gracefully.
    pub async fn close(&self) {
        let mut lock = self.ws.lock().await;
        let _ = lock.close(None).await;
    }

    pub fn events(&self) -> broadcast::Receiver<ClientEvent> {
        self.event_tx.subscribe()
    }

    // ── Auth ─────────────────────────────────────────────────────────────────

    pub async fn unlock_vault(&mut self, password: &str) -> Result<(), Error> {
        let mut fields = HashMap::new();
        fields.insert("password".into(), password.into());
        self.execute("vault.unlock", "", "", "", "", &fields, 1, 0).await?;
        Ok(())
    }

    pub async fn lock_vault(&mut self) -> Result<(), Error> {
        self.execute("vault.logout", "", "", "", "", &HashMap::new(), 1, 0).await?;
        Ok(())
    }

    pub async fn vault_status(&mut self) -> Result<Value, Error> {
        let resp = self.execute("vault.status", "", "", "", "", &HashMap::new(), 1, 0).await?;
        Ok(serde_json::to_value(&resp)?)
    }

    // ── Entries ───────────────────────────────────────────────────────────────

    pub async fn list_entries(&mut self, category: Option<&str>) -> Result<Vec<Entry>, Error> {
        let resp = self.execute(
            if category.is_some() { "query_entries" } else { "list_entries" },
            "default", "", category.unwrap_or(""), "", &HashMap::new(), 100, 0,
        ).await?;
        self.parse_entries(&resp)
    }

    pub async fn get_entry(&mut self, id: &str) -> Result<Entry, Error> {
        let resp = self.execute("get_entry", "default", id, "", "", &HashMap::new(), 1, 0).await?;
        let entries = self.parse_entries(&resp)?;
        entries.into_iter().next()
            .ok_or_else(|| Error::Daemon { code: crate::error::ErrorCode::EntryNotFound, message: format!("entry not found: {id}") })
    }

    pub async fn create_entry(&mut self, title: &str, category: &str, fields: &HashMap<String, String>) -> Result<Entry, Error> {
        let resp = self.execute("create_entry", "default", "", category, title, fields, 1, 0).await?;
        let entries = self.parse_entries(&resp)?;
        entries.into_iter().next()
            .ok_or_else(|| Error::Daemon { code: crate::error::ErrorCode::CreateFailed, message: "create returned no entry".into() })
    }

    pub async fn update_entry(&mut self, id: &str, title: &str, fields: &HashMap<String, String>) -> Result<(), Error> {
        self.execute("update_entry", "default", id, "", title, fields, 1, 0).await?;
        Ok(())
    }

    pub async fn delete_entry(&mut self, id: &str) -> Result<(), Error> {
        self.execute("delete_entry", "default", id, "", "", &HashMap::new(), 1, 0).await?;
        Ok(())
    }

    // ── Typed helpers ─────────────────────────────────────────────────────────

    pub async fn list_passwords(&mut self) -> Result<Vec<PasswordEntry>, Error> {
        let entries = self.list_entries(Some("PASSWORD")).await?;
        Ok(entries.iter().map(PasswordEntry::from_entry).collect())
    }

    pub async fn create_password(&mut self, p: &PasswordEntry) -> Result<String, Error> {
        let e = self.create_entry(&p.title, "PASSWORD", &p.to_fields()).await?;
        Ok(e.id)
    }

    pub async fn list_ssh_keys(&mut self) -> Result<Vec<SshKeyEntry>, Error> {
        let entries = self.list_entries(Some("SSH_KEY")).await?;
        Ok(entries.iter().map(SshKeyEntry::from_entry).collect())
    }

    pub async fn create_ssh_key(&mut self, k: &SshKeyEntry) -> Result<String, Error> {
        let e = self.create_entry(&k.title, "SSH_KEY", &k.to_fields()).await?;
        Ok(e.id)
    }

    pub async fn list_certificates(&mut self) -> Result<Vec<CertificateEntry>, Error> {
        let entries = self.list_entries(Some("CERTIFICATE")).await?;
        Ok(entries.iter().map(CertificateEntry::from_entry).collect())
    }

    pub async fn create_certificate(&mut self, c: &CertificateEntry) -> Result<String, Error> {
        let e = self.create_entry(&c.title, "CERTIFICATE", &c.to_fields()).await?;
        Ok(e.id)
    }

    pub async fn search_entries(&mut self, query: &str, category: Option<&str>) -> Result<Vec<Entry>, Error> {
        let mut fields = HashMap::new();
        fields.insert("search".into(), query.into());
        if let Some(cat) = category {
            fields.insert("category".into(), cat.into());
        }
        let resp = self.execute("search_entries", "default", "", "", "", &fields, 100, 0).await?;
        self.parse_entries(&resp)
    }

    // ── File Vault ────────────────────────────────────────────────────────────

    pub async fn list_folder(&mut self, folder_id: Option<&str>) -> Result<FolderListing, Error> {
        let mut fields = HashMap::new();
        fields.insert("folder_id".into(), folder_id.unwrap_or("").into());
        let resp = self.execute("file.list_folder", "default", "", "", "", &fields, 200, 0).await?;
        let data = serde_json::to_value(&resp)?;
        Ok(serde_json::from_value(data).unwrap_or_default())
    }

    pub async fn create_folder(&mut self, name: &str, parent_id: Option<&str>) -> Result<String, Error> {
        let mut fields = HashMap::new();
        fields.insert("name".into(), name.into());
        fields.insert("parent_id".into(), parent_id.unwrap_or("").into());
        let resp = self.execute("file.create_folder", "default", "", "", "", &fields, 1, 0).await?;
        let entries = self.parse_entries(&resp)?;
        Ok(entries.first().map(|e| e.id.clone()).unwrap_or_default())
    }

    pub async fn rename_folder(&mut self, id: &str, name: &str) -> Result<(), Error> {
        let mut fields = HashMap::new();
        fields.insert("name".into(), name.into());
        self.execute("file.rename_folder", "default", id, "", "", &fields, 1, 0).await?;
        Ok(())
    }

    pub async fn delete_folder(&mut self, id: &str) -> Result<(), Error> {
        self.execute("file.delete_folder", "default", id, "", "", &HashMap::new(), 1, 0).await?;
        Ok(())
    }

    pub async fn move_file(&mut self, manifest_block_id: &str, folder_id: &str) -> Result<(), Error> {
        let mut fields = HashMap::new();
        fields.insert("manifest_block_id".into(), manifest_block_id.into());
        fields.insert("folder_id".into(), folder_id.into());
        self.execute("file.move", "default", "", "", "", &fields, 1, 0).await?;
        Ok(())
    }

    pub async fn upload_file<F>(
        &mut self,
        data: &[u8],
        filename: &str,
        mime_type: &str,
        folder_id: Option<&str>,
        on_progress: F,
    ) -> Result<FileEntry, Error>
    where
        F: Fn(UploadProgress),
    {
        let total = data.len() as u64;
        on_progress(UploadProgress { bytes_sent: 0, total_bytes: total });

        let mut fields = HashMap::new();
        fields.insert("file_name".into(), filename.into());
        fields.insert("mime_type".into(), mime_type.into());
        fields.insert("folder_id".into(), folder_id.unwrap_or("").into());
        fields.insert("data_b64".into(), base64::Engine::encode(&base64::engine::general_purpose::STANDARD, data));
        let resp = self.execute("file.ingest", "default", "", "FILE_VAULT", filename, &fields, 1, 0).await?;

        on_progress(UploadProgress { bytes_sent: total, total_bytes: total });

        let entries = self.parse_entries(&resp)?;
        let entry = entries.into_iter().next()
            .ok_or_else(|| Error::Daemon { code: crate::error::ErrorCode::CreateFailed, message: "upload returned no entry".into() })?;
        Ok(FileEntry {
            id:                entry.id.clone(),
            title:             entry.title.clone(),
            file_name:         entry.field("file_name").to_owned(),
            mime_type:         entry.field("mime_type").to_owned(),
            total_size:        entry.field("total_size").parse().unwrap_or(0),
            manifest_block_id: entry.field("manifest_block_id").to_owned(),
            folder_id:         entry.field("folder_id").to_owned(),
        })
    }

    // ── Workspaces ────────────────────────────────────────────────────────────

    pub async fn list_workspaces(&mut self) -> Result<Vec<Workspace>, Error> {
        let resp = self.execute("workspace.list", "default", "", "", "", &HashMap::new(), 100, 0).await?;
        let v = serde_json::to_value(&resp)?;
        let items = v.as_array().cloned().unwrap_or_default();
        Ok(items.iter().filter_map(|i| serde_json::from_value(i.clone()).ok()).collect())
    }

    pub async fn create_workspace(&mut self, name: &str) -> Result<Workspace, Error> {
        let mut fields = HashMap::new();
        fields.insert("name".into(), name.into());
        let resp = self.execute("workspace.create", "default", "", "", name, &fields, 1, 0).await?;
        let v = serde_json::to_value(&resp)?;
        serde_json::from_value(v).map_err(|e| Error::Protocol(e.to_string()))
    }

    pub async fn switch_workspace(&mut self, id: &str) -> Result<(), Error> {
        let mut fields = HashMap::new();
        fields.insert("id".into(), id.into());
        self.execute("workspace.switch", "default", id, "", "", &fields, 1, 0).await?;
        Ok(())
    }

    pub async fn rename_workspace(&mut self, id: &str, name: &str) -> Result<(), Error> {
        let mut fields = HashMap::new();
        fields.insert("name".into(), name.into());
        self.execute("workspace.rename", "default", id, "", name, &fields, 1, 0).await?;
        Ok(())
    }

    pub async fn delete_workspace(&mut self, id: &str) -> Result<(), Error> {
        self.execute("workspace.delete", "default", id, "", "", &HashMap::new(), 1, 0).await?;
        Ok(())
    }

    // ── Sync ──────────────────────────────────────────────────────────────────

    pub async fn list_sync_peers(&mut self) -> Result<SyncStatus, Error> {
        let resp = self.execute("sync.list_peers", "", "", "", "", &HashMap::new(), 1, 0).await?;
        let v = serde_json::to_value(&resp)?;
        serde_json::from_value(v).map_err(|e| Error::Protocol(e.to_string()))
    }

    pub async fn trigger_sync(&mut self) -> Result<(), Error> {
        self.execute("sync.trigger", "", "", "", "", &HashMap::new(), 1, 0).await?;
        Ok(())
    }

    // ── Audit ─────────────────────────────────────────────────────────────────

    pub async fn list_audit_events(&mut self, n: usize) -> Result<Vec<AuditEvent>, Error> {
        let mut fields = HashMap::new();
        fields.insert("n".into(), n.to_string());
        let resp = self.execute("audit.list", "", "", "", "", &fields, n as u32, 0).await?;
        let v = serde_json::to_value(&resp)?;
        let items = v.as_array().cloned().unwrap_or_default();
        Ok(items.iter().filter_map(|i| serde_json::from_value(i.clone()).ok()).collect())
    }

    // ── Health ────────────────────────────────────────────────────────────────

    pub async fn health_check(&mut self) -> Result<Value, Error> {
        let resp = self.execute("vault.status", "", "", "", "", &HashMap::new(), 1, 0).await?;
        Ok(serde_json::to_value(&resp)?)
    }

    /// Generate an SSH key pair. If save_to_vault is true, stores in the vault.
    pub async fn generate_ssh_key(&mut self, comment: &str, save_to_vault: bool) -> Result<SshKeyResult, Error> {
        let mut fields = HashMap::new();
        fields.insert("comment".into(), comment.into());
        fields.insert("save_to_vault".into(), if save_to_vault { "true" } else { "false" }.into());
        let resp = self.execute("tool.ssh_keygen", "", "", "", "", &fields, 1, 0).await?;
        let v = serde_json::to_value(&resp)?;
        Ok(SshKeyResult {
            public_key:  v.get("public_key").and_then(|s| s.as_str()).unwrap_or("").into(),
            fingerprint: v.get("fingerprint").and_then(|s| s.as_str()).unwrap_or("").into(),
            entry_id:    v.get("entry_id").and_then(|s| s.as_str()).map(String::from),
        })
    }

    /// Retrieve the recovery phrase using the master password.
    pub async fn recovery_phrase(&mut self, password: &str) -> Result<String, Error> {
        let mut fields = HashMap::new();
        fields.insert("password".into(), password.into());
        let resp = self.execute("vault.recovery_phrase", "", "", "", "", &fields, 1, 0).await?;
        let v = serde_json::to_value(&resp)?;
        v.get("phrase").and_then(|s| s.as_str()).map(String::from)
            .ok_or_else(|| Error::Protocol("recovery phrase not returned".into()))
    }

    /// Download a file from the File Vault. Returns the decrypted file bytes.
    pub async fn download_file(&mut self, manifest_block_id: &str) -> Result<Vec<u8>, Error> {
        let mut fields = HashMap::new();
        fields.insert("manifest_block_id".into(), manifest_block_id.into());
        let resp = self.execute("file.download", "", "", "", "", &fields, 1, 0).await?;
        let v = serde_json::to_value(&resp)?;
        let b64 = v.get("data_b64").and_then(|s| s.as_str()).unwrap_or("");
        base64::Engine::decode(&base64::engine::general_purpose::STANDARD, b64)
            .map_err(|e| Error::Protocol(format!("base64 decode: {e}")))
    }

    // ── Internal ─────────────────────────────────────────────────────────────

    async fn execute(
        &mut self,
        operation: &str,
        namespace: &str,
        entry_id:  &str,
        category:  &str,
        title:     &str,
        fields:    &HashMap<String, String>,
        limit:     u32,
        offset:    u32,
    ) -> Result<GqlResponse, Error> {
        let frame = protocol::encode_query(operation, namespace, entry_id, category, title, fields, limit, offset);
        let mut lock = self.ws.lock().await;
        lock.send(Message::Binary(frame.into())).await
            .map_err(|e| Error::WebSocket(e.to_string()))?;
        let msg = lock.next().await
            .ok_or_else(|| Error::WebSocket("connection closed".into()))?
            .map_err(|e| Error::WebSocket(e.to_string()))?;
        drop(lock);

        let data = match msg {
            Message::Binary(b) => b.to_vec(),
            Message::Text(t)   => t.into_bytes(),
            other => return Err(Error::Protocol(format!("unexpected message type: {other:?}"))),
        };
        protocol::parse_response(&data)
    }

    fn parse_entries(&self, resp: &GqlResponse) -> Result<Vec<Entry>, Error> {
        let raw = resp.entries.clone().unwrap_or_default();
        raw.iter()
            .map(|v| serde_json::from_value(v.clone()).map_err(|e| Error::Json(e)))
            .collect()
    }
}

impl Drop for GrimlockerClient {
    fn drop(&mut self) {
        // Best-effort close — ignore errors during drop
        let ws = Arc::clone(&self.ws);
        tokio::spawn(async move {
            let mut lock = ws.lock().await;
            let _ = lock.close(None).await;
        });
    }
}
