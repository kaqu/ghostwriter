use std::ops::Range;

use ghostwriter_core::{Debouncer, RopeBuffer, ViewportParams, compose_viewport};
use ghostwriter_proto::Frame;
use tokio::sync::mpsc;

/// Commands that can be sent to the session actor.
pub enum SessionCmd {
    /// Insert `text` at the current cursor position.
    Insert { text: String },
    /// Request the current frame without modifying state.
    RequestFrame,
}

/// Handle for interacting with a running session.
pub struct SessionHandle {
    pub cmd: mpsc::Sender<SessionCmd>,
    pub frames: mpsc::Receiver<Frame>,
}

#[allow(dead_code)]
struct Session {
    buffer: RopeBuffer,
    doc_v: u64,
    selection: Range<usize>,
    debounce: Debouncer,
    cols: u16,
    rows: u16,
    first_line: usize,
    hscroll: u16,
}

#[allow(dead_code)]
impl Session {
    /// Spawn a session actor with the provided buffer and viewport size.
    pub fn spawn(buffer: RopeBuffer, cols: u16, rows: u16) -> SessionHandle {
        let (cmd_tx, cmd_rx) = mpsc::channel(8);
        let (frame_tx, frame_rx) = mpsc::channel(8);
        let session = Session {
            buffer,
            doc_v: 0,
            selection: 0..0,
            debounce: Debouncer::default(),
            cols,
            rows,
            first_line: 0,
            hscroll: 0,
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
                    let pos = self.selection.end;
                    self.buffer.insert(pos, &text);
                    let new_pos = pos + text.len();
                    self.selection = new_pos..new_pos;
                    self.doc_v += 1;
                    self.debounce.call(|| {});
                    self.emit_frame(&tx).await;
                }
                SessionCmd::RequestFrame => {
                    self.emit_frame(&tx).await;
                }
            }
        }
    }

    async fn emit_frame(&self, tx: &mpsc::Sender<Frame>) {
        let selections = vec![self.selection.clone()];
        let cursors = vec![self.selection.end];
        let params = ViewportParams {
            selections: &selections,
            cursors: &cursors,
            doc_v: self.doc_v,
            status_left: "",
            status_right: "",
        };
        let frame = compose_viewport(
            &self.buffer,
            self.first_line,
            self.cols,
            self.rows,
            self.hscroll,
            params,
        );
        let _ = tx.send(frame).await;
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[tokio::test]
    async fn insert_emits_frame() {
        let mut handle = Session::spawn(RopeBuffer::from_text(""), 80, 24);
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
}
