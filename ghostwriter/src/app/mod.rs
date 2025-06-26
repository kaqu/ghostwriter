use std::path::PathBuf;

use crossterm::event::{Event, KeyCode, KeyEvent, KeyModifiers};
use ratatui::prelude::*;

use crate::editor::{cursor::Cursor, key_handler::KeyHandler, rope::Rope, selection::Selection};
use crate::error::Result;
use crate::files::file_manager::{FileContents, FileManager};
use crate::ui::{
    TerminalUI,
    editor_widget::{EditorState, EditorWidget},
    status_bar::{ConnectionStatus, LockStatus, Mode, StatusBar},
};

/// Basic application state used during editing.
#[derive(Debug)]
pub struct App {
    rope: Rope,
    cursor: Cursor,
    selection: Option<Selection>,
    dirty: bool,
    path: PathBuf,
    handler: KeyHandler,
}

impl App {
    /// Open a file from disk into a new `App` instance.
    pub fn open(path: impl Into<PathBuf>) -> Result<Self> {
        let path = path.into();
        let contents = FileManager::read(&path)?;
        let rope = match contents {
            FileContents::InMemory(d) => Rope::from_bytes(&d),
            FileContents::Mapped(m) => Rope::from_bytes(m.as_ref()),
        };
        Ok(Self {
            rope,
            cursor: Cursor::new(),
            selection: None,
            dirty: false,
            path,
            handler: KeyHandler::new(),
        })
    }

    /// Apply a key event to the editor state.
    pub fn handle_key(&mut self, event: KeyEvent) {
        let before = self.rope.as_string();
        self.handler
            .handle(event, &mut self.rope, &mut self.cursor, &mut self.selection);
        if self.rope.as_string() != before {
            self.dirty = true;
        }
    }

    /// Save the current buffer to disk.
    pub fn save(&mut self) -> Result<()> {
        FileManager::atomic_write(&self.path, self.rope.as_string().as_bytes())?;
        self.dirty = false;
        Ok(())
    }

    /// Draw the editor and status bar widgets.
    pub fn draw<B: Backend>(&mut self, ui: &mut TerminalUI<B>) -> Result<()> {
        let size = ui.terminal().size()?;
        let rect = Rect::new(0, 0, size.width, size.height);
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .constraints([Constraint::Min(1), Constraint::Length(1)])
            .split(rect);
        let mut state = EditorState {
            cursor: self.cursor,
            selection: self.selection.clone(),
            scroll_x: 0,
            scroll_y: 0,
        };
        ui.terminal().draw(|f| {
            f.render_stateful_widget(EditorWidget::new(&self.rope), chunks[0], &mut state);
            f.render_widget(
                StatusBar {
                    file_path: self.path.to_str().unwrap_or_default(),
                    cursor: self.cursor.position(),
                    lock_status: LockStatus::None,
                    connection_status: ConnectionStatus::Online,
                    dirty: self.dirty,
                    mode: Mode::Local,
                },
                chunks[1],
            );
        })?;
        Ok(())
    }

    /// Run a simple event loop until Ctrl+Q is pressed.
    pub fn run(&mut self) -> Result<()> {
        use crossterm::event::{poll, read};
        use std::time::Duration;

        let mut ui = TerminalUI::new()?;
        loop {
            self.draw(&mut ui)?;
            if poll(Duration::from_millis(50))? {
                if let Event::Key(key) = read()? {
                    if key.code == KeyCode::Char('q')
                        && key.modifiers.contains(KeyModifiers::CONTROL)
                    {
                        break;
                    }
                    self.handle_key(key);
                }
            }
        }
        ui.cleanup()?;
        if self.dirty {
            self.save()?;
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ratatui::backend::TestBackend;
    use tempfile::NamedTempFile;

    #[test]
    fn test_open_modify_and_save() {
        let mut file = NamedTempFile::new().unwrap();
        std::io::Write::write_all(&mut file, b"hi").unwrap();
        let path = file.path().to_path_buf();
        let mut app = App::open(&path).unwrap();
        app.cursor.move_doc_end(&app.rope);
        app.handle_key(KeyEvent::new(KeyCode::Char('!'), KeyModifiers::empty()));
        app.save().unwrap();
        let data = std::fs::read_to_string(&path).unwrap();
        assert_eq!(data, "hi!");
    }

    #[test]
    fn test_draw_renders_without_error() {
        let file = NamedTempFile::new().unwrap();
        let mut app = App::open(file.path()).unwrap();
        let backend = TestBackend::new(40, 5);
        let mut ui = TerminalUI::with_backend(backend).unwrap();
        assert!(app.draw(&mut ui).is_ok());
    }
}
