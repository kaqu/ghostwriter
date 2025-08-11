use rand::Rng;
use std::fs::{self, File, OpenOptions};
use std::io::{self, Write};
use std::path::Path;

/// Atomically write `bytes` to `path`.
///
/// Writes to a temporary file, syncs, renames over `path` and then fsyncs the
/// parent directory to ensure durability.
pub fn atomic_write(path: &Path, bytes: &[u8]) -> io::Result<()> {
    let dir = path
        .parent()
        .ok_or_else(|| io::Error::other("missing parent"))?;
    let mut tmp = dir.to_path_buf();
    let name = path
        .file_name()
        .ok_or_else(|| io::Error::other("missing file name"))?;
    let nonce: u64 = rand::thread_rng().r#gen();
    tmp.push(format!(".{}.gw.tmp.{}", name.to_string_lossy(), nonce));
    let mut f = OpenOptions::new().create_new(true).write(true).open(&tmp)?;
    f.write_all(bytes)?;
    f.sync_all()?;
    fs::rename(&tmp, path)?;
    let dirf = File::open(dir)?;
    dirf.sync_all()?;
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs;
    use std::io::Read;
    use tempfile::tempdir;

    #[test]
    fn atomic_write_replaces_file() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("file.txt");
        fs::write(&path, b"old").unwrap();
        atomic_write(&path, b"new").unwrap();
        let mut data = String::new();
        File::open(&path)
            .unwrap()
            .read_to_string(&mut data)
            .unwrap();
        assert_eq!(data, "new");
        // ensure no temp files remain
        let entries: Vec<_> = fs::read_dir(dir.path())
            .unwrap()
            .map(|e| e.unwrap().file_name())
            .collect();
        assert_eq!(entries.len(), 1);
    }

    #[test]
    fn atomic_write_missing_parent_errors() {
        let path = std::path::Path::new("");
        assert!(atomic_write(path, b"data").is_err());
    }

    #[test]
    fn atomic_write_missing_name_errors() {
        let path = std::path::Path::new("/");
        assert!(atomic_write(path, b"data").is_err());
    }
}
