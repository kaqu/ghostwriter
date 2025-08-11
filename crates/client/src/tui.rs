use anyhow::Result;
use crossterm::terminal::{disable_raw_mode, enable_raw_mode};
use ghostwriter_proto::Frame;
use ratatui::{Terminal, backend::Backend, prelude::*, widgets::Paragraph};

/// Terminal user interface renderer.
pub struct Tui<B: Backend> {
    terminal: Terminal<B>,
    raw_mode: bool,
}

impl<B: Backend> Tui<B> {
    /// Create a new instance using the provided backend.
    /// Enables terminal raw mode and hides the cursor.
    pub fn new(backend: B) -> Result<Self> {
        enable_raw_mode()?;
        let mut terminal = Terminal::new(backend)?;
        terminal.hide_cursor()?;
        Ok(Self {
            terminal,
            raw_mode: true,
        })
    }

    /// Create a TUI without enabling raw mode (for tests).
    #[cfg(test)]
    pub fn new_for_test(backend: B) -> Result<Self> {
        let terminal = Terminal::new(backend)?;
        Ok(Self {
            terminal,
            raw_mode: false,
        })
    }

    /// Draw the given frame.
    pub fn draw(&mut self, frame: &Frame) -> Result<()> {
        self.terminal.draw(|f| {
            let size = f.area();
            let text_height = size.height.saturating_sub(1);

            // Text area
            let lines: Vec<ratatui::text::Line<'static>> = frame
                .lines
                .iter()
                .map(|l| ratatui::text::Line::raw(l.text.clone()))
                .collect();
            let text_area = Rect {
                x: 0,
                y: 0,
                width: size.width,
                height: text_height,
            };
            f.render_widget(Paragraph::new(lines), text_area);

            // Status line
            let mut status = frame.status_left.clone();
            let right = frame.status_right.clone();
            let total_width = size.width as usize;
            if status.len() + right.len() < total_width {
                let padding = total_width - status.len() - right.len();
                status.push_str(&" ".repeat(padding));
            }
            status.push_str(&right);
            let status_area = Rect {
                x: 0,
                y: text_height,
                width: size.width,
                height: 1,
            };
            f.render_widget(Paragraph::new(status), status_area);

            // Cursor placement
            if let Some(cur) = frame.cursors.first() {
                let x = cur.col;
                let y = (cur.line - frame.first_line) as u16;
                f.set_cursor_position((x, y));
            }
        })?;
        Ok(())
    }
}

impl<B: Backend> Drop for Tui<B> {
    fn drop(&mut self) {
        if self.raw_mode {
            let _ = self.terminal.show_cursor();
            let _ = disable_raw_mode();
        }
    }
}

#[cfg(test)]
impl<B: Backend> Tui<B> {
    pub fn backend(&mut self) -> &mut B {
        self.terminal.backend_mut()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ghostwriter_proto::{Cursor, Line, StyleSpan};
    use ratatui::backend::TestBackend;

    #[test]
    fn draws_frame_and_status() {
        let backend = TestBackend::new(10, 3);
        let mut tui = Tui::new_for_test(backend).unwrap();

        let frame = Frame {
            id: "editor".into(),
            kind: "editor".into(),
            doc_v: 1,
            first_line: 0,
            cols: 10,
            rows: 3,
            lines: vec![Line {
                text: "hello".into(),
                spans: vec![StyleSpan {
                    start_col: 0,
                    end_col: 5,
                    class_name: "sel".into(),
                }],
            }],
            cursors: vec![Cursor { line: 0, col: 5 }],
            status_left: "L".into(),
            status_right: "R".into(),
        };

        tui.draw(&frame).unwrap();

        let backend = tui.backend();
        let buffer = backend.buffer().clone();
        let cursor = backend.get_cursor_position().unwrap();
        assert_eq!(
            buffer,
            Buffer::with_lines(vec!["hello     ", "          ", "L        R",]),
        );
        assert_eq!(cursor, (5, 0).into());
    }
}
