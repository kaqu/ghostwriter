use std::path::Path;

use crate::error::{GhostwriterError, Result};

/// Sanitize a path input to prevent injection and traversal attacks.
pub fn sanitize_path(path: &Path) -> Result<()> {
    let s = path.to_string_lossy();
    if s.is_empty() {
        return Err(GhostwriterError::InvalidArgument("empty path".into()));
    }
    if s.contains('\0') || s.contains('\n') || s.contains('\r') || s.contains('\t') {
        return Err(GhostwriterError::InvalidArgument(
            "invalid characters".into(),
        ));
    }
    for part in s.split(['/', '\\'].as_ref()) {
        if part == ".." {
            return Err(GhostwriterError::InvalidArgument(
                "path traversal detected".into(),
            ));
        }
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_reject_traversal() {
        assert!(sanitize_path(Path::new("../evil")).is_err());
    }

    #[test]
    fn test_reject_control_chars() {
        assert!(sanitize_path(Path::new("bad\x00name")).is_err());
    }

    #[test]
    fn test_accept_normal_path() {
        assert!(sanitize_path(Path::new("normal.txt")).is_ok());
    }
}
