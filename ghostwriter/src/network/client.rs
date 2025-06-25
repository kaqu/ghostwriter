// WebSocket client implementation
#![allow(dead_code)]

use std::time::Duration;

use futures_util::{SinkExt, StreamExt};
use tokio::net::TcpStream;
use tokio_tungstenite::{
    MaybeTlsStream, WebSocketStream, connect_async, tungstenite::Message as WsMessage,
};
use url::Url;
use uuid::Uuid;

use crate::error::{GhostwriterError, Result};
use crate::network::protocol::{Message, MessageKind};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ConnectionStatus {
    Connected,
    Reconnecting,
    Disconnected,
}

pub struct GhostwriterClient {
    url: String,
    key: Option<String>,
    ws: Option<WebSocketStream<MaybeTlsStream<TcpStream>>>,
    backoff: u64,
    queue: Vec<Message>,
    status: ConnectionStatus,
}

impl GhostwriterClient {
    fn validate_url(url: &str) -> Result<()> {
        let parsed =
            Url::parse(url).map_err(|e| GhostwriterError::InvalidArgument(e.to_string()))?;
        match parsed.scheme() {
            "ws" | "wss" => Ok(()),
            _ => Err(GhostwriterError::InvalidArgument(
                "invalid url scheme".into(),
            )),
        }
    }

    pub fn new(url: String, key: Option<String>) -> Result<Self> {
        Self::validate_url(&url)?;
        Ok(Self {
            url,
            key,
            ws: None,
            backoff: 100,
            queue: Vec::new(),
            status: ConnectionStatus::Disconnected,
        })
    }

    pub fn status(&self) -> ConnectionStatus {
        self.status
    }

    pub async fn connect(&mut self) -> Result<()> {
        self.ensure_connection().await
    }

    async fn ensure_connection(&mut self) -> Result<()> {
        if self.ws.is_some() {
            return Ok(());
        }
        self.status = ConnectionStatus::Reconnecting;
        let mut delay = self.backoff;
        loop {
            match connect_async(&self.url).await {
                Ok((mut stream, _)) => {
                    let auth = Message {
                        id: Uuid::new_v4(),
                        kind: MessageKind::AuthRequest {
                            key: self.key.clone(),
                        },
                    };
                    stream
                        .send(WsMessage::Text(serde_json::to_string(&auth)?.into()))
                        .await?;
                    let resp = stream
                        .next()
                        .await
                        .ok_or_else(|| GhostwriterError::Network("no response".into()))??;
                    let msg: Message = serde_json::from_str(&resp.into_text()?)?;
                    match msg.kind {
                        MessageKind::AuthResponse { success, .. } if success => {
                            self.ws = Some(stream);
                            self.status = ConnectionStatus::Connected;
                            self.backoff = 100;
                            self.flush_queue().await?;
                            return Ok(());
                        }
                        _ => {
                            return Err(GhostwriterError::Network("auth failed".into()));
                        }
                    }
                }
                Err(_) => {
                    tokio::time::sleep(Duration::from_millis(delay)).await;
                    delay = (delay * 2).min(1600);
                    if delay > 800 {
                        self.status = ConnectionStatus::Disconnected;
                        return Err(GhostwriterError::Network("connection failed".into()));
                    }
                }
            }
        }
    }

    /// Attempt to recover from a disconnected state by reconnecting.
    pub async fn recover(&mut self) -> Result<()> {
        self.ensure_connection().await
    }

    async fn flush_queue(&mut self) -> Result<()> {
        if self.queue.is_empty() {
            return Ok(());
        }
        let ws = self
            .ws
            .as_mut()
            .ok_or_else(|| GhostwriterError::Network("not connected".into()))?;
        let queued = std::mem::take(&mut self.queue);
        for msg in queued {
            ws.send(WsMessage::Text(serde_json::to_string(&msg)?.into()))
                .await?;
            let _ = ws.next().await;
        }
        Ok(())
    }

    pub async fn request(&mut self, kind: MessageKind, timeout: Duration) -> Result<Message> {
        let msg = Message {
            id: Uuid::new_v4(),
            kind,
        };
        if let Err(e) = self.ensure_connection().await {
            self.queue.push(msg);
            return Err(e);
        }
        if let Some(ws) = self.ws.as_mut() {
            ws.send(WsMessage::Text(serde_json::to_string(&msg)?.into()))
                .await?;
            let fut = ws.next();
            match tokio::time::timeout(timeout, fut).await {
                Ok(Some(Ok(resp))) => {
                    let res_msg: Message = serde_json::from_str(&resp.into_text()?)?;
                    if res_msg.id == msg.id {
                        Ok(res_msg)
                    } else {
                        Err(GhostwriterError::Network("mismatched response".into()))
                    }
                }
                _ => {
                    self.ws = None;
                    self.status = ConnectionStatus::Disconnected;
                    self.queue.push(msg);
                    Err(GhostwriterError::Network("timeout".into()))
                }
            }
        } else {
            Err(GhostwriterError::Network("not connected".into()))
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serial_test::serial;
    use tempfile::tempdir;
    use tokio::time::Duration;

    use crate::files::workspace::WorkspaceManager;
    use crate::network::server::GhostwriterServer;

    #[tokio::test]
    #[serial]
    async fn test_client_connection() {
        let dir = tempdir().unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let server = GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, None)
            .await
            .unwrap();
        let addr = server.local_addr().unwrap();
        let handle = tokio::spawn(server.run());

        let mut client = GhostwriterClient::new(format!("ws://{}", addr), None).unwrap();
        client.connect().await.unwrap();
        assert_eq!(client.status(), ConnectionStatus::Connected);
        let resp = client
            .request(MessageKind::Ping, Duration::from_secs(1))
            .await
            .unwrap();
        assert!(matches!(resp.kind, MessageKind::Pong));

        handle.abort();
        let _ = handle.await;
        if let Some(mut ws) = client.ws.take() {
            let _ = ws.close(None).await;
        }
    }

    #[tokio::test]
    #[serial]
    async fn test_automatic_reconnection() {
        let dir = tempdir().unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let server = GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, None)
            .await
            .unwrap();
        let addr = server.local_addr().unwrap();
        let handle = tokio::spawn(server.run());

        let mut client = GhostwriterClient::new(format!("ws://{}", addr), None).unwrap();
        client.connect().await.unwrap();
        let _ = client
            .request(MessageKind::Ping, Duration::from_secs(1))
            .await
            .unwrap();

        handle.abort();
        let _ = handle.await;
        if let Some(mut ws) = client.ws.take() {
            let _ = ws.close(None).await;
        }

        // send request while offline - should fail and queue
        assert!(
            client
                .request(MessageKind::Ping, Duration::from_millis(100))
                .await
                .is_err()
        );

        let ws2 = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let server2 = GhostwriterServer::bind(addr, ws2, None).await.unwrap();
        let handle2 = tokio::spawn(server2.run());

        // this will reconnect and flush queue
        let resp = client
            .request(MessageKind::Ping, Duration::from_secs(2))
            .await
            .unwrap();
        assert!(matches!(resp.kind, MessageKind::Pong));

        handle2.abort();
        let _ = handle2.await;
    }

    #[tokio::test]
    #[serial]
    async fn test_operation_queueing() {
        let dir = tempdir().unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let server = GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, None)
            .await
            .unwrap();
        let addr = server.local_addr().unwrap();
        let handle = tokio::spawn(server.run());

        let mut client = GhostwriterClient::new(format!("ws://{}", addr), None).unwrap();
        client.connect().await.unwrap();

        handle.abort();
        let _ = handle.await;
        if let Some(mut ws) = client.ws.take() {
            let _ = ws.close(None).await;
        }

        let write = MessageKind::FileWriteRequest {
            path: "file.txt".into(),
            data: b"queued".to_vec(),
        };
        // offline queue
        assert!(
            client
                .request(write.clone(), Duration::from_millis(100))
                .await
                .is_err()
        );

        let ws2 = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let server2 = GhostwriterServer::bind(addr, ws2, None).await.unwrap();
        let handle2 = tokio::spawn(server2.run());

        // trigger reconnection and flush queue
        let _ = client
            .request(MessageKind::Ping, Duration::from_secs(2))
            .await
            .unwrap();

        let read = MessageKind::FileReadRequest {
            path: "file.txt".into(),
        };
        let resp = client.request(read, Duration::from_secs(1)).await.unwrap();
        if let MessageKind::FileReadResponse { data, .. } = resp.kind {
            assert_eq!(data.unwrap(), b"queued".to_vec());
        } else {
            panic!("unexpected response");
        }

        handle2.abort();
        let _ = handle2.await;
    }

    #[tokio::test]
    #[serial]
    async fn test_request_timeout_handling() {
        let dir = tempdir().unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let server = GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, None)
            .await
            .unwrap();
        let addr = server.local_addr().unwrap();
        let handle = tokio::spawn(server.run());

        let mut client = GhostwriterClient::new(format!("ws://{}", addr), None).unwrap();
        client.connect().await.unwrap();

        handle.abort();
        let _ = handle.await;
        if let Some(mut ws) = client.ws.take() {
            let _ = ws.close(None).await;
        }

        let res = client
            .request(MessageKind::Ping, Duration::from_millis(50))
            .await;
        assert!(res.is_err());
    }

    #[tokio::test]
    #[serial]
    async fn test_network_failure_recovery() {
        let dir = tempdir().unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let server = GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, None)
            .await
            .unwrap();
        let addr = server.local_addr().unwrap();
        let handle = tokio::spawn(server.run());

        let mut client = GhostwriterClient::new(format!("ws://{}", addr), None).unwrap();
        client.connect().await.unwrap();

        handle.abort();
        let _ = handle.await;
        if let Some(mut ws) = client.ws.take() {
            let _ = ws.close(None).await;
        }

        // request should fail and queue
        assert!(
            client
                .request(MessageKind::Ping, Duration::from_millis(100))
                .await
                .is_err()
        );

        let ws2 = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let server2 = GhostwriterServer::bind(addr, ws2, None).await.unwrap();
        let handle2 = tokio::spawn(server2.run());

        client.recover().await.unwrap();
        assert_eq!(client.status(), ConnectionStatus::Connected);

        let resp = client
            .request(MessageKind::Ping, Duration::from_secs(1))
            .await
            .unwrap();
        assert!(matches!(resp.kind, MessageKind::Pong));

        handle2.abort();
        let _ = handle2.await;
    }

    #[test]
    fn test_input_validation() {
        let res = GhostwriterClient::new("http://invalid".into(), None);
        assert!(res.is_err());
    }
}
