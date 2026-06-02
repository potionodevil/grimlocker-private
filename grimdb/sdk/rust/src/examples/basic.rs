//! Basic example showing connect, unlock, list passwords, create, delete.
use grimlocker_sdk::GrimlockerClient;

#[tokio::main]
async fn main() -> Result<(), grimlocker_sdk::Error> {
    let mut client = GrimlockerClient::connect("ws://127.0.0.1:36352/ws", "TOKEN").await?;
    client.unlock_vault("master-password").await?;
    let passwords = client.list_passwords().await?;
    for p in &passwords { println!("{} — {}", p.title, p.username); }
    client.close().await;
    Ok(())
}
