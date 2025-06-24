// undo/redo system module
#![allow(dead_code)]

use super::{cursor::Cursor, rope::Rope};

/// Undo operation types
#[derive(Debug, Clone)]
pub enum UndoOperation {
    Insert { index: usize, text: String },
    Delete { index: usize, text: String },
}

#[derive(Debug, Clone)]
pub struct UndoRecord {
    pub op: UndoOperation,
    pub before: Cursor,
    pub after: Cursor,
}

pub struct UndoStack {
    limit: usize,
    entries: Vec<UndoRecord>,
    position: usize,
}

impl UndoStack {
    pub fn new(limit: usize) -> Self {
        Self {
            limit,
            entries: Vec::new(),
            position: 0,
        }
    }

    pub fn with_limit(limit: usize) -> Self {
        Self::new(limit)
    }

    pub fn push(&mut self, record: UndoRecord) {
        if self.position < self.entries.len() {
            self.entries.truncate(self.position);
        }
        if self.entries.len() >= self.limit {
            let drop = self.entries.len() - self.limit + 1;
            self.entries.drain(0..drop);
            if self.position >= drop {
                self.position -= drop;
            } else {
                self.position = 0;
            }
        }
        self.entries.push(record);
        self.position = self.entries.len();
    }

    pub fn undo(&mut self, rope: &mut Rope, cursor: &mut Cursor) -> bool {
        if self.position == 0 {
            return false;
        }
        self.position -= 1;
        let record = self.entries[self.position].clone();
        match &record.op {
            UndoOperation::Insert { index, text } => {
                rope.delete(*index..*index + text.chars().count());
            }
            UndoOperation::Delete { index, text } => {
                rope.insert(*index, text);
            }
        }
        *cursor = record.before;
        true
    }

    pub fn redo(&mut self, rope: &mut Rope, cursor: &mut Cursor) -> bool {
        if self.position >= self.entries.len() {
            return false;
        }
        let record = self.entries[self.position].clone();
        match &record.op {
            UndoOperation::Insert { index, text } => {
                rope.insert(*index, text);
            }
            UndoOperation::Delete { index, text } => {
                rope.delete(*index..*index + text.chars().count());
            }
        }
        *cursor = record.after;
        self.position += 1;
        true
    }

    pub fn clear(&mut self) {
        self.entries.clear();
        self.position = 0;
    }
}

impl Default for UndoStack {
    fn default() -> Self {
        Self::new(1000)
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_undo_insert_operation() {
        let mut rope = Rope::from_str("hello");
        let mut cursor = Cursor::new();
        let mut stack = UndoStack::new(10);
        let before = cursor;
        rope.insert(0, "a");
        cursor.move_right(&rope);
        let after = cursor;
        stack.push(UndoRecord {
            op: UndoOperation::Insert {
                index: 0,
                text: "a".to_string(),
            },
            before,
            after,
        });
        assert!(stack.undo(&mut rope, &mut cursor));
        assert_eq!(rope.as_string(), "hello");
        assert_eq!(cursor, before);
    }

    #[test]
    fn test_redo_after_undo() {
        let mut rope = Rope::from_str("hello");
        let mut cursor = Cursor::new();
        let mut stack = UndoStack::new(10);
        let before = cursor;
        rope.insert(0, "a");
        cursor.move_right(&rope);
        let after = cursor;
        stack.push(UndoRecord {
            op: UndoOperation::Insert {
                index: 0,
                text: "a".to_string(),
            },
            before,
            after,
        });
        stack.undo(&mut rope, &mut cursor);
        assert!(stack.redo(&mut rope, &mut cursor));
        assert_eq!(rope.as_string(), "ahello");
        assert_eq!(cursor, after);
    }

    #[test]
    fn test_undo_stack_limit() {
        let mut stack = UndoStack::new(2);
        for i in 0..3 {
            stack.push(UndoRecord {
                op: UndoOperation::Insert {
                    index: i,
                    text: "x".to_string(),
                },
                before: Cursor::new(),
                after: Cursor::new(),
            });
        }
        assert_eq!(stack.entries.len(), 2);
    }

    #[test]
    fn test_cursor_position_restoration() {
        let mut rope = Rope::from_str("hi");
        let mut cursor = Cursor::new();
        cursor.move_right(&rope); // cursor at 1
        let mut stack = UndoStack::new(10);
        let before = cursor;
        rope.insert(1, "a");
        cursor.move_right(&rope);
        let after = cursor;
        stack.push(UndoRecord {
            op: UndoOperation::Insert {
                index: 1,
                text: "a".to_string(),
            },
            before,
            after,
        });
        assert!(stack.undo(&mut rope, &mut cursor));
        assert_eq!(cursor, before);
    }
}
