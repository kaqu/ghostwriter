# Ghostwriter - Complete Implementation Plan

## Phase 1: Project Foundation

### 1.1 Project Setup and Structure
**Objective**: Create Rust project with proper structure and dependencies

**Implementation Details**:
- Initialize Cargo project: `cargo init ghostwriter`
- Set up `Cargo.toml` with dependencies: `ratatui`, `crossterm`, `tokio`, `clap`, etc.
- Create module structure: `src/{main.rs, app/, editor/, files/, ui/, network/}`
- Configure static linking in `Cargo.toml` profile settings
- Set up cross-compilation targets for Linux x86_64/ARM64, macOS ARM64

**Tests to Write**:
```rust
#[test]
fn test_project_compiles() {
    // Ensure project compiles without warnings
}

#[test]
fn test_dependencies_load() {
    // Import all major dependencies without errors
}
```

**Verification Criteria**:
- Project compiles successfully with `cargo build`
- All target platforms can be cross-compiled
- Module structure is accessible
- No dependency conflicts

---

### 1.2 Command Line Interface
**Objective**: Implement CLI argument parsing with clap

**Implementation Details**:
- Define `Args` struct with `clap` derive macros
- Support modes: local file, local directory, server, client
- Handle optional authentication key parameter
- Implement server binding and port options
- Add validation for file/directory paths

**Tests to Write**:
```rust
#[test]
fn test_parse_local_file_args() {
    // Test: ghostwriter /path/to/file.txt
}

#[test]
fn test_parse_server_args() {
    // Test: ghostwriter --server /workspace --port 8080 --key "secret"
}

#[test]
fn test_parse_client_args() {
    // Test: ghostwriter --connect ws://server:8080 --key "secret"
}

#[test]
fn test_invalid_args_rejected() {
    // Test conflicting arguments are rejected
}
```

**Verification Criteria**:
- All argument combinations parse correctly
- Invalid combinations show helpful error messages
- Help text displays properly
- Path validation works for file/directory arguments

---

### 1.3 Basic Error Handling System
**Objective**: Create comprehensive error types and handling

**Implementation Details**:
- Define `GhostwriterError` enum covering all error scenarios
- Implement `From` traits for converting external errors
- Create error context system for debugging
- Implement user-friendly error display
- Add error logging infrastructure

**Tests to Write**:
```rust
#[test]
fn test_file_not_found_error() {
    // Test error creation and display for missing files
}

#[test]
fn test_permission_denied_error() {
    // Test error handling for permission issues
}

#[test]
fn test_network_error_conversion() {
    // Test WebSocket errors convert properly
}
```

**Verification Criteria**:
- All error types implement proper Display messages
- Error chain preserves context through conversions
- User sees helpful error messages, not debug output
- Critical errors are logged appropriately

---

## Phase 2: Core Text Editing Engine

### 2.1 Rope Data Structure Implementation
**Objective**: Build efficient text buffer using rope data structure

**Implementation Details**:
- Implement `Rope` struct with tree-based text storage
- Support operations: insert, delete, slice, char_at, line_at
- Optimize for files up to 1GB with chunked nodes (64KB chunks)
- Implement iterator for chars, lines, and byte ranges
- Add UTF-8 validation and hex fallback for invalid sequences

**Tests to Write**:
```rust
#[test]
fn test_rope_insert_at_position() {
    // Test inserting text at various positions
}

#[test]
fn test_rope_delete_range() {
    // Test deleting text ranges
}

#[test]
fn test_rope_line_operations() {
    // Test line counting and line-based access
}

#[test]
fn test_rope_large_file_performance() {
    // Test with 100MB+ text data
}

#[test]
fn test_utf8_invalid_sequences() {
    // Test hex fallback for invalid UTF-8 bytes
}
```

**Verification Criteria**:
- Insert/delete operations complete in < 10ms for typical edits
- Memory usage scales efficiently with file size
- Line-based operations work correctly
- Invalid UTF-8 displays as hex without crashing

---

### 2.2 Cursor Management System
**Objective**: Implement cursor positioning and movement logic

**Implementation Details**:
- Create `Cursor` struct with line/column position
- Implement all navigation operations (word, line, document boundaries)
- Handle line ending conversions (LF, CRLF)
- Implement word boundary detection (alphanumeric + underscore)
- Add cursor validation to prevent out-of-bounds positions

**Tests to Write**:
```rust
#[test]
fn test_cursor_word_navigation() {
    // Test Alt+Left/Right word movement
}

#[test]
fn test_cursor_line_boundaries() {
    // Test Home/End and Ctrl+Left/Right
}

#[test]
fn test_cursor_document_boundaries() {
    // Test Alt+Up/Down to document start/end
}

#[test]
fn test_cursor_validation() {
    // Test cursor stays within valid bounds
}
```

**Verification Criteria**:
- All navigation commands work as specified in key bindings
- Cursor position remains valid after text modifications
- Word boundaries detected correctly for various text types
- Performance remains fast for large files

---

### 2.3 Text Selection System
**Objective**: Implement text selection with Shift+navigation

**Implementation Details**:
- Create `Selection` struct with start/end positions
- Implement selection extension for all cursor movements
- Handle selection normalization (start ≤ end)
- Support operations on selected text (cut, copy, delete)
- Integrate with cursor navigation system

**Tests to Write**:
```rust
#[test]
fn test_selection_with_shift_navigation() {
    // Test Shift+Arrow combinations create selections
}

#[test]
fn test_selection_operations() {
    // Test cut, copy, delete on selected text
}

#[test]
fn test_selection_normalization() {
    // Test backward selections are normalized
}

#[test]
fn test_selection_across_lines() {
    // Test multi-line selections work correctly
}
```

**Verification Criteria**:
- Selection extends correctly with Shift+navigation
- Selected text operations modify buffer correctly
- Visual selection highlights work in UI
- Selection state updates properly after text changes

---

### 2.4 Undo/Redo System
**Objective**: Linear undo/redo for all text operations

**Implementation Details**:
- Create `UndoOperation` enum for different edit types
- Implement `UndoStack` with configurable size limit (default: 1000)
- Track cursor position changes with each operation
- Support compound operations (e.g., paste as single undo unit)
- Clear undo stack when switching files

**Tests to Write**:
```rust
#[test]
fn test_undo_insert_operation() {
    // Test undoing text insertion
}

#[test]
fn test_redo_after_undo() {
    // Test redo functionality
}

#[test]
fn test_undo_stack_limit() {
    // Test old operations are discarded
}

#[test]
fn test_cursor_position_restoration() {
    // Test cursor returns to correct position after undo
}
```

**Verification Criteria**:
- Undo/redo operations restore exact previous state
- Cursor position is correctly restored
- Memory usage bounded by stack size limit
- Compound operations treated as single undo unit

---

## Phase 3: File System Operations

### 3.1 File I/O with Memory Mapping
**Objective**: Efficient file reading/writing with large file support

**Implementation Details**:
- Implement `FileManager` with atomic write operations
- Use memory mapping for files > 100MB via `memmap2`
- Support UTF-8 detection with hex fallback
- Implement binary file detection (null byte presence)
- Create chunked reading for streaming large files

**Tests to Write**:
```rust
#[test]
fn test_atomic_file_write() {
    // Test writes are atomic (temp file + rename)
}

#[test]
fn test_memory_mapped_large_file() {
    // Test reading 500MB+ files
}

#[test]
fn test_binary_file_detection() {
    // Test binary vs text file classification
}

#[test]
fn test_utf8_hex_fallback() {
    // Test invalid UTF-8 sequences become hex
}
```

**Verification Criteria**:
- Small files (< 100MB) load completely into memory
- Large files use memory mapping efficiently
- Atomic writes prevent file corruption
- Binary files display as hex dump

---

### 3.2 File Locking System
**Objective**: Exclusive file locks with automatic cleanup

**Implementation Details**:
- Implement platform-specific file locking (fcntl/LockFile)
- Support single file lock per process maximum
- Automatic lock release on process exit/crash
- Lock acquisition timeout (5 seconds)
- Graceful fallback to read-only mode

**Tests to Write**:
```rust
#[test]
fn test_exclusive_file_lock() {
    // Test only one process can lock a file
}

#[test]
fn test_automatic_lock_release() {
    // Test locks released on process termination
}

#[test]
fn test_lock_timeout() {
    // Test lock acquisition timeout
}

#[test]
fn test_readonly_fallback() {
    // Test read-only mode when lock fails
}
```

**Verification Criteria**:
- Only one process can write-lock a file
- Locks are released when process terminates
- Lock conflicts result in read-only mode
- Lock acquisition completes within timeout

---

### 3.3 File Watching and External Changes
**Objective**: Detect external file modifications

**Implementation Details**:
- Use `notify` crate for cross-platform file watching
- Monitor currently locked file for external changes
- Implement conflict detection comparing modification times
- Show diff view for external changes
- Allow user choice: keep local, accept external, manual merge

**Tests to Write**:
```rust
#[test]
fn test_external_change_detection() {
    // Test file watcher detects external modifications
}

#[test]
fn test_conflict_resolution_options() {
    // Test user can choose conflict resolution
}

#[test]
fn test_modification_time_comparison() {
    // Test accurate conflict detection
}
```

**Verification Criteria**:
- External file changes detected within 1 second
- User notified of conflicts with clear options
- Conflict resolution preserves user choice
- No data loss during conflict resolution

---

### 3.4 Directory Operations and Sandboxing
**Objective**: Secure directory traversal within workspace

**Implementation Details**:
- Implement path canonicalization to prevent traversal attacks
- Create `WorkspaceManager` to enforce boundary restrictions
- Support directory listing with file metadata
- Implement file/folder creation, deletion, rename operations
- Add permission checking before operations

**Tests to Write**:
```rust
#[test]
fn test_path_canonicalization() {
    // Test "../" and symlinks are resolved safely
}

#[test]
fn test_workspace_boundary_enforcement() {
    // Test access outside workspace is blocked
}

#[test]
fn test_directory_operations() {
    // Test create, delete, rename files/folders
}

#[test]
fn test_permission_checking() {
    // Test operations respect file system permissions
}
```

**Verification Criteria**:
- No access possible outside defined workspace
- All path traversal attempts are blocked
- File operations work within workspace
- Permission errors are handled gracefully

---

## Phase 4: Terminal User Interface

### 4.1 Basic Terminal Setup
**Objective**: Initialize terminal interface with ratatui

**Implementation Details**:
- Set up `crossterm` for cross-platform terminal control
- Initialize `ratatui` terminal with raw mode
- Implement graceful cleanup on exit (restore terminal state)
- Handle terminal resize events
- Set up basic event loop structure

**Tests to Write**:
```rust
#[test]
fn test_terminal_initialization() {
    // Test terminal can be initialized and restored
}

#[test]
fn test_resize_handling() {
    // Test terminal resize events are handled
}

#[test]
fn test_graceful_cleanup() {
    // Test terminal state restored on exit
}
```

**Verification Criteria**:
- Terminal initializes without affecting other apps
- Resize events update UI layout properly
- Clean exit restores original terminal state
- Works across different terminal emulators

---

### 4.2 Text Editor Widget
**Objective**: Render text buffer with syntax highlighting

**Implementation Details**:
- Create `EditorWidget` using ratatui widget system
- Implement text rendering with line numbers
- Add cursor visualization and selection highlighting
- Support horizontal/vertical scrolling for large files
- Implement UTF-8 text rendering with hex fallback display

**Tests to Write**:
```rust
#[test]
fn test_text_rendering() {
    // Test text displays correctly in widget
}

#[test]
fn test_cursor_visualization() {
    // Test cursor appears at correct position
}

#[test]
fn test_selection_highlighting() {
    // Test selected text is visually highlighted
}

#[test]
fn test_scrolling_behavior() {
    // Test scrolling keeps cursor in view
}
```

**Verification Criteria**:
- Text renders correctly with proper line wrapping
- Cursor position is visually accurate
- Selected text is clearly highlighted
- Scrolling keeps edited content visible

---

### 4.3 Status Bar Implementation
**Objective**: Display file info and connection status

**Implementation Details**:
- Create `StatusBar` widget showing file path, cursor position
- Display file lock status (🔒 locked, 👁 read-only)
- Show connection status for remote mode (🟢🟡🔴)
- Add dirty indicator for unsaved changes
- Display current mode (local/remote)

**Tests to Write**:
```rust
#[test]
fn test_status_bar_file_info() {
    // Test file path and cursor position display
}

#[test]
fn test_lock_status_indicators() {
    // Test lock status icons appear correctly
}

#[test]
fn test_connection_status_display() {
    // Test network status indicators
}
```

**Verification Criteria**:
- File information is accurate and up-to-date
- Status indicators change appropriately
- Visual indicators are clear and consistent
- Status bar updates in real-time

---

### 4.4 File Picker Interface
**Objective**: Full-screen file picker with tree and search

**Implementation Details**:
- Create overlay `FilePicker` widget covering entire screen
- Implement tree view with expand/collapse functionality
- Add fuzzy search filtering that preserves directory structure
- Show file preview panel for selected files
- Support keyboard navigation and file operations

**Tests to Write**:
```rust
#[test]
fn test_file_picker_overlay() {
    // Test picker appears as full-screen overlay
}

#[test]
fn test_fuzzy_search_filtering() {
    // Test search filters tree while preserving structure
}

#[test]
fn test_file_preview() {
    // Test selected file preview displays correctly
}

#[test]
fn test_keyboard_navigation() {
    // Test all file picker key bindings work
}
```

**Verification Criteria**:
- File picker covers entire screen when active
- Search filtering works instantly as user types
- File preview shows first 10 lines of selected file
- All keyboard shortcuts work as specified

---

### 4.5 Key Event Handling
**Objective**: Process all keyboard input according to key bindings

**Implementation Details**:
- Implement `KeyHandler` to route events based on current mode
- Support all navigation key combinations (Alt+arrows, Ctrl+arrows)
- Handle text input with proper UTF-8 encoding
- Implement selection extension with Shift modifier
- Add proper handling for special keys (Escape, Enter, etc.)

**Tests to Write**:
```rust
#[test]
fn test_navigation_key_handling() {
    // Test all cursor movement combinations
}

#[test]
fn test_selection_with_shift() {
    // Test Shift+navigation creates selections
}

#[test]
fn test_text_input_processing() {
    // Test character input modifies buffer
}

#[test]
fn test_special_key_handling() {
    // Test Escape, Enter, Tab, etc.
}
```

**Verification Criteria**:
- All key bindings work as specified in PRD
- Text input appears immediately in editor
- Navigation commands update cursor position
- Selection commands highlight text properly

---

## Phase 5: WebSocket Communication

### 5.1 WebSocket Protocol Definition
**Objective**: Define message types and serialization

**Implementation Details**:
- Create `Message` enum with all protocol messages
- Implement JSON serialization/deserialization with `serde`
- Add request ID system for request/response matching
- Define authentication message flow
- Create error message types with proper context

**Tests to Write**:
```rust
#[test]
fn test_message_serialization() {
    // Test all message types serialize/deserialize correctly
}

#[test]
fn test_request_id_system() {
    // Test request/response matching works
}

#[test]
fn test_authentication_flow() {
    // Test auth messages serialize properly
}
```

**Verification Criteria**:
- All messages serialize to valid JSON
- Request IDs enable proper response matching
- Authentication flow messages are well-formed
- Error messages include helpful context

---

### 5.2 WebSocket Server Implementation
**Objective**: Stateless server handling single client

**Implementation Details**:
- Create `GhostwriterServer` using `tokio-tungstenite`
- Implement single client connection management
- Handle authentication with optional passphrase (Argon2 hashing)
- Process file operation requests (read, write, lock, list)
- Maintain single file lock with automatic cleanup

**Tests to Write**:
```rust
#[test]
fn test_single_client_enforcement() {
    // Test second client is rejected
}

#[test]
fn test_authentication_flow() {
    // Test optional auth works correctly
}

#[test]
fn test_file_operations() {
    // Test read, write, lock operations
}

#[test]
fn test_automatic_lock_cleanup() {
    // Test locks released on disconnect
}
```

**Verification Criteria**:
- Only one client can connect at a time
- Authentication works with and without passphrase
- All file operations complete successfully
- Locks are automatically released on disconnect

---

### 5.3 WebSocket Client Implementation
**Objective**: Client connection with graceful degradation

**Implementation Details**:
- Create `GhostwriterClient` with connection management
- Implement automatic reconnection with exponential backoff
- Handle request/response correlation with timeouts
- Support operation queueing during disconnection
- Provide clear connection status to UI

**Tests to Write**:
```rust
#[test]
fn test_client_connection() {
    // Test client connects to server successfully
}

#[test]
fn test_automatic_reconnection() {
    // Test client reconnects after network failure
}

#[test]
fn test_operation_queueing() {
    // Test operations queued when offline
}

#[test]
fn test_request_timeout_handling() {
    // Test timeouts are handled gracefully
}
```

**Verification Criteria**:
- Client connects and authenticates successfully
- Network interruptions trigger automatic reconnection
- Operations queue properly during offline periods
- UI receives clear connection status updates

---

### 5.4 Internal Server for Local Mode
**Objective**: Unified architecture using local WebSocket

**Implementation Details**:
- Spawn internal server on random port for local editing
- Use loopback connection (127.0.0.1) for client
- Maintain same message protocol as remote mode
- Ensure sub-millisecond latency for local operations
- Automatic cleanup when editor exits

**Tests to Write**:
```rust
#[test]
fn test_internal_server_startup() {
    // Test internal server starts on random port
}

#[test]
fn test_loopback_connection() {
    // Test client connects to internal server
}

#[test]
fn test_local_operation_latency() {
    // Test operations complete in < 1ms
}

#[test]
fn test_cleanup_on_exit() {
    // Test internal server stops when editor exits
}
```

**Verification Criteria**:
- Internal server starts without port conflicts
- Local operations feel instantaneous
- Same code path works for local and remote
- Clean shutdown when editor terminates

---

## Phase 6: File History and Navigation

### 6.1 File History Stack
**Objective**: Browser-like navigation with branching

**Implementation Details**:
- Create `FileHistory` with unlimited stack size
- Implement branching logic (new file cuts forward history)
- Store complete file state (cursor, scroll, undo buffer)
- Support forward/backward navigation with Alt+arrows
- Persist history within session only

**Tests to Write**:
```rust
#[test]
fn test_history_stack_operations() {
    // Test push, back, forward operations
}

#[test]
fn test_branching_behavior() {
    // Test opening new file cuts forward stack
}

#[test]
fn test_state_persistence() {
    // Test cursor/scroll position restored
}

#[test]
fn test_unlimited_history() {
    // Test large history doesn't cause memory issues
}
```

**Verification Criteria**:
- History navigation works like browser back/forward
- File state (cursor, undo) is perfectly restored
- Branching cuts forward history appropriately
- Memory usage scales reasonably with history size

---

### 6.2 Search Functionality
**Objective**: Fast incremental search with regex support

**Implementation Details**:
- Implement incremental search with live highlighting
- Support regex patterns with proper error handling
- Add case sensitivity toggle (Ctrl+F to open search)
- Implement search result navigation (Enter/Shift+Enter)
- Clear search highlighting on Escape

**Tests to Write**:
```rust
#[test]
fn test_incremental_search() {
    // Test search updates as user types
}

#[test]
fn test_regex_pattern_support() {
    // Test regex patterns work correctly
}

#[test]
fn test_search_navigation() {
    // Test moving between search results
}

#[test]
fn test_case_sensitivity_toggle() {
    // Test case sensitivity affects results
}
```

**Verification Criteria**:
- Search results highlight immediately as user types
- Regex patterns work without crashing on invalid input
- Navigation between matches works smoothly
- Search performance is acceptable for large files

---

### 6.3 Content Search Across Files
**Objective**: Server-side grep functionality

**Implementation Details**:
- Implement content search request in WebSocket protocol
- Use efficient string matching algorithm (Boyer-Moore or similar)
- Return results with file path, line number, and context
- Support regex patterns for advanced searching
- Limit results to prevent overwhelming UI (max 1000 matches)

**Tests to Write**:
```rust
#[test]
fn test_cross_file_search() {
    // Test searching across multiple files
}

#[test]
fn test_search_result_format() {
    // Test results include path, line, context
}

#[test]
fn test_regex_cross_file_search() {
    // Test regex patterns in file search
}

#[test]
fn test_result_limiting() {
    // Test maximum result count is enforced
}
```

**Verification Criteria**:
- Content search finds matches across all workspace files
- Results include enough context to be useful
- Search completes within reasonable time for large workspaces
- Result limiting prevents UI performance issues

---

## Phase 7: Advanced Features

### 7.1 Binary File Handling
**Objective**: Hex dump display for binary files

**Implementation Details**:
- Detect binary files by scanning for null bytes
- Implement hex dump format with ASCII sidebar
- Support same chunking strategy as text files
- Allow navigation through hex dump with cursor
- Display file offset information

**Tests to Write**:
```rust
#[test]
fn test_binary_file_detection() {
    // Test files with null bytes detected as binary
}

#[test]
fn test_hex_dump_format() {
    // Test hex display format is correct
}

#[test]
fn test_hex_navigation() {
    // Test cursor navigation in hex mode
}

#[test]
fn test_large_binary_files() {
    // Test binary files > 100MB work efficiently
}
```

**Verification Criteria**:
- Binary files automatically switch to hex view
- Hex dump format is readable and standard
- Navigation works smoothly in hex mode
- Large binary files load efficiently

---

### 7.2 Performance Optimization
**Objective**: Optimize for large files and workspaces

**Implementation Details**:
- Implement virtual scrolling for text rendering
- Add lazy loading for file tree with 10,000+ files
- Optimize rope operations for common edit patterns
- Cache file metadata to avoid repeated stat calls
- Implement efficient diff algorithm for conflict resolution

**Tests to Write**:
```rust
#[test]
fn test_virtual_scrolling_performance() {
    // Test scrolling through 1M+ line files
}

#[test]
fn test_large_directory_handling() {
    // Test file picker with 10,000+ files
}

#[test]
fn test_rope_operation_performance() {
    // Test edit operations stay under 10ms
}

#[test]
fn test_memory_usage_scaling() {
    // Test memory doesn't grow unbounded
}
```

**Verification Criteria**:
- UI remains responsive with very large files
- File picker handles large directories smoothly
- Edit operations complete within performance targets
- Memory usage scales predictably with file size

---

### 7.3 Error Recovery and Robustness
**Objective**: Handle edge cases gracefully

**Implementation Details**:
- Implement comprehensive error recovery for network failures
- Handle file system edge cases (permissions, disk full, etc.)
- Add validation for all user inputs and file operations
- Create safe recovery mechanisms for corrupted state
- Implement proper logging for debugging issues

**Tests to Write**:
```rust
#[test]
fn test_network_failure_recovery() {
    // Test recovery from various network failures
}

#[test]
fn test_file_system_error_handling() {
    // Test graceful handling of FS errors
}

#[test]
fn test_input_validation() {
    // Test all inputs are properly validated
}

#[test]
fn test_state_corruption_recovery() {
    // Test recovery from invalid state
}
```

**Verification Criteria**:
- All error conditions are handled gracefully
- User never loses unsaved work due to errors
- Error messages are helpful and actionable
- System recovers automatically when possible

---

## Phase 8: Integration and Polish

### 8.1 End-to-End Integration Testing
**Objective**: Test complete workflows across all modes

**Implementation Details**:
- Create integration tests for local editing workflow
- Test complete remote editing session (connect → edit → disconnect)
- Verify file operations work across client-server boundary
- Test authentication flows with and without passwords
- Validate all key bindings work in integration

**Tests to Write**:
```rust
#[test]
fn test_complete_local_editing_session() {
    // Test: start → open file → edit → save → exit
}

#[test]
fn test_complete_remote_editing_session() {
    // Test: connect → auth → edit → disconnect
}

#[test]
fn test_file_operations_integration() {
    // Test: create → rename → delete files remotely
}

#[test]
fn test_authentication_integration() {
    // Test complete auth flows
}
```

**Verification Criteria**:
- All user workflows complete successfully
- No integration issues between components
- Performance targets met in realistic scenarios
- Error handling works end-to-end

---

### 8.2 Cross-Platform Compatibility
**Objective**: Ensure consistent behavior across platforms

**Implementation Details**:
- Test on Linux x86_64, ARM64, and macOS ARM64
- Verify file locking works on all platforms
- Test terminal compatibility across emulators
- Validate static linking produces portable binaries
- Check WebSocket connectivity across platforms

**Tests to Write**:
```rust
#[cfg(target_os = "linux")]
#[test]
fn test_linux_file_locking() {
    // Test fcntl-based file locking
}

#[cfg(target_os = "macos")]
#[test]
fn test_macos_file_locking() {
    // Test macOS file locking behavior
}

#[test]
fn test_cross_platform_websockets() {
    // Test WebSocket communication works everywhere
}
```

**Verification Criteria**:
- Identical behavior across all target platforms
- Static binaries run without dependencies
- File operations respect platform conventions
- Network communication works reliably

---

### 8.3 Security Audit
**Objective**: Verify security implementation is robust

**Implementation Details**:
- Audit path traversal prevention mechanisms
- Verify file locking prevents concurrent write access
- Test authentication implementation against attacks
- Validate input sanitization prevents injection
- Check workspace sandboxing is complete

**Tests to Write**:
```rust
#[test]
fn test_path_traversal_prevention() {
    // Test various traversal attack vectors
}

#[test]
fn test_authentication_attack_resistance() {
    // Test brute force and timing attacks
}

#[test]
fn test_input_sanitization() {
    // Test malicious inputs are handled safely
}

#[test]
fn test_workspace_escape_attempts() {
    // Test sandbox cannot be escaped
}
```

**Verification Criteria**:
- No path traversal vulnerabilities exist
- Authentication is secure against common attacks
- All inputs are properly validated and sanitized
- Workspace boundaries cannot be bypassed

---

### 8.4 Performance Validation
**Objective**: Verify all performance targets are met

**Implementation Details**:
- Benchmark startup time across different scenarios
- Measure edit operation latency under various conditions
- Test memory usage with large files and workspaces
- Validate network operation performance
- Profile CPU usage during intensive operations

**Tests to Write**:
```rust
#[test]
fn test_startup_performance_targets() {
    // Test startup < 50ms local, < 200ms remote
}

#[test]
fn test_edit_operation_latency() {
    // Test keystrokes process in < 10ms
}

#[test]
fn test_memory_usage_limits() {
    // Test memory stays within bounds
}

#[test]
fn test_network_operation_performance() {
    // Test remote operations meet latency targets
}
```

**Verification Criteria**:
- All performance targets from PRD are met
- No performance regressions under stress testing
- Memory usage is predictable and bounded
- Network operations complete within SLA times

---

## Final Verification Checklist

### Functional Completeness
- [ ] All key bindings work as specified
- [ ] File operations (CRUD) work in all modes
- [ ] Text editing with undo/redo functions correctly
- [ ] File history and navigation work properly
- [ ] Search functionality works locally and remotely
- [ ] Binary file display works correctly
- [ ] Authentication system functions properly

### Performance Requirements
- [ ] Startup time < 50ms (local), < 200ms (remote)
- [ ] Keystroke latency < 10ms
- [ ] File operations < 100ms
- [ ] Memory usage < 100MB for typical projects
- [ ] Handles 1GB files efficiently

### Security and Robustness
- [ ] Workspace sandboxing prevents path traversal
- [ ] File locking prevents concurrent access
- [ ] Authentication resists common attacks
- [ ] Input validation prevents malicious input
- [ ] Error handling prevents crashes and data loss

### Cross-Platform Compatibility
- [ ] Linux x86_64 static binary works
- [ ] Linux ARM64 static binary works
- [ ] macOS ARM64 static binary works
- [ ] All platforms show identical behavior
- [ ] Terminal compatibility across emulators

This implementation plan provides 32 detailed, testable tasks that build Ghostwriter incrementally with TDD principles, ensuring each component works correctly before building the next layer.
