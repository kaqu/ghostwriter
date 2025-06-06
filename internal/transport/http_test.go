package transport

import (
	"bytes"
	"encoding/json"
	"file-editor-server/internal/errors" // Using for error codes if needed
	"file-editor-server/internal/mcp"
	"file-editor-server/internal/models"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// MockMCPProcessor is a mock implementation of mcp.MCPProcessorInterface.
type MockMCPProcessor struct {
	ExecuteToolFunc    func(toolName string, argumentsStruct interface{}) (*models.MCPToolResult, error)
	ProcessRequestFunc func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError)
}

func (m *MockMCPProcessor) ExecuteTool(toolName string, argumentsStruct interface{}) (*models.MCPToolResult, error) {
	if m.ExecuteToolFunc != nil {
		return m.ExecuteToolFunc(toolName, argumentsStruct)
	}
	return nil, fmt.Errorf("ExecuteToolFunc not implemented")
}

func (m *MockMCPProcessor) ProcessRequest(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
	if m.ProcessRequestFunc != nil {
		return m.ProcessRequestFunc(req)
	}
	// This method is for stdio, but interface requires it.
	return nil, &models.JSONRPCError{Code: -32000, Message: "ProcessRequestFunc not implemented in HTTP test mock"}
}

func TestHTTPHandler_ToolCalls(t *testing.T) {
	tests := []struct {
		name           string
		toolPath       string // e.g., "/list_files"
		toolName       string // e.g., "list_files"
		reqBody        interface{}
		mockMCPResult  *models.MCPToolResult
		mockMCPError   error
		expectedStatus int
	}{
		{
			name:     "list_files success",
			toolPath: "/list_files",
			toolName: "list_files",
			reqBody:  models.ListFilesRequest{},
			mockMCPResult: &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: "list_files_output"}},
				IsError: false,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "read_file success",
			toolPath: "/read_file",
			toolName: "read_file",
			reqBody:  models.ReadFileRequest{Name: "test.txt"},
			mockMCPResult: &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: "read_file_output"}},
				IsError: false,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "edit_file success",
			toolPath: "/edit_file",
			toolName: "edit_file",
			reqBody:  models.EditFileRequest{Name: "edit.txt"},
			mockMCPResult: &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: "edit_file_output"}},
				IsError: false,
			},
			expectedStatus: http.StatusOK,
		},
		{
			name:     "tool execution returns MCP error result",
			toolPath: "/read_file",
			toolName: "read_file",
			reqBody:  models.ReadFileRequest{Name: "error.txt"},
			mockMCPResult: &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: "MCP error for read_file"}},
				IsError: true,
			},
			expectedStatus: http.StatusOK, // Still 200 OK, error is in MCP result
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockProcessor := &MockMCPProcessor{
				ExecuteToolFunc: func(toolName string, argumentsStruct interface{}) (*models.MCPToolResult, error) {
					if toolName != tt.toolName {
						t.Errorf("Expected tool name '%s', got '%s'", tt.toolName, toolName)
					}
					// Basic type check for argumentsStruct can be added here if necessary
					return tt.mockMCPResult, tt.mockMCPError
				},
			}
			// Service is now part of MCPProcessor, so HTTPHandler doesn't need it directly.
			// The `nil` for service in NewHTTPHandler is fine if it's not used,
			// but since we provide mcpProcessor, it should be okay.
			// The second arg for NewHTTPHandler is cfgMaxReqSizeMB.
			handler := NewHTTPHandler(nil, mockProcessor, 10) // 10MB max request size

			reqBytes, _ := json.Marshal(tt.reqBody)
			req, _ := http.NewRequest("POST", tt.toolPath, bytes.NewBuffer(reqBytes))
			req.Header.Set("Content-Type", "application/json")

			rr := httptest.NewRecorder()
			// Get the appropriate handler func based on tt.toolPath
			var httpHandlerFunc http.HandlerFunc
			switch tt.toolPath {
			case "/list_files":
				httpHandlerFunc = handler.handleListFiles
			case "/read_file":
				httpHandlerFunc = handler.handleReadFile
			case "/edit_file":
				httpHandlerFunc = handler.handleEditFile
			default:
				t.Fatalf("No handler registered for path: %s", tt.toolPath)
			}
			httpHandlerFunc(rr, req)

			if status := rr.Code; status != tt.expectedStatus {
				t.Errorf("Handler returned wrong status code: got %v want %v. Body: %s", status, tt.expectedStatus, rr.Body.String())
			}

			if tt.expectedStatus == http.StatusOK {
				var mcpResp models.MCPToolResult
				if err := json.NewDecoder(rr.Body).Decode(&mcpResp); err != nil {
					t.Fatalf("Failed to decode MCPToolResult response: %v. Body: %s", err, rr.Body.String())
				}
				if mcpResp.IsError != tt.mockMCPResult.IsError {
					t.Errorf("Expected IsError %t, got %t", tt.mockMCPResult.IsError, mcpResp.IsError)
				}
				if len(mcpResp.Content) > 0 && len(tt.mockMCPResult.Content) > 0 &&
					mcpResp.Content[0].Text != tt.mockMCPResult.Content[0].Text {
					t.Errorf("Expected content text %q, got %q", tt.mockMCPResult.Content[0].Text, mcpResp.Content[0].Text)
				}
			}
		})
	}
}

func TestHTTPHandler_InvalidToolArguments(t *testing.T) {
	mockProcessor := &MockMCPProcessor{} // Not expected to be called for these errors
	handler := NewHTTPHandler(nil, mockProcessor, 10)

	server := httptest.NewServer(http.HandlerFunc(handler.handleReadFile)) // Using ReadFile for test
	defer server.Close()

	// Malformed JSON
	reqBodyMalformed := `{"name": "test.txt",,}`
	respMalformed, _ := http.Post(server.URL, "application/json", bytes.NewBufferString(reqBodyMalformed))
	if respMalformed.StatusCode != http.StatusBadRequest {
		body, _ := io.ReadAll(respMalformed.Body)
		t.Errorf("Expected status %d for malformed JSON, got %d. Body: %s", http.StatusBadRequest, respMalformed.StatusCode, string(body))
	}
	respMalformed.Body.Close()

	// JSON not matching schema (e.g. unknown field, if DisallowUnknownFields is active)
	// models.ReadFileRequest has Name, StartLine, EndLine.
	reqBodyUnknownField := `{"name": "test.txt", "unexpected_field": 123}`
	respUnknown, _ := http.Post(server.URL, "application/json", bytes.NewBufferString(reqBodyUnknownField))
	if respUnknown.StatusCode != http.StatusBadRequest { // Decoder with DisallowUnknownFields should cause this
		body, _ := io.ReadAll(respUnknown.Body)
		t.Errorf("Expected status %d for JSON with unknown field, got %d. Body: %s", http.StatusBadRequest, respUnknown.StatusCode, string(body))
	}
	var errResp models.ErrorResponse
	if err := json.NewDecoder(respUnknown.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response for unknown field: %v", err)
	}
	if errResp.Error.Code != errors.CodeParseError || !strings.Contains(errResp.Error.Message, "unknown field") {
		t.Errorf("Unexpected error response for unknown field: %+v", errResp.Error)
	}
	respUnknown.Body.Close()
}

func TestHTTPHandler_MCPProcessorError(t *testing.T) {
	mockProcessor := &MockMCPProcessor{
		ExecuteToolFunc: func(toolName string, argumentsStruct interface{}) (*models.MCPToolResult, error) {
			// This error is from ExecuteTool itself (e.g. bad arg type passed from handler, or internal setup error)
			return nil, fmt.Errorf("internal processor issue")
		},
	}
	handler := NewHTTPHandler(nil, mockProcessor, 10)
	server := httptest.NewServer(http.HandlerFunc(handler.handleReadFile)) // Using ReadFile for test
	defer server.Close()

	reqBody := `{"name": "test.txt"}` // Valid request for ReadFile
	resp, _ := http.Post(server.URL, "application/json", bytes.NewBufferString(reqBody))

	if resp.StatusCode != http.StatusInternalServerError {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("Expected status %d for MCPProcessor error, got %d. Body: %s", http.StatusInternalServerError, resp.StatusCode, string(body))
	}
	var errResp models.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response for MCPProcessor error: %v", err)
	}
	if errResp.Error.Code != errors.CodeInternalError || !strings.Contains(errResp.Error.Message, "internal processor issue") {
		t.Errorf("Unexpected error response for MCPProcessor error: %+v", errResp.Error)
	}
	resp.Body.Close()
}

func TestHTTPHandler_TransportErrors(t *testing.T) {
	mockProcessor := &MockMCPProcessor{} // Not called for these
	handler := NewHTTPHandler(nil, mockProcessor, 1) // 1MB max size

	// Test MethodNotAllowed
	t.Run("MethodNotAllowed", func(t *testing.T) {
		req, _ := http.NewRequest("GET", "/read_file", nil) // GET instead of POST
		rr := httptest.NewRecorder()
		handler.handleReadFile(rr, req)
		if status := rr.Code; status != http.StatusMethodNotAllowed {
			t.Errorf("handleReadFile GET: expected status %d, got %d", http.StatusMethodNotAllowed, status)
		}
	})

	// Test UnsupportedMediaType
	t.Run("UnsupportedMediaType", func(t *testing.T) {
		req, _ := http.NewRequest("POST", "/read_file", bytes.NewBufferString(`{"name":"test.txt"}`))
		req.Header.Set("Content-Type", "text/plain") // Wrong Content-Type
		rr := httptest.NewRecorder()
		handler.handleReadFile(rr, req)
		if status := rr.Code; status != http.StatusUnsupportedMediaType {
			t.Errorf("handleReadFile wrong Content-Type: expected status %d, got %d", http.StatusUnsupportedMediaType, status)
		}
	})

	// Test BodyTooLarge
	t.Run("BodyTooLarge", func(t *testing.T) {
		// MaxFileSizeMB is 10 for the handler, maxReqSize is 1MB.
		// Let's set handler's maxReqSize to something small for this test.
		handler.maxReqSize = 10 // 10 bytes
		largeBody := `{"name": "this_is_a_very_long_filename_to_exceed_10_bytes"}`
		req, _ := http.NewRequest("POST", "/read_file", bytes.NewBufferString(largeBody))
		req.Header.Set("Content-Type", "application/json")
		rr := httptest.NewRecorder()
		handler.handleReadFile(rr, req)
		if status := rr.Code; status != http.StatusRequestEntityTooLarge {
			t.Errorf("handleReadFile body too large: expected status %d, got %d. Body: %s", http.StatusRequestEntityTooLarge, status, rr.Body.String())
		}
	})
}

func TestHTTPHandler_HealthCheck(t *testing.T) {
	handler := NewHTTPHandler(nil, nil, 0) // Service and processor not needed for health check
	server := httptest.NewServer(http.HandlerFunc(handler.handleHealthCheck))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("GET request to /health failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status %d for /health, got %d", http.StatusOK, resp.StatusCode)
	}
	var statusMap map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&statusMap); err != nil {
		t.Fatalf("Failed to decode /health response: %v", err)
	}
	if statusMap["status"] != "ok" {
		t.Errorf("expected status 'ok' in /health response, got %s", statusMap["status"])
	}
}
