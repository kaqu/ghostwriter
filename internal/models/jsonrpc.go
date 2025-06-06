package models

import "encoding/json"

const ErrCodeParseError = -32700

// JSONRPCRequest represents a JSON-RPC request object.
type JSONRPCRequest struct {
	// JSONRPC specifies the version of the JSON-RPC protocol, must be "2.0".
	JSONRPC string `json:"jsonrpc"`
	// ID is a unique identifier established by the client.
	// It can be a string or a number. The server must reply with the same ID.
	// This field is omitted for notifications.
	ID interface{} `json:"id"`
	// Method is a string containing the name of the method to be invoked.
	Method string `json:"method"`
	// Params is a structured value that holds the parameter values to be used
	// during the invocation of the method. This field can be omitted.
	// We use json.RawMessage to defer parsing until the method is known.
	Params json.RawMessage `json:"params"`
}

// JSONRPCErrorData defines the structure for the 'data' field within a JSON-RPC error object.
// It provides additional, application-specific error information.
type JSONRPCErrorData struct {
	// Filename is the name of the file involved in the error, if applicable.
	Filename string `json:"filename,omitempty"`
	// Operation is the operation being performed when the error occurred, if applicable.
	Operation string `json:"operation,omitempty"`
	// Timestamp records when the error occurred.
	Timestamp string `json:"timestamp,omitempty"`
	// Details provides any other specific details about the error.
	Details string `json:"details,omitempty"`
}

// JSONRPCError represents a JSON-RPC error object.
type JSONRPCError struct {
	// Code is a number that indicates the error type that occurred.
	// Predefined JSON-RPC error codes are used, or application-specific ones.
	Code int `json:"code"`
	// Message is a string providing a short description of the error.
	Message string `json:"message"`
	// Data is a primitive or structured value that contains additional
	// information about the error. It may be omitted.
	// The value of this member is defined by the server.
	Data *JSONRPCErrorData `json:"data,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC response object.
type JSONRPCResponse struct {
	// JSONRPC specifies the version of the JSON-RPC protocol, must be "2.0".
	JSONRPC string `json:"jsonrpc"`
	// ID is the identifier of the request to which this response is a reply.
	// It must be the same as the ID of the request.
	ID interface{} `json:"id"`
	// Result contains the result of the method invocation if there was no error.
	// This field is required on success.
	// This field must not exist if there was an error invoking the method.
	Result interface{} `json:"result,omitempty"`
	// Error contains an error object if an error occurred during the method invocation.
	// This field is required on failure.
	// This field must not exist if there was no error invoking the method.
	Error *JSONRPCError `json:"error,omitempty"`
}
