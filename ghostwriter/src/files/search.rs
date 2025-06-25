//! Cross-file search utilities

use serde::{Deserialize, Serialize};
use std::fs;
use std::path::Path;

use regex::Regex;
use twoway::find_bytes;
use walkdir::WalkDir;

use crate::error::{GhostwriterError, Result};
use crate::files::file_manager::FileManager;

#[derive(Debug, Clone, PartialEq, Eq, Serialize, Deserialize)]
pub struct SearchResult {
    pub path: String,
    pub line: usize,
    pub context: String,
}

pub fn search_dir(
    root: &Path,
    pattern: &str,
    regex: bool,
    case_sensitive: bool,
    limit: usize,
) -> Result<Vec<SearchResult>> {
    let mut results = Vec::new();
    let walker = WalkDir::new(root);
    let regex_obj = if regex {
        let pat = if case_sensitive {
            pattern.to_string()
        } else {
            format!("(?i){}", pattern)
        };
        Some(Regex::new(&pat).map_err(|e| GhostwriterError::InvalidArgument(e.to_string()))?)
    } else {
        None
    };
    let pattern_lower = if !case_sensitive && !regex {
        Some(pattern.to_lowercase())
    } else {
        None
    };
    for entry in walker.into_iter().filter_map(|e| e.ok()) {
        if !entry.file_type().is_file() {
            continue;
        }
        if results.len() >= limit {
            break;
        }
        let data = fs::read(entry.path())?;
        if FileManager::is_binary(&data) {
            continue;
        }
        let text = String::from_utf8_lossy(&data);
        for (idx, line) in text.lines().enumerate() {
            if results.len() >= limit {
                break;
            }
            let is_match = if let Some(re) = &regex_obj {
                re.is_match(line)
            } else if case_sensitive {
                find_bytes(line.as_bytes(), pattern.as_bytes()).is_some()
            } else {
                let hay = line.to_lowercase();
                find_bytes(hay.as_bytes(), pattern_lower.as_ref().unwrap().as_bytes()).is_some()
            };
            if is_match {
                let rel_path = entry
                    .path()
                    .strip_prefix(root)
                    .unwrap()
                    .to_string_lossy()
                    .into();
                results.push(SearchResult {
                    path: rel_path,
                    line: idx + 1,
                    context: line.to_string(),
                });
            }
        }
    }
    Ok(results)
}

#[cfg(test)]
mod tests {
    use crate::files::workspace::WorkspaceManager;
    use tempfile::tempdir;

    #[test]
    fn test_cross_file_search() {
        let dir = tempdir().unwrap();
        std::fs::write(dir.path().join("a.txt"), "hello world\n").unwrap();
        std::fs::write(dir.path().join("b.txt"), "world here\n").unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let res = ws.search("world", false, true, 1000).unwrap();
        assert_eq!(res.len(), 2);
    }

    #[test]
    fn test_search_result_format() {
        let dir = tempdir().unwrap();
        std::fs::write(dir.path().join("sub.txt"), "line1\nmatch line\n").unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let res = ws.search("match", false, true, 1000).unwrap();
        assert_eq!(res[0].path, "sub.txt");
        assert_eq!(res[0].line, 2);
        assert_eq!(res[0].context, "match line");
    }

    #[test]
    fn test_regex_cross_file_search() {
        let dir = tempdir().unwrap();
        std::fs::write(dir.path().join("c.txt"), "foo1\nfoo2\n").unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let res = ws.search(r"foo\d", true, true, 1000).unwrap();
        assert_eq!(res.len(), 2);
    }

    #[test]
    fn test_result_limiting() {
        let dir = tempdir().unwrap();
        let mut contents = String::new();
        for _ in 0..10 {
            contents.push_str("match\n");
        }
        std::fs::write(dir.path().join("d.txt"), contents).unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let res = ws.search("match", false, true, 5).unwrap();
        assert_eq!(res.len(), 5);
    }

    #[test]
    fn test_invalid_regex_error() {
        let dir = tempdir().unwrap();
        std::fs::write(dir.path().join("e.txt"), "text").unwrap();
        let ws = WorkspaceManager::new(dir.path().to_path_buf()).unwrap();
        let res = ws.search("(", true, true, 1000);
        assert!(res.is_err(), "invalid regex should return error");
    }
}
