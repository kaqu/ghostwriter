use ropey::Rope;
use std::{io, ops::Range, path::Path};
use unicode_segmentation::UnicodeSegmentation;

/// Line ending style.
#[derive(Clone, Copy, Debug, PartialEq, Eq)]
pub enum Eol {
    Lf,
    CrLf,
}

/// Rope-based text buffer with invalid UTF-8 tracking.
pub struct RopeBuffer {
    rope: Rope,
    has_invalid: bool,
    eol: Eol,
}

impl RopeBuffer {
    /// Create a new `RopeBuffer` from the provided text.
    pub fn from_text(text: &str) -> Self {
        Self {
            rope: Rope::from_str(text),
            has_invalid: false,
            eol: Eol::Lf,
        }
    }

    /// Open a file from disk into a `RopeBuffer`.
    pub fn open<P: AsRef<Path>>(path: P) -> io::Result<Self> {
        let bytes = std::fs::read(path)?;
        let (mut text, has_invalid) = match String::from_utf8(bytes) {
            Ok(s) => (s, false),
            Err(e) => {
                let bytes = e.into_bytes();
                (String::from_utf8_lossy(&bytes).into_owned(), true)
            }
        };
        let eol = if text.contains("\r\n") {
            text = text.replace("\r\n", "\n");
            Eol::CrLf
        } else {
            Eol::Lf
        };
        Ok(Self {
            rope: Rope::from_str(&text),
            has_invalid,
            eol,
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

    /// Return up to `max_lines` lines starting from `first_line`.
    /// Lines are returned without trailing newlines.
    pub fn slice_lines(&self, first_line: usize, max_lines: usize) -> Vec<String> {
        let total = self.rope.len_lines();
        let mut out = Vec::new();
        for i in first_line..std::cmp::min(first_line + max_lines, total) {
            let mut line = self.rope.line(i).to_string();
            if line.ends_with('\n') {
                line.pop();
            }
            out.push(line);
        }
        out
    }

    /// Return the byte index at the start of `line`.
    pub fn line_to_byte(&self, line: usize) -> usize {
        self.rope.line_to_byte(line)
    }

    /// Total number of lines in the buffer.
    pub fn len_lines(&self) -> usize {
        self.rope.len_lines()
    }

    /// Insert `text` at the given byte index.
    pub fn insert(&mut self, byte_idx: usize, text: &str) {
        let char_idx = self.rope.byte_to_char(byte_idx);
        self.rope.insert(char_idx, text);
    }

    /// Delete the bytes in `range`.
    pub fn delete(&mut self, range: Range<usize>) {
        let start = self.rope.byte_to_char(range.start);
        let end = self.rope.byte_to_char(range.end);
        self.rope.remove(start..end);
    }

    /// Return the text within `range` as a [`String`].
    pub fn slice(&self, range: Range<usize>) -> String {
        let start = self.rope.byte_to_char(range.start);
        let end = self.rope.byte_to_char(range.end);
        self.rope.slice(start..end).to_string()
    }

    /// Convert a byte index to a (line, column) pair.
    /// Line and column are both zero-based, and column counts bytes from
    /// the start of the line.
    pub fn byte_to_line_col(&self, byte_idx: usize) -> (usize, usize) {
        let line = self.rope.byte_to_line(byte_idx);
        let line_start = self.rope.line_to_byte(line);
        (line, byte_idx - line_start)
    }

    /// Convert a (line, column) pair to a byte index.
    /// Line and column are zero-based, and column counts bytes from the
    /// start of the line.
    pub fn line_col_to_byte(&self, line: usize, col: usize) -> usize {
        self.rope.line_to_byte(line) + col
    }

    /// Return the byte index of the grapheme cluster immediately to the left
    /// of `byte_idx`, or `None` if at the start of the buffer.
    pub fn grapheme_left(&self, byte_idx: usize) -> Option<usize> {
        if byte_idx == 0 {
            return None;
        }
        let end_char = self.rope.byte_to_char(byte_idx);
        let slice = self.rope.slice(..end_char).to_string();
        UnicodeSegmentation::grapheme_indices(slice.as_str(), true)
            .next_back()
            .map(|(idx, _)| idx)
    }

    /// Return the byte index of the grapheme cluster immediately to the right
    /// of `byte_idx`, or `None` if at the end of the buffer.
    pub fn grapheme_right(&self, byte_idx: usize) -> Option<usize> {
        if byte_idx >= self.rope.len_bytes() {
            return None;
        }
        let start_char = self.rope.byte_to_char(byte_idx);
        let slice = self.rope.slice(start_char..).to_string();
        UnicodeSegmentation::graphemes(slice.as_str(), true)
            .next()
            .map(|g| byte_idx + g.len())
    }

    /// Save the buffer to `path`, preserving original EOL style.
    pub fn save_to<P: AsRef<Path>>(&self, path: P) -> io::Result<()> {
        let mut text = self.rope.to_string();
        if self.eol == Eol::CrLf {
            text = text.replace('\n', "\r\n");
        }
        crate::fs::atomic_write(path.as_ref(), text.as_bytes())
    }

    /// Return the line ending style of this buffer.
    pub fn eol(&self) -> Eol {
        self.eol
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;
    use tempfile::{NamedTempFile, tempdir};

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

    #[test]
    fn insert_and_delete() {
        let mut buf = RopeBuffer::from_text("hello");
        buf.insert(5, " world");
        assert_eq!(buf.text(), "hello world");
        buf.delete(5..11);
        assert_eq!(buf.text(), "hello");
    }

    #[test]
    fn byte_line_col_roundtrip() {
        let buf = RopeBuffer::from_text("one\ntwo\n");
        let (line, col) = buf.byte_to_line_col(5); // 't' in "two"
        assert_eq!((line, col), (1, 1));
        let byte = buf.line_col_to_byte(line, col);
        assert_eq!(byte, 5);
    }

    #[test]
    fn grapheme_navigation() {
        let buf = RopeBuffer::from_text("a\u{0301}😊b");
        // Text bytes: a + accent (3 bytes), emoji (4), b (1) => total 8 bytes
        assert_eq!(buf.grapheme_right(0), Some(3));
        assert_eq!(buf.grapheme_right(3), Some(7));
        assert_eq!(buf.grapheme_right(7), Some(8));
        assert_eq!(buf.grapheme_right(8), None);
        assert_eq!(buf.grapheme_left(8), Some(7));
        assert_eq!(buf.grapheme_left(7), Some(3));
        assert_eq!(buf.grapheme_left(3), Some(0));
        assert_eq!(buf.grapheme_left(0), None);
    }

    #[test]
    fn open_and_save_preserves_crlf() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("test.txt");
        std::fs::write(&path, b"one\r\ntwo\r\n").unwrap();
        let buf = RopeBuffer::open(&path).unwrap();
        assert_eq!(buf.text(), "one\ntwo\n");
        assert_eq!(buf.eol(), Eol::CrLf);
        // modify and save
        buf.save_to(&path).unwrap();
        let data = std::fs::read(&path).unwrap();
        assert_eq!(data, b"one\r\ntwo\r\n");
    }

    #[test]
    fn save_preserves_lf() {
        let dir = tempdir().unwrap();
        let path = dir.path().join("lf.txt");
        let buf = RopeBuffer::from_text("hello\n");
        buf.save_to(&path).unwrap();
        let data = std::fs::read(&path).unwrap();
        assert_eq!(data, b"hello\n");
        assert_eq!(buf.eol(), Eol::Lf);
    }
}
