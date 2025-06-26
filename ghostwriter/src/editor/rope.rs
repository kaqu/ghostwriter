#![allow(dead_code)]

use std::cell::{Ref, RefCell};
use std::fmt;
use std::ops::Range;

/// Rope data structure storing text in fixed-size chunks.
#[derive(Debug, Clone)]
pub struct Rope {
    chunks: Vec<Chunk>,
    cache: RefCell<Option<String>>,
}

#[derive(Debug, Clone)]
struct Chunk {
    data: String,
    char_len: usize,
}

const CHUNK_SIZE: usize = 64 * 1024;

impl Chunk {
    fn new(data: String) -> Self {
        let char_len = data.chars().count();
        Self { data, char_len }
    }

    fn update_len(&mut self) {
        self.char_len = self.data.chars().count();
    }
}

impl Rope {
    /// Create an empty rope.
    pub fn new() -> Self {
        Self {
            chunks: Vec::new(),
            cache: RefCell::new(None),
        }
    }

    /// Create a rope from a UTF-8 string.
    pub fn from_str(s: &str) -> Self {
        let mut rope = Self::new();
        rope.push_str(s);
        rope
    }

    /// Create a rope from raw bytes with hex fallback for invalid UTF-8.
    pub fn from_bytes(bytes: &[u8]) -> Self {
        let mut result = String::new();
        let mut i = 0;
        while i < bytes.len() {
            match std::str::from_utf8(&bytes[i..]) {
                Ok(valid) => {
                    result.push_str(valid);
                    break;
                }
                Err(e) => {
                    let valid_up_to = e.valid_up_to();
                    if valid_up_to > 0 {
                        let part =
                            unsafe { std::str::from_utf8_unchecked(&bytes[i..i + valid_up_to]) };
                        result.push_str(part);
                    }
                    let invalid = bytes[i + valid_up_to];
                    result.push_str(&format!("<{:02X}>", invalid));
                    i += valid_up_to + 1;
                }
            }
        }
        Self::from_str(&result)
    }

    /// Push string data onto the end of the rope, splitting into chunks.
    fn push_str(&mut self, mut s: &str) {
        self.cache.borrow_mut().take();
        if let Some(last) = self.chunks.last_mut() {
            let space = CHUNK_SIZE.saturating_sub(last.data.len());
            if space > 0 {
                let take = Self::split_index(s, space);
                last.data.push_str(&s[..take]);
                last.update_len();
                s = &s[take..];
            }
        }
        while !s.is_empty() {
            let take = Self::split_index(s, CHUNK_SIZE);
            self.chunks.push(Chunk::new(s[..take].to_string()));
            s = &s[take..];
        }
    }

    fn split_index(s: &str, max_bytes: usize) -> usize {
        if s.len() <= max_bytes {
            return s.len();
        }
        let mut idx = max_bytes;
        while !s.is_char_boundary(idx) {
            idx -= 1;
        }
        idx
    }

    fn char_to_byte(s: &str, char_idx: usize) -> usize {
        if char_idx >= s.chars().count() {
            return s.len();
        }
        s.char_indices()
            .nth(char_idx)
            .map(|(i, _)| i)
            .unwrap_or(s.len())
    }

    pub fn cached_text(&self) -> Ref<'_, str> {
        if self.cache.borrow().is_none() {
            let mut s = String::new();
            for c in &self.chunks {
                s.push_str(&c.data);
            }
            *self.cache.borrow_mut() = Some(s);
        }
        Ref::map(self.cache.borrow(), |c| c.as_deref().unwrap())
    }

    /// Return a range of lines as owned strings.
    pub fn lines_range(&self, start: usize, count: usize) -> Vec<String> {
        let text = self.cached_text();
        text.lines()
            .skip(start)
            .take(count)
            .map(|s| s.to_string())
            .collect()
    }

    /// Append text to the end of the rope.
    pub fn append(&mut self, text: &str) {
        self.push_str(text);
    }

    /// Number of chunks currently used.
    pub fn chunk_count(&self) -> usize {
        self.chunks.len()
    }

    fn find_chunk(&self, mut index: usize) -> (usize, usize) {
        for (i, chunk) in self.chunks.iter().enumerate() {
            if index <= chunk.char_len {
                return (i, index);
            }
            index -= chunk.char_len;
        }
        let last = self.chunks.len() - 1;
        (last, self.chunks[last].char_len)
    }

    /// Total number of characters in the rope.
    pub fn len_chars(&self) -> usize {
        self.chunks.iter().map(|c| c.char_len).sum()
    }

    /// Insert text at the given character index.
    pub fn insert(&mut self, index: usize, text: &str) {
        self.cache.borrow_mut().take();
        if self.chunks.is_empty() {
            self.push_str(text);
            return;
        }
        let (chunk_idx, off) = self.find_chunk(index);
        let chunk = &mut self.chunks[chunk_idx];
        let byte_off = Self::char_to_byte(&chunk.data, off);
        chunk.data.insert_str(byte_off, text);
        chunk.update_len();
        self.rebalance(chunk_idx);
    }

    /// Delete the text within the given character range.
    pub fn delete(&mut self, range: Range<usize>) {
        self.cache.borrow_mut().take();
        if range.start >= range.end {
            return;
        }
        let (start_chunk, start_off) = self.find_chunk(range.start);
        let (end_chunk, end_off) = self.find_chunk(range.end);
        if start_chunk == end_chunk {
            let chunk = &mut self.chunks[start_chunk];
            let start_b = Self::char_to_byte(&chunk.data, start_off);
            let end_b = Self::char_to_byte(&chunk.data, end_off);
            chunk.data.replace_range(start_b..end_b, "");
            chunk.update_len();
            if chunk.data.is_empty() {
                self.chunks.remove(start_chunk);
            }
            return;
        }
        {
            let chunk = &mut self.chunks[start_chunk];
            let start_b = Self::char_to_byte(&chunk.data, start_off);
            chunk.data.replace_range(start_b.., "");
            chunk.update_len();
        }
        {
            let chunk = &mut self.chunks[end_chunk];
            let end_b = Self::char_to_byte(&chunk.data, end_off);
            chunk.data.replace_range(..end_b, "");
            chunk.update_len();
        }
        for i in (start_chunk + 1..end_chunk).rev() {
            self.chunks.remove(i);
        }
        if start_chunk + 1 < self.chunks.len()
            && self.chunks[start_chunk].data.len() + self.chunks[start_chunk + 1].data.len()
                <= CHUNK_SIZE
        {
            let chunk = self.chunks.remove(start_chunk + 1);
            self.chunks[start_chunk].data.push_str(&chunk.data);
            self.chunks[start_chunk].update_len();
        }
    }

    fn rebalance(&mut self, mut idx: usize) {
        self.cache.borrow_mut().take();
        while idx < self.chunks.len() && self.chunks[idx].data.len() > CHUNK_SIZE {
            let split_at = Self::split_index(&self.chunks[idx].data, CHUNK_SIZE);
            let extra = self.chunks[idx].data.split_off(split_at);
            self.chunks[idx].update_len();
            self.chunks.insert(idx + 1, Chunk::new(extra));
            idx += 1;
        }
    }

    /// Return the characters within the range as a `String`.
    pub fn slice(&self, range: Range<usize>) -> String {
        let mut result = String::new();
        let mut pos = 0;
        for chunk in &self.chunks {
            let end_pos = pos + chunk.char_len;
            if range.start < end_pos && range.end > pos {
                let start_idx = range.start.saturating_sub(pos);
                let end_idx = if range.end < end_pos {
                    range.end - pos
                } else {
                    chunk.char_len
                };
                let start_b = Self::char_to_byte(&chunk.data, start_idx);
                let end_b = Self::char_to_byte(&chunk.data, end_idx);
                result.push_str(&chunk.data[start_b..end_b]);
            }
            if end_pos >= range.end {
                break;
            }
            pos = end_pos;
        }
        result
    }

    /// Get the character at the given index.
    pub fn char_at(&self, index: usize) -> Option<char> {
        let (chunk_idx, off) = self.find_chunk(index);
        self.chunks.get(chunk_idx)?.data.chars().nth(off)
    }

    /// Get the specified line as a `String`.
    pub fn line_at(&self, line_idx: usize) -> Option<String> {
        self.as_string()
            .lines()
            .nth(line_idx)
            .map(|s| s.to_string())
    }

    /// Convert the rope into a single `String`.
    pub fn as_string(&self) -> String {
        let mut s = String::new();
        for c in &self.chunks {
            s.push_str(&c.data);
        }
        s
    }
}

impl fmt::Display for Rope {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        for c in &self.chunks {
            write!(f, "{}", c.data)?;
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_rope_insert_at_position() {
        let mut r = Rope::from_str("Hello World");
        r.insert(6, "Rust ");
        assert_eq!(r.as_string(), "Hello Rust World");
        assert_eq!(r.len_chars(), 16);
    }

    #[test]
    fn test_rope_delete_range() {
        let mut r = Rope::from_str("Hello Rust World");
        r.delete(6..11);
        assert_eq!(r.as_string(), "Hello World");
    }

    #[test]
    fn test_rope_line_operations() {
        let r = Rope::from_str("line1\nline2\nline3");
        assert_eq!(r.line_at(1).as_deref(), Some("line2"));
        assert_eq!(r.as_string().lines().count(), 3);
    }

    #[test]
    fn test_rope_large_file_performance() {
        let chunk = "a".repeat(1024 * 1024); // 1MB
        let mut data = String::new();
        for _ in 0..5 {
            data.push_str(&chunk);
        }
        let mut r = Rope::from_str(&data);
        r.insert(r.len_chars() / 2, "b");
        r.delete(r.len_chars() / 2..r.len_chars() / 2 + 1);
        assert_eq!(r.len_chars(), 5 * 1024 * 1024);
    }

    #[test]
    fn test_utf8_invalid_sequences() {
        let bytes = [0x66, 0x80, 0x67];
        let r = Rope::from_bytes(&bytes);
        assert_eq!(r.as_string(), "f<80>g");
    }

    #[test]
    fn test_rope_insert_empty_and_delete_all() {
        let mut r = Rope::new();
        r.insert(0, "abc");
        assert_eq!(r.as_string(), "abc");
        r.delete(0..r.len_chars());
        assert!(r.as_string().is_empty());
    }

    #[test]
    fn test_rope_slice_across_chunks() {
        let mut data = "a".repeat(CHUNK_SIZE + 10);
        data.push_str(&"b".repeat(20));
        let r = Rope::from_str(&data);
        let slice = r.slice(CHUNK_SIZE - 5..CHUNK_SIZE + 15);
        assert_eq!(slice.chars().count(), 20);
    }

    #[test]
    fn test_char_at_out_of_bounds() {
        let r = Rope::from_str("abc");
        assert_eq!(r.char_at(5), None);
    }

    #[test]
    fn test_line_at_out_of_bounds() {
        let r = Rope::from_str("a\nb");
        assert_eq!(r.line_at(5), None);
    }

    #[test]
    fn test_lines_range_virtual_scrolling() {
        let text = "a\n".repeat(1_000_000);
        let r = Rope::from_str(&text);
        let lines = r.lines_range(999_990, 5);
        assert_eq!(lines.len(), 5);
    }

    #[test]
    fn test_rope_operation_performance() {
        use std::time::Instant;
        let mut r = Rope::new();
        let start = Instant::now();
        for _ in 0..1000 {
            r.append("a");
        }
        let dur = start.elapsed();
        assert!(dur.as_millis() < 200);
    }

    #[test]
    fn test_memory_usage_scaling() {
        let text = "a".repeat(CHUNK_SIZE * 3);
        let r = Rope::from_str(&text);
        assert!(r.chunk_count() <= 4);
    }
}
