# Ghostwriter editor

**Ghostwriter** is a lightweight, fast terminal-based text editor with continuous file synchronization and optional client-server architecture for remote editing. It prioritizes zero-configuration operation, blazing performance, and strict security sandboxing while providing a modern editing experience.

**Key Value Propositions:**
- **Zero Config**: Works perfectly out of the box with opinionated defaults
- **Continuous Sync**: Never lose data with contiuous file synchronization
- **Remote Ready**: Seamless local and remote editing with identical user experience
- **Security First**: Strict workspace sandboxing and single-user model

## Cross-Platform Compatibility

Ghostwriter builds as a single static binary on Linux targets and dynamically for macOS. The project is tested on Linux x86_64, Linux ARM64 and macOS ARM64 to ensure identical behavior. To build for another target install the appropriate Rust target and run:

```bash
cargo build --release --target <target-triple>
```

Static linking flags are configured in `.cargo/config.toml` for Linux targets so the resulting binaries have no external dependencies.
