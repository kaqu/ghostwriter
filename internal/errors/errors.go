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
)

// CodeFileTooLargeType is a string identifier for file too large errors.
const CodeFileTooLargeType = "file_too_large"

// CodeInvalidEncodingType is a string identifier for invalid encoding errors.
const CodeInvalidEncodingType = "invalid_encoding"

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
	data := map[string]interface{}{
		"details":   details,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	return NewErrorDetail(CodeParseError, "Parse error", data)
}

// NewInvalidRequestError creates an ErrorDetail for invalid JSON-RPC Request objects.
// JSON-RPC: -32600
func NewInvalidRequestError(details string) *models.ErrorDetail {
	data := map[string]interface{}{
		"details":   details,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	return NewErrorDetail(CodeInvalidRequest, "Invalid Request", data)
}

// NewMethodNotFoundError creates an ErrorDetail when a JSON-RPC method is not found.
// JSON-RPC: -32601
func NewMethodNotFoundError(methodName string) *models.ErrorDetail {
	data := map[string]interface{}{
		"method":    methodName,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	return NewErrorDetail(CodeMethodNotFound, "Method not found", data)
}

// NewInvalidParamsError creates an ErrorDetail for invalid method parameters.
// JSON-RPC: -32602
// paramIssues can contain specific details about which parameters were invalid.
// Optional filename and operation can be provided for context.
func NewInvalidParamsError(summaryMessage string, paramIssues map[string]interface{}, filename ...string) *models.ErrorDetail {
	finalMessage := "Invalid params"
	if summaryMessage != "" {
		finalMessage = summaryMessage
	}

	dataPayload := map[string]interface{}{
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}

	if paramIssues == nil {
		dataPayload["details"] = finalMessage
	} else {
		dataPayload["details"] = summaryMessage
		dataPayload["param_issues"] = paramIssues
	}

	// Check for optional filename and operation (assuming operation might be the second element if filename is present)
	// This is a simple way to handle optional args; a struct or options pattern might be better for more args.
	if len(filename) > 0 && filename[0] != "" {
		dataPayload["filename"] = filename[0]
		if len(filename) > 1 && filename[1] != "" {
			dataPayload["operation"] = filename[1] // Assuming second optional arg is operation
		}
	}

	return NewErrorDetail(CodeInvalidParams, finalMessage, dataPayload)
}

// NewInternalError creates an ErrorDetail for unexpected server errors.
// JSON-RPC: -32603
func NewInternalError(details string) *models.ErrorDetail {
	data := map[string]interface{}{
		"details":   details,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	return NewErrorDetail(CodeInternalError, "Internal error", data)
}

// NewFileSystemError creates a generic file system ErrorDetail.
// App specific: -32001
func NewFileSystemError(filename, operation, details string) *models.ErrorDetail {
	data := map[string]interface{}{
		"filename":  filename,
		"operation": operation,
		"details":   details,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	return NewErrorDetail(CodeFileSystemError, "File system error", data)
}

// NewFileNotFoundError creates an ErrorDetail for file not found errors.
// App specific: -32001. HTTP status: 404.
func NewFileNotFoundError(filename, operation string) *models.ErrorDetail {
	data := map[string]interface{}{
		"filename":  filename,
		"operation": operation,
		"type":      "file_not_found",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	return NewErrorDetail(CodeFileSystemError, fmt.Sprintf("File '%s' not found", filename), data)
}

// NewPermissionDeniedError creates an ErrorDetail for permission denied errors.
// App specific: -32001. HTTP status: 403.
func NewPermissionDeniedError(filename, operation string) *models.ErrorDetail {
	data := map[string]interface{}{
		"filename":  filename,
		"operation": operation,
		"type":      "permission_denied",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	return NewErrorDetail(CodeFileSystemError, fmt.Sprintf("Permission denied for file '%s'", filename), data)
}

// NewFileTooLargeError creates an ErrorDetail for files exceeding size limits.
// App specific: uses CodeFileSystemError. HTTP status: 413.
func NewFileTooLargeError(filename string, operation string, currentSize int64, maxSizeMB int) *models.ErrorDetail {
	message := fmt.Sprintf("File '%s' (%d bytes) exceeds maximum allowed size of %d MB during %s operation", filename, currentSize, maxSizeMB, operation)
	data := map[string]interface{}{
		"filename":           filename,
		"operation":          operation,
		"current_size_bytes": currentSize,
		"max_size_mb":        maxSizeMB,
		"type":               CodeFileTooLargeType,
		"timestamp":          time.Now().UTC().Format(time.RFC3339),
	}
	return NewErrorDetail(CodeFileSystemError, message, data)
}

// NewInvalidEncodingError creates an ErrorDetail for invalid file encoding.
// App specific: uses CodeFileSystemError. HTTP status: 400.
func NewInvalidEncodingError(filename, operation, details string) *models.ErrorDetail {
	data := map[string]interface{}{
		"filename":  filename,
		"operation": operation,
		"details":   details,
		"type":      CodeInvalidEncodingType,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	return NewErrorDetail(CodeFileSystemError, fmt.Sprintf("File '%s' has invalid encoding: %s", filename, details), data)
}

// NewOperationLockFailedError creates an ErrorDetail for failures to acquire a lock.
// App specific: -32002. HTTP status: 409 (Conflict) or 503 (Service Unavailable) might be appropriate.
func NewOperationLockFailedError(filename, operation string, details string) *models.ErrorDetail {
	data := map[string]interface{}{
		"filename":  filename,
		"operation": operation,
		"details":   details,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	return NewErrorDetail(CodeOperationLockFailed,
		fmt.Sprintf("Could not acquire lock for operation '%s' on file '%s'", operation, filename),
		data)
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
// It maps fields from ErrorDetail.Data (expected to be map[string]interface{})
// to models.JSONRPCErrorData.
func ToJSONRPCError(errDetail *models.ErrorDetail) *models.JSONRPCError {
	if errDetail == nil {
		return nil
	}
	rpcErr := &models.JSONRPCError{
		Code:    errDetail.Code,
		Message: errDetail.Message,
	}

	if errDetail.Data != nil {
		if dataMap, ok := errDetail.Data.(map[string]interface{}); ok {
			// Initialize JSONRPCErrorData
			rpcErr.Data = &models.JSONRPCErrorData{}

			// Directly map known fields, checking for type safety
			if ts, ok := dataMap["timestamp"].(string); ok {
				rpcErr.Data.Timestamp = ts
			} else {
				// Fallback if timestamp is missing or not a string
				rpcErr.Data.Timestamp = time.Now().UTC().Format(time.RFC3339)
			}

			if filename, ok := dataMap["filename"].(string); ok {
				rpcErr.Data.Filename = filename
			}
			if operation, ok := dataMap["operation"].(string); ok {
				rpcErr.Data.Operation = operation
			}

			// Handle 'details' which might be a simple string or part of structured data
			// For instance, 'param_issues' might exist alongside a 'details' summary.
			var detailsString string
			if details, ok := dataMap["details"].(string); ok {
				detailsString = details
			}

			if paramIssues, ok := dataMap["param_issues"]; ok {
				if detailsString != "" {
					detailsString = fmt.Sprintf("%s. Parameter issues: %v", detailsString, paramIssues)
				} else {
					detailsString = fmt.Sprintf("Parameter issues: %v", paramIssues)
				}
			}
			rpcErr.Data.Details = detailsString

			// Include any other fields from dataMap directly, if JSONRPCErrorData is extended
			// or if we want to pass them through. For now, we map specific fields.
			// Example for 'type' or other custom fields if they were part of JSONRPCErrorData:
			// if typeVal, ok := dataMap["type"].(string); ok { rpcErr.Data.Type = typeVal }

		} else {
			// Fallback if Data is not map[string]interface{} (should ideally not happen)
			// Create minimal JSONRPCErrorData with details and a new timestamp.
			rpcErr.Data = &models.JSONRPCErrorData{
				Details:   fmt.Sprintf("%v", errDetail.Data), // Convert whatever Data is to string
				Timestamp: time.Now().UTC().Format(time.RFC3339),
			}
		}
	}
	// The else branch for "errDetail.Data == nil" was empty and has been removed.
	// If errDetail.Data is nil, rpcErr.Data remains nil, which is the intended behavior.

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
					case CodeInvalidEncodingType:
						return http.StatusBadRequest
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
		// For CodeFileSystemError, also check for file_too_large type
		if errDetail != nil && errDetail.Data != nil {
			if dataMapIf, ok := errDetail.Data.(map[string]interface{}); ok {
				if errorType, exists := dataMapIf["type"].(string); exists {
					if errorType == CodeFileTooLargeType {
						return http.StatusRequestEntityTooLarge
					}
				}
			}
		}
		return http.StatusInternalServerError // Or a more generic client error if appropriate
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
