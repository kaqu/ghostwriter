use std::path::Path;

/// Log an audit event for opening a file.
pub fn log_open<P: AsRef<Path>>(path: P) {
    tracing::info!(target: "audit", op = "open", path = %path.as_ref().display());
}

/// Log an audit event for saving a file.
pub fn log_save<P: AsRef<Path>>(path: P, result: &str) {
    tracing::info!(target: "audit", op = "save", path = %path.as_ref().display(), result);
}

/// Log an audit event for renaming a file.
pub fn log_rename<P: AsRef<Path>>(from: P, to: P, result: &str) {
    tracing::info!(target: "audit", op = "rename", from = %from.as_ref().display(), to = %to.as_ref().display(), result);
}

/// Log an audit event for deleting a file.
pub fn log_delete<P: AsRef<Path>>(path: P, result: &str) {
    tracing::info!(target: "audit", op = "delete", path = %path.as_ref().display(), result);
}

/// Log an authentication result.
pub fn log_auth(result: &str) {
    tracing::info!(target: "audit", op = "auth", result);
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::io::{self, Write};
    use std::sync::{Arc, Mutex};
    use tracing_subscriber::fmt::MakeWriter;

    #[derive(Clone)]
    struct Buf(Arc<Mutex<Vec<u8>>>);

    impl<'a> MakeWriter<'a> for Buf {
        type Writer = BufGuard;
        fn make_writer(&'a self) -> Self::Writer {
            BufGuard(self.0.clone())
        }
    }

    struct BufGuard(Arc<Mutex<Vec<u8>>>);

    impl Write for BufGuard {
        fn write(&mut self, buf: &[u8]) -> io::Result<usize> {
            self.0.lock().unwrap().write(buf)
        }
        fn flush(&mut self) -> io::Result<()> {
            self.0.lock().unwrap().flush()
        }
    }

    #[test]
    fn logs_open_and_save() {
        let buf = Buf(Arc::new(Mutex::new(Vec::new())));
        let subscriber = tracing_subscriber::fmt()
            .json()
            .with_writer(buf.clone())
            .finish();
        let _guard = tracing::subscriber::set_default(subscriber);

        log_open("/tmp/test");
        log_save("/tmp/test", "ok");

        let logs = String::from_utf8(buf.0.lock().unwrap().clone()).unwrap();
        assert!(logs.contains("\"op\":\"open\""));
        assert!(logs.contains("\"op\":\"save\""));

        drop(_guard);
    }
}
