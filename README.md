# Ghostwriter editor

**Ghostwriter** is a lightweight, fast terminal-based text editor with continuous file synchronization and optional client-server architecture for remote editing. It prioritizes zero-configuration operation, blazing performance, and strict security sandboxing while providing a modern editing experience.

**Key Value Propositions:**
- **Zero Config**: Works perfectly out of the box with opinionated defaults
- **Continuous Sync**: Never lose data with contiuous file synchronization
- **Remote Ready**: Seamless local and remote editing with identical user experience
- **Security First**: Strict workspace sandboxing and single-user model

## Usage Examples

Start a server hosting a workspace directory:

```bash
ghostwriter --server /workspace --port 8080
```

Connect to a remote server from another machine:

```bash
ghostwriter --connect ws://server:8080
```
