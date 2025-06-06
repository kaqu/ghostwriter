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
		// Attempt to parse the ID first to include in parse error responses
		var preParse struct {
			ID      interface{} `json:"id"`
			JSONRPC string      `json:"jsonrpc"`
		}
		json.Unmarshal(lineBytes, &preParse) // Ignore error, ID might be null or missing

		if err := json.Unmarshal(lineBytes, &req); err != nil {
			// Handle JSON parse error (-32700)
			resp := models.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      preParse.ID, // Use pre-parsed ID if available
				Error: &models.JSONRPCError{
					Code:    errors.CodeParseError,
					Message: fmt.Sprintf("Parse error: %v", err),
				},
			}
			h.writeJSONRPCResponse(output, resp)
			continue
		}

		// Basic validation of the JSON-RPC request structure
		if req.JSONRPC != "2.0" {
			resp := models.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &models.JSONRPCError{
					Code:    errors.CodeInvalidRequest,
					Message: "Invalid JSON-RPC version. Must be '2.0'.",
				},
			}
			h.writeJSONRPCResponse(output, resp)
			continue
		}
		if req.Method == "" {
			resp := models.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      req.ID,
				Error: &models.JSONRPCError{
					Code:    errors.CodeInvalidRequest,
					Message: "Method not specified.",
				},
			}
			h.writeJSONRPCResponse(output, resp)
			continue
		}

		mcpResult, jsonrpcErr := h.processor.ProcessRequest(req)

		finalResp := models.JSONRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
		}

		if jsonrpcErr != nil {
			finalResp.Error = jsonrpcErr
		} else {
			finalResp.Result = mcpResult // MCPToolResult is the result
		}
		h.writeJSONRPCResponse(output, finalResp)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading from stdio: %v", err)
		return err
	}

	log.Println("Stdio JSON-RPC handler finished.")
	return nil
}
