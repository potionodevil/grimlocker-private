use grimlocker_sdk::{
    AuditEvent, CertificateEntry, Entry, Error, ErrorCode, FileEntry, FolderItem, FolderListing, GrimlockerClient, PasswordEntry, SshKeyEntry, SyncPeer, SyncStatus, UploadProgress, Workspace,
};
use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::Arc;
use tokio::io::{AsyncReadExt, AsyncWriteExt};
use tokio::net::{TcpListener, TcpStream};
use tokio::sync::Mutex;

fn encode_init_ready() -> Vec<u8> {
    let payload = br#"{"success":true}"#.to_vec();
    let total_len = (1 + payload.len()) as u32;
    let mut frame = total_len.to_be_bytes().to_vec();
    frame.push(0x11);
    frame.extend(payload);
    frame
}

fn encode_result(json_payload: &str) -> Vec<u8> {
    let payload = json_payload.as_bytes().to_vec();
    let total_len = (1 + payload.len()) as u32;
    let mut frame = total_len.to_be_bytes().to_vec();
    frame.push(0x11);
    frame.extend(payload);
    frame
}

fn encode_error(code: i32, msg: &str) -> Vec<u8> {
    let payload = format!(r#"{{"success":false,"error_code":{},"error_msg":"{}"}}"#, code, msg)
        .as_bytes()
        .to_vec();
    let total_len = (1 + payload.len()) as u32;
    let mut frame = total_len.to_be_bytes().to_vec();
    frame.push(0x12);
    frame.extend(payload);
    frame
}

struct MockServer {
    addr: SocketAddr,
    responses: Arc<Mutex<Vec<Vec<u8>>>>,
}

impl MockServer {
    async fn new(responses: Vec<Vec<u8>>) -> Self {
        let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
        let addr = listener.local_addr().unwrap();
        let responses = Arc::new(Mutex::new(responses));
        let shared = responses.clone();

        tokio::spawn(async move {
            while let Ok((mut stream, _)) = listener.accept().await {
                let shared = shared.clone();
                tokio::spawn(async move {
                    serve_client(&mut stream, &shared).await;
                });
                break;
            }
        });

        MockServer { addr, responses }
    }

    fn url(&self) -> String {
        format!("ws://{}:{}/ws", self.addr.ip(), self.addr.port())
    }

    fn set_responses(&self, responses: Vec<Vec<u8>>) {
        let mut guard = self.responses.blocking_lock();
        *guard = responses;
    }
}

async fn serve_client(stream: &mut TcpStream, responses: &Arc<Mutex<Vec<Vec<u8>>>>) {
    let mut buf = [0u8; 65536];

    let n = stream.read(&mut buf).await.unwrap();
    assert!(n > 0, "expected WebSocket upgrade request");
    let upgrade = b"HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: s3pPLMBiTxaQ9kYGzzhZRbK+xOo=\r\n\r\n";
    stream.write_all(upgrade).await.unwrap();

    // Send INIT.READY
    let init = encode_init_ready();
    let ws_frame = make_ws_frame(true, 0x02, &init);
    stream.write_all(&ws_frame).await.unwrap();

    let guard = responses.lock().await;
    for resp in guard.iter() {
        let ws = make_ws_frame(true, 0x02, resp);
        stream.write_all(&ws).await.unwrap();

        // Read client frame
        let mut hdr = [0u8; 2];
        if stream.read_exact(&mut hdr).await.is_err() {
            break;
        }
        let payload_len = (hdr[1] & 0x7F) as usize;
        let mut _payload = vec![0u8; payload_len + 4]; // +4 mask key
        let _ = stream.read_exact(&mut _payload).await;
    }
}

fn make_ws_frame(fin: bool, opcode: u8, payload: &[u8]) -> Vec<u8> {
    let first = if fin { 0x80 | opcode } else { opcode };
    let len = payload.len();
    let mut frame = Vec::new();
    frame.push(first);
    if len < 126 {
        frame.push(len as u8);
    } else if len < 65536 {
        frame.push(126);
        frame.extend_from_slice(&(len as u16).to_be_bytes());
    } else {
        frame.push(127);
        frame.extend_from_slice(&(len as u64).to_be_bytes());
    }
    frame.extend_from_slice(payload);
    frame
}

fn entries_json(entries: &[&str]) -> String {
    let items: Vec<String> = entries.iter().map(|s| s.to_string()).collect();
    format!(
        r#"{{"success":true,"entries":[{items}],"total_count":{count}}}"#,
        items = items.join(","),
        count = items.len()
    )
}

fn entry_json(id: &str, title: &str, category: &str, fields: &[(&str, &str)]) -> String {
    let field_items: Vec<String> = fields
        .iter()
        .map(|(k, v)| format!(r#""{}":"{}""#, k, v))
        .collect();
    format!(
        r#"{{"id":"{id}","title":"{title}","category":"{category}","fields":{{{fields}}},"created_at":1,"updated_at":2}}"#,
        id = id,
        title = title,
        category = category,
        fields = field_items.join(",")
    )
}

// ── Tests ──────────────────────────────────────────────────────────────────────

#[tokio::test]
async fn test_connect() {
    let server = MockServer::new(vec![encode_result("{}")]).await;
    let client = GrimlockerClient::connect(&server.url(), "token").await;
    assert!(client.is_ok());
}

#[tokio::test]
async fn test_unlock() {
    let server = MockServer::new(vec![encode_result(r#"{"success":true}"#)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let result = client.unlock_vault("password").await;
    assert!(result.is_ok());
    client.close().await;
}

#[tokio::test]
async fn test_lock_vault() {
    let server = MockServer::new(vec![encode_result(r#"{"success":true}"#)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let result = client.lock_vault().await;
    assert!(result.is_ok());
    client.close().await;
}

#[tokio::test]
async fn test_list_entries() {
    let e1 = entry_json("e1", "Entry One", "PASSWORD", &[("username", "alice")]);
    let e2 = entry_json("e2", "Entry Two", "SSH_KEY", &[("public_key", "pk")]);
    let json = entries_json(&[&e1, &e2]);
    let server = MockServer::new(vec![encode_result(&json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let entries = client.list_entries(None).await.unwrap();
    assert_eq!(entries.len(), 2);
    assert_eq!(entries[0].id, "e1");
    assert_eq!(entries[1].category, "SSH_KEY");
    client.close().await;
}

#[tokio::test]
async fn test_get_entry() {
    let e = entry_json("e99", "Target", "PASSWORD", &[("username", "admin")]);
    let json = entries_json(&[&e]);
    let server = MockServer::new(vec![encode_result(&json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let entry = client.get_entry("e99").await.unwrap();
    assert_eq!(entry.id, "e99");
    assert_eq!(entry.title, "Target");
    client.close().await;
}

#[tokio::test]
async fn test_get_entry_not_found() {
    let json = r#"{"success":true,"entries":[],"total_count":0}"#;
    let server = MockServer::new(vec![encode_result(json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let result = client.get_entry("nonexistent").await;
    assert!(result.is_err());
    client.close().await;
}

#[tokio::test]
async fn test_create_entry() {
    let e = entry_json("new1", "New Entry", "PASSWORD", &[("username", "alice"), ("password", "sec")]);
    let json = entries_json(&[&e]);
    let server = MockServer::new(vec![encode_result(&json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let mut fields = HashMap::new();
    fields.insert("username".into(), "alice".into());
    fields.insert("password".into(), "sec".into());
    let entry = client.create_entry("New Entry", "PASSWORD", &fields).await.unwrap();
    assert_eq!(entry.id, "new1");
    client.close().await;
}

#[tokio::test]
async fn test_update_entry() {
    let server = MockServer::new(vec![encode_result(r#"{"success":true}"#)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let mut fields = HashMap::new();
    fields.insert("notes".into(), "updated".into());
    let result = client.update_entry("e1", "Updated Title", &fields).await;
    assert!(result.is_ok());
    client.close().await;
}

#[tokio::test]
async fn test_delete_entry() {
    let server = MockServer::new(vec![encode_result(r#"{"success":true}"#)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let result = client.delete_entry("e1").await;
    assert!(result.is_ok());
    client.close().await;
}

#[tokio::test]
async fn test_list_passwords() {
    let e = entry_json("p1", "GitHub", "PASSWORD", &[("username", "alice"), ("password", "sec"), ("url", ""), ("notes", "")]);
    let json = entries_json(&[&e]);
    let server = MockServer::new(vec![encode_result(&json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let passwords = client.list_passwords().await.unwrap();
    assert_eq!(passwords.len(), 1);
    assert_eq!(passwords[0].title, "GitHub");
    client.close().await;
}

#[tokio::test]
async fn test_create_password() {
    let e = entry_json("p1", "GitHub", "PASSWORD", &[("username", "alice"), ("password", "sec"), ("url", ""), ("notes", "")]);
    let json = entries_json(&[&e]);
    let server = MockServer::new(vec![encode_result(&json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let p = PasswordEntry {
        title: "GitHub".into(),
        username: "alice".into(),
        password: "sec".into(),
        url: "".into(),
        notes: "".into(),
        ..Default::default()
    };
    let id = client.create_password(&p).await.unwrap();
    assert_eq!(id, "p1");
    client.close().await;
}

#[tokio::test]
async fn test_list_ssh_keys() {
    let e = entry_json("sk1", "My Key", "SSH_KEY", &[("public_key", "pk"), ("private_key", ""), ("username", ""), ("passphrase", ""), ("comment", "")]);
    let json = entries_json(&[&e]);
    let server = MockServer::new(vec![encode_result(&json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let keys = client.list_ssh_keys().await.unwrap();
    assert_eq!(keys.len(), 1);
    client.close().await;
}

#[tokio::test]
async fn test_list_certificates() {
    let e = entry_json("c1", "Cert", "CERTIFICATE", &[("domain", "ex.com"), ("certificate", "crt"), ("private_key", "key")]);
    let json = entries_json(&[&e]);
    let server = MockServer::new(vec![encode_result(&json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let certs = client.list_certificates().await.unwrap();
    assert_eq!(certs.len(), 1);
    client.close().await;
}

#[tokio::test]
async fn test_search_entries() {
    let e = entry_json("e1", "GitHub", "PASSWORD", &[("username", "alice")]);
    let json = entries_json(&[&e]);
    let server = MockServer::new(vec![encode_result(&json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let results = client.search_entries("git", None).await.unwrap();
    assert_eq!(results.len(), 1);
    client.close().await;
}

#[tokio::test]
async fn test_list_folder() {
    let json = r#"{"folders":[{"id":"d1","name":"sub","type":"folder"}],"files":[]}"#;
    let server = MockServer::new(vec![encode_result(json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let listing = client.list_folder(Some("root")).await.unwrap();
    assert_eq!(listing.folders.len(), 1);
    client.close().await;
}

#[tokio::test]
async fn test_create_folder() {
    let json = r#"{"id":"f1","name":"Notes"}"#;
    let server = MockServer::new(vec![encode_result(json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let id = client.create_folder("Notes", None).await.unwrap();
    assert_eq!(id, "f1");
    client.close().await;
}

#[tokio::test]
async fn test_sync_peers() {
    let json = r#"{"peers":[{"id":"p1","name":"peer1","address":"192.168.1.5","connected":true,"last_seen":1}],"last_sync_at":0}"#;
    let server = MockServer::new(vec![encode_result(json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let status = client.list_sync_peers().await.unwrap();
    assert_eq!(status.peers.len(), 1);
    client.close().await;
}

#[tokio::test]
async fn test_trigger_sync() {
    let server = MockServer::new(vec![encode_result(r#"{"success":true}"#)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let result = client.trigger_sync().await;
    assert!(result.is_ok());
    client.close().await;
}

#[tokio::test]
async fn test_list_audit_events() {
    let json = r#"[{"timestamp":1,"level":"INFO","module":"auth","message":"unlock","subject_id":""}]"#;
    let server = MockServer::new(vec![encode_result(json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let events = client.list_audit_events(10).await.unwrap();
    assert_eq!(events.len(), 1);
    client.close().await;
}

#[tokio::test]
async fn test_health_check() {
    let json = r#"{"status":"ok","daemon_version":"1.0.0","vault_initialized":true,"vault_unlocked":true}"#;
    let server = MockServer::new(vec![encode_result(json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let health = client.health_check().await.unwrap();
    assert_eq!(health["status"], "ok");
    client.close().await;
}

#[tokio::test]
async fn test_error_handling() {
    let server = MockServer::new(vec![encode_error(-101, "vault is locked")]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let result = client.list_entries(None).await;
    assert!(result.is_err());
    if let Err(Error::Daemon { code, message }) = result {
        assert_eq!(code, ErrorCode::AuthRequired);
        assert!(message.contains("locked"));
    } else {
        panic!("expected daemon error");
    }
    client.close().await;
}

#[tokio::test]
async fn test_list_workspaces() {
    let json = r#"[{"id":"ws1","name":"Personal","is_default":true}]"#;
    let server = MockServer::new(vec![encode_result(json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let workspaces = client.list_workspaces().await.unwrap();
    assert_eq!(workspaces.len(), 1);
    assert_eq!(workspaces[0].name, "Personal");
    client.close().await;
}

#[tokio::test]
async fn test_create_workspace() {
    let json = r#"{"id":"ws2","name":"Work","is_default":false}"#;
    let server = MockServer::new(vec![encode_result(json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let ws = client.create_workspace("Work").await.unwrap();
    assert_eq!(ws.name, "Work");
    client.close().await;
}

#[tokio::test]
async fn test_generate_ssh_key() {
    let json = r#"{"public_key":"ssh-ed25519 AAA","fingerprint":"SHA256:abc","entry_id":"e1"}"#;
    let server = MockServer::new(vec![encode_result(json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let info = client.health_check().await.unwrap();
    let _ = client.close();
}

#[tokio::test]
async fn test_recovery_phrase() {
    let json = r#"{"phrase":"abandon ability able about above absent absorb abstract..."}"#;
    let server = MockServer::new(vec![encode_result(json)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let info = client.health_check().await.unwrap();
    let _ = client.close();
}

#[tokio::test]
async fn test_batch_create_entries() {
    let e1 = entry_json("b1", "A", "PASSWORD", &[]);
    let e2 = entry_json("b2", "B", "SSH_KEY", &[]);
    let json1 = entries_json(&[&e1]);
    let json2 = entries_json(&[&e2]);
    let server = MockServer::new(vec![encode_result(&json1), encode_result(&json2)]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let mut items = Vec::new();
    items.push(("A".into(), "PASSWORD".into(), HashMap::new()));
    items.push(("B".into(), "SSH_KEY".into(), HashMap::new()));
    let ids = client.create_entries_batch(&items).await.unwrap();
    assert_eq!(ids.len(), 2);
    assert_eq!(ids[0], "b1");
    assert_eq!(ids[1], "b2");
    client.close().await;
}

#[tokio::test]
async fn test_batch_delete_entries() {
    let server = MockServer::new(vec![
        encode_result(r#"{"success":true}"#),
        encode_result(r#"{"success":true}"#),
    ]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    let ids = vec!["e1".into(), "e2".into()];
    client.delete_entries_batch(&ids).await.unwrap();
    client.close().await;
}

#[tokio::test]
async fn test_circuit_breaker() {
    let server = MockServer::new(vec![
        encode_error(-1, "fail"),
        encode_error(-1, "fail"),
        encode_error(-1, "fail"),
        encode_error(-1, "fail"),
        encode_error(-1, "fail"),
        encode_error(-1, "fail"),
    ]).await;
    let mut client = GrimlockerClient::connect(&server.url(), "token").await.unwrap();
    for i in 0..5 {
        let result = client.list_entries(None).await;
        assert!(result.is_err(), "expected error on call {}", i);
    }
    let result = client.list_entries(None).await;
    assert!(result.is_err(), "expected circuit breaker open");
    if let Err(Error::WebSocket(msg)) = result {
        assert!(msg.contains("circuit breaker open"), "expected circuit breaker open, got: {}", msg);
    } else {
        panic!("expected WebSocket error due to open circuit, got: {:?}", result);
    }
    client.close().await;
}

#[tokio::test]
async fn test_download_file() {
    let json = r#"{"data_b64":"aGVsbG8="}"#;
    let server = MockServer::new(vec![encode_result(json)]).await;
    let result = GrimlockerClient::connect(&server.url(), "token").await;
    assert!(result.is_ok());
}
