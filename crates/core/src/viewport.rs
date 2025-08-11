// Viewport composer: converts buffer and selections into proto::Frame.

use std::ops::Range;

use crate::RopeBuffer;
use ghostwriter_proto::{Cursor, Frame, StyleSpan};

/// Compose a frame for the given viewport parameters.
pub fn compose(
    buf: &RopeBuffer,
    first_line: usize,
    rows: usize,
    cols: usize,
    hscroll: usize,
    cursors: &[(usize, usize)],
    selection: Option<Range<usize>>,
) -> Frame {
    // Extract and horizontally slice lines.
    let raw_lines = buf.slice_lines(first_line, rows);
    let lines: Vec<String> = raw_lines
        .into_iter()
        .map(|line| {
            if hscroll < line.len() {
                let end = (hscroll + cols).min(line.len());
                line[hscroll..end].to_string()
            } else {
                String::new()
            }
        })
        .collect();

    let mut spans = Vec::new();

    for (row, line) in lines.iter().enumerate() {
        let trimmed = line.trim_end_matches([' ', '\t']);
        if trimmed.len() < line.len() {
            spans.push(StyleSpan {
                row: row as u16,
                start_col: trimmed.len() as u16,
                end_col: line.len() as u16,
                class: "ws".into(),
            });
        }

        for (col, ch) in line.chars().enumerate() {
            if ch == '\u{FFFD}' {
                spans.push(StyleSpan {
                    row: row as u16,
                    start_col: col as u16,
                    end_col: (col + 1) as u16,
                    class: "err".into(),
                });
            }
        }
    }

    if let Some(sel) = selection {
        let (start_line, start_col) = buf.byte_to_line_col(sel.start);
        let (end_line, end_col) = buf.byte_to_line_col(sel.end);
        for line in start_line..=end_line {
            if line < first_line || line >= first_line + rows {
                continue;
            }
            let row = (line - first_line) as u16;
            let start = if line == start_line { start_col } else { 0 };
            let end = if line == end_line {
                end_col
            } else {
                buf.slice_lines(line, 1)
                    .first()
                    .map(|s| s.len())
                    .unwrap_or(0)
            };
            if end > start {
                let adj_start = start.saturating_sub(hscroll);
                let adj_end = end.saturating_sub(hscroll);
                if adj_end > 0 && adj_start < cols {
                    spans.push(StyleSpan {
                        row,
                        start_col: adj_start.min(cols) as u16,
                        end_col: adj_end.min(cols) as u16,
                        class: "sel".into(),
                    });
                }
            }
        }
    }

    let mut cursor_out = Vec::new();
    for &(line, col) in cursors {
        if line < first_line || line >= first_line + rows {
            continue;
        }
        if col < hscroll || col >= hscroll + cols {
            continue;
        }
        cursor_out.push(Cursor {
            row: (line - first_line) as u16,
            col: (col - hscroll) as u16,
        });
    }

    let status = if let Some(&(line, col)) = cursors.first() {
        format!("Ln {}, Col {}", line + 1, col + 1)
    } else {
        String::new()
    };

    Frame {
        first_line: first_line as u64,
        lines,
        spans,
        cursors: cursor_out,
        status,
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::RopeBuffer;
    use ghostwriter_proto::{Cursor, StyleSpan};

    #[test]
    fn compose_basic() {
        let buf = RopeBuffer::from_text("hello  \nworld\n");
        let cursor = (0usize, 1usize);
        let selection = Some(1..4);
        let frame = compose(&buf, 0, 2, 80, 0, &[cursor], selection);
        assert_eq!(
            frame.lines,
            vec!["hello  ".to_string(), "world".to_string()]
        );
        assert_eq!(frame.cursors, vec![Cursor { row: 0, col: 1 }]);
        assert_eq!(frame.status, "Ln 1, Col 2");
        assert!(frame.spans.contains(&StyleSpan {
            row: 0,
            start_col: 1,
            end_col: 4,
            class: "sel".into()
        }));
        assert!(frame.spans.contains(&StyleSpan {
            row: 0,
            start_col: 5,
            end_col: 7,
            class: "ws".into()
        }));
    }

    #[test]
    fn compose_error_span() {
        let buf = RopeBuffer::from_text("bad\u{FFFD}line\n");
        let frame = compose(&buf, 0, 1, 80, 0, &[], None);
        assert!(frame.spans.contains(&StyleSpan {
            row: 0,
            start_col: 3,
            end_col: 4,
            class: "err".into()
        }));
    }

    #[test]
    fn compose_hscroll_beyond_line() {
        let buf = RopeBuffer::from_text("short\n");
        // hscroll past the end of the line should yield an empty visible line
        let frame = compose(&buf, 0, 1, 80, 10, &[], None);
        assert_eq!(frame.lines, vec![String::new()]);
    }

    #[test]
    fn compose_multiline_selection() {
        let buf = RopeBuffer::from_text("hello\nworld\n");
        // select from 'l' in hello (byte idx 2) to 'r' in world (byte idx 8)
        let frame = compose(&buf, 0, 2, 80, 0, &[], Some(2..8));
        assert!(frame.spans.contains(&StyleSpan {
            row: 0,
            start_col: 2,
            end_col: 5,
            class: "sel".into(),
        }));
        assert!(frame.spans.contains(&StyleSpan {
            row: 1,
            start_col: 0,
            end_col: 2,
            class: "sel".into(),
        }));
    }
}
