use std::{
    io,
    ops::Range,
    path::{Path, PathBuf},
    sync::{Arc, Mutex},
};

use ghostwriter_core::{Debouncer, RopeBuffer, ViewportParams, compose_hex, compose_viewport};
use ghostwriter_proto::Frame;
use tokio::sync::mpsc;

/// Commands that can be sent to the session actor.
pub enum SessionCmd {
    /// Insert `text` at the current cursor position.
    Insert { text: String },
    /// Request the current frame without modifying state.
    RequestFrame,
    /// Save the current buffer to disk immediately.
    Save,
}

/// Handle for interacting with a running session.
pub struct SessionHandle {
    pub cmd: mpsc::Sender<SessionCmd>,
    pub frames: mpsc::Receiver<Frame>,
}

#[allow(dead_code)]
struct Session {
    buffer: Arc<Mutex<RopeBuffer>>,
    hex_bytes: Option<Vec<u8>>,
    path: PathBuf,
    doc_v: u64,
    selection: Range<usize>,
    debounce: Debouncer,
    cols: u16,
    rows: u16,
    first_line: usize,
    hscroll: u16,
    status: String,
}

#[allow(dead_code)]
impl Session {
    /// Open a file from `path` and spawn a session actor with the provided viewport size.
    pub fn open<P: AsRef<Path>>(path: P, cols: u16, rows: u16) -> io::Result<SessionHandle> {
        let path = path.as_ref().to_path_buf();
        let buffer = match RopeBuffer::open(&path) {
            Ok(b) => b,
            Err(e) if e.kind() == io::ErrorKind::NotFound => RopeBuffer::from_text(""),
            Err(e) => return Err(e),
        };
        let hex_bytes = if buffer.has_invalid() {
            std::fs::read(&path).ok()
        } else {
            None
        };
        Ok(Self::spawn_inner(buffer, hex_bytes, path, cols, rows))
    }

    /// Spawn a session actor with the provided buffer and viewport size.
    pub fn spawn(buffer: RopeBuffer, path: PathBuf, cols: u16, rows: u16) -> SessionHandle {
        Self::spawn_inner(buffer, None, path, cols, rows)
    }

    fn spawn_inner(
        buffer: RopeBuffer,
        hex_bytes: Option<Vec<u8>>,
        path: PathBuf,
        cols: u16,
        rows: u16,
    ) -> SessionHandle {
        let (cmd_tx, cmd_rx) = mpsc::channel(8);
        let (frame_tx, frame_rx) = mpsc::channel(8);
        let session = Session {
            buffer: Arc::new(Mutex::new(buffer)),
            hex_bytes,
            path,
            doc_v: 0,
            selection: 0..0,
            debounce: Debouncer::default(),
            cols,
            rows,
            first_line: 0,
            hscroll: 0,
            status: "server".into(),
        };
        tokio::spawn(async move {
            session.run(cmd_rx, frame_tx).await;
        });
        SessionHandle {
            cmd: cmd_tx,
            frames: frame_rx,
        }
    }

    async fn run(mut self, mut rx: mpsc::Receiver<SessionCmd>, tx: mpsc::Sender<Frame>) {
        while let Some(cmd) = rx.recv().await {
            match cmd {
                SessionCmd::Insert { text } => {
                    if self.hex_bytes.is_none() {
                        let pos = self.selection.end;
                        {
                            let mut buf = self.buffer.lock().unwrap();
                            buf.insert(pos, &text);
                        }
                        let new_pos = pos + text.len();
                        self.selection = new_pos..new_pos;
                        self.doc_v += 1;
                        let buffer = Arc::clone(&self.buffer);
                        let path = self.path.clone();
                        self.debounce.call(move || {
                            if let Ok(buf) = buffer.lock() {
                                let _ = buf.save_to(&path);
                            }
                        });
                        self.emit_frame(&tx).await;
                    }
                }
                SessionCmd::RequestFrame => {
                    self.emit_frame(&tx).await;
                }
                SessionCmd::Save => {
                    if self.hex_bytes.is_none()
                        && let Ok(buf) = self.buffer.lock()
                    {
                        let _ = buf.save_to(&self.path);
                    }
                }
            }
        }

        if self.hex_bytes.is_none()
            && let Ok(buf) = self.buffer.lock()
        {
            let _ = buf.save_to(&self.path);
        }
    }

    async fn emit_frame(&self, tx: &mpsc::Sender<Frame>) {
        let selections = vec![self.selection.clone()];
        let cursors = vec![self.selection.end];
        let params = ViewportParams {
            selections: &selections,
            cursors: &cursors,
            doc_v: self.doc_v,
            status_left: &self.status,
            status_right: "",
        };
        let frame = if let Some(bytes) = &self.hex_bytes {
            compose_hex(
                bytes,
                self.first_line,
                self.cols,
                self.rows,
                self.doc_v,
                &self.status,
                "",
            )
        } else {
            let buf = self.buffer.lock().unwrap();
            compose_viewport(
                &buf,
                self.first_line,
                self.cols,
                self.rows,
                self.hscroll,
                params,
            )
        };
        let _ = tx.send(frame).await;
    }
}

/// Open a file from `path` and spawn a session actor.
pub fn open<P: AsRef<Path>>(path: P, cols: u16, rows: u16) -> io::Result<SessionHandle> {
    Session::open(path, cols, rows)
}

/// Spawn a session with the provided `buffer` for testing purposes.
pub fn spawn(buffer: RopeBuffer, path: PathBuf, cols: u16, rows: u16) -> SessionHandle {
    Session::spawn(buffer, path, cols, rows)
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::Write;
    use tempfile::NamedTempFile;

    #[tokio::test]
    async fn insert_emits_frame() {
        let file = NamedTempFile::new().unwrap();
        let mut handle =
            Session::spawn(RopeBuffer::from_text(""), file.path().to_path_buf(), 80, 24);
        handle
            .cmd
            .send(SessionCmd::Insert { text: "hi".into() })
            .await
            .unwrap();
        let frame = handle.frames.recv().await.unwrap();
        assert_eq!(frame.doc_v, 1);
        assert_eq!(frame.lines[0].text, "hi");
        assert_eq!(frame.cursors[0].line, 0);
        assert_eq!(frame.cursors[0].col, 2);

        handle.cmd.send(SessionCmd::RequestFrame).await.unwrap();
        let frame2 = handle.frames.recv().await.unwrap();
        assert_eq!(frame2.doc_v, 1);
        assert_eq!(frame2.lines[0].text, "hi");
    }

    #[tokio::test]
    async fn open_and_save_roundtrip() {
        let mut file = NamedTempFile::new().unwrap();
        write!(file, "hi").unwrap();
        let path = file.path().to_path_buf();
        let mut handle = open(&path, 80, 24).unwrap();
        handle
            .cmd
            .send(SessionCmd::Insert {
                text: " there".into(),
            })
            .await
            .unwrap();
        let _ = handle.frames.recv().await.unwrap();
        handle.cmd.send(SessionCmd::Save).await.unwrap();
        handle.cmd.send(SessionCmd::RequestFrame).await.unwrap();
        let _ = handle.frames.recv().await.unwrap();
        let contents = std::fs::read_to_string(&path).unwrap();
        assert_eq!(contents, " therehi");
    }

    #[tokio::test]
    async fn banner_and_save_on_exit() {
        use tokio::time::{Duration, sleep};

        let file = NamedTempFile::new().unwrap();
        let path = file.path().to_path_buf();
        let SessionHandle { cmd, mut frames } = open(&path, 80, 24).unwrap();

        cmd.send(SessionCmd::RequestFrame).await.unwrap();
        let frame = frames.recv().await.unwrap();
        assert_eq!(frame.status_left, "server");

        cmd.send(SessionCmd::Insert { text: "hi".into() })
            .await
            .unwrap();
        drop(cmd); // close channel to end session

        sleep(Duration::from_millis(20)).await;

        let contents = std::fs::read_to_string(&path).unwrap();
        assert_eq!(contents, "hi");
    }

    #[tokio::test]
    async fn opens_invalid_file_in_hex_mode() {
        let mut file = NamedTempFile::new().unwrap();
        file.write_all(&[0xFF, 0x00, b'A']).unwrap();
        let path = file.path().to_path_buf();
        let mut handle = open(&path, 80, 24).unwrap();
        handle.cmd.send(SessionCmd::RequestFrame).await.unwrap();
        let frame = handle.frames.recv().await.unwrap();
        assert_eq!(frame.kind, "hex");
        assert_eq!(
            frame.lines[0].text,
            "FF 00 41                                         |..A",
        );
    }
}
