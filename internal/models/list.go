package models

// FileInfo describes a file in the directory listing.
type FileInfo struct {
	Name     string `json:"name"`
	Size     int64  `json:"size"`     // Using int64 for file size
	Modified string `json:"modified"` // ISO 8601 date-time string
	Readable bool   `json:"readable"`
	Writable bool   `json:"writable"`
	Lines    int    `json:"lines"`
}

// ListFilesRequest represents a request to list files.
// Currently, the specification implies no parameters are needed.
type ListFilesRequest struct {
	// No parameters currently specified by the spec for list_files
}

// ListFilesResponse represents the response from a list files operation.
type ListFilesResponse struct {
	Files      []FileInfo `json:"files"`
	TotalCount int        `json:"total_count"`
	Directory  string     `json:"directory"`
}
