package service

import (
	stdErrors "errors"
	"file-editor-server/internal/config"
	"file-editor-server/internal/errors"
	"file-editor-server/internal/filesystem"
	"file-editor-server/internal/lock"
	"file-editor-server/internal/models"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	defaultMaxLineCount      = 100000 // Default max line count if not specified in config (though config has a default)
	defaultMaxFilenameLength = 255
	maxEditsAllowed          = 1000
)

// FileOperationService defines the interface for file operations.
type FileOperationService interface {
	ReadFile(req models.ReadFileRequest) (*models.ReadFileResponse, *models.ErrorDetail)
	EditFile(req models.EditFileRequest) (*models.EditFileResponse, *models.ErrorDetail)
	ListFiles(req models.ListFilesRequest) (*models.ListFilesResponse, *models.ErrorDetail)
}

// DefaultFileOperationService implements the FileOperationService interface.
type DefaultFileOperationService struct {
	fsAdapter     filesystem.FileSystemAdapter
	lockManager   lock.LockManagerInterface
	workingDir    string
	maxFileSize   int64 // in bytes
	maxLineCount  int
	opTimeout     time.Duration
	filenameRegex *regexp.Regexp
}

// NewDefaultFileOperationService creates a new DefaultFileOperationService.
func NewDefaultFileOperationService(
	fs filesystem.FileSystemAdapter,
	lm lock.LockManagerInterface,
	cfg *config.Config,
) (*DefaultFileOperationService, error) {
	if cfg == nil {
		return nil, fmt.Errorf("configuration is required")
	}
	if fs == nil {
		return nil, fmt.Errorf("filesystem adapter is required")
	}
	if lm == nil {
		return nil, fmt.Errorf("lock manager is required")
	}

	// Validate working directory
	absWorkingDir, err := filepath.Abs(cfg.WorkingDirectory)
	if err != nil {
		return nil, fmt.Errorf("could not get absolute path for working directory: %w", err)
	}
	info, err := os.Stat(absWorkingDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("working directory does not exist: %s", absWorkingDir)
		}
		return nil, fmt.Errorf("error accessing working directory %s: %w", absWorkingDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("working directory path is not a directory: %s", absWorkingDir)
	}
	// Writability check is also good, but fsAdapter.IsWritable can be used by the service itself if needed.
	// The config validation already does a basic writability check.

	filenameRegex, err := regexp.Compile("^[a-zA-Z0-9._-]+$")
	if err != nil {
		// This should not happen with a static regex
		return nil, fmt.Errorf("failed to compile filename regex: %w", err)
	}

	maxLineCount := defaultMaxLineCount // Consider taking this from config if added there
	// For now, using a const. The problem desc implies it's a service-level concern.

	return &DefaultFileOperationService{
		fsAdapter:     fs,
		lockManager:   lm,
		workingDir:    absWorkingDir,
		maxFileSize:   int64(cfg.MaxFileSizeMB) * 1024 * 1024,
		maxLineCount:  maxLineCount,
		opTimeout:     time.Duration(cfg.OperationTimeoutSec) * time.Second,
		filenameRegex: filenameRegex,
	}, nil
}

func (s *DefaultFileOperationService) resolveAndValidatePath(filename string) (string, *models.ErrorDetail) {
	// Note: 'filename' here is the raw input, which is suitable for 'filename' field in error details.
	// 'operation' is 'path_resolution' for these validation errors.
	if !s.filenameRegex.MatchString(filename) {
		return "", errors.NewInvalidParamsError("Filename contains invalid characters.", map[string]interface{}{"filename": filename}, filename, "path_resolution")
	}
	if len(filename) == 0 || len(filename) > defaultMaxFilenameLength {
		return "", errors.NewInvalidParamsError(
			fmt.Sprintf("Filename length must be between 1 and %d characters.", defaultMaxFilenameLength),
			map[string]interface{}{"filename": filename, "length": fmt.Sprintf("%d", len(filename))},
			filename, "path_resolution",
		)
	}

	filePath := filepath.Join(s.workingDir, filename)
	cleanedPath := filepath.Clean(filePath)

	// Initial check for basic traversal before symlink evaluation
	if !strings.HasPrefix(cleanedPath, s.workingDir) {
		// This check might be redundant if filenameRegex is strict enough to disallow path chars,
		// but kept for defense in depth against 'filename' being crafted like 'symlink/../../foo'
		// if symlink itself points within workdir.
		return "", errors.NewInvalidParamsError("Path traversal attempt detected (pre-symlink).", map[string]interface{}{"filename": filename}, filename, "path_resolution")
	}

	resolvedPath, symlinkErr := s.fsAdapter.EvalSymlinks(cleanedPath)
	if symlinkErr != nil {
		// Handle errors from EvalSymlinks, e.g., if path doesn't exist after cleaning,
		// or if a component of the path used as a symlink target does not exist.
		// It could also be a permission error to EvalSymlinks.
		// Differentiate between "file not found" type errors vs. "permission" vs. "internal".
		// For now, treat as a file system error.
		// os.IsNotExist(symlinkErr) or os.IsPermission(symlinkErr) might be useful here.
		// Check if the error is because the file itself (or a part of its path) does not exist.
		// EvalSymlinks can return PathError.
		if underlyingErr := stdErrors.Unwrap(symlinkErr); underlyingErr != nil {
			if os.IsNotExist(underlyingErr) {
				// This means the original cleanedPath or a component of it does not exist.
				// This is not necessarily a symlink issue, but a general "file not found" for the given path.
				return "", errors.NewFileNotFoundError(filename, "eval_symlinks_path_not_found")
			}
			if os.IsPermission(underlyingErr) {
				return "", errors.NewPermissionDeniedError(filename, "eval_symlinks_permission")
			}
		}
		// For other errors (e.g. "too many links", "invalid path component")
		return "", errors.NewFileSystemError(filename, "eval_symlinks", fmt.Sprintf("Error evaluating symlinks: %v", symlinkErr))
	}

	// Validate that the fully resolved path is still within the working directory.
	if !strings.HasPrefix(resolvedPath, s.workingDir) {
		return "", errors.NewInvalidParamsError("Path traversal attempt detected (post-symlink).", map[string]interface{}{"filename": filename, "resolved_path": resolvedPath}, filename, "path_resolution")
	}
	// The path returned for actual file operations should still be `cleanedPath`.
	// The OS handles the symlink resolution during open/read/write.
	// The check `!strings.HasPrefix(resolvedPath, s.workingDir)` ensures that *if* `cleanedPath` is a symlink,
	// its ultimate target is confined. This is the correct interpretation.

	// Additional relative path check on cleanedPath (which is relative to s.workingDir effectively)
	rel, err := filepath.Rel(s.workingDir, cleanedPath)
	if err != nil {
		// This is an internal server error, not an invalid parameter from the user.
		return "", errors.NewInternalError(fmt.Sprintf("Error creating relative path for '%s': %v", filename, err))
	}
	if strings.HasPrefix(rel, "..") {
		// This case indicates that even after joining with workingDir and cleaning,
		// the path tries to go "up" from workingDir. This is a traversal attempt.
		return "", errors.NewInvalidParamsError("Path traversal attempt detected (relative path).", map[string]interface{}{"filename": filename}, filename, "path_resolution")
	}

	return cleanedPath, nil
}

// ReadFile implements the FileOperationService interface.
func (s *DefaultFileOperationService) ReadFile(req models.ReadFileRequest) (*models.ReadFileResponse, *models.ErrorDetail) {
	filePath, errDetail := s.resolveAndValidatePath(req.Name)
	if errDetail != nil {
		return nil, errDetail
	}

	// Operation for these validation errors is "read_validation"
	if (req.StartLine != 0 && req.StartLine < 1) || (req.EndLine != 0 && req.EndLine < 1) {
		return nil, errors.NewInvalidParamsError("Line numbers must be 1 or greater if specified.", map[string]interface{}{"start_line": req.StartLine, "end_line": req.EndLine}, req.Name, "read_validation")
	}
	if req.StartLine > 0 && req.EndLine > 0 && req.StartLine > req.EndLine {
		return nil, errors.NewInvalidParamsError("start_line cannot be greater than end_line.", map[string]interface{}{"start_line": req.StartLine, "end_line": req.EndLine}, req.Name, "read_validation")
	}

	exists, err := s.fsAdapter.FileExists(filePath)
	if err != nil {
		// Try to unwrap the error to check for specific types like permission errors.
		underlyingErr := err
		// Loop to unwrap multiple layers if necessary, though os.IsPermission should handle it.
		for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
			underlyingErr = unwrapped
		}
		if os.IsPermission(underlyingErr) {
			return nil, errors.NewPermissionDeniedError(req.Name, "check_exists")
		}
		return nil, errors.NewFileSystemError(req.Name, "check_exists", fmt.Sprintf("Error checking file existence: %v", err))
	}
	if !exists {
		return nil, errors.NewFileNotFoundError(req.Name, "read")
	}

	stats, err := s.fsAdapter.GetFileStats(filePath)
	if err != nil {
		underlyingErr := err
		for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
			underlyingErr = unwrapped
		}
		if os.IsPermission(underlyingErr) {
			return nil, errors.NewPermissionDeniedError(req.Name, "get_stats")
		}
		return nil, errors.NewFileSystemError(req.Name, "get_stats", fmt.Sprintf("Error getting file stats: %v", err))
	}
	if stats.IsDir {
		return nil, errors.NewInvalidParamsError(fmt.Sprintf("Path '%s' is a directory, not a file.", req.Name), map[string]interface{}{"filename": req.Name}, req.Name, "read_validation")
	}
	if stats.Size > s.maxFileSize {
		return nil, errors.NewFileTooLargeError(req.Name, "read", stats.Size, int(s.maxFileSize/(1024*1024)))
	}

	fileContent, err := s.fsAdapter.ReadFileBytes(filePath)
	if err != nil {
		underlyingErr := err
		for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
			underlyingErr = unwrapped
		}
		if os.IsPermission(underlyingErr) {
			return nil, errors.NewPermissionDeniedError(req.Name, "read_bytes")
		}
		return nil, errors.NewFileSystemError(req.Name, "read_bytes", fmt.Sprintf("Error reading file content: %v", err))
	}

	if !s.fsAdapter.IsValidUTF8(fileContent) {
		return nil, errors.NewInvalidEncodingError(req.Name, "read", "File content is not valid UTF-8")
	}

	lines := s.fsAdapter.SplitLines(fileContent)
	totalLineCount := len(lines)

	if totalLineCount > s.maxLineCount {
		return nil, errors.NewInvalidParamsError(fmt.Sprintf("File exceeds maximum line count of %d.", s.maxLineCount),
			map[string]interface{}{"filename": req.Name, "line_count": totalLineCount, "max_line_count": s.maxLineCount}, req.Name, "read_validation")
	}

	startLine := req.StartLine
	endLine := req.EndLine

	if startLine == 0 && endLine == 0 {
		startLine = 1
		endLine = totalLineCount
	} else if startLine == 0 {
		startLine = 1
	} else if endLine == 0 {
		endLine = totalLineCount
	}

	if startLine > totalLineCount && totalLineCount > 0 {
		return nil, errors.NewInvalidParamsError(
			fmt.Sprintf("start_line %d is greater than total lines %d.", startLine, totalLineCount),
			map[string]interface{}{"filename": req.Name, "start_line": startLine, "total_lines": totalLineCount},
			req.Name, "read_validation",
		)
	}
	if totalLineCount == 0 && startLine > 1 {
		return nil, errors.NewInvalidParamsError(
			fmt.Sprintf("start_line %d is invalid for an empty file.", startLine),
			map[string]interface{}{"filename": req.Name, "start_line": startLine, "total_lines": totalLineCount},
			req.Name, "read_validation",
		)
	}
	if totalLineCount == 0 && startLine == 1 {
		endLine = 1
	}

	if endLine > totalLineCount {
		endLine = totalLineCount
	}
	// At this point, startLine and endLine hold the final semantic range.
	// For empty file, full request: startLine=1, endLine=0.
	// For 1-line file, full request: startLine=1, endLine=1.

	responseStartLine := startLine
	responseEndLine := endLine

	var selectedLines []string
	if totalLineCount == 0 {
		selectedLines = []string{}
	} else {
		// Adjust for 0-based slicing.
		// For startLine=1, endLine=0 (empty file semantic), this should result in lines[0:0]
		sliceStartIdx := responseStartLine - 1
		sliceEndIdx := responseEndLine

		if sliceStartIdx < 0 { // Should not happen if startLine is always >= 1 for non-empty
			sliceStartIdx = 0
		}
		if sliceEndIdx > totalLineCount { // Should not happen if endLine is capped
			sliceEndIdx = totalLineCount
		}
		// If semantic range was 1-0 (empty file), sliceStartIdx=0, sliceEndIdx=0. Correct.
		if sliceStartIdx > sliceEndIdx {
			selectedLines = []string{} // e.g. startLine=1, endLine=0 -> sliceStartIdx=0, sliceEndIdx=0
		} else {
			selectedLines = lines[sliceStartIdx:sliceEndIdx]
		}
	}

	resultContent := s.fsAdapter.JoinLinesWithNewlines(selectedLines)

	respRange := models.RangeRequested{
		StartLine: responseStartLine,
		EndLine:   responseEndLine,
	}

	return &models.ReadFileResponse{
		Content:        string(resultContent),
		TotalLines:     totalLineCount,
		RangeRequested: &respRange,
	}, nil
}

// EditFile implements the FileOperationService interface.
func (s *DefaultFileOperationService) EditFile(req models.EditFileRequest) (*models.EditFileResponse, *models.ErrorDetail) {
	filePath, errDetail := s.resolveAndValidatePath(req.Name)
	if errDetail != nil {
		return nil, errDetail
	}

	if len(req.Edits) > maxEditsAllowed {
		return nil, errors.NewInvalidParamsError(
			fmt.Sprintf("Number of edits exceeds maximum allowed of %d.", maxEditsAllowed),
			map[string]interface{}{"num_edits": len(req.Edits), "max_edits": maxEditsAllowed},
			req.Name, "edit_validation",
		)
	}

	// Operation for these validation errors is "edit_validation"
	for i, edit := range req.Edits {
		if edit.Line < 1 {
			return nil, errors.NewInvalidParamsError(fmt.Sprintf("Edit operation #%d: line number must be 1 or greater.", i+1),
				map[string]interface{}{"edit_index": i, "line": edit.Line}, req.Name, "edit_validation")
		}
		op := strings.ToLower(edit.Operation)
		if op != "replace" && op != "insert" && op != "delete" {
			return nil, errors.NewInvalidParamsError(fmt.Sprintf("Edit operation #%d: invalid operation '%s'. Must be 'replace', 'insert', or 'delete'.", i+1, edit.Operation),
				map[string]interface{}{"edit_index": i, "operation": edit.Operation}, req.Name, "edit_validation")
		}
		req.Edits[i].Operation = op

		if op == "delete" && edit.Content != "" {
			return nil, errors.NewInvalidParamsError(fmt.Sprintf("Edit operation #%d ('delete'): content must be empty.", i+1),
				map[string]interface{}{"edit_index": i}, req.Name, "edit_validation")
		}
		if (op == "insert" || op == "replace") && !utf8.ValidString(edit.Content) {
			return nil, errors.NewInvalidParamsError(fmt.Sprintf("Edit operation #%d: content contains invalid UTF-8 encoding.", i+1),
				map[string]interface{}{"edit_index": i}, req.Name, "edit_validation")
		}
	}
	if req.Append != "" && !utf8.ValidString(req.Append) {
		return nil, errors.NewInvalidParamsError("Append content contains invalid UTF-8 encoding.", nil, req.Name, "edit_validation")
	}

	lockErr := s.lockManager.AcquireLock(req.Name, s.opTimeout)
	if lockErr != nil {
		return nil, errors.NewOperationLockFailedError(req.Name, "edit", lockErr.Error())
	}
	defer func() {
		if err := s.lockManager.ReleaseLock(req.Name); err != nil {
			fmt.Printf("Error releasing lock for file '%s' in defer: %v\n", req.Name, err)
		}
	}()

	var lines []string
	var fileCreated bool
	var originalLineCount int

	fileExists, fsErr := s.fsAdapter.FileExists(filePath)
	if fsErr != nil {
		underlyingErr := fsErr
		for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
			underlyingErr = unwrapped
		}
		if os.IsPermission(underlyingErr) {
			return nil, errors.NewPermissionDeniedError(req.Name, "check_exists_on_edit")
		}
		return nil, errors.NewFileSystemError(req.Name, "check_exists_on_edit", fmt.Sprintf("Error checking file existence: %v", fsErr))
	}

	if fileExists {
		stats, statErr := s.fsAdapter.GetFileStats(filePath)
		if statErr != nil {
			underlyingErr := statErr
			for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
				underlyingErr = unwrapped
			}
			if os.IsPermission(underlyingErr) {
				return nil, errors.NewPermissionDeniedError(req.Name, "get_stats_on_edit")
			}
			return nil, errors.NewFileSystemError(req.Name, "get_stats_on_edit", fmt.Sprintf("Error getting file stats: %v", statErr))
		}
		if stats.IsDir {
			return nil, errors.NewInvalidParamsError(fmt.Sprintf("Path '%s' is a directory, not a file.", req.Name), map[string]interface{}{"filename": req.Name}, req.Name, "edit_validation")
		}
		if stats.Size > s.maxFileSize {
			return nil, errors.NewFileTooLargeError(req.Name, "read_for_edit", stats.Size, int(s.maxFileSize/(1024*1024)))
		}

		fileContent, readErr := s.fsAdapter.ReadFileBytes(filePath)
		if readErr != nil {
			underlyingErr := readErr
			for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
				underlyingErr = unwrapped
			}
			if os.IsPermission(underlyingErr) {
				return nil, errors.NewPermissionDeniedError(req.Name, "read_bytes_on_edit")
			}
			return nil, errors.NewFileSystemError(req.Name, "read_bytes_on_edit", fmt.Sprintf("Error reading file content: %v", readErr))
		}
		if !s.fsAdapter.IsValidUTF8(fileContent) {
			return nil, errors.NewInvalidEncodingError(req.Name, "read_for_edit", "File content is not valid UTF-8")
		}
		lines = s.fsAdapter.SplitLines(fileContent)
		fileCreated = false
	} else {
		if !req.CreateIfMissing {
			return nil, errors.NewFileNotFoundError(req.Name, "edit")
		}
		lines = []string{}
		fileCreated = true
	}
	originalLineCount = len(lines)

	if !fileCreated && originalLineCount > s.maxLineCount {
		return nil, errors.NewInvalidParamsError(fmt.Sprintf("File exceeds maximum line count of %d before edits.", s.maxLineCount),
			map[string]interface{}{"filename": req.Name, "line_count": originalLineCount, "max_line_count": s.maxLineCount}, req.Name, "edit_validation")
	}

	sortedEdits := make([]models.EditOperation, len(req.Edits))
	copy(sortedEdits, req.Edits)
	sort.SliceStable(sortedEdits, func(i, j int) bool {
		return sortedEdits[i].Line > sortedEdits[j].Line
	})

	// linesModifiedCount := 0 // This specific counter was causing "declared and not used"

	for _, edit := range sortedEdits {
		lineIndex := edit.Line - 1
		currentLineCount := len(lines)
		// opChangedLineCount := false // This was also unused

		switch edit.Operation {
		case "replace":
			if lineIndex < 0 || lineIndex >= currentLineCount {
				return nil, errors.NewInvalidParamsError(
					fmt.Sprintf("Edit 'replace': line %d is out of range (1-%d).", edit.Line, currentLineCount),
					map[string]interface{}{"filename": req.Name, "line": edit.Line, "total_lines": currentLineCount}, req.Name, "edit_validation")
			}
			if lines[lineIndex] != edit.Content {
				lines[lineIndex] = edit.Content
				// linesModifiedCount++ // Removed this to use overall diff later
			}
		case "insert":
			if lineIndex < 0 || lineIndex > currentLineCount {
				return nil, errors.NewInvalidParamsError(
					fmt.Sprintf("Edit 'insert': line %d is out of range (1-%d allow insert at %d).", edit.Line, currentLineCount, currentLineCount+1),
					map[string]interface{}{"filename": req.Name, "line": edit.Line, "total_lines": currentLineCount}, req.Name, "edit_validation")
			}
			lines = append(lines[:lineIndex], append([]string{edit.Content}, lines[lineIndex:]...)...)
			// opChangedLineCount = true
		case "delete":
			if currentLineCount == 0 {
				return nil, errors.NewInvalidParamsError(
					fmt.Sprintf("Edit 'delete': line %d is out of range, file is empty.", edit.Line),
					map[string]interface{}{"filename": req.Name, "line": edit.Line, "total_lines": currentLineCount}, req.Name, "edit_validation")
			}
			if lineIndex < 0 || lineIndex >= currentLineCount {
				return nil, errors.NewInvalidParamsError(
					fmt.Sprintf("Edit 'delete': line %d is out of range (1-%d).", edit.Line, currentLineCount),
					map[string]interface{}{"filename": req.Name, "line": edit.Line, "total_lines": currentLineCount}, req.Name, "edit_validation")
			}
			lines = append(lines[:lineIndex], lines[lineIndex+1:]...)
			// opChangedLineCount = true
		}
		// if opChangedLineCount {} // temp use
	}

	// linesAfterEditsCount := len(lines) // This was unused

	if req.Append != "" {
		appendLines := s.fsAdapter.SplitLines([]byte(req.Append))
		// The empty "if" branch that was here previously (SA9003) has been removed.
		// The append operation will now always apply if req.Append is not empty,
		// which seems to be the original intent after removing prior empty conditional branches.
		lines = append(lines, appendLines...)
	}

	newTotalLines := len(lines)

	if newTotalLines > s.maxLineCount {
		return nil, errors.NewInvalidParamsError(
			fmt.Sprintf("Edit results in file exceeding maximum line count of %d (new count: %d).", s.maxLineCount, newTotalLines),
			map[string]interface{}{"filename": req.Name, "new_line_count": newTotalLines, "max_line_count": s.maxLineCount}, req.Name, "edit_validation")
	}

	finalContentBytes := s.fsAdapter.JoinLinesWithNewlines(lines)
	if int64(len(finalContentBytes)) > s.maxFileSize {
		return nil, errors.NewFileTooLargeError(req.Name, "edit_write", int64(len(finalContentBytes)), int(s.maxFileSize/(1024*1024)))
	}

	writeErr := s.fsAdapter.WriteFileBytesAtomic(filePath, finalContentBytes, 0644)
	if writeErr != nil {
		underlyingErr := writeErr
		for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
			underlyingErr = unwrapped
		}
		if os.IsPermission(underlyingErr) {
			return nil, errors.NewPermissionDeniedError(req.Name, "write_atomic")
		}
		return nil, errors.NewFileSystemError(req.Name, "write_atomic", fmt.Sprintf("Error writing file: %v", writeErr))
	}

	var finalLinesModified int
	if fileCreated {
		finalLinesModified = newTotalLines
	} else {
		finalLinesModified = abs(newTotalLines - originalLineCount)
	}

	return &models.EditFileResponse{
		Success:       true,
		LinesModified: finalLinesModified,
		FileCreated:   fileCreated,
		NewTotalLines: newTotalLines,
	}, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ListFiles implements the FileOperationService interface.
func (s *DefaultFileOperationService) ListFiles(req models.ListFilesRequest) (*models.ListFilesResponse, *models.ErrorDetail) {
	// Request object is currently empty, so no params to validate from req itself.

	dirEntries, err := s.fsAdapter.ListDir(s.workingDir)
	if err != nil {
		// This could be permission denied on workingDir itself, or other errors.
		underlyingErr := err
		for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
			underlyingErr = unwrapped
		}
		if os.IsPermission(underlyingErr) {
			return nil, errors.NewPermissionDeniedError(s.workingDir, "list_dir_working_dir")
		}
		return nil, errors.NewFileSystemError(s.workingDir, "list_dir", fmt.Sprintf("Failed to list directory: %v", err))
	}

	var files []models.FileInfo
	for _, entry := range dirEntries {
		if entry.IsDir || entry.IsHidden {
			continue // Skip directories and hidden files
		}

		// Basic info from ListDir
		fileInfo := models.FileInfo{
			Name:     entry.Name,
			Size:     entry.Size,
			Modified: entry.ModTime.UTC().Format(time.RFC3339), // RFC3339 with UTC 'Z'
			Readable: (entry.Mode & 0400) != 0,                 // Owner read permission
			Writable: (entry.Mode & 0200) != 0,                 // Owner write permission
			Lines:    -1,                                       // Default to -1 (unknown/error)
		}

		// Determine line count
		if entry.Size == 0 {
			fileInfo.Lines = 0 // Empty file has 0 lines (or 1 empty line "" depending on strict interpretation)
			// Spec for ReadFile with empty file returns 0 lines and "" content.
			// If SplitLines on "" returns [], then len is 0.
			// If SplitLines on "\n" returns [""], then len is 1.
			// Let's stick to len(SplitLines(content)) for consistency.
			// For a 0-byte file, ReadFileBytes returns [], SplitLines returns [], len is 0. Consistent.
		} else if entry.Size > s.maxFileSize {
			fileInfo.Lines = -1 // File too large, lines unknown
		} else {
			// Read file content to count lines
			filePath := filepath.Join(s.workingDir, entry.Name)
			content, readErr := s.fsAdapter.ReadFileBytes(filePath)
			if readErr != nil {
				// Log this error, but don't fail the whole ListFiles operation.
				// Lines will remain -1.
				// Example logging: log.Printf("ListFiles: Error reading file %s for line count: %v", entry.Name, readErr)
				// Based on error types, could set specific line counts (e.g. permission denied vs actual read error)
				// For now, any error in reading means -1.
				fileInfo.Lines = -1
			} else {
				if !s.fsAdapter.IsValidUTF8(content) {
					fileInfo.Lines = -1 // Not UTF-8, lines unknown
				} else {
					lines := s.fsAdapter.SplitLines(content)
					fileInfo.Lines = len(lines)
					if fileInfo.Lines > s.maxLineCount { // Double check, though size check is primary
						fileInfo.Lines = -1 // Exceeds max line count policy
					}
				}
			}
		}
		files = append(files, fileInfo)
	}

	// Sort files by name
	sort.Slice(files, func(i, j int) bool {
		return files[i].Name < files[j].Name
	})

	return &models.ListFilesResponse{
		Files:      files,
		TotalCount: len(files),
		Directory:  s.workingDir, // Return the absolute path of the listed directory
	}, nil
}
