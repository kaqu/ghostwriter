package mcp

import (
	"encoding/json"
	"file-editor-server/internal/models"
	"file-editor-server/internal/service" // Will be needed for mock
	"fmt"
	"strings"
	"testing"
	"time"

	// Mocking framework, if available/preferred, could be used here.
	// For now, using a simple mock implementation.
)

// MockFileOperationService is a mock implementation of FileOperationService.
type MockFileOperationService struct {
	ListFilesFunc func(req models.ListFilesRequest) ([]models.FileInfo, *models.ErrorDetail)
	ReadFileFunc  func(req models.ReadFileRequest) (content string, filename string, totalLines int, reqStartLine int, reqEndLine int, actualEndLine int, isRangeRequest bool, err *models.ErrorDetail)
	EditFileFunc  func(req models.EditFileRequest) (filename string, linesModified int, newTotalLines int, fileCreated bool, err *models.ErrorDetail)
}

func (m *MockFileOperationService) ListFiles(req models.ListFilesRequest) ([]models.FileInfo, *models.ErrorDetail) {
	if m.ListFilesFunc != nil {
		return m.ListFilesFunc(req)
	}
	return nil, nil
}

func (m *MockFileOperationService) ReadFile(req models.ReadFileRequest) (string, string, int, int, int, int, bool, *models.ErrorDetail) {
	if m.ReadFileFunc != nil {
		return m.ReadFileFunc(req)
	}
	return "", "", 0, 0, 0, 0, false, nil
}

func (m *MockFileOperationService) EditFile(req models.EditFileRequest) (string, int, int, bool, *models.ErrorDetail) {
	if m.EditFileFunc != nil {
		return m.EditFileFunc(req)
	}
	return "", 0, 0, false, nil
}

func TestMCPProcessor_Initialize(t *testing.T) {
	mockService := &MockFileOperationService{}
	processor := NewMCPProcessor(mockService)

	req := models.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "initialize",
		ID:      "1",
	}

	result, rpcErr := processor.ProcessRequest(req)
	if rpcErr != nil {
		t.Fatalf("ProcessRequest returned an RPC error: %v", rpcErr)
	}
	if result == nil {
		t.Fatalf("ProcessRequest returned a nil result")
	}
	if result.IsError {
		t.Fatalf("MCPToolResult indicates an error: %s", result.Content[0].Text)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("Unexpected content structure: %+v", result.Content)
	}

	var initResp models.InitializeResponse
	if err := json.Unmarshal([]byte(result.Content[0].Text), &initResp); err != nil {
		t.Fatalf("Failed to unmarshal InitializeResponse: %v. JSON: %s", err, result.Content[0].Text)
	}

	if initResp.ProtocolVersion != "2024-11-05" {
		t.Errorf("Expected ProtocolVersion '2024-11-05', got '%s'", initResp.ProtocolVersion)
	}

	expectedServerInfo := models.ServerInfo{
		Name:        "file-editing-server",
		Version:     "1.0.0",
		Description: "High-performance file editing server for AI agents",
	}
	if initResp.ServerInfo != expectedServerInfo {
		t.Errorf("Expected ServerInfo %+v, got %+v", expectedServerInfo, initResp.ServerInfo)
	}

	// Check if Capabilities.Tools is an empty object (it's a struct, so non-nil check is enough for empty {})
	// The actual check for emptiness is that it's a default/zero-value struct.
	// If ToolsCapabilities had fields, we'd check them. Since it's empty, this is fine.
	if initResp.Capabilities.Tools != (models.ToolsCapabilities{}) {
		t.Errorf("Expected Capabilities.Tools to be an empty struct, got %+v", initResp.Capabilities.Tools)
	}
}

func TestMCPProcessor_ToolsList(t *testing.T) {
	mockService := &MockFileOperationService{}
	processor := NewMCPProcessor(mockService)

	req := models.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  "tools/list",
		ID:      "2",
	}

	result, rpcErr := processor.ProcessRequest(req)
	if rpcErr != nil {
		t.Fatalf("ProcessRequest returned an RPC error: %v", rpcErr)
	}
	if result.IsError {
		t.Fatalf("MCPToolResult indicates an error: %s", result.Content[0].Text)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" {
		t.Fatalf("Unexpected content structure: %+v", result.Content)
	}

	var listResp models.ToolsListResponse
	if err := json.Unmarshal([]byte(result.Content[0].Text), &listResp); err != nil {
		t.Fatalf("Failed to unmarshal ToolsListResponse: %v. JSON: %s", err, result.Content[0].Text)
	}

	if len(listResp.Tools) != 3 {
		t.Fatalf("Expected 3 tools, got %d", len(listResp.Tools))
	}

	expectedTools := map[string]struct {
		description     string
		readOnlyHint    bool
		destructiveHint bool
	}{
		"list_files": {"Lists all non-hidden files in the working directory, providing name, modification time, and line count.", true, false},
		"read_file":  {"Reads the content of a specified file, optionally within a given line range.", true, false},
		"edit_file":  {"Edits a file using line-based operations, creates if missing (with flag), or appends content.", false, true},
	}

	for _, tool := range listResp.Tools {
		expected, ok := expectedTools[tool.Name]
		if !ok {
			t.Errorf("Unexpected tool found: %s", tool.Name)
			continue
		}

		if tool.Description != expected.description {
			t.Errorf("For tool %s, expected description '%s', got '%s'", tool.Name, expected.description, tool.Description)
		}
		if tool.Annotations.ReadOnlyHint != expected.readOnlyHint {
			t.Errorf("For tool %s, expected ReadOnlyHint %t, got %t", tool.Name, expected.readOnlyHint, tool.Annotations.ReadOnlyHint)
		}
		if tool.Annotations.DestructiveHint != expected.destructiveHint {
			t.Errorf("For tool %s, expected DestructiveHint %t, got %t", tool.Name, expected.destructiveHint, tool.Annotations.DestructiveHint)
		}

		if tool.ArgumentsSchema == nil {
			t.Errorf("For tool %s, ArgumentsSchema is nil", tool.Name)
		} else {
			if _, okType := tool.ArgumentsSchema["type"]; !okType || tool.ArgumentsSchema["type"] != "object" {
				t.Errorf("For tool %s, ArgumentsSchema type is not 'object' or missing", tool.Name)
			}
		}

		if tool.ResponseSchema == nil {
			t.Errorf("For tool %s, ResponseSchema is nil", tool.Name)
		} else {
			if _, okType := tool.ResponseSchema["type"]; !okType || tool.ResponseSchema["type"] != "string" {
				t.Errorf("For tool %s, ResponseSchema type is not 'string' or missing", tool.Name)
			}
		}
	}
}

func TestFormatListFilesResult(t *testing.T) {
	p := &MCPProcessor{} // Formatting functions don't depend on service state

	// Test with empty slice
	emptyFiles := []models.FileInfo{}
	expectedEmpty := "Total files: 0"
	if result := p.formatListFilesResult(emptyFiles); result != expectedEmpty {
		t.Errorf("formatListFilesResult with empty slice: expected '%s', got '%s'", expectedEmpty, result)
	}

	// Test with one file
	oneFile := []models.FileInfo{
		{Name: "file1.txt", Modified: "2023-01-01T12:00:00Z", Lines: 100},
	}
	expectedOne := "Files in directory:\n\nname: file1.txt, modified: 2023-01-01T12:00:00Z, lines: 100\n\nTotal files: 1"
	if result := p.formatListFilesResult(oneFile); result != expectedOne {
		t.Errorf("formatListFilesResult with one file: expected\n'%s'\ngot\n'%s'", expectedOne, result)
	}

	// Test with multiple files and one with Lines: -1
	twoFiles := []models.FileInfo{
		{Name: "file1.txt", Modified: "2023-01-01T12:00:00Z", Lines: 100},
		{Name: "another.log", Modified: "2023-01-02T15:30:00Z", Lines: -1},
	}
	expectedTwo := "Files in directory:\n\nname: file1.txt, modified: 2023-01-01T12:00:00Z, lines: 100\nname: another.log, modified: 2023-01-02T15:30:00Z, lines: (unknown)\n\nTotal files: 2"
	if result := p.formatListFilesResult(twoFiles); result != expectedTwo {
		t.Errorf("formatListFilesResult with two files: expected\n'%s'\ngot\n'%s'", expectedTwo, result)
	}
}

func TestFormatReadFileResult(t *testing.T) {
	p := &MCPProcessor{}

	// Test entire file scenario
	entireFileContent := "Line 1\nLine 2\nLine 3"
	expectedEntire := "File: test.txt (3 lines)\n\nLine 1\nLine 2\nLine 3"
	if result := p.formatReadFileResult(entireFileContent, "test.txt", 3, 0, 0, 0, false); result != expectedEntire {
		t.Errorf("formatReadFileResult entire file: expected\n'%s'\ngot\n'%s'", expectedEntire, result)
	}

	// Test entire empty file scenario
	expectedEmptyEntire := "File: empty.txt (0 lines)\n\n"
	if result := p.formatReadFileResult("", "empty.txt", 0, 0, 0, 0, false); result != expectedEmptyEntire {
		t.Errorf("formatReadFileResult entire empty file: expected\n'%s'\ngot\n'%s'", expectedEmptyEntire, result)
	}

	// Test line range scenario - content present
	rangeContent := "Line 2\nLine 3"
	// reqStartLine=2, reqEndLine=3, actualEndLine=1 (0-indexed for "Line 3" if content is "Line 2\nLine 3")
	// service.ReadFile returns: content, filename, totalLines, reqStartLine, reqEndLine, actualEndLine (0-based index of last line IN content), isRangeRequest
	// For content "Line 2\nLine 3", actualEndLine would be 1.
	expectedRange := "File: ranged.txt (lines 2-3 of 5 total)\n\nLine 2\nLine 3"
	if result := p.formatReadFileResult(rangeContent, "ranged.txt", 5, 2, 3, 1, true); result != expectedRange {
		t.Errorf("formatReadFileResult range: expected\n'%s'\ngot\n'%s'", expectedRange, result)
	}

	// Test line range scenario - reqStartLine = 0 (from beginning)
	// content="Line 1\nLine 2", totalLines=5, reqStartLine=0, reqEndLine=2, actualEndLine=1
	expectedRangeStart0 := "File: ranged0.txt (lines 1-2 of 5 total)\n\nLine 1\nLine 2"
	if result := p.formatReadFileResult("Line 1\nLine 2", "ranged0.txt", 5, 0, 2, 1, true); result != expectedRangeStart0 {
		t.Errorf("formatReadFileResult range start 0: expected\n'%s'\ngot\n'%s'", expectedRangeStart0, result)
	}


	// Test line range scenario - content empty (range outside actual lines, e.g., lines 10-12 of a 5 line file)
	// service.ReadFile returns: content="", filename, totalLines=5, reqStartLine=10, reqEndLine=12, actualEndLine=-1 (or similar to indicate no lines returned), isRangeRequest=true
	expectedEmptyRange := "File: empty_range.txt (lines 10-9 of 5 total)\n\n"
	if result := p.formatReadFileResult("", "empty_range.txt", 5, 10, 12, -1, true); result != expectedEmptyRange {
		t.Errorf("formatReadFileResult empty range: expected\n'%s'\ngot\n'%s'", expectedEmptyRange, result)
	}

	// Test line range scenario - content empty (range 0-5 on an empty file)
	// service.ReadFile returns: content="", filename, totalLines=0, reqStartLine=0, reqEndLine=5, actualEndLine=-1, isRangeRequest=true
	expectedEmptyFileRange := "File: empty_file_ranged.txt (lines 1-0 of 0 total)\n\n"
	if result := p.formatReadFileResult("", "empty_file_ranged.txt", 0, 0, 5, -1, true); result != expectedEmptyFileRange {
		t.Errorf("formatReadFileResult empty file range: expected\n'%s'\ngot\n'%s'", expectedEmptyFileRange, result)
	}
}

func TestFormatEditFileResult(t *testing.T) {
	p := &MCPProcessor{}

	// Test with fileCreated = true
	expectedCreated := "File edited successfully: newfile.txt\nLines modified: 5\nTotal lines: 5\nFile created: true"
	if result := p.formatEditFileResult("newfile.txt", 5, 5, true); result != expectedCreated {
		t.Errorf("formatEditFileResult created: expected\n'%s'\ngot\n'%s'", expectedCreated, result)
	}

	// Test with fileCreated = false
	expectedEdited := "File edited successfully: oldfile.txt\nLines modified: 2\nTotal lines: 10\nFile created: false"
	if result := p.formatEditFileResult("oldfile.txt", 2, 10, false); result != expectedEdited {
		t.Errorf("formatEditFileResult edited: expected\n'%s'\ngot\n'%s'", expectedEdited, result)
	}
}

func TestFormatToolError(t *testing.T) {
	p := &MCPProcessor{}

	errDetail := &models.ErrorDetail{
		Code:    1001,
		Message: "File not found: specific.txt",
		Data:    map[string]string{"filename": "specific.txt"},
	}
	expectedError := "Error: File not found: specific.txt"
	if result := p.formatToolError(errDetail); result != expectedError {
		t.Errorf("formatToolError: expected '%s', got '%s'", expectedError, result)
	}

	// Test with nil error detail
	expectedNilError := "Error: An unexpected error occurred, but no details were provided."
	if result := p.formatToolError(nil); result != expectedNilError {
		t.Errorf("formatToolError with nil: expected '%s', got '%s'", expectedNilError, result)
	}
}

// TestMCPProcessor_HandleToolCall_Errors provides basic coverage for error paths in handleToolCall
// More specific error condition tests (e.g. service layer returning errors) would require more mock setup.
func TestMCPProcessor_HandleToolCall_Errors(t *testing.T) {
	mockService := &MockFileOperationService{
		ListFilesFunc: func(req models.ListFilesRequest) ([]models.FileInfo, *models.ErrorDetail) {
			return nil, &models.ErrorDetail{Code: 123, Message: "service list error"}
		},
		ReadFileFunc: func(req models.ReadFileRequest) (string, string, int, int, int, int, bool, *models.ErrorDetail) {
			return "", "", 0, 0, 0, 0, false, &models.ErrorDetail{Code: 124, Message: "service read error"}
		},
		EditFileFunc: func(req models.EditFileRequest) (string, int, int, bool, *models.ErrorDetail) {
			return "",0,0,false, &models.ErrorDetail{Code:125, Message:"service edit error"}
		},
	}
	processor := NewMCPProcessor(mockService)

	tests := []struct {
		name          string
		toolName      string
		toolArgs      json.RawMessage
		expectedError string
		expectRPCErr  bool
	}{
		{
			name:          "list_files with service error",
			toolName:      "list_files",
			toolArgs:      json.RawMessage(`{}`),
			expectedError: "Error: service list error",
			expectRPCErr:  false,
		},
		{
			name:          "read_file with service error",
			toolName:      "read_file",
			toolArgs:      json.RawMessage(`{"name":"test.txt"}`),
			expectedError: "Error: service read error",
			expectRPCErr:  false,
		},
		{
			name:          "edit_file with service error",
			toolName:      "edit_file",
			toolArgs:      json.RawMessage(`{"name":"test.txt"}`),
			expectedError: "Error: service edit error",
			expectRPCErr:  false,
		},
		{
			name:          "unknown tool",
			toolName:      "unknown_tool",
			toolArgs:      json.RawMessage(`{}`),
			expectedError: "Error: Unknown tool 'unknown_tool'.",
			expectRPCErr:  false,
		},
		{
			name:          "list_files with bad params",
			toolName:      "list_files",
			// This causes unmarshal error because ListFilesRequest is an empty struct
			// so any fields here are "unknown" if DisallowUnknownFields were used,
			// or simply a type mismatch if it expects {}.
			// json.Unmarshal into an empty struct with non-empty JSON results in an error
			// if the types don't match (e.g. json is `[]` but target is struct)
			// or if fields are present but not in struct.
			// For `json.RawMessage("{}")` into `models.ListFilesRequest{}`, it works.
			// For `json.RawMessage("null")` or `json.RawMessage("123")` it fails.
			// `json.RawMessage(`{"bad":true}`)` should fail if ListFilesRequest is `struct{}`.
			toolArgs:      json.RawMessage(`123`), // Invalid JSON for a struct type
			expectedError: "Invalid parameters for list_files",
			expectRPCErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, rpcErr := processor.handleToolCall(tt.toolName, tt.toolArgs)
			if tt.expectRPCErr {
				if rpcErr == nil {
					t.Fatalf("Expected RPC error, got nil. Result: %+v", result)
				}
				if !strings.Contains(rpcErr.Message, tt.expectedError) {
					t.Errorf("Expected RPC error message to contain '%s', got '%s'", tt.expectedError, rpcErr.Message)
				}
			} else {
				if rpcErr != nil {
					t.Fatalf("Unexpected RPC error: %v", rpcErr)
				}
				if !result.IsError {
					t.Errorf("Expected MCPToolResult to be an error, but IsError is false.")
				}
				if len(result.Content) == 0 || !strings.Contains(result.Content[0].Text, tt.expectedError) {
					t.Errorf("Expected error message in content to contain '%s', got '%s'", tt.expectedError, result.Content[0].Text)
				}
			}
		})
	}
}

// TestExecuteTool is similar to handleToolCall but takes parsed args.
// This mainly tests the type assertions and error paths for ExecuteTool itself.
func TestMCPProcessor_ExecuteTool_ErrorsAndBasic(t *testing.T) {
    mockService := &MockFileOperationService{
        ListFilesFunc: func(req models.ListFilesRequest) ([]models.FileInfo, *models.ErrorDetail) {
            if req == (models.ListFilesRequest{}) { // Basic check for valid empty request
                 return []models.FileInfo{{Name: "test.txt", Lines: 1, Modified: "2023-01-01T00:00:00Z"}}, nil
            }
            return nil, &models.ErrorDetail{Code: 700, Message: "list files error in ExecuteTool"}
        },
    }
    processor := NewMCPProcessor(mockService)

    // Valid call
    _, err := processor.ExecuteTool("list_files", models.ListFilesRequest{})
    if err != nil {
        t.Errorf("ExecuteTool with valid args for list_files failed: %v", err)
    }

    // Invalid argument type
    expectedErrText := "invalid arguments type for list_files: expected models.ListFilesRequest, got string"
    _, err = processor.ExecuteTool("list_files", "not-a-struct")
    if err == nil || err.Error() != expectedErrText {
        t.Errorf("ExecuteTool with invalid arg type: expected error '%s', got '%v'", expectedErrText, err)
    }

    // Unknown tool
    res, err := processor.ExecuteTool("super_tool", models.ListFilesRequest{})
    if err != nil { // ExecuteTool returns MCPToolResult for unknown tool, not an error directly
        t.Errorf("ExecuteTool with unknown tool returned an unexpected error: %v", err)
    }
    if !res.IsError || !strings.Contains(res.Content[0].Text, "Error: Unknown tool 'super_tool'.") {
         t.Errorf("ExecuteTool with unknown tool: unexpected result: %+v", res)
    }
}

func init() {
	// Suppress log output during tests if necessary, though these tests don't directly cause logging from processor.
	// log.SetOutput(ioutil.Discard)
}

// Example of how you might test ProcessRequest for "tools/call"
// This would involve more setup for the mock service.
func TestMCPProcessor_ProcessRequest_ToolsCall_ListFiles(t *testing.T) {
    mockService := &MockFileOperationService{
        ListFilesFunc: func(req models.ListFilesRequest) ([]models.FileInfo, *models.ErrorDetail) {
            return []models.FileInfo{
                {Name: "alpha.txt", Lines: 10, Modified: time.Now().UTC().Format(time.RFC3339)},
            }, nil
        },
    }
    processor := NewMCPProcessor(mockService)

    argsJSON := `{}`
    paramsJSON := fmt.Sprintf(`{"name":"list_files", "arguments": %s}`, argsJSON)

    rpcReq := models.JSONRPCRequest{
        JSONRPC: "2.0",
        Method:  "tools/call",
        Params:  json.RawMessage(paramsJSON),
        ID:      "test-id-list",
    }

    result, rpcErr := processor.ProcessRequest(rpcReq)
    if rpcErr != nil {
        t.Fatalf("ProcessRequest(tools/call list_files) returned RPC error: %v", rpcErr)
    }
    if result.IsError {
        t.Fatalf("ProcessRequest(tools/call list_files) result IsError=true: %s", result.Content[0].Text)
    }
    // Further checks on result.Content[0].Text would verify the formatted output
    // which is already covered by TestFormatListFilesResult
    if !strings.Contains(result.Content[0].Text, "name: alpha.txt") {
        t.Errorf("Expected list_files output to contain 'name: alpha.txt', got: %s", result.Content[0].Text)
    }
}
