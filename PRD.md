# Ghostwriter - Product Requirements Document

## Executive Summary

**Ghostwriter** is a lightweight, fast terminal-based text editor with continuous file synchronization and optional client-server architecture for remote editing. It prioritizes zero-configuration operation, blazing performance, and strict security sandboxing while providing a modern editing experience.

**Key Value Propositions:**
- **Zero Config**: Works perfectly out of the box with opinionated defaults
- **Continuous Sync**: Never lose data with keystroke-level file synchronization
- **Remote Ready**: Seamless local and remote editing with identical user experience
- **Security First**: Strict workspace sandboxing and single-user model
- **Performance**: Sub-100ms operations, handles files up to 1GB

## Product Vision

Create the fastest, most reliable terminal text editor that "just works" for both local and remote editing scenarios, with enterprise-grade security and zero data loss guarantees.

## Target Users

### Primary Users
- **Developers** working on remote servers via SSH
- **System Administrators** editing configuration files on production systems
- **DevOps Engineers** managing infrastructure files across multiple environments
- **Technical Writers** working with text files in terminal environments

### Secondary Users
- **Students** learning command-line development workflows
- **Power Users** seeking lightweight alternatives to heavy IDEs
- **Security-conscious Users** requiring sandboxed editing environments

## Core Use Cases

### UC1: Local Development Editing
**Actor**: Developer
**Goal**: Edit files locally with continuous backup
**Flow**:
1. Run `ghostwriter /my/project` in terminal
2. Press `Ctrl+O` to open full-screen file picker
3. Type to filter files: `src/ma` shows `src/main.rs`, `src/manager.rs`
4. Navigate tree structure and select file
5. Edit file with immediate auto-save
6. Use `Ctrl+H` for file history or `Ctrl+O` for new files
7. Create new files/folders directly in file picker with `Ctrl+N`/`Ctrl+Shift+N`
8. Close editor knowing no work is lost

**Success Criteria**:
- Startup < 50ms
- File picker filtering < 10ms response time
- File operations complete without leaving picker
- Zero data loss even on crash

### UC2: Remote Server Editing
**Actor**: DevOps Engineer
**Goal**: Edit configuration files on production server
**Flow**:
1. SSH to server: `ssh user@production-server`
2. Start server: `ghostwriter --server /etc/config --key "prod-secret"`
3. From local machine: `ghostwriter --connect ws://server:8080 --key "prod-secret"`
4. Edit files seamlessly as if local
5. Disconnect safely with automatic cleanup

**Success Criteria**:
- Connection latency < 200ms
- Edit sync latency < 100ms
- Automatic lock cleanup on disconnect
- No configuration files required

### UC3: Secure File Sharing
**Actor**: Team Lead
**Goal**: Share read-only access to documentation
**Flow**:
1. Start read-only server: `ghostwriter --server /docs --readonly`
2. Share connection info: `ghostwriter --connect ws://teamserver:8080`
3. Team members browse and read files
4. Only one user can access server at a time for security

**Success Criteria**:
- Clear read-only indicators
- Secure single-user model
- Optional authentication when needed

## Feature Requirements

### F1: Core Text Editing
**Priority**: P0 (Must Have)

**Requirements**:
- Standard editing operations (insert, delete, cut, copy, paste)
- Linear undo/redo system (no branching for simplicity)
- Incremental search with regex support
- Advanced cursor navigation (word, line, document boundaries)
- Text selection with Shift modifier for all navigation commands
- Line operations (go to line, duplicate, delete)
- UTF-8 text with hex fallback for invalid bytes
- Binary file hex dump display
- Support files up to 1GB with memory mapping

**Acceptance Criteria**:
- Edit operations respond in < 10ms
- Cursor navigation works smoothly across word/document boundaries
- Shift+navigation creates precise text selections
- Undo/redo preserves exact state including selections
- Search highlights matches in real-time
- Binary files display cleanly in hex format

### F2: File Management
**Priority**: P0 (Must Have)

**Requirements**:
- Single buffer architecture (one file open at a time)
- Infinite file history with browser-like navigation
- Branching history (opening new file cuts forward stack)
- Full-screen file picker with integrated tree view
- Fuzzy text filtering that filters the entire tree structure
- File/folder operations within picker (add, rename, delete)
- Strict workspace sandboxing
- Read-only fallback when write lock unavailable

**File Picker Features**:
- Full-screen overlay when activated with `Ctrl+O`
- Live fuzzy filtering that shows matching files and their parent directories
- Tree navigation with expand/collapse functionality
- File operations accessible via keyboard shortcuts
- Real-time preview of selected file content
- Breadcrumb navigation showing current directory context

**Acceptance Criteria**:
- File picker filters 1000+ files instantly as user types
- File operations (create/rename/delete) work seamlessly within picker
- Tree filtering preserves directory structure context
- File switching preserves cursor and scroll position
- Sandbox prevents access outside workspace
- Picker dismisses cleanly without affecting editor state

### F3: Continuous Synchronization
**Priority**: P0 (Must Have)

**Requirements**:
- Save on every keystroke (100ms debounced)
- Atomic writes using temp file + rename
- File locking to prevent conflicts
- External change detection and notification
- Conflict resolution with diff view
- Network resilience with operation queueing

**Acceptance Criteria**:
- No data loss even on system crash
- External changes detected within 1 second
- Write operations complete within 50ms
- Graceful handling of disk full scenarios

### F4: Client-Server Architecture
**Priority**: P1 (Should Have)

**Requirements**:
- Unified WebSocket architecture (local and remote)
- Stateless server design
- Single user model (one client per server)
- Optional passphrase authentication
- Request/response protocol with UUIDs
- Automatic lock cleanup on disconnect

**Acceptance Criteria**:
- Local mode startup < 50ms
- Remote connection established < 200ms
- Edit synchronization < 100ms
- Zero server state corruption on disconnect

### F5: Security & Sandboxing
**Priority**: P0 (Must Have)

**Requirements**:
- Workspace root path validation
- Prevention of directory traversal attacks
- Single file lock per client maximum
- Automatic lock release on disconnect/file switch
- Optional authentication with secure password hashing
- Rate limiting on authentication attempts

**Acceptance Criteria**:
- No access outside defined workspace
- All file paths canonicalized and validated
- Auth keys hashed with Argon2
- Failed auth attempts rate limited (3/minute)

### F6: Performance Optimization
**Priority**: P1 (Should Have)

**Requirements**:
- Rope data structure for efficient large file editing
- Memory-mapped I/O for files > 100MB
- Incremental rendering (only redraw changes)
- Background file operations
- Virtual scrolling for large directories
- Lazy loading of file content

**Acceptance Criteria**:
- Handle 1GB files without performance degradation
- Memory usage < 100MB for typical workspaces
- Directory with 1000 files loads < 200ms
- UI remains responsive during file operations

### F7: Network Resilience
**Priority**: P1 (Should Have)

**Requirements**:
- Graceful degradation during connection issues
- Current buffer editing always works
- Operation queueing for offline periods
- Automatic reconnection with exponential backoff
- Non-invasive status indicators
- Clear error messages for connection failures

**Acceptance Criteria**:
- Can edit current file even when offline
- Reconnection successful within 10 seconds
- All queued operations replay correctly
- Status changes don't interrupt typing

## Non-Functional Requirements

### Performance
- **Startup Time**: < 50ms for local mode, < 200ms for remote connection
- **Keystroke Latency**: < 10ms for text input
- **File Operations**: < 100ms for file switching, < 50ms for saves
- **File Picker**: < 10ms response time for filtering, handles 10,000+ files
- **Memory Usage**: < 100MB for typical projects, streaming for 1GB+ files
- **Network Latency**: < 100ms for remote edit synchronization

### Reliability
- **Data Integrity**: Zero data loss guarantee, even on crashes
- **Uptime**: Server can run continuously for weeks without restart
- **Error Recovery**: Graceful handling of all file system errors
- **Consistency**: Identical behavior across all supported platforms

### Security
- **Sandboxing**: Complete isolation within workspace directory
- **Authentication**: Optional but secure when enabled
- **Access Control**: Single user model prevents unauthorized access
- **Audit Trail**: Log all file operations for security review

### Compatibility
- **Platforms**: Linux (x86_64, ARM64), macOS (ARM64)
- **Terminals**: Support for 256-color and true-color terminals
- **File Systems**: Works with all POSIX-compliant file systems
- **Networks**: WebSocket over TCP, optional TLS support

## Technical Constraints

### Language & Runtime
- **Implementation**: Rust with static linking
- **Dependencies**: Zero runtime dependencies
- **Binary Size**: < 8MB for complete functionality
- **Compilation**: Cross-platform static compilation

### Architecture Decisions
- **Single Binary**: All modes (local/server/client) in one executable
- **WebSocket Protocol**: Unified communication layer
- **Stateless Server**: No persistent state except file locks
- **Client-Side State**: All editing state managed by client

### Resource Limits
- **File Size**: 1GB maximum with graceful degradation
- **Directory Size**: Efficient handling of 10,000+ files
- **Connection Timeout**: 30 seconds for network operations
- **Memory Mapping**: For files > 100MB to control memory usage

## Success Metrics

### Adoption Metrics
- **Downloads**: 10,000 downloads within first 6 months of release
- **GitHub Stars**: 1,000 stars within first year
- **Usage Growth**: 25% month-over-month active usage

### Performance Metrics
- **Startup Time**: 95% of sessions start within 50ms
- **Crash Rate**: < 0.1% of editing sessions experience crashes
- **Data Loss**: Zero reported instances of data loss
- **Response Time**: 99% of operations complete within SLA

### User Satisfaction
- **Net Promoter Score**: > 50 from power users
- **Issue Resolution**: 90% of bugs fixed within one release cycle
- **Documentation**: 95% of users can complete basic tasks without help

## Implementation Roadmap

### Phase 1: Core Foundation
**Deliverables:**
- Basic terminal UI with ratatui
- File buffer management with Rope
- Text editing with undo/redo
- UTF-8 decoding with hex fallback
- Local file I/O with atomic writes

**Success Criteria:**
- Functional local text editor
- Handles files up to 100MB
- Basic editing operations work smoothly

### Phase 2: Stateless Server
**Deliverables:**
- WebSocket server implementation
- Request/response protocol
- File locking with automatic cleanup
- Authentication system
- Directory operations

**Success Criteria:**
- Server handles client connections
- File operations work via WebSocket
- Single lock model enforced

### Phase 3: Client Integration
**Deliverables:**
- WebSocket client implementation
- Connection state management
- Graceful degradation logic
- Operation queueing for offline mode
- Status indicators

**Success Criteria:**
- Seamless local/remote experience
- Resilient to network issues
- Clear user feedback on connection state

### Phase 4: Advanced Features
**Deliverables:**
- File history with branching
- Full-screen file picker with integrated tree management
- Fuzzy filtering system for large file trees
- File operations (create, rename, delete) within picker
- Content search across workspace
- Binary file handling
- Performance optimizations

**Success Criteria:**
- File picker handles 10,000+ files with instant filtering
- All file operations work seamlessly within picker interface
- File preview shows first few lines of selected files
- Search performs well on large codebases
- Memory usage optimized for large workspaces

### Phase 5: Polish & Release
**Deliverables:**
- Cross-platform compilation
- Documentation and examples
- Error handling improvements
- Security audit
- Release packaging

**Success Criteria:**
- Binaries work on all target platforms
- Documentation enables user onboarding
- Security review passes

## Risk Assessment

### High Risk Items
1. **Network Reliability**: WebSocket connections may be unstable
   - **Mitigation**: Robust reconnection logic and offline mode
2. **File System Edge Cases**: Unusual file permissions or corruption
   - **Mitigation**: Comprehensive error handling and graceful degradation
3. **Performance with Large Files**: Memory usage could spike
   - **Mitigation**: Memory mapping and streaming for large files

### Medium Risk Items
1. **Cross-Platform Compatibility**: Terminal behavior varies
   - **Mitigation**: Extensive testing on target platforms
2. **User Experience**: Terminal UI may feel limited
   - **Mitigation**: Focus on keyboard efficiency and clear indicators

### Low Risk Items
1. **Competition**: Existing editors are well-established
   - **Mitigation**: Focus on unique value proposition (remote + continuous sync)

## Appendix

### Command Line Interface
```bash
# Local editing
ghostwriter /path/to/file.txt
ghostwriter /path/to/directory

# Server mode
ghostwriter --server /workspace
ghostwriter --server /workspace --port 9000 --key "secret"

# Client mode
ghostwriter --connect ws://server:8080
ghostwriter --connect ws://server:8080 --key "secret"

# Options
--readonly          # Read-only mode
--port PORT         # Custom port (default: 8080)
--bind ADDRESS      # Bind address (default: 127.0.0.1)
--key PASSPHRASE    # Optional authentication
--timeout SECONDS   # Connection timeout (default: 30s)
```

### Key Bindings (Non-configurable)
```
File Operations:
  Ctrl+O      Open file picker (full-screen overlay)
  Ctrl+H      File history navigation
  Ctrl+Q      Quit (auto-saves)

File Picker (when active):
  Escape      Close picker
  Enter       Open selected file
  Ctrl+N      Create new file
  Ctrl+Shift+N Create new folder
  Ctrl+R      Rename selected item
  Ctrl+D      Delete selected item
  Space       Expand/collapse directory
  Up/Down     Navigate file tree
  Tab         Toggle between tree and preview

Editing:
  Ctrl+Z      Undo
  Ctrl+Y      Redo
  Ctrl+F      Find in current file
  Ctrl+G      Go to line number
  Ctrl+A      Select all
  Ctrl+C/X/V  Copy/Cut/Paste

Navigation:
  Alt+Up/Home     Go to start of document (line 0)
  Alt+Down/End    Go to end of document (last line)
  Alt+Left        Previous word start in current line
  Alt+Right       Next word end in current line
  Home            Go to start of current line
  End             Go to end of current line
  Ctrl+Left       Go to start of current line (same as Home)
  Ctrl+Right      Go to end of current line (same as End)
  Page Up/Down    Scroll by screen height

Selection (add Shift to any navigation command):
  Shift+Alt+Up/Home    Select to start of document
  Shift+Alt+Down/End   Select to end of document
  Shift+Alt+Left       Select to previous word start
  Shift+Alt+Right      Select to next word end
  Shift+Home           Select to start of current line
  Shift+End            Select to end of current line
  Shift+Ctrl+Left      Select to start of current line (same as Shift+Home)
  Shift+Ctrl+Right     Select to end of current line (same as Shift+End)
  Shift+Up/Down        Select line by line
  Shift+Page Up/Down   Select by screen height
```
