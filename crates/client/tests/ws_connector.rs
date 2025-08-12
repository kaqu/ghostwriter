use futures_util::StreamExt;
use ghostwriter_client::remote::WsClient;
use ghostwriter_proto::{Envelope, Hello, MessageType, RequestFrame, Resize, decode};
use tokio::net::TcpListener;
use tokio_tungstenite::accept_async;

#[tokio::test]
async fn hello_and_request_frame_on_connect_and_resize() {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();

    let server = tokio::spawn(async move {
        let (stream, _) = listener.accept().await.unwrap();
        let mut ws = accept_async(stream).await.unwrap();

        // Hello
        let msg = ws.next().await.unwrap().unwrap();
        let env: Envelope<Hello> = decode(&msg.into_data()).unwrap();
        assert_eq!(env.ty, MessageType::Hello);

        // RequestFrame (initial)
        let msg = ws.next().await.unwrap().unwrap();
        let env: Envelope<RequestFrame> = decode(&msg.into_data()).unwrap();
        assert_eq!(env.data.reason, "initial");

        // Resize
        let msg = ws.next().await.unwrap().unwrap();
        let env: Envelope<Resize> = decode(&msg.into_data()).unwrap();
        assert_eq!(env.data.cols, 100);
        assert_eq!(env.data.rows, 50);

        // RequestFrame (resize)
        let msg = ws.next().await.unwrap().unwrap();
        let env: Envelope<RequestFrame> = decode(&msg.into_data()).unwrap();
        assert_eq!(env.data.reason, "resize");
    });

    let url = format!("ws://{addr}");
    let mut client = WsClient::connect(&url, 80, 24).await.unwrap();
    client.resize(100, 50).await.unwrap();

    server.await.unwrap();
}
