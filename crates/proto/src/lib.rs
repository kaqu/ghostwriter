/// Protocol helpers for Ghostwriter.
pub fn version() -> &'static str {
    "0.1.0"
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn version_matches() {
        assert_eq!(version(), "0.1.0");
    }
}
