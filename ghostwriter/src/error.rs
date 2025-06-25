use std::fmt;
use std::io;

/// Result alias using `GhostwriterError`.
pub type Result<T> = std::result::Result<T, GhostwriterError>;

/// Primary error type for Ghostwriter.
#[derive(Debug)]
pub enum GhostwriterError {
    /// File was not found on disk.
    FileNotFound,
    /// Operation failed due to insufficient permissions.
    PermissionDenied,
    /// Any network related error.
    Network(String),
    /// Catch-all for other I/O errors.
    Io(io::Error),
    /// Invalid argument or input.
    InvalidArgument(String),
}

impl fmt::Display for GhostwriterError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            GhostwriterError::FileNotFound => write!(f, "file not found"),
            GhostwriterError::PermissionDenied => write!(f, "permission denied"),
            GhostwriterError::Network(msg) => write!(f, "network error: {msg}"),
            GhostwriterError::Io(err) => write!(f, "io error: {err}"),
            GhostwriterError::InvalidArgument(msg) => write!(f, "invalid argument: {msg}"),
        }
    }
}

impl std::error::Error for GhostwriterError {}

impl From<io::Error> for GhostwriterError {
    fn from(err: io::Error) -> Self {
        use io::ErrorKind::*;
        match err.kind() {
            NotFound => GhostwriterError::FileNotFound,
            PermissionDenied => GhostwriterError::PermissionDenied,
            _ => GhostwriterError::Io(err),
        }
    }
}

impl From<tokio_tungstenite::tungstenite::Error> for GhostwriterError {
    fn from(err: tokio_tungstenite::tungstenite::Error) -> Self {
        GhostwriterError::Network(err.to_string())
    }
}

impl From<notify::Error> for GhostwriterError {
    fn from(err: notify::Error) -> Self {
        GhostwriterError::Io(std::io::Error::other(err))
    }
}

impl From<serde_json::Error> for GhostwriterError {
    fn from(err: serde_json::Error) -> Self {
        GhostwriterError::InvalidArgument(err.to_string())
    }
}

/// Error type carrying additional context string.
#[derive(Debug)]
pub struct ContextualError {
    context: String,
    source: GhostwriterError,
}

#[allow(dead_code)]
impl ContextualError {
    /// Create a new contextual error.
    pub fn new(context: impl Into<String>, source: GhostwriterError) -> Self {
        Self {
            context: context.into(),
            source,
        }
    }

    /// Access the underlying error.
    pub fn source(&self) -> &GhostwriterError {
        &self.source
    }
}

impl fmt::Display for ContextualError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        write!(f, "{}: {}", self.context, self.source)
    }
}

impl std::error::Error for ContextualError {
    fn source(&self) -> Option<&(dyn std::error::Error + 'static)> {
        Some(&self.source)
    }
}

#[allow(dead_code)]
impl GhostwriterError {
    /// Attach a context string to the error.
    pub fn with_context(self, ctx: impl Into<String>) -> ContextualError {
        ContextualError::new(ctx, self)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use std::fs::File;

    #[test]
    fn test_file_not_found_error() {
        let err: GhostwriterError = File::open("/no/such/file").map_err(Into::into).unwrap_err();
        assert!(matches!(err, GhostwriterError::FileNotFound));
        assert_eq!(err.to_string(), "file not found");
    }

    #[test]
    fn test_permission_denied_error() {
        let io_err = io::Error::from(io::ErrorKind::PermissionDenied);
        let err: GhostwriterError = io_err.into();
        assert!(matches!(err, GhostwriterError::PermissionDenied));
    }

    #[test]
    fn test_network_error_conversion() {
        use tokio_tungstenite::tungstenite::error::{Error as WsError, ProtocolError};
        let ws_err = WsError::Protocol(ProtocolError::SendAfterClosing);
        let err: GhostwriterError = ws_err.into();
        assert!(matches!(err, GhostwriterError::Network(_)));
        assert!(err.to_string().contains("closing"));
    }
}
