use crc32fast::Hasher;
use std::fs::{File, OpenOptions};
use std::io::{self, Read, Seek, SeekFrom, Write};
use std::ops::Range;
use std::path::{Path, PathBuf};

const MAGIC: &[u8; 4] = b"GWAL";
const VERSION: u8 = 1;
const TYPE_INSERT: u8 = 1;
const TYPE_DELETE: u8 = 2;

/// Edit operation for WAL records.
pub enum EditOp {
    Insert { idx: u64, bytes: Vec<u8> },
    Delete { range: Range<u64> },
}

/// WAL edit record with document version.
pub struct EditRecord {
    pub doc_v: u64,
    pub op: EditOp,
}

/// Write-ahead log.
pub struct Wal {
    path: PathBuf,
    file: File,
    doc_v: u64,
}

impl Wal {
    /// Open or create WAL at `path` and determine current document version.
    pub fn new<P: AsRef<Path>>(path: P) -> io::Result<Self> {
        let path_buf = path.as_ref().to_path_buf();
        let file = OpenOptions::new()
            .create(true)
            .append(true)
            .read(true)
            .open(&path_buf)?;
        let mut wal = Self {
            path: path_buf,
            file,
            doc_v: 0,
        };
        // Determine last doc version from existing records
        if let Ok(records) = Self::replay(&wal.path) {
            if let Some(last) = records.last() {
                wal.doc_v = last.doc_v;
            }
        }
        Ok(wal)
    }

    /// Append a record to the WAL.
    pub fn append(&mut self, record: &EditRecord) -> io::Result<()> {
        let mut payload = Vec::new();
        let record_type = match &record.op {
            EditOp::Insert { idx, bytes } => {
                payload.extend_from_slice(&idx.to_be_bytes());
                payload.extend_from_slice(bytes);
                TYPE_INSERT
            }
            EditOp::Delete { range } => {
                payload.extend_from_slice(&range.start.to_be_bytes());
                payload.extend_from_slice(&range.end.to_be_bytes());
                TYPE_DELETE
            }
        };

        let mut type_section = Vec::new();
        type_section.push(record_type);
        type_section.extend_from_slice(&(payload.len() as u32).to_be_bytes());
        type_section.extend_from_slice(&payload);

        let mut hasher = Hasher::new();
        hasher.update(&type_section);
        let crc = hasher.finalize();

        let mut record_bytes = Vec::new();
        record_bytes.extend_from_slice(MAGIC);
        record_bytes.push(VERSION);
        record_bytes.extend_from_slice(&record.doc_v.to_be_bytes());
        record_bytes.extend_from_slice(&type_section);
        record_bytes.extend_from_slice(&crc.to_be_bytes());

        self.file.write_all(&record_bytes)?;
        self.file.sync_all()?;
        self.doc_v = record.doc_v;
        Ok(())
    }

    /// Replay WAL at `path` into a list of records.
    pub fn replay<P: AsRef<Path>>(path: P) -> io::Result<Vec<EditRecord>> {
        let mut f = match File::open(path) {
            Ok(file) => file,
            Err(e) if e.kind() == io::ErrorKind::NotFound => return Ok(Vec::new()),
            Err(e) => return Err(e),
        };
        let mut records = Vec::new();
        loop {
            let mut header = [0u8; 13];
            if f.read_exact(&mut header).is_err() {
                break;
            }
            if &header[0..4] != MAGIC || header[4] != VERSION {
                break;
            }
            let doc_v = u64::from_be_bytes(header[5..13].try_into().unwrap());

            let mut type_buf = [0u8; 5];
            if f.read_exact(&mut type_buf).is_err() {
                break;
            }
            let typ = type_buf[0];
            let len = u32::from_be_bytes(type_buf[1..5].try_into().unwrap()) as usize;
            let mut payload = vec![0u8; len];
            if f.read_exact(&mut payload).is_err() {
                break;
            }
            let mut crc_buf = [0u8; 4];
            if f.read_exact(&mut crc_buf).is_err() {
                break;
            }
            let expected_crc = u32::from_be_bytes(crc_buf);
            let mut hasher = Hasher::new();
            hasher.update(&type_buf);
            hasher.update(&payload);
            let actual_crc = hasher.finalize();
            if expected_crc != actual_crc {
                continue; // discard corrupt record
            }

            let op = match typ {
                TYPE_INSERT => {
                    if payload.len() < 8 {
                        continue;
                    }
                    let idx = u64::from_be_bytes(payload[0..8].try_into().unwrap());
                    let bytes = payload[8..].to_vec();
                    EditOp::Insert { idx, bytes }
                }
                TYPE_DELETE => {
                    if payload.len() != 16 {
                        continue;
                    }
                    let start = u64::from_be_bytes(payload[0..8].try_into().unwrap());
                    let end = u64::from_be_bytes(payload[8..16].try_into().unwrap());
                    EditOp::Delete { range: start..end }
                }
                _ => continue,
            };
            records.push(EditRecord { doc_v, op });
        }
        Ok(records)
    }

    /// Compact the WAL file if it exceeds `threshold` bytes.
    pub fn compact_if_needed(&mut self, threshold: u64) -> io::Result<()> {
        let size = self.file.metadata()?.len();
        if size >= threshold {
            self.file.set_len(0)?;
            self.file.seek(SeekFrom::Start(0))?;
            self.file.sync_all()?;
            self.doc_v = 0;
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use tempfile::tempdir;

    #[test]
    fn append_and_replay_roundtrip() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("test.wal");
        let mut wal = Wal::new(&path).unwrap();
        let rec1 = EditRecord {
            doc_v: 1,
            op: EditOp::Insert {
                idx: 0,
                bytes: b"hello".to_vec(),
            },
        };
        wal.append(&rec1).unwrap();
        let rec2 = EditRecord {
            doc_v: 2,
            op: EditOp::Delete { range: 1..3 },
        };
        wal.append(&rec2).unwrap();
        let replayed = Wal::replay(&path).unwrap();
        assert_eq!(replayed.len(), 2);
        match &replayed[0].op {
            EditOp::Insert { idx, bytes } => {
                assert_eq!(*idx, 0);
                assert_eq!(bytes, b"hello");
            }
            _ => panic!("expected insert"),
        }
        match &replayed[1].op {
            EditOp::Delete { range } => {
                assert_eq!(range.clone(), 1..3);
            }
            _ => panic!("expected delete"),
        }
    }

    #[test]
    fn crc_corruption_detected() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("corrupt.wal");
        let mut wal = Wal::new(&path).unwrap();
        let rec = EditRecord {
            doc_v: 1,
            op: EditOp::Insert {
                idx: 0,
                bytes: b"hi".to_vec(),
            },
        };
        wal.append(&rec).unwrap();
        let mut data = fs::read(&path).unwrap();
        let last = data.len() - 1;
        data[last] ^= 0xFF; // flip last byte to corrupt CRC
        fs::write(&path, data).unwrap();
        let replayed = Wal::replay(&path).unwrap();
        assert!(replayed.is_empty());
    }

    #[test]
    fn compact_if_needed_truncates() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("compact.wal");
        let mut wal = Wal::new(&path).unwrap();
        for i in 0..5 {
            let rec = EditRecord {
                doc_v: i + 1,
                op: EditOp::Insert {
                    idx: 0,
                    bytes: b"data".to_vec(),
                },
            };
            wal.append(&rec).unwrap();
        }
        wal.compact_if_needed(100).unwrap();
        let size = fs::metadata(&path).unwrap().len();
        assert!(size < 100);
    }
}
