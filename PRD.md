# Ghostwriter - Product Requirements Document (Zig Edition)

## Executive Summary
Ghostwriter is a lightweight terminal text editor rebuilt in **Zig** for speed, safety, and cross-platform consistency. It provides continuous file synchronization and an optional client/server model for remote editing. The new Zig implementation aims to deliver static binaries with minimal dependencies while retaining the core experience of the Rust prototype.

**Key Value Propositions**
- **Zero Configuration:** sensible defaults, works immediately after download.
- **Continuous Sync:** keystroke-level persistence to avoid data loss.
- **Remote Ready:** optional server mode for editing over the network.
- **Security First:** workspace sandboxing and single-user model.
- **Performance:** sub‑100ms operations and support for files up to 1 GB.

## Product Vision
Deliver the fastest and most reliable terminal editor that "just works" locally or remotely, compiled from Zig for predictable performance on Linux and macOS.

## Target Users
- Developers and administrators editing text on servers via SSH.
- Power users who prefer terminal workflows.
- Security‑conscious users needing sandboxed editing environments.

## Core Use Cases
### UC1: Local Editing
1. Run `ghostwriter path/to/file`.
2. File loads instantly; edits auto‑save in the background.
3. Undo/redo and search operate with low latency.

**Success Metrics**
- Startup < 50 ms.
- Keystroke latency < 10 ms.
- No data loss on crash.

### UC2: Remote Editing
1. Launch server: `ghostwriter --server /workspace --key secret`.
2. Connect from client: `ghostwriter --connect ws://host:8080 --key secret`.
3. Edit files as if local with automatic synchronization.

**Success Metrics**
- Connection latency < 200 ms.
- Sync latency < 100 ms.
- Lock cleanup on disconnect.

## Feature Requirements
### F1: Core Text Editing (P0)
- Insertion, deletion, copy, cut, paste.
- Cursor navigation by character, word, and line.
- Text selection via Shift + navigation.
- Linear undo/redo stack.
- Incremental search with regex.
- UTF‑8 validation with hex fallback.
- Binary file view as hex dump.

**Acceptance Tests**
- Editing operations respond in <10 ms.
- Undo/redo restores exact buffer state.
- Search highlights matches in real time.

### F2: File Management (P0)
- Single buffer; open one file at a time.
- Infinite history with back/forward navigation.
- Full‑screen file picker with tree view and fuzzy filter.
- Create, rename, delete files and folders inside picker.
- Workspace sandboxing with read‑only fallback when locked.

**Acceptance Tests**
- File picker handles 10k+ files with instant filtering.
- Path traversal outside workspace is rejected.

### F3: Networking (P1)
- Optional WebSocket server with single authenticated client.
- Delta synchronization at keystroke granularity.
- Read‑only mode for sharing files.

**Acceptance Tests**
- Unauthorized clients are rejected.
- Client edits appear on server within 100 ms.

## Non‑Functional Requirements
- Compiles with Zig 0.13 or newer.
- Static binaries for Linux x86_64, Linux ARM64, and macOS ARM64.
- Memory usage <100 MB for typical projects.
- Handles 1 GB files using memory mapping.

## Risks & Mitigations
- **Large File Performance:** use memory‑mapped I/O and chunked loading.
- **Network Reliability:** implement reconnect logic and graceful degradation.
- **Terminal Differences:** test on multiple emulators and platforms.

## Verification
- `zig build test` passes all unit tests.
- Benchmark suite confirms performance targets.
- Security review covers sandboxing and authentication.
- Documentation provides clear installation and usage examples.

Ghostwriter's Zig rewrite retains the original vision while leveraging Zig's simplicity and performance to deliver a robust, secure, and portable editor.
