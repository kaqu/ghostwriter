use std::ops::Range;

use crate::RopeBuffer;

/// Edit operation that can be undone/redone.
pub enum Edit {
    Insert { idx: usize, text: String },
    Delete { idx: usize, text: String },
}

/// Linear undo/redo stack.
pub struct UndoStack {
    past: Vec<Edit>,
    future: Vec<Edit>,
}

impl UndoStack {
    /// Create a new empty `UndoStack`.
    pub fn new() -> Self {
        Self {
            past: Vec::new(),
            future: Vec::new(),
        }
    }

    /// Apply an insert and record it.
    pub fn insert(&mut self, buf: &mut RopeBuffer, idx: usize, text: &str) {
        buf.insert(idx, text);
        match self.past.last_mut() {
            Some(Edit::Insert {
                idx: last_idx,
                text: last_text,
            }) if idx == *last_idx + last_text.len() => {
                last_text.push_str(text);
                self.future.clear();
                return;
            }
            _ => {}
        }
        self.past.push(Edit::Insert {
            idx,
            text: text.to_string(),
        });
        self.future.clear();
    }

    /// Apply a delete and record it.
    pub fn delete(&mut self, buf: &mut RopeBuffer, range: Range<usize>) {
        let start = range.start;
        let end = range.end;
        let removed = buf.slice(start..end);
        buf.delete(start..end);
        self.past.push(Edit::Delete {
            idx: start,
            text: removed,
        });
        self.future.clear();
    }

    /// Undo the most recent edit. Returns `true` if an edit was undone.
    pub fn undo(&mut self, buf: &mut RopeBuffer) -> bool {
        if let Some(edit) = self.past.pop() {
            match &edit {
                Edit::Insert { idx, text } => {
                    buf.delete(*idx..*idx + text.len());
                }
                Edit::Delete { idx, text } => {
                    buf.insert(*idx, text);
                }
            }
            self.future.push(edit);
            true
        } else {
            false
        }
    }

    /// Redo the most recently undone edit. Returns `true` if an edit was redone.
    pub fn redo(&mut self, buf: &mut RopeBuffer) -> bool {
        if let Some(edit) = self.future.pop() {
            match &edit {
                Edit::Insert { idx, text } => {
                    buf.insert(*idx, text);
                }
                Edit::Delete { idx, text } => {
                    buf.delete(*idx..*idx + text.len());
                }
            }
            self.past.push(edit);
            true
        } else {
            false
        }
    }
}

impl Default for UndoStack {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn undo_redo_insert() {
        let mut buf = RopeBuffer::from_text("hi");
        let mut stack = UndoStack::new();
        stack.insert(&mut buf, 2, " there");
        assert_eq!(buf.text(), "hi there");
        assert!(stack.undo(&mut buf));
        assert_eq!(buf.text(), "hi");
        assert!(stack.redo(&mut buf));
        assert_eq!(buf.text(), "hi there");
    }

    #[test]
    fn undo_redo_delete() {
        let mut buf = RopeBuffer::from_text("hello world");
        let mut stack = UndoStack::new();
        stack.delete(&mut buf, 5..11);
        assert_eq!(buf.text(), "hello");
        assert!(stack.undo(&mut buf));
        assert_eq!(buf.text(), "hello world");
        assert!(stack.redo(&mut buf));
        assert_eq!(buf.text(), "hello");
    }

    #[test]
    fn coalesce_adjacent_inserts() {
        let mut buf = RopeBuffer::from_text("");
        let mut stack = UndoStack::new();
        stack.insert(&mut buf, 0, "h");
        stack.insert(&mut buf, 1, "i");
        stack.insert(&mut buf, 2, "!");
        assert_eq!(buf.text(), "hi!");
        assert!(stack.undo(&mut buf));
        assert_eq!(buf.text(), "");
        assert!(!stack.undo(&mut buf));
    }

    #[test]
    fn separate_non_adjacent_inserts() {
        let mut buf = RopeBuffer::from_text("ab");
        let mut stack = UndoStack::new();
        stack.insert(&mut buf, 1, "1");
        stack.insert(&mut buf, 0, "0");
        assert_eq!(buf.text(), "0a1b");
        assert!(stack.undo(&mut buf));
        assert_eq!(buf.text(), "a1b");
        assert!(stack.undo(&mut buf));
        assert_eq!(buf.text(), "ab");
        assert!(!stack.undo(&mut buf));
    }
}
