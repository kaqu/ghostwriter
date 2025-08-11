# Ghostwriter — Product Requirements Document

**Document status:** v1.0
**Last updated:** 2025-08-11

---

## 1. Scope

Ghostwriter is a fast, terminal-based text editor with a **minimal (dumb) client** and a **stateful server** that owns all editing logic and persistence. This PRD is the authoritative specification for v1.0 and uses RFC-2119 keywords (**MUST/SHOULD/MAY**).

Out of scope for v1.0: plugins, LSP, syntax highlighting beyond simple token classes, multi-user collaboration, GUI.

---

## 2. Goals & Non-Goals

### 2.1 Goals

* Zero-configuration operation for local editing.
* Single-user, secure remote editing via WebSocket with optional TLS.
* Keystroke-level durability (debounced) with crash safety.
* Responsiveness with large files (up to 1 GB).
* Strict workspace sandboxing.

### 2.2 Non-Goals

* Multi-document tabs or splits (single buffer visible at a time).
* Custom keybinding configuration.
* Multi-user concurrency on the same server instance.
* Windows support in v1.0.

---

## 3. Definitions

* **Client:** Terminal app that only captures input, requests frames, and renders frames. Holds no text model.
* **Server:** Stateful process that manages buffers, editing, search, file I/O, locks, auth, sandbox, and frame composition.
* **Frame:** A fully composed, styled viewport (or picker/dialog) to render verbatim.
* **Doc Version (`doc_v`):** Monotonic integer incremented on each applied edit.
* **Seq:** Client-supplied monotonic integer for deduplication/acknowledgment of mutating requests.
* **Workspace:** Canonicalized directory root to which all file operations are confined.
* **WAL:** Append-only write-ahead log of edits for crash recovery.

---

## 4. Architecture Overview

* **Transport:** WebSocket over TCP; optional TLS. Local mode MAY use Unix Domain Sockets.
* **Server state:** Per-session editor state (buffers, selections, undo/redo), per-workspace tree index, WAL, file locks.
* **Client state:** Screen size, last received frame (for redraw), small bounded outbound event queue during transient disconnects.

Concurrency model:

* One server process **MUST** accept exactly one active client session (single-user model). Additional connection attempts **MUST** be rejected with a clear error.
* Internally, the server **SHOULD** isolate the active buffer in a single-threaded actor to avoid contention; background tasks (indexing, fs watch) run on separate executors.

---

## 5. Functional Requirements

### F1. Core Text Editing (Server)

**MUST:**

* Insert, delete, cut, copy, paste.
* Linear undo/redo; selection state is part of undo frame.
* Navigation: char, word (prev start/next end), line start/end, document start/end, page up/down.
* Selection via Shift + any navigation.
* Line ops: go-to-line, duplicate line(s), delete line(s).
* UTF-8 primary encoding; invalid sequences preserved and shown via hex fallback glyphs.
* Files up to **1 GB** openable; edits MUST remain responsive.

**SHOULD:**

* Word boundaries per Unicode UAX#29 (grapheme boundaries for cursor movement).
* CRLF ↔ LF normalization in memory; preserve original EOL on save.
* Tabs rendered at configurable width (default 4), stored as tab bytes.

**Acceptance:**

* Edit apply time `< 10 ms` p95 for 10k edits in a 10 MB file on reference hardware.
* Undo/redo restores buffer and selections exactly.

---

### F2. File Management & Picker (Server)

**MUST:**

* Single visible buffer; history of visited files with back/forward semantics (branching when opening new file mid-history).
* Full-screen picker with:

  * Tree view rooted at workspace.
  * Fuzzy filter across the entire path.
  * Real-time preview (first N lines; N configurable, default 200).
  * File ops: create file, create folder, rename, delete (with confirmation).
* Strict workspace sandboxing; canonicalization prevents traversal outside root.
* Read-only fallback if write lock unavailable.

**Acceptance:**

* Index of 10,000 files loads in `< 200 ms` initial, updates incrementally.
* Each keystroke in filter scores and updates view in `< 10 ms`.
* File switch restores cursor and scroll positions.

---

### F3. Continuous Synchronization & Durability (Server)

**MUST:**

* Debounced save (default 100 ms idle).
* Atomic write: create temp in same directory, write, `fsync(temp)`, `rename`, `fsync(parent dir)`.
* Per-buffer WAL: append edit record **before** applying to in-memory model.
* External change detection with notification and conflict flow (view-only diff + “Reload/Keep mine”).
* Disk-full and permission errors surfaced as dialogs; editor remains responsive.

**Acceptance:**

* No data loss after forced crash/power loss during editing or saving.
* External changes detected `< 1 s`.

---

### F4. Client Behavior

**MUST:**

* Capture keyboard events and terminal resize; send to server.
* Render frames exactly as specified (text lines + style spans + cursor(s) + status).
* Maintain small bounded queue (default 256 events) during temporary disconnects, with clear overflow warning.
* Optional predictive echo for single-character inserts; reconcile on next frame.

**MUST NOT:**

* Store or mutate buffer state.
* Compute selections, wrapping, or syntax locally.

---

### F5. Protocol

See §8 for full schema. High-level requirements:

**MUST:**

* Request/response with `seq` on mutating requests; `Ack{seq, doc_v}` from server.
* Server-pushed `Frame` updates after state changes or explicit `RequestFrame`.
* Versioned envelope; unknown message types ignored with `Error{code=UNSUPPORTED}`.
* Heartbeats bidirectional with `Ping/Pong` (60 s interval, 15 s timeout).

**Performance:**

* End-to-end keystroke to frame latency `< 100 ms` p95 over 100 ms RTT.

---

### F6. Security & Sandboxing (Server)

**MUST:**

* Workspace root canonicalization: resolve symlinks; reject paths escaping root.
* Open files with `O_NOFOLLOW`; prevent following symlinks during creation/rename.
* Single active file lock; automatic release on disconnect or file switch.
* Optional authentication:

  * Secrets stored as Argon2id hashes with salted, memory-hard parameters (memory ≥ 64 MB, iterations tuned for \~250 ms on reference CPU).
  * Authentication attempts rate-limited (≤ 3/minute).
* Audit log of metadata (path, op, timestamp, result); **MUST NOT** log file contents.

**SHOULD:**

* Linux Landlock sandbox if available; otherwise best-effort path and fd discipline.
* TLS support with rustls; modern ciphersuites only.

---

### F7. Performance & Large Files (Server)

**MUST:**

* Text store uses rope for general case.
* For files ≥ 100 MB, server **MUST** employ piece-table with memory-mapped original file (read-only) + append-only scratch file.
* Viewport extraction **MUST** be O(lines\_in\_view).
* Incremental frame composition: only changed lines re-tokenized and re-styled.

**Targets:**

* 1 GB file navigable with paging latency `< 100 ms` p95.
* Typical memory footprint `< 100 MB` excluding mmaps.

---

### F8. Network Resilience

**MUST:**

* Automatic reconnect with exponential backoff (initial 250 ms, cap 5 s).
* Session resumption token; on reconnect, server reattaches to session if within 30 s grace.
* If outbound queue overflows, client drops oldest and shows non-intrusive warning.
* All mutating requests are idempotent under retries (by `seq`).

---

### F9. Terminal Rendering

**MUST:**

* ANSI/VT compatible; 256-color minimum; truecolor when available.
* Cursor shapes: block/beam via DECSCUSR where supported; fallback to block.
* Grapheme cluster rendering per UAX#29; East Asian width handling for alignment.
* Tabs rendered at configured width; wide characters occupy two cells.
* No mouse support in v1.0.

---

## 6. Non-Functional Requirements

* **Startup:** Local (in-proc) `< 50 ms`; remote connection `< 200 ms`.
* **Keystroke latency:** Server apply `< 10 ms` p95; end-to-end `< 100 ms` p95.
* **Saves:** `< 50 ms` typical; durable as per §F3.
* **Picker:** `< 10 ms` per keystroke on 10k files.
* **Uptime:** Weeks without restart.
* **Crash rate:** `< 0.1%` sessions.
* **Memory:** `< 100 MB` typical (excluding mmaps).

---

## 7. CLI, Config & Modes

### 7.1 CLI

```
ghostwriter [PATH]                       # Local editing; in-proc server + client
ghostwriter --server WORKSPACE [opts]    # Dedicated server
ghostwriter --connect URL [opts]         # Dumb client to remote server
```

**Options**

* `--readonly` (server): Force read-only.
* `--port PORT` (server): Default 8080.
* `--bind ADDR` (server): Default 127.0.0.1.
* `--key SECRET` (server/client): Passphrase for auth.
* `--timeout SECONDS` (client): Default 30.
* `--tls` (server/client): Enable TLS; uses `--cert`/`--key-file`.
* `--cert PATH`, `--key-file PATH` (server): TLS materials.
* `--uds PATH` (local): Use Unix Domain Socket instead of TCP.
* `--log-level trace|debug|info|warn|error` (both).
* `--tab-width N` (server-rendering).
* `--preview-lines N` (picker).

### 7.2 Environment Variables

* `GHOSTWRITER_KEY` (client/server auth secret).
* `GHOSTWRITER_LOG` (log level).
* `NO_COLOR` (disable color).

### 7.3 Precedence

CLI flag > environment variable > built-in default.

---

## 8. Wire Protocol (v1)

**Encoding:** MessagePack on the wire. Schemas below shown in JSON for readability. Envelope:

```json
{ "v": 1, "type": "MessageType", "data": { /* message-specific */ } }
```

### 8.1 Client → Server

* `Hello`

  ```json
  { "v":1, "type":"Hello", "data":{"client_name":"ghostwriter","client_ver":"1.0.0","cols":120,"rows":40,truecolor:true} }
  ```

* `Auth`

  ```json
  { "v":1, "type":"Auth", "data":{"username":"user","proof":"<opaque>"} }
  ```

* `Open`

  ```json
  { "v":1, "type":"Open", "data":{"path":"src/main.rs"} }
  ```

* `Key`

  ```json
  { "v":1, "type":"Key", "data":{"code":"Char","ch":"a","mods":["Ctrl","Alt","Shift"],"ts":1723371000} }
  ```

  `code` enum: `Char`, `Enter`, `Esc`, `Tab`, `Backspace`, `Del`, `Left`, `Right`, `Up`, `Down`, `Home`, `End`, `PgUp`, `PgDn`, `F1..F12`.
  For copy/cut/paste, client sends corresponding high-level commands (below) in addition to raw keys when appropriate.

* High-level commands (server interprets):

  * `Insert` *(mutating; carries `seq`)*

    ```json
    { "v":1,"type":"Insert","data":{"pos":12345,"text":"x","seq":42} }
    ```
  * `Delete` *(mutating)*

    ```json
    { "v":1,"type":"Delete","data":{"range":{"from":120,"to":121},"seq":43} }
    ```
  * `Move`, `Select`, `Scroll`, `Resize`, `Search`, `GotoLine`, `DuplicateLine`, `DeleteLine`, `Save`, `ClosePicker`, `PickerAction` (create/rename/delete/expand/collapse).

* `RequestFrame`

  ```json
  { "v":1,"type":"RequestFrame","data":{"reason":"initial|resize|scroll|dirty"} }
  ```

* `Ping`

  ```json
  { "v":1,"type":"Ping","data":{"nonce":123456} }
  ```

### 8.2 Server → Client

* `Ack`

  ```json
  { "v":1,"type":"Ack","data":{"seq":42,"doc_v":918} }
  ```

* `Frame` (editor)

  ```json
  {
    "v":1,"type":"Frame",
    "data":{
      "id":"main","kind":"editor","doc_v":918,
      "first_line":100,"cols":120,"rows":40,
      "lines":[
        {"text":"fn main() {","spans":[[0,2,"kw"]]},
        {"text":"    println!(\"hi\");","spans":[[4,11,"fn"],[12,18,"str"]]}
      ],
      "cursors":[{"line":101,"col":16}],
      "selections":[{"start":{"line":101,"col":3},"end":{"line":101,"col":16}}],
      "status":{"left":"main.rs  UTF-8","right":"Ln 102, Col 17  RO:false  doc_v:918"}
    }
  }
  ```

* `Frame` (picker)

  ```json
  {
    "v":1,"type":"Frame",
    "data":{
      "id":"picker","kind":"picker",
      "query":"src/ma","breadcrumbs":["/","project","src"],
      "tree":[{"path":"src","expanded":true,"children":[{"path":"src/main.rs","match":[0,1,2]}]}],
      "preview":{"lines":["fn main() {", "  ..."],"path":"src/main.rs"},
      "status":{"left":"Picker","right":"Enter:Open  Ctrl+N:New  Ctrl+D:Delete"}
    }
  }
  ```

* `Dirty`

  ```json
  { "v":1,"type":"Dirty","data":{"ranges":[{"from_line":100,"to_line":120}],"doc_v":919} }
  ```

* `Status`

  ```json
  { "v":1,"type":"Status","data":{"level":"info|warn|error","msg":"Saved main.rs"} }
  ```

* `Dialog`

  ```json
  { "v":1,"type":"Dialog","data":{"title":"Conflict","body":"File changed on disk.","options":["Reload","Keep mine"]} }
  ```

* `Error`

  ```json
  { "v":1,"type":"Error","data":{"code":"UNAUTHORIZED|INVALID|UNSUPPORTED|BUSY|IO|SANDBOX","msg":"...","detail":{}} }
  ```

* `Pong`

  ```json
  { "v":1,"type":"Pong","data":{"nonce":123456} }
  ```

**General rules**

* Unknown fields are ignored (forward compatible).
* Server MAY coalesce multiple edits into a single frame.
* Mutating requests without `seq` MUST be rejected (`INVALID`).

---

## 9. Data Models & Persistence

### 9.1 Buffer

* **Primary:** Rope (UTF-8).
* **Large files path:** Piece-table with:

  * **Original:** memory-mapped read-only file.
  * **Add buffer:** append-only scratch file in workspace `.ghostwriter/`.
* Indexes:

  * Line index (chunked), updated incrementally.
  * Word boundary cache per page (optional).

### 9.2 WAL Format

* File: `.ghostwriter/<doc_id>.wal` next to target file (same filesystem).
* Record:

  ```
  +---------+---------+-----------+---------+---------+-----------+
  | Magic   | Version | DocV_BE64 | Type    | Length  | Payload   |
  | GWAL    |   01    |           | u8      | BE32    | bytes     |
  +---------+---------+-----------+---------+---------+-----------+
  | CRC32_BE (over Type..Payload)                                  |
  +----------------------------------------------------------------+
  ```

  Types: INSERT, DELETE, REPLACE, META (EOL, encoding), SNAPSHOT (optional).
* On startup: replay WAL in order; discard records with invalid CRC; finalize to latest consistent state, then truncate/compact if size exceeds threshold.

### 9.3 File Lock

* Lock file `.ghostwriter/<file>.lock` with PID and start time; `O_EXCL` semantics. Stale lock detection via PID liveness check; auto-clean on disconnect.

---

## 10. File System Rules

* **Canonicalization algorithm (required):**

  1. Resolve path relative to workspace root.
  2. `realpath()`/equivalent to resolve symlinks.
  3. Verify the resulting path has the workspace root as prefix boundary.
  4. Reject otherwise with `SANDBOX`.

* **Atomic writes:** temp name pattern: `.<name>.gw.tmp.<pid>.<nonce>` in the target directory.

* **External changes detection:** inotify/kqueue watcher; fallback to periodic stat (1 s).

---

## 11. Search

* Regex engine with literal fast path; case sensitivity flag; whole-word flag.
* Incremental search:

  * Server maintains current query and match iterator.
  * Frame includes highlights for visible matches only.
  * `Find Next/Prev` moves cursor; selection updates accordingly.

Performance: streaming over rope/piece-table chunks; p95 first match `< 50 ms` in 100 MB file.

---

## 12. Hex Viewer (Read-Only)

* Triggered automatically if file contains invalid UTF-8 and user declines UTF-8 view.
* Layout: left address column, 16 bytes per row, ASCII gutter (printable, dot otherwise).
* Navigation: same as text; no edits permitted.

---

## 13. Terminal UI Specification

* **Layout (editor):**

  * Title/status line (1 row) + text viewport.
  * Status right segment MUST include line/column, RO flag, `doc_v`.

* **Style spans:**

  * Span: `[start_col, end_col, "class"]`.
  * Classes v1: `"kw"|"fn"|"str"|"num"|"cm"|"id"|"op"|"ws"|"err"|"sel"`.
  * Color mapping by server based on terminal capabilities; client prints provided escape sequences inline or via pre-sliced attributes.

* **Wrapping:**

  * v1.0: No soft wrap (long lines scroll horizontally); horizontal scroll controlled via `Scroll{dx}` (dx MAY be negative).

---

## 14. Error Codes

| Code         | Meaning                                     | Client Action                    |
| ------------ | ------------------------------------------- | -------------------------------- |
| UNAUTHORIZED | Auth required or failed                     | Prompt user; retry/backoff       |
| INVALID      | Malformed message or missing fields         | Log; correct bug; show error     |
| UNSUPPORTED  | Feature not supported in this version       | Inform user                      |
| BUSY         | Server already has an active client         | Inform user; retry later         |
| IO           | Filesystem error                            | Show dialog; suggest RO fallback |
| SANDBOX      | Path escapes workspace or symlink violation | Show dialog                      |
| CONFLICT     | On-disk change vs in-memory                 | Present conflict dialog (§F3)    |
| RATE\_LIMIT  | Too many auth attempts                      | Backoff                          |

---

## 15. Security Model

* **Authentication:** Optional shared secret. Server stores Argon2id hash: `m=64MB, t≈3, p=1` (t tuned to \~250 ms).
* **Rate limiting:** Fixed window per IP/UDS peer: 3 attempts/minute.
* **Transport security:** TLS via rustls optional; when disabled, users SHOULD tunnel via SSH.
* **Sandboxing:** Landlock (Linux) enabled when kernel supports; otherwise fd-based path discipline.
* **Logging:** Metadata only; redact secrets; rotate logs daily or at 10 MB.

---

## 16. Observability

* **Metrics (Prometheus-style or internal counters):**

  * `gw_edit_apply_ms{p50,p95,p99}`
  * `gw_frame_push_ms{p50,p95,p99}`
  * `gw_saves_total`, `gw_save_fail_total`
  * `gw_reconnects_total`, `gw_queue_drops_total`
* **Structured logs:** JSON lines with `ts`, `level`, `event`, `fields`.
* **Audit:** `open`, `save`, `delete`, `rename`, `create`, `auth_result`.

---

## 17. Compatibility Matrix

| Component | Linux x86\_64 | Linux ARM64 | macOS ARM64 |
| --------- | ------------- | ----------- | ----------- |
| Server    | Yes           | Yes         | Yes         |
| Client    | Yes           | Yes         | Yes         |

Terminal capability:

* Truecolor: preferred.
* 256 color: supported.
* <256 color: degrade styles to mono emphasis.

---

## 18. Key Bindings

**Global**

* `Ctrl+Q`: Quit (server saves/cleans).
* `Ctrl+O`: Open picker overlay.
* `Ctrl+H`: File history back/forward dialog.

**Editing**

* `Ctrl+Z`/`Ctrl+Y`: Undo/Redo
* `Ctrl+F`: Find
* `Ctrl+G`: Go to line
* `Ctrl+A`: Select all
* `Ctrl+C/X/V`: Copy/Cut/Paste

**Navigation**

* `Alt+Up/Home`: Start of doc
* `Alt+Down/End`: End of doc
* `Alt+Left/Right`: Prev/Next word boundary
* `Home/End`: Line start/end
* `Ctrl+Left/Right`: Line start/end (alias)
* `PgUp/PgDn`: Scroll by screen height

**Selections:** Add `Shift` to any navigation.

**Picker**

* `Esc`: Close
* `Enter`: Open
* `Ctrl+N`: New file
* `Ctrl+Shift+N`: New folder
* `Ctrl+R`: Rename
* `Ctrl+D`: Delete
* `Space`: Expand/collapse
* `Tab`: Toggle tree/preview focus

---

## 19. Acceptance Tests (High Level)

1. **Startup (local):** Start with file path; first frame within 50 ms.
2. **Large file navigation:** Open 1 GB file; page down repeatedly; p95 paging `< 100 ms`.
3. **Keystroke latency:** Type 1,000 chars at 120 wpm; p95 char echo `< 100 ms` remote.
4. **Crash safety:** Kill server during burst edits; restart; contents reflect all edits up to last fsync or WAL replay; no corruption.
5. **External change:** Modify file on disk; within 1 s client shows conflict dialog; choosing “Reload” updates viewport.
6. **Sandbox:** Attempt `../outside` path open; receive `SANDBOX` error and no file operation performed.
7. **Picker performance:** Index 10k files; typing “src/ma” updates matches `< 10 ms` per keystroke.
8. **RO fallback:** Open locked file; server presents RO mode; edits refused with status message.
9. **Auth rate limit:** 4th wrong password within a minute returns `RATE_LIMIT`; further attempts blocked until window resets.
10. **Single-user:** Second client connect attempt receives `BUSY` with explanation.
11. **Reconnect:** Disconnect network for 3 s mid-typing; client queues ≤256 events; reconnect applies in-order; no data loss; overflow shows warning if induced.

---

## 20. Performance Benchmarks (Methodology)

* **Reference hardware:** 4-core 3.0 GHz CPU, NVMe SSD, 16 GB RAM.
* **Datasets:** Synthetic 1 GB text (80-char lines), real code repositories (≥ 100k LOC).
* **Measurements:** Wall time for apply, frame compose, frame transmit; memory RSS excluding mmaps.
* **Tools:** Built-in benchmarking mode (`--bench`) running scripted scenarios; outputs JSON report.

---

## 21. Build & Packaging

* **Language:** Rust (stable).
* **Binary:** Single static-linked binary when feasible; `panic=abort`, LTO, `codegen-units=1`, symbols stripped.
* **Feature gates:** `tls`, `landlock`, `largefile` (piece-table path), `truecolor`.
* **Binary size target:** `< 8 MB` (without TLS certs).

---

## 22. Deployment

* **Server:** foreground process or systemd service (sample unit provided).
* **TLS:** Provide cert/key paths or terminate TLS at a reverse proxy.
* **Remote usage:** Recommended over SSH tunnel if TLS disabled.

---

## 23. Upgrade & Compatibility

* **Protocol:** `v` field governs compatibility. Minor additions MUST be backward compatible. Breaking changes require new `v`.
* **WAL:** Versioned; migration tool MUST be provided if format changes.

---

## 24. Risks & Mitigations

* **Large-file performance:** Use piece-table + mmap; incremental indexes; avoid full-line re-tokenization.
* **Terminal variance:** Conservative ANSI; detect capabilities on `Hello`.
* **Binary bloat:** Feature-gate heavy deps; strip; avoid large static data (e.g., tree-sitter grammars) in v1.0.

---

## 25. Glossary

* **Actor:** concurrency unit encapsulating buffer mutations.
* **Viewport:** slice of the buffer with styles and cursors sized to terminal.
* **Idempotency:** repeated delivery of a request has no additional effect (via `seq`).

---

## 26. Compliance Checklist (Go/No-Go)

* [ ] All F1–F9 acceptance tests pass.
* [ ] Non-functional targets met on reference hardware.
* [ ] Security checks: sandbox, auth, rate limiting, audit log verified.
* [ ] Protocol conformance tests green.
* [ ] Crash-recovery and external-change test green.
* [ ] Packaging and cross-platform builds produced and smoke-tested.

---

## 27. Appendices

### A. Fuzzy Filter Scoring (Picker)

* Algorithm: fzy-style subsequence match with bonuses (consecutive, beginning of word, path separators).
* Score range normalized to `[0,1]`.
* Tie-breakers: shorter path length, fewer directory components, lexicographic.

### B. Word/Boundary Rules

* Grapheme segmentation per UAX#29.
* Words for nav: sequences of `XID_Continue` or ASCII alnum; punctuation splits.

### C. Keepalive & Timeouts

* Client sends `Ping` every 60 s with `nonce`.
* If no `Pong` within 15 s, client begins reconnect backoff.
* Server drops idle connection after 90 s without any message.

### D. Conflict Resolution Flow

1. Detect on-disk mtime/size/inode change or content hash mismatch.
2. Present `Dialog{Reload|Keep mine}`.
3. `Reload`: discard in-memory changes since last save; WAL truncated accordingly.
4. `Keep mine`: continue; future saves overwrite on disk (no auto-merge in v1.0).

---

This specification is complete for v1.0. If you want, I can generate a matching starter repo layout and the exact Rust type definitions for the protocol messages in `serde` to bootstrap implementation.
