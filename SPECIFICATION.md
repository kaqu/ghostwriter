# File Editing Server - Complete Implementation Specification

**Version:** 1.0  
**Date:** June 4, 2025  
**Document Type:** Single-File Implementation Specification

## 1. System Overview & Scope

### 1.1 Implementation Boundaries

The File Editing Server SHALL implement a high-performance binary providing text file manipulation capabilities for LLM and AI agents. The system MUST operate exclusively within a single designated folder path and SHALL provide flat file access using filename-only references.

**Included in Implementation:**

- Single binary executable with command-line argument configuration
- MCP protocol server with two transport options: HTTP OR stdio JSON-RPC communication
- Three MCP tools: list_files, read_file and edit_file with complete functionality
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
- Linux (x86_64, arm64)
- macOS (x86_64, aarch64)
- macOS (x86_64, arm64)
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
    IF filename is empty OR contains invalid characters THEN
        RETURN tool_error_result("Error: Invalid filename format")
    END IF
    
    IF start_line < 0 OR end_line < 0 THEN
        RETURN tool_error_result("Error: Line numbers must be non-negative integers")
    END IF
    
    IF start_line > end_line THEN
        RETURN tool_error_result("Error: Start line cannot exceed end line")
    END IF
    
    // File access branch
    file_path = join(working_directory, filename)
    
    IF NOT file_exists(file_path) THEN
        RETURN tool_error_result("Error: File '{filename}' not found")
    END IF
    
    file_size = get_file_size(file_path)
    IF file_size > max_file_size_bytes THEN
        RETURN tool_error_result("Error: File size {file_size}MB exceeds maximum limit {max_size}MB")
    END IF
    
    // Content processing branch
    file_content = read_file_bytes(file_path)
    IF NOT is_valid_utf8(file_content) THEN
        RETURN tool_error_result("Error: File contains invalid UTF-8 encoding")
    END IF
    
    lines = split_lines(file_content)
    total_line_count = count(lines)
    
    // Range selection branch (0-based indexing)
    IF start_line IS NULL AND end_line IS NULL THEN
        selected_lines = lines
        result_text = format_file_result(filename, lines, total_line_count)
    ELSE IF start_line IS NOT NULL AND end_line IS NULL THEN
        IF start_line >= total_line_count THEN
            RETURN tool_error_result("Error: Start line {start_line} exceeds file length {total_line_count}")
        END IF
        selected_lines = lines[start_line : total_line_count]
        result_text = format_file_range_result(filename, selected_lines, start_line, total_line_count-1, total_line_count)
    ELSE IF start_line IS NULL AND end_line IS NOT NULL THEN
        IF end_line >= total_line_count THEN
            end_line = total_line_count - 1
        END IF
        selected_lines = lines[0 : end_line + 1]
        result_text = format_file_range_result(filename, selected_lines, 0, end_line, total_line_count)
    ELSE
        IF start_line >= total_line_count THEN
            RETURN tool_error_result("Error: Start line {start_line} exceeds file length {total_line_count}")
        END IF
        actual_end = MIN(end_line, total_line_count - 1)
        selected_lines = lines[start_line : actual_end + 1]
        result_text = format_file_range_result(filename, selected_lines, start_line, actual_end, total_line_count)
    END IF
    
    RETURN tool_success_result(result_text)
END FUNCTION

FUNCTION tool_success_result(text_content):
    RETURN {
        content: [{type: "text", text: text_content}],
        isError: false
    }
END FUNCTION

FUNCTION tool_error_result(error_message):
    RETURN {
        content: [{type: "text", text: error_message}],
        isError: true
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

**Error Response Specifications:**

- File not found: Tool result with `isError: true`, message “Error: File ‘{filename}’ not found”
- Invalid line range: Tool result with `isError: true`, message “Error: Invalid line range: start {start} > end {end}”
- Permission denied: Tool result with `isError: true`, message “Error: Permission denied accessing ‘{filename}’”
- File too large: Tool result with `isError: true`, message “Error: File size {size}MB exceeds maximum limit {max}MB”
- Invalid UTF-8: Tool result with `isError: true`, message “Error: File contains invalid UTF-8 encoding”

**Acceptance Criteria:**

- GIVEN a file “test.txt” exists with content “line1\nline2\nline3” WHEN read_file is called with name=“test.txt” THEN content=“line1\nline2\nline3” AND total_lines=3
- GIVEN a file exists WHEN read_file is called with start_line=2, end_line=3 THEN only lines 2-3 are returned
- GIVEN a file “missing.txt” does not exist WHEN read_file is called THEN error code -32001 is returned
- GIVEN start_line=5, end_line=3 WHEN read_file is called THEN error code -32602 is returned
- GIVEN a file larger than max_file_size WHEN read_file is called THEN error code -32001 is returned

### 2.2 File Listing Operations

**User Story:** As an AI agent, I MUST list all available files in the working directory to discover which files can be read or edited.

**Business Logic Decision Tree:**

```pseudocode
FUNCTION list_files():
    // Directory access validation
    IF NOT can_read_directory(working_directory) THEN
        RETURN tool_error_result("Error: Cannot access working directory")
    END IF
    
    // File enumeration
    TRY
        all_entries = get_directory_entries(working_directory)
        file_list = []
        
        FOR each entry in all_entries DO
            IF entry.is_file AND NOT entry.is_hidden THEN
                file_info = {
                    name: entry.filename,
                    modified: entry.last_modified_time,
                    lines: count_file_lines(entry.path)
                }
                file_list.append(file_info)
            END IF
        END FOR
        
        // Sort by filename for consistent ordering
        sorted_files = sort_by_filename(file_list)
        
        result_text = format_file_list_result(sorted_files)
        RETURN tool_success_result(result_text)
        
    CATCH filesystem_error:
        RETURN tool_error_result("Error: Filesystem error during directory listing: {filesystem_error.message}")
    END TRY
END FUNCTION
```

**File Information Algorithm:**

```pseudocode
FUNCTION get_file_metadata(file_path):
    IF NOT file_exists(file_path) THEN
        RETURN error "File not found"
    END IF
    
    file_stats = get_file_stats(file_path)
    
    RETURN {
        size_bytes: file_stats.size,
        modified_timestamp: file_stats.modified_time,
        readable: test_file_read_access(file_path),
        writable: test_file_write_access(file_path),
        lines: count_file_lines(file_path),
        is_text_file: detect_text_file(file_path)
    }
END FUNCTION

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

**Error Response Specifications:**

- Directory not accessible: Tool result with `isError: true`, message “Error: Cannot access working directory”
- Filesystem error: Tool result with `isError: true`, message “Error: Filesystem error during directory listing”

**Acceptance Criteria:**

- GIVEN working directory contains files [“a.txt”, “b.txt”] WHEN list_files is called THEN both files are returned in sorted order with modification time and line counts
- GIVEN working directory is empty WHEN list_files is called THEN empty files list with total_count=0 is returned
- GIVEN working directory contains subdirectories WHEN list_files is called THEN only files are returned, not directories
- GIVEN working directory contains hidden files WHEN list_files is called THEN hidden files are excluded from results
- GIVEN file is too large or binary WHEN list_files is called THEN lines=-1 is returned for that file
- GIVEN working directory is not readable WHEN list_files is called THEN tool result with isError=true is returned

### 2.3 File Editing Operations

**User Story:** As an AI agent, I MUST edit files using line-based diff operations and append functionality to modify text content precisely while maintaining data integrity.

**Business Logic Decision Tree:**

```pseudocode
FUNCTION edit_file(filename, edits_array):
    // Input validation branch
    IF filename is empty OR contains invalid characters THEN
        RETURN tool_error_result("Error: Invalid filename format")
    END IF
    
    FOR each edit in edits_array DO
        IF edit.line < 0 THEN
            RETURN tool_error_result("Error: Line numbers must be non-negative integers")
        END IF
        IF edit.operation NOT IN ["replace", "insert", "delete"] THEN
            RETURN tool_error_result("Error: Invalid edit operation: {edit.operation}")
        END IF
        IF edit.operation = "delete" AND edit.content IS NOT NULL THEN
            RETURN tool_error_result("Error: Delete operation cannot specify content")
        END IF
    END FOR
    
    // File handling branch
    file_path = join(working_directory, filename)
    file_exists = check_file_exists(file_path)
    
    // Acquire exclusive file lock
    acquire_file_lock(filename)
    TRY
        // Content initialization branch
        IF file_exists THEN
            file_content = read_file_bytes(file_path)
            IF NOT is_valid_utf8(file_content) THEN
                RETURN tool_error_result("Error: File contains invalid UTF-8 encoding")
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
                RETURN tool_error_result("Error: {result.message}")
            END IF
        END FOR
        
        // Atomic write operation
        final_content = join_lines_with_newlines(lines)
        write_result = write_file_atomic(file_path, final_content)
        IF write_result is error THEN
            RETURN tool_error_result("Error: Failed to write file: {write_result.message}")
        END IF
        
        result_text = format_edit_result(filename, abs(count(lines) - original_line_count), count(lines), file_created)
        RETURN tool_success_result(result_text)
        
    CATCH filesystem_error:
        RETURN tool_error_result("Error: Filesystem error: {filesystem_error.message}")
    FINALLY
        release_file_lock(filename)
    END TRY
END FUNCTION
```

**Line Edit Processing Algorithm:**

```pseudocode
FUNCTION apply_line_edit(lines_array, edit):
    line_index = edit.line  // Already 0-based
    line_count = count(lines_array)
    
    SWITCH edit.operation:
        CASE "replace":
            IF line_index < 0 OR line_index >= line_count THEN
                RETURN error_result("Line {edit.line} out of range for replace operation")
            END IF
            lines_array[line_index] = edit.content
            
        CASE "insert":
            IF line_index < 0 OR line_index > line_count THEN
                RETURN error_result("Line {edit.line} out of range for insert operation")
            END IF
            insert_at_position(lines_array, line_index, edit.content)
            
        CASE "delete":
            IF line_index < 0 OR line_index >= line_count THEN
                RETURN error_result("Line {edit.line} out of range for delete operation")
            END IF
            remove_at_position(lines_array, line_index)
            
        DEFAULT:
            RETURN error_result("Unknown edit operation: {edit.operation}")
    END SWITCH
    
    RETURN success_result()
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

- GIVEN a file exists WHEN edit_file is called with line=2, operation=“replace”, content=“new line” THEN line 2 is replaced with “new line”
- GIVEN a file with 5 lines WHEN edit_file is called with line=3, operation=“insert”, content=“inserted” THEN new line is inserted at position 3
- GIVEN a file exists WHEN edit_file is called with line=2, operation=“delete” THEN line 2 is removed
- GIVEN file does not exist WHEN edit_file is called with create_if_missing=true THEN new file is created
- GIVEN multiple edits targeting different lines WHEN edit_file is called THEN all edits are applied successfully in single operation

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

**Filesystem Locking Benefits:**

- Multiple server instances can coordinate access to same files
- OS handles lock cleanup on process termination
- No artificial concurrency limits - filesystem manages queuing
- Cross-process synchronization without shared memory

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
          "modified": {
            "type": "string",
            "format": "date-time",
            "description": "Last modified timestamp"
          },
          "lines": {
            "type": "integer",
            "minimum": -1,
            "description": "Number of lines in file (-1 if unable to count)"
          }
        },
        "required": ["name", "modified", "lines"],
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

**Protocol Version:** JSON-RPC 2.0 (MUST include “jsonrpc”: “2.0”)  
**Transport:** stdin/stdout with line-delimited JSON messages  
**Character Encoding:** UTF-8

**Method Names:**

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

#### 3.3.1 Tool Execution Errors

**Tool errors MUST be returned as successful MCP responses with `isError: true`:**

```json
{
  "jsonrpc": "2.0",
  "id": 1, 
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Error: File 'example.txt' not found"
      }
    ],
    "isError": true
  }
}
```

**Tool Error Categories:**

- File not found: `"Error: File '{filename}' not found"`
- Invalid line range: `"Error: Invalid line range: start {start} > end {end}"`
- Permission denied: `"Error: Permission denied accessing '{filename}'"`
- File too large: `"Error: File size {size}MB exceeds maximum limit {max}MB"`
- Invalid UTF-8: `"Error: File contains invalid UTF-8 encoding"`
- Line out of range: `"Error: Line {line} out of range for {operation} operation"`
- Invalid filename: `"Error: Invalid filename format"`

#### 3.3.2 MCP Protocol Errors

**Protocol-level errors use standard JSON-RPC error responses:**

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32602,
    "message": "Invalid params",
    "data": {
      "tool": "read_file",
      "validation_errors": ["Line numbers must be non-negative integers"]
    }
  }
}
```

**MCP Error Code Mappings:**

- `-32700`: Parse error (invalid JSON)
- `-32600`: Invalid request (missing required MCP fields)
- `-32601`: Method not found (invalid MCP method name)
- `-32602`: Invalid params (tool argument validation failure)
- `-32603`: Internal error (unexpected server error)

#### 3.3.3 HTTP Transport Error Handling

**HTTP status codes are ONLY used for transport-level issues:**

- `200`: Success (MCP communication successful - check `result.isError` for tool errors)
- `400`: Bad Request (invalid JSON, malformed MCP request structure)
- `405`: Method Not Allowed (non-POST request)
- `413`: Payload Too Large (request size exceeds transport limit)
- `500`: Internal Server Error (server cannot process MCP request)

**Important:** File system errors, validation failures, and other tool execution problems MUST return HTTP 200 with `isError: true` in the tool result.

### 3.4 Tool Result Formats

#### 3.4.1 list_files Result Format

**Success Result:**

```json
{
  "content": [
    {
      "type": "text",
      "text": "Files in directory:\n\nname: example.txt, modified: 2025-06-04T10:30:00Z, lines: 42\nname: config.ini, modified: 2025-06-04T09:15:00Z, lines: 15\nname: data.json, modified: 2025-06-04T11:00:00Z, lines: 128\n\nTotal files: 3"
    }
  ],
  "isError": false
}
```

**Structured Data Format (within text):**

```
Files in directory:

name: {filename}, modified: {ISO8601_timestamp}, lines: {line_count}
name: {filename}, modified: {ISO8601_timestamp}, lines: {line_count}
...

Total files: {count}
```

#### 3.4.2 read_file Result Format

**Success Result (entire file):**

```json
{
  "content": [
    {
      "type": "text", 
      "text": "File: example.txt (42 lines)\n\nline 1 content\nline 2 content\nline 3 content\n..."
    }
  ],
  "isError": false
}
```

**Success Result (line range):**

```json
{
  "content": [
    {
      "type": "text",
      "text": "File: example.txt (lines 5-10 of 42 total)\n\nline 6 content\nline 7 content\nline 8 content\nline 9 content\nline 10 content\nline 11 content"
    }
  ],
  "isError": false
}
```

**Structured Format:**

```
File: {filename} (lines {start}-{end} of {total} total)

{file_content}
```

Or for entire file:

```
File: {filename} ({total} lines)

{file_content}
```

#### 3.4.3 edit_file Result Format

**Success Result:**

```json
{
  "content": [
    {
      "type": "text",
      "text": "File edited successfully: example.txt\nLines modified: 3\nTotal lines: 45\nFile created: false"
    }
  ],
  "isError": false
}
```

**Success Result (new file):**

```json
{
  "content": [
    {
      "type": "text", 
      "text": "File edited successfully: newfile.txt\nLines modified: 5\nTotal lines: 5\nFile created: true"
    }
  ],
  "isError": false
}
```

**Structured Format:**

```
File edited successfully: {filename}
Lines modified: {lines_modified}
Total lines: {total_lines}
File created: {true|false}
```

#### 3.4.4 Error Result Format

**Tool Error Result:**

```json
{
  "content": [
    {
      "type": "text",
      "text": "Error: File 'missing.txt' not found"
    }
  ],
  "isError": true
}
```

**Error Format:**

```
Error: {error_message}
```

**HTTP Status Codes (HTTP Transport Only):**

- `200`: Success (MCP response in body, check result.isError for tool errors)
- `400`: Bad Request (invalid JSON, malformed MCP request)
- `405`: Method Not Allowed (non-POST request)
- `413`: Payload Too Large (request size exceeds limit)
- `500`: Internal Server Error (unexpected server error)

## 4. Performance & Quality Requirements

### 4.1 Performance Targets

**Response Time Requirements:**

- File reading operations MUST complete within 50 milliseconds for files up to 1MB
- File editing operations MUST complete within 100 milliseconds for files up to 1MB
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
- Line numbers MUST be validated as non-negative integers (minimum value 0)
- File content MUST be validated as valid UTF-8 encoding
- JSON request payloads MUST be validated against strict schemas
- Maximum request size MUST be enforced (configurable, default 10MB)

**File System Security Requirements:**

- File operations MUST be strictly confined to specified working directory
- Path traversal attempts (../, .., absolute paths) MUST be rejected with error code -32602
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

### 5.1 Architecture Pattern Requirements

**Simple Modular Architecture:**

```pseudocode
// Core MCP Server Structure
STRUCTURE MCPServer:
    working_directory: string
    max_file_size: integer
    operation_timeout: duration
    transport_type: string
    server_info: mcp_server_info
    
    FUNCTION initialize(config: server_config) -> initialization_result
    FUNCTION start() -> void
    FUNCTION stop() -> void
    FUNCTION handle_mcp_request(mcp_request) -> mcp_response

// MCP Tool Operations Module
STRUCTURE MCPTools:
    FUNCTION list_files(working_dir: string) -> tool_result
    FUNCTION read_file(file_path: string, start_line: integer, end_line: integer) -> tool_result
    FUNCTION edit_file(file_path: string, edits: edit_array, append: string) -> tool_result
    FUNCTION validate_tool_arguments(tool_name: string, arguments: map) -> validation_result

// MCP Protocol Handlers
STRUCTURE MCPHTTPHandler:
    FUNCTION start_mcp_server(port: integer) -> server_handle
    FUNCTION handle_mcp_over_http(http_request) -> http_response

STRUCTURE MCPStdioHandler:
    FUNCTION start_mcp_stdio_loop() -> void
    FUNCTION handle_mcp_over_stdio(jsonrpc_request) -> jsonrpc_response

// File System Operations (unchanged)
STRUCTURE FileSystemUtils:
    FUNCTION read_file_content(file_path: string) -> file_content
    FUNCTION write_file_atomic(file_path: string, content: string) -> write_result
    FUNCTION acquire_file_lock(file_path: string) -> lock_handle
    FUNCTION release_file_lock(lock_handle) -> void
```

**Component Relationships:**

- MCPServer coordinates all MCP protocol handling and tool execution
- FileOperations handles core business logic for the three tools
- Transport handlers manage MCP protocol over HTTP vs stdio
- FileSystemUtils provides low-level file operations with error handling

### 5.2 Data Flow Implementation

**Read Operation Flow:**

```pseudocode
FUNCTION handle_read_request(transport_request):
    // Step 1: Input parsing and validation
    parsed_request = parse_json(transport_request.body)
    validation_result = validate_read_request(parsed_request)
    IF validation_result.has_errors THEN
        RETURN create_error_response(validation_result.errors)
    END IF
    
    // Step 2: Business logic execution
    read_result = file_service.read_file(
        parsed_request.name,
        parsed_request.start_line,
        parsed_request.end_line
    )
    
    // Step 3: Response formatting
    IF read_result.is_success THEN
        response_data = {
            content: read_result.content,
            total_lines: read_result.total_lines,
            range_requested: read_result.range_info
        }
        RETURN create_success_response(response_data)
    ELSE
        RETURN create_error_response(read_result.error)
    END IF
END FUNCTION
```

**Edit Operation Flow:**

```pseudocode
FUNCTION handle_edit_request(transport_request):
    // Step 1: Input parsing and validation
    parsed_request = parse_json(transport_request.body)
    validation_result = validate_edit_request(parsed_request)
    IF validation_result.has_errors THEN
        RETURN create_error_response(validation_result.errors)
    END IF
    
    // Step 2: Pre-operation checks
    file_path = resolve_file_path(parsed_request.name)
    access_check = verify_file_access(file_path, parsed_request.create_if_missing)
    IF access_check.has_errors THEN
        RETURN create_error_response(access_check.errors)
    END IF
    
    // Step 3: Atomic edit execution
    lock_handle = acquire_file_lock(parsed_request.name)
    TRY
        edit_result = file_service.edit_file(
            parsed_request.name,
            parsed_request.edits,
            parsed_request.append,
            parsed_request.create_if_missing
        )
        
        IF edit_result.is_success THEN
            response_data = {
                success: true,
                lines_modified: edit_result.lines_modified,
                file_created: edit_result.file_created,
                new_total_lines: edit_result.new_total_lines
            }
            RETURN create_success_response(response_data)
        ELSE
            RETURN create_error_response(edit_result.error)
        END IF
    FINALLY
        release_file_lock(lock_handle)
    END TRY
END FUNCTION
```

### 5.3 Concurrency Implementation

**Filesystem-Level Lock Manager:**

```pseudocode
CLASS FilesystemLockManager:
    PRIVATE lock_timeout: duration
    
    FUNCTION acquire_file_lock(file_path: string) -> lock_handle:
        start_time = current_time()
        
        TRY
            // Use OS-level file locking (flock, fcntl, or LockFileEx)
            lock_handle = create_exclusive_file_lock(file_path)
            
            WHILE current_time() - start_time < lock_timeout DO
                lock_result = try_acquire_lock(lock_handle)
                
                IF lock_result.success THEN
                    log_lock_acquired(file_path, current_time() - start_time)
                    RETURN lock_handle
                END IF
                
                sleep(10_milliseconds)
            END WHILE
            
            close_lock_handle(lock_handle)
            THROW timeout_error("Failed to acquire filesystem lock within timeout")
            
        CATCH filesystem_error:
            THROW lock_error("Filesystem lock error: {filesystem_error}")
        END TRY
    END FUNCTION
    
    FUNCTION release_file_lock(lock_handle: lock_handle) -> void:
        TRY
            release_exclusive_lock(lock_handle)
            close_lock_handle(lock_handle)
            log_lock_released(lock_handle.file_path)
        CATCH release_error:
            log_error("Failed to release filesystem lock", release_error)
        END TRY
    END FUNCTION
    
    // No cleanup needed - OS handles lock cleanup on process termination
END CLASS
```

### 5.4 Error Handling Strategy

**Error Classification System:**

```pseudocode
ABSTRACT CLASS OperationError:
    error_code: integer
    error_message: string
    error_category: string  // "client", "server", "system"
    error_details: map<string, any>
    timestamp: datetime

CLASS ClientError EXTENDS OperationError:
    // 400-level errors: invalid input, validation failures
    CONSTRUCTOR(message: string, details: map):
        error_code = -32602
        error_category = "client"
        error_message = message
        error_details = details
        timestamp = current_time()
    END CONSTRUCTOR

CLASS ServerError EXTENDS OperationError:
    // 500-level errors: internal server problems
    CONSTRUCTOR(message: string, details: map):
        error_code = -32603
        error_category = "server"
        error_message = message
        error_details = details
        timestamp = current_time()
    END CONSTRUCTOR

CLASS SystemError EXTENDS OperationError:
    // File system, OS-level errors
    CONSTRUCTOR(message: string, details: map):
        error_code = -32001
        error_category = "system"
        error_message = message
        error_details = details
        timestamp = current_time()
    END CONSTRUCTOR
```

**Error Recovery Procedures:**

```pseudocode
FUNCTION handle_operation_error(error: OperationError, context: operation_context):
    // Log error with full context
    log_entry = {
        error_code: error.error_code,
        error_message: error.error_message,
        error_category: error.error_category,
        operation: context.operation_name,
        filename: context.filename,
        request_id: context.request_id,
        timestamp: error.timestamp,
        stack_trace: get_stack_trace()
    }
    logger.error(log_entry)
    
    // Update metrics
    metrics_collector.increment_error_count(error.error_category)
    metrics_collector.record_error_rate(context.operation_name)
    
    // Attempt recovery based on error category
    SWITCH error.error_category:
        CASE "client":
            // No recovery needed - return error to client
            RETURN create_client_error_response(error)
            
        CASE "server":
            // Log for debugging, attempt graceful degradation
            attempt_graceful_degradation(context)
            RETURN create_server_error_response(error)
            
        CASE "system":
            // Attempt system-level recovery
            recovery_result = attempt_system_recovery(error, context)
            IF recovery_result.success THEN
                // Retry operation once
                RETURN retry_operation(context)
            ELSE
                RETURN create_system_error_response(error)
            END IF
    END SWITCH
END FUNCTION
```

### 5.5 Performance Optimization Guidelines

**Memory Optimization Strategies:**

```pseudocode
// Streaming file reader for large files
FUNCTION read_file_lines_streaming(file_path: string, start_line: integer, end_line: integer):
    IF end_line - start_line > 1000 THEN
        // Use streaming approach for large ranges
        RETURN read_lines_with_streaming(file_path, start_line, end_line)
    ELSE
        // Use in-memory approach for small ranges
        RETURN read_lines_in_memory(file_path, start_line, end_line)
    END IF
END FUNCTION

FUNCTION read_lines_with_streaming(file_path: string, start_line: integer, end_line: integer):
    file_stream = open_file_stream(file_path)
    result_lines = []
    current_line_number = 1
    
    TRY
        WHILE NOT file_stream.is_end_of_file() DO
            line = file_stream.read_line()
            
            IF current_line_number >= start_line AND current_line_number <= end_line THEN
                result_lines.append(line)
            END IF
            
            current_line_number += 1
            
            IF current_line_number > end_line THEN
                BREAK
            END IF
        END WHILE
    FINALLY
        file_stream.close()
    END TRY
    
    RETURN result_lines
END FUNCTION
```

**I/O Optimization Patterns:**

```pseudocode
// Buffered writing for multiple edits
FUNCTION write_file_optimized(file_path: string, content: string):
    content_size = size_of(content)
    
    IF content_size < 64_KB THEN
        // Small files: direct write
        RETURN write_file_direct(file_path, content)
    ELSE
        // Large files: buffered write
        RETURN write_file_buffered(file_path, content)
    END IF
END FUNCTION

// Atomic operations with temporary files
FUNCTION write_file_atomic_optimized(file_path: string, content: string):
    temp_path = file_path + ".tmp." + generate_uuid()
    
    TRY
        // Write to temporary file with optimal buffer size
        buffer_size = calculate_optimal_buffer_size(size_of(content))
        write_file_with_buffer(temp_path, content, buffer_size)
        
        // Verify write integrity
        verify_file_integrity(temp_path, content)
        
        // Atomic rename
        atomic_rename(temp_path, file_path)
        
        RETURN success_result()
    CATCH any_error:
        cleanup_temp_file(temp_path)
        RETURN error_result(any_error)
    END TRY
END FUNCTION
```

## 6. Integration Contracts & Protocols

### 6.1 Server Startup Contract

**Command Line Validation Algorithm:**

```pseudocode
FUNCTION validate_startup_arguments(args: command_line_args):
    validation_errors = []
    
    // Required arguments validation
    IF args.working_dir IS NULL OR args.working_dir = "" THEN
        validation_errors.append("--dir argument is required")
    ELSE
        dir_validation = validate_working_directory(args.working_dir)
        IF NOT dir_validation.is_valid THEN
            validation_errors.append(dir_validation.error_message)
        END IF
    END IF
    
    // Optional arguments validation
    IF args.transport NOT IN ["http", "stdio"] THEN
        validation_errors.append("--transport must be one of: http, stdio")
    END IF
    
    IF args.port < 1024 OR args.port > 65535 THEN
        validation_errors.append("--port must be between 1024 and 65535")
    END IF
    
    IF args.max_size < 1 OR args.max_size > 100 THEN
        validation_errors.append("--max-size must be between 1 and 100 MB")
    END IF
    
    IF args.timeout < 1 OR args.timeout > 300 THEN
        validation_errors.append("--timeout must be between 1 and 300 seconds")
    END IF
    
    RETURN validation_result(validation_errors)
END FUNCTION
```

**Server Initialization Sequence:**

```pseudocode
FUNCTION initialize_server(validated_config: server_config):
    startup_start_time = current_time()
    
    // Step 1: Validate working directory access
    IF NOT directory_exists(validated_config.working_dir) THEN
        RETURN initialization_failure("working_directory", "Directory does not exist")
    END IF
    
    IF NOT directory_writable(validated_config.working_dir) THEN
        RETURN initialization_failure("working_directory", "Directory not writable")
    END IF
    
    // Step 2: Initialize file operations
    file_ops = create_file_operations(validated_config.working_dir, validated_config.max_size)
    
    // Step 3: Start selected transport
    SWITCH validated_config.transport:
        CASE "http":
            transport_result = start_http_server(validated_config.port, file_ops)
        CASE "stdio":
            transport_result = start_stdio_handler(file_ops)
        DEFAULT:
            RETURN initialization_failure("transport", "Invalid transport type")
    END SWITCH
    
    IF NOT transport_result.success THEN
        RETURN initialization_failure("transport", transport_result.error)
    END IF
    
    startup_duration = current_time() - startup_start_time
    log_server_ready(startup_duration, validated_config.transport)
    
    // Verify startup time requirement
    IF startup_duration > 100_milliseconds THEN
        log_warning("Startup time exceeded target", startup_duration)
    END IF
    
    RETURN initialization_success()
END FUNCTION
```

### 6.2 Transport Protocol Contracts

**MCP HTTP Transport Implementation Contract:**

```pseudocode
INTERFACE MCPHTTPTransport:
    FUNCTION start_mcp_server(port: integer, mcp_handler: mcp_request_handler) -> server_handle
    FUNCTION set_request_timeout(timeout_seconds: integer) -> void
    FUNCTION set_max_request_size(size_bytes: integer) -> void
    FUNCTION stop_server(graceful_timeout: duration) -> shutdown_result

// MCP HTTP request handling contract
FUNCTION handle_mcp_http_request(http_request: http_request) -> http_response:
    // Request preprocessing
    IF http_request.method != "POST" THEN
        RETURN http_error_response(405, "Method not allowed")
    END IF
    
    IF http_request.content_length > max_request_size THEN
        RETURN http_error_response(413, "Request too large")
    END IF
    
    IF http_request.content_type != "application/json" THEN
        RETURN http_error_response(400, "Content-Type must be application/json")
    END IF
    
    // MCP request processing
    TRY
        mcp_request = parse_jsonrpc(http_request.body)
        validate_mcp_request_structure(mcp_request)
        
        mcp_response = process_mcp_request(mcp_request)
        
        RETURN http_success_response(200, mcp_response)
        
    CATCH json_parse_error:
        mcp_error = create_mcp_error(-32700, "Parse error", mcp_request.id)
        RETURN http_success_response(200, mcp_error)
    CATCH validation_error:
        mcp_error = create_mcp_error(-32600, "Invalid Request", mcp_request.id)
        RETURN http_success_response(200, mcp_error)
    CATCH any_other_error:
        log_unexpected_error(any_other_error)
        mcp_error = create_mcp_error(-32603, "Internal error", mcp_request.id)
        RETURN http_success_response(200, mcp_error)
    END TRY
END FUNCTION
```

**MCP stdio Implementation Contract:**

```pseudocode
INTERFACE MCPStdioTransport:
    FUNCTION start_mcp_stdio_loop(mcp_handler: mcp_request_handler) -> void
    FUNCTION read_mcp_request() -> mcp_request
    FUNCTION write_mcp_response(response: mcp_response) -> void
    FUNCTION stop_stdio_loop() -> void

// MCP stdio request handling contract
FUNCTION handle_mcp_stdio_request(stdin_line: string) -> string:
    TRY
        // Parse MCP JSON-RPC request
        mcp_request = parse_jsonrpc(stdin_line)
        validate_mcp_request_structure(mcp_request)
        
        // Process MCP request
        mcp_response = process_mcp_request(mcp_request)
        
        RETURN json_encode(mcp_response)
        
    CATCH json_parse_error:
        mcp_error = create_mcp_error(-32700, "Parse error", null)
        RETURN json_encode(mcp_error)
    CATCH validation_error:
        mcp_error = create_mcp_error(-32600, "Invalid Request", null)
        RETURN json_encode(mcp_error)
    CATCH any_other_error:
        log_unexpected_error(any_other_error)
        mcp_error = create_mcp_error(-32603, "Internal error", mcp_request.id)
        RETURN json_encode(mcp_error)
    END TRY
END FUNCTION

// Core MCP request processor
FUNCTION process_mcp_request(mcp_request: mcp_request) -> mcp_response:
    SWITCH mcp_request.method:
        CASE "initialize":
            RETURN handle_mcp_initialize(mcp_request.params, mcp_request.id)
        CASE "tools/list":
            RETURN handle_tools_list(mcp_request.id)
        CASE "tools/call":
            RETURN handle_tool_call(mcp_request.params, mcp_request.id)
        CASE "notifications/initialized":
            handle_initialized_notification()
            RETURN null  // Notifications don't require responses
        DEFAULT:
            RETURN create_mcp_error(-32601, "Method not found", mcp_request.id)
    END SWITCH
END FUNCTION

FUNCTION handle_mcp_initialize(params: initialize_params, request_id: id) -> mcp_response:
    // Validate protocol version
    IF params.protocolVersion != "2024-11-05" THEN
        RETURN create_mcp_error(-32602, "Unsupported protocol version", request_id)
    END IF
    
    // Validate client capabilities
    IF NOT params.capabilities OR NOT params.capabilities.tools THEN
        RETURN create_mcp_error(-32602, "Client must support tools capability", request_id)
    END IF
    
    // Return server capabilities and info
    RETURN create_mcp_success({
        protocolVersion: "2024-11-05",
        capabilities: {tools: {}},
        serverInfo: {
            name: "file-editing-server",
            version: "1.0.0",
            description: "High-performance file editing server for AI agents"
        }
    }, request_id)
END FUNCTION

FUNCTION handle_tools_list(request_id: id) -> mcp_response:
    tool_definitions = get_tool_definitions_with_annotations()
    RETURN create_mcp_success({tools: tool_definitions}, request_id)
END FUNCTION

FUNCTION handle_tool_call(params: tool_call_params, request_id: id) -> mcp_response:
    tool_name = params.name
    tool_arguments = params.arguments
    
    // Validate tool exists
    IF tool_name NOT IN ["list_files", "read_file", "edit_file"] THEN
        RETURN create_mcp_error(-32602, "Unknown tool: {tool_name}", request_id)
    END IF
    
    // Execute tool and return result
    tool_result = execute_tool(tool_name, tool_arguments)
    RETURN create_mcp_success(tool_result, request_id)
END FUNCTION
```

### 6.3 Client Integration Patterns

**HTTP Client Usage Examples:**

```pseudocode
// Simple HTTP client pattern
FUNCTION call_file_server_http(base_url: string, operation: string, params: map) -> response:
    request_url = base_url + "/" + operation
    request_headers = {
        "Content-Type": "application/json; charset=utf-8",
        "Accept": "application/json"
    }
    request_body = json_encode(params)
    
    http_response = http_post(request_url, request_headers, request_body)
    
    IF http_response.status_code = 200 THEN
        response_data = json_decode(http_response.body)
        RETURN success_result(response_data)
    ELSE
        error_data = json_decode(http_response.body)
        RETURN error_result(error_data.error)
    END IF
END FUNCTION

// Usage examples
list_result = call_file_server_http(
    "http://localhost:8080",
    "list_files", 
    {}
)

read_result = call_file_server_http(
    "http://localhost:8080",
    "read_file", 
    {name: "example.txt", start_line: 5, end_line: 10}
)

edit_result = call_file_server_http(
    "http://localhost:8080",
    "edit_file",
    {
        name: "example.txt",
        edits: [{line: 5, content: "new content", operation: "replace"}],
        create_if_missing: true
    }
)
```

**stdio Client Usage Examples:**

```pseudocode
// stdio JSON-RPC client pattern  
FUNCTION call_file_server_stdio(process_handle: process, method: string, params: map) -> response:
    request_id = generate_request_id()
    jsonrpc_request = {
        jsonrpc: "2.0",
        id: request_id,
        method: method,
        params: params
    }
    
    // Send request
    request_line = json_encode(jsonrpc_request) + "\n"
    write_to_process_stdin(process_handle, request_line)
    
    // Read response
    response_line = read_from_process_stdout(process_handle)
    jsonrpc_response = json_decode(response_line)
    
    IF jsonrpc_response.error IS NULL THEN
        RETURN success_result(jsonrpc_response.result)
    ELSE
        RETURN error_result(jsonrpc_response.error)
    END IF
END FUNCTION

// Process management
FUNCTION start_file_server_process(working_dir: string) -> process_handle:
    command_args = [
        "file-editor",
        "--dir=" + working_dir,
        "--transport=stdio"
    ]
    
    process_handle = start_process(
        command_args,
        stdin_mode="pipe",
        stdout_mode="pipe", 
        stderr_mode="pipe"
    )
    
    // Verify server is ready
    health_check_result = call_file_server_stdio(
        process_handle,
        "read_file",
        {name: ".health_check_file"}
    )
    
    RETURN process_handle
END FUNCTION
```

### 6.4 Testing Integration Requirements

**Unit Test Coverage Requirements:**

```pseudocode
// Required test categories with minimum coverage
test_categories = {
    "input_validation": {
        minimum_coverage: 100,
        test_cases: [
            "valid_filename_acceptance",
            "invalid_filename_rejection", 
            "line_number_validation",
            "request_size_limits",
            "utf8_encoding_validation",
            "mcp_request_structure_validation"
        ]
    },
    "mcp_tools": {
        minimum_coverage: 95,
        test_cases: [
            "successful_file_listing",
            "successful_file_reading",
            "line_range_processing",
            "edit_operation_types",
            "atomic_write_behavior",
            "tool_error_handling"
        ]
    },
    "concurrency": {
        minimum_coverage: 90,
        test_cases: [
            "concurrent_tool_calls",
            "exclusive_write_locking",
            "filesystem_lock_behavior",
            "multi_instance_coordination"
        ]
    },
    "mcp_protocol": {
        minimum_coverage: 95,
        test_cases: [
            "mcp_initialization_sequence",
            "tools_list_response",
            "tool_call_execution",
            "error_response_formatting",
            "jsonrpc_compliance"
        ]
    }
}
```

**Integration Test Scenarios:**

```pseudocode
// End-to-end MCP integration test specification
FUNCTION test_complete_mcp_workflow():
    // Setup
    test_server = start_mcp_test_server(temp_directory)
    
    // Test MCP initialization sequence
    init_result = call_mcp_method(test_server, "initialize", {
        protocolVersion: "2024-11-05",
        capabilities: {tools: {}},
        clientInfo: {name: "test-client", version: "1.0.0"}
    })
    assert_success(init_result)
    assert_equals(init_result.result.protocolVersion, "2024-11-05")
    assert_not_null(init_result.result.capabilities.tools)
    
    // Send initialized notification
    send_mcp_notification(test_server, "notifications/initialized", {})
    
    // Test tools listing with annotations
    tools_result = call_mcp_method(test_server, "tools/list", {})
    assert_success(tools_result)
    assert_equals(length(tools_result.result.tools), 3)
    
    expected_tools = ["list_files", "read_file", "edit_file"]
    FOR each tool_name in expected_tools DO
        tool_def = find_tool_by_name(tools_result.result.tools, tool_name)
        assert_not_null(tool_def)
        assert_not_null(tool_def.annotations)
        assert_has_property(tool_def.annotations, "readOnlyHint")
        assert_has_property(tool_def.annotations, "destructiveHint")
    END FOR
    
    // Verify tool annotations are correct
    list_files_tool = find_tool_by_name(tools_result.result.tools, "list_files")
    assert_equals(list_files_tool.annotations.readOnlyHint, true)
    assert_equals(list_files_tool.annotations.destructiveHint, false)
    
    edit_file_tool = find_tool_by_name(tools_result.result.tools, "edit_file")
    assert_equals(edit_file_tool.annotations.readOnlyHint, false)
    assert_equals(edit_file_tool.annotations.destructiveHint, false)
    
    // Test file operations workflow
    initial_list = call_mcp_tool(test_server, "list_files", {})
    assert_tool_success(initial_list)
    list_data = parse_tool_result(initial_list)
    assert_contains(list_data, "Total files: 0")
    
    create_file_result = call_mcp_tool(test_server, "edit_file", {
        name: "test.txt",
        edits: [
            {line: 0, content: "line1", operation: "insert"},
            {line: 1, content: "line2", operation: "insert"},
            {line: 2, content: "line3", operation: "insert"},
            {line: 3, content: "line4", operation: "insert"},
            {line: 4, content: "line5", operation: "insert"}
        ]
    })
    assert_tool_success(create_file_result)
    create_data = parse_tool_result(create_file_result)
    assert_contains(create_data, "File created: true")
    
    updated_list = call_mcp_tool(test_server, "list_files", {})
    assert_tool_success(updated_list)
    list_data = parse_tool_result(updated_list)
    assert_contains(list_data, "Total files: 1")
    assert_contains(list_data, "name: test.txt")
    
    read_result = call_mcp_tool(test_server, "read_file", {name: "test.txt"})
    assert_tool_success(read_result)
    read_data = parse_tool_result(read_result)
    assert_contains(read_data, "line1")
    assert_contains(read_data, "line5")
    
    edit_result = call_mcp_tool(test_server, "edit_file", {
        name: "test.txt",
        edits: [{
            line: 2,
            content: "modified line 3",
            operation: "replace"
        }]
    })
    assert_tool_success(edit_result)
    
    verify_result = call_mcp_tool(test_server, "read_file", {
        name: "test.txt",
        start_line: 2,
        end_line: 2
    })
    assert_tool_success(verify_result)
    verify_data = parse_tool_result(verify_result)
    assert_contains(verify_data, "modified line 3")
    
    // Test error handling through tool results
    error_result = call_mcp_tool(test_server, "read_file", {name: "missing.txt"})
    assert_tool_error(error_result)
    error_data = parse_tool_result(error_result)
    assert_contains(error_data, "Error: File 'missing.txt' not found")
    
    // Cleanup
    stop_mcp_test_server(test_server)
END FUNCTION

// Helper functions for MCP testing
FUNCTION call_mcp_tool(server: test_server, tool_name: string, arguments: map) -> mcp_response:
    RETURN call_mcp_method(server, "tools/call", {
        name: tool_name,
        arguments: arguments
    })
END FUNCTION

FUNCTION assert_tool_success(mcp_response: mcp_response) -> void:
    assert_success(mcp_response)
    assert_equals(mcp_response.result.isError, false)
END FUNCTION

FUNCTION assert_tool_error(mcp_response: mcp_response) -> void:
    assert_success(mcp_response)  // MCP call succeeded
    assert_equals(mcp_response.result.isError, true)  // But tool failed
END FUNCTION

FUNCTION parse_tool_result(mcp_response: mcp_response) -> parsed_data:
    tool_text = mcp_response.result.content[0].text
    RETURN tool_text
END FUNCTION
```

**Performance Test Requirements:**

```pseudocode
// Performance benchmark specifications
FUNCTION benchmark_file_operations():
    test_scenarios = [
        {
            name: "file_listing_performance",
            directory_size: "100 files",
            operation: "list_files",
            target_time: "20ms",
            iterations: 1000
        },
        {
            name: "server_startup_performance",
            operation: "server_start",
            target_time: "100ms",
            iterations: 10
        },
        {
            name: "small_file_read_performance",
            file_size: "1KB",
            operation: "read_file",
            target_time: "5ms",
            iterations: 1000
        },
        {
            name: "medium_file_read_performance", 
            file_size: "1MB",
            operation: "read_file",
            target_time: "50ms",
            iterations: 100
        },
        {
            name: "edit_operation_performance",
            file_size: "1MB", 
            operation: "edit_file",
            target_time: "100ms",
            iterations: 100
        },
        {
            name: "filesystem_lock_performance",
            file_size: "100KB",
            operation: "concurrent_edits",
            concurrent_processes: 5,
            target_time: "200ms",
            iterations: 50
        }
    ]
    
    FOR each scenario in test_scenarios DO
        benchmark_result = execute_performance_test(scenario)
        assert_performance_target_met(benchmark_result, scenario.target_time)
        log_performance_metrics(scenario.name, benchmark_result)
    END FOR
END FUNCTION
```