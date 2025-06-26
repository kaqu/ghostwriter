#![allow(dead_code)]

use ratatui::{buffer::Buffer, layout::Rect, widgets::Widget};

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum LockStatus {
    None,
    Locked,
    ReadOnly,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ConnectionStatus {
    Online,
    Connecting,
    Offline,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Mode {
    Local,
    Remote,
}

pub struct StatusBar<'a> {
    pub file_path: &'a str,
    pub cursor: (usize, usize),
    pub lock_status: LockStatus,
    pub connection_status: ConnectionStatus,
    pub dirty: bool,
    pub mode: Mode,
}

impl<'a> Widget for StatusBar<'a> {
    fn render(self, area: Rect, buf: &mut Buffer) {
        let lock_icon = match self.lock_status {
            LockStatus::Locked => "\u{1F512}",   // 🔒
            LockStatus::ReadOnly => "\u{1F441}", // 👁
            LockStatus::None => "",
        };
        let conn_icon = match self.connection_status {
            ConnectionStatus::Online => "\u{1F7E2}",     // 🟢
            ConnectionStatus::Connecting => "\u{1F7E1}", // 🟡
            ConnectionStatus::Offline => "\u{1F534}",    // 🔴
        };
        let dirty = if self.dirty { "*" } else { "" };
        let mode_text = match self.mode {
            Mode::Local => "local",
            Mode::Remote => "remote",
        };
        let text = format!(
            "{}{} | {}:{} {} {} {}",
            self.file_path, dirty, self.cursor.0, self.cursor.1, lock_icon, conn_icon, mode_text
        );
        let mut truncated = text.clone();
        if truncated.chars().count() > area.width as usize {
            truncated = truncated.chars().take(area.width as usize).collect();
        }
        buf.set_string(area.x, area.y, truncated, ratatui::style::Style::default());
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use ratatui::{Terminal, backend::TestBackend};

    #[test]
    fn test_status_bar_file_info() {
        let backend = TestBackend::new(80, 1);
        let mut terminal = Terminal::new(backend).unwrap();
        terminal
            .draw(|f| {
                let area = Rect::new(0, 0, 80, 1);
                f.render_widget(
                    StatusBar {
                        file_path: "/tmp/file.txt",
                        cursor: (3, 7),
                        lock_status: LockStatus::None,
                        connection_status: ConnectionStatus::Online,
                        dirty: true,
                        mode: Mode::Local,
                    },
                    area,
                );
            })
            .unwrap();
        let buffer = terminal.backend().buffer();
        let line: String = (0..80).map(|x| buffer[(x, 0)].symbol()).collect();
        assert!(line.contains("/tmp/file.txt*"));
        assert!(line.contains("3:7"));
    }

    #[test]
    fn test_lock_status_indicators() {
        let backend = TestBackend::new(30, 1);
        let mut terminal = Terminal::new(backend).unwrap();
        terminal
            .draw(|f| {
                let area = Rect::new(0, 0, 30, 1);
                f.render_widget(
                    StatusBar {
                        file_path: "file",
                        cursor: (0, 0),
                        lock_status: LockStatus::Locked,
                        connection_status: ConnectionStatus::Online,
                        dirty: false,
                        mode: Mode::Local,
                    },
                    area,
                );
            })
            .unwrap();
        let buffer = terminal.backend().buffer();
        let line: String = (0..30).map(|x| buffer[(x, 0)].symbol()).collect();
        assert!(line.contains("\u{1F512}"));

        let mut terminal = Terminal::new(TestBackend::new(30, 1)).unwrap();
        terminal
            .draw(|f| {
                let area = Rect::new(0, 0, 30, 1);
                f.render_widget(
                    StatusBar {
                        file_path: "file",
                        cursor: (0, 0),
                        lock_status: LockStatus::ReadOnly,
                        connection_status: ConnectionStatus::Online,
                        dirty: false,
                        mode: Mode::Local,
                    },
                    area,
                );
            })
            .unwrap();
        let buffer = terminal.backend().buffer();
        let line: String = (0..30).map(|x| buffer[(x, 0)].symbol()).collect();
        assert!(line.contains("\u{1F441}"));
    }

    #[test]
    fn test_connection_status_display() {
        let backend = TestBackend::new(30, 1);
        let mut terminal = Terminal::new(backend).unwrap();
        terminal
            .draw(|f| {
                let area = Rect::new(0, 0, 30, 1);
                f.render_widget(
                    StatusBar {
                        file_path: "file",
                        cursor: (0, 0),
                        lock_status: LockStatus::None,
                        connection_status: ConnectionStatus::Connecting,
                        dirty: false,
                        mode: Mode::Remote,
                    },
                    area,
                );
            })
            .unwrap();
        let buffer = terminal.backend().buffer();
        let line: String = (0..30).map(|x| buffer[(x, 0)].symbol()).collect();
        assert!(line.contains("\u{1F7E1}"));
        assert!(line.contains("remote"));
    }
}
