package transport

import (
	"encoding/json"
	stdErrors "errors" // Alias for standard errors package
	"encoding/json"
	stdErrors "errors" // Alias for standard errors package
	"file-editor-server/internal/errors"
	"file-editor-server/internal/mcp" // Added for MCPProcessorInterface
	"file-editor-server/internal/models"
	"file-editor-server/internal/service"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
	// "strconv" // Not used yet, can remove if not needed later
)

const (
	defaultReadTimeout  = 60 * time.Second
	defaultWriteTimeout = 60 * time.Second
)

// HTTPHandler handles HTTP requests for file operations.
type HTTPHandler struct {
	service      service.FileOperationService
	mcpProcessor mcp.MCPProcessorInterface // Added MCPProcessor
	readTimeout  time.Duration             // For http.Server
	writeTimeout time.Duration             // For http.Server
	maxReqSize   int64         // Max request body size in bytes
	Server       *http.Server  // Holds the server instance
}

// NewHTTPHandler creates a new HTTPHandler.
func NewHTTPHandler(svc service.FileOperationService, mcpProcessor mcp.MCPProcessorInterface, cfgMaxReqSizeMB int) *HTTPHandler {
	if svc == nil {
		// This should ideally not happen if dependencies are correctly injected.
		// Consider panicking or returning an error if critical dependencies are nil.
		log.Printf("Warning: FileOperationService is nil in NewHTTPHandler")
	}
	if mcpProcessor == nil {
		log.Printf("Warning: MCPProcessorInterface is nil in NewHTTPHandler")
	}
	return &HTTPHandler{
		service:      svc,
		mcpProcessor: mcpProcessor, // Store the MCPProcessor
		readTimeout:  defaultReadTimeout,  // Sensible defaults, can be made configurable
		writeTimeout: defaultWriteTimeout, // Sensible defaults, can be made configurable
		maxReqSize:   int64(cfgMaxReqSizeMB) * 1024 * 1024,
		Server:       &http.Server{}, // Initialize the server field
	}
}

// RegisterRoutes sets up the HTTP routes for the handler.
func (h *HTTPHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/read_file", h.handleReadFile)
	mux.HandleFunc("/edit_file", h.handleEditFile)
	mux.HandleFunc("/health", h.handleHealthCheck)
	mux.HandleFunc("/list_files", h.handleListFiles)
}

// writeJSONResponse is a helper to write JSON data to the response.
func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if data != nil { // Avoid writing null for empty body responses if desired
		if err := json.NewEncoder(w).Encode(data); err != nil {
			// Log error, but response header is already sent.
			log.Printf("Error encoding JSON response: %v", err)
		}
	}
}

// writeJSONErrorResponse is a helper to write a JSON error response.
func writeJSONErrorResponse(w http.ResponseWriter, httpStatusCode int, errorDetail *models.ErrorDetail) {
	if errorDetail == nil {
		// Fallback if a nil error detail is somehow passed
		errorDetail = errors.NewInternalError("An unexpected error occurred and error details were lost.")
		httpStatusCode = http.StatusInternalServerError
	}
	response := models.ErrorResponse{Error: *errorDetail}
	writeJSONResponse(w, httpStatusCode, response)
}

func (h *HTTPHandler) handleHealthCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	writeJSONResponse(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (h *HTTPHandler) handleReadFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errDetail := errors.NewInvalidRequestError(fmt.Sprintf("Method %s not allowed for /read_file. Use POST.", r.Method))
		writeJSONErrorResponse(w, http.StatusMethodNotAllowed, errDetail)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		errDetail := errors.NewInvalidRequestError("Invalid Content-Type header. Must be 'application/json' or 'application/json; charset=utf-8'.")
		writeJSONErrorResponse(w, http.StatusUnsupportedMediaType, errDetail)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxReqSize)
	defer func() {
		_ = r.Body.Close()
	}()

	var req models.ReadFileRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // Enforce strict parsing

	if err := decoder.Decode(&req); err != nil {
		// Determine if it's a size error or parse error
		var jsonSyntaxError *json.SyntaxError
		var jsonUnmarshalTypeError *json.UnmarshalTypeError
		if err.Error() == "http: request body too large" {
			errDetail := errors.NewInvalidRequestError(fmt.Sprintf("Request body exceeds maximum size of %dMB.", h.maxReqSize/(1024*1024)))
			writeJSONErrorResponse(w, http.StatusRequestEntityTooLarge, errDetail)
		} else if stdErrors.As(err, &jsonSyntaxError) { // Use aliased stdErrors
			msg := fmt.Sprintf("Invalid JSON syntax at offset %d: %s", jsonSyntaxError.Offset, jsonSyntaxError.Error())
			errDetail := errors.NewParseError(msg)
			writeJSONErrorResponse(w, http.StatusBadRequest, errDetail)
		} else if stdErrors.As(err, &jsonUnmarshalTypeError) { // Use aliased stdErrors
			msg := fmt.Sprintf("Invalid JSON type for field '%s'. Expected '%s' but got '%s' at offset %d.", jsonUnmarshalTypeError.Field, jsonUnmarshalTypeError.Type, jsonUnmarshalTypeError.Value, jsonUnmarshalTypeError.Offset)
			errDetail := errors.NewParseError(msg)
			writeJSONErrorResponse(w, http.StatusBadRequest, errDetail)
		} else {
			errDetail := errors.NewParseError(fmt.Sprintf("Failed to decode request body: %v", err))
			writeJSONErrorResponse(w, http.StatusBadRequest, errDetail)
		}
		return
	}

	mcpResult, err := h.mcpProcessor.ExecuteTool("read_file", req)
	if err != nil {
		// This error is from ExecuteTool itself, likely an argument type mismatch
		// or unknown tool, which is an internal server error.
		errDetail := errors.NewInternalError(fmt.Sprintf("Error executing tool 'read_file': %v", err))
		writeJSONErrorResponse(w, http.StatusInternalServerError, errDetail)
		return
	}

	// mcpResult already contains the formatted text and IsError flag
	writeJSONResponse(w, http.StatusOK, mcpResult) // Adhering to MCP spec: 200 OK with IsError flag in result
}

// Redundant formatting functions are removed as their functionality is now centralized in mcp/processor.go.
// - formatHTTPListFilesResult
// - formatHTTPReadFileResult
// - formatHTTPEditFileResult
// - formatHTTPToolError

func (h *HTTPHandler) handleEditFile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errDetail := errors.NewInvalidRequestError(fmt.Sprintf("Method %s not allowed for /edit_file. Use POST.", r.Method))
		writeJSONErrorResponse(w, http.StatusMethodNotAllowed, errDetail)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") {
		errDetail := errors.NewInvalidRequestError("Invalid Content-Type header. Must be 'application/json' or 'application/json; charset=utf-8'.")
		writeJSONErrorResponse(w, http.StatusUnsupportedMediaType, errDetail)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxReqSize)
	defer func() {
		_ = r.Body.Close()
	}()

	var req models.EditFileRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		var jsonSyntaxError *json.SyntaxError
		var jsonUnmarshalTypeError *json.UnmarshalTypeError
		if err.Error() == "http: request body too large" {
			errDetail := errors.NewInvalidRequestError(fmt.Sprintf("Request body exceeds maximum size of %dMB.", h.maxReqSize/(1024*1024)))
			writeJSONErrorResponse(w, http.StatusRequestEntityTooLarge, errDetail)
		} else if stdErrors.As(err, &jsonSyntaxError) { // Use aliased stdErrors
			msg := fmt.Sprintf("Invalid JSON syntax at offset %d: %s", jsonSyntaxError.Offset, jsonSyntaxError.Error())
			errDetail := errors.NewParseError(msg)
			writeJSONErrorResponse(w, http.StatusBadRequest, errDetail)
		} else if stdErrors.As(err, &jsonUnmarshalTypeError) { // Use aliased stdErrors
			msg := fmt.Sprintf("Invalid JSON type for field '%s'. Expected '%s' but got '%s' at offset %d.", jsonUnmarshalTypeError.Field, jsonUnmarshalTypeError.Type, jsonUnmarshalTypeError.Value, jsonUnmarshalTypeError.Offset)
			errDetail := errors.NewParseError(msg)
			writeJSONErrorResponse(w, http.StatusBadRequest, errDetail)
		} else {
			errDetail := errors.NewParseError(fmt.Sprintf("Failed to decode request body: %v", err))
			writeJSONErrorResponse(w, http.StatusBadRequest, errDetail)
		}
		return
	}

	filename, linesModified, newTotalLines, fileCreated, serviceErr := h.service.EditFile(req)
	if serviceErr != nil {
		errorText := formatHTTPToolError(serviceErr)
		resp := models.MCPToolResult{
			Content: []models.MCPToolContent{{Type: "text", Text: errorText}},
			IsError: true,
		}
		writeJSONResponse(w, http.StatusOK, resp) // Adhering to MCP spec: 200 OK with IsError:true
		return
	}

	successText := formatHTTPEditFileResult(filename, linesModified, newTotalLines, fileCreated)
	resp := models.MCPToolResult{
		Content: []models.MCPToolContent{{Type: "text", Text: successText}},
		IsError: false,
	}
	writeJSONResponse(w, http.StatusOK, resp)
}

// StartServer initializes and starts the HTTP server.
// The timeouts passed here will override the defaults set in NewHTTPHandler for the http.Server instance.
func (h *HTTPHandler) StartServer(port int, readTimeoutSec int, writeTimeoutSec int) error {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Use timeouts from parameters if provided, otherwise use handler's defaults (which are also defaults)
	actualReadTimeout := h.readTimeout
	if readTimeoutSec > 0 {
		actualReadTimeout = time.Duration(readTimeoutSec) * time.Second
	}
	actualWriteTimeout := h.writeTimeout
	if writeTimeoutSec > 0 {
		actualWriteTimeout = time.Duration(writeTimeoutSec) * time.Second
	}

	// Configure the server instance stored in the handler
	h.Server.Addr = fmt.Sprintf(":%d", port)
	h.Server.Handler = mux
	h.Server.ReadTimeout = actualReadTimeout
	h.Server.WriteTimeout = actualWriteTimeout
	// IdleTimeout can also be set if desired, e.g., h.Server.IdleTimeout = 120 * time.Second

	log.Printf("HTTP server starting on port %d (ReadTimeout: %s, WriteTimeout: %s)", port, actualReadTimeout, actualWriteTimeout)
	// ListenAndServe always returns a non-nil error.
	// If it's http.ErrServerClosed, it's a graceful shutdown, not necessarily a "failure" to log as fatal.
	err := h.Server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Printf("HTTP server ListenAndServe error: %v", err)
		return err
	}
	log.Printf("HTTP server on port %d shut down.", port)
	return nil // Or return http.ErrServerClosed if caller needs to know
}

func (h *HTTPHandler) handleListFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errDetail := errors.NewInvalidRequestError(fmt.Sprintf("Method %s not allowed for /list_files. Use POST.", r.Method))
		writeJSONErrorResponse(w, http.StatusMethodNotAllowed, errDetail)
		return
	}

	contentType := r.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "application/json") && contentType != "" { // Allow empty body with no content type, or require application/json
		// The spec says "Content-Type: application/json (REQUIRED for all requests and responses)"
		// An empty request body for list_files might not strictly need it, but spec is strict.
		// Let's enforce it.
		errDetail := errors.NewInvalidRequestError("Invalid Content-Type header. Must be 'application/json' or 'application/json; charset=utf-8'.")
		writeJSONErrorResponse(w, http.StatusUnsupportedMediaType, errDetail)
		return
	}

	// Request body for list_files is an empty JSON object {} as per spec 3.1.1
	// We can try to decode it into an empty struct to validate it's indeed an empty object.
	var req models.ListFilesRequest
	// Only decode if there's a body. Some clients might send Content-Length: 0 for empty POST.
	if r.ContentLength > 0 {
		// MaxBytesReader to prevent large empty bodies if someone sends one.
		r.Body = http.MaxBytesReader(w, r.Body, 1024) // Limit empty body to 1KB
		defer func() {
			_ = r.Body.Close()
		}()

		decoder := json.NewDecoder(r.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&req); err != nil && err != io.EOF { // EOF is fine for empty or {} body
			// Handle cases where body is not an empty JSON object e.g. `[]` or `"string"`
			if _, ok := err.(*json.SyntaxError); ok || strings.Contains(err.Error(), "cannot unmarshal") {
				errDetail := errors.NewParseError(fmt.Sprintf("Request body must be an empty JSON object {} or empty: %v", err))
				writeJSONErrorResponse(w, http.StatusBadRequest, errDetail)
				return
			}
			// For other decode errors
			errDetail := errors.NewParseError(fmt.Sprintf("Failed to decode request body for list_files: %v", err))
			writeJSONErrorResponse(w, http.StatusBadRequest, errDetail)
			return
		}
	}

	files, serviceErr := h.service.ListFiles(req)
	if serviceErr != nil {
		// MCPProcessor.ExecuteTool will format this into an MCPToolResult
		// So, this block should ideally not be reached if ExecuteTool is called.
		// However, if we directly call service and then plan to use mcpProcessor for formatting ONLY,
		// this would be different. But the aim is to call ExecuteTool.

		// Assuming ExecuteTool is used, this direct service call and subsequent formatting is replaced.
		// For now, let's leave the old path commented out or remove it once fully refactored.
		// errorText := formatHTTPToolError(serviceErr) // This would be p.formatToolError(serviceErr)
		// resp := models.MCPToolResult{
		// 	Content: []models.MCPToolContent{{Type: "text", Text: errorText}},
		// 	IsError: true,
		// }
		// writeJSONResponse(w, http.StatusOK, resp)
		// return
	}

	// Format the success response using the new helper
	// successText := formatHTTPListFilesResult(files) // This would be p.formatListFilesResult(files)
	// resp := models.MCPToolResult{
	// 	Content: []models.MCPToolContent{{Type: "text", Text: successText}},
	// 	IsError: false,
	// }
	// writeJSONResponse(w, http.StatusOK, resp)

	// New path using mcpProcessor.ExecuteTool for list_files
	mcpResult, err := h.mcpProcessor.ExecuteTool("list_files", req)
	if err != nil {
		errDetail := errors.NewInternalError(fmt.Sprintf("Error executing tool 'list_files': %v", err))
		writeJSONErrorResponse(w, http.StatusInternalServerError, errDetail)
		return
	}
	writeJSONResponse(w, http.StatusOK, mcpResult)
}
