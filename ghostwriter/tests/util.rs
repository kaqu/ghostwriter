use ghostwriter::files::workspace::WorkspaceManager;
use ghostwriter::network::{client::GhostwriterClient, server::GhostwriterServer};
use std::net::SocketAddr;
use std::path::Path;
use tokio::task::JoinHandle;

pub async fn start_server(
    root: &Path,
    key: Option<String>,
) -> (JoinHandle<()>, GhostwriterClient, SocketAddr) {
    let ws = WorkspaceManager::new(root.to_path_buf()).unwrap();
    let server = GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, key.clone())
        .await
        .unwrap();
    let addr = server.local_addr().unwrap();
    let handle = tokio::spawn(async move {
        if let Err(e) = server.run().await {
            eprintln!("server error: {e}");
        }
    });
    let client = GhostwriterClient::new(format!("ws://{}", addr), key).unwrap();
    (handle, client, addr)
}

pub async fn start_server_with_addr(
    root: &Path,
    addr: SocketAddr,
    key: Option<String>,
) -> (JoinHandle<()>, GhostwriterClient) {
    let ws = WorkspaceManager::new(root.to_path_buf()).unwrap();
    let server = GhostwriterServer::bind(addr, ws, key.clone())
        .await
        .unwrap();
    let handle = tokio::spawn(async move {
        if let Err(e) = server.run().await {
            eprintln!("server error: {e}");
        }
    });
    let client = GhostwriterClient::new(format!("ws://{}", addr), key).unwrap();
    (handle, client)
}
