use anyhow::{Result, anyhow};
use clap::Parser;
use std::path::PathBuf;

#[derive(Debug, Parser)]
#[command(author, version, about, long_about = None)]
pub struct Args {
    /// Run in server mode hosting the given workspace directory
    #[arg(long, value_name = "DIR", conflicts_with = "connect")]
    pub server: Option<PathBuf>,

    /// Connect to a remote server at the given URL
    #[arg(long, value_name = "URL", conflicts_with = "server")]
    pub connect: Option<String>,
}

#[derive(Debug, PartialEq, Eq)]
pub enum Mode {
    Local,
    Server { root: PathBuf },
    Connect { url: String },
}

impl Args {
    pub fn mode(&self) -> Result<Mode> {
        match (&self.server, &self.connect) {
            (Some(_), Some(_)) => Err(anyhow!("--server and --connect are mutually exclusive")),
            (Some(root), None) => Ok(Mode::Server { root: root.clone() }),
            (None, Some(url)) => Ok(Mode::Connect { url: url.clone() }),
            (None, None) => Ok(Mode::Local),
        }
    }
}

pub fn init_logging() {
    use tracing_subscriber::{EnvFilter, fmt};

    let filter = EnvFilter::try_from_default_env().unwrap_or_else(|_| EnvFilter::new("info"));
    let subscriber = fmt().with_env_filter(filter).finish();
    let _ = tracing::subscriber::set_global_default(subscriber);
}

pub async fn run() -> Result<()> {
    run_with_args(Args::parse()).await.map(|_| ())
}

async fn run_with_args(args: Args) -> Result<&'static str> {
    init_logging();
    let output = dispatch(args.mode()?);
    println!("{}", output);
    Ok(output)
}

fn dispatch(mode: Mode) -> &'static str {
    match mode {
        Mode::Local => {
            tracing::info!("mode = local");
            ghostwriter_client::run()
        }
        Mode::Server { .. } => {
            tracing::info!("mode = server");
            ghostwriter_server::run()
        }
        Mode::Connect { .. } => {
            tracing::info!("mode = connect");
            ghostwriter_client::run()
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use clap::Parser;

    fn parse_mode(args: &[&str]) -> Mode {
        let cli = Args::parse_from(std::iter::once("ghostwriter").chain(args.iter().cloned()));
        cli.mode().unwrap()
    }

    fn run_args(args: Args) -> &'static str {
        tokio::runtime::Runtime::new()
            .unwrap()
            .block_on(run_with_args(args))
            .unwrap()
    }

    #[test]
    fn default_is_local() {
        assert_eq!(parse_mode(&[]), Mode::Local);
    }

    #[test]
    fn parses_server() {
        assert_eq!(
            parse_mode(&["--server", "/tmp"]),
            Mode::Server {
                root: PathBuf::from("/tmp")
            }
        );
    }

    #[test]
    fn parses_connect() {
        assert_eq!(
            parse_mode(&["--connect", "ws://localhost"]),
            Mode::Connect {
                url: "ws://localhost".into()
            }
        );
    }

    #[test]
    fn rejects_conflicting_args() {
        let args = Args {
            server: Some(PathBuf::from("/tmp")),
            connect: Some("ws://localhost".into()),
        };
        assert!(args.mode().is_err());
    }

    #[test]
    fn dispatches_local() {
        assert_eq!(dispatch(Mode::Local), "client");
    }

    #[test]
    fn dispatches_server() {
        assert_eq!(
            dispatch(Mode::Server {
                root: PathBuf::from("/tmp"),
            }),
            "server"
        );
    }

    #[test]
    fn dispatches_connect() {
        assert_eq!(
            dispatch(Mode::Connect {
                url: "ws://localhost".into(),
            }),
            "client"
        );
    }

    #[test]
    fn run_with_args_local() {
        assert_eq!(
            run_args(Args {
                server: None,
                connect: None
            }),
            "client"
        );
    }

    #[test]
    fn run_with_args_server() {
        assert_eq!(
            run_args(Args {
                server: Some(PathBuf::from("/tmp")),
                connect: None,
            }),
            "server"
        );
    }

    #[test]
    fn run_with_args_connect() {
        assert_eq!(
            run_args(Args {
                server: None,
                connect: Some("ws://localhost".into()),
            }),
            "client"
        );
    }

    #[test]
    fn run_defaults_to_local() {
        assert_eq!(
            run_args(Args {
                server: None,
                connect: None,
            }),
            "client",
        );
    }
}
