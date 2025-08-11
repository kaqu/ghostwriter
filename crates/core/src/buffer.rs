use ropey::Rope;
use std::{io, path::Path};

/// Rope-based text buffer with invalid UTF-8 tracking.
pub struct RopeBuffer {
    rope: Rope,
    has_invalid: bool,
}

impl RopeBuffer {
    /// Open a file from disk into a `RopeBuffer`.
    pub fn open<P: AsRef<Path>>(path: P) -> io::Result<Self> {
        let bytes = std::fs::read(path)?;
        let (text, has_invalid) = match String::from_utf8(bytes) {
            Ok(s) => (s, false),
            Err(e) => {
                let bytes = e.into_bytes();
                (String::from_utf8_lossy(&bytes).into_owned(), true)
            }
        };
        Ok(Self {
            rope: Rope::from_str(&text),
            has_invalid,
        })
    }

    /// Returns true if the loaded file contained invalid UTF-8 bytes.
    pub fn has_invalid(&self) -> bool {
        self.has_invalid
    }

    /// Returns the entire text as a [`String`].
    pub fn text(&self) -> String {
        self.rope.to_string()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;
    use tempfile::NamedTempFile;

    #[test]
    fn open_valid_utf8() {
        let mut file = NamedTempFile::new().unwrap();
        writeln!(file, "hello").unwrap();
        let buf = RopeBuffer::open(file.path()).unwrap();
        assert_eq!(buf.text(), "hello\n");
        assert!(!buf.has_invalid());
    }

    #[test]
    fn open_invalid_utf8_sets_flag() {
        let mut file = NamedTempFile::new().unwrap();
        file.write_all(&[0x66, 0x6f, 0x80, 0x6f]).unwrap();
        let buf = RopeBuffer::open(file.path()).unwrap();
        assert!(buf.has_invalid());
        assert_eq!(buf.text(), "fo\u{FFFD}o");
    }
}
