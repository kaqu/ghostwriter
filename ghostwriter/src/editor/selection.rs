// Text selection module
#![allow(dead_code)]

use super::cursor::Cursor;
use super::rope::Rope;
use std::ops::Range;

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Selection {
    pub start: Cursor,
    pub end: Cursor,
}

impl Selection {
    pub fn new(cursor: Cursor) -> Self {
        Self {
            start: cursor,
            end: cursor,
        }
    }

    pub fn is_empty(&self) -> bool {
        self.start == self.end
    }

    pub fn extend(&mut self, cursor: Cursor) {
        self.end = cursor;
    }

    pub fn normalize(&mut self) {
        if (self.end.line < self.start.line)
            || (self.end.line == self.start.line && self.end.column < self.start.column)
        {
            std::mem::swap(&mut self.start, &mut self.end);
        }
    }

    fn range(&self, rope: &Rope) -> Range<usize> {
        use super::cursor::{line_col_to_index, normalized_text};
        let text = normalized_text(rope);
        let start = line_col_to_index(&text, self.start.line, self.start.column);
        let end = line_col_to_index(&text, self.end.line, self.end.column);
        if start <= end { start..end } else { end..start }
    }

    pub fn copy(&self, rope: &Rope) -> String {
        if self.is_empty() {
            String::new()
        } else {
            rope.slice(self.range(rope))
        }
    }

    pub fn delete(&self, rope: &mut Rope) {
        if !self.is_empty() {
            let range = self.range(rope);
            rope.delete(range);
        }
    }

    pub fn cut(&self, rope: &mut Rope) -> String {
        let text = self.copy(rope);
        self.delete(rope);
        text
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::editor::cursor::Cursor;
    use crate::editor::rope::Rope;

    #[test]
    fn test_selection_with_shift_navigation() {
        let rope = Rope::from_str("hello");
        let mut cursor = Cursor::new();
        cursor.move_right(&rope);
        cursor.move_right(&rope);
        let mut sel = Selection::new(cursor);
        cursor.move_left(&rope); // shift left
        sel.extend(cursor);
        cursor.move_left(&rope);
        sel.extend(cursor);
        sel.normalize();
        assert_eq!(sel.copy(&rope), "he");
    }

    #[test]
    fn test_selection_operations() {
        let mut rope = Rope::from_str("Hello world");
        let mut cursor = Cursor::new();
        let mut sel = Selection::new(cursor);
        for _ in 0..5 {
            cursor.move_right(&rope);
            sel.extend(cursor);
        }
        sel.normalize();
        assert_eq!(sel.copy(&rope), "Hello");
        assert_eq!(sel.cut(&mut rope), "Hello");
        assert_eq!(rope.as_string(), " world");

        let mut rope = Rope::from_str("Hello world");
        let mut cursor = Cursor::new();
        let mut sel = Selection::new(cursor);
        for _ in 0..6 {
            cursor.move_right(&rope);
            sel.extend(cursor);
        }
        sel.delete(&mut rope);
        assert_eq!(rope.as_string(), "world");
    }

    #[test]
    fn test_selection_normalization() {
        let rope = Rope::from_str("abc");
        let mut sel = Selection {
            start: Cursor { line: 0, column: 2 },
            end: Cursor { line: 0, column: 0 },
        };
        sel.normalize();
        assert_eq!(sel.copy(&rope), "ab");
    }

    #[test]
    fn test_selection_across_lines() {
        let rope = Rope::from_str("ab\ncd\nef");
        let mut cursor = Cursor::new();
        cursor.move_right(&rope);
        let mut sel = Selection::new(cursor);
        cursor.move_down(&rope);
        cursor.move_right(&rope);
        sel.extend(cursor);
        sel.normalize();
        assert_eq!(sel.copy(&rope), "b\ncd");
    }
}
