// FileWatcher monitors file system changes for a specific file.
#![allow(dead_code)]

use std::path::{Path, PathBuf};
use std::sync::mpsc::{Receiver, channel};
use std::time::SystemTime;

use notify::{Event, EventKind, RecursiveMode, Watcher, recommended_watcher};

use crate::error::{GhostwriterError, Result};

/// Represents an external modification to a file.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct ExternalChange {
    pub path: PathBuf,
    pub old_modified: SystemTime,
    pub new_modified: SystemTime,
}

/// Possible strategies when a conflict is detected.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ConflictResolution {
    /// Keep the local version of the file.
    KeepLocal,
    /// Overwrite with the external version of the file.
    AcceptExternal,
    /// Simple manual merge strategy.
    ManualMerge,
}

/// Watches a single file for external modifications.
pub struct FileWatcher {
    _watcher: notify::RecommendedWatcher,
    rx: Receiver<notify::Result<Event>>,
    path: PathBuf,
    last_modified: SystemTime,
}

impl FileWatcher {
    /// Create a new file watcher for the given path.
    pub fn new(path: &Path) -> Result<Self> {
        let (tx, rx) = channel();
        let mut watcher = recommended_watcher(tx)?;
        watcher.watch(path, RecursiveMode::NonRecursive)?;
        let metadata = std::fs::metadata(path)?;
        let last_modified = metadata.modified()?;
        Ok(Self {
            _watcher: watcher,
            rx,
            path: path.to_path_buf(),
            last_modified,
        })
    }

    /// Check for external updates. Returns `Some(change)` if detected.
    pub fn check(&mut self) -> Result<Option<ExternalChange>> {
        while let Ok(res) = self.rx.try_recv() {
            let event = res.map_err(GhostwriterError::from)?;
            if matches!(event.kind, EventKind::Modify(_))
                || matches!(event.kind, EventKind::Create(_))
            {
                let metadata = std::fs::metadata(&self.path)?;
                let modified = metadata.modified()?;
                if modified > self.last_modified {
                    let change = ExternalChange {
                        path: self.path.clone(),
                        old_modified: self.last_modified,
                        new_modified: modified,
                    };
                    self.last_modified = modified;
                    return Ok(Some(change));
                }
            }
        }
        Ok(None)
    }
}

/// Resolve a conflict between local and external changes.
pub fn resolve_conflict(choice: ConflictResolution, local: &[u8], external: &[u8]) -> Vec<u8> {
    match choice {
        ConflictResolution::KeepLocal => local.to_vec(),
        ConflictResolution::AcceptExternal => external.to_vec(),
        ConflictResolution::ManualMerge => {
            let mut result = String::new();
            result.push_str(std::str::from_utf8(local).unwrap_or_default());
            for line in std::str::from_utf8(external).unwrap_or_default().lines() {
                if !result.contains(line) {
                    result.push_str(line);
                    result.push('\n');
                }
            }
            result.into_bytes()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs::{self, File};
    use std::io::Write;
    use std::thread::sleep;
    use std::time::Duration;
    use tempfile::tempdir;

    #[test]
    fn test_external_change_detection() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("watch.txt");
        fs::write(&path, b"a").unwrap();
        let mut watcher = FileWatcher::new(&path).unwrap();
        sleep(Duration::from_millis(200));
        fs::write(&path, b"b").unwrap();
        let mut detected = None;
        for _ in 0..10 {
            if let Some(change) = watcher.check().unwrap() {
                detected = Some(change);
                break;
            }
            sleep(Duration::from_millis(100));
        }
        assert!(detected.is_some(), "no change detected");
    }

    #[test]
    fn test_conflict_resolution_options() {
        let local = b"local\n";
        let external = b"external\n";
        assert_eq!(
            resolve_conflict(ConflictResolution::KeepLocal, local, external),
            local
        );
        assert_eq!(
            resolve_conflict(ConflictResolution::AcceptExternal, local, external),
            external
        );
        let merged = resolve_conflict(ConflictResolution::ManualMerge, local, external);
        let merged_str = String::from_utf8(merged).unwrap();
        assert!(merged_str.contains("local"));
        assert!(merged_str.contains("external"));
    }

    #[test]
    fn test_modification_time_comparison() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("file.txt");
        let mut f = File::create(&path).unwrap();
        writeln!(f, "one").unwrap();
        f.flush().unwrap();
        let mut watcher = FileWatcher::new(&path).unwrap();
        let old = watcher.last_modified;
        sleep(Duration::from_millis(200));
        fs::write(&path, b"two").unwrap();
        let mut change = None;
        for _ in 0..10 {
            if let Some(ch) = watcher.check().unwrap() {
                change = Some(ch);
                break;
            }
            sleep(Duration::from_millis(100));
        }
        let ch = change.expect("no change detected");
        assert!(ch.new_modified > old);
    }
}
