package mcp

import (
	"encoding/json"
	"file-editor-server/internal/errors"
	"file-editor-server/internal/models"
	"file-editor-server/internal/service"
	"fmt"
	"strings"
)

const (
	protocolVersion   = "2024-11-05"
	serverVersion     = "1.0.0"
	serverName        = "file-editing-server"
	serverDescription = "High-performance file editing server for AI agents"
)

// ToolCallParams represents the parameters for a tool call.
type ToolCallParams struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// MCPProcessor handles MCP (Meta-Circular Protocol) requests.
type MCPProcessor struct {
	service service.FileOperationService
}

// MCPProcessorInterface defines the interface for MCP request processing.
type MCPProcessorInterface interface {
	ProcessRequest(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError)
	ExecuteTool(toolName string, argumentsStruct interface{}) (*models.MCPToolResult, error) // Added for HTTP handler
}

// NewMCPProcessor creates a new MCPProcessor.
func NewMCPProcessor(svc service.FileOperationService) *MCPProcessor {
	return &MCPProcessor{
		service: svc,
	}
}

// ProcessRequest handles a JSON-RPC request and returns an MCPToolResult or a JSONRPCError.
func (p *MCPProcessor) ProcessRequest(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
	switch req.Method {
	case "initialize":
		initResponse := models.InitializeResponse{ // Use models.InitializeResponse
			ProtocolVersion: protocolVersion,
			Capabilities: models.Capabilities{ // Use models.Capabilities
				Tools: models.ToolsCapabilities{}, // Empty as per spec
			},
			ServerInfo: models.ServerInfo{ // Use models.ServerInfo
				Name:        serverName,
				Version:     serverVersion,
				Description: serverDescription,
			},
		}
		jsonResponseBytes, err := json.Marshal(initResponse)
		if err != nil {
			// This should ideally not happen for a fixed structure.
			// Return a standard MCP error if it does.
			return &models.MCPToolResult{
				Content: []models.MCPToolContent{
					{Type: "text", Text: fmt.Sprintf("Error: Failed to marshal initialize response: %v (Code: %d)", err, errors.CodeInternalError)},
				},
				IsError: true,
			}, nil
		}
		return &models.MCPToolResult{
			Content: []models.MCPToolContent{
				{Type: "text", Text: string(jsonResponseBytes)},
			},
			IsError: false,
		}, nil
	case "tools/list":
		tools := []models.ToolDefinition{ // Use models.ToolDefinition
			{
				Name:        "list_files",
				Description: "Lists all non-hidden files in the working directory, providing name, modification time, and line count.",
				ArgumentsSchema: models.Schema{ // Use models.Schema
					"type":       "object",
					"properties": models.Schema{}, // Use models.Schema
				},
				ResponseSchema: models.Schema{ // Use models.Schema
					"type":        "string",
					"description": "Text output detailing files: name, modified, lines. See spec 3.4.1.",
				},
				Annotations: models.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: false}, // Use models.ToolAnnotations
			},
			{
				Name:        "read_file",
				Description: "Reads the content of a specified file, optionally within a given line range.",
				ArgumentsSchema: models.Schema{ // Use models.Schema
					"type": "object",
					"properties": models.Schema{ // Use models.Schema
						"name":       models.Schema{"type": "string", "pattern": "^[a-zA-Z0-9._-]+$", "minLength": 1, "maxLength": 255},
						"start_line": models.Schema{"type": "integer", "minimum": 1},
						"end_line":   models.Schema{"type": "integer", "minimum": 1},
					},
					"required": []string{"name"},
				},
				ResponseSchema: models.Schema{ // Use models.Schema
					"type":        "string",
					"description": "Text output of file content or range. See spec 3.4.2.",
				},
				Annotations: models.ToolAnnotations{ReadOnlyHint: true, DestructiveHint: false}, // Use models.ToolAnnotations
			},
			{
				Name:        "edit_file",
				Description: "Edits a file using line-based operations, creates if missing (with flag), or appends content.",
				ArgumentsSchema: models.Schema{ // Use models.Schema
					"type": "object",
					"properties": models.Schema{ // Use models.Schema
						"name": models.Schema{"type": "string", "pattern": "^[a-zA-Z0-9._-]+$", "minLength": 1, "maxLength": 255},
						"edits": models.Schema{ // Use models.Schema
							"type": "array",
							"items": models.Schema{ // Use models.Schema
								"type": "object",
								"properties": models.Schema{ // Use models.Schema
									"line":      models.Schema{"type": "integer", "minimum": 1},
									"content":   models.Schema{"type": "string"},
									"operation": models.Schema{"type": "string", "enum": []string{"replace", "insert", "delete"}},
								},
								"required": []string{"line", "operation"},
							},
						},
						"append":            models.Schema{"type": "string"},
						"create_if_missing": models.Schema{"type": "boolean", "default": false},
					},
					"required": []string{"name"},
				},
				ResponseSchema: models.Schema{ // Use models.Schema
					"type":        "string",
					"description": "Text output summarizing edit results. See spec 3.4.3.",
				},
				Annotations: models.ToolAnnotations{ReadOnlyHint: false, DestructiveHint: true}, // Use models.ToolAnnotations
			},
		}
		toolsListResponse := models.ToolsListResponse{Tools: tools} // Use models.ToolsListResponse
		jsonResponseBytes, err := json.Marshal(toolsListResponse)
		if err != nil {
			return &models.MCPToolResult{
				Content: []models.MCPToolContent{
					{Type: "text", Text: fmt.Sprintf("Error: Failed to marshal tools/list response: %v (Code: %d)", err, errors.CodeInternalError)},
				},
				IsError: true,
			}, nil
		}
		return &models.MCPToolResult{
			Content: []models.MCPToolContent{{Type: "text", Text: string(jsonResponseBytes)}},
			IsError: false,
		}, nil
	case "tools/call":
		var params ToolCallParams
		if err := json.Unmarshal(req.Params, &params); err != nil {
			return nil, &models.JSONRPCError{
				Code:    -32602, // Invalid Params
				Message: "Invalid parameters for tools/call: " + err.Error(),
			}
		}
		return p.handleToolCall(params.Name, params.Arguments)
	default:
		return nil, &models.JSONRPCError{
			Code:    -32601, // Method not found
			Message: "Method not found: " + req.Method,
		}
	}
}

// handleToolCall is a helper to dispatch tool calls based on name and arguments.
func (p *MCPProcessor) handleToolCall(toolName string, toolArgs json.RawMessage) (*models.MCPToolResult, *models.JSONRPCError) {
	switch toolName {
	case "list_files":
		var listParams models.ListFilesRequest
		if err := json.Unmarshal(toolArgs, &listParams); err != nil {
			return nil, &models.JSONRPCError{
				Code:    -32602, // Invalid Params
				Message: "Invalid parameters for list_files: " + err.Error(),
			}
		}
		files, serviceErr := p.service.ListFiles(listParams)
		if serviceErr != nil {
			return &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: p.formatToolError(serviceErr)}},
				IsError: true,
			}, nil
		}
		return &models.MCPToolResult{
			Content: []models.MCPToolContent{{Type: "text", Text: p.formatListFilesResult(files)}},
			IsError: false,
		}, nil
	case "read_file":
		var readParams models.ReadFileRequest
		if err := json.Unmarshal(toolArgs, &readParams); err != nil {
			return nil, &models.JSONRPCError{
				Code:    -32602, // Invalid Params
				Message: "Invalid parameters for read_file: " + err.Error(),
			}
		}
		content, filename, totalLines, reqStartLine, reqEndLine, actualEndLine, isRangeRequest, serviceErr := p.service.ReadFile(readParams)
		if serviceErr != nil {
			return &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: p.formatToolError(serviceErr)}},
				IsError: true,
			}, nil
		}
		return &models.MCPToolResult{
			Content: []models.MCPToolContent{{Type: "text", Text: p.formatReadFileResult(content, filename, totalLines, reqStartLine, reqEndLine, actualEndLine, isRangeRequest)}},
			IsError: false,
		}, nil
	case "edit_file":
		var editParams models.EditFileRequest
		if err := json.Unmarshal(toolArgs, &editParams); err != nil {
			return nil, &models.JSONRPCError{
				Code:    -32602, // Invalid Params
				Message: "Invalid parameters for edit_file: " + err.Error(),
			}
		}
		filename, linesModified, newTotalLines, fileCreated, serviceErr := p.service.EditFile(editParams)
		if serviceErr != nil {
			return &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: p.formatToolError(serviceErr)}},
				IsError: true,
			}, nil
		}
		return &models.MCPToolResult{
			Content: []models.MCPToolContent{{Type: "text", Text: p.formatEditFileResult(filename, linesModified, newTotalLines, fileCreated)}},
			IsError: false,
		}, nil
	default:
		return &models.MCPToolResult{
			Content: []models.MCPToolContent{{Type: "text", Text: "Error: Unknown tool '" + toolName + "'."}},
			IsError: true,
		}, nil
	}
}

// formatListFilesResult formats the result of a list_files call. (Spec 3.4.1)
func (p *MCPProcessor) formatListFilesResult(files []models.FileInfo) string {
	if len(files) == 0 {
		return "Total files: 0"
	}
	var builder strings.Builder
	builder.WriteString("Files in directory:\n\n") // Two newlines as per example structure
	for _, f := range files {
		// Note: FileInfo.Modified is already ISO8601 string from service layer
		// Lines is f.Lines, which can be -1 for error or too large. Spec doesn't explicitly cover -1.
		// Assuming -1 means "unknown" or should be handled gracefully if spec clarified.
		// For now, using the value directly.
		lineCountStr := fmt.Sprintf("%d", f.Lines)
		if f.Lines == -1 {
			lineCountStr = "(unknown)" // Or handle as per more detailed spec if available
		}
		builder.WriteString(fmt.Sprintf("name: %s, modified: %s, lines: %s\n", f.Name, f.Modified, lineCountStr))
	}
	builder.WriteString(fmt.Sprintf("\nTotal files: %d", len(files))) // One newline before total
	return builder.String()
}

// formatReadFileResult formats the result of a read_file call. (Spec 3.4.2)
func (p *MCPProcessor) formatReadFileResult(content string, filename string, totalLines int, reqStartLine int, reqEndLine int, actualEndLine int, isRangeRequest bool) string {
	if totalLines == 0 && !isRangeRequest { // Handles empty file case specifically
		return fmt.Sprintf("File: %s (0 lines)\n\n", filename)
	}

	var header string
	if isRangeRequest {
		var startLineForDisplay, endLineForDisplay int

		if content == "" && reqStartLine > 0 { // Range requested yielded no content (e.g. start_line > totalLines)
			startLineForDisplay = reqStartLine
			// To show an empty range like "lines 5-4", end_line_for_display should be start_line_for_display - 1
			endLineForDisplay = reqStartLine - 1
			if endLineForDisplay < 0 { // Avoid negative line numbers if reqStartLine was 1 and content empty
				endLineForDisplay = 0
			}
		} else if content == "" && reqStartLine == 0 && reqEndLine > 0 { // Range like 0-5 on empty file
			startLineForDisplay = 1
			endLineForDisplay = 0
		} else if content == "" && reqStartLine == 0 && reqEndLine == 0 { // Full read of empty file, but isRangeRequest might be true if service defaults it.
			// This case should ideally be caught by totalLines == 0 && !isRangeRequest above.
			// If isRangeRequest is true for a full empty file read, treat as lines 1-0 of 0 total.
			startLineForDisplay = 1
			endLineForDisplay = 0
		} else {
			if reqStartLine > 0 {
				startLineForDisplay = reqStartLine
			} else {
				startLineForDisplay = 1 // Default to 1 if reqStartLine is 0 (means from beginning)
			}
			endLineForDisplay = actualEndLine + 1
		}
		header = fmt.Sprintf("File: %s (lines %d-%d of %d total)", filename, startLineForDisplay, endLineForDisplay, totalLines)
	} else {
		header = fmt.Sprintf("File: %s (%d lines)", filename, totalLines)
	}

	// Ensure two newlines after header if content follows, or if content is empty.
	// If content has its own trailing newline, one extra newline is fine.
	// If content is empty, two newlines are needed.
	if content == "" {
		return header + "\n\n"
	}
	return fmt.Sprintf("%s\n\n%s", header, content)
}

// formatEditFileResult formats the result of an edit_file call. (Spec 3.4.3)
func (p *MCPProcessor) formatEditFileResult(filename string, linesModified int, newTotalLines int, fileCreated bool) string {
	return fmt.Sprintf("File edited successfully: %s\nLines modified: %d\nTotal lines: %d\nFile created: %t",
		filename, linesModified, newTotalLines, fileCreated)
}

// formatToolError formats a service error into the specified string. (Spec 3.4.4)
func (p *MCPProcessor) formatToolError(serviceErr *models.ErrorDetail) string {
	if serviceErr == nil {
		return "Error: An unexpected error occurred, but no details were provided."
	}
	return fmt.Sprintf("Error: %s", serviceErr.Message)
}

// ExecuteTool handles a direct tool call with already parsed arguments.
// It's used by transports like HTTP where request parsing happens before MCP processing.
func (p *MCPProcessor) ExecuteTool(toolName string, argumentsStruct interface{}) (*models.MCPToolResult, error) {
	switch toolName {
	case "list_files":
		params, ok := argumentsStruct.(models.ListFilesRequest)
		if !ok {
			return nil, fmt.Errorf("invalid arguments type for list_files: expected models.ListFilesRequest, got %T", argumentsStruct)
		}
		files, serviceErr := p.service.ListFiles(params)
		if serviceErr != nil {
			return &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: p.formatToolError(serviceErr)}},
				IsError: true,
			}, nil
		}
		return &models.MCPToolResult{
			Content: []models.MCPToolContent{{Type: "text", Text: p.formatListFilesResult(files)}},
			IsError: false,
		}, nil
	case "read_file":
		params, ok := argumentsStruct.(models.ReadFileRequest)
		if !ok {
			return nil, fmt.Errorf("invalid arguments type for read_file: expected models.ReadFileRequest, got %T", argumentsStruct)
		}
		content, filename, totalLines, reqStartLine, reqEndLine, actualEndLine, isRangeRequest, serviceErr := p.service.ReadFile(params)
		if serviceErr != nil {
			return &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: p.formatToolError(serviceErr)}},
				IsError: true,
			}, nil
		}
		return &models.MCPToolResult{
			Content: []models.MCPToolContent{{Type: "text", Text: p.formatReadFileResult(content, filename, totalLines, reqStartLine, reqEndLine, actualEndLine, isRangeRequest)}},
			IsError: false,
		}, nil
	case "edit_file":
		params, ok := argumentsStruct.(models.EditFileRequest)
		if !ok {
			return nil, fmt.Errorf("invalid arguments type for edit_file: expected models.EditFileRequest, got %T", argumentsStruct)
		}
		filename, linesModified, newTotalLines, fileCreated, serviceErr := p.service.EditFile(params)
		if serviceErr != nil {
			return &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: p.formatToolError(serviceErr)}},
				IsError: true,
			}, nil
		}
		return &models.MCPToolResult{
			Content: []models.MCPToolContent{{Type: "text", Text: p.formatEditFileResult(filename, linesModified, newTotalLines, fileCreated)}},
			IsError: false,
		}, nil
	default:
		return &models.MCPToolResult{
			Content: []models.MCPToolContent{{Type: "text", Text: "Error: Unknown tool '" + toolName + "'."}},
			IsError: true,
		}, nil // Or return an error: fmt.Errorf("unknown tool: %s", toolName)
	}
}
