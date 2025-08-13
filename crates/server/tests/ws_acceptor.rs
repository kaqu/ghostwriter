use argon2::password_hash::SaltString;
use argon2::{Argon2, PasswordHasher};
use futures_util::{SinkExt, StreamExt};
use ghostwriter_proto::{Auth, Envelope, ErrorCode, ErrorMsg, Hello, MessageType, decode, encode};
use ghostwriter_server::acceptor;
use rand_core::OsRng;
use tokio::net::TcpListener;
use tokio_tungstenite::tungstenite::Message;

#[tokio::test]
async fn rejects_second_client_with_busy() {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let server = tokio::spawn(async move {
        acceptor::run_tcp(listener, None).await.unwrap();
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

#[tokio::test]
async fn rejects_invalid_auth() {
    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();

    let salt = SaltString::generate(&mut OsRng);
    let argon2 = Argon2::default();
    let hash = argon2
        .hash_password("s3cr3t".as_bytes(), &salt)
        .unwrap()
        .to_string();

    let server = tokio::spawn(async move {
        acceptor::run_tcp(listener, Some(hash)).await.unwrap();
    });

    let (mut ws, _) = tokio_tungstenite::connect_async(format!("ws://{addr}"))
        .await
        .unwrap();

    // Send Hello
    let hello = Hello {
        client_name: "c".into(),
        client_ver: "1".into(),
        cols: 80,
        rows: 24,
        truecolor: true,
    };
    let env = Envelope::new(MessageType::Hello, hello);
    ws.send(Message::Binary(encode(&env).unwrap().into()))
        .await
        .unwrap();

    // Send wrong Auth
    let auth = Auth {
        secret: "bad".into(),
    };
    let env = Envelope::new(MessageType::Auth, auth);
    ws.send(Message::Binary(encode(&env).unwrap().into()))
        .await
        .unwrap();

    match ws.next().await.unwrap().unwrap() {
        Message::Binary(data) => {
            let env: Envelope<ErrorMsg> = decode(&data).unwrap();
            assert_eq!(env.data.code, ErrorCode::Unauthorized);
        }
        other => panic!("unexpected: {other:?}"),
    }

    server.abort();
}

#[tokio::test]
async fn accepts_valid_auth() {
    use tokio::time::{Duration, timeout};

    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();

    let salt = SaltString::generate(&mut OsRng);
    let argon2 = Argon2::default();
    let hash = argon2
        .hash_password("s3cr3t".as_bytes(), &salt)
        .unwrap()
        .to_string();

    let server = tokio::spawn(async move {
        acceptor::run_tcp(listener, Some(hash)).await.unwrap();
    });

    let (mut ws, _) = tokio_tungstenite::connect_async(format!("ws://{addr}"))
        .await
        .unwrap();

    // Hello
    let hello = Hello {
        client_name: "c".into(),
        client_ver: "1".into(),
        cols: 80,
        rows: 24,
        truecolor: true,
    };
    let env = Envelope::new(MessageType::Hello, hello);
    ws.send(Message::Binary(encode(&env).unwrap().into()))
        .await
        .unwrap();

    // Correct Auth
    let auth = Auth {
        secret: "s3cr3t".into(),
    };
    let env = Envelope::new(MessageType::Auth, auth);
    ws.send(Message::Binary(encode(&env).unwrap().into()))
        .await
        .unwrap();

    // Ensure no error is sent within 100ms
    assert!(
        timeout(Duration::from_millis(100), ws.next())
            .await
            .is_err()
    );

    ws.close(None).await.unwrap();
    server.abort();
}

#[tokio::test]
async fn rate_limits_connections() {
    use tokio::time::{Duration, sleep, timeout};

    let listener = TcpListener::bind("127.0.0.1:0").await.unwrap();
    let addr = listener.local_addr().unwrap();
    let server = tokio::spawn(async move {
        acceptor::run_tcp(listener, None).await.unwrap();
    });

    // Three quick connections should succeed
    for _ in 0..3 {
        let (mut ws, _) = tokio_tungstenite::connect_async(format!("ws://{addr}"))
            .await
            .unwrap();
        let hello = Hello {
            client_name: "c".into(),
            client_ver: "1".into(),
            cols: 80,
            rows: 24,
            truecolor: true,
        };
        let env = Envelope::new(MessageType::Hello, hello);
        ws.send(Message::Binary(encode(&env).unwrap().into()))
            .await
            .unwrap();
        ws.close(None).await.unwrap();
        // Give the server a moment to clean up
        sleep(Duration::from_millis(10)).await;
    }

    // Fourth connection should be rate-limited
    let (mut ws, _) = tokio_tungstenite::connect_async(format!("ws://{addr}"))
        .await
        .unwrap();

    match timeout(Duration::from_millis(200), ws.next()).await {
        Ok(Some(Ok(Message::Binary(data)))) => {
            let env: Envelope<ErrorMsg> = decode(&data).unwrap();
            assert_eq!(env.ty, MessageType::Error);
            assert_eq!(env.data.code, ErrorCode::RateLimit);
            assert!(env.data.msg.contains("retry"));
        }
        other => panic!("unexpected message: {other:?}"),
    }
    match timeout(Duration::from_millis(200), ws.next()).await {
        Ok(Some(Ok(Message::Close(_)))) | Ok(None) => {}
        other => panic!("unexpected message: {other:?}"),
    }

    server.abort();
}
