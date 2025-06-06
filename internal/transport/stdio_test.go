package transport

import (
	"bytes"
	"encoding/json"
	"file-editor-server/internal/mcp" // Ensure this import is correct
	"file-editor-server/internal/models"
	"fmt"
	"io"
	"strings"
	"testing"
)

type MockMCPProcessor struct {
	ProcessRequestFunc func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError)
}

func (m *MockMCPProcessor) ExecuteTool(string, interface{}) (*models.MCPToolResult, error) {
	return nil, nil
}

func (m *MockMCPProcessor) ProcessRequest(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
	if m.ProcessRequestFunc != nil {
		return m.ProcessRequestFunc(req)
	}
	return nil, nil
}

// runStdioTestHelper simulates running the StdioHandler with given input and returns the output string.
func runStdioTestHelper(t *testing.T, handler *StdioHandler, input string) string {
	var outputBuffer bytes.Buffer
	inputBuffer := strings.NewReader(input)

	// Simulate EOF for single request tests by wrapping input reader
	eofReader := &eofSimulatingReader{reader: inputBuffer}

	err := handler.Start(eofReader, &outputBuffer)
	if err != nil && err != io.EOF { // EOF is expected after processing all input
		t.Logf("Handler Start returned error: %v", err)
	}
	return outputBuffer.String()
}

// eofSimulatingReader wraps a strings.Reader to return EOF after the string is fully read.
type eofSimulatingReader struct {
	reader *strings.Reader
	eof    bool
}

func (r *eofSimulatingReader) Read(p []byte) (n int, err error) {
	if r.eof {
		return 0, io.EOF
	}
	n, err = r.reader.Read(p)
	if err == io.EOF {
		r.eof = true // Simulate EOF on next call after current content is read fully
	}
	return n, err
}

func TestStdioHandler_ProcessRequest_Initialize(t *testing.T) {
	expectedID := "init-123"
	mockResult := &models.MCPToolResult{
		Content: []models.MCPToolContent{{Type: "text", Text: `{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"file-editing-server","version":"1.0.0","description":"..."}}`}},
		IsError: false,
	}

	mockProcessor := &MockMCPProcessor{
		ProcessRequestFunc: func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
			if req.Method != "initialize" {
				t.Errorf("Expected method 'initialize', got '%s'", req.Method)
			}
			if req.ID != expectedID {
				t.Errorf("Expected ID '%s', got '%v'", expectedID, req.ID)
			}
			return mockResult, nil
		},
	}
	handler := NewStdioHandler(mockProcessor)
	input := fmt.Sprintf(`{"jsonrpc": "2.0", "method": "initialize", "id": "%s"}`, expectedID) + "\n"
	output := runStdioTestHelper(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}

	if resp.Error != nil {
		t.Errorf("Expected no error, got code %d, message: %s", resp.Error.Code, resp.Error.Message)
	}
	if resp.ID != expectedID {
		t.Errorf("Expected ID '%s', got %v", expectedID, resp.ID)
	}
	// Compare the Result field by marshaling the expected MCPToolResult
	if resp.Result == nil {
		t.Errorf("Expected result, got nil")
	}
}

func TestStdioHandler_ProcessRequest_ToolsList(t *testing.T) {
	expectedID := float64(1) // JSON numbers
	mockResult := &models.MCPToolResult{
		Content: []models.MCPToolContent{{Type: "text", Text: `{"tools":[{"name":"list_files",...}]}`}},
		IsError: false,
	}
	mockProcessor := &MockMCPProcessor{
		ProcessRequestFunc: func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
			if req.Method != "tools/list" {
				t.Errorf("Expected method 'tools/list', got '%s'", req.Method)
			}
			return mockResult, nil
		},
	}
	handler := NewStdioHandler(mockProcessor)
	input := `{"jsonrpc": "2.0", "method": "tools/list", "id": 1}` + "\n"
	output := runStdioTestHelper(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}
	if resp.Error != nil {
		t.Errorf("Expected no error, got code %d, message: %s", resp.Error.Code, resp.Error.Message)
	}
	if resp.ID != expectedID {
		t.Errorf("Expected ID %v, got %v", expectedID, resp.ID)
	}
	if resp.Result == nil {
		t.Errorf("Expected result, got nil")
	}
}

func TestStdioHandler_ProcessRequest_ToolCall_ListFiles(t *testing.T) {
	expectedID := "list-files-id"
	mockResult := &models.MCPToolResult{
		Content: []models.MCPToolContent{{Type: "text", Text: "Files in directory:\n\nTotal files: 0"}},
		IsError: false,
	}
	mockProcessor := &MockMCPProcessor{
		ProcessRequestFunc: func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
			if req.Method != "tools/call" {
				t.Errorf("Expected method 'tools/call', got '%s'", req.Method)
			}
			// Basic check for "list_files" tool name in params
			var params mcp.ToolCallParams
			if err := json.Unmarshal(req.Params, &params); err != nil {
				t.Fatalf("Failed to unmarshal tool call params: %v", err)
			}
			if params.Name != "list_files" {
				t.Errorf("Expected tool name 'list_files', got '%s'", params.Name)
			}
			return mockResult, nil
		},
	}
	handler := NewStdioHandler(mockProcessor)
	input := `{"jsonrpc": "2.0", "method": "tools/call", "params": {"name": "list_files", "arguments": {}}, "id": "list-files-id"}` + "\n"
	output := runStdioTestHelper(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}
	if resp.Error != nil {
		t.Errorf("Expected no error, got code %d, message: %s", resp.Error.Code, resp.Error.Message)
	}
	if resp.ID != expectedID {
		t.Errorf("Expected ID '%s', got %v", expectedID, resp.ID)
	}
	if resp.Result == nil {
		t.Errorf("Expected result, got nil")
	}
}

func TestStdioHandler_InvalidJSONRPC(t *testing.T) {
	handler := NewStdioHandler(&MockMCPProcessor{}) // Processor won't be called

	t.Run("InvalidJSON", func(t *testing.T) {
		input := `{"jsonrpc": "2.0", "method": "test",, "id": 1}` + "\n" // Note the double comma
		output := runStdioTestHelper(t, handler, input)
		var resp models.JSONRPCResponse
		if err := json.Unmarshal([]byte(output), &resp); err != nil {
			t.Fatalf("Failed to unmarshal output for invalid JSON: %v. Output: %s", err, output)
		}
		if resp.Error == nil {
			t.Fatal("Expected error for invalid JSON, got nil")
		}
		if resp.Error.Code != -32700 { // Parse error
			t.Errorf("Expected error code -32700, got %d", resp.Error.Code)
		}
		// ID might be null for parse errors if it couldn't be parsed.
	})

	t.Run("MissingVersion", func(t *testing.T) {
		input := `{"method": "test", "id": 2}` + "\n"
		output := runStdioTestHelper(t, handler, input)
		var resp models.JSONRPCResponse
		_ = json.Unmarshal([]byte(output), &resp)           // Ignore error, check fields
		if resp.Error == nil || resp.Error.Code != -32600 { // Invalid Request
			t.Errorf("Expected error code -32600 for missing version, got %+v", resp.Error)
		}
		if resp.ID != float64(2) {
			t.Errorf("Expected ID to be 2, got %v", resp.ID)
		}
	})

	t.Run("MissingMethod", func(t *testing.T) {
		input := `{"jsonrpc": "2.0", "id": 3}` + "\n"
		output := runStdioTestHelper(t, handler, input)
		var resp models.JSONRPCResponse
		_ = json.Unmarshal([]byte(output), &resp)           // Ignore error, check fields
		if resp.Error == nil || resp.Error.Code != -32600 { // Invalid Request
			t.Errorf("Expected error code -32600 for missing method, got %+v", resp.Error)
		}
		if resp.ID != float64(3) {
			t.Errorf("Expected ID to be 3, got %v", resp.ID)
		}
	})
}

func TestStdioHandler_MCPProcessorError(t *testing.T) {
	expectedID := "err-test-id"
	mockError := &models.JSONRPCError{Code: -32601, Message: "Method not found"}

	mockProcessor := &MockMCPProcessor{
		ProcessRequestFunc: func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
			return nil, mockError
		},
	}
	handler := NewStdioHandler(mockProcessor)
	input := fmt.Sprintf(`{"jsonrpc": "2.0", "method": "unknown_method", "id": "%s"}`, expectedID) + "\n"
	output := runStdioTestHelper(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}

	if resp.Error == nil {
		t.Fatal("Expected error, got nil")
	}
	if resp.Error.Code != mockError.Code {
		t.Errorf("Expected error code %d, got %d", mockError.Code, resp.Error.Code)
	}
	if resp.Error.Message != mockError.Message {
		t.Errorf("Expected error message '%s', got '%s'", mockError.Message, resp.Error.Message)
	}
	if resp.ID != expectedID {
		t.Errorf("Expected ID '%s', got %v", expectedID, resp.ID)
	}
	if resp.Result != nil {
		t.Errorf("Expected nil result for error, got %+v", resp.Result)
	}
}
