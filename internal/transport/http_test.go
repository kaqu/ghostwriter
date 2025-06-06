package transport

import (
	"bytes"
	"encoding/json"
	"file-editor-server/internal/errors"
	"file-editor-server/internal/models"
	// "file-editor-server/internal/service" // No longer directly needed due to mock
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	// "time" // No longer directly needed
)

// --- Mock FileOperationService ---
// This mock is adjusted to align with the new service signatures used by MCPProcessor tests.
type mockFileOperationService struct {
	ListFilesFunc func(req models.ListFilesRequest) ([]models.FileInfo, string, *models.ErrorDetail)
	ReadFileFunc  func(req models.ReadFileRequest) (content string, filename string, totalLines int, reqStartLine int, reqEndLine int, actualEndLine int, isRangeRequest bool, err *models.ErrorDetail)
	EditFileFunc  func(req models.EditFileRequest) (filename string, linesModified int, newTotalLines int, fileCreated bool, err *models.ErrorDetail)
}

func (m *mockFileOperationService) ListFiles(req models.ListFilesRequest) ([]models.FileInfo, string, *models.ErrorDetail) {
	if m.ListFilesFunc != nil {
		return m.ListFilesFunc(req)
	}
	return nil, "", errors.NewInternalError("ListFilesFunc not implemented in mock")
}

func (m *mockFileOperationService) ReadFile(req models.ReadFileRequest) (content string, filename string, totalLines int, reqStartLine int, reqEndLine int, actualEndLine int, isRangeRequest bool, err *models.ErrorDetail) {
	if m.ReadFileFunc != nil {
		return m.ReadFileFunc(req)
	}
	return "", "", 0, 0, 0, 0, false, errors.NewInternalError("ReadFileFunc not implemented in mock")
}

func (m *mockFileOperationService) EditFile(req models.EditFileRequest) (filename string, linesModified int, newTotalLines int, fileCreated bool, err *models.ErrorDetail) {
	if m.EditFileFunc != nil {
		return m.EditFileFunc(req)
	}
	return "", 0, 0, false, errors.NewInternalError("EditFileFunc not implemented in mock")
}


func TestHTTPHandler_handleReadFile_Success(t *testing.T) {
	mockService := &mockFileOperationService{
		ReadFileFunc: func(req models.ReadFileRequest) (string, string, int, int, int, int, bool, *models.ErrorDetail) {
			if req.Name == "test.txt" {
				// content string, filename string, totalLines int, reqStartLine int, reqEndLine int, actualEndLine int, isRangeRequest bool
				return "hello world", req.Name, 1, req.StartLine, req.EndLine, 0, (req.StartLine != 0 || req.EndLine != 0), nil
			}
			return "", req.Name, 0, 0, 0, -1, false, errors.NewFileNotFoundError(req.Name, "read")
		},
	}
	handler := NewHTTPHandler(mockService, 10)
	server := httptest.NewServer(http.HandlerFunc(handler.handleReadFile))
	defer server.Close()

	reqBody := `{"name": "test.txt"}`
	resp, err := http.Post(server.URL, "application/json", bytes.NewBufferString(reqBody))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("expected status %d, got %d. Body: %s", http.StatusOK, resp.StatusCode, string(bodyBytes))
	}

	var mcpResp models.MCPToolResult
	if err := json.NewDecoder(resp.Body).Decode(&mcpResp); err != nil {
		t.Fatalf("Failed to decode MCPToolResult response: %v", err)
	}
	if mcpResp.IsError {
		t.Errorf("Expected IsError to be false, got true. Content: %s", mcpResp.Content[0].Text)
	}
	// formatHTTPReadFileResult(content string, filename string, totalLines int, reqStartLine int, reqEndLine int, actualEndLine int, isRangeRequest bool)
	expectedText := formatHTTPReadFileResult("hello world", "test.txt", 1, 0,0,0, false)
	if len(mcpResp.Content) != 1 || mcpResp.Content[0].Text != expectedText {
		t.Errorf("Expected content text %q, got %q", expectedText, mcpResp.Content[0].Text)
	}
}

func TestHTTPHandler_handleReadFile_ServiceError(t *testing.T) {
	mockService := &mockFileOperationService{
		ReadFileFunc: func(req models.ReadFileRequest) (string, string, int, int, int, int, bool, *models.ErrorDetail) {
			return "", req.Name, 0, 0, 0, -1, false, errors.NewFileNotFoundError(req.Name, "read")
		},
	}
	handler := NewHTTPHandler(mockService, 10)
	server := httptest.NewServer(http.HandlerFunc(handler.handleReadFile))
	defer server.Close()

	reqBody := `{"name": "nonexistent.txt"}`
	resp, err := http.Post(server.URL, "application/json", bytes.NewBufferString(reqBody))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	// Service errors are now returned as HTTP 200 with IsError: true in MCPToolResult
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("expected status %d, got %d. Body: %s", http.StatusOK, resp.StatusCode, string(bodyBytes))
	}

	var mcpResp models.MCPToolResult
	if err := json.NewDecoder(resp.Body).Decode(&mcpResp); err != nil {
		t.Fatalf("Failed to decode MCPToolResult error response: %v", err)
	}
	if !mcpResp.IsError {
		t.Error("Expected IsError to be true")
	}
	serviceErr := errors.NewFileNotFoundError("nonexistent.txt", "read")
	expectedErrorText := formatHTTPToolError(serviceErr)
	if len(mcpResp.Content) != 1 || mcpResp.Content[0].Text != expectedErrorText {
		t.Errorf("Expected error text %q, got %q", expectedErrorText, mcpResp.Content[0].Text)
	}
}

func TestHTTPHandler_handleReadFile_InvalidJSON(t *testing.T) {
	handler := NewHTTPHandler(&mockFileOperationService{}, 10)
	server := httptest.NewServer(http.HandlerFunc(handler.handleReadFile))
	defer server.Close()

	reqBody := `{"name": "test.txt",,}` // Invalid JSON
	resp, err := http.Post(server.URL, "application/json", bytes.NewBufferString(reqBody))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}
	var errResp models.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}
	if errResp.Error.Code != errors.CodeParseError {
		t.Errorf("expected error code %d, got %d", errors.CodeParseError, errResp.Error.Code)
	}
}

func TestHTTPHandler_handleReadFile_BodyTooLarge(t *testing.T) {
	handler := NewHTTPHandler(&mockFileOperationService{}, 10)
	handler.maxReqSize = 10 // Set small for test
	server := httptest.NewServer(http.HandlerFunc(handler.handleReadFile))
	defer server.Close()

	reqBody := `{"name": "this_body_is_definitely_larger_than_10_bytes"}`
	resp, err := http.Post(server.URL, "application/json", bytes.NewBufferString(reqBody))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("expected status %d, got %d", http.StatusRequestEntityTooLarge, resp.StatusCode)
	}
	// Body might be empty or contain a specific error from MaxBytesReader, then our handler writes JSON error
	var errResp models.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		// MaxBytesReader might close the connection or send a plain text error
		// For this test, primarily check the status code. The exact body can vary.
		bodyBytes, _ := io.ReadAll(io.NopCloser(bytes.NewBufferString(errResp.Error.Message))) // Read it if JSON parsing failed
		t.Logf("Response body for too large: %s", string(bodyBytes))
		// t.Fatalf("Failed to decode error response (this might be okay if MaxBytesReader sent plain text): %v", err)
	} else {
		if errResp.Error.Message == "" {
			t.Errorf("Expected non-empty error message for too large request")
		}
	}
}

func TestHTTPHandler_handleReadFile_WrongMethod(t *testing.T) {
	handler := NewHTTPHandler(&mockFileOperationService{}, 10)
	server := httptest.NewServer(http.HandlerFunc(handler.handleReadFile))
	defer server.Close()

	resp, err := http.Get(server.URL) // Using GET instead of POST
	if err != nil {
		t.Fatalf("GET request failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, resp.StatusCode)
	}
}

func TestHTTPHandler_handleEditFile_Success(t *testing.T) {
	mockService := &mockFileOperationService{
		EditFileFunc: func(req models.EditFileRequest) (string, int, int, bool, *models.ErrorDetail) {
			if req.Name == "editme.txt" {
				// filename string, linesModified int, newTotalLines int, fileCreated bool
				return req.Name, 1, 5, false, nil
			}
			return "", 0, 0, false, errors.NewInternalError("mock edit error")
		},
	}
	handler := NewHTTPHandler(mockService, 10)
	server := httptest.NewServer(http.HandlerFunc(handler.handleEditFile))
	defer server.Close()

	reqBody := `{"name": "editme.txt", "edits": [{"line": 1, "operation": "insert", "content": "new first line"}]}`
	resp, err := http.Post(server.URL, "application/json", bytes.NewBufferString(reqBody))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("expected status %d, got %d. Body: %s", http.StatusOK, resp.StatusCode, string(bodyBytes))
	}
	var mcpResp models.MCPToolResult
	if err := json.NewDecoder(resp.Body).Decode(&mcpResp); err != nil {
		t.Fatalf("Failed to decode MCPToolResult response: %v", err)
	}
	if mcpResp.IsError {
		t.Errorf("Expected IsError to be false, got true. Content: %s", mcpResp.Content[0].Text)
	}
	expectedText := formatHTTPEditFileResult("editme.txt", 1, 5, false)
	if len(mcpResp.Content) != 1 || mcpResp.Content[0].Text != expectedText {
		t.Errorf("Expected content text %q, got %q", expectedText, mcpResp.Content[0].Text)
	}
}

func TestHTTPHandler_handleEditFile_ServiceError(t *testing.T) {
	serviceErrDetail := errors.NewInvalidParamsError("Invalid edit operation", nil)
	mockService := &mockFileOperationService{
		EditFileFunc: func(req models.EditFileRequest) (string, int, int, bool, *models.ErrorDetail) {
			return req.Name, 0, 0, false, serviceErrDetail
		},
	}
	handler := NewHTTPHandler(mockService, 10)
	server := httptest.NewServer(http.HandlerFunc(handler.handleEditFile))
	defer server.Close()

	reqBody := `{"name": "badedit.txt", "edits": [{"line": 0, "operation": "insert", "content": "fail"}]}` // Line 0 is invalid
	resp, err := http.Post(server.URL, "application/json", bytes.NewBufferString(reqBody))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK { // Service errors now return 200 OK with MCPToolResult.IsError = true
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("expected status %d, got %d. Body: %s", http.StatusOK, resp.StatusCode, string(bodyBytes))
	}
	var mcpResp models.MCPToolResult
	if err := json.NewDecoder(resp.Body).Decode(&mcpResp); err != nil {
		t.Fatalf("Failed to decode MCPToolResult error response: %v", err)
	}
	if !mcpResp.IsError {
		t.Error("Expected IsError to be true")
	}
	expectedErrorText := formatHTTPToolError(serviceErrDetail)
	if len(mcpResp.Content) != 1 || mcpResp.Content[0].Text != expectedErrorText {
		t.Errorf("Expected error text %q, got %q", expectedErrorText, mcpResp.Content[0].Text)
	}
}

func TestHTTPHandler_HealthCheck(t *testing.T) {
	handler := NewHTTPHandler(nil, 0) // Service not needed for health check
	server := httptest.NewServer(http.HandlerFunc(handler.handleHealthCheck))
	defer server.Close()

	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("GET request to /health failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

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

func TestHTTPHandler_RegisterRoutes(t *testing.T) {
	// Provide a minimal mock service, even if its methods aren't deeply tested here.
	mockService := &mockFileOperationService{
		ReadFileFunc: func(req models.ReadFileRequest) (string, string, int, int, int, int, bool, *models.ErrorDetail) {
			// For ReadFile, the service expects a "name" field. Empty JSON "{}" will lead to service error if not caught by decode.
			// However, the HTTP handler decodes into models.ReadFileRequest first. If "name" is missing, it's a bad request.
			// If "name" is present but file not found by service, then service error.
			// This mock will simulate a service error if called, to show the path.
			return "", req.Name, 0, 0,0,0,false, errors.NewInvalidParamsError("test read error from mock", nil)
		},
		EditFileFunc: func(req models.EditFileRequest) (string, int, int, bool, *models.ErrorDetail) {
			return req.Name, 0,0,false, errors.NewInvalidParamsError("test edit error from mock", nil)
		},
		ListFilesFunc: func(req models.ListFilesRequest) ([]models.FileInfo, string, *models.ErrorDetail) {
			return []models.FileInfo{}, "/testdir", nil // Successful list files
		},
	}
	handler := NewHTTPHandler(mockService, 1)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test /list_files (success)
	reqListFiles, _ := http.NewRequest("POST", "/list_files", bytes.NewBufferString("{}")) // Empty JSON object for ListFiles
	reqListFiles.Header.Set("Content-Type", "application/json")
	rrListFiles := httptest.NewRecorder()
	mux.ServeHTTP(rrListFiles, reqListFiles)
	if rrListFiles.Code != http.StatusOK {
		t.Errorf("/list_files: expected status %d, got %d. Body: %s", http.StatusOK, rrListFiles.Code, rrListFiles.Body.String())
	}
	var mcpListResp models.MCPToolResult
	if err := json.NewDecoder(rrListFiles.Body).Decode(&mcpListResp); err != nil {
		t.Fatalf("/list_files: Failed to decode MCPToolResult: %v", err)
	}
	if mcpListResp.IsError {
		t.Errorf("/list_files: expected IsError false, got true. Content: %s", mcpListResp.Content[0].Text)
	}
	expectedListText := formatHTTPListFilesResult([]models.FileInfo{}, "/testdir")
	if len(mcpListResp.Content) != 1 || mcpListResp.Content[0].Text != expectedListText {
		t.Errorf("/list_files: expected content text %q, got %q", expectedListText, mcpListResp.Content[0].Text)
	}


	// Test /read_file (expecting bad request from handler due to missing "name" in "{}")
	reqReadFile, _ := http.NewRequest("POST", "/read_file", bytes.NewBufferString("{}"))
	reqReadFile.Header.Set("Content-Type", "application/json")
	rrReadFile := httptest.NewRecorder()
	mux.ServeHTTP(rrReadFile, reqReadFile)
	// The ReadFileRequest requires a "name". Sending "{}" will fail model validation at service level,
	// or if strict decoding is used in handler before service call (which it is).
	// The service call `h.service.ReadFile(req)` will receive a req with empty Name.
	// The service's `resolveAndValidatePath` will return an error for empty filename.
	// This service error will then be wrapped into MCPToolResult with IsError:true.
	if rrReadFile.Code != http.StatusOK { // Should be 200 OK, with MCPToolResult.IsError=true
		t.Errorf("/read_file: expected status %d, got %d. Body: %s", http.StatusOK, rrReadFile.Code, rrReadFile.Body.String())
	}
	var mcpReadResp models.MCPToolResult
	if err := json.NewDecoder(rrReadFile.Body).Decode(&mcpReadResp); err != nil {
		t.Fatalf("/read_file: Failed to decode MCPToolResult: %v", err)
	}
	if !mcpReadResp.IsError {
		t.Errorf("/read_file: expected IsError true for empty JSON, got false. Content: %s", mcpReadResp.Content[0].Text)
	}
	// Check for specific error message related to empty filename might be too brittle.
	// The key is that it's an error reported via MCPToolResult.

	// Test /edit_file (expecting bad request from handler due to missing "name" in "{}")
	reqEditFile, _ := http.NewRequest("POST", "/edit_file", bytes.NewBufferString("{}"))
	reqEditFile.Header.Set("Content-Type", "application/json")
	rrEditFile := httptest.NewRecorder()
	mux.ServeHTTP(rrEditFile, reqEditFile)
	if rrEditFile.Code != http.StatusOK { // Should be 200 OK, with MCPToolResult.IsError=true
		t.Errorf("/edit_file: expected status %d, got %d. Body: %s", http.StatusOK, rrEditFile.Code, rrEditFile.Body.String())
	}
	var mcpEditResp models.MCPToolResult
	if err := json.NewDecoder(rrEditFile.Body).Decode(&mcpEditResp); err != nil {
		t.Fatalf("/edit_file: Failed to decode MCPToolResult: %v", err)
	}
	if !mcpEditResp.IsError {
		t.Errorf("/edit_file: expected IsError true for empty JSON, got false. Content: %s", mcpEditResp.Content[0].Text)
	}

	// Test /health
	reqHealth, _ := http.NewRequest("GET", "/health", nil) // GET requests typically don't have a body
	rrHealth := httptest.NewRecorder()
	mux.ServeHTTP(rrHealth, reqHealth)
	if rrHealth.Code != http.StatusOK {
		t.Errorf("/health route not registered or not handled correctly (status %d)", rrHealth.Code)
	}
}

// Note: Testing StartServer directly is more of an integration test.
// The core logic is in the handlers, which are tested above.
// If StartServer had more complex logic, it might warrant specific tests,
// potentially by using a mock http.Server or by running the server in a goroutine
// and making actual HTTP calls, then shutting it down.
// For now, its direct testing is omitted as its main job is wiring and calling ListenAndServe.

func TestHTTPHandler_handleReadFile_DisallowUnknownFields(t *testing.T) {
	mockService := &mockFileOperationService{ /* ... */ }
	handler := NewHTTPHandler(mockService, 10)
	server := httptest.NewServer(http.HandlerFunc(handler.handleReadFile))
	defer server.Close()

	reqBody := `{"name": "test.txt", "unknown_field": "some_value"}` // Contains an unknown field
	resp, err := http.Post(server.URL, "application/json", bytes.NewBufferString(reqBody))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusBadRequest { // Expecting bad request due to unknown field
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Errorf("expected status %d, got %d. Body: %s", http.StatusBadRequest, resp.StatusCode, string(bodyBytes))
	}
	var errResp models.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}
	if errResp.Error.Code != errors.CodeParseError {
		t.Errorf("expected error code %d for unknown field, got %d", errors.CodeParseError, errResp.Error.Code)
	}
	if !strings.Contains(errResp.Error.Message, "unknown field") && !strings.Contains(errResp.Error.Data.(map[string]interface{})["details"].(string), "json: unknown field") {
		// The exact message from json.Decoder.DisallowUnknownFields can vary slightly.
		// It might be in Error.Message or Error.Data.details depending on how NewParseError structures it.
		t.Errorf("expected 'unknown field' in error message, got: %s / Data: %+v", errResp.Error.Message, errResp.Error.Data)
	}
}

func TestNewHTTPHandler_MaxRequestSizeConfiguration(t *testing.T) {
	testCases := []struct {
		name         string
		cfgMaxReqMB  int
		expectedSize int64
	}{
		{
			name:         "Typical value 10MB",
			cfgMaxReqMB:  10,
			expectedSize: 10 * 1024 * 1024,
		},
		{
			name:         "Minimum value 1MB",
			cfgMaxReqMB:  1,
			expectedSize: 1 * 1024 * 1024,
		},
		{
			name:         "Maximum value 100MB",
			cfgMaxReqMB:  100,
			expectedSize: 100 * 1024 * 1024,
		},
		{
			name:         "Zero value 0MB", // Test edge case, though practical minimum might be 1
			cfgMaxReqMB:  0,
			expectedSize: 0 * 1024 * 1024,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Pass nil for FileOperationService as it's not relevant for this specific test
			handler := NewHTTPHandler(nil, tc.cfgMaxReqMB)

			if handler.maxReqSize != tc.expectedSize {
				t.Errorf("Expected maxReqSize to be %d bytes, but got %d bytes for %d MB input",
					tc.expectedSize, handler.maxReqSize, tc.cfgMaxReqMB)
			}
		})
	}
}
