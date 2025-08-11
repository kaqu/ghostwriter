# 0) Project bootstrap

**Create a Cargo workspace with feature gates and release flags.**

```
ghostwriter/
  Cargo.toml
  crates/
    proto/
    core/
    server/
    client/
  src/main.rs
```

**Top-level `Cargo.toml`:**

```toml
[workspace]
members = ["crates/*"]

[profile.release]
lto = "fat"
codegen-units = 1
panic = "abort"
strip = "symbols"
opt-level = "z"

[workspace.dependencies]
anyhow = "1"
thiserror = "2"
bytes = "1"
tokio = { version = "1", features = ["full"] }
futures = "0.3"
serde = { version = "1", features = ["derive"] }
rmp-serde = "1"
serde_json = "1"
clap = { version = "4", features = ["derive"] }
tracing = "0.1"
tracing-subscriber = { version = "0.3", features = ["fmt", "env-filter", "json"] }
uuid = { version = "1", features = ["v4", "serde"] }
parking_lot = "0.12"

# client
crossterm = "0.28"
ratatui = "0.28"

# networking & tls
tokio-tungstenite = { version = "0.23", features = ["rustls-tls-native-roots"] }
url = "2"
rustls = "0.23"
rustls-pemfile = "2"

# core text & fs
ropey = "1"
memmap2 = "0.9"
regex-automata = "0.4"
notify = "6"
argon2 = "0.5"
rand = "0.8"
blake3 = "1"
crc32fast = "1"
nix = { version = "0.29", features = ["fs", "poll", "user"] }

# tests & benches
proptest = "1"
criterion = { version = "0.5", default-features = false }
```

**`src/main.rs` skeleton:**

```rust
mod cli;
use anyhow::Result;

#[tokio::main]
async fn main() -> Result<()> {
    ghostwriter::cli::run().await
}
```

---

# 1) Protocol & transport (proto crate)

**Goal:** A stable, versioned wire format + transport helpers used by both client and server.

```
crates/proto/
  Cargo.toml
  src/lib.rs
```

**Design decisions**

* MessagePack on the wire via `rmp-serde`.
* Versioned envelope `{ v: 1, type: "...", data: ... }`.
* Mutations carry `seq: u64`; server replies `Ack{seq, doc_v}`.
* Frames transmit ready-to-render lines and style spans.

**`proto/src/lib.rs`:**

```rust
use serde::{Serialize, Deserialize};

pub const PROTOCOL_VERSION: u16 = 1;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Envelope<T> {
    pub v: u16,
    #[serde(rename = "type")]
    pub ty: MessageType,
    pub data: T,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub enum MessageType {
    Hello, Auth, Open, Insert, Delete, Move, Select, Scroll, Resize,
    Search, GotoLine, DuplicateLine, DeleteLine, Save, RequestFrame, PickerAction,
    Ack, Frame, Dirty, Status, Dialog, Error, Ping, Pong,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Hello {
    pub client_name: String,
    pub client_ver: String,
    pub cols: u16,
    pub rows: u16,
    pub truecolor: bool,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Ack { pub seq: u64, pub doc_v: u64 }

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Range { pub from: u64, pub to: u64 }

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Insert { pub pos: u64, pub text: String, pub seq: u64 }

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Delete { pub range: Range, pub seq: u64 }

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum MoveKind { Left, Right, Up, Down, WordPrev, WordNext, LineStart, LineEnd, DocStart, DocEnd, PageUp, PageDown }
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct MoveCmd { pub kind: MoveKind }

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Resize { pub cols: u16, pub rows: u16 }

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct StyleSpan { pub start_col: u16, pub end_col: u16, pub class_: String }

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Line { pub text: String, pub spans: Vec<StyleSpan> }

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Cursor { pub line: u64, pub col: u16 }

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Frame {
    pub id: String,               // "editor" | "picker" | "dialog"
    pub kind: String,
    pub doc_v: u64,
    pub first_line: u64,
    pub cols: u16, pub rows: u16,
    pub lines: Vec<Line>,
    pub cursors: Vec<Cursor>,
    pub status_left: String, pub status_right: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum ErrorCode { Unauthorized, Invalid, Unsupported, Busy, Io, Sandbox, Conflict, RateLimit }

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ErrorMsg { pub code: ErrorCode, pub msg: String }
```

**Transport helpers** (serialize/deserialize to/from `tungstenite::Message::Binary`). Add:

* `encode<T: Serialize>(ty, data) -> Vec<u8>`
* `decode(bytes) -> (MessageType, serde_json::Value)` or strongly typed via matching.

---

# 2) Core text engine (core crate)

**Goal:** Rope-based buffer (with piece-table path later), undo/redo, search, viewport composer, WAL.

```
crates/core/
  src/lib.rs
  src/buffer.rs
  src/undo.rs
  src/viewport.rs
  src/search.rs
  src/wal.rs
  src/fs.rs
  src/picker.rs
```

## 2.1 Buffer trait and implementations

```rust
pub trait BufferEngine: Send {
    fn len_bytes(&self) -> u64;
    fn insert(&mut self, byte_pos: u64, text: &str);
    fn delete(&mut self, from: u64, to: u64);
    fn slice_lines(&self, first_line: u64, max_lines: u64) -> Vec<String>;
    fn byte_to_line_col(&self, byte_pos: u64) -> (u64, u16);
    fn line_col_to_byte(&self, line: u64, col: u16) -> u64;
    fn grapheme_left(&self, byte_pos: u64) -> u64;
    fn grapheme_right(&self, byte_pos: u64) -> u64;
    fn save_to(&self, path: &std::path::Path) -> anyhow::Result<()>;
}
```

**Initial impl:** `RopeBuffer` using `ropey` with an internal line index cache.
**Large files (feature `largefile`):** later add `PieceTableBuffer` (mmap + append-only add buffer). Gate behind runtime selection by file size.

## 2.2 Undo/redo

* `undo.rs` keeps a stack of `Edit { before_sel: Range, after_sel: Range, op: Insert/Delete, payload }`.
* Linear history; coalesce adjacent inserts during debounce window.

## 2.3 Viewport composer

* `viewport.rs`: converts buffer + selections into `proto::Frame`:

  * Computes `first_line`, extracts `lines`, emits style spans (v1: minimal classes `"sel"`, `"ws"`, `"err"`; syntax optional later).
  * Horizontal scroll support (store `hscroll` in session).

## 2.4 Search

* `search.rs`: regex-automata over chunked text; incremental search state with last query and positions; highlights only visible matches.

## 2.5 WAL

* `wal.rs`: binary format per PRD.

  * API: `append(&EditRecord)`, `replay() -> Vec<EditRecord>`, `compact_if_needed()`.
  * CRC32 over record body.

## 2.6 FS & atomic save

* `fs.rs`: path canonicalization, sandbox checks, lock file, atomic write:

```rust
pub fn atomic_write(path: &Path, bytes: &[u8]) -> anyhow::Result<()> {
    use std::fs::{File, OpenOptions};
    use std::io::{Write};
    let dir = path.parent().unwrap();
    let mut tmp = dir.to_path_buf();
    let nonce = rand::random::<u64>();
    tmp.push(format!(".{}.gw.tmp.{}", path.file_name().unwrap().to_string_lossy(), nonce));
    let mut f = OpenOptions::new().create_new(true).write(true).open(&tmp)?;
    f.write_all(bytes)?;
    f.sync_all()?;
    std::fs::rename(&tmp, path)?;
    // fsync dir
    let dirf = File::open(dir)?;
    #[cfg(target_os = "linux")] { use std::os::fd::AsRawFd; nix::unistd::fsync(dirf.as_raw_fd())?; }
    Ok(())
}
```

## 2.7 Picker index

* `picker.rs`: workspace tree scan (ignore `.git`), flat index for fuzzy scoring (fzy-style), cached preview (first N lines).

---

# 3) Server (server crate)

```
crates/server/
  src/lib.rs
  src/ws.rs           # acceptor + handshake
  src/session.rs      # single active session actor
  src/auth.rs
  src/tree.rs         # picker tree + fuzzy
  src/watch.rs        # external file watcher
  src/state.rs        # workspace state, locks
```

## 3.1 Single-user acceptor

* Listens on TCP (and UDS).
* Maintains `Option<SessionHandle>`; if Some, new connections rejected with `Error{Busy}`.
* `ws.rs`:

  * Use `tokio_tungstenite::accept_async` (server) / `connect_async` (client).
  * Read binary messages → decode via `proto`.
  * Heartbeat: respond to `Ping` with `Pong`.

## 3.2 Authentication & rate limit

* `auth.rs`:

  * If a secret is configured, require `Auth` after `Hello`.
  * Store Argon2id hash/salt in `.ghostwriter/auth.json`.
  * Fixed-window counter per peer addr.

## 3.3 Session actor (stateful)

* Holds:

  * `buffer: Box<dyn BufferEngine>`
  * `doc_v: u64`
  * `undo: UndoStack`
  * `cursor: u64` (byte offset) + selection ranges
  * `viewport: { first_line, rows, cols, hscroll }`
  * `wal: WalWriter`
  * `watch: WatchHandle`
  * `readonly: bool`
* Exposes `apply_insert(seq, pos, text)`, `apply_delete(seq, range)`, `move_(kind)`, `select(kind)`, `save()`, `open(path)`, `compose_frame()`.
* After every mutating op:

  * Append to WAL → apply to buffer → bump `doc_v` → schedule save (debounced) → send `Ack` and `Frame`.

**Debouncers:** use `tokio::time::sleep` keyed per buffer; 100 ms idle triggers save.

## 3.4 External change detection

* `watch.rs` (notify crate): if the underlying file changes (mtime/inode/len/hash), pause saves and push `Dialog{Reload|Keep mine}` to client.

## 3.5 Picker

* `tree.rs` uses `core::picker`.
* Handles `PickerAction`:

  * `QueryChanged`, `Open`, `CreateFile`, `CreateFolder`, `Rename`, `Delete`, `Expand`, `Collapse`.
* Emits `Frame{kind:"picker"}` with tree + preview.

---

# 4) Client (client crate)

```
crates/client/
  src/lib.rs
  src/tui.rs         # ratatui renderer
  src/input.rs       # key mapping
  src/net.rs         # ws client, event queue, reconnect
  src/app.rs         # glue
```

## 4.1 TUI renderer

* Consumes `proto::Frame` and draws:

  * Status line (left/right).
  * Text area: print `lines[i].text`; apply `spans` with `ratatui::style`.
  * Cursor: move terminal cursor to provided coordinates.
* No local state besides last frame and terminal size.

## 4.2 Input mapping

* Map crossterm events to protocol:

  * `Ctrl+O` → `PickerAction{OpenOverlay}`
  * `Ctrl+Z` → send high-level `Undo` (or `Key` + `Move/Select` as needed)
  * Character keys → `Insert` with `seq` and echo locally (optional).

## 4.3 Networking & queue

* Maintain a bounded MPSC (256) for outbound events when WS is down.
* Reconnect with backoff (250 ms → 5 s); re-Hello on connect; re-Auth if configured; send `RequestFrame` after `Resize`.

---

# 5) Single binary integration

**`src/cli.rs`** (in root crate; depends on server/client/proto):

```rust
use clap::{Parser, Subcommand};
use anyhow::Result;

#[derive(Parser)]
pub struct Cli {
  #[arg(value_name="PATH")] path: Option<String>,
  #[command(subcommand)] cmd: Option<Cmd>,
  #[arg(long)] readonly: bool,
  #[arg(long)] port: Option<u16>,
  #[arg(long)] bind: Option<String>,
  #[arg(long)] connect: Option<String>,
  #[arg(long)] key: Option<String>,
  #[arg(long)] uds: Option<String>,
  // ... tls flags, logs, etc.
}

#[derive(Subcommand)]
enum Cmd {
  Server { workspace: String },
  // reserved for future subcommands
}

pub async fn run() -> Result<()> {
  let cli = Cli::parse();
  tracing_subscriber::fmt()
    .with_env_filter(tracing_subscriber::EnvFilter::from_default_env())
    .json()
    .init();

  match (&cli.cmd, &cli.connect, &cli.path) {
    (Some(Cmd::Server{workspace}), _, _) => run_server_mode(workspace, &cli).await,
    (_, Some(url), _) => run_client_mode(url, &cli).await,
    (_, _, Some(path)) => run_local_mode(path, &cli).await,
    _ => { eprintln!("Usage..."); Ok(()) }
  }
}
```

**Local mode options:**

* **In-proc:** spin up server on a loopback UDS (random path), then client connects to that UDS. On exit: send shutdown signal; clean UDS; fsync if pending.
* **Or** directly spawn server & client tasks and bridge via in-memory channel implementing a `Transport` trait (optimization later).

---

# 6) Phase plan with acceptance checks

## Phase 1 — Core editor (local only)

**Deliverables**

* RopeBuffer + Undo/Redo + Viewport composer.
* Atomic save + WAL.
* Minimal server session (no WS yet): expose async channel for requests.
* Minimal client (TUI) bound to same process channel.

**Tests**

* Unit: insert/delete/undo redo invariants.
* Property: random edit scripts → buffer equals replay(WAL) result.
* Integration: open/save; crash during save → WAL recovery.

**Run**

```
cargo run -- /tmp/foo.txt
```

## Phase 2 — Real server over WebSocket

**Deliverables**

* WS acceptor (TCP & UDS), single-user gating.
* Protocol handshake (Hello/Ack), Ping/Pong.
* Auth (optional) with Argon2; rate limit.
* Client connects to `ws://127.0.0.1:8080` and renders frames.

**Tests**

* Integration: second client → `Busy`.
* Security: wrong password thrice → `RateLimit`.
* Latency: keystroke → frame p95 < 100 ms locally.

**Run**

```
ghostwriter --server /workspace
ghostwriter --connect ws://127.0.0.1:8080
```

## Phase 3 — Picker & filesystem operations

**Deliverables**

* Workspace scan + fuzzy filter + preview.
* Picker frames and actions: create/rename/delete/dirs, RO checks.
* Cursor/scroll restore per file; file history back/forward.

**Tests**

* 10k files synthetic tree: filter update < 10 ms per keystroke.
* File ops succeed/reflect immediately; sandbox escape attempts → `Sandbox`.

## Phase 4 — Resilience & external changes

**Deliverables**

* File watcher; conflict dialog (Reload/Keep mine).
* Debounced saves (100 ms), coalesced inserts; disk-full handling.
* Bounded client event queue; reconnect/backoff with session resume window.

**Tests**

* Kill server mid-typing, restart: WAL replay restores to last edit before crash.
* Edit offline (disconnect): queue ≤256; reconnect applies; overflow → warning.

## Phase 5 — Performance & polish

**Deliverables**

* Optional piece-table backend for ≥100 MB files (feature `largefile`).
* Incremental frame composition (only dirty lines recomputed).
* Status line details, hex viewer for binary.
* Logs/metrics; CLI polish; packaging.

**Bench**

* 1 GB file paging p95 < 100 ms.
* Memory < 100 MB (excluding mmaps).

---

# 7) Key integration points (interfaces & flows)

## 7.1 Client event → Server mutation → Frame

1. Client captures key `a`:

   * `seq += 1`
   * Sends `Insert{pos, "a", seq}`
   * Optionally echoes `a` locally (optimistic).

2. Server `session.apply_insert(seq, pos, text)`:

   * WAL.append(INSERT)
   * buffer.insert(...)
   * doc\_v += 1
   * schedule save (debounced)
   * send `Ack{seq, doc_v}` and `Frame{...}`

3. Client receives `Frame`, replaces screen content, reconciles cursor.

## 7.2 Resize

* Client sends `Resize{cols, rows}` and `RequestFrame{reason:"resize"}`
* Server updates viewport dims; recomposes; sends `Frame`.

## 7.3 Picker open

* Client sends `PickerAction{Open}`
* Server sets `ui_mode = Picker`, composes picker `Frame`
* Client renders; all keys go as `PickerAction{...}` until `Close`.

---

# 8) Concrete algorithms & data details

* **Word/grapheme navigation:** use `unicode-segmentation` (or ropey’s graphemes) to move left/right; cache per line for speed.
* **Fuzzy scoring:** implement fzy subsequence with bonuses (start of word, path separator, adjacency). Keep a heap of top N matches (page window).
* **Dirty line tracking:** on insert/delete, record affected line range; viewport composer only regenerates those lines’ spans.
* **Hex viewer:** fixed 16 bytes/row, compute ASCII gutter; navigation uses line/col to compute byte offset.

---

# 9) Observability & logging

* `tracing` everywhere; default human fmt in dev, JSON in release (`GHOSTWRITER_LOG=info`).
* Counters (simple, internal):

  * `apply_ms_p95`, `frame_ms_p95`, `save_fail_total`, `reconnect_total`, `queue_drop_total`.
* Audit log: `open/save/delete/rename/create/auth_result` (no contents).

---

# 10) CI/CD & quality gates

* **GitHub Actions**:

  * `cargo fmt -- --check`
  * `cargo clippy -- -D warnings`
  * `cargo test --workspace`
  * `cargo deny check` (optional)
  * Release job builds Linux x86\_64/ARM64 and macOS ARM64.

* **Pre-commit**: format, clippy, `cargo audit`.

---

# 11) Security specifics

* **Auth storage:** `.ghostwriter/auth.json`:

```json
{"kdf":"argon2id","mem_mib":64,"time_ms":250,"salt_b64":"...","hash_b64":"..."}
```

* **Rate limit:** HashMap\<Peer, Counter{count, window\_start}> with pruning.
* **Sandbox:** canonicalize path; ensure `resolved.starts_with(root)` by path component boundary; open with `O_NOFOLLOW`. Landlock (feature-gated) if kernel supports.

---

# 12) Packaging & flags

* Features:

  * `default = ["largefile", "truecolor"]`
  * `tls`, `landlock`
* Targets:

  * `x86_64-unknown-linux-musl`, `aarch64-unknown-linux-musl`, `aarch64-apple-darwin`.
* Size trim: avoid bundling large data; no tree-sitter in v1.

---

# 13) Test matrix (must pass for 1.0)

* **Functional:** F1–F9 acceptance tests from PRD.
* **Property:** edit scripts vs WAL replay equality.
* **FS:** atomic save survives crash (kill -9 loop).
* **Network:** drop packets / delay injection; ensure idempotent `seq`.
* **Picker:** synthetic 10k files performance.
* **Security:** path traversal attempts; symlink traps; RO mode enforcement.

---

# 14) Stretch (post-1.0, keep hooks ready)

* QUIC transport (`quinn`) with 0-RTT resumption.
* Syntax highlighting (server-side) with tree-sitter (feature-gated).
* LSP (server-side); translate diagnostics to spans/gutter.
* Mouse support; soft-wrap; split panes.
