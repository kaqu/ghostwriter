use std::ops::Range;

use ghostwriter_proto::{Cursor, Frame, Line, StyleSpan};

use crate::buffer::RopeBuffer;

pub fn compose(
    buf: &RopeBuffer,
    first_line: usize,
    cols: u16,
    rows: u16,
    hscroll: u16,
    selections: &[Range<usize>],
    cursors: &[usize],
    doc_v: u64,
    status_left: &str,
    status_right: &str,
) -> Frame {
    let mut lines_out = Vec::new();
    let raw_lines = buf.slice_lines(first_line, rows as usize);
    for (idx, mut line) in raw_lines.into_iter().enumerate() {
        let line_idx = first_line + idx;
        let line_start = buf.line_to_byte(line_idx);
        let line_end = line_start + line.len();
        let mut spans: Vec<StyleSpan> = Vec::new();

        // Selection spans
        for sel in selections {
            let start = sel.start.max(line_start);
            let end = sel.end.min(line_end);
            if start < end {
                let mut sc = (start - line_start) as i64;
                let mut ec = (end - line_start) as i64;
                let hs = hscroll as i64;
                if ec > hs && sc < hs + cols as i64 {
                    sc = sc.max(hs) - hs;
                    ec = ec.min(hs + cols as i64) - hs;
                    spans.push(StyleSpan {
                        start_col: sc as u16,
                        end_col: ec as u16,
                        class_name: "sel".into(),
                    });
                }
            }
        }

        // Trailing whitespace span
        let trimmed_len = line.trim_end_matches([' ', '\t']).len();
        if trimmed_len < line.len() {
            let mut start = trimmed_len as i64;
            let mut end = line.len() as i64;
            let hs = hscroll as i64;
            if end > hs && start < hs + cols as i64 {
                start = start.max(hs) - hs;
                end = end.min(hs + cols as i64) - hs;
                spans.push(StyleSpan {
                    start_col: start as u16,
                    end_col: end as u16,
                    class_name: "ws".into(),
                });
            }
        }

        // Apply horizontal scroll to text
        let start = hscroll as usize;
        if start < line.len() {
            let end = std::cmp::min(line.len(), start + cols as usize);
            line = line[start..end].to_string();
        } else {
            line.clear();
        }

        lines_out.push(Line { text: line, spans });
    }

    let mut cursor_out = Vec::new();
    for &c in cursors {
        let (line, col) = buf.byte_to_line_col(c);
        cursor_out.push(Cursor {
            line: line as u64,
            col: col as u16,
        });
    }

    Frame {
        id: "editor".into(),
        kind: "editor".into(),
        doc_v,
        first_line: first_line as u64,
        cols,
        rows,
        lines: lines_out,
        cursors: cursor_out,
        status_left: status_left.into(),
        status_right: status_right.into(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn composes_selection_and_whitespace() {
        let buf = RopeBuffer::from_text("hello \nworld\t\n");
        let frame = compose(&buf, 0, 10, 2, 0, &[3..9], &[8], 1, "L", "R");
        assert_eq!(frame.lines.len(), 2);
        assert_eq!(frame.lines[0].text, "hello ");
        assert_eq!(frame.lines[1].text, "world\t");
        assert_eq!(
            frame.lines[0].spans,
            vec![
                StyleSpan {
                    start_col: 3,
                    end_col: 6,
                    class_name: "sel".into(),
                },
                StyleSpan {
                    start_col: 5,
                    end_col: 6,
                    class_name: "ws".into(),
                },
            ]
        );
        assert_eq!(
            frame.lines[1].spans,
            vec![
                StyleSpan {
                    start_col: 0,
                    end_col: 2,
                    class_name: "sel".into(),
                },
                StyleSpan {
                    start_col: 5,
                    end_col: 6,
                    class_name: "ws".into(),
                },
            ]
        );
        assert_eq!(frame.cursors, vec![Cursor { line: 1, col: 1 }]);
        assert_eq!(frame.status_left, "L");
        assert_eq!(frame.status_right, "R");
    }
}
