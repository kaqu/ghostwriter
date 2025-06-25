// WebSocket server implementation

use std::net::SocketAddr;
use std::path::{Path, PathBuf};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use argon2::{
    Argon2,
    password_hash::{PasswordHash, PasswordHasher, PasswordVerifier, SaltString},
};
use futures_util::{SinkExt, StreamExt};
use rand::rngs::OsRng;
use tokio::net::{TcpListener, TcpStream};
use tokio_tungstenite::{accept_async, tungstenite::Message as WsMessage};
use uuid::Uuid;

use crate::error::{GhostwriterError, Result};
use crate::files::file_lock::FileLock;
use crate::files::file_manager::{FileContents, FileManager};
use crate::files::workspace::WorkspaceManager;
use crate::network::protocol::{Message, MessageKind};

#[allow(dead_code)]
pub struct GhostwriterServer {
    listener: TcpListener,
    ws: WorkspaceManager,
    pass_hash: Option<String>,
    client_active: Arc<std::sync::atomic::AtomicBool>,
    lock: Arc<Mutex<Option<FileLock>>>,
}

#[allow(dead_code)]
impl GhostwriterServer {
    pub async fn bind(
        addr: SocketAddr,
        ws: WorkspaceManager,
        pass: Option<String>,
    ) -> Result<Self> {
        let listener = TcpListener::bind(addr).await?;
        let pass_hash = if let Some(p) = pass {
            let salt = SaltString::generate(&mut OsRng);
            let hash = Argon2::default()
                .hash_password(p.as_bytes(), &salt)
                .map_err(|e| GhostwriterError::InvalidArgument(e.to_string()))?
                .to_string();
            Some(hash)
        } else {
            None
        };
        Ok(Self {
            listener,
            ws,
            pass_hash,
            client_active: Arc::new(std::sync::atomic::AtomicBool::new(false)),
            lock: Arc::new(Mutex::new(None)),
        })
    }

    pub fn local_addr(&self) -> Result<SocketAddr> {
        Ok(self.listener.local_addr()?)
    }

    pub async fn run(self) -> Result<()> {
        loop {
            let (stream, _) = self.listener.accept().await?;
            if self.client_active.load(std::sync::atomic::Ordering::SeqCst) {
                let mut ws = accept_async(stream).await?;
                let err = Message {
                    id: Uuid::new_v4(),
                    kind: MessageKind::Error {
                        context: "server busy".into(),
                    },
                };
                ws.send(WsMessage::Text(serde_json::to_string(&err).unwrap().into()))
                    .await?;
                let _ = ws.close(None).await;
                continue;
            }
            self.client_active
                .store(true, std::sync::atomic::Ordering::SeqCst);
            let ws_man = self.ws.clone();
            let lock = self.lock.clone();
            let pass = self.pass_hash.clone();
            let active = self.client_active.clone();
            tokio::spawn(async move {
                let res = handle_client(stream, ws_man, lock, pass).await;
                active.store(false, std::sync::atomic::Ordering::SeqCst);
                if let Err(e) = res {
                    eprintln!("client error: {e}");
                }
            });
        }
    }
}

fn resolve_existing(ws: &WorkspaceManager, path: &Path) -> Result<PathBuf> {
    let joined = if path.is_absolute() {
        PathBuf::from(path)
    } else {
        ws.root().join(path)
    };
    let canonical = joined.canonicalize()?;
    if !canonical.starts_with(ws.root()) {
        return Err(GhostwriterError::InvalidArgument(
            "path outside workspace".into(),
        ));
    }
    Ok(canonical)
}

fn resolve_new(ws: &WorkspaceManager, path: &Path) -> Result<PathBuf> {
    let joined = if path.is_absolute() {
        PathBuf::from(path)
    } else {
        ws.root().join(path)
    };
    let parent = joined
        .parent()
        .ok_or_else(|| GhostwriterError::InvalidArgument("invalid path".into()))?;
    let canonical_parent = parent.canonicalize()?;
    if !canonical_parent.starts_with(ws.root()) {
        return Err(GhostwriterError::InvalidArgument(
            "path outside workspace".into(),
        ));
    }
    Ok(canonical_parent.join(joined.file_name().unwrap()))
}

async fn handle_client(
    stream: TcpStream,
    ws: WorkspaceManager,
    lock: Arc<Mutex<Option<FileLock>>>,
    pass_hash: Option<String>,
) -> Result<()> {
    let mut ws_stream = accept_async(stream).await?;
    if let Some(hash) = pass_hash {
        let msg = ws_stream
            .next()
            .await
            .ok_or_else(|| GhostwriterError::Network("no auth".into()))??;
        let txt = msg.into_text()?;
        let req: Message = serde_json::from_str(&txt)?;
        let key = match req.kind {
            MessageKind::AuthRequest { key } => key.unwrap_or_default(),
            _ => String::new(),
        };
        let parsed = PasswordHash::new(&hash).unwrap();
        let valid = Argon2::default()
            .verify_password(key.as_bytes(), &parsed)
            .is_ok();
        let resp = Message {
            id: req.id,
            kind: MessageKind::AuthResponse {
                success: valid,
                reason: if valid {
                    None
                } else {
                    Some("invalid key".into())
                },
            },
        };
        ws_stream
            .send(WsMessage::Text(
                serde_json::to_string(&resp).unwrap().into(),
            ))
            .await?;
        if !valid {
            let _ = ws_stream.close(None).await;
            return Ok(());
        }
    } else if let Some(msg) = ws_stream.next().await {
        let msg = msg?;
        if msg.is_text() {
            let txt = msg.into_text()?;
            if let Ok(req) = serde_json::from_str::<Message>(&txt) {
                if matches!(req.kind, MessageKind::AuthRequest { .. }) {
                    let resp = Message {
                        id: req.id,
                        kind: MessageKind::AuthResponse {
                            success: true,
                            reason: None,
                        },
                    };
                    ws_stream
                        .send(WsMessage::Text(
                            serde_json::to_string(&resp).unwrap().into(),
                        ))
                        .await?;
                }
            }
        } else if msg.is_close() {
            let _ = ws_stream.close(None).await;
            return Ok(());
        }
    }

    while let Some(msg) = ws_stream.next().await {
        let msg = msg?;
        if msg.is_text() {
            let txt = msg.into_text()?;
            let req: Message = serde_json::from_str(&txt)?;
            match req.kind {
                MessageKind::Ping => {
                    let resp = Message {
                        id: req.id,
                        kind: MessageKind::Pong,
                    };
                    ws_stream
                        .send(WsMessage::Text(
                            serde_json::to_string(&resp).unwrap().into(),
                        ))
                        .await?;
                }
                MessageKind::FileReadRequest { path } => {
                    let full = resolve_existing(&ws, Path::new(&path));
                    let resp = match full.and_then(|p| FileManager::read(&p)) {
                        Ok(FileContents::InMemory(data)) => Message {
                            id: req.id,
                            kind: MessageKind::FileReadResponse {
                                success: true,
                                data: Some(data),
                                reason: None,
                            },
                        },
                        Ok(FileContents::Mapped(m)) => Message {
                            id: req.id,
                            kind: MessageKind::FileReadResponse {
                                success: true,
                                data: Some(m.as_ref().to_vec()),
                                reason: None,
                            },
                        },
                        Err(e) => Message {
                            id: req.id,
                            kind: MessageKind::FileReadResponse {
                                success: false,
                                data: None,
                                reason: Some(e.to_string()),
                            },
                        },
                    };
                    ws_stream
                        .send(WsMessage::Text(
                            serde_json::to_string(&resp).unwrap().into(),
                        ))
                        .await?;
                }
                MessageKind::FileWriteRequest { path, data } => {
                    let full = resolve_new(&ws, Path::new(&path));
                    let res = full.and_then(|p| FileManager::atomic_write(&p, &data));
                    let resp = match res {
                        Ok(_) => Message {
                            id: req.id,
                            kind: MessageKind::FileWriteResponse {
                                success: true,
                                reason: None,
                            },
                        },
                        Err(e) => Message {
                            id: req.id,
                            kind: MessageKind::FileWriteResponse {
                                success: false,
                                reason: Some(e.to_string()),
                            },
                        },
                    };
                    ws_stream
                        .send(WsMessage::Text(
                            serde_json::to_string(&resp).unwrap().into(),
                        ))
                        .await?;
                }
                MessageKind::DirListRequest { path } => {
                    let resp = match ws.list_dir(Path::new(&path)) {
                        Ok(entries) => Message {
                            id: req.id,
                            kind: MessageKind::DirListResponse {
                                entries: Some(entries),
                                reason: None,
                            },
                        },
                        Err(e) => Message {
                            id: req.id,
                            kind: MessageKind::DirListResponse {
                                entries: None,
                                reason: Some(e.to_string()),
                            },
                        },
                    };
                    ws_stream
                        .send(WsMessage::Text(
                            serde_json::to_string(&resp).unwrap().into(),
                        ))
                        .await?;
                }
                MessageKind::LockRequest { path } => {
                    let resp_json;
                    {
                        let mut guard = lock.lock().unwrap();
                        resp_json = if guard.is_some() {
                            let resp = Message {
                                id: req.id,
                                kind: MessageKind::LockResponse {
                                    success: false,
                                    readonly: false,
                                    reason: Some("already locked".into()),
                                },
                            };
                            serde_json::to_string(&resp).unwrap()
                        } else {
                            let full = resolve_existing(&ws, Path::new(&path));
                            let res =
                                full.and_then(|p| FileLock::acquire(&p, Duration::from_secs(5)));
                            let resp = match res {
                                Ok(l) => {
                                    *guard = Some(l);
                                    let readonly = guard.as_ref().unwrap().readonly();
                                    Message {
                                        id: req.id,
                                        kind: MessageKind::LockResponse {
                                            success: true,
                                            readonly,
                                            reason: None,
                                        },
                                    }
                                }
                                Err(e) => Message {
                                    id: req.id,
                                    kind: MessageKind::LockResponse {
                                        success: false,
                                        readonly: false,
                                        reason: Some(e.to_string()),
                                    },
                                },
                            };
                            serde_json::to_string(&resp).unwrap()
                        };
                    }
                    ws_stream.send(WsMessage::Text(resp_json.into())).await?;
                }
                _ => {}
            }
        } else if msg.is_close() {
            let _ = ws_stream.close(None).await;
            break;
        }
    }
    let mut guard = lock.lock().unwrap();
    *guard = None;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use serial_test::serial;
    use tempfile::tempdir;
    use tokio_tungstenite::connect_async;

    #[tokio::test]
    #[serial]
    async fn test_single_client_enforcement() {
        let dir = tempdir().unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        std::fs::write(dir.path().join("file.txt"), b"data").unwrap();
        let server = GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, None)
            .await
            .unwrap();
        let addr = server.local_addr().unwrap();
        let handle = tokio::spawn(server.run());

        let (mut c1, _) = connect_async(format!("ws://{}", addr)).await.unwrap();
        let auth = Message {
            id: Uuid::new_v4(),
            kind: MessageKind::AuthRequest { key: None },
        };
        c1.send(WsMessage::Text(
            serde_json::to_string(&auth).unwrap().into(),
        ))
        .await
        .unwrap();
        let _ = c1.next().await.unwrap().unwrap();

        let (mut c2, _) = connect_async(format!("ws://{}", addr)).await.unwrap();
        let resp = c2.next().await.unwrap().unwrap();
        let msg: Message = serde_json::from_str(&resp.into_text().unwrap()).unwrap();
        assert!(matches!(msg.kind, MessageKind::Error { .. }));
        c1.close(None).await.unwrap();
        handle.abort();
        let _ = handle.await;
    }

    #[tokio::test]
    #[serial]
    async fn test_authentication_flow() {
        let dir = tempdir().unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let server =
            GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, Some("secret".into()))
                .await
                .unwrap();
        let addr = server.local_addr().unwrap();
        let handle = tokio::spawn(server.run());

        let (mut wrong, _) = connect_async(format!("ws://{}", addr)).await.unwrap();
        let req = Message {
            id: Uuid::new_v4(),
            kind: MessageKind::AuthRequest {
                key: Some("bad".into()),
            },
        };
        wrong
            .send(WsMessage::Text(serde_json::to_string(&req).unwrap().into()))
            .await
            .unwrap();
        let resp = wrong.next().await.unwrap().unwrap();
        let msg: Message = serde_json::from_str(&resp.into_text().unwrap()).unwrap();
        if let MessageKind::AuthResponse { success, .. } = msg.kind {
            assert!(!success);
        }

        let (mut okc, _) = connect_async(format!("ws://{}", addr)).await.unwrap();
        let req = Message {
            id: Uuid::new_v4(),
            kind: MessageKind::AuthRequest {
                key: Some("secret".into()),
            },
        };
        okc.send(WsMessage::Text(serde_json::to_string(&req).unwrap().into()))
            .await
            .unwrap();
        let resp = okc.next().await.unwrap().unwrap();
        let msg: Message = serde_json::from_str(&resp.into_text().unwrap()).unwrap();
        if let MessageKind::AuthResponse { success, .. } = msg.kind {
            assert!(success);
        }
        wrong.close(None).await.unwrap();
        okc.close(None).await.unwrap();
        handle.abort();
        let _ = handle.await;
    }

    #[tokio::test]
    #[serial]
    async fn test_file_operations() {
        let dir = tempdir().unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let server = GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, None)
            .await
            .unwrap();
        let addr = server.local_addr().unwrap();
        let handle = tokio::spawn(server.run());

        let (mut client, _) = connect_async(format!("ws://{}", addr)).await.unwrap();
        let auth = Message {
            id: Uuid::new_v4(),
            kind: MessageKind::AuthRequest { key: None },
        };
        client
            .send(WsMessage::Text(
                serde_json::to_string(&auth).unwrap().into(),
            ))
            .await
            .unwrap();
        let _ = client.next().await.unwrap().unwrap();

        let write = Message {
            id: Uuid::new_v4(),
            kind: MessageKind::FileWriteRequest {
                path: "file.txt".into(),
                data: b"hello".to_vec(),
            },
        };
        client
            .send(WsMessage::Text(
                serde_json::to_string(&write).unwrap().into(),
            ))
            .await
            .unwrap();
        let resp = client.next().await.unwrap().unwrap();
        let msg: Message = serde_json::from_str(&resp.into_text().unwrap()).unwrap();
        assert!(matches!(
            msg.kind,
            MessageKind::FileWriteResponse { success: true, .. }
        ));

        let read = Message {
            id: Uuid::new_v4(),
            kind: MessageKind::FileReadRequest {
                path: "file.txt".into(),
            },
        };
        client
            .send(WsMessage::Text(
                serde_json::to_string(&read).unwrap().into(),
            ))
            .await
            .unwrap();
        let resp = client.next().await.unwrap().unwrap();
        let msg: Message = serde_json::from_str(&resp.into_text().unwrap()).unwrap();
        if let MessageKind::FileReadResponse { success, data, .. } = msg.kind {
            assert!(success);
            assert_eq!(data.unwrap(), b"hello".to_vec());
        }

        let list = Message {
            id: Uuid::new_v4(),
            kind: MessageKind::DirListRequest { path: ".".into() },
        };
        client
            .send(WsMessage::Text(
                serde_json::to_string(&list).unwrap().into(),
            ))
            .await
            .unwrap();
        let resp = client.next().await.unwrap().unwrap();
        let msg: Message = serde_json::from_str(&resp.into_text().unwrap()).unwrap();
        if let MessageKind::DirListResponse { entries, .. } = msg.kind {
            assert_eq!(entries.unwrap().len(), 1);
        }

        let lock_msg = Message {
            id: Uuid::new_v4(),
            kind: MessageKind::LockRequest {
                path: "file.txt".into(),
            },
        };
        client
            .send(WsMessage::Text(
                serde_json::to_string(&lock_msg).unwrap().into(),
            ))
            .await
            .unwrap();
        let resp = client.next().await.unwrap().unwrap();
        let msg: Message = serde_json::from_str(&resp.into_text().unwrap()).unwrap();
        assert!(matches!(
            msg.kind,
            MessageKind::LockResponse { success: true, .. }
        ));
        client.close(None).await.unwrap();
        let _ = client.close(None).await;
        handle.abort();
        let _ = handle.await;
    }

    #[tokio::test]
    #[serial]
    async fn test_automatic_lock_cleanup() {
        let dir = tempdir().unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        std::fs::write(dir.path().join("file.txt"), b"data").unwrap();
        let server = GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, None)
            .await
            .unwrap();
        let addr = server.local_addr().unwrap();
        let handle = tokio::spawn(server.run());

        let (mut c1, _) = connect_async(format!("ws://{}", addr)).await.unwrap();
        let auth = Message {
            id: Uuid::new_v4(),
            kind: MessageKind::AuthRequest { key: None },
        };
        c1.send(WsMessage::Text(
            serde_json::to_string(&auth).unwrap().into(),
        ))
        .await
        .unwrap();
        let _ = c1.next().await.unwrap().unwrap();
        let lock_msg = Message {
            id: Uuid::new_v4(),
            kind: MessageKind::LockRequest {
                path: "file.txt".into(),
            },
        };
        c1.send(WsMessage::Text(
            serde_json::to_string(&lock_msg).unwrap().into(),
        ))
        .await
        .unwrap();
        let _ = c1.next().await.unwrap().unwrap();
        c1.close(None).await.unwrap();
        tokio::time::sleep(Duration::from_millis(100)).await;

        let (mut c2, _) = connect_async(format!("ws://{}", addr)).await.unwrap();
        c2.send(WsMessage::Text(
            serde_json::to_string(&auth).unwrap().into(),
        ))
        .await
        .unwrap();
        let _ = c2.next().await.unwrap().unwrap();
        c2.send(WsMessage::Text(
            serde_json::to_string(&lock_msg).unwrap().into(),
        ))
        .await
        .unwrap();
        let resp = c2.next().await.unwrap().unwrap();
        let msg: Message = serde_json::from_str(&resp.into_text().unwrap()).unwrap();
        assert!(matches!(
            msg.kind,
            MessageKind::LockResponse { success: true, .. }
        ));
        c2.close(None).await.unwrap();
        handle.abort();
        let _ = handle.await;
    }
}
