use anyhow::Result;
use futures_util::SinkExt;
use ghostwriter_proto::{Envelope, Hello, MessageType, RequestFrame, Resize, encode};
use tokio::net::TcpStream;
use tokio_tungstenite::{MaybeTlsStream, WebSocketStream, connect_async, tungstenite::Message};
use url::Url;

/// WebSocket client that communicates with the Ghostwriter server.
pub struct WsClient {
    ws: WebSocketStream<MaybeTlsStream<TcpStream>>,
}

impl WsClient {
    /// Connect to `url` and perform the Hello handshake. Sends a `RequestFrame`
    /// with reason `"initial"` after connecting.
    pub async fn connect(url: &str, cols: u16, rows: u16) -> Result<Self> {
        let url = Url::parse(url)?;
        let (mut ws, _resp) = connect_async(url.as_str()).await?;

        let hello = Hello {
            client_name: "ghostwriter".into(),
            client_ver: env!("CARGO_PKG_VERSION").into(),
            cols,
            rows,
            truecolor: true,
        };
        let env = Envelope::new(MessageType::Hello, hello);
        ws.send(Message::Binary(encode(&env)?.into())).await?;

        let req = RequestFrame {
            reason: "initial".into(),
        };
        let env = Envelope::new(MessageType::RequestFrame, req);
        ws.send(Message::Binary(encode(&env)?.into())).await?;

        Ok(Self { ws })
    }

    /// Notify the server that the viewport has been resized and request a new frame.
    pub async fn resize(&mut self, cols: u16, rows: u16) -> Result<()> {
        let resize = Resize { cols, rows };
        let env = Envelope::new(MessageType::Resize, resize);
        self.ws.send(Message::Binary(encode(&env)?.into())).await?;

        let req = RequestFrame {
            reason: "resize".into(),
        };
        let env = Envelope::new(MessageType::RequestFrame, req);
        self.ws.send(Message::Binary(encode(&env)?.into())).await?;
        Ok(())
    }
}
