use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Default, Deserialize, Serialize)]
pub struct SyncPeer {
    pub device_id: String,
    pub host:      String,
    pub port:      u16,
    pub seen_at:   i64,
    pub reachable: Option<bool>,
}

#[derive(Debug, Clone, Default, Deserialize, Serialize)]
pub struct SyncStatus {
    pub peers:        Vec<SyncPeer>,
    pub last_sync_at: i64,
    pub device_id:    String,
}
