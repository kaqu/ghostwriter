package models

// ErrorDetail provides a structured way to represent an error.
type ErrorDetail struct {
	// Code is an application-specific error code.
	Code int `json:"code"`
	// Message is a human-readable error message.
	Message string `json:"message"`
	// Data holds additional context about the error, like filename or operation.
	Data interface{} `json:"data,omitempty"`
}

// ErrorResponse is a generic structure for returning errors, often used in HTTP responses.
type ErrorResponse struct {
	// Error contains the details of the error.
	Error ErrorDetail `json:"error"`
}
