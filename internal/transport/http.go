package transport

import (
	"encoding/json"
	stdErrors "errors"
	"file-editor-server/internal/errors"
	"file-editor-server/internal/mcp"
	"file-editor-server/internal/models"
	"fmt"
	"log"
	"net/http"
	"time"
)

const (
	defaultReadTimeout  = 60 * time.Second
	defaultWriteTimeout = 60 * time.Second
)

// HTTPHandler exposes a single MCP endpoint over HTTP.
type HTTPHandler struct {
	mcpProcessor mcp.MCPProcessorInterface
	readTimeout  time.Duration
	writeTimeout time.Duration
	maxReqSize   int64
	Server       *http.Server
}

// NewHTTPHandler initializes the handler.
func NewHTTPHandler(mcpProcessor mcp.MCPProcessorInterface, cfgMaxReqSizeMB int) *HTTPHandler {
	if mcpProcessor == nil {
		log.Printf("Warning: MCPProcessorInterface is nil in NewHTTPHandler")
	}
	return &HTTPHandler{
		mcpProcessor: mcpProcessor,
		readTimeout:  defaultReadTimeout,
		writeTimeout: defaultWriteTimeout,
		maxReqSize:   int64(cfgMaxReqSizeMB) * 1024 * 1024,
		Server:       &http.Server{},
	}
}

func (h *HTTPHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/mcp", h.handleMCP)
}

func writeJSONResponse(w http.ResponseWriter, statusCode int, data interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(statusCode)
	if data != nil {
		_ = json.NewEncoder(w).Encode(data)
	}
}

func writeJSONErrorResponse(w http.ResponseWriter, statusCode int, detail *models.ErrorDetail) {
	if detail == nil {
		detail = errors.NewInternalError("An unexpected error occurred and error details were lost.")
		statusCode = http.StatusInternalServerError
	}
	writeJSONResponse(w, statusCode, models.ErrorResponse{Error: *detail})
}

// handleMCP processes a single JSON-RPC request via HTTP.
func (h *HTTPHandler) handleMCP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		errDetail := errors.NewInvalidRequestError("Method not allowed; use POST")
		writeJSONErrorResponse(w, http.StatusMethodNotAllowed, errDetail)
		return
	}
	if ct := r.Header.Get("Content-Type"); ct != "" && ct != "application/json" && ct != "application/json; charset=utf-8" {
		errDetail := errors.NewInvalidRequestError("Invalid Content-Type header. Must be 'application/json'.")
		writeJSONErrorResponse(w, http.StatusUnsupportedMediaType, errDetail)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, h.maxReqSize)
	defer func() { _ = r.Body.Close() }()

	var req models.JSONRPCRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		var syntax *json.SyntaxError
		if err.Error() == "http: request body too large" {
			errDetail := errors.NewInvalidRequestError("Request body too large")
			writeJSONErrorResponse(w, http.StatusRequestEntityTooLarge, errDetail)
			return
		} else if stdErrors.As(err, &syntax) {
			errDetail := errors.NewParseError(fmt.Sprintf("Invalid JSON at offset %d", syntax.Offset))
			writeJSONErrorResponse(w, http.StatusBadRequest, errDetail)
			return
		}
		errDetail := errors.NewParseError("Failed to decode request body")
		writeJSONErrorResponse(w, http.StatusBadRequest, errDetail)
		return
	}

	mcpRes, jsonErr := h.mcpProcessor.ProcessRequest(req)
	resp := models.JSONRPCResponse{JSONRPC: "2.0", ID: req.ID}
	if jsonErr != nil {
		resp.Error = jsonErr
	} else {
		resp.Result = mcpRes
	}
	writeJSONResponse(w, http.StatusOK, resp)
}

// StartServer starts the HTTP server on the given port.
func (h *HTTPHandler) StartServer(port int, readTimeoutSec int, writeTimeoutSec int) error {
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	rt := h.readTimeout
	if readTimeoutSec > 0 {
		rt = time.Duration(readTimeoutSec) * time.Second
	}
	wt := h.writeTimeout
	if writeTimeoutSec > 0 {
		wt = time.Duration(writeTimeoutSec) * time.Second
	}

	h.Server.Addr = fmt.Sprintf(":%d", port)
	h.Server.Handler = mux
	h.Server.ReadTimeout = rt
	h.Server.WriteTimeout = wt

	log.Printf("HTTP server starting on port %d", port)
	err := h.Server.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Printf("HTTP server error: %v", err)
		return err
	}
	return nil
}
