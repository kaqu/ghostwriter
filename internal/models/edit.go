package models

// EditOperation defines a single operation to be performed on a file.
type EditOperation struct {
	// Line is the 1-based line number where the operation should occur.
	Line int `json:"line"`
	// Content is the new content for "replace" or "insert" operations. Optional for "delete".
	Content string `json:"content,omitempty"`
	// Operation specifies the type of edit: "replace", "insert", or "delete".
	Operation string `json:"operation"`
}

// EditFileRequest represents a request to edit a file.
// It can include multiple edit operations, an append operation, or create the file if it's missing.
type EditFileRequest struct {
	// Name is the name of the file to edit.
	Name string `json:"name"`
	// Edits is an optional list of edit operations to apply to the file.
	// These are processed sequentially.
	Edits []EditOperation `json:"edits,omitempty"`
	// Append is an optional string to append to the end of the file.
	// This is processed after all 'Edits'.
	Append string `json:"append,omitempty"`
	// CreateIfMissing, if true, will create the file if it does not already exist.
	// If false and the file doesn't exist, an error will be returned.
	CreateIfMissing bool `json:"create_if_missing,omitempty"`
}

// EditFileResponse represents the response from a file edit operation.
type EditFileResponse struct {
	// Success indicates whether the overall edit operation was successful.
	Success bool `json:"success"`
	// LinesModified is the number of lines that were actually modified by the edit operations.
	LinesModified int `json:"lines_modified"`
	// FileCreated indicates if a new file was created as part of the operation.
	FileCreated bool `json:"file_created"`
	// NewTotalLines is the total number of lines in the file after the edits.
	NewTotalLines int `json:"new_total_lines"`
}
