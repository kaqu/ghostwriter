use anyhow::Result;

#[tokio::main]
async fn main() -> Result<()> {
    ghostwriter::cli::run().await
}
