//! # grimlocker-sdk
//!
//! Rust SDK for the Grimlocker Zero-Trust Vault daemon.
//!
//! Communicates over the GQL binary WebSocket protocol for maximum performance.
//!
//! ## Quick Start
//!
//! ```no_run
//! use grimlocker_sdk::GrimlockerClient;
//!
//! #[tokio::main]
//! async fn main() -> Result<(), grimlocker_sdk::Error> {
//!     let mut client = GrimlockerClient::connect("ws://127.0.0.1:36352/ws", "TOKEN").await?;
//!     client.unlock_vault("master-password").await?;
//!
//!     let passwords = client.list_passwords().await?;
//!     for p in &passwords {
//!         println!("{} — {}", p.title, p.username);
//!     }
//!     Ok(())
//! }
//! ```

pub mod client;
pub mod entries;
pub mod error;
pub mod protocol;
pub mod files;
pub mod workspaces;
pub mod sync;
pub mod audit;

pub use client::GrimlockerClient;
pub use client::ClientEvent;
pub use entries::{Entry, PasswordEntry, SshKeyEntry, CertificateEntry, FileEntry, SshKeyResult};
pub use error::{Error, ErrorCode};
pub use files::{FolderListing, FolderItem, UploadProgress};
pub use workspaces::Workspace;
pub use sync::{SyncStatus, SyncPeer};
pub use audit::AuditEvent;
