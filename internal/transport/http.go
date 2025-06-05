package transport

import (
	"encoding/json"
	stdErrors "errors" // Alias for standard errors package
	"file-editor-server/internal/errors"
	"file-editor-server/internal/models"
	"file-editor-server/internal/service"
	"fmt"
	"log"
	"net/http"
	"strings" // Added for Content-Type check
	"time"
	// "strconv" // Not used yet, can remove if not needed later
)

const (
	defaultReadTimeout  = 60 * time.Second
	defaultWriteTimeout = 60 * time.Second
	// Max request size as per spec 4.2.1 "Maximum request size MUST be enforced at 50MB"
	defaultMaxRequestSizeMB = 50
)

// HTTPHandler handles HTTP requests for file operations.
type HTTPHandler struct {
	service      service.FileOperationService
	readTimeout  time.Duration // For http.Server
	writeTimeout time.Duration // For http.Server
	maxReqSize   int64         // Max request body size in bytes
	Server       *http.Server  // Holds the server instance
}

// NewHTTPHandler creates a new HTTPHandler.
// cfgMaxReqSizeMB is currently ignored in favor of a default 50MB as per spec.
// If a separate config for HTTP request size is added, it can be used.
func NewHTTPHandler(svc service.FileOperationService, _ /*cfgMaxReqSizeMB*/ int) *HTTPHandler {
	if svc == nil {
		// This should ideally not happen if dependencies are correctly injected.
		// Consider panicking or returning an error if critical dependencies are nil.
		log.Printf("Warning: FileOperationService is nil in NewHTTPHandler")
	}
	return &HTTPHandler{
		service:      svc,
		readTimeout:  defaultReadTimeout,  // Sensible defaults, can be made configurable
		writeTimeout: defaultWriteTimeout, // Sensible defaults, can be made configurable
		maxReqSize:   int64(defaultMaxRequestSizeMB) * 1024 * 1024,
		Server:       &http.Server{}, // Initialize the server field
	}
}

// RegisterRoutes sets up the HTTP routes for the handler.
func (h *HTTPHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/read_file", h.handleReadFile)
	mux.HandleFunc("/edit_file", h.handleEditFile)
	mux.HandleFunc("/health", h.handleHealthCheck)
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
	defer r.Body.Close()

	var req models.ReadFileRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields() // Enforce strict parsing

	if err := decoder.Decode(&req); err != nil {
		// Determine if it's a size error or parse error
		var jsonSyntaxError *json.SyntaxError
		var jsonUnmarshalTypeError *json.UnmarshalTypeError
		if err.Error() == "http: request body too large" {
			errDetail := errors.NewInvalidRequestError(fmt.Sprintf("Request body exceeds maximum size of %dMB.", defaultMaxRequestSizeMB))
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

	serviceResp, serviceErr := h.service.ReadFile(req)
	if serviceErr != nil {
		// Map internal error to HTTP status
		// The serviceErr is already *models.ErrorDetail
		httpStatus := errors.MapErrorToHTTPStatus(serviceErr.Code, serviceErr)
		writeJSONErrorResponse(w, httpStatus, serviceErr)
		return
	}

	writeJSONResponse(w, http.StatusOK, serviceResp)
}

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
	defer r.Body.Close()

	var req models.EditFileRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		var jsonSyntaxError *json.SyntaxError
		var jsonUnmarshalTypeError *json.UnmarshalTypeError
		if err.Error() == "http: request body too large" {
			errDetail := errors.NewInvalidRequestError(fmt.Sprintf("Request body exceeds maximum size of %dMB.", defaultMaxRequestSizeMB))
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

	serviceResp, serviceErr := h.service.EditFile(req)
	if serviceErr != nil {
		httpStatus := errors.MapErrorToHTTPStatus(serviceErr.Code, serviceErr)
		writeJSONErrorResponse(w, httpStatus, serviceErr)
		return
	}

	writeJSONResponse(w, http.StatusOK, serviceResp)
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
