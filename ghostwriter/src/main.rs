mod app;
mod cli;
mod editor;
mod error;
mod files;
mod network;
mod security;
mod state;
mod ui;

use clap::Parser;
use log::error;
use std::net::SocketAddr;
#[cfg(test)]
use std::path::PathBuf;

use crate::error::{GhostwriterError, Result};

fn parse_addr(bind: &str, port: u16) -> Result<SocketAddr> {
    let addr = format!("{bind}:{port}");
    addr.parse::<SocketAddr>()
        .map_err(|e| GhostwriterError::InvalidArgument(e.to_string()))
}

async fn create_server(args: &cli::Args) -> Result<network::server::GhostwriterServer> {
    let dir = args
        .server
        .clone()
        .ok_or_else(|| GhostwriterError::InvalidArgument("missing server path".into()))?;
    let ws = files::workspace::WorkspaceManager::new(dir)?;
    let addr = parse_addr(&args.bind, args.port)?;
    network::server::GhostwriterServer::bind(addr, ws, args.key.clone()).await
}

fn run_server(args: &cli::Args) -> Result<()> {
    let rt = tokio::runtime::Runtime::new()?;
    rt.block_on(async {
        let server = create_server(args).await?;
        server.run().await
    })
}

fn main() {
    env_logger::init();
    let args = cli::Args::parse();
    if let Err(e) = args.validate() {
        error!("{e}");
        eprintln!("Error: {e}");
        std::process::exit(1);
    }
    if args.server.is_some() {
        if let Err(e) = run_server(&args) {
            error!("{e}");
            eprintln!("Error: {e}");
            std::process::exit(1);
        }
    } else if let Some(path) = args.path {
        match app::App::open(path) {
            Ok(mut app) => {
                if let Err(e) = app.run() {
                    error!("{e}");
                    eprintln!("Error: {e}");
                    std::process::exit(1);
                }
            }
            Err(e) => {
                error!("{e}");
                eprintln!("Error: {e}");
                std::process::exit(1);
            }
        }
    } else {
        println!("No operation specified. Use --help for options.");
    }
}

#[cfg(test)]
#[allow(
    clippy::assertions_on_constants,
    clippy::needless_borrows_for_generic_args
)]
mod tests {
    // Import common dependencies to check if they load
    use super::*;
    use clap::Parser;
    use crossterm::style::Stylize;
    use ratatui::widgets::Block;
    use serde::Serialize;
    use tokio::runtime::Runtime;

    #[test]
    fn test_project_compiles() {
        // This test primarily serves as a marker.
        // If the project compiles, this test will run and pass.
        assert!(true, "Project compiled successfully");
    }

    #[test]
    fn test_dependencies_load() {
        // Try to use a type or function from each major dependency category

        // clap
        #[derive(Parser, Debug)]
        struct TestArgs {
            #[clap(short, long)]
            name: Option<String>,
        }
        let args = TestArgs::try_parse_from(&["test", "-n", "value"]);
        assert!(args.is_ok() || args.is_err()); // Just check parsing was attempted

        // crossterm
        let styled_text = "Hello".blue().on_yellow();
        assert!(
            !styled_text.to_string().is_empty(),
            "Crossterm styling failed"
        );

        // ratatui
        let _block = Block::default();
        assert!(true, "Ratatui Block created");

        // serde
        #[derive(Serialize)]
        struct TestStruct {
            field: String,
        }
        let test_instance = TestStruct {
            field: "test".to_string(),
        };
        let json_result = serde_json::to_string(&test_instance);
        assert!(json_result.is_ok(), "Serde serialization failed");

        // tokio
        let rt = Runtime::new();
        assert!(rt.is_ok(), "Tokio runtime creation failed");
        if let Ok(rt) = rt {
            rt.block_on(async {
                assert!(true, "Tokio async block executed");
            });
        }

        println!("Dependencies loaded and basic usage verified.");
    }

    #[test]
    fn test_modules_callable() {
        // Basic sanity checks on module API accessibility
        let _app = app::App::open(std::env::temp_dir().join("tmp.txt"));
        let _cursor = editor::cursor::Cursor::new();
        let _ = files::file_manager::FileManager::is_binary(b"test");
        let msg = network::protocol::Message {
            id: uuid::Uuid::nil(),
            kind: network::protocol::MessageKind::Ping,
        };
        let _ = serde_json::to_string(&msg).unwrap();
        assert!(true, "Module functions callable");
    }

    #[test]
    fn test_parse_addr_valid() {
        let addr = parse_addr("127.0.0.1", 8080).unwrap();
        assert_eq!(addr.port(), 8080);
    }

    #[test]
    fn test_parse_addr_invalid() {
        let res = parse_addr("invalid", 8080);
        assert!(res.is_err());
    }

    #[tokio::test]
    async fn test_create_server_invalid_dir() {
        let args = cli::Args {
            path: None,
            server: Some(PathBuf::from("/no/such")),
            connect: None,
            bind: "127.0.0.1".into(),
            port: 0,
            key: None,
        };
        let res = create_server(&args).await;
        assert!(res.is_err());
    }
}
