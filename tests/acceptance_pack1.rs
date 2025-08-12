use ghostwriter_core::{EditOp, EditRecord, RopeBuffer, UndoStack, Wal};
use tempfile::tempdir;

#[test]
fn edit_undo_save_roundtrip() {
    let dir = tempdir().unwrap();
    let path = dir.path().join("doc.txt");
    std::fs::write(&path, b"hello").unwrap();
    let mut buf = RopeBuffer::open(&path).unwrap();
    let mut undo = UndoStack::new();

    undo.insert(&mut buf, 5, " world");
    assert_eq!(buf.text(), "hello world");
    buf.save_to(&path).unwrap();
    let content = std::fs::read_to_string(&path).unwrap();
    assert_eq!(content, "hello world");

    assert!(undo.undo(&mut buf));
    assert_eq!(buf.text(), "hello");
    buf.save_to(&path).unwrap();
    let content = std::fs::read_to_string(&path).unwrap();
    assert_eq!(content, "hello");
}

#[test]
fn wal_crash_replay_recovers() {
    let dir = tempdir().unwrap();
    let path = dir.path().join("file.txt");
    std::fs::write(&path, b"hello").unwrap();
    let wal_path = dir.path().join("file.wal");
    let mut wal = Wal::new(&wal_path).unwrap();
    let mut buf = RopeBuffer::open(&path).unwrap();

    wal.append(&EditRecord {
        doc_v: 1,
        op: EditOp::Insert {
            idx: 5,
            bytes: b" world".to_vec(),
        },
    })
    .unwrap();
    buf.insert(5, " world");

    wal.append(&EditRecord {
        doc_v: 2,
        op: EditOp::Delete { range: 0..1 },
    })
    .unwrap();
    buf.delete(0..1);

    assert_eq!(buf.text(), "ello world");

    drop(buf);
    drop(wal);

    let mut buf2 = RopeBuffer::open(&path).unwrap();
    assert_eq!(buf2.text(), "hello");
    let records = Wal::replay(&wal_path).unwrap();
    for rec in records {
        match rec.op {
            EditOp::Insert { idx, bytes } => {
                let text = std::str::from_utf8(&bytes).unwrap();
                buf2.insert(idx as usize, text);
            }
            EditOp::Delete { range } => {
                buf2.delete(range.start as usize..range.end as usize);
            }
        }
    }
    assert_eq!(buf2.text(), "ello world");
}
