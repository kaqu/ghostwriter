use futures_util::{SinkExt, StreamExt, stream::SplitSink, stream::SplitStream};
use std::{sync::Arc, time::Instant};
use tokio::sync::{Mutex, mpsc};
use tokio::task::JoinHandle;
use tokio::time::Duration;
use tokio_tungstenite::{
    WebSocketStream,
    tungstenite::{Error as WsError, Message},
};

/// WebSocket transport wrapper providing binary send/recv and heartbeat.
pub struct Transport<S> {
    writer: Arc<Mutex<SplitSink<WebSocketStream<S>, Message>>>,
    rx: mpsc::UnboundedReceiver<Vec<u8>>,
    last_pong: Arc<Mutex<Instant>>,
    _reader: JoinHandle<()>,
    _pinger: JoinHandle<()>,
}

impl<S> Transport<S>
where
    S: tokio::io::AsyncRead + tokio::io::AsyncWrite + Unpin + Send + 'static,
{
    /// Create a new transport and start heartbeat with the given interval.
    pub fn new(ws: WebSocketStream<S>, ping_interval: Duration) -> Self {
        let (sink, mut stream): (
            SplitSink<WebSocketStream<S>, Message>,
            SplitStream<WebSocketStream<S>>,
        ) = ws.split();
        let writer = Arc::new(Mutex::new(sink));
        let (tx, rx) = mpsc::unbounded_channel();
        let last_pong = Arc::new(Mutex::new(Instant::now()));

        // Reader task handles incoming messages, responding to pings and
        // forwarding binary frames to the channel.
        let reader_writer = Arc::clone(&writer);
        let reader_last_pong = Arc::clone(&last_pong);
        let reader_handle = tokio::spawn(async move {
            while let Some(msg) = stream.next().await {
                match msg {
                    Ok(Message::Binary(data)) => {
                        if tx.send(data.to_vec()).is_err() {
                            break;
                        }
                    }
                    Ok(Message::Ping(data)) => {
                        let _ = reader_writer.lock().await.send(Message::Pong(data)).await;
                    }
                    Ok(Message::Pong(_)) => {
                        *reader_last_pong.lock().await = Instant::now();
                    }
                    Ok(Message::Close(_)) => break,
                    Ok(_) => {}
                    Err(_) => break,
                }
            }
        });

        // Pinger task periodically sends Ping frames.
        let pinger_writer = Arc::clone(&writer);
        let pinger_handle = tokio::spawn(async move {
            let mut ticker = tokio::time::interval(ping_interval);
            loop {
                ticker.tick().await;
                if pinger_writer
                    .lock()
                    .await
                    .send(Message::Ping(Vec::new().into()))
                    .await
                    .is_err()
                {
                    break;
                }
            }
        });

        Self {
            writer,
            rx,
            last_pong,
            _reader: reader_handle,
            _pinger: pinger_handle,
        }
    }

    /// Send binary data over the WebSocket.
    pub async fn send(&self, data: &[u8]) -> Result<(), WsError> {
        self.writer
            .lock()
            .await
            .send(Message::Binary(data.to_vec().into()))
            .await
    }

    /// Receive the next binary message, if any.
    pub async fn recv(&mut self) -> Option<Vec<u8>> {
        self.rx.recv().await
    }

    /// Get the instant of the most recent Pong frame.
    pub async fn last_pong(&self) -> Instant {
        *self.last_pong.lock().await
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::io::duplex;
    use tokio_tungstenite::{WebSocketStream, tungstenite::protocol::Role};

    #[tokio::test]
    async fn binary_roundtrip_and_heartbeat() {
        let (a, b) = duplex(64);
        let ws_a = WebSocketStream::from_raw_socket(a, Role::Client, None).await;
        let ws_b = WebSocketStream::from_raw_socket(b, Role::Server, None).await;

        let ta = Transport::new(ws_a, Duration::from_millis(50));
        let mut tb = Transport::new(ws_b, Duration::from_millis(50));

        let start_a = ta.last_pong().await;
        let start_b = tb.last_pong().await;
        tokio::time::sleep(Duration::from_millis(120)).await;
        assert!(ta.last_pong().await > start_a);
        assert!(tb.last_pong().await > start_b);

        ta.send(b"hello").await.expect("send");
        let msg = tb.recv().await.expect("recv");
        assert_eq!(msg, b"hello");
    }
}
