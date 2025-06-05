package models

// ReadFileRequest represents a request to read a file.
type ReadFileRequest struct {
	// Name is the name of the file to read.
	Name string `json:"name"`
	// StartLine is the optional 1-based starting line number for partial file reads.
	StartLine int `json:"start_line,omitempty"`
	// EndLine is the optional 1-based ending line number for partial file reads.
	EndLine int `json:"end_line,omitempty"`
}

// RangeRequested indicates the range of lines that were requested,
// useful in the response to confirm what was processed.
type RangeRequested struct {
	// StartLine is the 1-based starting line number that was requested.
	StartLine int `json:"start_line,omitempty"`
	// EndLine is the 1-based ending line number that was requested.
	EndLine int `json:"end_line,omitempty"`
}

// ReadFileResponse represents the response from a file read operation.
type ReadFileResponse struct {
	// Content is the content of the file, or a specific range of lines.
	Content string `json:"content"`
	// TotalLines is the total number of lines in the file.
	TotalLines int `json:"total_lines"`
	// RangeRequested indicates the specific range of lines returned if a partial read was requested.
	RangeRequested *RangeRequested `json:"range_requested,omitempty"`
}
