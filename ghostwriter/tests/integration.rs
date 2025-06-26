use clap::Parser;
use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ghostwriter::cli::Args;
use ghostwriter::editor::{cursor::Cursor, key_handler::KeyHandler, rope::Rope};
use ghostwriter::files::file_manager::{FileContents, FileManager};
use ghostwriter::files::workspace::WorkspaceManager;
use ghostwriter::network::protocol::MessageKind;
use ghostwriter::network::{client::GhostwriterClient, server::GhostwriterServer};
use serial_test::serial;
use std::time::Duration;

#[test]
fn test_complete_local_editing_session() {
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("file.txt");
    std::fs::write(&path, b"start").unwrap();
    let args = Args::try_parse_from(["ghostwriter", path.to_str().unwrap()]).unwrap();
    args.validate().unwrap();

    let mut rope = match FileManager::read(&path).unwrap() {
        FileContents::InMemory(d) => Rope::from_bytes(&d),
        FileContents::Mapped(m) => Rope::from_bytes(m.as_ref()),
    };
    let mut handler = KeyHandler::new();
    let mut cursor = Cursor::new();
    cursor.move_doc_end(&rope);
    let mut sel = None;
    handler.handle(
        KeyEvent::new(KeyCode::Char('!'), KeyModifiers::empty()),
        &mut rope,
        &mut cursor,
        &mut sel,
    );
    FileManager::atomic_write(&path, rope.as_string().as_bytes()).unwrap();
    let result = std::fs::read_to_string(&path).unwrap();
    assert_eq!(result, "start!");
}

#[tokio::test]
#[serial]
async fn test_complete_remote_editing_session() {
    let dir = tempfile::tempdir().unwrap();
    let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
    std::fs::write(dir.path().join("file.txt"), b"hello").unwrap();

    let server = GhostwriterServer::bind(
        "127.0.0.1:0".parse().unwrap(),
        ws.clone(),
        Some("secret".into()),
    )
    .await
    .unwrap();
    let addr = server.local_addr().unwrap();
    let handle = tokio::spawn(server.run());

    let mut client =
        GhostwriterClient::new(format!("ws://{}", addr), Some("secret".into())).unwrap();
    client.connect().await.unwrap();

    let resp = client
        .request(
            MessageKind::FileReadRequest {
                path: "file.txt".into(),
            },
            Duration::from_secs(1),
        )
        .await
        .unwrap();
    let mut rope = if let MessageKind::FileReadResponse {
        success: true,
        data: Some(data),
        ..
    } = resp.kind
    {
        Rope::from_bytes(&data)
    } else {
        panic!("bad response");
    };
    let mut handler = KeyHandler::new();
    let mut cursor = Cursor::new();
    cursor.move_doc_end(&rope);
    let mut sel = None;
    handler.handle(
        KeyEvent::new(KeyCode::Char('!'), KeyModifiers::empty()),
        &mut rope,
        &mut cursor,
        &mut sel,
    );
    client
        .request(
            MessageKind::FileWriteRequest {
                path: "file.txt".into(),
                data: rope.as_string().into_bytes(),
            },
            Duration::from_secs(1),
        )
        .await
        .unwrap();

    handle.abort();
    let _ = handle.await;

    let result = std::fs::read_to_string(dir.path().join("file.txt")).unwrap();
    assert_eq!(result, "hello!");
}

#[tokio::test]
#[serial]
async fn test_file_operations_integration() {
    let dir = tempfile::tempdir().unwrap();
    let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
    let server = GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws.clone(), None)
        .await
        .unwrap();
    let addr = server.local_addr().unwrap();
    let handle = tokio::spawn(server.run());

    let mut client = GhostwriterClient::new(format!("ws://{}", addr), None).unwrap();
    client.connect().await.unwrap();
    client
        .request(
            MessageKind::FileWriteRequest {
                path: "file1.txt".into(),
                data: b"data".to_vec(),
            },
            Duration::from_secs(1),
        )
        .await
        .unwrap();
    handle.abort();
    let _ = handle.await;

    ws.rename(
        std::path::Path::new("file1.txt"),
        std::path::Path::new("file2.txt"),
    )
    .unwrap();
    let ws2 = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
    let server2 = GhostwriterServer::bind(addr, ws2.clone(), None)
        .await
        .unwrap();
    let handle2 = tokio::spawn(server2.run());

    let mut client2 = GhostwriterClient::new(format!("ws://{}", addr), None).unwrap();
    client2.connect().await.unwrap();
    let resp = client2
        .request(
            MessageKind::DirListRequest { path: ".".into() },
            Duration::from_secs(1),
        )
        .await
        .unwrap();
    if let MessageKind::DirListResponse {
        entries: Some(list),
        ..
    } = resp.kind
    {
        assert!(list.iter().any(|e| e.name == "file2.txt"));
    } else {
        panic!("dir list failed");
    }
    handle2.abort();
    let _ = handle2.await;

    ws2.delete(std::path::Path::new("file2.txt")).unwrap();
    let ws3 = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
    let server3 = GhostwriterServer::bind(addr, ws3.clone(), None)
        .await
        .unwrap();
    let handle3 = tokio::spawn(server3.run());

    let mut client3 = GhostwriterClient::new(format!("ws://{}", addr), None).unwrap();
    client3.connect().await.unwrap();
    let resp = client3
        .request(
            MessageKind::DirListRequest { path: ".".into() },
            Duration::from_secs(1),
        )
        .await
        .unwrap();
    if let MessageKind::DirListResponse {
        entries: Some(list),
        ..
    } = resp.kind
    {
        assert!(!list.iter().any(|e| e.name == "file2.txt"));
    }
    handle3.abort();
    let _ = handle3.await;
}

#[tokio::test]
#[serial]
async fn test_authentication_integration() {
    let dir = tempfile::tempdir().unwrap();
    let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
    let server = GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, Some("pass".into()))
        .await
        .unwrap();
    let addr = server.local_addr().unwrap();
    let handle = tokio::spawn(server.run());

    let mut bad = GhostwriterClient::new(format!("ws://{}", addr), Some("wrong".into())).unwrap();
    assert!(bad.connect().await.is_err());

    let mut good = GhostwriterClient::new(format!("ws://{}", addr), Some("pass".into())).unwrap();
    good.connect().await.unwrap();
    assert_eq!(
        good.status(),
        ghostwriter::network::client::ConnectionStatus::Connected
    );

    handle.abort();
    let _ = handle.await;
}
