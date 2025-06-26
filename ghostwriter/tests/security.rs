use ghostwriter::files::workspace::WorkspaceManager;
use ghostwriter::network::client::GhostwriterClient;
use ghostwriter::network::protocol::MessageKind;
use ghostwriter::network::server::GhostwriterServer;
use serial_test::serial;
use tokio::time::Duration;

#[tokio::test]
#[serial]
async fn test_path_traversal_prevention() {
    let dir = tempfile::tempdir().unwrap();
    let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
    let server = GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, None)
        .await
        .unwrap();
    let addr = server.local_addr().unwrap();
    let handle = tokio::spawn(server.run());

    let mut client = GhostwriterClient::new(format!("ws://{}", addr), None).unwrap();
    client.connect().await.unwrap();
    let resp = client
        .request(
            MessageKind::FileReadRequest {
                path: "../secret.txt".into(),
            },
            Duration::from_secs(1),
        )
        .await
        .unwrap();
    if let MessageKind::FileReadResponse {
        success, reason, ..
    } = resp.kind
    {
        assert!(!success);
        assert!(reason.is_some());
    } else {
        panic!("unexpected response");
    }

    handle.abort();
    let _ = handle.await;
}

#[tokio::test]
#[serial]
async fn test_authentication_attack_resistance() {
    let dir = tempfile::tempdir().unwrap();
    let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
    let server = GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, Some("pass".into()))
        .await
        .unwrap();
    let addr = server.local_addr().unwrap();
    let handle = tokio::spawn(server.run());

    let mut bad = GhostwriterClient::new(format!("ws://{}", addr), Some("wrong".into())).unwrap();
    assert!(bad.connect().await.is_err());

    handle.abort();
    let _ = handle.await;
}

#[test]
fn test_input_sanitization() {
    use ghostwriter::security::sanitize_path;
    use std::path::Path;
    assert!(sanitize_path(Path::new("../evil")).is_err());
    assert!(sanitize_path(Path::new("good.txt")).is_ok());
    assert!(sanitize_path(Path::new("bad\x00name")).is_err());
}

#[tokio::test]
#[serial]
async fn test_workspace_escape_attempts() {
    let dir = tempfile::tempdir().unwrap();
    let outside = tempfile::tempdir().unwrap();
    let link = dir.path().join("link");
    #[cfg(unix)]
    std::os::unix::fs::symlink(outside.path(), &link).unwrap();
    #[cfg(windows)]
    std::os::windows::fs::symlink_dir(outside.path(), &link).unwrap();
    let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
    let server = GhostwriterServer::bind("127.0.0.1:0".parse().unwrap(), ws, None)
        .await
        .unwrap();
    let addr = server.local_addr().unwrap();
    let handle = tokio::spawn(server.run());

    let mut client = GhostwriterClient::new(format!("ws://{}", addr), None).unwrap();
    client.connect().await.unwrap();
    let resp = client
        .request(
            MessageKind::DirListRequest {
                path: "link".into(),
            },
            Duration::from_secs(1),
        )
        .await
        .unwrap();
    if let MessageKind::DirListResponse { entries, reason } = resp.kind {
        assert!(entries.is_none());
        assert!(reason.is_some());
    } else {
        panic!("unexpected response");
    }

    handle.abort();
    let _ = handle.await;
}
