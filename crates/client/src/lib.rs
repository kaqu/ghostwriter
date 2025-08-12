pub mod keymap;
pub mod tui;

/// Client entry point.
pub fn run() -> &'static str {
    "client"
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn run_returns_client() {
        assert_eq!(run(), "client");
    }
}
