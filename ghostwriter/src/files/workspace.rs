// WorkspaceManager enforces sandboxed directory operations
#![allow(dead_code)]

use std::fs::{self, OpenOptions};
use std::path::{Path, PathBuf};

use crate::error::{GhostwriterError, Result};

/// File or directory entry information
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct DirEntryInfo {
    pub name: String,
    pub is_dir: bool,
    pub size: u64,
}

/// Manages workspace directory and sandboxing
#[derive(Debug, Clone)]
pub struct WorkspaceManager {
    root: PathBuf,
}

impl WorkspaceManager {
    /// Create a new workspace manager with the given root directory
    pub fn new(root: PathBuf) -> Result<Self> {
        let canonical = root.canonicalize()?;
        if !canonical.is_dir() {
            return Err(GhostwriterError::InvalidArgument(
                "workspace root must be a directory".into(),
            ));
        }
        Ok(Self { root: canonical })
    }

    /// Return the workspace root directory path.
    pub fn root(&self) -> &Path {
        &self.root
    }

    /// Resolve an existing path within the workspace.
    fn resolve_existing(&self, path: &Path) -> Result<PathBuf> {
        let joined = if path.is_absolute() {
            PathBuf::from(path)
        } else {
            self.root.join(path)
        };
        let canonical = joined.canonicalize()?;
        if !canonical.starts_with(&self.root) {
            return Err(GhostwriterError::InvalidArgument(
                "path outside workspace".into(),
            ));
        }
        Ok(canonical)
    }

    /// Resolve a new path for creation within the workspace.
    fn resolve_new(&self, path: &Path) -> Result<PathBuf> {
        let joined = if path.is_absolute() {
            PathBuf::from(path)
        } else {
            self.root.join(path)
        };
        let parent = joined
            .parent()
            .ok_or_else(|| GhostwriterError::InvalidArgument("invalid path".into()))?;
        let canonical_parent = parent.canonicalize()?;
        if !canonical_parent.starts_with(&self.root) {
            return Err(GhostwriterError::InvalidArgument(
                "path outside workspace".into(),
            ));
        }
        Ok(canonical_parent.join(joined.file_name().unwrap()))
    }

    /// List directory contents with basic metadata
    pub fn list_dir(&self, path: &Path) -> Result<Vec<DirEntryInfo>> {
        let dir = self.resolve_existing(path)?;
        let mut entries = Vec::new();
        for entry in fs::read_dir(&dir)? {
            let entry = entry?;
            let meta = entry.metadata()?;
            entries.push(DirEntryInfo {
                name: entry.file_name().to_string_lossy().into(),
                is_dir: meta.is_dir(),
                size: meta.len(),
            });
        }
        Ok(entries)
    }

    /// Create a new empty file
    pub fn create_file(&self, path: &Path) -> Result<()> {
        let file = self.resolve_new(path)?;
        if let Some(parent) = file.parent() {
            fs::create_dir_all(parent)?;
        }
        OpenOptions::new()
            .write(true)
            .create_new(true)
            .open(&file)?;
        Ok(())
    }

    /// Create a new directory
    pub fn create_dir(&self, path: &Path) -> Result<()> {
        let dir = self.resolve_new(path)?;
        fs::create_dir_all(&dir)?;
        Ok(())
    }

    /// Delete a file or directory
    pub fn delete(&self, path: &Path) -> Result<()> {
        let target = self.resolve_existing(path)?;
        let meta = fs::metadata(&target)?;
        if meta.is_dir() {
            fs::remove_dir_all(&target)?;
        } else {
            fs::remove_file(&target)?;
        }
        Ok(())
    }

    /// Rename a file or directory within the workspace
    pub fn rename(&self, from: &Path, to: &Path) -> Result<()> {
        let src = self.resolve_existing(from)?;
        let dst = self.resolve_new(to)?;
        fs::rename(&src, &dst)?;
        Ok(())
    }

    /// Search for a pattern across all files in the workspace. Results are
    /// limited by `limit` to avoid excessive memory usage.
    pub fn search(
        &self,
        pattern: &str,
        regex: bool,
        case_sensitive: bool,
        limit: usize,
    ) -> Result<Vec<crate::files::search::SearchResult>> {
        crate::files::search::search_dir(self.root(), pattern, regex, case_sensitive, limit)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::tempdir;

    #[test]
    fn test_path_canonicalization() {
        let dir = tempdir().unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let res = ws.resolve_new(Path::new("../outside"));
        assert!(res.is_err(), "traversal should fail");
    }

    #[test]
    fn test_workspace_boundary_enforcement() {
        let dir = tempdir().unwrap();
        let outside = tempdir().unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let symlink = dir.path().join("link");
        #[cfg(unix)]
        std::os::unix::fs::symlink(outside.path(), &symlink).unwrap();
        #[cfg(windows)]
        std::os::windows::fs::symlink_dir(outside.path(), &symlink).unwrap();
        let res = ws.list_dir(Path::new("link"));
        assert!(res.is_err(), "symlink outside workspace should fail");
    }

    #[test]
    fn test_directory_operations() {
        let dir = tempdir().unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        ws.create_dir(Path::new("subdir")).unwrap();
        ws.create_file(Path::new("subdir/file.txt")).unwrap();
        let entries = ws.list_dir(Path::new("subdir")).unwrap();
        assert_eq!(entries.len(), 1);
        ws.rename(
            Path::new("subdir/file.txt"),
            Path::new("subdir/renamed.txt"),
        )
        .unwrap();
        ws.delete(Path::new("subdir/renamed.txt")).unwrap();
        ws.delete(Path::new("subdir")).unwrap();
        assert!(ws.list_dir(Path::new("subdir")).is_err());
    }

    #[test]
    fn test_permission_checking() {
        let ws = WorkspaceManager::new(PathBuf::from("/sys")).unwrap();
        let res = ws.create_file(Path::new("denied.txt"));
        assert!(res.is_err());
    }
}
