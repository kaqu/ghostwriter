// Hex view utilities for binary file display
#![allow(dead_code)]

use crate::files::file_manager::FileContents;

/// Viewer for binary files using hex dump format.
#[derive(Debug)]
pub struct HexView {
    contents: FileContents,
    offset: usize,
    bytes_per_line: usize,
}

impl HexView {
    /// Create a new hex view from file contents.
    pub fn new(contents: FileContents) -> Self {
        Self {
            contents,
            offset: 0,
            bytes_per_line: 16,
        }
    }

    fn data(&self) -> &[u8] {
        match &self.contents {
            FileContents::InMemory(v) => v,
            FileContents::Mapped(m) => &m[..],
        }
    }

    /// Total size of the data in bytes.
    pub fn len(&self) -> usize {
        self.data().len()
    }

    /// Current cursor offset in bytes.
    pub fn offset(&self) -> usize {
        self.offset
    }

    /// Produce the formatted hex dump line at the given line index.
    pub fn line(&self, line: usize) -> Option<String> {
        let start = line * self.bytes_per_line;
        if start >= self.len() {
            return None;
        }
        let end = usize::min(start + self.bytes_per_line, self.len());
        let slice = &self.data()[start..end];
        let hex: String = slice.iter().map(|b| format!("{:02X} ", b)).collect();
        let ascii: String = slice
            .iter()
            .map(|b| {
                if b.is_ascii_graphic() || *b == b' ' {
                    *b as char
                } else {
                    '.'
                }
            })
            .collect();
        Some(format!(
            "{:08X}: {:<width$} {}",
            start,
            hex,
            ascii,
            width = self.bytes_per_line * 3
        ))
    }

    /// Move the cursor one byte to the left.
    pub fn move_left(&mut self) {
        if self.offset > 0 {
            self.offset -= 1;
        }
    }

    /// Move the cursor one byte to the right.
    pub fn move_right(&mut self) {
        if self.offset + 1 < self.len() {
            self.offset += 1;
        }
    }

    /// Move the cursor up by one line.
    pub fn move_up(&mut self) {
        if self.offset >= self.bytes_per_line {
            self.offset -= self.bytes_per_line;
        } else {
            self.offset = 0;
        }
    }

    /// Move the cursor down by one line.
    pub fn move_down(&mut self) {
        if self.offset + self.bytes_per_line < self.len() {
            self.offset += self.bytes_per_line;
        } else {
            self.offset = self.len().saturating_sub(1);
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::files::file_manager::FileManager;
    use std::fs::File;
    use std::io::Write;
    use tempfile::tempdir;

    #[test]
    fn test_hex_dump_format() {
        let hv = HexView::new(FileContents::InMemory(b"Hi".to_vec()));
        let line = hv.line(0).unwrap();
        let expected = format!("{:08X}: {:<48} {}", 0, "48 69 ", "Hi");
        assert_eq!(line, expected);
    }

    #[test]
    fn test_hex_navigation() {
        let hv = HexView::new(FileContents::InMemory((0u8..32).collect()));
        let mut hv = hv;
        assert_eq!(hv.offset(), 0);
        hv.move_right();
        assert_eq!(hv.offset(), 1);
        hv.move_down();
        assert_eq!(hv.offset(), 17);
        hv.move_up();
        assert_eq!(hv.offset(), 1);
        hv.move_left();
        assert_eq!(hv.offset(), 0);
    }

    #[test]
    fn test_large_binary_files() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("large.bin");
        let size = (100 * 1024 * 1024 + 1) as usize; // slightly above mmap threshold
        let mut f = File::create(&path).unwrap();
        f.write_all(&vec![0u8; size]).unwrap();
        drop(f);
        let contents = FileManager::read(&path).unwrap();
        let hv = HexView::new(contents);
        assert_eq!(hv.len(), size);
    }
}
