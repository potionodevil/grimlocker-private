use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct FolderItem {
    pub id:   String,
    pub name: String,
    #[serde(rename = "type")]
    pub kind: String,   // "folder" | "file"
}

#[derive(Debug, Clone, Default, Deserialize, Serialize)]
pub struct FolderListing {
    pub folders: Vec<FolderItem>,
    pub files:   Vec<FileInfo>,
}

#[derive(Debug, Clone, Deserialize, Serialize)]
pub struct FileInfo {
    pub id:                String,
    pub file_name:         String,
    pub mime_type:         String,
    pub total_size:        u64,
    pub manifest_block_id: String,
}

/// Emitted during a file upload to report progress.
#[derive(Debug, Clone)]
pub struct UploadProgress {
    /// Bytes transferred so far.
    pub bytes_sent: u64,
    /// Total file size in bytes.
    pub total_bytes: u64,
}

impl UploadProgress {
    pub fn percent(&self) -> f64 {
        if self.total_bytes == 0 { return 100.0; }
        (self.bytes_sent as f64 / self.total_bytes as f64) * 100.0
    }
}
