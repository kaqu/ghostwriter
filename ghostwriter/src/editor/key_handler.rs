#![allow(dead_code)]

use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};

use super::cursor::{Cursor, line_col_to_index, normalized_text};
use super::rope::Rope;
use super::selection::Selection;

/// Input mode indicating which component should receive events.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub enum InputMode {
    /// Standard text editing mode
    #[default]
    Editing,
    /// File picker overlay or other modal component
    Picker,
}

/// Handles key events and updates editor state accordingly.
#[derive(Debug, Default)]
pub struct KeyHandler {
    mode: InputMode,
}

impl KeyHandler {
    /// Create a new key handler in editing mode.
    pub fn new() -> Self {
        Self {
            mode: InputMode::Editing,
        }
    }

    /// Set the active input mode.
    pub fn set_mode(&mut self, mode: InputMode) {
        self.mode = mode;
    }

    fn is_navigation(code: KeyCode) -> bool {
        matches!(
            code,
            KeyCode::Left
                | KeyCode::Right
                | KeyCode::Up
                | KeyCode::Down
                | KeyCode::Home
                | KeyCode::End
                | KeyCode::PageUp
                | KeyCode::PageDown
        )
    }

    /// Handle a key event in editing mode.
    pub fn handle(
        &mut self,
        event: KeyEvent,
        rope: &mut Rope,
        cursor: &mut Cursor,
        selection: &mut Option<Selection>,
    ) {
        if self.mode != InputMode::Editing {
            return;
        }
        let shift = event.modifiers.contains(KeyModifiers::SHIFT);
        let alt = event.modifiers.contains(KeyModifiers::ALT);
        let ctrl = event.modifiers.contains(KeyModifiers::CONTROL);
        let prev = *cursor;
        match event.code {
            KeyCode::Char(c) if !ctrl && !alt => {
                let idx = line_col_to_index(&normalized_text(rope), cursor.line, cursor.column);
                rope.insert(idx, &c.to_string());
                cursor.move_right(rope);
                *selection = None;
                return;
            }
            KeyCode::Enter => {
                let idx = line_col_to_index(&normalized_text(rope), cursor.line, cursor.column);
                rope.insert(idx, "\n");
                cursor.line += 1;
                cursor.column = 0;
                *selection = None;
                return;
            }
            KeyCode::Tab => {
                let idx = line_col_to_index(&normalized_text(rope), cursor.line, cursor.column);
                rope.insert(idx, "    ");
                for _ in 0..4 {
                    cursor.move_right(rope);
                }
                *selection = None;
                return;
            }
            KeyCode::Backspace => {
                if let Some(sel) = selection.take() {
                    sel.delete(rope);
                    cursor.validate(rope);
                    return;
                }
                if cursor.line == 0 && cursor.column == 0 {
                    return;
                }
                cursor.move_left(rope);
                let idx = line_col_to_index(&normalized_text(rope), cursor.line, cursor.column);
                rope.delete(idx..idx + 1);
                return;
            }
            KeyCode::Esc => {
                *selection = None;
                return;
            }
            KeyCode::Left => {
                if alt {
                    cursor.move_prev_word(rope);
                } else {
                    cursor.move_left(rope);
                }
            }
            KeyCode::Right => {
                if alt {
                    cursor.move_next_word(rope);
                } else {
                    cursor.move_right(rope);
                }
            }
            KeyCode::Up => {
                cursor.move_up(rope);
            }
            KeyCode::Down => {
                cursor.move_down(rope);
            }
            KeyCode::Home => {
                if alt || ctrl {
                    cursor.move_doc_start();
                } else {
                    cursor.move_line_start();
                }
            }
            KeyCode::End => {
                if alt || ctrl {
                    cursor.move_doc_end(rope);
                } else {
                    cursor.move_line_end(rope);
                }
            }
            KeyCode::PageUp => {
                for _ in 0..10 {
                    cursor.move_up(rope);
                }
            }
            KeyCode::PageDown => {
                for _ in 0..10 {
                    cursor.move_down(rope);
                }
            }
            _ => {}
        }

        if Self::is_navigation(event.code) {
            if shift {
                match selection {
                    Some(sel) => sel.extend(*cursor),
                    None => {
                        *selection = Some(Selection {
                            start: prev,
                            end: *cursor,
                        })
                    }
                }
            } else {
                *selection = None;
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::editor::rope::Rope;

    fn key(code: KeyCode, mods: KeyModifiers) -> KeyEvent {
        KeyEvent::new(code, mods)
    }

    #[test]
    fn test_navigation_key_handling() {
        let mut rope = Rope::from_str("hello world\nsecond");
        let mut cursor = Cursor::new();
        let mut handler = KeyHandler::new();
        let mut sel = None;
        handler.handle(
            key(KeyCode::Right, KeyModifiers::ALT),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert_eq!(cursor.position(), (0, 4));
        handler.handle(
            key(KeyCode::Right, KeyModifiers::ALT),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert_eq!(cursor.position(), (0, 10));
        handler.handle(
            key(KeyCode::Left, KeyModifiers::ALT),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert_eq!(cursor.position(), (0, 6));
        handler.handle(
            key(KeyCode::Home, KeyModifiers::CONTROL),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert_eq!(cursor.position(), (0, 0));
        handler.handle(
            key(KeyCode::End, KeyModifiers::ALT),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert_eq!(cursor.position(), (1, 6));
    }

    #[test]
    fn test_selection_with_shift() {
        let mut rope = Rope::from_str("abc");
        let mut cursor = Cursor::new();
        let mut handler = KeyHandler::new();
        let mut sel = None;
        handler.handle(
            key(KeyCode::Right, KeyModifiers::SHIFT),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert!(sel.is_some());
        let s = sel.unwrap();
        assert_eq!(s.start.position(), (0, 0));
        assert_eq!(s.end.position(), (0, 1));
    }

    #[test]
    fn test_text_input_processing() {
        let mut rope = Rope::new();
        let mut cursor = Cursor::new();
        let mut handler = KeyHandler::new();
        let mut sel = None;
        handler.handle(
            key(KeyCode::Char('a'), KeyModifiers::empty()),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        handler.handle(
            key(KeyCode::Char('b'), KeyModifiers::empty()),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        handler.handle(
            key(KeyCode::Char('c'), KeyModifiers::empty()),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert_eq!(rope.as_string(), "abc");
        assert_eq!(cursor.position(), (0, 3));
    }

    #[test]
    fn test_special_key_handling() {
        let mut rope = Rope::from_str("ab");
        let mut cursor = Cursor { line: 0, column: 2 };
        let mut handler = KeyHandler::new();
        let mut sel = None;
        handler.handle(
            key(KeyCode::Enter, KeyModifiers::empty()),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert_eq!(rope.as_string(), "ab\n");
        assert_eq!(cursor.position(), (1, 0));
        handler.handle(
            key(KeyCode::Char('c'), KeyModifiers::empty()),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        handler.handle(
            key(KeyCode::Backspace, KeyModifiers::empty()),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert_eq!(rope.as_string(), "ab\n");
        handler.handle(
            key(KeyCode::Esc, KeyModifiers::empty()),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert!(sel.is_none());
    }

    #[test]
    fn test_tab_inserts_spaces() {
        let mut rope = Rope::new();
        let mut cursor = Cursor::new();
        let mut handler = KeyHandler::new();
        let mut sel = None;
        handler.handle(
            key(KeyCode::Tab, KeyModifiers::empty()),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert_eq!(rope.as_string(), "    ");
        assert_eq!(cursor.position(), (0, 4));
    }

    #[test]
    fn test_page_navigation() {
        let mut text = String::new();
        for _ in 0..30 {
            text.push_str("line\n");
        }
        let mut rope = Rope::from_str(&text);
        let mut cursor = Cursor::new();
        let mut handler = KeyHandler::new();
        let mut sel = None;
        handler.handle(
            key(KeyCode::PageDown, KeyModifiers::empty()),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert_eq!(cursor.line, 10);
        handler.handle(
            key(KeyCode::PageUp, KeyModifiers::empty()),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert_eq!(cursor.line, 0);
    }

    #[test]
    fn test_shift_selection_across_lines() {
        let mut rope = Rope::from_str("a\nb\n");
        let mut cursor = Cursor::new();
        let mut handler = KeyHandler::new();
        let mut sel = None;
        handler.handle(
            key(KeyCode::Down, KeyModifiers::SHIFT),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert!(sel.is_some());
        let sel = sel.unwrap();
        assert_eq!(sel.start.position(), (0, 0));
        assert_eq!(sel.end.position(), (1, 0));
    }

    #[test]
    fn test_input_mode_respected() {
        let mut rope = Rope::from_str("hi");
        let mut cursor = Cursor::new();
        let mut handler = KeyHandler::new();
        handler.set_mode(InputMode::Picker);
        let mut sel = None;
        handler.handle(
            key(KeyCode::Char('x'), KeyModifiers::empty()),
            &mut rope,
            &mut cursor,
            &mut sel,
        );
        assert_eq!(rope.as_string(), "hi");
        assert_eq!(cursor.position(), (0, 0));
    }
}
