//! Text search module

use regex::Regex;
use std::ops::Range;

use super::rope::Rope;

#[allow(dead_code)]
#[derive(Debug, Default)]
pub struct Search {
    query: String,
    pub case_sensitive: bool,
    pub regex: bool,
    matches: Vec<Range<usize>>, // character indices
    current: usize,
}

#[allow(dead_code)]
impl Search {
    pub fn new() -> Self {
        Self {
            query: String::new(),
            case_sensitive: false,
            regex: false,
            matches: Vec::new(),
            current: 0,
        }
    }

    pub fn update(
        &mut self,
        rope: &Rope,
        query: &str,
        regex: bool,
        case_sensitive: bool,
    ) -> Result<(), regex::Error> {
        self.query = query.to_string();
        self.regex = regex;
        self.case_sensitive = case_sensitive;
        self.current = 0;
        self.matches.clear();
        if query.is_empty() {
            return Ok(());
        }
        let text = rope.as_string();
        if regex {
            let pat = if case_sensitive {
                Regex::new(query)?
            } else {
                Regex::new(&format!("(?i){}", query))?
            };
            for m in pat.find_iter(&text) {
                let start = byte_to_char_index(&text, m.start());
                let end = byte_to_char_index(&text, m.end());
                self.matches.push(start..end);
            }
        } else {
            let haystack = if case_sensitive {
                text.clone()
            } else {
                text.to_lowercase()
            };
            let needle = if case_sensitive {
                query.to_string()
            } else {
                query.to_lowercase()
            };
            let mut offset = 0;
            while let Some(pos) = haystack[offset..].find(&needle) {
                let start_b = offset + pos;
                let end_b = start_b + needle.len();
                let start = byte_to_char_index(&text, start_b);
                let end = byte_to_char_index(&text, end_b);
                self.matches.push(start..end);
                offset = end_b;
            }
        }
        Ok(())
    }

    pub fn next(&mut self) -> Option<Range<usize>> {
        if self.matches.is_empty() {
            return None;
        }
        let r = self.matches[self.current].clone();
        self.current = (self.current + 1) % self.matches.len();
        Some(r)
    }

    pub fn prev(&mut self) -> Option<Range<usize>> {
        if self.matches.is_empty() {
            return None;
        }
        if self.current == 0 {
            self.current = self.matches.len() - 1;
        } else {
            self.current -= 1;
        }
        Some(self.matches[self.current].clone())
    }

    pub fn matches(&self) -> &[Range<usize>] {
        &self.matches
    }
}

fn byte_to_char_index(s: &str, byte_idx: usize) -> usize {
    s[..byte_idx].chars().count()
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::editor::rope::Rope;

    #[test]
    fn test_incremental_search() {
        let rope = Rope::from_str("abc abcd abcde");
        let mut search = Search::new();
        search.update(&rope, "a", false, true).unwrap();
        assert_eq!(search.matches.len(), 3);
        search.update(&rope, "ab", false, true).unwrap();
        assert_eq!(search.matches.len(), 3);
        search.update(&rope, "abc", false, true).unwrap();
        assert_eq!(search.matches.len(), 3);
    }

    #[test]
    fn test_regex_pattern_support() {
        let rope = Rope::from_str("foo1 foo2 foo3");
        let mut search = Search::new();
        search.update(&rope, r"foo\d", true, true).unwrap();
        assert_eq!(search.matches.len(), 3);
    }

    #[test]
    fn test_search_navigation() {
        let rope = Rope::from_str("one two one two");
        let mut search = Search::new();
        search.update(&rope, "two", false, true).unwrap();
        let first = search.next().unwrap();
        assert_eq!(rope.slice(first.clone()), "two");
        let second = search.next().unwrap();
        assert_eq!(rope.slice(second.clone()), "two");
        let prev = search.prev().unwrap();
        assert_eq!(prev, second);
    }

    #[test]
    fn test_case_sensitivity_toggle() {
        let rope = Rope::from_str("Foo foo FOO");
        let mut search = Search::new();
        search.update(&rope, "foo", false, false).unwrap();
        assert_eq!(search.matches.len(), 3);
        search.update(&rope, "foo", false, true).unwrap();
        assert_eq!(search.matches.len(), 1);
    }
}
