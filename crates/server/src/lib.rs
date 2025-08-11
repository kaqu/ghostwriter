pub mod session;

/// Server entry point.
pub fn run() -> &'static str {
    "server"
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn run_returns_server() {
        assert_eq!(run(), "server");
    }
}
