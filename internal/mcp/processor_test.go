package mcp

import (
	"encoding/json"
	"file-editor-server/internal/models"
	"reflect"
	"testing"
	"time"
)

// mockFileOperationService is a mock implementation of FileOperationService for testing.
type mockFileOperationService struct {
	ListFilesFunc func(req models.ListFilesRequest) ([]models.FileInfo, string, *models.ErrorDetail)
	ReadFileFunc  func(req models.ReadFileRequest) (content string, filename string, totalLines int, reqStartLine int, reqEndLine int, actualEndLine int, isRangeRequest bool, err *models.ErrorDetail)
	EditFileFunc  func(req models.EditFileRequest) (filename string, linesModified int, newTotalLines int, fileCreated bool, err *models.ErrorDetail)
}

func (m *mockFileOperationService) ListFiles(req models.ListFilesRequest) ([]models.FileInfo, string, *models.ErrorDetail) {
	if m.ListFilesFunc != nil {
		return m.ListFilesFunc(req)
	}
	return nil, "", &models.ErrorDetail{Code: 1, Message: "ListFilesFunc not implemented"}
}

func (m *mockFileOperationService) ReadFile(req models.ReadFileRequest) (content string, filename string, totalLines int, reqStartLine int, reqEndLine int, actualEndLine int, isRangeRequest bool, err *models.ErrorDetail) {
	if m.ReadFileFunc != nil {
		return m.ReadFileFunc(req)
	}
	return "", "", 0, 0, 0, 0, false, &models.ErrorDetail{Code: 1, Message: "ReadFileFunc not implemented"}
}

func (m *mockFileOperationService) EditFile(req models.EditFileRequest) (filename string, linesModified int, newTotalLines int, fileCreated bool, err *models.ErrorDetail) {
	if m.EditFileFunc != nil {
		return m.EditFileFunc(req)
	}
	return "", 0, 0, false, &models.ErrorDetail{Code: 1, Message: "EditFileFunc not implemented"}
}

func TestMCPProcessor_ProcessRequest(t *testing.T) {
	mockService := &mockFileOperationService{}
	processor := NewMCPProcessor(mockService)

	// Define test cases
	testCases := []struct {
		name           string
		request        models.JSONRPCRequest
		mockSetup      func() // To setup mockService behavior for this test case
		expectedResult *models.MCPToolResult
		expectedError  *models.JSONRPCError
	}{
		{
			name: "Initialize method",
			request: models.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "initialize",
				ID:      "init1",
			},
			mockSetup: nil,
			expectedResult: &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: "MCP server initialized. Available tools: list_files, read_file, edit_file."}},
				IsError: false,
			},
			expectedError: nil,
		},
		{
			name: "Tools/list method",
			request: models.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "tools/list",
				ID:      "toolslist1",
			},
			mockSetup: nil,
			expectedResult: &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: "Available tools:\n" +
					"- list_files: Lists all non-hidden files in the working directory. Provides name, size, modification time, permissions, and line count for each file.\n" +
					"- read_file: Reads the content of a specified file. Can read the full file or a specified range of lines. Reports total lines and the range read.\n" +
					"- edit_file: Edits a specified file with a series of operations (insert, replace, delete lines). Can also append content and create the file if it doesn't exist. Reports lines modified, new total lines, and if the file was created."}},
				IsError: false,
			},
			expectedError: nil,
		},
		{
			name: "Unknown MCP method",
			request: models.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "unknown_method",
				ID:      "unknown1",
			},
			mockSetup: nil,
			expectedResult: nil,
			expectedError: &models.JSONRPCError{
				Code:    -32601, // Method not found
				Message: "Method not found: unknown_method",
			},
		},
		{
			name: "tools/call with list_files success",
			request: models.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name": "list_files", "arguments": {}}`),
				ID:      "list1",
			},
			mockSetup: func() {
				mockService.ListFilesFunc = func(req models.ListFilesRequest) ([]models.FileInfo, string, *models.ErrorDetail) {
					return []models.FileInfo{
						{Name: "file1.txt", Size: 100, Modified: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339), Readable: true, Writable: true, Lines: 10},
					}, "/test/dir", nil
				}
			},
			expectedResult: &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: processor.formatListFilesResult(
					[]models.FileInfo{
						{Name: "file1.txt", Size: 100, Modified: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC).Format(time.RFC3339), Readable: true, Writable: true, Lines: 10},
					}, "/test/dir"),
				}},
				IsError: false,
			},
			expectedError: nil,
		},
		{
			name: "tools/call with list_files service error",
			request: models.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name": "list_files", "arguments": {}}`),
				ID:      "list_err1",
			},
			mockSetup: func() {
				mockService.ListFilesFunc = func(req models.ListFilesRequest) ([]models.FileInfo, string, *models.ErrorDetail) {
					return nil, "", &models.ErrorDetail{Code: 1000, Message: "Service error listing files"}
				}
			},
			expectedResult: &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: processor.formatToolError(&models.ErrorDetail{Code: 1000, Message: "Service error listing files"})}},
				IsError: true,
			},
			expectedError: nil,
		},
		{
			name: "tools/call with read_file success",
			request: models.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name": "read_file", "arguments": {"name": "test.txt"}}`),
				ID:      "read1",
			},
			mockSetup: func() {
				mockService.ReadFileFunc = func(req models.ReadFileRequest) (string, string, int, int, int, int, bool, *models.ErrorDetail) {
					return "file content", req.Name, 1, 0, 0, 0, false, nil
				}
			},
			expectedResult: &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: processor.formatReadFileResult("file content", "test.txt", 1, 0, 0, 0, false)}},
				IsError: false,
			},
			expectedError: nil,
		},
		{
			name: "tools/call with read_file service error",
			request: models.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name": "read_file", "arguments": {"name": "error.txt"}}`),
				ID:      "read_err1",
			},
			mockSetup: func() {
				mockService.ReadFileFunc = func(req models.ReadFileRequest) (string, string, int, int, int, int, bool, *models.ErrorDetail) {
					return "", req.Name, 0, 0, 0, -1, false, &models.ErrorDetail{Code: 1001, Message: "Service error reading file"}
				}
			},
			expectedResult: &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: processor.formatToolError(&models.ErrorDetail{Code: 1001, Message: "Service error reading file"})}},
				IsError: true,
			},
			expectedError: nil,
		},
		{
			name: "tools/call with edit_file success",
			request: models.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name": "edit_file", "arguments": {"name": "edit.txt", "edits": []}}`),
				ID:      "edit1",
			},
			mockSetup: func() {
				mockService.EditFileFunc = func(req models.EditFileRequest) (string, int, int, bool, *models.ErrorDetail) {
					return req.Name, 0, 1, false, nil
				}
			},
			expectedResult: &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: processor.formatEditFileResult("edit.txt", 0, 1, false)}},
				IsError: false,
			},
			expectedError: nil,
		},
		{
			name: "tools/call with edit_file service error",
			request: models.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name": "edit_file", "arguments": {"name": "error.txt", "edits": []}}`),
				ID:      "edit_err1",
			},
			mockSetup: func() {
				mockService.EditFileFunc = func(req models.EditFileRequest) (string, int, int, bool, *models.ErrorDetail) {
					return req.Name, 0, 0, false, &models.ErrorDetail{Code: 1002, Message: "Service error editing file"}
				}
			},
			expectedResult: &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: processor.formatToolError(&models.ErrorDetail{Code: 1002, Message: "Service error editing file"})}},
				IsError: true,
			},
			expectedError: nil,
		},
		{
			name: "tools/call with unknown tool name",
			request: models.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name": "unknown_tool", "arguments": {}}`),
				ID:      "unknown_tool1",
			},
			mockSetup: nil,
			expectedResult: &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: "Error: Unknown tool 'unknown_tool'."}},
				IsError: true,
			},
			expectedError: nil,
		},
		{
			name: "tools/call with invalid arguments for list_files (bad structure)",
			request: models.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "tools/call",
				Params:  json.RawMessage(`{"name": "list_files", "arguments": "not_an_object"}`),
				ID:      "invalid_args1",
			},
			mockSetup: nil,
			expectedResult: nil, // Error is at JSON-RPC level
			expectedError: &models.JSONRPCError{
				Code:    -32602, // Invalid Params
				Message: "Invalid parameters for list_files: json: cannot unmarshal string into Go value of type models.ListFilesRequest",
			},
		},
		{
			name: "tools/call with invalid params for tools/call itself (not ToolCallParams)",
			request: models.JSONRPCRequest{
				JSONRPC: "2.0",
				Method:  "tools/call",
				Params:  json.RawMessage(`"not_tool_call_params_object"`),
				ID:      "invalid_tool_call_params",
			},
			mockSetup: nil,
			expectedResult: nil,
			expectedError: &models.JSONRPCError{
				Code:    -32602, // Invalid Params
				Message: "Invalid parameters for tools/call: json: cannot unmarshal string into Go value of type mcp.ToolCallParams",
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.mockSetup != nil {
				tc.mockSetup()
			}

			result, err := processor.ProcessRequest(tc.request)

			if !reflect.DeepEqual(result, tc.expectedResult) {
				t.Errorf("Expected result %+v, got %+v", tc.expectedResult, result)
			}
			if !reflect.DeepEqual(err, tc.expectedError) {
				t.Errorf("Expected error %+v, got %+v", tc.expectedError, err)
			}
		})
	}
}
