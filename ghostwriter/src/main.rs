mod app;
mod cli;
mod editor;
mod files;
mod network;
mod ui;

use clap::Parser;

fn main() {
    let args = cli::Args::parse();
    if let Err(e) = args.validate() {
        eprintln!("Error: {e}");
        std::process::exit(1);
    }
    println!("Parsed arguments: {args:?}");
    // Placeholder module calls
    app::hello_app();
    editor::hello_editor();
    files::hello_files();
    network::hello_network();
    ui::hello_ui();
}

#[cfg(test)]
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
        // Check if placeholder functions from modules are callable
        app::hello_app();
        editor::hello_editor();
        files::hello_files();
        network::hello_network();
        ui::hello_ui();
        assert!(true, "Module functions callable");
    }
}
