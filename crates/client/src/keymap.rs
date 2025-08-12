use crossterm::event::{KeyCode, KeyEvent, KeyModifiers};

/// Direction for cursor movement or selection.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Direction {
    Left,
    Right,
    Up,
    Down,
}

/// High-level editor command derived from a key event.
#[derive(Debug, Clone, PartialEq, Eq)]
pub enum Command {
    /// Insert the given text at the cursor position.
    Insert(String),
    /// Delete the character before the cursor.
    DeletePrev,
    /// Delete the character at the cursor.
    DeleteNext,
    /// Move the cursor in the given direction without modifying the selection.
    Move(Direction),
    /// Extend the selection in the given direction.
    Select(Direction),
}

/// Translate a crossterm [`KeyEvent`] into an editor [`Command`].
///
/// Returns `None` for keys that have no associated command.
pub fn map_key_event(ev: KeyEvent) -> Option<Command> {
    match ev.code {
        KeyCode::Char(c) => {
            if ev
                .modifiers
                .intersects(KeyModifiers::CONTROL | KeyModifiers::ALT)
            {
                None
            } else {
                Some(Command::Insert(c.to_string()))
            }
        }
        KeyCode::Enter => Some(Command::Insert("\n".into())),
        KeyCode::Tab => Some(Command::Insert("\t".into())),
        KeyCode::Backspace => Some(Command::DeletePrev),
        KeyCode::Delete => Some(Command::DeleteNext),
        KeyCode::Left => Some(if ev.modifiers.contains(KeyModifiers::SHIFT) {
            Command::Select(Direction::Left)
        } else {
            Command::Move(Direction::Left)
        }),
        KeyCode::Right => Some(if ev.modifiers.contains(KeyModifiers::SHIFT) {
            Command::Select(Direction::Right)
        } else {
            Command::Move(Direction::Right)
        }),
        KeyCode::Up => Some(if ev.modifiers.contains(KeyModifiers::SHIFT) {
            Command::Select(Direction::Up)
        } else {
            Command::Move(Direction::Up)
        }),
        KeyCode::Down => Some(if ev.modifiers.contains(KeyModifiers::SHIFT) {
            Command::Select(Direction::Down)
        } else {
            Command::Move(Direction::Down)
        }),
        _ => None,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn maps_char_to_insert() {
        let ev = KeyEvent::new(KeyCode::Char('a'), KeyModifiers::NONE);
        assert_eq!(map_key_event(ev), Some(Command::Insert("a".into())));
    }

    #[test]
    fn maps_enter_to_newline() {
        let ev = KeyEvent::new(KeyCode::Enter, KeyModifiers::NONE);
        assert_eq!(map_key_event(ev), Some(Command::Insert("\n".into())));
    }

    #[test]
    fn maps_backspace_to_delete_prev() {
        let ev = KeyEvent::new(KeyCode::Backspace, KeyModifiers::NONE);
        assert_eq!(map_key_event(ev), Some(Command::DeletePrev));
    }

    #[test]
    fn maps_left_to_move_left() {
        let ev = KeyEvent::new(KeyCode::Left, KeyModifiers::NONE);
        assert_eq!(map_key_event(ev), Some(Command::Move(Direction::Left)));
    }

    #[test]
    fn maps_shift_left_to_select_left() {
        let ev = KeyEvent::new(KeyCode::Left, KeyModifiers::SHIFT);
        assert_eq!(map_key_event(ev), Some(Command::Select(Direction::Left)));
    }
}
