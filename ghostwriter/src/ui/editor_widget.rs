#[allow(dead_code)]
pub struct EditorState {
    pub cursor: crate::editor::cursor::Cursor,
    pub selection: Option<crate::editor::selection::Selection>,
    pub scroll_x: u16,
    pub scroll_y: u16,
}

#[allow(dead_code)]
pub struct EditorWidget<'a> {
    rope: &'a crate::editor::rope::Rope,
}

#[allow(dead_code)]
impl<'a> EditorWidget<'a> {
    pub fn new(rope: &'a crate::editor::rope::Rope) -> Self {
        Self { rope }
    }
}

impl<'a> ratatui::widgets::StatefulWidget for EditorWidget<'a> {
    type State = EditorState;

    fn render(
        self,
        area: ratatui::layout::Rect,
        buf: &mut ratatui::buffer::Buffer,
        state: &mut Self::State,
    ) {
        let total_lines = self.rope.as_string().lines().count();
        let line_number_width = total_lines.to_string().len() as u16 + 1;
        for y in 0..area.height {
            let line_idx = state.scroll_y as usize + y as usize;
            if let Some(line) = self.rope.line_at(line_idx) {
                let ln = format!(
                    "{:>width$} ",
                    line_idx + 1,
                    width = line_number_width as usize - 1
                );
                let y_pos = area.y + y;
                buf.set_string(area.x, y_pos, ln, ratatui::style::Style::default());
                let visible: String = line
                    .chars()
                    .skip(state.scroll_x as usize)
                    .take((area.width - line_number_width) as usize)
                    .collect();
                buf.set_string(
                    area.x + line_number_width,
                    y_pos,
                    visible.clone(),
                    ratatui::style::Style::default(),
                );

                if let Some(sel) = &state.selection {
                    let mut sel = sel.clone();
                    sel.normalize();
                    if line_idx >= sel.start.line && line_idx <= sel.end.line {
                        let start_col = if line_idx == sel.start.line {
                            sel.start.column
                        } else {
                            0
                        };
                        let end_col = if line_idx == sel.end.line {
                            sel.end.column
                        } else {
                            line.chars().count()
                        };
                        for col in start_col..end_col {
                            if col < state.scroll_x as usize {
                                continue;
                            }
                            if col
                                >= state.scroll_x as usize
                                    + (area.width - line_number_width) as usize
                            {
                                break;
                            }
                            let x = area.x + line_number_width + (col as u16 - state.scroll_x);
                            if let Some(c) = buf.cell_mut((x, y_pos)) {
                                c.set_style(
                                    ratatui::style::Style::default()
                                        .add_modifier(ratatui::style::Modifier::REVERSED),
                                );
                            }
                        }
                    }
                }

                if state.cursor.line == line_idx {
                    let col = state.cursor.column;
                    if col >= state.scroll_x as usize
                        && col < state.scroll_x as usize + (area.width - line_number_width) as usize
                    {
                        let x = area.x + line_number_width + (col as u16 - state.scroll_x);
                        if let Some(c) = buf.cell_mut((x, y_pos)) {
                            c.set_style(
                                ratatui::style::Style::default()
                                    .add_modifier(ratatui::style::Modifier::REVERSED),
                            );
                        }
                    }
                }
            }
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::editor::{cursor::Cursor, rope::Rope, selection::Selection};
    use ratatui::Terminal;
    use ratatui::backend::TestBackend;
    use ratatui::prelude::*;

    #[test]
    fn test_text_rendering() {
        let rope = Rope::from_str("hello\nworld");
        let mut state = EditorState {
            cursor: Cursor {
                line: 10,
                column: 0,
            },
            selection: None,
            scroll_x: 0,
            scroll_y: 0,
        };
        let backend = TestBackend::new(7, 2);
        let mut terminal = Terminal::new(backend).unwrap();
        terminal
            .draw(|f| {
                let area = Rect::new(0, 0, 7, 2);
                f.render_stateful_widget(EditorWidget::new(&rope), area, &mut state);
            })
            .unwrap();
        let buffer = terminal.backend().buffer();
        let width = 7u16;
        let expected = ["1 hello", "2 world"];
        for (y, exp) in expected.iter().enumerate() {
            let mut line = String::new();
            for x in 0..width {
                line.push_str(buffer[(x, y as u16)].symbol());
            }
            assert_eq!(line, format!("{exp:<width$}", width = width as usize));
        }
    }

    #[test]
    fn test_cursor_visualization() {
        let rope = Rope::from_str("hello");
        let mut state = EditorState {
            cursor: Cursor { line: 0, column: 1 },
            selection: None,
            scroll_x: 0,
            scroll_y: 0,
        };
        let backend = TestBackend::new(7, 1);
        let mut terminal = Terminal::new(backend).unwrap();
        terminal
            .draw(|f| {
                let area = Rect::new(0, 0, 7, 1);
                f.render_stateful_widget(EditorWidget::new(&rope), area, &mut state);
            })
            .unwrap();
        let buffer = terminal.backend().buffer();
        let cell = &buffer[(3, 0)];
        assert!(cell.modifier.contains(Modifier::REVERSED));
    }

    #[test]
    fn test_selection_highlighting() {
        let rope = Rope::from_str("hello");
        let mut state = EditorState {
            cursor: Cursor::new(),
            selection: Some(Selection {
                start: Cursor { line: 0, column: 1 },
                end: Cursor { line: 0, column: 3 },
            }),
            scroll_x: 0,
            scroll_y: 0,
        };
        let backend = TestBackend::new(7, 1);
        let mut terminal = Terminal::new(backend).unwrap();
        terminal
            .draw(|f| {
                let area = Rect::new(0, 0, 7, 1);
                f.render_stateful_widget(EditorWidget::new(&rope), area, &mut state);
            })
            .unwrap();
        let buffer = terminal.backend().buffer();
        assert!(buffer[(3, 0)].modifier.contains(Modifier::REVERSED));
        assert!(buffer[(4, 0)].modifier.contains(Modifier::REVERSED));
    }

    #[test]
    fn test_scrolling_behavior() {
        let mut text = String::new();
        for i in 0..5 {
            text.push_str(&format!("{}\n", i));
        }
        let rope = Rope::from_str(&text);
        let mut state = EditorState {
            cursor: Cursor::new(),
            selection: None,
            scroll_x: 0,
            scroll_y: 2,
        };
        let backend = TestBackend::new(4, 1);
        let mut terminal = Terminal::new(backend).unwrap();
        terminal
            .draw(|f| {
                let area = Rect::new(0, 0, 4, 1);
                f.render_stateful_widget(EditorWidget::new(&rope), area, &mut state);
            })
            .unwrap();
        let buffer = terminal.backend().buffer();
        let mut line = String::new();
        for x in 0..4u16 {
            line.push_str(buffer[(x, 0)].symbol());
        }
        assert_eq!(line, "3 2 ");
    }
}
