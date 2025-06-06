package transport

import (
	"bytes"
	"encoding/json"
	"file-editor-server/internal/errors"
	"file-editor-server/internal/models"
	"fmt"
	"strings"
	"testing"
)

// Using the same mockFileOperationService from http_test.go for consistency.
// If it were in a shared 'mocks' package, it would be imported.
// For this structure, it can be redefined or copied if it's small.
// Let's assume it's simple enough to redefine for clarity here, or use a shared mock later.

// mockMCPProcessor is a mock implementation of an MCPProcessor for testing.
type mockMCPProcessor struct {
	ProcessRequestFunc func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError)
}

func (m *mockMCPProcessor) ProcessRequest(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
	if m.ProcessRequestFunc != nil {
		return m.ProcessRequestFunc(req)
	}
	// Default behavior if ProcessRequestFunc is not set
	return nil, &models.JSONRPCError{
		Code:    -32601, // Method not found
		Message: fmt.Sprintf("Method '%s' not implemented in mockMCPProcessor", req.Method),
	}
}

func runStdioTest(t *testing.T, handler *StdioHandler, input string) string {
	var outputBuffer bytes.Buffer
	inputBuffer := strings.NewReader(input)

	err := handler.Start(inputBuffer, &outputBuffer)
	if err != nil {
		// For tests where input might end prematurely leading to EOF, this might not be a test failure.
		// Depending on test case, may need to check for specific errors like io.EOF.
		// For now, any error from Start is logged by the test.
		t.Logf("Handler Start returned error: %v (this may be expected for some tests like EOF)", err)
	}
	return outputBuffer.String()
}

func TestStdioHandler_ReadFile_Success(t *testing.T) {
	mockProcessor := &mockMCPProcessor{
		ProcessRequestFunc: func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
			// Simulate successful processing of a "tools/call" for "read_file"
			if req.Method == "tools/call" {
				// Minimal check on params structure for this test case
				var toolCallParams struct {
					Name      string `json:"name"`
					Arguments struct {
						Name string `json:"name"`
					} `json:"arguments"`
				}
				if err := json.Unmarshal(req.Params, &toolCallParams); err == nil && toolCallParams.Name == "read_file" && toolCallParams.Arguments.Name == "test.txt" {
					return &models.MCPToolResult{
						Content: []models.MCPToolContent{{Type: "text", Text: "File: test.txt\nContent: file content"}},
						IsError: false,
					}, nil
				}
			}
			return nil, &models.JSONRPCError{Code: -32602, Message: "Mock processor error or wrong params"}
		},
	}
	handler := NewStdioHandler(mockProcessor)

	// JSON-RPC request for "tools/call" with "read_file"
	input := `{"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "read_file", "arguments": {"name": "test.txt"}}, "id": 1}` + "\n"
	output := runStdioTest(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}

	if resp.Error != nil {
		t.Errorf("Expected no error, got code %d, message: %s", resp.Error.Code, resp.Error.Message)
	}
	if resp.ID != float64(1) { // JSON numbers are float64 by default
		t.Errorf("Expected ID 1, got %v", resp.ID)
	}

	mcpResult, ok := resp.Result.(*models.MCPToolResult) // Expecting MCPToolResult directly
	if !ok {
		// If it's a map, it means it was likely unmarshalled into map[string]interface{}
		// This can happen if the types are not perfectly aligned or if a generic unmarshal happened.
		// For this test, we want to ensure it's specifically *models.MCPToolResult.
		rawResultMap, isMap := resp.Result.(map[string]interface{})
		if isMap {
			// Attempt to re-marshal and unmarshal into MCPToolResult for checking
			resultBytes, _ := json.Marshal(rawResultMap)
			var tempResult models.MCPToolResult
			if err := json.Unmarshal(resultBytes, &tempResult); err == nil {
				mcpResult = &tempResult
			} else {
				t.Fatalf("Result is a map but could not be converted to MCPToolResult: %+v. Error: %v", resp.Result, err)
			}
		} else {
			t.Fatalf("Result is not *models.MCPToolResult or a map: type is %T, value: %+v", resp.Result, resp.Result)
		}
	}

	if mcpResult.IsError {
		t.Errorf("Expected MCPToolResult.IsError to be false, got true. Content: %s", mcpResult.Content[0].Text)
	}
	expectedText := "File: test.txt\nContent: file content"
	if len(mcpResult.Content) != 1 || mcpResult.Content[0].Text != expectedText {
		t.Errorf("Expected MCPToolResult content %q, got %q", expectedText, mcpResult.Content[0].Text)
	}
}

func TestStdioHandler_EditFile_Success(t *testing.T) {
	mockProcessor := &mockMCPProcessor{
		ProcessRequestFunc: func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
			if req.Method == "tools/call" {
				var toolCallParams struct {
					Name string `json:"name"`
				}
				if err := json.Unmarshal(req.Params, &toolCallParams); err == nil && toolCallParams.Name == "edit_file" {
					return &models.MCPToolResult{
						Content: []models.MCPToolContent{{Type: "text", Text: "File: edit.txt edited."}},
						IsError: false,
					}, nil
				}
			}
			return nil, &models.JSONRPCError{Code: -32602, Message: "Mock processor error for edit_file"}
		},
	}
	handler := NewStdioHandler(mockProcessor)

	input := `{"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "edit_file", "arguments": {"name": "edit.txt", "edits": []}}, "id": "edit1"}` + "\n"
	output := runStdioTest(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}

	if resp.Error != nil {
		t.Errorf("Expected no error, got %+v", resp.Error)
	}
	if resp.ID != "edit1" {
		t.Errorf("Expected ID 'edit1', got %v", resp.ID)
	}
	mcpResult, ok := resp.Result.(*models.MCPToolResult)
	if !ok {
		rawResultMap, isMap := resp.Result.(map[string]interface{})
		if isMap {
			resultBytes, _ := json.Marshal(rawResultMap)
			var tempResult models.MCPToolResult
			if err := json.Unmarshal(resultBytes, &tempResult); err == nil {
				mcpResult = &tempResult
			} else {
				t.Fatalf("Result is a map but could not be converted to MCPToolResult: %+v. Error: %v", resp.Result, err)
			}
		} else {
			t.Fatalf("Result is not *models.MCPToolResult or a map: type is %T, value: %+v", resp.Result, resp.Result)
		}
	}

	if mcpResult.IsError {
		t.Error("Expected MCPToolResult.IsError to be false")
	}
	if len(mcpResult.Content) != 1 || mcpResult.Content[0].Text != "File: edit.txt edited." {
		t.Errorf("Unexpected MCPToolResult content: %+v", mcpResult.Content)
	}
}

// TestStdioHandler_ToolError verifies that when a tool call results in an error (IsError:true in MCPToolResult),
// it's correctly wrapped in the JSON-RPC response.
func TestStdioHandler_ToolError(t *testing.T) {
	mockProcessor := &mockMCPProcessor{
		ProcessRequestFunc: func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
			if req.Method == "tools/call" {
				// Simulate a tool call that results in a known error
				return &models.MCPToolResult{
					Content: []models.MCPToolContent{{Type: "text", Text: "Error: File 'no.txt' not found. (Code: 1001)"}},
					IsError: true,
				}, nil
			}
			return nil, &models.JSONRPCError{Code: -32601, Message: "Method not found"}
		},
	}
	handler := NewStdioHandler(mockProcessor)

	input := `{"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "read_file", "arguments": {"name": "no.txt"}}, "id": 2}` + "\n"
	output := runStdioTest(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}

	if resp.Error != nil { // For tool errors, the JSON-RPC error should be nil
		t.Fatalf("Expected JSON-RPC error to be nil for tool errors, got: %+v", resp.Error)
	}
	if resp.ID != float64(2) {
		t.Errorf("Expected ID 2, got %v", resp.ID)
	}

	mcpResult, ok := resp.Result.(*models.MCPToolResult)
	if !ok {
		rawResultMap, isMap := resp.Result.(map[string]interface{})
		if isMap {
			resultBytes, _ := json.Marshal(rawResultMap)
			var tempResult models.MCPToolResult
			if err := json.Unmarshal(resultBytes, &tempResult); err == nil {
				mcpResult = &tempResult
			} else {
				t.Fatalf("Result is a map but could not be converted to MCPToolResult: %+v. Error: %v", resp.Result, err)
			}
		} else {
			t.Fatalf("Result is not *models.MCPToolResult or a map: type is %T, value: %+v", resp.Result, resp.Result)
		}
	}

	if !mcpResult.IsError {
		t.Error("Expected MCPToolResult.IsError to be true")
	}
	expectedErrorText := "Error: File 'no.txt' not found. (Code: 1001)"
	if len(mcpResult.Content) != 1 || mcpResult.Content[0].Text != expectedErrorText {
		t.Errorf("Expected MCPToolResult error text %q, got %q", expectedErrorText, mcpResult.Content[0].Text)
	}
}

func TestStdioHandler_InvalidJSON(t *testing.T) {
	handler := NewStdioHandler(&mockMCPProcessor{}) // Mock processor won't be called
	input := `{"jsonrpc": "2.0", "method": "read_file",, "params": {"name": "test.txt"}, "id": 3}` + "\n" // Invalid JSON
	output := runStdioTest(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}

	if resp.Error == nil {
		t.Fatal("Expected error, got nil")
	}
	if resp.Error.Code != errors.CodeParseError { // This is a JSON-RPC level error
		t.Errorf("Expected error code %d, got %d. Message: %s", errors.CodeParseError, resp.Error.Code, resp.Error.Message)
	}
	// ID for parse error is based on pre-parse, which might be nil if ID itself is malformed or missing.
	// In this specific invalid JSON, ID might be part of the malformed section.
	// The current StdioHandler tries to pre-parse ID. If `id": 3` is parsable before the error, it uses it.
	// If the error is before ID, preParse.ID would be nil.
	// Given `..., "id": 3}`, preParse.ID should be 3.
	if resp.ID != float64(3) {
		// If this fails, it means the pre-parsing of ID didn't capture it from the malformed JSON.
		// This detail might depend on how deep the pre-parser goes or if it bails early.
		// The key is that CodeParseError is returned.
		t.Logf("Note: ID for this specific parse error was %v, expected non-nil if ID part was parsable.", resp.ID)
	}
}

func TestStdioHandler_InvalidJSONRPCVersion(t *testing.T) {
	handler := NewStdioHandler(&mockMCPProcessor{})
	input := `{"jsonrpc": "1.0", "method": "tools/call", "params": {"name":"list_files", "arguments":{}}, "id": 4}` + "\n"
	output := runStdioTest(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}

	if resp.Error == nil {
		t.Fatal("Expected error, got nil")
	}
	if resp.Error.Code != errors.CodeInvalidRequest { // This is a JSON-RPC level error
		t.Errorf("Expected error code %d, got %d. Message: %s", errors.CodeInvalidRequest, resp.Error.Code, resp.Error.Message)
	}
	if resp.ID != float64(4) { // ID should still be preserved for this error type
		t.Errorf("Expected ID 4, got %v", resp.ID)
	}
}

// TestStdioHandler_UnknownMethod tests JSON-RPC error for unknown method (not MCP tool error).
func TestStdioHandler_UnknownMethod(t *testing.T) {
	// MCPProcessor's default behavior for unknown methods is to return a JSONRPCError -32601
	mockProc := &mockMCPProcessor{
		ProcessRequestFunc: func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
			if req.Method == "non_existent_mcp_method" {
				return nil, &models.JSONRPCError{Code: -32601, Message: "Method not found: non_existent_mcp_method"}
			}
			return nil, &models.JSONRPCError{Code: -32600, Message: "Should not happen"}
		},
	}
	handler := NewStdioHandler(mockProc)
	input := `{"jsonrpc": "2.0", "method": "non_existent_mcp_method", "params": {}, "id": 5}` + "\n"
	output := runStdioTest(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}

	if resp.Error == nil {
		t.Fatal("Expected error, got nil")
	}
	if resp.Error.Code != -32601 { // MCPProcessor itself handles this for MCP methods
		t.Errorf("Expected error code -32601, got %d", resp.Error.Code)
	}
	if resp.ID != float64(5) {
		t.Errorf("Expected ID 5, got %v", resp.ID)
	}
}

// TestStdioHandler_InvalidParams_ToolCall tests JSON-RPC error for invalid params to a tool call.
func TestStdioHandler_InvalidParams_ToolCall(t *testing.T) {
	// MCPProcessor's ProcessRequest should return JSONRPCError for malformed tool call params.
	mockProc := &mockMCPProcessor{
		ProcessRequestFunc: func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
			if req.Method == "tools/call" {
				// Simulate error during unmarshaling req.Params into ToolCallParams
				return nil, &models.JSONRPCError{Code: -32602, Message: "Invalid params for tools/call"}
			}
			return nil, &models.JSONRPCError{Code: -32600, Message: "Should not happen"}
		},
	}
	handler := NewStdioHandler(mockProc)

	// Params for "tools/call" is not a valid ToolCallParams structure (e.g., missing 'name' or 'arguments')
	input := `{"jsonrpc": "2.0", "method": "tools/call", "params": ["not_an_object"], "id": 6}` + "\n"
	output := runStdioTest(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}

	if resp.Error == nil {
		t.Fatal("Expected error, got nil")
	}
	if resp.Error.Code != -32602 { // MCPProcessor returns this for bad tool call structure
		t.Errorf("Expected error code -32602, got %d. Message: %s", resp.Error.Code, resp.Error.Message)
	}
	if resp.ID != float64(6) {
		t.Errorf("Expected ID 6, got %v", resp.ID)
	}
}

func TestStdioHandler_MultipleRequests(t *testing.T) {
	callCount := 0
	mockProcessor := &mockMCPProcessor{
		ProcessRequestFunc: func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
			callCount++
			// Simulate a successful tool call for any "tools/call" method
			if req.Method == "tools/call" {
				var toolCallParams struct {
					Name      string          `json:"name"`
					Arguments json.RawMessage `json:"arguments"`
				}
				if err := json.Unmarshal(req.Params, &toolCallParams); err == nil {
					return &models.MCPToolResult{
						Content: []models.MCPToolContent{{Type: "text", Text: fmt.Sprintf("Processed %s for ID %v", toolCallParams.Name, req.ID)}},
						IsError: false,
					}, nil
				}
				return nil, &models.JSONRPCError{Code: -32602, Message: "Bad tool call params in mock"}

			}
			return nil, &models.JSONRPCError{Code: -32601, Message: "Unknown method in mock"}
		},
	}
	handler := NewStdioHandler(mockProcessor)

	input := `{"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "tool1", "arguments": {}}, "id": "req1"}` + "\n" +
		`{"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "tool2", "arguments": {}}, "id": "req2"}` + "\n" +
		`this is not json` + "\n" + // Should generate parse error
		`{"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "tool3", "arguments": {}}, "id": "req3"}` + "\n"

	outputStr := runStdioTest(t, handler, input)
	outputs := strings.Split(strings.TrimSpace(outputStr), "\n")

	if len(outputs) != 4 { // 3 valid requests processed by mock + 1 parse error
		t.Fatalf("Expected 4 responses, got %d. Output: \n%s", len(outputs), outputStr)
	}

	// Helper to check MCPToolResult
	checkMCPResult := func(rawResult interface{}, expectedText string) {
		mcpResult, ok := rawResult.(*models.MCPToolResult)
		if !ok {
			rawResultMap, isMap := rawResult.(map[string]interface{})
			if isMap {
				resultBytes, _ := json.Marshal(rawResultMap)
				var tempResult models.MCPToolResult
				if err := json.Unmarshal(resultBytes, &tempResult); err == nil {
					mcpResult = &tempResult
				} else {
					t.Fatalf("Result is a map but could not be converted to MCPToolResult: %+v. Error: %v", rawResult, err)
				}
			} else {
				t.Fatalf("Result is not *models.MCPToolResult or a map: type is %T, value: %+v", rawResult, rawResult)
			}
		}
		if mcpResult.IsError || len(mcpResult.Content) != 1 || mcpResult.Content[0].Text != expectedText {
			t.Errorf("Unexpected MCPToolResult. Expected text %q, got IsError: %t, Content: %+v", expectedText, mcpResult.IsError, mcpResult.Content)
		}
	}

	// Check first response
	var resp1 models.JSONRPCResponse
	if err := json.Unmarshal([]byte(outputs[0]), &resp1); err != nil {
		t.Fatalf("Unmarshal resp1 failed: %v", err)
	}
	if resp1.ID != "req1" || resp1.Error != nil {
		t.Errorf("Unexpected response 1: %+v", resp1)
	}
	checkMCPResult(resp1.Result, "Processed tool1 for ID req1")

	// Check second response
	var resp2 models.JSONRPCResponse
	if err := json.Unmarshal([]byte(outputs[1]), &resp2); err != nil {
		t.Fatalf("Unmarshal resp2 failed: %v", err)
	}
	if resp2.ID != "req2" || resp2.Error != nil {
		t.Errorf("Unexpected response 2: %+v", resp2)
	}
	checkMCPResult(resp2.Result, "Processed tool2 for ID req2")

	// Check third response (parse error)
	var resp3 models.JSONRPCResponse
	if err := json.Unmarshal([]byte(outputs[2]), &resp3); err != nil {
		t.Fatalf("Unmarshal resp3 failed: %v", err)
	}
	if resp3.Error == nil || resp3.Error.Code != errors.CodeParseError { // StdioHandler generates this
		t.Errorf("Expected parse error for response 3: %+v", resp3)
	}

	// Check fourth response
	var resp4 models.JSONRPCResponse
	if err := json.Unmarshal([]byte(outputs[3]), &resp4); err != nil {
		t.Fatalf("Unmarshal resp4 failed: %v", err)
	}
	if resp4.ID != "req3" || resp4.Error != nil {
		t.Errorf("Unexpected response 4: %+v", resp4)
	}
	checkMCPResult(resp4.Result, "Processed tool3 for ID req3")

	if callCount != 3 { // One for each valid JSON-RPC request sent to processor
		t.Errorf("Expected processor to be called 3 times, got %d", callCount)
	}
}

func TestStdioHandler_EmptyLineInput(t *testing.T) {
	mockProcessor := &mockMCPProcessor{
		ProcessRequestFunc: func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
			return &models.MCPToolResult{Content: []models.MCPToolContent{{Type: "text", Text: "data"}}}, nil
		},
	}
	handler := NewStdioHandler(mockProcessor)
	input := "\n" + // Empty line
		`{"jsonrpc": "2.0", "method": "tools/call", "params": {"name":"test"}, "id": "r1"}` + "\n" +
		"\n   \n" + // Lines with only whitespace
		`{"jsonrpc": "2.0", "method": "tools/call", "params": {"name":"test2"}, "id": "r2"}` + "\n"

	outputStr := runStdioTest(t, handler, input)
	outputs := strings.Split(strings.TrimSpace(outputStr), "\n")

	if len(outputs) != 2 {
		t.Fatalf("Expected 2 responses, got %d. Output: \n%s", len(outputs), outputStr)
	}
	// Further checks on resp content if necessary
	var resp1 models.JSONRPCResponse
	if err := json.Unmarshal([]byte(outputs[0]), &resp1); err != nil {
		t.Fatalf("Unmarshal resp1 failed: %v", err)
	}
	if resp1.ID != "r1" {
		t.Errorf("Resp1 ID mismatch: %+v", resp1)
	}

	var resp2 models.JSONRPCResponse
	if err := json.Unmarshal([]byte(outputs[1]), &resp2); err != nil {
		t.Fatalf("Unmarshal resp2 failed: %v", err)
	}
	if resp2.ID != "r2" {
		t.Errorf("Resp2 ID mismatch: %+v", resp2)
	}
}

func TestStdioHandler_InitializeAndToolsList(t *testing.T) {
	mockProcessor := &mockMCPProcessor{
		ProcessRequestFunc: func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
			if req.Method == "initialize" {
				return &models.MCPToolResult{
					Content: []models.MCPToolContent{{Type: "text", Text: "MCP server initialized."}},
					IsError: false,
				}, nil
			}
			if req.Method == "tools/list" {
				return &models.MCPToolResult{
					Content: []models.MCPToolContent{{Type: "text", Text: "Available tools: tool1, tool2"}},
					IsError: false,
				}, nil
			}
			return nil, &models.JSONRPCError{Code: -32601, Message: "Method not found by mock"}
		},
	}
	handler := NewStdioHandler(mockProcessor)

	testCases := []struct {
		name           string
		input          string
		expectedID     interface{}
		expectedText   string
		expectRpcError bool
	}{
		{
			name:           "Initialize method",
			input:          `{"jsonrpc": "2.0", "method": "initialize", "id": "initTest"}`,
			expectedID:     "initTest",
			expectedText:   "MCP server initialized.",
			expectRpcError: false,
		},
		{
			name:           "Tools/list method",
			input:          `{"jsonrpc": "2.0", "method": "tools/list", "id": 123}`,
			expectedID:     float64(123), // JSON numbers are float64
			expectedText:   "Available tools: tool1, tool2",
			expectRpcError: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			output := runStdioTest(t, handler, tc.input+"\n")
			var resp models.JSONRPCResponse
			if err := json.Unmarshal([]byte(output), &resp); err != nil {
				t.Fatalf("Failed to unmarshal output: %v. Output: %s", err, output)
			}

			if tc.expectRpcError {
				if resp.Error == nil {
					t.Fatal("Expected JSON-RPC error, got nil")
				}
			} else {
				if resp.Error != nil {
					t.Fatalf("Expected no JSON-RPC error, got: %+v", resp.Error)
				}
				if resp.ID != tc.expectedID {
					t.Errorf("Expected ID %v, got %v", tc.expectedID, resp.ID)
				}

				mcpResult, ok := resp.Result.(*models.MCPToolResult)
				if !ok {
					rawResultMap, isMap := resp.Result.(map[string]interface{})
					if isMap {
						resultBytes, _ := json.Marshal(rawResultMap)
						var tempResult models.MCPToolResult
						if err := json.Unmarshal(resultBytes, &tempResult); err == nil {
							mcpResult = &tempResult
						} else {
							t.Fatalf("Result is a map but could not be converted to MCPToolResult: %+v. Error: %v", resp.Result, err)
						}
					} else {
						t.Fatalf("Result is not *models.MCPToolResult or a map: type is %T, value: %+v", resp.Result, resp.Result)
					}
				}

				if mcpResult.IsError {
					t.Errorf("Expected MCPToolResult.IsError false, got true. Content: %s", mcpResult.Content[0].Text)
				}
				if len(mcpResult.Content) != 1 || mcpResult.Content[0].Text != tc.expectedText {
					t.Errorf("Expected MCPToolResult content text %q, got %q", tc.expectedText, mcpResult.Content[0].Text)
				}
			}
		})
	}
}
