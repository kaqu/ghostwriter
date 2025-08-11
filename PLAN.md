# Ghostwriter Zig Migration Plan

## Goal
Port the existing Rust prototype of Ghostwriter to Zig while preserving core features and ensuring cross-platform performance. Development follows a test-driven approach; each task lists the tests required to validate it.

## Phase 0: Toolchain & Scaffolding
**Objective:** Establish a minimal Zig project.

**Implementation Details:**
- Initialize `build.zig` with executable target.
- Add `src/main.zig` and placeholder modules.
- Integrate `zig fmt` and `zig test` into CI.

**Tests to Write:**
```zig
const std = @import("std");

test "build pipeline works" {
    try std.testing.expect(true);
}
```

**Verification Criteria:**
- `zig fmt` produces no diff.
- `zig build test` runs sample test successfully.

---

## Phase 1: Core Editor Engine
**Objective:** Implement text buffer and undo/redo system.

**Implementation Details:**
- Create `buffer.zig` using gap buffer or rope structure.
- Provide insertion, deletion, cursor movement, and selection APIs.
- Implement linear undo/redo stack with configurable depth.

**Tests to Write:**
```zig
test "insert and delete" { /* verify buffer mutations */ }

test "undo redo cycle" { /* ensure operations revert correctly */ }

test "selection across lines" { /* multi-line handling */ }
```

**Verification Criteria:**
- Buffer operations modify text precisely.
- Undo/redo restores previous state including cursor.
- Selections span multiple lines correctly.

---

## Phase 2: Terminal User Interface
**Objective:** Render text and handle input in the terminal.

**Implementation Details:**
- Use Zig's standard library for terminal control or bind to `crossterm` via C.
- Implement event loop for keyboard navigation and editing commands.
- Display status line and basic highlighting.

**Tests to Write:**
```zig
test "cursor moves with arrow keys" { /* simulate key events */ }

test "screen redraw" { /* verify rendering API updates buffer */ }
```

**Verification Criteria:**
- UI responds to key events with <10ms latency in benchmarks.
- Cursor and screen state remain consistent after edits.

---

## Phase 3: File System & Persistence
**Objective:** Add file loading, saving, and workspace sandboxing.

**Implementation Details:**
- Implement `file.zig` for atomic reads/writes.
- Support memory mapping for files >100MB.
- Enforce working directory sandbox with path validation.

**Tests to Write:**
```zig
test "atomic save" { /* writes via temp + rename */ }

test "binary file detection" { /* null byte heuristic */ }
```

**Verification Criteria:**
- Small files load into memory; large files use mapping.
- Binary files trigger hex view path.
- Sandbox prevents path traversal outside workspace.

---

## Phase 4: Networking (Optional)
**Objective:** Provide optional client/server mode for remote editing.

**Implementation Details:**
- Use Zig networking APIs or WebSocket library for sync.
- Single-client server enforcing authentication key.
- Delta sync at keystroke granularity.

**Tests to Write:**
```zig
test "remote connection" { /* client connects and edits */ }

test "auth required" { /* rejected without key */ }
```

**Verification Criteria:**
- Connection latency <200ms on local network.
- Unauthorized clients cannot modify files.

---

## Phase 5: Cross-Platform Packaging
**Objective:** Deliver static binaries for Linux x86_64, Linux ARM64, and macOS ARM64.

**Implementation Details:**
- Configure build targets in `build.zig`.
- Ensure no dynamic library dependencies.
- Provide startup scripts and installation instructions.

**Tests to Write:**
```bash
zig build -Dtarget=x86_64-linux-gnu
zig build -Dtarget=aarch64-linux-gnu
zig build -Dtarget=aarch64-macos
```

**Verification Criteria:**
- Binaries run on target platforms with identical behavior.
- Startup time <50ms for empty workspace.

---

## Final Verification
- All unit tests pass: `zig build test`.
- Benchmarks meet performance targets (keystroke latency <10ms).
- Security review confirms sandboxing and file locking.
- Documentation updated for Zig usage.

This plan guides the migration from Rust to Zig while maintaining high quality through TDD and clear acceptance tests for each milestone.
