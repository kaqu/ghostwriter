# Ghostwriter MVP Completion Plan

## Goal
Deliver a minimal yet usable terminal text editor. The editor must open, edit, and save local files, search within buffers, manage multiple files, and handle binary data.

## 1. Basic Editing and Navigation
**Objective:** Provide fundamental text editing features and responsive cursor movement.

**Implementation Details:**
- Handle character insertion and deletion within the buffer.
- Implement cursor movement for arrow, Home/End, and PageUp/PageDown keys.
- Support word, line, and document navigation shortcuts (e.g. `Ctrl+Arrow`, `Ctrl+Home/End`).
- Scroll the viewport when the cursor moves beyond visible bounds.

**Tests to Write:**
```rust
#[test]
fn cursor_moves_across_words_and_lines() {
    // simulate navigation commands and assert final position
}
```

**Verification Criteria:**
- Cursor movement matches expected positions and view scrolls accordingly.
- Insert and delete operations update the buffer without corruption.

## 2. Selection and Line Operations
**Objective:** Allow selecting text and manipulating entire lines.

**Implementation Details:**
- Use Shift modifiers with navigation keys to create a selection.
- Implement cut/copy/delete operations on the selection.
- Add line commands: duplicate (`Ctrl+D`), delete (`Ctrl+Shift+K`), and go to line (`Ctrl+G`).

**Tests to Write:**
```rust
#[test]
fn line_duplicate_and_delete_work() {
    // duplicate and remove lines, verify buffer content
}
```

**Verification Criteria:**
- Selection highlights correctly and operations affect only the selected region.
- Line commands modify the buffer as expected.

## 3. File Loading and Saving
**Objective:** Persist file contents and track dirty state.

**Implementation Details:**
- Map files into memory on open and mark buffer dirty on edits.
- Implement `Ctrl+S` to write changes back to disk.
- Auto-save or prompt on exit when the buffer is dirty.

**Tests to Write:**
```rust
#[test]
fn save_writes_buffer_to_disk() {
    // edit temp file, invoke save, reload to confirm changes
}
```

**Verification Criteria:**
- Dirty flag reflects unsaved changes.
- Saving updates the file without data loss.

## 4. Undo/Redo Integration
**Objective:** Connect editing operations to an `UndoStack` with standard shortcuts.

**Implementation Details:**
- Store an `UndoStack` in `App` and record inserts/deletes from `KeyHandler`.
- Bind `Ctrl+Z`/`Ctrl+Y` for undo/redo operations.
- Clear the stack when a new file is opened.

**Tests to Write:**
```rust
#[test]
fn undo_redo_shortcuts_work() {
    // simulate typing then undo and redo via key events
}
```

**Verification Criteria:**
- Editing operations are reversible and cursor position is restored.
- No history corruption after repeated undo/redo.

## 5. Clipboard Copy/Cut/Paste
**Objective:** Support copy, cut, and paste using the system clipboard.

**Implementation Details:**
- Integrate a cross-platform clipboard crate (e.g. `copypasta`).
- Implement `Ctrl+C`, `Ctrl+X`, and `Ctrl+V` handling in `KeyHandler`.
- Use a clipboard trait to allow mocking in tests.

**Tests to Write:**
```rust
#[test]
fn clipboard_round_trip() {
    // select text, copy, delete, then paste
}
```

**Verification Criteria:**
- Selected text is placed on the clipboard.
- Paste inserts clipboard contents at the cursor.
- Clipboard interactions work on Linux and macOS.

## 6. Incremental Search Overlay
**Objective:** Find text within the buffer with live highlighting.

**Implementation Details:**
- Add search overlay triggered by `Ctrl+F` that updates results as the user types.
- Highlight all matches and jump between them with `Enter`/`Shift+Enter`.
- Exit search with `Esc` while preserving the last query.

**Tests to Write:**
```rust
#[test]
fn search_moves_cursor_to_match() {
    // open buffer, perform search, assert cursor lands on match
}
```

**Verification Criteria:**
- Search overlay accepts input and updates matches in real time.
- Cursor jumps correctly to next/previous match.

## 7. File Picker and File Switching
**Objective:** Open additional files without restarting the editor.

**Implementation Details:**
- Show `FilePicker` overlay with `Ctrl+O`.
- Prompt to save if the current buffer is dirty before switching files.
- Refresh editor state and undo history after opening a new file.

**Tests to Write:**
```rust
#[test]
fn switching_files_updates_buffer() {
    // modify file A, open file B via picker, ensure buffer shows file B
}
```

**Verification Criteria:**
- Multiple files can be opened sequentially.
- Unsaved changes are not lost when switching.

## 8. Hex View for Binary Files
**Objective:** Display binary files in a readable hex dump.

**Implementation Details:**
- Detect binary files with `FileManager::is_binary`.
- Initialize `HexView` and allow toggling between text and hex with a key (e.g. `Ctrl+H`).
- Cursor movement should navigate by bytes in hex mode.

**Tests to Write:**
```rust
#[test]
fn binary_file_opens_in_hex_mode() {
    // open binary file and verify hex lines are rendered
}
```

**Verification Criteria:**
- Hex view shows offsets, bytes, and ASCII column.
- Toggling returns to normal text view without state loss.

## 9. Status Bar and Mode Reporting
**Objective:** Keep the user informed about editor state.

**Implementation Details:**
- Extend `StatusBar` to show current mode (edit, search, picker, hex) and dirty flag.
- Update status bar whenever mode changes.

**Tests to Write:**
```rust
#[test]
fn status_bar_updates_on_mode_change() {
    // switch to search mode and verify status text
}
```

**Verification Criteria:**
- Status bar always reflects active mode and unsaved status.

## 10. Integration and QA
**Objective:** Ensure cohesive behavior across all features.

**Implementation Details:**
- Centralize keybindings so shortcuts do not conflict.
- Add integration tests covering edit → undo → save flow and file switching after search.
- Perform manual smoke tests on both text and binary files.

**Verification Criteria:**
- All tests pass and a manual session exercises every feature without errors.
- Documentation reflects implemented shortcuts and modes.

## Final Acceptance Checklist
- [ ] Basic editing and navigation work smoothly with scrolling.
- [ ] Selection and line operations behave correctly.
- [ ] Files open and save with accurate dirty tracking.
- [ ] Undo/redo shortcuts work for all edits.
- [ ] Copy, cut, and paste operate through the clipboard.
- [ ] Search overlay highlights matches and navigates correctly.
- [ ] Users can switch files via the picker without data loss.
- [ ] Binary files render in hex mode with toggle support.
- [ ] Status bar displays accurate state information.
- [ ] End-to-end integration tests and manual QA confirm stability.
