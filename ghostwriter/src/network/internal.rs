use std::net::SocketAddr;
use std::path::PathBuf;

use tokio::task::JoinHandle;

use crate::error::Result;
use crate::files::workspace::WorkspaceManager;
use crate::network::{client::GhostwriterClient, server::GhostwriterServer};

/// Internal server used for local editing mode.
#[derive(Debug)]
#[allow(dead_code)]
pub struct InternalServer {
    addr: SocketAddr,
    handle: JoinHandle<()>,
}

#[allow(dead_code)]
impl InternalServer {
    /// Start a new internal server bound to 127.0.0.1 on a random port.
    pub async fn start(root: PathBuf, key: Option<String>) -> Result<(Self, GhostwriterClient)> {
        let ws = WorkspaceManager::new(root)?;
        let server =
            GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, key.clone()).await?;
        let addr = server.local_addr()?;
        let handle = tokio::spawn(async move {
            if let Err(e) = server.run().await {
                eprintln!("internal server error: {e}");
            }
        });
        let client = GhostwriterClient::new(format!("ws://{}", addr), key);
        Ok((Self { addr, handle }, client))
    }

    /// Address the server is bound to.
    pub fn addr(&self) -> SocketAddr {
        self.addr
    }
}

impl Drop for InternalServer {
    fn drop(&mut self) {
        self.handle.abort();
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::network::protocol::MessageKind;
    use serial_test::serial;
    use tempfile::tempdir;
    use tokio::time::{Duration, Instant};

    #[tokio::test]
    #[serial]
    async fn test_internal_server_startup() {
        let dir = tempdir().unwrap();
        let (server, mut client) = InternalServer::start(dir.path().to_path_buf(), None)
            .await
            .unwrap();
        assert_eq!(server.addr().ip().to_string(), "127.0.0.1");
        assert!(server.addr().port() != 0);
        client.connect().await.unwrap();
        drop(client);
        drop(server);
    }

    #[tokio::test]
    #[serial]
    async fn test_loopback_connection() {
        let dir = tempdir().unwrap();
        let (server, mut client) = InternalServer::start(dir.path().to_path_buf(), None)
            .await
            .unwrap();
        client.connect().await.unwrap();
        let resp = client
            .request(MessageKind::Ping, Duration::from_secs(1))
            .await
            .unwrap();
        assert!(matches!(resp.kind, MessageKind::Pong));
        drop(client);
        drop(server);
    }

    #[tokio::test]
    #[serial]
    async fn test_local_operation_latency() {
        let dir = tempdir().unwrap();
        let (server, mut client) = InternalServer::start(dir.path().to_path_buf(), None)
            .await
            .unwrap();
        client.connect().await.unwrap();
        let start = Instant::now();
        let _ = client
            .request(MessageKind::Ping, Duration::from_secs(1))
            .await
            .unwrap();
        let elapsed = start.elapsed();
        assert!(elapsed < Duration::from_millis(10));
        drop(client);
        drop(server);
    }

    #[tokio::test]
    #[serial]
    async fn test_cleanup_on_exit() {
        let dir = tempdir().unwrap();
        let (server, mut client) = InternalServer::start(dir.path().to_path_buf(), None)
            .await
            .unwrap();
        let addr = server.addr();
        client.connect().await.unwrap();
        drop(client);
        drop(server);
        tokio::time::sleep(Duration::from_millis(100)).await;
        let res = GhostwriterClient::new(format!("ws://{}", addr), None)
            .connect()
            .await;
        assert!(res.is_err());
    }
}
