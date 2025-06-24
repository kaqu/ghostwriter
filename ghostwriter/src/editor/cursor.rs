use crate::editor::rope::Rope;

/// Represents a cursor position within a text document.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
#[allow(dead_code)]
pub struct Cursor {
    pub line: usize,
    pub column: usize,
}

#[allow(dead_code)]
impl Cursor {
    /// Create a new cursor at the beginning of the document.
    pub fn new() -> Self {
        Self { line: 0, column: 0 }
    }

    /// Get the current cursor position as `(line, column)`.
    pub fn position(&self) -> (usize, usize) {
        (self.line, self.column)
    }

    /// Move the cursor to the start of the document.
    pub fn move_doc_start(&mut self) {
        self.line = 0;
        self.column = 0;
    }

    /// Move the cursor to the end of the document.
    pub fn move_doc_end(&mut self, rope: &Rope) {
        let text = normalized_text(rope);
        if text.is_empty() {
            self.move_doc_start();
            return;
        }
        self.line = text.lines().count() - 1;
        if let Some(last) = text.lines().last() {
            self.column = last.chars().count();
        } else {
            self.column = 0;
        }
    }

    /// Move the cursor to the start of the current line.
    pub fn move_line_start(&mut self) {
        self.column = 0;
    }

    /// Move the cursor to the end of the current line.
    pub fn move_line_end(&mut self, rope: &Rope) {
        let text = normalized_text(rope);
        if let Some(line) = text.lines().nth(self.line) {
            self.column = line.chars().count();
        }
    }

    /// Move the cursor one character to the left.
    pub fn move_left(&mut self, rope: &Rope) {
        let text = normalized_text(rope);
        let idx = line_col_to_index(&text, self.line, self.column);
        if idx == 0 {
            return;
        }
        let prev_idx = text[..idx]
            .char_indices()
            .last()
            .map(|(i, _)| i)
            .unwrap_or(0);
        let (line, col) = index_to_line_col(&text, prev_idx);
        self.line = line;
        self.column = col;
    }

    /// Move the cursor one character to the right.
    pub fn move_right(&mut self, rope: &Rope) {
        let text = normalized_text(rope);
        let idx = line_col_to_index(&text, self.line, self.column);
        if idx >= text.len() {
            return;
        }
        let next_idx = idx
            + text[idx..]
                .chars()
                .next()
                .map(|c| c.len_utf8())
                .unwrap_or(0);
        let (line, col) = index_to_line_col(&text, next_idx);
        self.line = line;
        self.column = col;
    }

    /// Move the cursor up one line, keeping column if possible.
    pub fn move_up(&mut self, rope: &Rope) {
        if self.line == 0 {
            self.column = 0;
            return;
        }
        self.line -= 1;
        self.validate(rope);
    }

    /// Move the cursor down one line, keeping column if possible.
    pub fn move_down(&mut self, rope: &Rope) {
        let text = normalized_text(rope);
        if self.line + 1 >= text.lines().count() {
            self.move_doc_end(rope);
            return;
        }
        self.line += 1;
        self.validate(rope);
    }

    /// Move the cursor to the previous word start.
    pub fn move_prev_word(&mut self, rope: &Rope) {
        let text = normalized_text(rope);
        let idx = line_col_to_index(&text, self.line, self.column);
        if idx == 0 {
            return;
        }
        let chars: Vec<(usize, char)> = text.char_indices().collect();
        let mut pos = chars
            .iter()
            .position(|(i, _)| *i >= idx)
            .unwrap_or(chars.len());
        while pos > 0 && !is_word_char(chars[pos - 1].1) {
            pos -= 1;
        }
        while pos > 0 && is_word_char(chars[pos - 1].1) {
            pos -= 1;
        }
        let new_idx = if pos < chars.len() { chars[pos].0 } else { 0 };
        let (line, col) = index_to_line_col(&text, new_idx);
        self.line = line;
        self.column = col;
    }

    /// Move the cursor to the next word end.
    pub fn move_next_word(&mut self, rope: &Rope) {
        let text = normalized_text(rope);
        let idx = line_col_to_index(&text, self.line, self.column);
        let chars: Vec<(usize, char)> = text.char_indices().collect();
        let len = chars.len();
        let mut pos = chars.iter().position(|(i, _)| *i > idx).unwrap_or(len);
        while pos < len && !is_word_char(chars[pos].1) {
            pos += 1;
        }
        if pos >= len {
            self.move_doc_end(rope);
            return;
        }
        let mut end = pos;
        while end < len && is_word_char(chars[end].1) {
            end += 1;
        }
        let end_idx = if end == 0 { 0 } else { chars[end - 1].0 };
        let (line, col) = index_to_line_col(&text, end_idx);
        self.line = line;
        self.column = col;
    }

    /// Ensure the cursor is within valid bounds of the document.
    pub fn validate(&mut self, rope: &Rope) {
        let text = normalized_text(rope);
        let mut lines: Vec<&str> = text.lines().collect();
        if lines.is_empty() {
            lines.push("");
        }
        if self.line >= lines.len() {
            self.line = lines.len() - 1;
        }
        let max_col = lines[self.line].chars().count();
        if self.column > max_col {
            self.column = max_col;
        }
    }
}

pub(crate) fn normalized_text(rope: &Rope) -> String {
    rope.as_string().replace("\r\n", "\n").replace('\r', "\n")
}

pub(crate) fn line_col_to_index(text: &str, line: usize, column: usize) -> usize {
    let mut l = 0;
    let mut c = 0;
    for (i, ch) in text.char_indices() {
        if l == line && c == column {
            return i;
        }
        if ch == '\n' {
            l += 1;
            c = 0;
        } else {
            c += 1;
        }
    }
    if l == line && c == column {
        return text.len();
    }
    text.len()
}

pub(crate) fn index_to_line_col(text: &str, idx: usize) -> (usize, usize) {
    let mut l = 0;
    let mut c = 0;
    for (i, ch) in text.char_indices() {
        if i >= idx {
            break;
        }
        if ch == '\n' {
            l += 1;
            c = 0;
        } else {
            c += 1;
        }
    }
    (l, c)
}

#[allow(dead_code)]
fn is_word_char(ch: char) -> bool {
    ch.is_alphanumeric() || ch == '_'
}

#[cfg(test)]
mod tests {
    use super::*;

    fn rope(text: &str) -> Rope {
        Rope::from_str(text)
    }

    #[test]
    fn test_cursor_word_navigation() {
        let r = rope("hello world foo_bar");
        let mut c = Cursor::new();
        c.move_next_word(&r);
        assert_eq!(c.position(), (0, 4));
        c.move_next_word(&r);
        assert_eq!(c.position(), (0, 10));
        c.move_prev_word(&r);
        assert_eq!(c.position(), (0, 6));
        c.move_prev_word(&r);
        assert_eq!(c.position(), (0, 0));
    }

    #[test]
    fn test_cursor_line_boundaries() {
        let r = rope("line1\nline2");
        let mut c = Cursor { line: 1, column: 2 };
        c.move_line_start();
        assert_eq!(c.position(), (1, 0));
        c.column = 1;
        c.move_line_end(&r);
        assert_eq!(c.position(), (1, 5));
    }

    #[test]
    fn test_cursor_document_boundaries() {
        let r = rope("a\nb\nc");
        let mut c = Cursor { line: 1, column: 1 };
        c.move_doc_start();
        assert_eq!(c.position(), (0, 0));
        c.move_doc_end(&r);
        assert_eq!(c.position(), (2, 1));
    }

    #[test]
    fn test_cursor_validation() {
        let r = rope("foo\r\nbar");
        let mut c = Cursor {
            line: 10,
            column: 10,
        };
        c.validate(&r);
        assert_eq!(c.position(), (1, 3));
    }

    #[test]
    fn test_cursor_boundary_movements() {
        let r = rope("line1\nline2");
        let mut c = Cursor::new();
        c.move_left(&r);
        assert_eq!(c.position(), (0, 0));
        c.move_doc_end(&r);
        let end = c.position();
        c.move_right(&r);
        assert_eq!(c.position(), end);
        c.move_doc_start();
        c.move_up(&r);
        assert_eq!(c.position(), (0, 0));
        c.move_doc_end(&r);
        c.move_down(&r);
        assert_eq!(c.position(), end);
    }

    #[test]
    fn test_cursor_word_navigation_edges() {
        let r = rope("foo bar");
        let mut c = Cursor::new();
        c.move_prev_word(&r);
        assert_eq!(c.position(), (0, 0));
        c.move_doc_end(&r);
        c.move_next_word(&r);
        assert_eq!(c.position(), (0, 7));
    }
}
