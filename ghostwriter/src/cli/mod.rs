use clap::{ArgGroup, Parser};
use std::path::PathBuf;

/// Command line arguments for Ghostwriter
#[derive(Debug, Parser, PartialEq)]
#[command(author, version, about)]
#[command(group(
    ArgGroup::new("mode")
        .required(true)
        .args(["path", "server", "connect"]),
))]
pub struct Args {
    /// Open a local file or directory
    pub path: Option<PathBuf>,

    /// Start in server mode with the given workspace directory
    #[arg(long)]
    pub server: Option<PathBuf>,

    /// Connect to a remote server WebSocket URL
    #[arg(long)]
    pub connect: Option<String>,

    /// Bind address for server mode
    #[arg(long, default_value = "127.0.0.1")]
    pub bind: String,

    /// Port for server mode
    #[arg(long, default_value_t = 8080)]
    pub port: u16,

    /// Optional authentication key
    #[arg(long)]
    pub key: Option<String>,
}

impl Args {
    /// Validate file or directory paths exist when provided
    pub fn validate(&self) -> crate::error::Result<()> {
        if let Some(path) = &self.path {
            if !path.exists() {
                return Err(crate::error::GhostwriterError::FileNotFound);
            }
        }
        if let Some(dir) = &self.server {
            if !dir.is_dir() {
                return Err(crate::error::GhostwriterError::InvalidArgument(
                    "server path must be a directory".into(),
                ));
            }
        }
        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::Args;
    use clap::Parser;
    use tempfile::{NamedTempFile, TempDir};

    #[test]
    fn test_parse_local_file_args() {
        let file = NamedTempFile::new().unwrap();
        let path = file.path().to_str().unwrap();
        let args = Args::try_parse_from(["ghostwriter", path]).unwrap();
        assert_eq!(args.path.as_deref().unwrap(), file.path());
        assert!(args.validate().is_ok());
    }

    #[test]
    fn test_parse_server_args() {
        let dir = TempDir::new().unwrap();
        let dpath = dir.path().to_str().unwrap();
        let args = Args::try_parse_from([
            "ghostwriter",
            "--server",
            dpath,
            "--port",
            "8080",
            "--key",
            "secret",
        ])
        .unwrap();
        assert_eq!(args.server.as_deref().unwrap(), dir.path());
        assert_eq!(args.port, 8080);
        assert_eq!(args.key.as_deref(), Some("secret"));
        assert!(args.validate().is_ok());
    }

    #[test]
    fn test_parse_client_args() {
        let args = Args::try_parse_from([
            "ghostwriter",
            "--connect",
            "ws://server:8080",
            "--key",
            "secret",
        ])
        .unwrap();
        assert_eq!(args.connect.as_deref(), Some("ws://server:8080"));
        assert_eq!(args.key.as_deref(), Some("secret"));
        assert!(args.validate().is_ok());
    }

    #[test]
    fn test_invalid_args_rejected() {
        let dir = TempDir::new().unwrap();
        let result = Args::try_parse_from([
            "ghostwriter",
            "--server",
            dir.path().to_str().unwrap(),
            "--connect",
            "ws://server:8080",
        ]);
        assert!(result.is_err());
    }
}
