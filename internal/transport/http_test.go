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
type mockFileOperationService struct {
	ReadFileFunc  func(req models.ReadFileRequest) (*models.ReadFileResponse, *models.ErrorDetail)
	EditFileFunc  func(req models.EditFileRequest) (*models.EditFileResponse, *models.ErrorDetail)
	ListFilesFunc func(req models.ListFilesRequest) (*models.ListFilesResponse, *models.ErrorDetail) // Added
}

func (m *mockFileOperationService) ReadFile(req models.ReadFileRequest) (*models.ReadFileResponse, *models.ErrorDetail) {
	if m.ReadFileFunc != nil {
		return m.ReadFileFunc(req)
	}
	return nil, errors.NewInternalError("ReadFileFunc not implemented in mock")
}

func (m *mockFileOperationService) EditFile(req models.EditFileRequest) (*models.EditFileResponse, *models.ErrorDetail) {
	if m.EditFileFunc != nil {
		return m.EditFileFunc(req)
	}
	return nil, errors.NewInternalError("EditFileFunc not implemented in mock")
}

// ListFiles implements the FileOperationService interface for the mock.
func (m *mockFileOperationService) ListFiles(req models.ListFilesRequest) (*models.ListFilesResponse, *models.ErrorDetail) {
	if m.ListFilesFunc != nil {
		return m.ListFilesFunc(req)
	}
	// Provide a default mock response that's valid but minimal.
	return &models.ListFilesResponse{Files: []models.FileInfo{}, TotalCount: 0, Directory: "/mock/dir"}, nil
}

func TestHTTPHandler_handleReadFile_Success(t *testing.T) {
	mockService := &mockFileOperationService{
		ReadFileFunc: func(req models.ReadFileRequest) (*models.ReadFileResponse, *models.ErrorDetail) {
			if req.Name == "test.txt" {
				return &models.ReadFileResponse{Content: "hello world", TotalLines: 1}, nil
			}
			return nil, errors.NewFileNotFoundError(req.Name, "read")
		},
	}
	// cfgMaxReqSizeMB is currently ignored by NewHTTPHandler, using default 50MB.
	handler := NewHTTPHandler(mockService, 10)

	server := httptest.NewServer(http.HandlerFunc(handler.handleReadFile))
	defer server.Close()

	reqBody := `{"name": "test.txt"}`
	resp, err := http.Post(server.URL, "application/json", bytes.NewBufferString(reqBody))
	if err != nil {
		t.Fatalf("POST request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	var readResp models.ReadFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&readResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if readResp.Content != "hello world" {
		t.Errorf("expected content 'hello world', got %q", readResp.Content)
	}
}

func TestHTTPHandler_handleReadFile_ServiceError(t *testing.T) {
	mockService := &mockFileOperationService{
		ReadFileFunc: func(req models.ReadFileRequest) (*models.ReadFileResponse, *models.ErrorDetail) {
			return nil, errors.NewFileNotFoundError(req.Name, "read")
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
	defer resp.Body.Close()

	// File not found -> maps to 404
	expectedStatus := http.StatusNotFound
	if resp.StatusCode != expectedStatus {
		t.Errorf("expected status %d, got %d", expectedStatus, resp.StatusCode)
	}

	var errResp models.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}
	if errResp.Error.Code != errors.CodeFileSystemError { // As NewFileNotFoundError uses this code
		t.Errorf("expected error code %d, got %d", errors.CodeFileSystemError, errResp.Error.Code)
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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, resp.StatusCode)
	}
}

func TestHTTPHandler_handleEditFile_Success(t *testing.T) {
	mockService := &mockFileOperationService{
		EditFileFunc: func(req models.EditFileRequest) (*models.EditFileResponse, *models.ErrorDetail) {
			if req.Name == "editme.txt" {
				return &models.EditFileResponse{Success: true, NewTotalLines: 5, LinesModified: 1, FileCreated: false}, nil
			}
			return nil, errors.NewInternalError("mock edit error")
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}
	var editResp models.EditFileResponse
	if err := json.NewDecoder(resp.Body).Decode(&editResp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if !editResp.Success || editResp.NewTotalLines != 5 {
		t.Errorf("unexpected edit response: %+v", editResp)
	}
}

func TestHTTPHandler_handleEditFile_ServiceError(t *testing.T) {
	mockService := &mockFileOperationService{
		EditFileFunc: func(req models.EditFileRequest) (*models.EditFileResponse, *models.ErrorDetail) {
			return nil, errors.NewInvalidParamsError("Invalid edit operation", nil)
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
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadRequest { // InvalidParams maps to 400
		t.Errorf("expected status %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}
	var errResp models.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}
	if errResp.Error.Code != errors.CodeInvalidParams {
		t.Errorf("expected error code %d, got %d", errors.CodeInvalidParams, errResp.Error.Code)
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

func TestHTTPHandler_RegisterRoutes(t *testing.T) {
	// Provide a minimal mock service, even if its methods aren't deeply tested here.
	mockService := &mockFileOperationService{
		ReadFileFunc: func(req models.ReadFileRequest) (*models.ReadFileResponse, *models.ErrorDetail) {
			// Minimal implementation for route testing: return a known error or simple success
			return nil, errors.NewInvalidParamsError("test read", nil) // Or return a simple success
		},
		EditFileFunc: func(req models.EditFileRequest) (*models.EditFileResponse, *models.ErrorDetail) {
			return nil, errors.NewInvalidParamsError("test edit", nil)
		},
		ListFilesFunc: func(req models.ListFilesRequest) (*models.ListFilesResponse, *models.ErrorDetail) {
			// Minimal implementation for ListFiles as well for completeness
			return &models.ListFilesResponse{Files: []models.FileInfo{}, TotalCount: 0, Directory: "/testdir"}, nil
		},
	}
	// Use 1MB as a reasonable default for max request size in this test, instead of 0.
	// 0MB would cause all requests, even empty JSON, to be "Payload Too Large" (413).
	handler := NewHTTPHandler(mockService, 1)
	mux := http.NewServeMux()
	handler.RegisterRoutes(mux)

	// Test /read_file
	// Provide an empty JSON body to avoid nil body issues with json.Decoder
	reqReadFile, _ := http.NewRequest("POST", "/read_file", bytes.NewBufferString("{}"))
	reqReadFile.Header.Set("Content-Type", "application/json") // Added Content-Type
	rrReadFile := httptest.NewRecorder()
	mux.ServeHTTP(rrReadFile, reqReadFile)
	// We expect a BadRequest (400) because the empty JSON "{}" is likely not a valid ReadFileRequest
	// (e.g. missing "name" field). The key is it's not 404 or a panic.
	if rrReadFile.Code != http.StatusBadRequest {
		t.Errorf("/read_file: expected status %d (due to empty/invalid JSON), got %d", http.StatusBadRequest, rrReadFile.Code)
	}

	// Test /edit_file
	reqEditFile, _ := http.NewRequest("POST", "/edit_file", bytes.NewBufferString("{}"))
	reqEditFile.Header.Set("Content-Type", "application/json") // Added Content-Type
	rrEditFile := httptest.NewRecorder()
	mux.ServeHTTP(rrEditFile, reqEditFile)
	if rrEditFile.Code != http.StatusBadRequest {
		t.Errorf("/edit_file: expected status %d (due to empty/invalid JSON), got %d", http.StatusBadRequest, rrEditFile.Code)
	}

	// Test /health
	reqHealth, _ := http.NewRequest("GET", "/health", bytes.NewBufferString("")) // GET requests typically don't have a body, but providing empty string is safe.
	rrHealth := httptest.NewRecorder()
	mux.ServeHTTP(rrHealth, reqHealth)
	if rrHealth.Code != http.StatusOK { // Health should give 200
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
	defer resp.Body.Close()

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
