use ghostwriter::files::file_manager::{FileContents, FileManager};
use ghostwriter::network::protocol::MessageKind;
use serial_test::serial;
use std::fs::File;
use std::time::{Duration, Instant};
use tempfile::tempdir;
mod util;

const ONE_GB: u64 = 1_024 * 1_024 * 1_024;

#[tokio::test]
#[serial]
async fn test_remote_search_functionality() {
    let dir = tempdir().unwrap();
    let file_path = dir.path().join("file.txt");
    std::fs::write(&file_path, b"hello world").unwrap();

    let (handle, mut client, _addr) = util::start_server(dir.path(), None).await;
    client.connect().await.unwrap();
    let resp = client
        .request(
            MessageKind::SearchRequest {
                pattern: "hello".into(),
                regex: false,
                case_sensitive: true,
            },
            Duration::from_secs(1),
        )
        .await
        .unwrap();
    handle.abort();
    let _ = handle.await;
    if let MessageKind::SearchResponse { matches, .. } = resp.kind {
        assert!(matches.unwrap().len() > 0);
    } else {
        panic!("unexpected response");
    }
}

#[test]
fn test_large_file_handling_1gb() {
    let dir = tempdir().unwrap();
    let path = dir.path().join("large.bin");
    let file = File::create(&path).unwrap();
    file.set_len(ONE_GB).unwrap();
    let contents = FileManager::read(&path).unwrap();
    match contents {
        FileContents::Mapped(m) => assert_eq!(m.len() as u64, ONE_GB),
        _ => panic!("expected memory mapped file"),
    }
}

#[test]
fn test_file_operation_speed() {
    let dir = tempdir().unwrap();
    let path = dir.path().join("speed.txt");
    let data = vec![b'x'; 1024 * 100];
    let start = Instant::now();
    FileManager::atomic_write(&path, &data).unwrap();
    let _ = FileManager::read(&path).unwrap();
    assert!(start.elapsed() < Duration::from_millis(100));
}
