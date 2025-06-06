package mcp

import (
	"encoding/json"
	"file-editor-server/internal/models"
	"file-editor-server/internal/service"
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
		return &models.MCPToolResult{
			Content: []models.MCPToolContent{
				{Type: "text", Text: "MCP server initialized. Available tools: list_files, read_file, edit_file."},
			},
			IsError: false,
		}, nil
	case "tools/list":
		// Full descriptions as per a potential SPECIFICATION.md (assuming simple descriptions for now)
		toolListText := "Available tools:\n" +
			"- list_files: Lists all non-hidden files in the working directory. Provides name, size, modification time, permissions, and line count for each file.\n" +
			"- read_file: Reads the content of a specified file. Can read the full file or a specified range of lines. Reports total lines and the range read.\n" +
			"- edit_file: Edits a specified file with a series of operations (insert, replace, delete lines). Can also append content and create the file if it doesn't exist. Reports lines modified, new total lines, and if the file was created."
		return &models.MCPToolResult{
			Content: []models.MCPToolContent{
				{Type: "text", Text: toolListText},
			},
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
		files, dir, serviceErr := p.service.ListFiles(listParams)
		if serviceErr != nil {
			return &models.MCPToolResult{
				Content: []models.MCPToolContent{{Type: "text", Text: p.formatToolError(serviceErr)}},
				IsError: true,
			}, nil
		}
		return &models.MCPToolResult{
			Content: []models.MCPToolContent{{Type: "text", Text: p.formatListFilesResult(files, dir)}},
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

// formatListFilesResult formats the result of a list_files call.
func (p *MCPProcessor) formatListFilesResult(files []models.FileInfo, directory string) string {
	if len(files) == 0 {
		return fmt.Sprintf("No files found in directory: %s", directory)
	}
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Directory: %s\nTotal files: %d\n\n", directory, len(files)))
	builder.WriteString("Files:\n")
	for _, f := range files {
		builder.WriteString(fmt.Sprintf("- Name: %s\n", f.Name))
		builder.WriteString(fmt.Sprintf("  Size: %d bytes\n", f.Size))
		builder.WriteString(fmt.Sprintf("  Modified: %s\n", f.Modified))
		builder.WriteString(fmt.Sprintf("  Readable: %t\n", f.Readable))
		builder.WriteString(fmt.Sprintf("  Writable: %t\n", f.Writable))
		if f.Lines == -1 {
			builder.WriteString("  Lines: (error or too large to count)\n")
		} else {
			builder.WriteString(fmt.Sprintf("  Lines: %d\n", f.Lines))
		}
	}
	return builder.String()
}

// formatReadFileResult formats the result of a read_file call.
func (p *MCPProcessor) formatReadFileResult(content string, filename string, totalLines int, reqStartLine int, reqEndLine int, actualEndLine int, isRangeRequest bool) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("File: %s\n", filename))
	builder.WriteString(fmt.Sprintf("Total Lines: %d\n", totalLines))

	if isRangeRequest {
		// Determine the 1-based actual start line for display
		actualDisplayStartLine := 0
		if totalLines == 0 || actualEndLine == -1 { // Empty file or no lines in range
			actualDisplayStartLine = 0
		} else if len(strings.Split(content, "\n")) > 0 && content != "" {
			// If content is not empty, the range implies actual lines were returned.
			// reqStartLine is the original request.
			// If full file read was effectively done due to only one of start/end being set,
			// then actual start is 1.
			if reqStartLine == 0 && reqEndLine != 0 { // end_line set, start_line was 0 -> effective start is 1
				actualDisplayStartLine = 1
			} else if reqStartLine != 0 {
				actualDisplayStartLine = reqStartLine
			} else { // Both 0, full file
				actualDisplayStartLine = 1
			}
		} else if reqStartLine > 0 { // Content is empty, but a start was requested.
			actualDisplayStartLine = reqStartLine
		}


		// Determine the 1-based actual end line for display
		actualDisplayEndLine := 0
		if actualEndLine != -1 {
			actualDisplayEndLine = actualEndLine + 1
		}

		// If the request was for a full file (isRangeRequest=false but handled by ReadFile logic as a range)
		// then the actualDisplayStartLine and actualDisplayEndLine should reflect the full file.
		if reqStartLine == 0 && reqEndLine == 0 && totalLines > 0 { // Full file implied
			actualDisplayStartLine = 1
			actualDisplayEndLine = totalLines
		} else if reqStartLine != 0 && reqEndLine == 0 && totalLines > 0 { // Start to end of file
			actualDisplayStartLine = reqStartLine
			actualDisplayEndLine = totalLines
		} else if reqStartLine == 0 && reqEndLine != 0 && totalLines > 0 { // Beginning of file to end_line
			actualDisplayStartLine = 1
			actualDisplayEndLine = reqEndLine
			if actualDisplayEndLine > totalLines { actualDisplayEndLine = totalLines }
		}


		builder.WriteString(fmt.Sprintf("Requested Range: start_line=%d, end_line=%d\n", reqStartLine, reqEndLine))
		builder.WriteString(fmt.Sprintf("Actual Range Returned: start_line=%d, end_line=%d\n", actualDisplayStartLine, actualDisplayEndLine))
	}
	builder.WriteString(fmt.Sprintf("\nContent:\n%s", content))
	return builder.String()
}

// formatEditFileResult formats the result of an edit_file call.
func (p *MCPProcessor) formatEditFileResult(filename string, linesModified int, newTotalLines int, fileCreated bool) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("File: %s\n", filename))
	if fileCreated {
		builder.WriteString("Status: File created successfully.\n")
	} else {
		builder.WriteString("Status: File edited successfully.\n")
	}
	builder.WriteString(fmt.Sprintf("Lines Modified: %d\n", linesModified))
	builder.WriteString(fmt.Sprintf("New Total Lines: %d\n", newTotalLines))
	return builder.String()
}

// formatToolError formats a service error into the specified string.
func (p *MCPProcessor) formatToolError(serviceErr *models.ErrorDetail) string {
	if serviceErr == nil {
		// This case should ideally not be reached if an error occurs.
		return "Error: An unexpected error occurred, but no details were provided."
	}
	// Using the structure from SPECIFICATION.md 3.4.4
	// "Error: <ErrorType>: <Message> (Code: <code>)"
	// ErrorDetail.Data might contain more specific type info, or it might be in Message.
	// For now, let's assume Message contains enough context.
	// If ErrorDetail.Data has a "type" field, it could be used.
	// Example: Error: file_not_found: File 'nonexistent.txt' not found. (Code: 1001)
	// For simplicity, using Message and Code directly.
	return fmt.Sprintf("Error: %s (Code: %d)", serviceErr.Message, serviceErr.Code)
}
