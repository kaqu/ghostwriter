#[cfg(target_os = "linux")]
#[test]
fn test_linux_file_locking() {
    use fs4::fs_std::FileExt;
    use ghostwriter::files::file_lock::FileLock;
    use std::fs::OpenOptions;
    use std::time::Duration;
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("lock.txt");
    std::fs::write(&path, b"data").unwrap();
    let _lock = FileLock::acquire(&path, Duration::from_millis(100)).unwrap();
    let file = OpenOptions::new()
        .read(true)
        .write(true)
        .open(&path)
        .unwrap();
    assert!(!file.try_lock_exclusive().unwrap());
}

mod util;

#[cfg(target_os = "macos")]
#[test]
fn test_macos_file_locking() {
    use fs4::fs_std::FileExt;
    use ghostwriter::files::file_lock::FileLock;
    use std::fs::OpenOptions;
    use std::time::Duration;
    let dir = tempfile::tempdir().unwrap();
    let path = dir.path().join("lock.txt");
    std::fs::write(&path, b"data").unwrap();
    let _lock = FileLock::acquire(&path, Duration::from_millis(100)).unwrap();
    let file = OpenOptions::new()
        .read(true)
        .write(true)
        .open(&path)
        .unwrap();
    assert!(!file.try_lock_exclusive().unwrap());
}

#[tokio::test]
async fn test_cross_platform_websockets() {
    use ghostwriter::network::client::ConnectionStatus;
    use ghostwriter::network::protocol::MessageKind;
    use std::time::Duration;
    use tempfile::tempdir;
    use tokio::time::timeout;

    let dir = tempdir().unwrap();
    let (handle, mut client, _addr) = util::start_server(dir.path(), None).await;
    timeout(Duration::from_secs(1), client.connect())
        .await
        .unwrap()
        .unwrap();
    assert_eq!(client.status(), ConnectionStatus::Connected);

    let resp = client
        .request(
            MessageKind::DirListRequest { path: ".".into() },
            Duration::from_secs(1),
        )
        .await
        .unwrap();
    assert!(matches!(resp.kind, MessageKind::DirListResponse { .. }));
    handle.abort();
    let _ = handle.await;
}
