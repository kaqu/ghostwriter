package transport

import (
	"bufio"
	"bytes" // Added bytes import
	"encoding/json"
	"file-editor-server/internal/errors"
	"file-editor-server/internal/models"
	"file-editor-server/internal/service"
	"fmt"
	"io"
	"log"
	// "os" // Removed
	// "strings" // Removed
	"time"
)

// StdioHandler handles JSON-RPC communication over standard input/output.
type StdioHandler struct {
	service service.FileOperationService
}

// NewStdioHandler creates a new StdioHandler.
func NewStdioHandler(svc service.FileOperationService) *StdioHandler {
	if svc == nil {
		// This should ideally not happen.
		log.Println("Warning: FileOperationService is nil in NewStdioHandler")
	}
	return &StdioHandler{
		service: svc,
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

		var jsonReq models.JSONRPCRequest
		var jsonResp models.JSONRPCResponse // Pre-declare to ensure ID is captured even for early errors

		if err := json.Unmarshal(lineBytes, &jsonReq); err != nil {
			errDetail := errors.NewParseError(fmt.Sprintf("Invalid JSON received: %v", err))
			jsonResp = models.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      nil, // ID might not be parsable from invalid JSON
				Error:   errors.ToJSONRPCError(errDetail),
			}
			h.writeJSONRPCResponse(output, jsonResp)
			continue
		}

		// Set ID for all responses from this point
		jsonResp.ID = jsonReq.ID
		jsonResp.JSONRPC = "2.0"

		if jsonReq.JSONRPC != "2.0" {
			errDetail := errors.NewInvalidRequestError("Invalid JSON-RPC version. Must be '2.0'.")
			jsonResp.Error = errors.ToJSONRPCError(errDetail)
			h.writeJSONRPCResponse(output, jsonResp)
			continue
		}
		if jsonReq.Method == "" {
			errDetail := errors.NewInvalidRequestError("Method not specified.")
			jsonResp.Error = errors.ToJSONRPCError(errDetail)
			h.writeJSONRPCResponse(output, jsonResp)
			continue
		}

		// var serviceReqData interface{} // Removed as it was not strictly needed
		var serviceRespData interface{}
		var serviceErr *models.ErrorDetail

		switch jsonReq.Method {
		case "read_file":
			var params models.ReadFileRequest
			if err := json.Unmarshal(jsonReq.Params, &params); err != nil {
				serviceErr = errors.NewInvalidParamsError(fmt.Sprintf("Invalid params for read_file: %v", err), nil)
			} else {
				// For JSON-RPC, pass context to service if it expects it, or enrich error later.
				// current service.ReadFile doesn't take extra context for filename/op in error detail data.
				serviceRespData, serviceErr = h.service.ReadFile(params)
			}
		case "edit_file":
			var params models.EditFileRequest
			if err := json.Unmarshal(jsonReq.Params, &params); err != nil {
				serviceErr = errors.NewInvalidParamsError(fmt.Sprintf("Invalid params for edit_file: %v", err), nil)
			} else {
				serviceRespData, serviceErr = h.service.EditFile(params)
			}
		case "list_files":
			var params models.ListFilesRequest
			// Params field for list_files should be empty or an empty object.
			// Unmarshal will succeed if jsonReq.Params is null or an empty JSON object "{}".
			if len(jsonReq.Params) > 0 && string(jsonReq.Params) != "null" && string(jsonReq.Params) != "{}" {
				// Check if it's a non-empty object or array
				var temp interface{}
				if err := json.Unmarshal(jsonReq.Params, &temp); err == nil {
					// if it's some other valid JSON that is not an empty object, it's an error
					if _, isMap := temp.(map[string]interface{}); !isMap || len(temp.(map[string]interface{})) > 0 {
						serviceErr = errors.NewInvalidParamsError("Parameters for list_files must be an empty JSON object or null.", nil)
					}
				} else { // Not valid JSON at all
					serviceErr = errors.NewInvalidParamsError(fmt.Sprintf("Invalid params for list_files: %v", err), nil)
				}
			}
			// If no error from param check, proceed (params is an empty ListFilesRequest)

			if serviceErr == nil {
				serviceRespData, serviceErr = h.service.ListFiles(params)
				// Removed dummy response:
				// serviceRespData = models.ListFilesResponse{Files: []models.FileInfo{}, TotalCount: 0, Directory: "dummy/path"}
				// serviceErr = nil
			}
		default:
			serviceErr = errors.NewMethodNotFoundError(jsonReq.Method)
		}

		if serviceErr != nil {
			// Enrich the JSONRPCError.Data field here if possible
			// The current ToJSONRPCError tries to extract from serviceErr.Data
			// We can create a new JSONRPCErrorData and pass it if more context is needed.
			rpcError := errors.ToJSONRPCError(serviceErr)
			if rpcError.Data == nil && serviceErr.Data != nil { // If ToJSONRPCError didn't populate Data well
				rpcError.Data = &models.JSONRPCErrorData{} // Ensure data is not nil
			}
			if rpcError.Data != nil { // Add more context if available
				rpcError.Data.Operation = jsonReq.Method
				// Filename might be in serviceErr.Data if service put it there
				if dataMap, ok := serviceErr.Data.(map[string]interface{}); ok {
					if fn, fnOk := dataMap["filename"].(string); fnOk {
						rpcError.Data.Filename = fn
					}
				}
				if rpcError.Data.Timestamp == "" {
					rpcError.Data.Timestamp = time.Now().UTC().Format(time.RFC3339)
				}
			}
			jsonResp.Error = rpcError
		} else {
			jsonResp.Result = serviceRespData
		}
		h.writeJSONRPCResponse(output, jsonResp)
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading from stdio: %v", err)
		return err
	}

	log.Println("Stdio JSON-RPC handler finished.")
	return nil
}
