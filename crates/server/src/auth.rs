use std::{fs, io, path::Path};

/// Load the Argon2 hash from `path`. Returns `Ok(None)` if the file does not
/// exist.
pub fn load_hash<P: AsRef<Path>>(path: P) -> io::Result<Option<String>> {
    match fs::read_to_string(path) {
        Ok(s) => Ok(Some(s.trim().to_string())),
        Err(e) if e.kind() == io::ErrorKind::NotFound => Ok(None),
        Err(e) => Err(e),
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use tempfile::NamedTempFile;

    #[test]
    fn loads_existing_hash() {
        let mut file = NamedTempFile::new().unwrap();
        std::io::Write::write_all(&mut file, b"hash").unwrap();
        let path = file.path();
        let loaded = load_hash(path).unwrap();
        assert_eq!(loaded, Some("hash".to_string()));
    }

    #[test]
    fn missing_file_returns_none() {
        let path = Path::new("nonexistent");
        let loaded = load_hash(path).unwrap();
        assert_eq!(loaded, None);
    }
}
