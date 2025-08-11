/// Core utilities for Ghostwriter.
pub fn add(a: i32, b: i32) -> i32 {
    a + b
}

pub mod buffer;
pub mod transport;

pub use buffer::RopeBuffer;
pub use transport::Transport;

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn adds_numbers() {
        assert_eq!(add(2, 2), 4);
    }
}
