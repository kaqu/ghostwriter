use std::io;
use std::path::PathBuf;

use ghostwriter_proto::Frame;
use ghostwriter_server::session::{self, SessionCmd, SessionHandle};

/// Local in-process client connected to a session via channels.
pub struct LocalClient {
    handle: SessionHandle,
}

impl LocalClient {
    /// Open a file at `path` and spawn a session with the given viewport size.
    pub fn open(path: PathBuf, cols: u16, rows: u16) -> io::Result<Self> {
        let handle = session::open(path, cols, rows)?;
        Ok(Self { handle })
    }

    /// Send an insert command to the session.
    pub async fn insert(&mut self, text: &str) {
        let _ = self
            .handle
            .cmd
            .send(SessionCmd::Insert { text: text.into() })
            .await;
    }

    /// Request the current frame and wait for it.
    pub async fn request_frame(&mut self) -> Frame {
        let _ = self.handle.cmd.send(SessionCmd::RequestFrame).await;
        self.next_frame().await
    }

    /// Receive the next frame emitted by the session.
    pub async fn next_frame(&mut self) -> Frame {
        self.handle.frames.recv().await.unwrap()
    }

    /// Trigger an immediate save of the buffer to disk.
    pub async fn save(&mut self) {
        let _ = self.handle.cmd.send(SessionCmd::Save).await;
    }
}
