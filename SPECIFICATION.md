# File Editing Server - Complete Implementation Specification

**Version:** 1.0.0  
**Date:** June 5, 2025  
**Document Type:** S2C Single-File Implementation Specification  
**Language:** English  
**Format:** Markdown

## 1. System Overview & Scope

### 1.1 Implementation Boundaries

The File Editing Server SHALL implement a high-performance binary providing text file manipulation capabilities for LLM and AI agents. The system MUST operate exclusively within a single designated folder path and SHALL provide flat file access using filename-only references.

**Included in Implementation:**
- Single binary executable with command-line argument configuration
- Two transport options: HTTP REST API OR stdio JSON-RPC communication
- Three file operations: list_files, read_file and edit_file with complete functionality
- Text file reading with optional line range selection capabilities
- Text file editing via line-based diff operations and append functionality
- New file creation within designated folder boundaries
- Flat file storage abstraction using filename-only references
- Cross-platform file system operations supporting Windows, macOS, Linux
- UTF-8 text encoding support with validation
- Concurrent request handling with filesystem-level locking mechanisms
- Comprehensive error handling for all file system operations

**Excluded from Implementation:**
- File operations outside designated folder boundaries
- Binary file handling or non-text file processing
- Directory traversal, nested folder support, or hierarchical access
- File permissions modification or ownership changes
- Version control integration or file history tracking
- Backup, recovery, or rollback mechanisms
- User authentication or authorization beyond transport security
- File watching, real-time notifications, or event streaming
- Configuration files or complex setup procedures

### 1.2 Platform and Environment Requirements

**Programming Language:** Compiled language with strong typing (Go 1.21+, Rust 1.70+, or Zig 0.11+)  
**Standard Libraries:** HTTP server, JSON processing, file I/O, concurrency primitives, string manipulation  
**Target Platforms:** Cross-compiled binaries for:
- Linux (x86_64, aarch64)
- macOS (x86_64, aarch64) 
- Windows (x86_64)
**File System Requirements:** POSIX-compliant or Windows NTFS with UTF-8 support  
**Text Encoding:** UTF-8 exclusively with validation  
**Memory Model:** Stack-allocated where possible, minimal heap usage

### 1.3 Command Line Interface Specification

**Binary Execution Pattern:**
```
file-editor --dir=<path> [--transport=<type>] [--port=<number>] [options]
```

**Required Arguments:**
- `--dir=<path>`: Working directory path (MUST exist and be writable)

**Optional Arguments:**
- `--transport=<type>`: Communication method (http|stdio) [default: http]
- `--port=<number>`: HTTP port number [default: 8080, range: 1024-65535]
- `--max-size=<mb>`: Maximum file and request size in megabytes [default: 10, range: 1-100]
- `--timeout=<seconds>`: Operation timeout in seconds [default: 10, range: 1-300]

**Validation Requirements:**
- Working directory MUST exist before server start
- Working directory MUST be writable by process user
- Port number MUST be available for binding when HTTP transport selected
- All numeric parameters MUST be within specified ranges
- Invalid arguments MUST cause immediate exit with error code 1

### 1.4 Technical Terminology Definitions

- **Transport Protocol:** Communication mechanism (HTTP REST or stdio JSON-RPC)
- **Flat File Access:** File operations using filename without path components
- **Line-Based Diff:** Modification targeting specific line numbers with operation type
- **Working Directory:** Single designated folder containing all accessible files
- **Line Range:** Inclusive start and end line numbers for partial reading
- **Atomic Operation:** File modification completing entirely or failing entirely
- **File Lock:** Exclusive access mechanism preventing concurrent modifications

## 2. Functional Implementation Requirements

### 2.1 File Reading Operations

**User Story:** As an AI agent, I MUST read file contents with optional line range specification to analyze and process text files efficiently without loading unnecessary data.

**Business Logic Decision Tree:**

```pseudocode
FUNCTION read_file(filename, start_line, end_line):
    // Input validation branch
    IF filename is empty OR contains path separators THEN
        RETURN error "Invalid filename format"
    END IF
    
    IF start_line < 1 OR end_line < 1 THEN
        RETURN error "Line numbers must be positive integers"
    END IF
    
    IF start_line > end_line THEN
        RETURN error "Start line cannot exceed end line"
    END IF
    
    // File access branch
    file_path = join(working_directory, filename)
    
    IF NOT file_exists(file_path) THEN
        RETURN error "File not found"
    END IF
    
    file_size = get_file_size(file_path)
    IF file_size > max_file_size_bytes THEN
        RETURN error "File exceeds maximum size limit"
    END IF
    
    // Content processing branch
    file_content = read_file_bytes(file_path)
    IF NOT is_valid_utf8(file_content) THEN
        RETURN error "File contains invalid UTF-8 encoding"
    END IF
    
    lines = split_lines(file_content)
    total_line_count = count(lines)
    
    // Range selection branch
    IF start_line IS NULL AND end_line IS NULL THEN
        selected_lines = lines
    ELSE IF start_line IS NOT NULL AND end_line IS NULL THEN
        IF start_line > total_line_count THEN
            RETURN error "Start line exceeds file length"
        END IF
        selected_lines = lines[start_line-1 : total_line_count]
    ELSE IF start_line IS NULL AND end_line IS NOT NULL THEN
        IF end_line > total_line_count THEN
            end_line = total_line_count
        END IF
        selected_lines = lines[0 : end_line]
    ELSE
        IF start_line > total_line_count THEN
            RETURN error "Start line exceeds file length"
        END IF
        actual_end = MIN(end_line, total_line_count)
        selected_lines = lines[start_line-1 : actual_end]
    END IF
    
    result_content = join_lines(selected_lines)
    
    RETURN {
        content: result_content,
        total_lines: total_line_count,
        range_requested: {start_line, end_line}
    }
END FUNCTION
```

**Data Processing Algorithm:**

```pseudocode
FUNCTION split_lines(file_content):
    // Handle different line ending types
    normalized_content = replace(file_content, "\r\n", "\n")
    normalized_content = replace(normalized_content, "\r", "\n")
    
    lines = split(normalized_content, "\n")
    
    // Remove trailing empty line if file ends with newline
    IF length(lines) > 0 AND lines[last_index] = "" THEN
        lines = lines[0 : last_index]
    END IF
    
    RETURN lines
END FUNCTION
```

**Acceptance Criteria:**
- **GIVEN** a file "test.txt" exists with content "line1\nline2\nline3" **WHEN** read_file is called with name="test.txt" **THEN** content="line1\nline2\nline3" AND total_lines=3
- **GIVEN** a file exists **WHEN** read_file is called with start_line=2, end_line=3 **THEN** only lines 2-3 are returned
- **GIVEN** a file "missing.txt" does not exist **WHEN** read_file is called **THEN** error code -32001 is returned
- **GIVEN** start_line=5, end_line=3 **WHEN** read_file is called **THEN** error code -32602 is returned
- **GIVEN** a file larger than max_file_size **WHEN** read_file is called **THEN** error code -32001 is returned

### 2.2 File Listing Operations

**User Story:** As an AI agent, I MUST list all available files in the working directory to discover which files can be read or edited.

**Business Logic Decision Tree:**

```pseudocode
FUNCTION list_files():
    // Directory access validation
    IF NOT can_read_directory(working_directory) THEN
        RETURN error "Cannot access working directory"
    END IF
    
    // File enumeration
    all_entries = get_directory_entries(working_directory)
    file_list = []
    
    FOR each entry in all_entries DO
        IF entry.is_file AND NOT entry.is_hidden THEN
            file_info = {
                name: entry.filename,
                size: entry.file_size,
                modified: entry.last_modified_time,
                readable: can_read_file(entry.path),
                writable: can_write_file(entry.path),
                lines: count_file_lines(entry.path)
            }
            file_list.append(file_info)
        END IF
    END FOR
    
    // Sort by filename for consistent ordering
    sorted_files = sort_by_filename(file_list)
    
    RETURN {
        files: sorted_files,
        total_count: count(sorted_files),
        directory: working_directory
    }
END FUNCTION
```

**File Line Counting Algorithm:**

```pseudocode
FUNCTION count_file_lines(file_path):
    IF get_file_size(file_path) > max_file_size THEN
        RETURN -1  // Indicate file too large to count
    END IF
    
    TRY
        file_content = read_file_bytes(file_path)
        IF NOT is_valid_utf8(file_content) THEN
            RETURN -1  // Indicate non-text file
        END IF
        
        lines = split_lines(file_content)
        RETURN count(lines)
    CATCH file_error:
        RETURN -1  // Indicate counting failed
    END TRY
END FUNCTION
```

**Acceptance Criteria:**
- **GIVEN** working directory contains files ["a.txt", "b.txt"] **WHEN** list_files is called **THEN** both files are returned in sorted order with line counts
- **GIVEN** working directory is empty **WHEN** list_files is called **THEN** empty files array with total_count=0 is returned
- **GIVEN** working directory contains subdirectories **WHEN** list_files is called **THEN** only files are returned, not directories
- **GIVEN** working directory contains hidden files **WHEN** list_files is called **THEN** hidden files are excluded from results
- **GIVEN** file is too large or binary **WHEN** list_files is called **THEN** lines=-1 is returned for that file
- **GIVEN** working directory is not readable **WHEN** list_files is called **THEN** error code -32001 is returned

### 2.3 File Editing Operations

**User Story:** As an AI agent, I MUST edit files using line-based diff operations and append functionality to modify text content precisely while maintaining data integrity.

**Business Logic Decision Tree:**

```pseudocode
FUNCTION edit_file(filename, edits_array, append_content, create_if_missing):
    // Input validation branch
    IF filename is empty OR contains path separators THEN
        RETURN error "Invalid filename format"
    END IF
    
    FOR each edit in edits_array DO
        IF edit.line < 1 THEN
            RETURN error "Line numbers must be positive integers"
        END IF
        IF edit.operation NOT IN ["replace", "insert", "delete"] THEN
            RETURN error "Invalid edit operation"
        END IF
        IF edit.operation = "delete" AND edit.content IS NOT NULL THEN
            RETURN error "Delete operation cannot specify content"
        END IF
    END FOR
    
    // File existence handling branch
    file_path = join(working_directory, filename)
    file_exists = check_file_exists(file_path)
    
    IF NOT file_exists AND NOT create_if_missing THEN
        RETURN error "File not found and create_if_missing is false"
    END IF
    
    // Acquire exclusive file lock
    acquire_file_lock(filename)
    TRY
        // Content initialization branch
        IF file_exists THEN
            file_content = read_file_bytes(file_path)
            IF NOT is_valid_utf8(file_content) THEN
                RETURN error "File contains invalid UTF-8 encoding"
            END IF
            lines = split_lines(file_content)
            file_created = false
        ELSE
            lines = empty_array()
            file_created = true
        END IF
        
        original_line_count = count(lines)
        
        // Edit processing branch (reverse order to maintain line stability)
        sorted_edits = sort_by_line_number_descending(edits_array)
        
        FOR each edit in sorted_edits DO
            result = apply_line_edit(lines, edit)
            IF result is error THEN
                RETURN result
            END IF
        END FOR
        
        // Append processing branch
        IF append_content IS NOT NULL THEN
            append_lines = split_lines(append_content)
            lines = concatenate(lines, append_lines)
        END IF
        
        // Atomic write operation
        final_content = join_lines_with_newlines(lines)
        write_result = write_file_atomic(file_path, final_content)
        IF write_result is error THEN
            RETURN write_result
        END IF
        
        RETURN {
            success: true,
            lines_modified: abs(count(lines) - original_line_count),
            file_created: file_created,
            new_total_lines: count(lines)
        }
    FINALLY
        release_file_lock(filename)
    END TRY
END FUNCTION
```

**Line Edit Processing Algorithm:**

```pseudocode
FUNCTION apply_line_edit(lines_array, edit):
    line_index = edit.line - 1  // Convert to zero-based indexing
    line_count = count(lines_array)
    
    SWITCH edit.operation:
        CASE "replace":
            IF line_index < 0 OR line_index >= line_count THEN
                RETURN error "Line {edit.line} out of range for replace operation"
            END IF
            lines_array[line_index] = edit.content
            
        CASE "insert":
            IF line_index < 0 OR line_index > line_count THEN
                RETURN error "Line {edit.line} out of range for insert operation"
            END IF
            insert_at_position(lines_array, line_index, edit.content)
            
        CASE "delete":
            IF line_index < 0 OR line_index >= line_count THEN
                RETURN error "Line {edit.line} out of range for delete operation"
            END IF
            remove_at_position(lines_array, line_index)
            
        DEFAULT:
            RETURN error "Unknown edit operation: {edit.operation}"
    END SWITCH
    
    RETURN success
END FUNCTION
```

**Atomic Write Implementation:**

```pseudocode
FUNCTION write_file_atomic(file_path, content):
    temp_path = file_path + ".tmp." + generate_random_suffix()
    
    TRY
        write_file_bytes(temp_path, utf8_encode(content))
        set_file_permissions(temp_path, read_write_owner_only)
        atomic_rename(temp_path, file_path)
        RETURN success
    CATCH any_error:
        delete_file_if_exists(temp_path)
        RETURN error "Failed to write file atomically: {any_error}"
    END TRY
END FUNCTION
```

**Acceptance Criteria:**
- **GIVEN** a file exists **WHEN** edit_file is called with line=2, operation="replace", content="new line" **THEN** line 2 is replaced with "new line"
- **GIVEN** a file with 5 lines **WHEN** edit_file is called with line=3, operation="insert", content="inserted" **THEN** new line is inserted at position 3
- **GIVEN** a file exists **WHEN** edit_file is called with line=2, operation="delete" **THEN** line 2 is removed
- **GIVEN** file does not exist **WHEN** edit_file is called with create_if_missing=true **THEN** new file is created
- **GIVEN** multiple edits targeting different lines **WHEN** edit_file is called **THEN** all edits are applied successfully in single operation

### 2.4 Concurrency and State Management

**Filesystem Locking Algorithm:**

```pseudocode
FUNCTION acquire_file_lock(file_path):
    // Use OS-level file locking instead of application-level
    lock_handle = create_file_lock(file_path, exclusive=true)
    
    TRY
        success = acquire_lock_with_timeout(lock_handle, timeout=30_seconds)
        IF NOT success THEN
            RETURN error "Failed to acquire file lock within timeout"
        END IF
        RETURN lock_handle
    CATCH lock_error:
        RETURN error "Lock acquisition failed: {lock_error}"
    END TRY
END FUNCTION

FUNCTION release_file_lock(lock_handle):
    TRY
        release_lock(lock_handle)
        close_lock_handle(lock_handle)
    CATCH release_error:
        log_error("Failed to release file lock", release_error)
    END TRY
END FUNCTION
```

**State Transition Requirements:**
- File states: [NonExistent, Readable, Locked, Modified, Error]
- NonExistent → Readable (file discovery)
- Readable → Locked (operation start)
- Locked → Modified (successful edit)
- Locked → Readable (operation completion)
- Any State → Error (system failure)
- Error → Readable (error recovery)

## 3. System Interface Specifications

### 3.1 HTTP REST API Specification

**Base URL Pattern:** `http://{host}:{port}`  
**Content-Type:** `application/json` (REQUIRED for all requests and responses)  
**Character Encoding:** UTF-8 (MUST be specified in Content-Type header)

#### 3.1.1 List Files Endpoint

**HTTP Method:** POST  
**Endpoint Path:** `/list_files`  
**Request Headers:**
- `Content-Type: application/json; charset=utf-8` (REQUIRED)
- `Accept: application/json` (RECOMMENDED)

**Request Schema:**
```json
{
  "type": "object",
  "properties": {},
  "additionalProperties": false
}
```

**Success Response (HTTP 200):**
```json
{
  "type": "object",
  "properties": {
    "files": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "name": {
            "type": "string",
            "description": "Filename without path"
          },
          "size": {
            "type": "integer",
            "minimum": 0,
            "description": "File size in bytes"
          },
          "modified": {
            "type": "string",
            "format": "date-time",
            "description": "Last modified timestamp"
          },
          "readable": {
            "type": "boolean",
            "description": "Whether file can be read"
          },
          "writable": {
            "type": "boolean",
            "description": "Whether file can be written"
          },
          "lines": {
            "type": "integer",
            "minimum": -1,
            "description": "Number of lines in file (-1 if unable to count)"
          }
        },
        "required": ["name", "size", "modified", "readable", "writable", "lines"],
        "additionalProperties": false
      }
    },
    "total_count": {
      "type": "integer",
      "minimum": 0,
      "description": "Total number of files"
    },
    "directory": {
      "type": "string",
      "description": "Working directory path"
    }
  },
  "required": ["files", "total_count", "directory"],
  "additionalProperties": false
}
```

#### 3.1.2 Read File Endpoint

**HTTP Method:** POST  
**Endpoint Path:** `/read_file`  
**Request Headers:**
- `Content-Type: application/json; charset=utf-8` (REQUIRED)
- `Accept: application/json` (RECOMMENDED)

**Request Schema:**
```json
{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "pattern": "^[a-zA-Z0-9._-]+$",
      "minLength": 1,
      "maxLength": 255,
      "description": "Filename without path components"
    },
    "start_line": {
      "type": "integer",
      "minimum": 1,
      "description": "Starting line number (1-based, inclusive)"
    },
    "end_line": {
      "type": "integer",
      "minimum": 1,
      "description": "Ending line number (1-based, inclusive)"
    }
  },
  "required": ["name"],
  "additionalProperties": false
}
```

**Success Response (HTTP 200):**
```json
{
  "type": "object",
  "properties": {
    "content": {
      "type": "string",
      "description": "File content or specified line range"
    },
    "total_lines": {
      "type": "integer",
      "minimum": 0,
      "description": "Total number of lines in the file"
    },
    "range_requested": {
      "type": "object",
      "properties": {
        "start_line": {"type": "integer", "minimum": 1},
        "end_line": {"type": "integer", "minimum": 1}
      },
      "description": "Echo of requested range parameters"
    }
  },
  "required": ["content", "total_lines"],
  "additionalProperties": false
}
```

#### 3.1.3 Edit File Endpoint

**HTTP Method:** POST  
**Endpoint Path:** `/edit_file`  
**Request Headers:** Same as read_file endpoint

**Request Schema:**
```json
{
  "type": "object",
  "properties": {
    "name": {
      "type": "string",
      "pattern": "^[a-zA-Z0-9._-]+$",
      "minLength": 1,
      "maxLength": 255,
      "description": "Filename without path components"
    },
    "edits": {
      "type": "array",
      "items": {
        "type": "object",
        "properties": {
          "line": {
            "type": "integer",
            "minimum": 1,
            "description": "Line number to edit (1-based)"
          },
          "content": {
            "type": "string",
            "description": "New content for the line (not required for delete)"
          },
          "operation": {
            "type": "string",
            "enum": ["replace", "insert", "delete"],
            "description": "Type of edit operation"
          }
        },
        "required": ["line", "operation"],
        "additionalProperties": false
      },
      "maxItems": 1000,
      "description": "Array of line-based edit operations"
    },
    "append": {
      "type": "string",
      "description": "Content to append to end of file"
    },
    "create_if_missing": {
      "type": "boolean",
      "default": false,
      "description": "Create file if it doesn't exist"
    }
  },
  "required": ["name"],
  "additionalProperties": false
}
```

**Success Response (HTTP 200):**
```json
{
  "type": "object",
  "properties": {
    "success": {
      "type": "boolean",
      "const": true,
      "description": "Operation success indicator"
    },
    "lines_modified": {
      "type": "integer",
      "minimum": 0,
      "description": "Number of lines affected by the operation"
    },
    "file_created": {
      "type": "boolean",
      "description": "Whether a new file was created"
    },
    "new_total_lines": {
      "type": "integer",
      "minimum": 0,
      "description": "Total lines in file after edit"
    }
  },
  "required": ["success", "lines_modified", "file_created", "new_total_lines"],
  "additionalProperties": false
}
```

### 3.2 stdio JSON-RPC Specification

**Protocol Version:** JSON-RPC 2.0 (MUST include "jsonrpc": "2.0")  
**Transport:** stdin/stdout with line-delimited JSON messages  
**Character Encoding:** UTF-8

**Method Names:**
- `list_files` (no parameters required)
- `read_file` (parameters match HTTP request schema)
- `edit_file` (parameters match HTTP request schema)

**Request Format:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "read_file",
  "params": {
    "name": "example.txt",
    "start_line": 5,
    "end_line": 10
  }
}
```

**Success Response Format:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "content": "line 5\nline 6\nline 7\nline 8\nline 9\nline 10",
    "total_lines": 100,
    "range_requested": {
      "start_line": 5,
      "end_line": 10
    }
  }
}
```

### 3.3 Error Response Specification

**HTTP Error Response Format:**
```json
{
  "error": {
    "code": -32001,
    "message": "File 'example.txt' not found",
    "data": {
      "filename": "example.txt",
      "operation": "read_file",
      "timestamp": "2025-06-05T10:30:00Z",
      "details": "Additional context information"
    }
  }
}
```

**JSON-RPC Error Response Format:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32001,
    "message": "File 'example.txt' not found",
    "data": {
      "filename": "example.txt",
      "operation": "read_file",
      "timestamp": "2025-06-05T10:30:00Z"
    }
  }
}
```

**Error Code Mappings:**
- `-32700`: Parse error (invalid JSON)
- `-32600`: Invalid request (missing required fields)
- `-32601`: Method not found
- `-32602`: Invalid params (validation failure)
- `-32603`: Internal error (unexpected server error)
- `-32001`: File system error (file not found, permission denied, etc.)

**HTTP Status Code Mappings:**
- `200`: Success (operation completed)
- `400`: Bad Request (invalid JSON, validation failure)
- `404`: Not Found (file not found)
- `403`: Forbidden (permission denied)
- `413`: Payload Too Large (file size limit exceeded)
- `409`: Conflict (concurrent modification detected)
- `500`: Internal Server Error (unexpected server error)

## 4. Performance & Quality Requirements

### 4.1 Performance Targets

**Response Time Requirements:**
- File reading operations MUST complete within 50 milliseconds for files up to 1MB under load of 10 concurrent requests
- File editing operations MUST complete within 100 milliseconds for files up to 1MB under load of 5 concurrent requests
- File listing operations MUST complete within 20 milliseconds for directories with up to 1000 files
- Server startup and initialization MUST complete within 100 milliseconds
- Graceful shutdown MUST complete within 2000 milliseconds

**Memory Usage Requirements:**
- Idle server memory usage MUST NOT exceed 5MB
- Memory usage per request MUST NOT exceed file size + 1MB overhead
- Server MUST NOT exhibit memory leaks during normal operation
- Memory usage SHOULD return to idle levels within 10 seconds after request completion

**Concurrent Operation Requirements:**
- Server MUST handle concurrent file operations without artificial limits
- Filesystem-level locking MUST prevent race conditions with zero data corruption incidents
- Lock acquisition timeout MUST be 30 seconds maximum
- Multiple server instances MUST coordinate access through filesystem locks

**Binary Size Requirements:**
- Compiled binary size MUST be under 10MB for each target platform
- Binary MUST be statically linked with no external runtime dependencies
- Cross-platform binaries MUST be functionally identical

### 4.2 Security Requirements

**Input Validation Requirements:**
- File names MUST be validated against regex pattern `^[a-zA-Z0-9._-]+$`
- File names MUST NOT exceed 255 characters in length
- Line numbers MUST be validated as positive integers (minimum value 1)
- File content MUST be validated as valid UTF-8 encoding
- JSON request payloads MUST be validated against strict schemas
- Maximum request size MUST be enforced (configurable, default 10MB)

**File System Security Requirements:**
- File operations MUST be strictly confined to specified working directory
- Path traversal attempts (../, ..\, absolute paths) MUST be rejected with error code -32602
- Symbolic links outside working directory MUST NOT be followed
- Temporary files MUST be created with restrictive permissions (600 octal)
- File locks MUST be released automatically on process termination

**Resource Protection Requirements:**
- Maximum file size MUST be enforced (configurable, default 10MB)
- Memory usage monitoring SHOULD trigger warnings at 80% of available memory
- CPU usage SHOULD be monitored to prevent resource exhaustion
- Request size limits MUST match file size limits

### 4.3 Reliability Requirements

**Error Recovery Requirements:**
- File system errors MUST be logged with structured context information
- Partial edit operations MUST be completely rolled back on any failure
- Server MUST continue operating normally after individual operation failures
- Temporary files MUST be cleaned up automatically on operation failure
- File locks MUST be released automatically on unexpected process termination

**Availability Requirements:**
- Server uptime MUST exceed 99.9% during normal operation periods
- Maximum unplanned downtime MUST be under 30 seconds per incident
- Planned restart downtime MUST be under 5 seconds
- Recovery from transient file system errors MUST be automatic
- Server MUST handle graceful shutdown signals (SIGTERM, SIGINT)

**Data Integrity Requirements:**
- Edit operations MUST be atomic (complete success or complete failure)
- File corruption MUST be prevented through checksums or atomic operations
- Concurrent file access MUST NOT result in data loss or corruption
- UTF-8 encoding MUST be preserved exactly in all read/write operations
- Line ending preservation MUST maintain original file format when possible

## 5. Technical Implementation Guide

### 5.1 Technology Stack Requirements

**Primary Language Selection:**
- **Go 1.21+** (RECOMMENDED): Built-in HTTP server, excellent concurrency, fast compilation
- **Rust 1.70+** (ALTERNATIVE): Memory safety, high performance, strong type system
- **Zig 0.11+** (ALTERNATIVE): Simple compilation, explicit control, cross-platform

**Required Standard Library Features:**
- HTTP server implementation with JSON handling
- File I/O operations with atomic write capabilities
- Concurrency primitives (goroutines/threads)
- UTF-8 string processing and validation
- Cross-platform file locking mechanisms
- Command-line argument parsing

**Build Configuration:**
- Static linking for all dependencies
- Cross-compilation for all target platforms
- Optimization level: Release/Production
- Debug symbols: Stripped for production builds

### 5.2 Architecture Pattern Requirements

**Simple Modular Architecture:**

```pseu
