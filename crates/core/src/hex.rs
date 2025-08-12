use ghostwriter_proto::{Frame, Line};

/// Compose a hex view frame for the given bytes.
/// Each row displays 16 bytes in hexadecimal followed by an ASCII gutter.
pub fn compose_hex(
    bytes: &[u8],
    first_row: usize,
    cols: u16,
    rows: u16,
    doc_v: u64,
    status_left: &str,
    status_right: &str,
) -> Frame {
    let mut lines = Vec::new();
    let total_rows = bytes.len().div_ceil(16);
    for row in first_row..std::cmp::min(first_row + rows as usize, total_rows) {
        let start = row * 16;
        let end = std::cmp::min(start + 16, bytes.len());

        let mut hex_part = String::new();
        for i in 0..16 {
            if start + i < end {
                hex_part.push_str(&format!("{:02X}", bytes[start + i]));
            } else {
                hex_part.push_str("  ");
            }
            if i != 15 {
                hex_part.push(' ');
            }
            if i == 7 {
                hex_part.push(' ');
            }
        }
        if hex_part.len() < 48 {
            hex_part.push_str(&" ".repeat(48 - hex_part.len()));
        }

        let mut ascii_part = String::new();
        for &b in &bytes[start..end] {
            if (0x20..=0x7E).contains(&b) {
                ascii_part.push(b as char);
            } else {
                ascii_part.push('.');
            }
        }

        let line_text = format!("{hex_part} |{ascii_part}");
        lines.push(Line {
            text: line_text,
            spans: Vec::new(),
        });
    }

    Frame {
        id: "hex".into(),
        kind: "hex".into(),
        doc_v,
        first_line: first_row as u64,
        cols,
        rows,
        lines,
        cursors: Vec::new(),
        status_left: status_left.into(),
        status_right: status_right.into(),
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn renders_single_line() {
        let bytes = b"hello\x00world\xff";
        let frame = compose_hex(bytes, 0, 80, 1, 1, "", "");
        assert_eq!(
            frame.lines[0].text,
            "68 65 6C 6C 6F 00 77 6F  72 6C 64 FF             |hello.world."
        );
    }
}
