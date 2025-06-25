//! Terminal setup and event loop utilities.
#![allow(dead_code)]

use std::io::{self, Stdout};
use std::time::Duration;

use crossterm::ExecutableCommand;
use crossterm::event::{self, Event};
use crossterm::terminal::{
    EnterAlternateScreen, LeaveAlternateScreen, disable_raw_mode, enable_raw_mode,
};
use ratatui::Terminal;
use ratatui::backend::{Backend, CrosstermBackend};
use ratatui::prelude::Rect;

use crate::error::{GhostwriterError, Result};

/// Basic terminal management structure.
pub struct TerminalUI<B: Backend> {
    terminal: Terminal<B>,
    cleaned: bool,
}

impl TerminalUI<CrosstermBackend<Stdout>> {
    /// Initialize a terminal using the Crossterm backend.
    pub fn new() -> Result<Self> {
        enable_raw_mode().map_err(GhostwriterError::from)?;
        let mut stdout = io::stdout();
        stdout
            .execute(EnterAlternateScreen)
            .map_err(GhostwriterError::from)?;
        let backend = CrosstermBackend::new(stdout);
        let terminal = Terminal::new(backend).map_err(GhostwriterError::from)?;
        Ok(Self {
            terminal,
            cleaned: false,
        })
    }
}

impl<B: Backend> TerminalUI<B> {
    /// Create a terminal from a custom backend. Used mainly for testing.
    pub fn with_backend(backend: B) -> Result<Self> {
        let terminal = Terminal::new(backend).map_err(GhostwriterError::from)?;
        Ok(Self {
            terminal,
            cleaned: false,
        })
    }

    /// Access the inner terminal.
    pub fn terminal(&mut self) -> &mut Terminal<B> {
        &mut self.terminal
    }

    /// Handle a single terminal event.
    pub fn handle_event(&mut self, event: Event) -> Result<()> {
        if let Event::Resize(w, h) = event {
            let area = Rect::new(0, 0, w, h);
            self.terminal.resize(area).map_err(GhostwriterError::from)?;
        }
        Ok(())
    }

    /// Simple event loop that processes events until the callback returns `false`.
    pub fn run<F>(&mut self, mut f: F) -> Result<()>
    where
        F: FnMut(Event) -> bool,
    {
        loop {
            if event::poll(Duration::from_millis(50)).map_err(GhostwriterError::from)? {
                let evt = event::read().map_err(GhostwriterError::from)?;
                self.handle_event(evt.clone())?;
                if !f(evt) {
                    break;
                }
            }
        }
        Ok(())
    }

    /// Restore terminal state. Safe to call multiple times.
    pub fn cleanup(&mut self) -> Result<()> {
        if self.cleaned {
            return Ok(());
        }
        disable_raw_mode().ok();
        let mut stdout = io::stdout();
        let _ = stdout.execute(LeaveAlternateScreen);
        self.cleaned = true;
        Ok(())
    }
}

impl<B: Backend> Drop for TerminalUI<B> {
    fn drop(&mut self) {
        let _ = self.cleanup();
    }
}
