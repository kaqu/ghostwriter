package models

// MCPToolContent represents the content of a tool call or result.
type MCPToolContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

// MCPToolResult represents the result of a tool call.
type MCPToolResult struct {
	Content []MCPToolContent `json:"content"`
	IsError bool             `json:"isError"`
}
