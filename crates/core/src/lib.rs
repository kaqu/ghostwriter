/// Core utilities for Ghostwriter.
pub fn add(a: i32, b: i32) -> i32 {
    a + b
}

pub mod buffer;
pub mod debounce;
pub mod fs;
pub mod transport;
pub mod undo;
pub mod viewport;
pub mod wal;

pub use buffer::RopeBuffer;
pub use debounce::Debouncer;
pub use fs::atomic_write;
pub use transport::Transport;
pub use undo::UndoStack;
pub use viewport::{ViewportParams, compose as compose_viewport};
pub use wal::{EditOp, EditRecord, Wal};

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn adds_numbers() {
        assert_eq!(add(2, 2), 4);
    }
}
