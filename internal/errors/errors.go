package errors

import (
	"fmt"
	"net/http"
	"time"

	"file-editor-server/internal/models"
)

// JSON-RPC Error Codes (as per JSON-RPC 2.0 Specification)
const (
	CodeParseError     = -32700 // Invalid JSON was received by the server. An error occurred on the server while parsing the JSON text.
	CodeInvalidRequest = -32600 // The JSON sent is not a valid Request object.
	CodeMethodNotFound = -32601 // The method does not exist / is not available.
	CodeInvalidParams  = -32602 // Invalid method parameter(s).
	CodeInternalError  = -32603 // Internal JSON-RPC error.
)

// Application Specific Error Codes
const (
	// CodeFileSystemError is a generic code for file system related issues.
	// Specific issues like file not found, permission denied will use this code
	// but provide more details in the message and data.
	CodeFileSystemError = -32001

	// CodeOperationLockFailed indicates that an operation could not proceed because a lock on the resource could not be acquired.
	CodeOperationLockFailed = -32002 // Example custom application error

	// CodeFileTooLarge indicates the file exceeds the configured size limit.
	CodeFileTooLarge = -32003
)

// --- Helper functions to create models.ErrorDetail ---

// NewErrorDetail creates a new ErrorDetail.
func NewErrorDetail(code int, message string, data interface{}) *models.ErrorDetail {
	return &models.ErrorDetail{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// --- Specific Error Creator Functions returning *models.ErrorDetail ---

// NewParseError creates an ErrorDetail for JSON parsing errors.
// JSON-RPC: -32700
func NewParseError(details string) *models.ErrorDetail {
	return NewErrorDetail(CodeParseError, "Parse error", map[string]string{"details": details})
}

// NewInvalidRequestError creates an ErrorDetail for invalid JSON-RPC Request objects.
// JSON-RPC: -32600
func NewInvalidRequestError(details string) *models.ErrorDetail {
	return NewErrorDetail(CodeInvalidRequest, "Invalid Request", map[string]string{"details": details})
}

// NewMethodNotFoundError creates an ErrorDetail when a JSON-RPC method is not found.
// JSON-RPC: -32601
func NewMethodNotFoundError(methodName string) *models.ErrorDetail {
	return NewErrorDetail(CodeMethodNotFound, "Method not found", map[string]string{"method": methodName})
}

// NewInvalidParamsError creates an ErrorDetail for invalid method parameters.
// JSON-RPC: -32602
// paramIssues can contain specific details about which parameters were invalid.
func NewInvalidParamsError(summaryMessage string, paramIssues map[string]interface{}) *models.ErrorDetail {
	// The main message for the error.
	finalMessage := "Invalid params"
	if summaryMessage != "" {
		finalMessage = summaryMessage // Use provided summary if not empty
	}

	// Construct the data payload.
	// paramIssues itself can be the primary data, or it can be part of a larger map.
	// For simplicity, if paramIssues is the only data, we can use it directly.
	// Otherwise, wrap it.
	var dataPayload interface{}
	if paramIssues == nil {
		// If there are no specific paramIssues, the summaryMessage is key.
		// We can add the summaryMessage to details for consistency if needed.
		dataPayload = map[string]interface{}{"details": finalMessage}
	} else {
		// If paramIssues are provided, make them available under a "param_issues" key for clarity,
		// and include the overall summary message as "details".
		dataPayload = map[string]interface{}{
			"details":      summaryMessage, // General summary of the problem
			"param_issues": paramIssues,    // Specific field errors
		}
	}
	// If summaryMessage was empty and paramIssues exist, the top-level "Invalid params" is used.
	// It might be better to always have a summary message.
	// Let's refine: the main message is "Invalid params" or the summary.
	// The data field contains the specifics.

	return NewErrorDetail(CodeInvalidParams, finalMessage, dataPayload)
}

// NewInternalError creates an ErrorDetail for unexpected server errors.
// JSON-RPC: -32603
func NewInternalError(details string) *models.ErrorDetail {
	return NewErrorDetail(CodeInternalError, "Internal error", map[string]string{"details": details})
}

// NewFileSystemError creates a generic file system ErrorDetail.
// App specific: -32001
func NewFileSystemError(filename, operation, details string) *models.ErrorDetail {
	return NewErrorDetail(CodeFileSystemError, "File system error", map[string]string{
		"filename":  filename,
		"operation": operation,
		"details":   details,
	})
}

// NewFileNotFoundError creates an ErrorDetail for file not found errors.
// App specific: -32001. HTTP status: 404.
func NewFileNotFoundError(filename, operation string) *models.ErrorDetail {
	return NewErrorDetail(CodeFileSystemError, fmt.Sprintf("File '%s' not found", filename), map[string]interface{}{ // Changed to map[string]interface{}
		"filename":  filename,
		"operation": operation,
		"type":      "file_not_found",
	})
}

// NewPermissionDeniedError creates an ErrorDetail for permission denied errors.
// App specific: -32001. HTTP status: 403.
func NewPermissionDeniedError(filename, operation string) *models.ErrorDetail {
	return NewErrorDetail(CodeFileSystemError, fmt.Sprintf("Permission denied for file '%s'", filename), map[string]interface{}{ // Changed to map[string]interface{}
		"filename":  filename,
		"operation": operation,
		"type":      "permission_denied",
	})
}

// NewFileTooLargeError creates an ErrorDetail for files exceeding size limits.
// App specific: -32003. HTTP status: 413.
func NewFileTooLargeError(filename string, maxSizeMB int) *models.ErrorDetail {
	return NewErrorDetail(CodeFileTooLarge,
		fmt.Sprintf("File '%s' exceeds maximum allowed size of %d MB", filename, maxSizeMB),
		map[string]interface{}{
			"filename":    filename,
			"max_size_mb": maxSizeMB,
			"type":        "file_too_large",
		})
}

// NewOperationLockFailedError creates an ErrorDetail for failures to acquire a lock.
// App specific: -32002. HTTP status: 409 (Conflict) or 503 (Service Unavailable) might be appropriate.
func NewOperationLockFailedError(filename, operation string, details string) *models.ErrorDetail {
	return NewErrorDetail(CodeOperationLockFailed,
		fmt.Sprintf("Could not acquire lock for operation '%s' on file '%s'", operation, filename),
		map[string]interface{}{ // Changed to map[string]interface{}
			"filename":  filename,
			"operation": operation,
			"details":   details,
		})
}

// --- Conversion to HTTP and JSON-RPC Error Structures ---

// ToErrorResponse converts an ErrorDetail to an HTTP models.ErrorResponse.
func ToErrorResponse(errDetail *models.ErrorDetail) *models.ErrorResponse {
	if errDetail == nil {
		return nil
	}
	return &models.ErrorResponse{Error: *errDetail}
}

// ToJSONRPCError converts an ErrorDetail to a models.JSONRPCError.
// It attempts to map the Data field of ErrorDetail to models.JSONRPCErrorData.
func ToJSONRPCError(errDetail *models.ErrorDetail) *models.JSONRPCError {
	if errDetail == nil {
		return nil
	}
	rpcErr := &models.JSONRPCError{
		Code:    errDetail.Code,
		Message: errDetail.Message,
	}
	if errDetail.Data != nil {
		// Attempt to cast Data to a map to populate JSONRPCErrorData
		// This is a simple approach; more robust mapping might be needed if Data has diverse structures.
		if dataMap, ok := errDetail.Data.(map[string]string); ok { // Keep this for backward compatibility if some errors still use it
			rpcErr.Data = &models.JSONRPCErrorData{
				Filename:  dataMap["filename"],
				Operation: dataMap["operation"],
				Details:   dataMap["details"],
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			}
		} else if dataMapIf, ok := errDetail.Data.(map[string]interface{}); ok {
			// Handle cases where data might have mixed types (e.g. max_size_mb int)
			var filename, operation, details string
			// Safely extract known fields, converting if necessary
			if val, ok := dataMapIf["filename"].(string); ok { filename = val }
			if val, ok := dataMapIf["operation"].(string); ok { operation = val }

			// For details, it could be a simple string or structured (like param_issues)
			// If param_issues exists, format it. Otherwise, try to get "details" string.
			if pi, piOk := dataMapIf["param_issues"]; piOk {
				details = fmt.Sprintf("Parameter issues: %v. Summary: %v", pi, dataMapIf["details"])
			} else if val, ok := dataMapIf["details"].(string); ok {
				details = val
			}


			rpcErr.Data = &models.JSONRPCErrorData{
				Filename:  filename,
				Operation: operation,
				Details:   details,
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			}
		} else {
			// Fallback if Data is not a map[string]string or map[string]interface{}
			rpcErr.Data = &models.JSONRPCErrorData{
				Details:   fmt.Sprintf("%v", errDetail.Data),
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			}
		}
	}
	return rpcErr
}

// --- HTTP Status Mapping ---

// MapErrorToHTTPStatus maps an internal error code to an HTTP status code.
// This function might need to be more context-aware for overloaded codes like CodeFileSystemError.
func MapErrorToHTTPStatus(errorCode int, errDetail *models.ErrorDetail) int {
	switch errorCode {
	case CodeParseError:
		return http.StatusBadRequest
	case CodeInvalidRequest:
		return http.StatusBadRequest
	case CodeMethodNotFound:
		return http.StatusNotFound // Or http.StatusNotImplemented for JSON-RPC if more suitable
	case CodeInvalidParams:
		return http.StatusBadRequest
	case CodeInternalError:
		return http.StatusInternalServerError
	case CodeFileSystemError:
		// For CodeFileSystemError, we need more context, typically from ErrorDetail.Data
		if errDetail != nil && errDetail.Data != nil {
			// Check if Data is map[string]interface{} first, as it's more general
			if dataMapIf, ok := errDetail.Data.(map[string]interface{}); ok {
				if errorType, exists := dataMapIf["type"].(string); exists {
					switch errorType {
					case "file_not_found":
						return http.StatusNotFound
					case "permission_denied":
						return http.StatusForbidden
					}
				}
			} else if dataMap, ok := errDetail.Data.(map[string]string); ok { // Fallback for older style
				if errorType, exists := dataMap["type"]; exists {
					switch errorType {
					case "file_not_found":
						return http.StatusNotFound
					case "permission_denied":
						return http.StatusForbidden
					}
				}
			}
		}
		// Default for unspecific CodeFileSystemError, though ideally it should always have a 'type'.
		// Consider logging a warning if a FileSystemError doesn't have a specific type.
		return http.StatusInternalServerError // Or a more generic client error if appropriate
	case CodeFileTooLarge:
		return http.StatusRequestEntityTooLarge
	case CodeOperationLockFailed:
		return http.StatusConflict // Or 503 Service Unavailable
	default:
		// For unknown or custom server-side errors not fitting above categories.
		if errorCode < -32000 && errorCode > -32099 { // Server error range in JSON-RPC for custom app errors
			return http.StatusInternalServerError
		}
		return http.StatusInternalServerError // Default fallback
	}
}
