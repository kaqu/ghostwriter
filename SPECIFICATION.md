# File Editing Server - Complete Implementation Specification

**Version:** 1.0  
**Date:** June 4, 2025  
**Document Type:** Single-File Implementation Specification

## 1. System Overview & Scope

### 1.1 Implementation Boundaries

The File Editing Server SHALL implement a high-performance binary providing text file manipulation capabilities for LLM and AI agents. The system MUST operate exclusively within a single designated folder path and SHALL provide flat file access using filename-only references.

**Included in Implementation:**

- Single binary executable with command-line argument configuration
- Two transport options: HTTP REST API OR stdio JSON-RPC communication
- Two file operations: read_file and edit_file with complete functionality
- Text file reading with optional line range selection capabilities
- Text file editing via line-based diff operations and append functionality
- New file creation within designated folder boundaries
- Flat file storage abstraction using filename-only references
- Cross-platform file system operations supporting Windows, macOS, Linux
- UTF-8 text encoding support with validation
- Concurrent request handling with file-level locking mechanisms
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
- `--max-file-size=<mb>`: Maximum file size in megabytes [default: 10, range: 1-100]
- `--max-concurrent=<count>`: Maximum concurrent operations [default: 10, range: 1-100]
- `--timeout=<seconds>`: Operation timeout in seconds [default: 30, range: 5-300]

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

**Error Response Specifications:**

- File not found: Error code -32001, HTTP status 404, message “File ‘{filename}’ not found”
- Invalid line range: Error code -32602, HTTP status 400, message “Invalid line range: start {start} > end {end}”
- Permission denied: Error code -32001, HTTP status 403, message “Permission denied accessing ‘{filename}’”
- File too large: Error code -32001, HTTP status 413, message “File size {size}MB exceeds maximum limit {max}MB”
- Invalid UTF-8: Error code -32001, HTTP status 400, message “File contains invalid UTF-8 encoding”

**Acceptance Criteria:**

- GIVEN a file “test.txt” exists with content “line1\nline2\nline3” WHEN read_file is called with name=“test.txt” THEN content=“line1\nline2\nline3” AND total_lines=3
- GIVEN a file exists WHEN read_file is called with start_line=2, end_line=3 THEN only lines 2-3 are returned
- GIVEN a file “missing.txt” does not exist WHEN read_file is called THEN error code -32001 is returned
- GIVEN start_line=5, end_line=3 WHEN read_file is called THEN error code -32602 is returned
- GIVEN a file larger than max_file_size WHEN read_file is called THEN error code -32001 is returned

### 2.2 File Editing Operations

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

- GIVEN a file exists WHEN edit_file is called with line=2, operation=“replace”, content=“new line” THEN line 2 is replaced with “new line”
- GIVEN a file with 5 lines WHEN edit_file is called with line=3, operation=“insert”, content=“inserted” THEN new line is inserted at position 3
- GIVEN a file exists WHEN edit_file is called with line=2, operation=“delete” THEN line 2 is removed
- GIVEN file does not exist WHEN edit_file is called with create_if_missing=true THEN new file is created
- GIVEN multiple edits targeting different lines WHEN edit_file is called THEN all edits are applied successfully in single operation

### 2.3 Concurrency and State Management

**File Locking Algorithm:**

```pseudocode
GLOBAL lock_map = thread_safe_map()

FUNCTION acquire_file_lock(filename):
    WHILE true DO
        success = lock_map.try_insert(filename, current_thread_id)
        IF success THEN
            BREAK
        END IF
        sleep(1_millisecond)
    END WHILE
END FUNCTION

FUNCTION release_file_lock(filename):
    lock_map.remove(filename)
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

#### 3.1.1 Read File Endpoint

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
      "pattern": "^[^/\\\\:*?\"<>|]+$",
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

#### 3.1.2 Edit File Endpoint

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
      "pattern": "^[^/\\\\:*?\"<>|]+$",
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

**HTTP Error Response Format:**

```json
{
  "error": {
    "code": -32001,
    "message": "File 'example.txt' not found",
    "data": {
      "filename": "example.txt",
      "operation": "read_file",
      "timestamp": "2025-06-04T10:30:00Z",
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
      "timestamp": "2025-06-04T10:30:00Z"
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

- File reading operations MUST complete within 50 milliseconds for files up to 1MB
- File editing operations MUST complete within 100 milliseconds for files up to 1MB
- Server startup and initialization MUST complete within 1000 milliseconds
- Graceful shutdown MUST complete within 5000 milliseconds

**Memory Usage Requirements:**

- Base server memory usage MUST NOT exceed 10MB at startup
- Memory usage per active request MUST NOT exceed 2MB for files up to 1MB
- Total memory usage MUST NOT exceed 50MB under maximum concurrent load
- Memory leaks MUST NOT occur during normal operation (zero growth after 1000 operations)

**Concurrent Operation Requirements:**

- Server MUST handle exactly 10 concurrent file operations without performance degradation
- File locking MUST prevent race conditions with zero data corruption incidents
- Lock acquisition timeout MUST be 30 seconds maximum
- Queue depth for pending operations MUST NOT exceed 20 requests

**Binary Size Requirements:**

- Compiled binary size MUST be under 10MB for each target platform
- Binary MUST be statically linked with no external runtime dependencies
- Cross-platform binaries MUST be functionally identical

### 4.2 Security Requirements

**Input Validation Requirements:**

- File names MUST be validated against regex pattern `^[^/\\:*?"<>|]+$`
- File names MUST NOT exceed 255 characters in length
- Line numbers MUST be validated as positive integers (minimum value 1)
- File content MUST be validated as valid UTF-8 encoding
- JSON request payloads MUST be validated against strict schemas
- Maximum request size MUST be enforced at 50MB

**File System Security Requirements:**

- File operations MUST be strictly confined to specified working directory
- Path traversal attempts (../, .., absolute paths) MUST be rejected with error code -32602
- Symbolic links outside working directory MUST NOT be followed
- Temporary files MUST be created with restrictive permissions (600 octal)
- File locks MUST be released automatically on process termination

**Resource Protection Requirements:**

- Maximum file size MUST be enforced (configurable, default 10MB)
- Maximum line count MUST be enforced at 100,000 lines per file
- Maximum concurrent operations MUST be enforced (configurable, default 10)
- Memory usage monitoring MUST trigger warnings at 80% of limits
- CPU usage MUST be monitored and limited to prevent resource exhaustion

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

**Layered Architecture MUST be implemented with the following layers:**

```pseudocode
// Presentation Layer (Transport Handlers)
INTERFACE TransportHandler:
    FUNCTION handle_request(raw_request) -> response
    FUNCTION validate_input(request_data) -> validation_result
    FUNCTION format_response(result_data) -> formatted_response
    FUNCTION handle_error(error_info) -> error_response

// Business Logic Layer (Core Operations)
INTERFACE FileOperationService:
    FUNCTION read_file(filename, start_line, end_line) -> file_data
    FUNCTION edit_file(filename, edits, append, create_flag) -> edit_result
    FUNCTION validate_filename(filename) -> validation_result
    FUNCTION acquire_lock(filename) -> lock_handle
    FUNCTION release_lock(lock_handle) -> void

// Data Access Layer (File System Abstraction)
INTERFACE FileSystemAdapter:
    FUNCTION read_file_bytes(file_path) -> byte_array
    FUNCTION write_file_bytes(file_path, content) -> write_result
    FUNCTION file_exists(file_path) -> boolean
    FUNCTION get_file_stats(file_path) -> file_metadata
    FUNCTION create_temp_file(prefix) -> temp_file_path
    FUNCTION atomic_rename(source_path, target_path) -> rename_result
```

**Dependency Injection Pattern:**

```pseudocode
CLASS ServerContainer:
    file_system_adapter: FileSystemAdapter
    lock_manager: LockManager
    validator: InputValidator
    logger: StructuredLogger
    metrics_collector: MetricsCollector
    
    CONSTRUCTOR(config: ServerConfig):
        file_system_adapter = create_file_system_adapter(config.working_dir)
        lock_manager = create_lock_manager(config.max_concurrent)
        validator = create_input_validator(config.max_file_size)
        logger = create_structured_logger(config.log_level)
        metrics_collector = create_metrics_collector()
    END CONSTRUCTOR
    
    FUNCTION create_file_service() -> FileOperationService:
        RETURN new FileOperationService(
            file_system_adapter,
            lock_manager,
            validator,
            logger
        )
    END FUNCTION
END CLASS
```

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

**Thread-Safe Lock Manager:**

```pseudocode
CLASS LockManager:
    PRIVATE active_locks: thread_safe_map<filename, lock_info>
    PRIVATE max_concurrent: integer
    PRIVATE lock_timeout: duration
    
    FUNCTION acquire_lock(filename: string) -> lock_handle:
        start_time = current_time()
        
        WHILE current_time() - start_time < lock_timeout DO
            current_lock_count = active_locks.size()
            IF current_lock_count >= max_concurrent THEN
                sleep(10_milliseconds)
                CONTINUE
            END IF
            
            lock_info = {
                thread_id: current_thread_id(),
                acquired_at: current_time(),
                filename: filename
            }
            
            success = active_locks.try_insert(filename, lock_info)
            IF success THEN
                RETURN create_lock_handle(filename, lock_info)
            END IF
            
            sleep(1_millisecond)
        END WHILE
        
        THROW timeout_error("Failed to acquire lock within timeout")
    END FUNCTION
    
    FUNCTION release_lock(lock_handle: lock_handle) -> void:
        active_locks.remove(lock_handle.filename)
        log_lock_release(lock_handle)
    END FUNCTION
    
    FUNCTION cleanup_expired_locks() -> void:
        current_time = now()
        FOR each (filename, lock_info) in active_locks DO
            IF current_time - lock_info.acquired_at > lock_timeout THEN
                active_locks.remove(filename)
                log_lock_timeout(filename, lock_info)
            END IF
        END FOR
    END FUNCTION
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
    
    IF args.max_file_size < 1 OR args.max_file_size > 100 THEN
        validation_errors.append("--max-file-size must be between 1 and 100 MB")
    END IF
    
    IF args.max_concurrent < 1 OR args.max_concurrent > 100 THEN
        validation_errors.append("--max-concurrent must be between 1 and 100")
    END IF
    
    RETURN validation_result(validation_errors)
END FUNCTION
```

**Server Initialization Sequence:**

```pseudocode
FUNCTION initialize_server(validated_config: server_config):
    initialization_steps = [
        "validate_working_directory",
        "initialize_file_system_adapter", 
        "create_lock_manager",
        "setup_error_handlers",
        "configure_logging",
        "initialize_metrics_collection",
        "setup_transport_handler",
        "perform_health_checks"
    ]
    
    FOR each step_name in initialization_steps DO
        step_start_time = current_time()
        
        step_result = execute_initialization_step(step_name, validated_config)
        
        step_duration = current_time() - step_start_time
        log_initialization_step(step_name, step_result, step_duration)
        
        IF NOT step_result.success THEN
            cleanup_partial_initialization()
            RETURN initialization_failure(step_name, step_result.error)
        END IF
    END FOR
    
    // Start selected transport
    SWITCH validated_config.transport:
        CASE "http":
            transport_result = start_http_server(validated_config.port)
        CASE "stdio":
            transport_result = start_stdio_handler()
        DEFAULT:
            RETURN initialization_failure("transport", "Invalid transport type")
    END SWITCH
    
    IF NOT transport_result.success THEN
        RETURN initialization_failure("transport", transport_result.error)
    END IF
    
    total_startup_time = current_time() - server_start_time
    log_server_ready(total_startup_time, validated_config.transport)
    
    RETURN initialization_success()
END FUNCTION
```

### 6.2 Transport Protocol Contracts

**HTTP Transport Implementation Contract:**

```pseudocode
INTERFACE HTTPTransport:
    FUNCTION start_http_server(port: integer, request_handler: request_handler) -> server_handle
    FUNCTION register_endpoint(path: string, method: string, handler: endpoint_handler) -> void
    FUNCTION set_request_timeout(timeout_seconds: integer) -> void
    FUNCTION set_max_request_size(size_bytes: integer) -> void
    FUNCTION stop_server(graceful_timeout: duration) -> shutdown_result

// HTTP request handling contract
FUNCTION handle_http_request(http_request: http_request) -> http_response:
    // Request preprocessing
    IF http_request.content_length > max_request_size THEN
        RETURN http_error_response(413, "Request too large")
    END IF
    
    IF http_request.content_type != "application/json" THEN
        RETURN http_error_response(400, "Content-Type must be application/json")
    END IF
    
    // Request processing
    TRY
        json_body = parse_json(http_request.body)
        operation_result = process_file_operation(http_request.path, json_body)
        
        IF operation_result.is_success THEN
            RETURN http_success_response(200, operation_result.data)
        ELSE
            http_status = map_error_to_http_status(operation_result.error)
            RETURN http_error_response(http_status, operation_result.error)
        END IF
    CATCH json_parse_error:
        RETURN http_error_response(400, "Invalid JSON in request body")
    CATCH any_other_error:
        log_unexpected_error(any_other_error)
        RETURN http_error_response(500, "Internal server error")
    END TRY
END FUNCTION
```

**stdio JSON-RPC Implementation Contract:**

```pseudocode
INTERFACE StdioTransport:
    FUNCTION start_stdio_loop(request_handler: jsonrpc_handler) -> void
    FUNCTION read_jsonrpc_request() -> jsonrpc_request
    FUNCTION write_jsonrpc_response(response: jsonrpc_response) -> void
    FUNCTION stop_stdio_loop() -> void

// JSON-RPC request handling contract
FUNCTION handle_stdio_request(stdin_line: string) -> string:
    TRY
        // Parse JSON-RPC request
        jsonrpc_request = parse_jsonrpc(stdin_line)
        validate_jsonrpc_structure(jsonrpc_request)
        
        // Route to appropriate method
        SWITCH jsonrpc_request.method:
            CASE "read_file":
                operation_result = file_service.read_file(jsonrpc_request.params)
            CASE "edit_file":
                operation_result = file_service.edit_file(jsonrpc_request.params)
            DEFAULT:
                RETURN create_jsonrpc_error(-32601, "Method not found", jsonrpc_request.id)
        END SWITCH
        
        // Format response
        IF operation_result.is_success THEN
            RETURN create_jsonrpc_success(operation_result.data, jsonrpc_request.id)
        ELSE
            error_code = map_error_to_jsonrpc_code(operation_result.error)
            RETURN create_jsonrpc_error(error_code, operation_result.error.message, jsonrpc_request.id)
        END IF
        
    CATCH json_parse_error:
        RETURN create_jsonrpc_error(-32700, "Parse error", null)
    CATCH validation_error:
        RETURN create_jsonrpc_error(-32600, "Invalid Request", null)
    CATCH any_other_error:
        log_unexpected_error(any_other_error)
        RETURN create_jsonrpc_error(-32603, "Internal error", jsonrpc_request.id)
    END TRY
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

// Usage example
read_result = call_file_server_http(
    "http://localhost:8080",
    "read_file", 
    {name: "example.txt", start_line: 5, end_line: 10}
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
            "utf8_encoding_validation"
        ]
    },
    "file_operations": {
        minimum_coverage: 95,
        test_cases: [
            "successful_file_reading",
            "line_range_processing",
            "edit_operation_types",
            "atomic_write_behavior",
            "error_condition_handling"
        ]
    },
    "concurrency": {
        minimum_coverage: 90,
        test_cases: [
            "concurrent_read_operations",
            "exclusive_write_locking",
            "lock_timeout_behavior",
            "deadlock_prevention"
        ]
    },
    "transport_protocols": {
        minimum_coverage: 95,
        test_cases: [
            "http_endpoint_behavior",
            "jsonrpc_message_handling",
            "error_response_formatting",
            "protocol_compliance"
        ]
    }
}
```

**Integration Test Scenarios:**

```pseudocode
// End-to-end integration test specification
FUNCTION test_complete_file_editing_workflow():
    // Setup
    test_server = start_test_server(temp_directory)
    test_file_content = "line1\nline2\nline3\nline4\nline5"
    
    // Test sequence
    create_file_result = call_edit_file({
        name: "test.txt",
        append: test_file_content,
        create_if_missing: true
    })
    assert_success(create_file_result)
    
    read_result = call_read_file({name: "test.txt"})
    assert_equals(read_result.content, test_file_content)
    assert_equals(read_result.total_lines, 5)
    
    edit_result = call_edit_file({
        name: "test.txt",
        edits: [{
            line: 3,
            content: "modified line 3",
            operation: "replace"
        }]
    })
    assert_success(edit_result)
    
    verify_result = call_read_file({
        name: "test.txt",
        start_line: 3,
        end_line: 3
    })
    assert_equals(verify_result.content, "modified line 3")
    
    // Cleanup
    stop_test_server(test_server)
END FUNCTION
```

**Performance Test Requirements:**

```pseudocode
// Performance benchmark specifications
FUNCTION benchmark_file_operations():
    test_scenarios = [
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
            name: "concurrent_read_performance",
            file_size: "100KB",
            operation: "read_file",
            concurrent_clients: 10,
            target_time: "100ms",
            iterations: 100
        },
        {
            name: "edit_operation_performance",
            file_size: "1MB", 
            operation: "edit_file",
            target_time: "100ms",
            iterations: 100
        }
    ]
    
    FOR each scenario in test_scenarios DO
        benchmark_result = execute_performance_test(scenario)
        assert_performance_target_met(benchmark_result, scenario.target_time)
        log_performance_metrics(scenario.name, benchmark_result)
    END FOR
END FUNCTION
```

-----

**Implementation Success Criteria:**

1. Zero clarification requests during implementation phase
1. All acceptance criteria pass automated testing
1. Performance targets achieved in initial implementation
1. Cross-platform binary compatibility verified
1. Protocol compliance validated with reference implementations
1. Memory usage stays within specified limits under load testing
1. Error handling covers all specified scenarios with correct codes
1. Security validation prevents all path traversal attempts