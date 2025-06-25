// FileLock provides exclusive file locking with automatic cleanup.
#![allow(dead_code)]

use std::fs::{File, OpenOptions};
use std::path::Path;
use std::thread::sleep;
use std::time::{Duration, Instant};

use crate::error::{GhostwriterError, Result};
use fs4::fs_std::FileExt;

#[derive(Debug)]
pub struct FileLock {
    file: File,
    process_file: Option<File>,
    readonly: bool,
}

impl FileLock {
    /// Acquire an exclusive lock on a file with a timeout.
    /// Falls back to read-only mode if the lock cannot be obtained.
    pub fn acquire(path: &Path, timeout: Duration) -> Result<Self> {
        let process_lock_path = std::env::temp_dir().join("ghostwriter.lock");
        let process_file = OpenOptions::new()
            .read(true)
            .write(true)
            .create(true)
            .truncate(false)
            .open(&process_lock_path)?;

        if !process_file.try_lock_exclusive()? {
            return Err(GhostwriterError::InvalidArgument(
                "only one lock allowed per process".into(),
            ));
        }

        let file_res = OpenOptions::new()
            .read(true)
            .write(true)
            .create(true)
            .truncate(false)
            .open(path);
        let file = match file_res {
            Ok(f) => f,
            Err(e) => {
                let _ = fs4::fs_std::FileExt::unlock(&process_file);
                return Err(e.into());
            }
        };

        let start = Instant::now();
        loop {
            match file.try_lock_exclusive() {
                Ok(true) => {
                    return Ok(Self {
                        file,
                        process_file: Some(process_file),
                        readonly: false,
                    });
                }
                Ok(false) => {
                    if start.elapsed() >= timeout {
                        drop(file);
                        let file = OpenOptions::new().read(true).open(path)?;
                        let _ = fs4::fs_std::FileExt::unlock(&process_file);
                        return Ok(Self {
                            file,
                            process_file: None,
                            readonly: true,
                        });
                    }
                    sleep(Duration::from_millis(50));
                }
                Err(e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                    if start.elapsed() >= timeout {
                        drop(file);
                        let file = OpenOptions::new().read(true).open(path)?;
                        let _ = fs4::fs_std::FileExt::unlock(&process_file);
                        return Ok(Self {
                            file,
                            process_file: None,
                            readonly: true,
                        });
                    }
                    sleep(Duration::from_millis(50));
                }
                Err(e) => {
                    let _ = fs4::fs_std::FileExt::unlock(&process_file);
                    return Err(e.into());
                }
            }
        }
    }

    /// Whether this lock is read-only.
    pub fn readonly(&self) -> bool {
        self.readonly
    }
}

impl Drop for FileLock {
    fn drop(&mut self) {
        let _ = fs4::fs_std::FileExt::unlock(&self.file);
        if let Some(pf) = &self.process_file {
            let _ = fs4::fs_std::FileExt::unlock(pf);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use serial_test::serial;
    use tempfile::tempdir;

    #[test]
    #[serial]
    fn test_exclusive_file_lock() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("test.txt");
        std::fs::write(&path, b"data").unwrap();
        let _lock = FileLock::acquire(&path, Duration::from_millis(100)).unwrap();
        let file = OpenOptions::new()
            .read(true)
            .write(true)
            .open(&path)
            .unwrap();
        assert!(!file.try_lock_exclusive().unwrap());
    }

    #[test]
    #[serial]
    fn test_automatic_lock_release() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("test.txt");
        std::fs::write(&path, b"data").unwrap();
        {
            let _lock = FileLock::acquire(&path, Duration::from_millis(100)).unwrap();
        }
        let file = OpenOptions::new()
            .read(true)
            .write(true)
            .open(&path)
            .unwrap();
        assert!(file.try_lock_exclusive().unwrap());
    }

    #[test]
    #[serial]
    fn test_lock_timeout() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("test.txt");
        std::fs::write(&path, b"data").unwrap();
        let file = OpenOptions::new()
            .read(true)
            .write(true)
            .open(&path)
            .unwrap();
        file.lock_exclusive().unwrap();
        let start = Instant::now();
        let lock = FileLock::acquire(&path, Duration::from_millis(200)).unwrap();
        assert!(start.elapsed() >= Duration::from_millis(200));
        assert!(lock.readonly());
        fs4::fs_std::FileExt::unlock(&file).unwrap();
    }

    #[test]
    #[serial]
    fn test_readonly_fallback() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("test.txt");
        std::fs::write(&path, b"data").unwrap();
        let file = OpenOptions::new()
            .read(true)
            .write(true)
            .open(&path)
            .unwrap();
        file.lock_exclusive().unwrap();
        let lock = FileLock::acquire(&path, Duration::from_millis(100)).unwrap();
        assert!(lock.readonly());
        fs4::fs_std::FileExt::unlock(&file).unwrap();
    }
}
