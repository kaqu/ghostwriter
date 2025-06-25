pub mod editor_widget;
pub mod file_picker;
pub mod status_bar;
pub mod terminal;

#[allow(unused_imports)]
pub use file_picker::FilePicker;
#[allow(unused_imports)]
pub use status_bar::{ConnectionStatus, LockStatus, Mode, StatusBar};
#[allow(unused_imports)]
pub use terminal::TerminalUI;

#[cfg(test)]
mod tests {
    use super::terminal::TerminalUI;
    use crossterm::event::Event;
    use ratatui::backend::TestBackend;

    #[test]
    fn test_terminal_initialization() {
        let mut ui = TerminalUI::with_backend(TestBackend::new(10, 10)).unwrap();
        let size = ui.terminal().size().unwrap();
        assert_eq!(size.width, 10);
        assert_eq!(size.height, 10);
    }

    #[test]
    fn test_resize_handling() {
        let mut ui = TerminalUI::with_backend(TestBackend::new(10, 10)).unwrap();
        ui.terminal().backend_mut().resize(20, 15);
        ui.handle_event(Event::Resize(20, 15)).unwrap();
        let size = ui.terminal().size().unwrap();
        assert_eq!(size.width, 20);
        assert_eq!(size.height, 15);
    }

    #[test]
    fn test_graceful_cleanup() {
        let mut ui = TerminalUI::with_backend(TestBackend::new(5, 5)).unwrap();
        assert!(ui.cleanup().is_ok());
        // second cleanup should be a no-op
        assert!(ui.cleanup().is_ok());
    }
}
