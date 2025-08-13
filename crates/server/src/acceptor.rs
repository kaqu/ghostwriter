use std::sync::{
    Arc,
    atomic::{AtomicBool, Ordering},
};

use argon2::{Argon2, PasswordHash, PasswordVerifier};
use futures_util::{SinkExt, StreamExt};
use ghostwriter_proto::{Auth, Envelope, ErrorCode, ErrorMsg, Hello, MessageType, decode, encode};
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::net::{TcpListener, UnixListener};
use tokio_tungstenite::{WebSocketStream, accept_async, tungstenite::Message};

async fn handle_busy<S>(mut ws: WebSocketStream<S>)
where
    S: AsyncRead + AsyncWrite + Unpin,
{
    let env = Envelope::new(
        MessageType::Error,
        ErrorMsg {
            code: ErrorCode::Busy,
            msg: "busy".into(),
        },
    );
    if let Ok(data) = encode(&env) {
        let _ = ws.send(Message::Binary(data.into())).await;
    }
    let _ = ws.close(None).await;
}

async fn handle_connection<S>(
    mut ws: WebSocketStream<S>,
    active: Arc<AtomicBool>,
    secret_hash: Option<String>,
) where
    S: AsyncRead + AsyncWrite + Unpin,
{
    // Expect Hello first
    if let Some(Ok(Message::Binary(data))) = ws.next().await {
        let _env: Envelope<Hello> = match decode(&data) {
            Ok(env) => env,
            Err(_) => {
                let _ = ws.close(None).await;
                active.store(false, Ordering::SeqCst);
                return;
            }
        };
    } else {
        let _ = ws.close(None).await;
        active.store(false, Ordering::SeqCst);
        return;
    }

    if let Some(hash) = secret_hash {
        match ws.next().await {
            Some(Ok(Message::Binary(data))) => {
                let env: Envelope<Auth> = match decode(&data) {
                    Ok(env) => env,
                    Err(_) => {
                        let _ = ws.close(None).await;
                        active.store(false, Ordering::SeqCst);
                        return;
                    }
                };
                let parsed = PasswordHash::new(&hash).expect("valid hash");
                let argon2 = Argon2::default();
                if argon2
                    .verify_password(env.data.secret.as_bytes(), &parsed)
                    .is_err()
                {
                    let env = Envelope::new(
                        MessageType::Error,
                        ErrorMsg {
                            code: ErrorCode::Unauthorized,
                            msg: "unauthorized".into(),
                        },
                    );
                    if let Ok(data) = encode(&env) {
                        let _ = ws.send(Message::Binary(data.into())).await;
                    }
                    let _ = ws.close(None).await;
                    active.store(false, Ordering::SeqCst);
                    return;
                }
            }
            _ => {
                let _ = ws.close(None).await;
                active.store(false, Ordering::SeqCst);
                return;
            }
        }
    }

    while let Some(msg) = ws.next().await {
        if msg.is_err() {
            break;
        }
    }
    active.store(false, Ordering::SeqCst);
}

pub async fn run_tcp(listener: TcpListener, secret_hash: Option<String>) -> tokio::io::Result<()> {
    let active = Arc::new(AtomicBool::new(false));
    loop {
        let (stream, _) = listener.accept().await?;
        let ws = accept_async(stream).await.map_err(std::io::Error::other)?;
        if active.load(Ordering::SeqCst) {
            handle_busy(ws).await;
        } else {
            active.store(true, Ordering::SeqCst);
            let active_clone = Arc::clone(&active);
            let hash = secret_hash.clone();
            tokio::spawn(async move { handle_connection(ws, active_clone, hash).await });
        }
    }
}

pub async fn run_uds(listener: UnixListener, secret_hash: Option<String>) -> tokio::io::Result<()> {
    let active = Arc::new(AtomicBool::new(false));
    loop {
        let (stream, _) = listener.accept().await?;
        let ws = accept_async(stream).await.map_err(std::io::Error::other)?;
        if active.load(Ordering::SeqCst) {
            handle_busy(ws).await;
        } else {
            active.store(true, Ordering::SeqCst);
            let active_clone = Arc::clone(&active);
            let hash = secret_hash.clone();
            tokio::spawn(async move { handle_connection(ws, active_clone, hash).await });
        }
    }
}
