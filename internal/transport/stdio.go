package transport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"file-editor-server/internal/errors"
	"file-editor-server/internal/mcp" // Added mcp import
	"file-editor-server/internal/models"
	// "file-editor-server/internal/service" // Service no longer directly used by handler
	"fmt"
	"io"
	"log"
	// "time" // Time might not be needed if MCP processor handles all relevant timestamps
)

// StdioHandler handles JSON-RPC communication over standard input/output.
type StdioHandler struct {
	processor mcp.MCPProcessorInterface // Use the interface
}

// NewStdioHandler creates a new StdioHandler.
func NewStdioHandler(processor mcp.MCPProcessorInterface) *StdioHandler { // Accept the interface
	if processor == nil {
		log.Fatal("MCPProcessorInterface cannot be nil in NewStdioHandler") // Fatal, as it's critical
	}
	return &StdioHandler{
		processor: processor,
	}
}

func (h *StdioHandler) writeJSONRPCResponse(writer io.Writer, response models.JSONRPCResponse) {
	responseBytes, err := json.Marshal(response)
	if err != nil {
		// This is a server-side error during response marshaling.
		// Create a fallback error response.
		log.Printf("Error marshaling JSON-RPC response: %v. Original ID: %v", err, response.ID)
		fallbackError := errors.NewInternalError("Server error: failed to marshal response.")
		// Use the ID from the original request if available
		errorResp := models.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      response.ID, // Preserve ID if possible
			Error:   errors.ToJSONRPCError(fallbackError),
		}
		responseBytes, _ = json.Marshal(errorResp) // Marshal again, should not fail for this simple struct
	}

	if _, err := fmt.Fprintln(writer, string(responseBytes)); err != nil {
		log.Printf("Error writing JSON-RPC response to output: %v", err)
		// If writing to output fails, not much else can be done for this request.
	}
}

// Start begins processing JSON-RPC requests from input and writing responses to output.
func (h *StdioHandler) Start(input io.Reader, output io.Writer) error {
	log.Println("Starting stdio JSON-RPC handler.")
	scanner := bufio.NewScanner(input)

	for scanner.Scan() {
		lineBytes := scanner.Bytes()
		if len(bytes.TrimSpace(lineBytes)) == 0 { // Skip empty lines
			continue
		}

		var req models.JSONRPCRequest
		response := models.JSONRPCResponse{JSONRPC: "2.0"} // Initialize response structure

		if err := json.Unmarshal(lineBytes, &req); err != nil {
			// Corrected version within the error block for "if err := json.Unmarshal(lineBytes, &req); err != nil { ... }":
			var idForErrorResponse interface{}
			// Attempt to extract ID specifically for this error response.
			// This is a best-effort attempt, so we can ignore the error from this particular Unmarshal.
			var idExtractor struct {
				ID interface{} `json:"id"`
			}
			_ = json.Unmarshal(lineBytes, &idExtractor)
			idForErrorResponse = idExtractor.ID

			response.ID = idForErrorResponse // Use the safely extracted ID (or nil if not found)
			response.Error = &models.JSONRPCError{
				Code:    models.ErrCodeParseError, // Should be -32700
				Message: fmt.Sprintf("Parse error: %v", err),
			}
			h.writeJSONRPCResponse(output, response)
			continue
		}
		response.ID = req.ID // Set ID for valid requests

		// Basic validation of the JSON-RPC request structure
		if req.JSONRPC != "2.0" {
			// Removed local 'resp' declaration, assign to outer 'response'
			response.Error = &models.JSONRPCError{
				Code:    errors.CodeInvalidRequest,
				Message: "Invalid JSON-RPC version. Must be '2.0'.",
			}
			h.writeJSONRPCResponse(output, response) // Use the initialized response
			continue
		}
		if req.Method == "" {
			response.Error = &models.JSONRPCError{ // Use the initialized response
				Code:    errors.CodeInvalidRequest,
				Message: "Method not specified.",
			}
			h.writeJSONRPCResponse(output, response) // Use the initialized response
			continue
		}

		mcpResult, jsonrpcErr := h.processor.ProcessRequest(req)

		if jsonrpcErr != nil {
			response.Error = jsonrpcErr
		} else {
			response.Result = mcpResult // MCPToolResult is the result
		}
		h.writeJSONRPCResponse(output, response) // Use the initialized response
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading from stdio: %v", err)
		return err
	}

	log.Println("Stdio JSON-RPC handler finished.")
	return nil
}
