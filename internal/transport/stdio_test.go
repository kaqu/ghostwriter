package transport

import (
	"bytes"
	"encoding/json"
	"file-editor-server/internal/errors"
	"file-editor-server/internal/models"
	"fmt" // Added fmt import
	// service_mock "file-editor-server/internal/service/mocks" // Using the same mock as http_test
	"strings"
	"testing"
	// "github.com/stretchr/testify/assert" // Using standard library for assertions
	// "github.com/stretchr/testify/mock"   // Using manual mock
)

// Using the same mockFileOperationService from http_test.go for consistency.
// If it were in a shared 'mocks' package, it would be imported.
// For this structure, it can be redefined or copied if it's small.
// Let's assume it's simple enough to redefine for clarity here, or use a shared mock later.

type mockStdioFileOperationService struct {
	ReadFileFunc func(req models.ReadFileRequest) (*models.ReadFileResponse, *models.ErrorDetail)
	EditFileFunc func(req models.EditFileRequest) (*models.EditFileResponse, *models.ErrorDetail)
}

func (m *mockStdioFileOperationService) ReadFile(req models.ReadFileRequest) (*models.ReadFileResponse, *models.ErrorDetail) {
	if m.ReadFileFunc != nil {
		return m.ReadFileFunc(req)
	}
	return nil, errors.NewInternalError("ReadFileFunc not implemented in mock")
}

func (m *mockStdioFileOperationService) EditFile(req models.EditFileRequest) (*models.EditFileResponse, *models.ErrorDetail) {
	if m.EditFileFunc != nil {
		return m.EditFileFunc(req)
	}
	return nil, errors.NewInternalError("EditFileFunc not implemented in mock")
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
	mockService := &mockStdioFileOperationService{
		ReadFileFunc: func(req models.ReadFileRequest) (*models.ReadFileResponse, *models.ErrorDetail) {
			if req.Name == "test.txt" {
				return &models.ReadFileResponse{Content: "file content", TotalLines: 1}, nil
			}
			return nil, errors.NewFileNotFoundError(req.Name, "read")
		},
	}
	handler := NewStdioHandler(mockService)

	// JSON-RPC request for read_file
	input := `{"jsonrpc": "2.0", "method": "read_file", "params": {"name": "test.txt"}, "id": 1}` + "\n"
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

	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Result is not a map: %+v", resp.Result)
	}
	if resultMap["content"] != "file content" {
		t.Errorf("Expected result content 'file content', got %v", resultMap["content"])
	}
}

func TestStdioHandler_EditFile_Success(t *testing.T) {
	mockService := &mockStdioFileOperationService{
		EditFileFunc: func(req models.EditFileRequest) (*models.EditFileResponse, *models.ErrorDetail) {
			return &models.EditFileResponse{Success: true, NewTotalLines: 10, LinesModified: 1, FileCreated: false}, nil
		},
	}
	handler := NewStdioHandler(mockService)

	input := `{"jsonrpc": "2.0", "method": "edit_file", "params": {"name": "edit.txt", "edits": []}, "id": "edit1"}` + "\n"
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
	resultMap, ok := resp.Result.(map[string]interface{})
	if !ok {
		t.Fatalf("Result is not a map: %+v", resp.Result)
	}
	if !resultMap["success"].(bool) {
		t.Error("Expected edit success to be true")
	}
}

func TestStdioHandler_ServiceError(t *testing.T) {
	mockService := &mockStdioFileOperationService{
		ReadFileFunc: func(req models.ReadFileRequest) (*models.ReadFileResponse, *models.ErrorDetail) {
			return nil, errors.NewFileNotFoundError(req.Name, "read")
		},
	}
	handler := NewStdioHandler(mockService)

	input := `{"jsonrpc": "2.0", "method": "read_file", "params": {"name": "no.txt"}, "id": 2}` + "\n"
	output := runStdioTest(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}

	if resp.Error == nil {
		t.Fatal("Expected error, got nil")
	}
	if resp.Error.Code != errors.CodeFileSystemError { // NewFileNotFoundError maps to this
		t.Errorf("Expected error code %d, got %d", errors.CodeFileSystemError, resp.Error.Code)
	}
	if resp.ID != float64(2) {
		t.Errorf("Expected ID 2, got %v", resp.ID)
	}
	if resp.Error.Data == nil {
		t.Error("Expected error data to be non-nil")
	} else {
		if resp.Error.Data.Filename != "no.txt" {
			t.Errorf("Expected filename 'no.txt' in error data, got '%s'", resp.Error.Data.Filename)
		}
		if resp.Error.Data.Operation != "read_file" { // Transport layer adds this
			t.Errorf("Expected operation 'read_file' in error data, got '%s'", resp.Error.Data.Operation)
		}
	}
}

func TestStdioHandler_InvalidJSON(t *testing.T) {
	handler := NewStdioHandler(&mockStdioFileOperationService{})
	input := `{"jsonrpc": "2.0", "method": "read_file",, "params": {name": "test.txt"}, "id": 3}` + "\n" // Invalid JSON
	output := runStdioTest(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}

	if resp.Error == nil {
		t.Fatal("Expected error, got nil")
	}
	if resp.Error.Code != errors.CodeParseError {
		t.Errorf("Expected error code %d, got %d", errors.CodeParseError, resp.Error.Code)
	}
	// ID for parse error is typically null
	if resp.ID != nil {
		t.Errorf("Expected ID nil for parse error, got %v", resp.ID)
	}
}

func TestStdioHandler_InvalidJSONRPCVersion(t *testing.T) {
	handler := NewStdioHandler(&mockStdioFileOperationService{})
	input := `{"jsonrpc": "1.0", "method": "read_file", "params": {"name": "test.txt"}, "id": 4}` + "\n"
	output := runStdioTest(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}

	if resp.Error == nil {
		t.Fatal("Expected error, got nil")
	}
	if resp.Error.Code != errors.CodeInvalidRequest {
		t.Errorf("Expected error code %d, got %d. Message: %s", errors.CodeInvalidRequest, resp.Error.Code, resp.Error.Message)
	}
	if resp.ID != float64(4) { // ID should still be preserved
		t.Errorf("Expected ID 4, got %v", resp.ID)
	}
}

func TestStdioHandler_UnknownMethod(t *testing.T) {
	handler := NewStdioHandler(&mockStdioFileOperationService{})
	input := `{"jsonrpc": "2.0", "method": "non_existent_method", "params": {}, "id": 5}` + "\n"
	output := runStdioTest(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}

	if resp.Error == nil {
		t.Fatal("Expected error, got nil")
	}
	if resp.Error.Code != errors.CodeMethodNotFound {
		t.Errorf("Expected error code %d, got %d", errors.CodeMethodNotFound, resp.Error.Code)
	}
	if resp.ID != float64(5) {
		t.Errorf("Expected ID 5, got %v", resp.ID)
	}
}

func TestStdioHandler_InvalidParams(t *testing.T) {
	mockService := &mockStdioFileOperationService{
		// ReadFile will not be called if params are invalid before service call
	}
	handler := NewStdioHandler(mockService)
	// Params for read_file should be an object, not an array
	input := `{"jsonrpc": "2.0", "method": "read_file", "params": ["invalid_array"], "id": 6}` + "\n"
	output := runStdioTest(t, handler, input)

	var resp models.JSONRPCResponse
	if err := json.Unmarshal([]byte(output), &resp); err != nil {
		t.Fatalf("Failed to unmarshal output: %v. Output was: %s", err, output)
	}

	if resp.Error == nil {
		t.Fatal("Expected error, got nil")
	}
	if resp.Error.Code != errors.CodeInvalidParams {
		t.Errorf("Expected error code %d, got %d", errors.CodeInvalidParams, resp.Error.Code)
	}
	if resp.ID != float64(6) {
		t.Errorf("Expected ID 6, got %v", resp.ID)
	}
}

func TestStdioHandler_MultipleRequests(t *testing.T) {
	callCount := 0
	mockService := &mockStdioFileOperationService{
		ReadFileFunc: func(req models.ReadFileRequest) (*models.ReadFileResponse, *models.ErrorDetail) {
			callCount++
			return &models.ReadFileResponse{Content: fmt.Sprintf("content %d for %s", callCount, req.Name)}, nil
		},
	}
	handler := NewStdioHandler(mockService)

	input := `{"jsonrpc": "2.0", "method": "read_file", "params": {"name": "file1.txt"}, "id": "req1"}` + "\n" +
	         `{"jsonrpc": "2.0", "method": "read_file", "params": {"name": "file2.txt"}, "id": "req2"}` + "\n" +
			 `this is not json` + "\n" + // Should generate parse error
			 `{"jsonrpc": "2.0", "method": "read_file", "params": {"name": "file3.txt"}, "id": "req3"}` + "\n"

	outputStr := runStdioTest(t, handler, input)
	outputs := strings.Split(strings.TrimSpace(outputStr), "\n")

	if len(outputs) != 4 { // 3 valid requests + 1 parse error
		t.Fatalf("Expected 4 responses, got %d. Output: \n%s", len(outputs), outputStr)
	}

	// Check first response
	var resp1 models.JSONRPCResponse
	if err := json.Unmarshal([]byte(outputs[0]), &resp1); err != nil {t.Fatalf("Unmarshal resp1 failed: %v", err)}
	if resp1.ID != "req1" || resp1.Error != nil || resp1.Result.(map[string]interface{})["content"] != "content 1 for file1.txt" {
		t.Errorf("Unexpected response 1: %+v", resp1)
	}

	// Check second response
	var resp2 models.JSONRPCResponse
	if err := json.Unmarshal([]byte(outputs[1]), &resp2); err != nil {t.Fatalf("Unmarshal resp2 failed: %v", err)}
	if resp2.ID != "req2" || resp2.Error != nil || resp2.Result.(map[string]interface{})["content"] != "content 2 for file2.txt" {
		t.Errorf("Unexpected response 2: %+v", resp2)
	}

	// Check third response (parse error)
	var resp3 models.JSONRPCResponse
	if err := json.Unmarshal([]byte(outputs[2]), &resp3); err != nil {t.Fatalf("Unmarshal resp3 failed: %v", err)}
	if resp3.Error == nil || resp3.Error.Code != errors.CodeParseError {
		t.Errorf("Expected parse error for response 3: %+v", resp3)
	}

	// Check fourth response
	var resp4 models.JSONRPCResponse
	if err := json.Unmarshal([]byte(outputs[3]), &resp4); err != nil {t.Fatalf("Unmarshal resp4 failed: %v", err)}
	if resp4.ID != "req3" || resp4.Error != nil || resp4.Result.(map[string]interface{})["content"] != "content 3 for file3.txt" {
		t.Errorf("Unexpected response 4: %+v", resp4)
	}

	if callCount != 3 {
		t.Errorf("Expected service to be called 3 times, got %d", callCount)
	}
}

func TestStdioHandler_EmptyLineInput(t *testing.T) {
	mockService := &mockStdioFileOperationService{
		ReadFileFunc: func(req models.ReadFileRequest) (*models.ReadFileResponse, *models.ErrorDetail) {
			return &models.ReadFileResponse{Content: "data"}, nil
		},
	}
	handler := NewStdioHandler(mockService)
	input := "\n" + // Empty line
	         `{"jsonrpc": "2.0", "method": "read_file", "params": {"name": "file.txt"}, "id": "r1"}` + "\n" +
			 "\n   \n" + // Lines with only whitespace
			 `{"jsonrpc": "2.0", "method": "read_file", "params": {"name": "file2.txt"}, "id": "r2"}` + "\n"

	outputStr := runStdioTest(t, handler, input)
	outputs := strings.Split(strings.TrimSpace(outputStr), "\n")

	if len(outputs) != 2 {
		t.Fatalf("Expected 2 responses, got %d. Output: \n%s", len(outputs), outputStr)
	}
	// Further checks on resp content if necessary
	var resp1 models.JSONRPCResponse
	if err := json.Unmarshal([]byte(outputs[0]), &resp1); err != nil {t.Fatalf("Unmarshal resp1 failed: %v", err)}
	if resp1.ID != "r1" {t.Errorf("Resp1 ID mismatch: %+v", resp1) }

	var resp2 models.JSONRPCResponse
	if err := json.Unmarshal([]byte(outputs[1]), &resp2); err != nil {t.Fatalf("Unmarshal resp2 failed: %v", err)}
	if resp2.ID != "r2" {t.Errorf("Resp2 ID mismatch: %+v", resp2) }
}
