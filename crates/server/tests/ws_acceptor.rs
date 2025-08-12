use futures_util::StreamExt;
use ghostwriter_proto::{Envelope, ErrorCode, ErrorMsg, MessageType, decode};
use ghostwriter_server::acceptor;
use tokio::net::TcpListener;
use tokio_tungstenite::tungstenite::Message;

#[tokio::test]
async fn rejects_second_client_with_busy() {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let server = tokio::spawn(async move {
        acceptor::run_tcp(listener).await.unwrap();
    });

    let (mut ws1, _) = tokio_tungstenite::connect_async(format!("ws://{addr}"))
        .await
        .unwrap();

    let (mut ws2, _) = tokio_tungstenite::connect_async(format!("ws://{addr}"))
        .await
        .unwrap();

    match ws2.next().await.unwrap().unwrap() {
        Message::Binary(data) => {
            let env: Envelope<ErrorMsg> = decode(&data).unwrap();
            assert_eq!(env.ty, MessageType::Error);
            assert_eq!(env.data.code, ErrorCode::Busy);
        }
        other => panic!("unexpected message: {other:?}"),
    }
    match ws2.next().await {
        Some(Ok(Message::Close(_))) | None => {}
        other => panic!("unexpected message: {other:?}"),
    }

    ws1.close(None).await.unwrap();
    let (mut ws3, _) = tokio_tungstenite::connect_async(format!("ws://{addr}"))
        .await
        .unwrap();
    ws3.close(None).await.unwrap();

    server.abort();
}
