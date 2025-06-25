// FileManager provides file read/write utilities with support for large files.
#![allow(dead_code)]

use std::fs::File;
use std::io::{Read, Write};
use std::path::Path;

use memmap2::Mmap;

use crate::error::{GhostwriterError, Result};

/// Threshold in bytes for using memory mapping instead of reading into memory.
const MMAP_THRESHOLD: u64 = 100 * 1024 * 1024; // 100MB

/// Represents contents of a file.
#[derive(Debug)]
pub enum FileContents {
    /// File data loaded into memory.
    InMemory(Vec<u8>),
    /// Memory-mapped file data.
    Mapped(Mmap),
}

/// File manager utility functions.
#[derive(Debug, Default)]
pub struct FileManager;

/// Iterator over file data in fixed-size chunks.
pub struct ChunkReader {
    file: File,
    buf_size: usize,
}

impl Iterator for ChunkReader {
    type Item = std::io::Result<Vec<u8>>;

    fn next(&mut self) -> Option<Self::Item> {
        let mut buf = vec![0u8; self.buf_size];
        match self.file.read(&mut buf) {
            Ok(0) => None,
            Ok(n) => {
                buf.truncate(n);
                Some(Ok(buf))
            }
            Err(e) => Some(Err(e)),
        }
    }
}

impl FileManager {
    /// Read file contents, using memory mapping for large files.
    pub fn read(path: &Path) -> Result<FileContents> {
        let file = File::open(path)?;
        let metadata = file.metadata()?;
        if metadata.len() > MMAP_THRESHOLD {
            // Safety: file is not mutated while mapping is active.
            let mmap = unsafe { Mmap::map(&file)? };
            Ok(FileContents::Mapped(mmap))
        } else {
            let mut buf = Vec::with_capacity(metadata.len() as usize);
            let mut file = file;
            file.read_to_end(&mut buf)?;
            Ok(FileContents::InMemory(buf))
        }
    }

    /// Write data to a file atomically using a temporary file and rename.
    pub fn atomic_write(path: &Path, data: &[u8]) -> Result<()> {
        let dir = path
            .parent()
            .ok_or_else(|| GhostwriterError::InvalidArgument("invalid path".into()))?;
        let mut tmp = tempfile::NamedTempFile::new_in(dir)?;
        tmp.write_all(data)?;
        tmp.flush()?;
        tmp.persist(path)
            .map_err(|e| GhostwriterError::Io(e.error))?;
        Ok(())
    }

    /// Read a file incrementally, returning an iterator over chunks.
    pub fn chunk_reader(path: &Path, chunk_size: usize) -> Result<ChunkReader> {
        let file = File::open(path)?;
        Ok(ChunkReader {
            file,
            buf_size: chunk_size,
        })
    }

    /// Determine if a slice of bytes likely represents binary data.
    pub fn is_binary(data: &[u8]) -> bool {
        data.contains(&0)
    }

    /// Convert bytes to UTF-8 string or hex representation on invalid UTF-8.
    pub fn to_utf8_or_hex(data: &[u8]) -> String {
        match std::str::from_utf8(data) {
            Ok(s) => s.to_string(),
            Err(_) => data
                .iter()
                .map(|b| format!("{:02X} ", b))
                .collect::<String>()
                .trim_end()
                .to_string(),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;
    use tempfile::tempdir;

    #[test]
    fn test_atomic_file_write() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("file.txt");
        FileManager::atomic_write(&path, b"first").unwrap();
        let metadata_before = std::fs::metadata(&path).unwrap();
        FileManager::atomic_write(&path, b"second").unwrap();
        let contents = std::fs::read(&path).unwrap();
        assert_eq!(contents, b"second");
        let metadata_after = std::fs::metadata(&path).unwrap();
        assert!(metadata_after.modified().unwrap() >= metadata_before.modified().unwrap());
    }

    #[test]
    fn test_memory_mapped_large_file() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("large.bin");
        // create file slightly larger than threshold
        let mut f = File::create(&path).unwrap();
        f.write_all(&vec![0u8; (MMAP_THRESHOLD + 1) as usize])
            .unwrap();
        drop(f);
        let contents = FileManager::read(&path).unwrap();
        match contents {
            FileContents::Mapped(_) => {}
            _ => panic!("expected memory mapped"),
        }
    }

    #[test]
    fn test_binary_file_detection() {
        assert!(FileManager::is_binary(b"abc\0def"));
        assert!(!FileManager::is_binary(b"plain text"));
    }

    #[test]
    fn test_utf8_hex_fallback() {
        let data = b"hello\xFFworld";
        let s = FileManager::to_utf8_or_hex(data);
        assert!(s.contains("FF"));
    }

    #[test]
    fn test_chunk_reader() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("chunk.txt");
        let data: Vec<u8> = (0..10u8).cycle().take(3000).collect();
        std::fs::write(&path, &data).unwrap();
        let mut reader = FileManager::chunk_reader(&path, 512).unwrap();
        let mut out = Vec::new();
        while let Some(chunk) = reader.next() {
            let chunk = chunk.unwrap();
            out.extend_from_slice(&chunk);
        }
        assert_eq!(out, data);
    }

    #[test]
    fn test_file_system_error_handling() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("nope").join("file.txt");
        let res = FileManager::atomic_write(&path, b"data");
        assert!(res.is_err());
    }
}
