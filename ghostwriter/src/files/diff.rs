#![allow(dead_code)]
pub enum DiffOp<'a> {
    Equal(&'a str),
    Insert(&'a str),
    Delete(&'a str),
    Replace(&'a str, &'a str),
}

pub fn diff_lines<'a>(old: &'a str, new: &'a str) -> Vec<DiffOp<'a>> {
    let old_lines: Vec<&str> = old.lines().collect();
    let new_lines: Vec<&str> = new.lines().collect();
    let mut i = 0;
    let mut j = 0;
    let mut ops = Vec::new();
    while i < old_lines.len() && j < new_lines.len() {
        if old_lines[i] == new_lines[j] {
            ops.push(DiffOp::Equal(old_lines[i]));
            i += 1;
            j += 1;
        } else if j + 1 < new_lines.len() && old_lines[i] == new_lines[j + 1] {
            ops.push(DiffOp::Insert(new_lines[j]));
            j += 1;
        } else if i + 1 < old_lines.len() && old_lines[i + 1] == new_lines[j] {
            ops.push(DiffOp::Delete(old_lines[i]));
            i += 1;
        } else {
            ops.push(DiffOp::Replace(old_lines[i], new_lines[j]));
            i += 1;
            j += 1;
        }
    }
    while i < old_lines.len() {
        ops.push(DiffOp::Delete(old_lines[i]));
        i += 1;
    }
    while j < new_lines.len() {
        ops.push(DiffOp::Insert(new_lines[j]));
        j += 1;
    }
    ops
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_simple_diff() {
        let old = "a\nb\nc";
        let new = "a\nc\nd";
        let diff = diff_lines(old, new);
        assert!(matches!(diff[1], DiffOp::Delete("b")));
        assert!(matches!(diff[2], DiffOp::Equal("c")));
    }
}
