// File history stack implementation
#![allow(dead_code)]

use std::path::PathBuf;

use crate::editor::{cursor::Cursor, undo::UndoStack};

/// State of an open file including editor context.
#[derive(Debug)]
pub struct FileState {
    pub path: PathBuf,
    pub cursor: Cursor,
    pub scroll_x: u16,
    pub scroll_y: u16,
    pub undo: UndoStack,
}

/// Browser-like file history with branching support.
#[derive(Debug, Default)]
pub struct FileHistory {
    entries: Vec<FileState>,
    position: usize,
}

impl FileHistory {
    /// Create an empty file history.
    pub fn new() -> Self {
        Self {
            entries: Vec::new(),
            position: 0,
        }
    }

    /// Push a new file state onto the history, truncating any forward entries.
    pub fn push(&mut self, state: FileState) {
        if self.position < self.entries.len() {
            self.entries.truncate(self.position + 1);
        }
        self.entries.push(state);
        self.position = self.entries.len() - 1;
    }

    /// Move backward in history, returning the new current state.
    pub fn back(&mut self) -> Option<&FileState> {
        if self.position == 0 || self.entries.is_empty() {
            return None;
        }
        self.position -= 1;
        self.entries.get(self.position)
    }

    /// Move forward in history, returning the new current state.
    pub fn forward(&mut self) -> Option<&FileState> {
        if self.position + 1 >= self.entries.len() {
            return None;
        }
        self.position += 1;
        self.entries.get(self.position)
    }

    /// Get the current file state.
    pub fn current(&self) -> Option<&FileState> {
        self.entries.get(self.position)
    }

    /// Number of entries stored.
    pub fn len(&self) -> usize {
        self.entries.len()
    }

    /// Whether history is empty.
    pub fn is_empty(&self) -> bool {
        self.entries.is_empty()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::editor::rope::Rope;
    use crate::editor::undo::{UndoOperation, UndoRecord};

    fn sample_state(path: &str) -> FileState {
        let mut undo = UndoStack::new(10);
        let mut rope = Rope::from_str("hi");
        let cursor = Cursor::new();
        rope.insert(2, "!");
        undo.push(UndoRecord {
            op: UndoOperation::Insert {
                index: 2,
                text: "!".into(),
            },
            before: cursor,
            after: cursor,
        });
        FileState {
            path: PathBuf::from(path),
            cursor,
            scroll_x: 0,
            scroll_y: 0,
            undo,
        }
    }

    #[test]
    fn test_history_stack_operations() {
        let mut hist = FileHistory::new();
        hist.push(sample_state("a"));
        hist.push(sample_state("b"));
        assert_eq!(hist.current().unwrap().path, PathBuf::from("b"));
        hist.back();
        assert_eq!(hist.current().unwrap().path, PathBuf::from("a"));
        hist.forward();
        assert_eq!(hist.current().unwrap().path, PathBuf::from("b"));
    }

    #[test]
    fn test_branching_behavior() {
        let mut hist = FileHistory::new();
        hist.push(sample_state("a"));
        hist.push(sample_state("b"));
        hist.back();
        hist.push(sample_state("c"));
        assert_eq!(hist.len(), 2);
        assert_eq!(hist.current().unwrap().path, PathBuf::from("c"));
        assert!(hist.forward().is_none());
        hist.back();
        assert_eq!(hist.current().unwrap().path, PathBuf::from("a"));
        hist.forward();
        assert_eq!(hist.current().unwrap().path, PathBuf::from("c"));
    }

    #[test]
    fn test_state_persistence() {
        let mut hist = FileHistory::new();
        let mut state = sample_state("a");
        state.cursor.line = 5;
        state.scroll_x = 2;
        hist.push(state);
        assert_eq!(hist.current().unwrap().cursor.line, 5);
        hist.back();
        // Should still be None because there was only one entry
        assert!(hist.current().is_some());
        assert_eq!(hist.current().unwrap().cursor.line, 5);
    }

    #[test]
    fn test_unlimited_history() {
        let mut hist = FileHistory::new();
        for i in 0..1000 {
            hist.push(sample_state(&format!("{i}")));
        }
        assert_eq!(hist.len(), 1000);
    }
}
