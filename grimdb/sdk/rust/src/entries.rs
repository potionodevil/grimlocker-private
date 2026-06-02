use serde::{Deserialize, Serialize};
use std::collections::HashMap;

#[derive(Debug, Clone, Serialize, Deserialize, Default)]
pub struct Entry {
    pub id:         String,
    pub title:      String,
    pub category:   String,
    pub fields:     HashMap<String, String>,
    pub created_at: i64,
    pub updated_at: i64,
}

impl Entry {
    pub fn field(&self, key: &str) -> &str {
        self.fields.get(key).map(String::as_str).unwrap_or("")
    }
}

#[derive(Debug, Clone, Default)]
pub struct PasswordEntry {
    pub id:       String,
    pub title:    String,
    pub username: String,
    pub password: String,
    pub url:      String,
    pub notes:    String,
}

impl PasswordEntry {
    pub fn from_entry(e: &Entry) -> Self {
        Self {
            id:       e.id.clone(),
            title:    e.title.clone(),
            username: e.field("username").to_owned(),
            password: e.field("password").to_owned(),
            url:      e.field("url").to_owned(),
            notes:    e.field("notes").to_owned(),
        }
    }
    pub fn to_fields(&self) -> HashMap<String, String> {
        [("username", &self.username), ("password", &self.password),
         ("url", &self.url), ("notes", &self.notes)]
            .into_iter().map(|(k, v)| (k.to_owned(), v.clone())).collect()
    }
}

#[derive(Debug, Clone, Default)]
pub struct SshKeyEntry {
    pub id:          String,
    pub title:       String,
    pub public_key:  String,
    pub private_key: String,
    pub username:    String,
    pub passphrase:  String,
    pub comment:     String,
}

impl SshKeyEntry {
    pub fn from_entry(e: &Entry) -> Self {
        Self {
            id:          e.id.clone(),
            title:       e.title.clone(),
            public_key:  e.field("public_key").to_owned(),
            private_key: e.field("private_key").to_owned(),
            username:    e.field("username").to_owned(),
            passphrase:  e.field("passphrase").to_owned(),
            comment:     e.field("comment").to_owned(),
        }
    }
    pub fn to_fields(&self) -> HashMap<String, String> {
        [("public_key", &self.public_key), ("private_key", &self.private_key),
         ("username", &self.username), ("passphrase", &self.passphrase),
         ("comment", &self.comment)]
            .into_iter().map(|(k, v)| (k.to_owned(), v.clone())).collect()
    }
}

#[derive(Debug, Clone, Default)]
pub struct CertificateEntry {
    pub id:          String,
    pub title:       String,
    pub domain:      String,
    pub certificate: String,
    pub private_key: String,
}

impl CertificateEntry {
    pub fn from_entry(e: &Entry) -> Self {
        Self {
            id:          e.id.clone(),
            title:       e.title.clone(),
            domain:      e.field("domain").to_owned(),
            certificate: e.field("certificate").to_owned(),
            private_key: e.field("private_key").to_owned(),
        }
    }
    pub fn to_fields(&self) -> HashMap<String, String> {
        [("domain", &self.domain), ("certificate", &self.certificate),
         ("private_key", &self.private_key)]
            .into_iter().map(|(k, v)| (k.to_owned(), v.clone())).collect()
    }
}

#[derive(Debug, Clone, Default, Deserialize, Serialize)]
pub struct FileEntry {
    pub id:                String,
    pub title:             String,
    pub file_name:         String,
    pub mime_type:         String,
    pub total_size:        u64,
    pub manifest_block_id: String,
    pub folder_id:         String,
}

#[derive(Debug, Clone, Default, Serialize, Deserialize)]
pub struct SshKeyResult {
    pub public_key:  String,
    pub fingerprint: String,
    pub entry_id:    Option<String>,
}
