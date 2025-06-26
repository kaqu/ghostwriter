use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};
use ghostwriter::editor::{cursor::Cursor, key_handler::KeyHandler, rope::Rope};
use ghostwriter::network::{internal::InternalServer, protocol::MessageKind};
use serial_test::serial;
use std::time::{Duration, Instant};
use sysinfo::{Pid, ProcessRefreshKind, ProcessesToUpdate, System};
use tempfile::tempdir;
mod util;

fn key(ch: char) -> KeyEvent {
    KeyEvent::new(KeyCode::Char(ch), KeyModifiers::empty())
}

#[tokio::test]
#[serial]
async fn test_startup_performance_targets() {
    let dir = tempdir().unwrap();
    let start = Instant::now();
    let (server, mut client) = InternalServer::start(dir.path().to_path_buf(), None)
        .await
        .unwrap();
    client.connect().await.unwrap();
    let elapsed = start.elapsed();
    assert!(
        elapsed < Duration::from_millis(50),
        "startup took {:?}",
        elapsed
    );
    drop(client);
    drop(server);
}

#[test]
fn test_edit_operation_latency() {
    let mut rope = Rope::new();
    let mut cursor = Cursor::new();
    let mut handler = KeyHandler::new();
    let mut sel = None;
    let start = Instant::now();
    for _ in 0..100 {
        handler.handle(key('a'), &mut rope, &mut cursor, &mut sel);
    }
    let elapsed = start.elapsed();
    assert!(
        elapsed < Duration::from_millis(10),
        "edit latency {:?}",
        elapsed
    );
}

#[tokio::test]
#[serial]
async fn test_memory_usage_limits() {
    let pid = Pid::from_u32(std::process::id());
    let mut sys = System::new();
    sys.refresh_processes_specifics(
        ProcessesToUpdate::Some(&[pid]),
        false,
        ProcessRefreshKind::everything(),
    );
    let before = sys.process(pid).unwrap().memory();
    let chunk = "a".repeat(1024 * 1024); // 1MB
    let mut rope = Rope::new();
    for _ in 0..50 {
        rope.append(&chunk);
    }
    sys.refresh_processes_specifics(
        ProcessesToUpdate::Some(&[pid]),
        false,
        ProcessRefreshKind::everything(),
    );
    let after = sys.process(pid).unwrap().memory();
    assert!(
        after - before < 100 * 1024 * 1024,
        "memory usage {} bytes",
        after - before
    );
    drop(rope);
}

#[tokio::test]
#[serial]
async fn test_network_operation_performance() {
    let dir = tempdir().unwrap();
    let (handle, mut client, _addr) = util::start_server(dir.path(), None).await;
    client.connect().await.unwrap();
    let start = Instant::now();
    client
        .request(MessageKind::Ping, Duration::from_secs(1))
        .await
        .unwrap();
    let elapsed = start.elapsed();
    assert!(
        elapsed < Duration::from_millis(100),
        "network latency {:?}",
        elapsed
    );
    handle.abort();
    let _ = handle.await;
}
