use std::sync::{
    Arc,
    atomic::{AtomicBool, Ordering},
};

use futures_util::{SinkExt, StreamExt};
use ghostwriter_proto::{Envelope, ErrorCode, ErrorMsg, MessageType, encode};
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

async fn handle_connection<S>(mut ws: WebSocketStream<S>, active: Arc<AtomicBool>)
where
    S: AsyncRead + AsyncWrite + Unpin,
{
    while let Some(msg) = ws.next().await {
        if msg.is_err() {
            break;
        }
    }
    active.store(false, Ordering::SeqCst);
}

pub async fn run_tcp(listener: TcpListener) -> tokio::io::Result<()> {
    let active = Arc::new(AtomicBool::new(false));
    loop {
        let (stream, _) = listener.accept().await?;
        let ws = accept_async(stream).await.map_err(std::io::Error::other)?;
        if active.load(Ordering::SeqCst) {
            handle_busy(ws).await;
        } else {
            active.store(true, Ordering::SeqCst);
            let active_clone = Arc::clone(&active);
            tokio::spawn(async move { handle_connection(ws, active_clone).await });
        }
    }
}

pub async fn run_uds(listener: UnixListener) -> tokio::io::Result<()> {
    let active = Arc::new(AtomicBool::new(false));
    loop {
        let (stream, _) = listener.accept().await?;
        let ws = accept_async(stream).await.map_err(std::io::Error::other)?;
        if active.load(Ordering::SeqCst) {
            handle_busy(ws).await;
        } else {
            active.store(true, Ordering::SeqCst);
            let active_clone = Arc::clone(&active);
            tokio::spawn(async move { handle_connection(ws, active_clone).await });
        }
    }
}
