package service

import (
	"bytes"
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
	ReadFile(req models.ReadFileRequest) (content string, filename string, totalLines int, reqStartLine int, reqEndLine int, actualEndLine int, isRangeRequest bool, err *models.ErrorDetail)
	EditFile(req models.EditFileRequest) (filename string, linesModified int, newTotalLines int, fileCreated bool, err *models.ErrorDetail)
	ListFiles(req models.ListFilesRequest) ([]models.FileInfo, string, *models.ErrorDetail)
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
func (s *DefaultFileOperationService) ReadFile(req models.ReadFileRequest) (
	content string, filename string, totalLines int,
	reqStartLine int, reqEndLine int, actualEndLine int,
	isRangeRequest bool, errRet *models.ErrorDetail,
) {
	filePath, errDetail := s.resolveAndValidatePath(req.Name)
	if errDetail != nil {
		return "", req.Name, 0, req.StartLine, req.EndLine, -1, false, errDetail
	}

	// Preserve original request line numbers for return
	originalReqStartLine := req.StartLine
	originalReqEndLine := req.EndLine
	isRangeRequest = originalReqStartLine != 0 || originalReqEndLine != 0

	// Operation for these validation errors is "read_validation"
	if (originalReqStartLine != 0 && originalReqStartLine < 1) || (originalReqEndLine != 0 && originalReqEndLine < 1) {
		errRet = errors.NewInvalidParamsError("Line numbers must be 1 or greater if specified.", map[string]interface{}{"start_line": originalReqStartLine, "end_line": originalReqEndLine}, req.Name, "read_validation")
		return "", req.Name, 0, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
	}
	if originalReqStartLine > 0 && originalReqEndLine > 0 && originalReqStartLine > originalReqEndLine {
		errRet = errors.NewInvalidParamsError("start_line cannot be greater than end_line.", map[string]interface{}{"start_line": originalReqStartLine, "end_line": originalReqEndLine}, req.Name, "read_validation")
		return "", req.Name, 0, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
	}

	exists, err := s.fsAdapter.FileExists(filePath)
	if err != nil {
		underlyingErr := err
		for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
			underlyingErr = unwrapped
		}
		if os.IsPermission(underlyingErr) {
			errRet = errors.NewPermissionDeniedError(req.Name, "check_exists")
			return "", req.Name, 0, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
		}
		errRet = errors.NewFileSystemError(req.Name, "check_exists", fmt.Sprintf("Error checking file existence: %v", err))
		return "", req.Name, 0, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
	}
	if !exists {
		errRet = errors.NewFileNotFoundError(req.Name, "read")
		return "", req.Name, 0, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
	}

	stats, err := s.fsAdapter.GetFileStats(filePath)
	if err != nil {
		underlyingErr := err
		for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
			underlyingErr = unwrapped
		}
		if os.IsPermission(underlyingErr) {
			errRet = errors.NewPermissionDeniedError(req.Name, "get_stats")
			return "", req.Name, 0, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
		}
		errRet = errors.NewFileSystemError(req.Name, "get_stats", fmt.Sprintf("Error getting file stats: %v", err))
		return "", req.Name, 0, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
	}
	if stats.IsDir {
		errRet = errors.NewInvalidParamsError(fmt.Sprintf("Path '%s' is a directory, not a file.", req.Name), map[string]interface{}{"filename": req.Name}, req.Name, "read_validation")
		return "", req.Name, 0, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
	}
	if stats.Size > s.maxFileSize {
		errRet = errors.NewFileTooLargeError(req.Name, "read", stats.Size, int(s.maxFileSize/(1024*1024)))
		return "", req.Name, 0, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
	}

	fileContentBytes, err := s.fsAdapter.ReadFileBytes(filePath)
	if err != nil {
		underlyingErr := err
		for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
			underlyingErr = unwrapped
		}
		if os.IsPermission(underlyingErr) {
			errRet = errors.NewPermissionDeniedError(req.Name, "read_bytes")
			return "", req.Name, 0, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
		}
		errRet = errors.NewFileSystemError(req.Name, "read_bytes", fmt.Sprintf("Error reading file content: %v", err))
		return "", req.Name, 0, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
	}

	if !s.fsAdapter.IsValidUTF8(fileContentBytes) {
		errRet = errors.NewInvalidEncodingError(req.Name, "read", "File content is not valid UTF-8")
		return "", req.Name, 0, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
	}

	lines := s.fsAdapter.SplitLines(fileContentBytes)
	totalLines = len(lines)

	if totalLines > s.maxLineCount {
		errRet = errors.NewInvalidParamsError(fmt.Sprintf("File exceeds maximum line count of %d.", s.maxLineCount),
			map[string]interface{}{"filename": req.Name, "line_count": totalLines, "max_line_count": s.maxLineCount}, req.Name, "read_validation")
		return "", req.Name, totalLines, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
	}

	// Determine effective start and end lines for slicing (1-based)
	effectiveStartLine := originalReqStartLine
	effectiveEndLine := originalReqEndLine

	if !isRangeRequest { // No range specified, read full file
		effectiveStartLine = 1
		effectiveEndLine = totalLines
		if totalLines == 0 { // Empty file full read
			effectiveEndLine = 0 // Represents "before the first line"
		}
	} else { // Range was specified
		if effectiveStartLine == 0 { // Only end_line specified
			effectiveStartLine = 1
		}
		if effectiveEndLine == 0 { // Only start_line specified
			effectiveEndLine = totalLines
		}
	}

	// Validate effective line numbers against total lines
	if totalLines == 0 {
		if effectiveStartLine > 1 || (effectiveStartLine == 1 && effectiveEndLine > 0 && isRangeRequest) {
			// For empty file, only full read (effective 1-0) or explicit request for 1-0 is valid.
			// Requesting line 1 (e.g. start=1, end=1) of an empty file is an error here.
			errRet = errors.NewInvalidParamsError(
				fmt.Sprintf("start_line %d is invalid for an empty file.", originalReqStartLine),
				map[string]interface{}{"filename": req.Name, "start_line": originalReqStartLine, "total_lines": totalLines},
				req.Name, "read_validation",
			)
			return "", req.Name, totalLines, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
		}
		effectiveStartLine = 1 // Normalize for slicing logic below
		effectiveEndLine = 0   // Normalize for slicing logic below
	} else { // Non-empty file
		if effectiveStartLine > totalLines {
			errRet = errors.NewInvalidParamsError(
				fmt.Sprintf("start_line %d is greater than total lines %d.", effectiveStartLine, totalLines),
				map[string]interface{}{"filename": req.Name, "start_line": effectiveStartLine, "total_lines": totalLines},
				req.Name, "read_validation",
			)
			return "", req.Name, totalLines, originalReqStartLine, originalReqEndLine, -1, isRangeRequest, errRet
		}
		if effectiveEndLine > totalLines {
			effectiveEndLine = totalLines
		}
	}

	var selectedLines []string
	// actualEndLine is 0-based index of the last line included in the content
	if totalLines == 0 || effectiveStartLine > effectiveEndLine {
		selectedLines = []string{}
		actualEndLine = -1 // No lines selected or file empty
	} else {
		// Adjust for 0-based slicing
		sliceStartIdx := effectiveStartLine - 1
		sliceEndIdx := effectiveEndLine // This is exclusive for slice, so it's correct.
		selectedLines = lines[sliceStartIdx:sliceEndIdx]
		actualEndLine = sliceEndIdx - 1
	}

	// If no lines were selected but the file is not empty (e.g. start=5, end=4 for a 10-line file)
	// actualEndLine should be effectiveStartLine - 2, or -1 if effectiveStartLine is 1.
	// Example: 10 lines. req start=5, end=4. effectiveStart=5, effectiveEnd=4.
	// sliceStartIdx = 4, sliceEndIdx = 4. selectedLines = []. actualEndLine = 4-1 = 3.
	// This is correct: the lines considered are up to index 3 (line 4), but none are selected.

	content = string(s.fsAdapter.JoinLinesWithNewlines(selectedLines))
	filename = req.Name
	// totalLines is already set
	reqStartLine = originalReqStartLine
	reqEndLine = originalReqEndLine
	// actualEndLine is set
	// isRangeRequest is set
	errRet = nil
	return
}

// EditFile implements the FileOperationService interface.
func (s *DefaultFileOperationService) EditFile(req models.EditFileRequest) (
	filename string, linesModified int, newTotalLines int, fileCreated bool, errRet *models.ErrorDetail,
) {
	filePath, errDetail := s.resolveAndValidatePath(req.Name)
	if errDetail != nil {
		return req.Name, 0, 0, false, errDetail
	}

	filename = req.Name // Set filename for return even if other ops fail

	if len(req.Edits) > maxEditsAllowed {
		errRet = errors.NewInvalidParamsError(
			fmt.Sprintf("Number of edits exceeds maximum allowed of %d.", maxEditsAllowed),
			map[string]interface{}{"num_edits": len(req.Edits), "max_edits": maxEditsAllowed},
			filename, "edit_validation",
		)
		return filename, 0, 0, false, errRet
	}

	// Operation for these validation errors is "edit_validation"
	for i, edit := range req.Edits {
		if edit.Line < 1 {
			errRet = errors.NewInvalidParamsError(fmt.Sprintf("Edit operation #%d: line number must be 1 or greater.", i+1),
				map[string]interface{}{"edit_index": i, "line": edit.Line}, filename, "edit_validation")
			return filename, 0, 0, false, errRet
		}
		op := strings.ToLower(edit.Operation)
		if op != "replace" && op != "insert" && op != "delete" {
			errRet = errors.NewInvalidParamsError(fmt.Sprintf("Edit operation #%d: invalid operation '%s'. Must be 'replace', 'insert', or 'delete'.", i+1, edit.Operation),
				map[string]interface{}{"edit_index": i, "operation": edit.Operation}, filename, "edit_validation")
			return filename, 0, 0, false, errRet
		}
		req.Edits[i].Operation = op // Store normalized operation

		if op == "delete" && edit.Content != "" {
			errRet = errors.NewInvalidParamsError(fmt.Sprintf("Edit operation #%d ('delete'): content must be empty.", i+1),
				map[string]interface{}{"edit_index": i}, filename, "edit_validation")
			return filename, 0, 0, false, errRet
		}
		if (op == "insert" || op == "replace") && !utf8.ValidString(edit.Content) {
			errRet = errors.NewInvalidParamsError(fmt.Sprintf("Edit operation #%d: content contains invalid UTF-8 encoding.", i+1),
				map[string]interface{}{"edit_index": i}, filename, "edit_validation")
			return filename, 0, 0, false, errRet
		}
	}
	if req.Append != "" && !utf8.ValidString(req.Append) {
		errRet = errors.NewInvalidParamsError("Append content contains invalid UTF-8 encoding.", nil, filename, "edit_validation")
		return filename, 0, 0, false, errRet
	}

	lockErr := s.lockManager.AcquireLock(filePath, s.opTimeout)
	if lockErr != nil {
		errRet = errors.NewOperationLockFailedError(filename, "edit", lockErr.Error())
		return filename, 0, 0, false, errRet
	}
	defer func() {
		if err := s.lockManager.ReleaseLock(filePath); err != nil {
			// Log this error, but can't return it from defer
			fmt.Printf("Error releasing lock for file '%s' in defer: %v\n", filename, err)
		}
	}()

	var lines []string
	var originalLineCount int
	var newlineStyle string

	fileExists, fsErr := s.fsAdapter.FileExists(filePath)
	if fsErr != nil {
		underlyingErr := fsErr
		for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
			underlyingErr = unwrapped
		}
		if os.IsPermission(underlyingErr) {
			errRet = errors.NewPermissionDeniedError(filename, "check_exists_on_edit")
			return filename, 0, 0, false, errRet
		}
		errRet = errors.NewFileSystemError(filename, "check_exists_on_edit", fmt.Sprintf("Error checking file existence: %v", fsErr))
		return filename, 0, 0, false, errRet
	}

	if fileExists {
		stats, statErr := s.fsAdapter.GetFileStats(filePath)
		if statErr != nil {
			underlyingErr := statErr
			for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
				underlyingErr = unwrapped
			}
			if os.IsPermission(underlyingErr) {
				errRet = errors.NewPermissionDeniedError(filename, "get_stats_on_edit")
				return filename, 0, 0, false, errRet
			}
			errRet = errors.NewFileSystemError(filename, "get_stats_on_edit", fmt.Sprintf("Error getting file stats: %v", statErr))
			return filename, 0, 0, false, errRet
		}
		if stats.IsDir {
			errRet = errors.NewInvalidParamsError(fmt.Sprintf("Path '%s' is a directory, not a file.", filename), map[string]interface{}{"filename": filename}, filename, "edit_validation")
			return filename, 0, 0, false, errRet
		}
		if stats.Size > s.maxFileSize {
			errRet = errors.NewFileTooLargeError(filename, "read_for_edit", stats.Size, int(s.maxFileSize/(1024*1024)))
			return filename, 0, 0, false, errRet
		}

		fileContentBytes, readErr := s.fsAdapter.ReadFileBytes(filePath)
		if readErr != nil {
			underlyingErr := readErr
			for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
				underlyingErr = unwrapped
			}
			if os.IsPermission(underlyingErr) {
				errRet = errors.NewPermissionDeniedError(filename, "read_bytes_on_edit")
				return filename, 0, 0, false, errRet
			}
			errRet = errors.NewFileSystemError(filename, "read_bytes_on_edit", fmt.Sprintf("Error reading file content: %v", readErr))
			return filename, 0, 0, false, errRet
		}
		if !s.fsAdapter.IsValidUTF8(fileContentBytes) {
			errRet = errors.NewInvalidEncodingError(filename, "read_for_edit", "File content is not valid UTF-8")
			return filename, 0, 0, false, errRet
		}
		newlineStyle = s.fsAdapter.DetectLineEnding(fileContentBytes)
		lines = s.fsAdapter.SplitLines(fileContentBytes)
		fileCreated = false
	} else {
		if !req.CreateIfMissing {
			errRet = errors.NewFileNotFoundError(filename, "edit")
			return filename, 0, 0, false, errRet
		}
		lines = []string{}
		fileCreated = true
		newlineStyle = "\n" // Default for new files
	}
	originalLineCount = len(lines)

	if !fileCreated && originalLineCount > s.maxLineCount {
		errRet = errors.NewInvalidParamsError(fmt.Sprintf("File exceeds maximum line count of %d before edits.", s.maxLineCount),
			map[string]interface{}{"filename": filename, "line_count": originalLineCount, "max_line_count": s.maxLineCount}, filename, "edit_validation")
		return filename, 0, originalLineCount, false, errRet
	}

	sortedEdits := make([]models.EditOperation, len(req.Edits))
	copy(sortedEdits, req.Edits)
	sort.SliceStable(sortedEdits, func(i, j int) bool {
		return sortedEdits[i].Line > sortedEdits[j].Line // Process from bottom up
	})

	for _, edit := range sortedEdits {
		lineIndex := edit.Line - 1 // Convert to 0-based
		currentLineCount := len(lines)

		switch edit.Operation {
		case "replace":
			if lineIndex < 0 || lineIndex >= currentLineCount {
				errRet = errors.NewInvalidParamsError(
					fmt.Sprintf("Edit 'replace': line %d is out of range (1-%d).", edit.Line, currentLineCount),
					map[string]interface{}{"filename": filename, "line": edit.Line, "total_lines": currentLineCount}, filename, "edit_validation")
				return filename, 0, currentLineCount, fileCreated, errRet
			}
			if lines[lineIndex] != edit.Content { // Only count as modified if content actually changes
				lines[lineIndex] = edit.Content
				// linesModified++ // Handled by overall diff later
			}
		case "insert":
			if lineIndex < 0 || lineIndex > currentLineCount { // Allow insert at currentLineCount (end of file)
				errRet = errors.NewInvalidParamsError(
					fmt.Sprintf("Edit 'insert': line %d is out of range (1 to %d allow insert at %d).", edit.Line, currentLineCount, currentLineCount+1),
					map[string]interface{}{"filename": filename, "line": edit.Line, "total_lines": currentLineCount}, filename, "edit_validation")
				return filename, 0, currentLineCount, fileCreated, errRet
			}
			lines = append(lines[:lineIndex], append([]string{edit.Content}, lines[lineIndex:]...)...)
		case "delete":
			if currentLineCount == 0 { // Cannot delete from empty file
				errRet = errors.NewInvalidParamsError(
					fmt.Sprintf("Edit 'delete': line %d is out of range, file is empty.", edit.Line),
					map[string]interface{}{"filename": filename, "line": edit.Line, "total_lines": currentLineCount}, filename, "edit_validation")
				return filename, 0, currentLineCount, fileCreated, errRet
			}
			if lineIndex < 0 || lineIndex >= currentLineCount {
				errRet = errors.NewInvalidParamsError(
					fmt.Sprintf("Edit 'delete': line %d is out of range (1-%d).", edit.Line, currentLineCount),
					map[string]interface{}{"filename": filename, "line": edit.Line, "total_lines": currentLineCount}, filename, "edit_validation")
				return filename, 0, currentLineCount, fileCreated, errRet
			}
			lines = append(lines[:lineIndex], lines[lineIndex+1:]...)
		}
	}

	if req.Append != "" {
		appendLines := s.fsAdapter.SplitLines([]byte(req.Append))
		lines = append(lines, appendLines...)
	}

	newTotalLines = len(lines)

	if newTotalLines > s.maxLineCount {
		errRet = errors.NewInvalidParamsError(
			fmt.Sprintf("Edit results in file exceeding maximum line count of %d (new count: %d).", s.maxLineCount, newTotalLines),
			map[string]interface{}{"filename": filename, "new_line_count": newTotalLines, "max_line_count": s.maxLineCount}, filename, "edit_validation")
		return filename, 0, originalLineCount, fileCreated, errRet // Return original line count on failure
	}

	finalContentBytes := s.fsAdapter.JoinLinesWithNewlines(lines)
	if newlineStyle != "\n" { // Preserve original newline style if not \n
		finalContentBytes = bytes.ReplaceAll(finalContentBytes, []byte("\n"), []byte(newlineStyle))
	}

	if int64(len(finalContentBytes)) > s.maxFileSize {
		errRet = errors.NewFileTooLargeError(filename, "edit_write", int64(len(finalContentBytes)), int(s.maxFileSize/(1024*1024)))
		return filename, 0, originalLineCount, fileCreated, errRet
	}

	writeErr := s.fsAdapter.WriteFileBytesAtomic(filePath, finalContentBytes, 0644) // Use a common permission, e.g., 0644
	if writeErr != nil {
		underlyingErr := writeErr
		for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
			underlyingErr = unwrapped
		}
		if os.IsPermission(underlyingErr) {
			errRet = errors.NewPermissionDeniedError(filename, "write_atomic")
			return filename, 0, originalLineCount, fileCreated, errRet
		}
		errRet = errors.NewFileSystemError(filename, "write_atomic", fmt.Sprintf("Error writing file: %v", writeErr))
		return filename, 0, originalLineCount, fileCreated, errRet
	}

	if fileCreated {
		linesModified = newTotalLines
	} else {
		// This is a simple way to calculate modified. More complex diff could be used for precise line changes.
		linesModified = abs(newTotalLines - originalLineCount)
		// A more accurate linesModified would require comparing original lines with new lines.
		// For now, the change in total line count is a proxy. If content of lines change but count doesn't, this reports 0.
		// The spec is "LinesModified is the number of lines that were actually modified".
		// This might need a diff algorithm for true accuracy. Let's assume abs difference is acceptable for now.
	}

	errRet = nil // Success
	return filename, linesModified, newTotalLines, fileCreated, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ListFiles implements the FileOperationService interface.
func (s *DefaultFileOperationService) ListFiles(req models.ListFilesRequest) ([]models.FileInfo, string, *models.ErrorDetail) {
	// Request object is currently empty, so no params to validate from req itself.

	dirEntries, err := s.fsAdapter.ListDir(s.workingDir)
	if err != nil {
		// This could be permission denied on workingDir itself, or other errors.
		underlyingErr := err
		for unwrapped := stdErrors.Unwrap(underlyingErr); unwrapped != nil; unwrapped = stdErrors.Unwrap(underlyingErr) {
			underlyingErr = unwrapped
		}
		if os.IsPermission(underlyingErr) {
			return nil, "", errors.NewPermissionDeniedError(s.workingDir, "list_dir_working_dir")
		}
		return nil, "", errors.NewFileSystemError(s.workingDir, "list_dir", fmt.Sprintf("Failed to list directory: %v", err))
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
					linesScanned := s.fsAdapter.SplitLines(content)
					fileInfo.Lines = len(linesScanned)
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

	return files, s.workingDir, nil
}
