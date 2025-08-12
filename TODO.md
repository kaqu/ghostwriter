# Phase 0 — Bootstrap & Plumbing

* [x] **Create workspace & crates** — root + `crates/{proto,core,server,client}`; set release flags (LTO, panic=abort).
* [x] **CLI scaffold** — clap parser, mode enum (local / `--server` / `--connect`) and logging init.
* [x] **Protocol envelope** — `Envelope{v,type,data}` + enums; MsgPack encode/decode helpers.
* [x] **Transport shim** — WS binary send/recv wrappers (tungstenite) + `Ping/Pong` timer.
* [x] **Dev tooling** — rustfmt, clippy, test harness, basic GitHub Actions CI (build/test).

---

# Phase 1 — Core Editor (Local In-Proc Loop)

* [x] **RopeBuffer (read/open)** — load file with UTF-8 + invalid-byte tracking (hex fallback flag).
* [x] **RopeBuffer (edit ops)** — `insert/delete`, byte↔line/col, grapheme left/right.
* [x] **Undo/Redo stack** — linear history, coalescing adjacent inserts.
* [x] **Viewport composer** — slice by lines, minimal style spans, status line, cursor(s).
* [x] **Atomic save** — temp+rename+fsync(dir); preserve EOL; configurable debounce (100ms).
* [x] **WAL writer/reader** — append before apply; CRC; replay on start; compaction threshold.
* [x] **Minimal session actor** — holds buffer, doc\_v, selection, debounce; emits Frames.
* [x] **TUI bootstrap (ratatui)** — raw mode, draw frame, status, cursor placement.
* [x] **Key→command mapping** — translate keystrokes to `Insert/Delete/Move/Select/...`.
* [ ] **Local loop glue** — in-proc channels client↔session (no WS yet); open/save workflow.
* [ ] **Hex viewer (RO)** — 16B/row, ASCII gutter; auto-trigger on invalid UTF-8.
* [ ] **Acceptance pack #1** — tests for edit/undo/save/WAL; crash-replay works.

---

# Phase 2 — Real Server & Single-User Control

* [ ] **WS acceptor (TCP/UDS)** — serve one active session; reject others with `Busy`.
* [ ] **Client WS connector** — `Hello` handshake, `RequestFrame` on connect/resize.
* [ ] **Auth (optional)** — Argon2id storage file, login flow, shared-secret env/CLI.
* [ ] **Rate limiting** — 3/min per peer; error `RateLimit`; backoff hints.
* [ ] **Session lifecycle** — clean shutdown, lock release, save on exit; server banner/status.
* [ ] **Logging & audit** — JSON logs + audit of open/save/rename/delete/auth (no contents).
* [ ] **Acceptance pack #2** — second client blocked; wrong password thrice ⇒ rate-limit.

---

# Phase 3 — Picker & Filesystem

* [ ] **Workspace scan** — tree index (ignore `.git`), flat path list, breadcrumbs.
* [ ] **Fuzzy scorer** — fzy-style subsequence match + bonuses; stable sort/ties.
* [ ] **Picker frames** — tree + matches + preview(first N lines); dedicated status hints.
* [ ] **Picker actions** — create file/folder, rename, delete, expand/collapse.
* [ ] **File history** — back/forward stack, branching on new open; restore cursor/scroll.
* [ ] **Sandboxing core** — canonicalize, prefix guard, `O_NOFOLLOW`; reject escapes.
* [ ] **Read-only fallback** — lock acquisition; RO banner; edits blocked with status.
* [ ] **Acceptance pack #3** — 10k files perf (<10ms/keypress), sandbox tests, picker ops.

---

# Phase 4 — Durability, Watching & Resilience

* [ ] **Debounced saver** — per-buffer timer; coalesce edits; explicit `Save` command.
* [ ] **External change watch** — inotify/kqueue; hash/mtime guard; suppress self-saves.
* [ ] **Conflict dialog** — `Reload | Keep mine`; WAL truncate on reload; status feedback.
* [ ] **Disk-full/IO errors** — classify `IO` errors; non-blocking dialogs; RO fallback path.
* [ ] **Bounded client queue** — 256 events; overflow warning; drop oldest.
* [ ] **Reconnect/backoff** — 250ms→5s; resume window (30s); re-Hello/resize/frame.
* [ ] **Idempotent mutations** — `seq` dedupe; replay safe; `Ack{seq,doc_v}` path.
* [ ] **Acceptance pack #4** — yank cable tests, queue behavior, conflict flow.

---

# Phase 5 — Large Files & Perf

* [ ] **Piece-table add-buffer** — mmap original RO + append-only scratch; swap for ≥100MB.
* [ ] **Line index cache** — chunked line starts; invalidate minimally on edits.
* [ ] **Dirty-line tracking** — compute affected line range; incremental frame compose.
* [ ] **Word/grapheme cache** — per-line cache for nav; invalidate on edits to line.
* [ ] **Pager tuning** — page up/down O(view) latency; h-scroll support.
* [ ] **Bench harness** — scripted scenarios; p95 metrics for apply/compose/page.
* [ ] **Acceptance pack #5** — 1GB file: p95 page <100ms; mem <100MB (excl. mmaps).

---

# Phase 6 — Security Hardening & TLS

* [ ] **Auth file mgmt** — create/update `.ghostwriter/auth.json`; argon params + rotation.
* [ ] **TLS (optional)** — rustls server/client, cert/key flags; ALPN; doc SSH-tunnel alt.
* [ ] **Landlock (opt-in)** — feature-gated enablement; graceful fallback.
* [ ] **Audit log rotation** — size/day rotation; retention policy.
* [ ] **Security tests** — symlink races, traversal attempts, lock stealing, RO enforcement.

---

# Phase 7 — UX Polish & Status

* [ ] **Status line details** — Ln/Col, RO, doc\_v, file name, encoding/EOL.
* [ ] **Search UX** — incremental highlights in view; next/prev; flags (case/word).
* [ ] **Dialogs & errors** — consistent framing, keyboard navigation, non-blocking.
* [ ] **Truecolor/256 fallback** — detect on `Hello`; palette map; NO\_COLOR support.
* [ ] **Help screen** — fixed keymap overlay (`?`); quick hints in picker.

---

# Phase 8 — Packaging, Docs & CI

* [ ] **Release profiles** — features: `default=["largefile","truecolor"]`, opt `tls,landlock`.
* [ ] **Cross-builds** — Linux x86\_64/ARM64 (musl), macOS ARM64; strip; size check (<8MB).
* [ ] **Smoke tests** — scripted runs (open/edit/save/picker/remote) on all targets.
* [ ] **User docs** — README, quickstart, CLI flags, SSH tunnel guide, safety notes.
* [ ] **Issue templates** — bug/perf/security templates; contribution guide.

---

# Phase 9 — Final Verification

* [ ] **Full PRD acceptance run** — execute all packs #1–#5 + security + perf gates.
* [ ] **Crash/replay soak** — automated kill-9 during edits loop; zero-corruption check.
* [ ] **Single-user invariants** — busy state across reconnect storms; correct cleanup.
* [ ] **Telemetry sanity** — counters/logs sane under load; no PII content logged.
