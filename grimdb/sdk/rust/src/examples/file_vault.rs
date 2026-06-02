//! File Vault example — list folders, upload, download.
use grimlocker_sdk::GrimlockerClient;

#[tokio::main]
async fn main() -> Result<(), grimlocker_sdk::Error> {
    let mut client = GrimlockerClient::connect("ws://127.0.0.1:36352/ws", "TOKEN").await?;
    client.unlock_vault("master-password").await?;

    let listing = client.list_folder(None).await?;
    println!("Folders: {}, Files: {}", listing.folders.len(), listing.files.len());

    client.close().await;
    Ok(())
}
