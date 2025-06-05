package service

import (
	"file-editor-server/internal/config"
	"file-editor-server/internal/errors"
	"file-editor-server/internal/filesystem"
	"file-editor-server/internal/lock"
	"file-editor-server/internal/models"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// --- Mock FileSystemAdapter ---
type mockFileSystemAdapter struct {
	files                map[string][]byte
	stats                map[string]*filesystem.FileStats
	existsShouldFail     bool
	readShouldFail       bool
	writeShouldFail      bool
	statsShouldFail      bool
	isWritableResult     bool
	isWritableShouldFail bool
	isValidUTF8Result    bool
	listDirShouldFail    bool                                 // New
	listDirEntries       map[string][]filesystem.DirEntryInfo // MODIFIED
	readFileErrorForPath map[string]error                     // New
	isInvalidUTF8Content map[string]bool                      // New
	// evalSymlinksPath      string             // REMOVED global evalSymlinksPath
	// evalSymlinksError     error              // REMOVED global evalSymlinksError
	evalSymlinksPaths        map[string]string // New: fromPath -> toPath
	evalSymlinksErrorForPath map[string]error  // New: fromPath -> error
	// Add more controls as needed
}

// mockDirEntryInfo is a helper struct to create filesystem.DirEntryInfo instances for tests.
// It is NOT an implementation of any interface, just a data holder.
type mockDirEntryInfo struct {
	name     string
	isDir    bool
	isHidden bool
	mode     os.FileMode
	modTime  time.Time
	size     int64
}

// toDirEntryInfo converts mockDirEntryInfo to filesystem.DirEntryInfo
func (mde mockDirEntryInfo) toDirEntryInfo() filesystem.DirEntryInfo {
	return filesystem.DirEntryInfo{
		Name:     mde.name,
		IsDir:    mde.isDir,
		IsHidden: mde.isHidden,
		Mode:     mde.mode,
		ModTime:  mde.modTime,
		Size:     mde.size,
	}
}

// mockFileInfo struct and its methods were removed as they were reported unused.
func newMockFsAdapter() *mockFileSystemAdapter {
	return &mockFileSystemAdapter{
		files:                    make(map[string][]byte),
		stats:                    make(map[string]*filesystem.FileStats),
		listDirEntries:           make(map[string][]filesystem.DirEntryInfo), // MODIFIED
		readFileErrorForPath:     make(map[string]error),                     // New
		isInvalidUTF8Content:     make(map[string]bool),                      // New
		isValidUTF8Result:        true,                                       // Default to valid UTF-8
		isWritableResult:         true,
		evalSymlinksPaths:        make(map[string]string), // New
		evalSymlinksErrorForPath: make(map[string]error),  // New
	}
}

func (m *mockFileSystemAdapter) ReadFileBytes(filePath string) ([]byte, error) {
	if err, specificError := m.readFileErrorForPath[filePath]; specificError { // New
		return nil, err
	}
	if m.readShouldFail {
		return nil, fmt.Errorf("mock read error")
	}
	content, ok := m.files[filePath]
	if !ok {
		return nil, os.ErrNotExist // Standard error for not found
	}
	return content, nil
}

func (m *mockFileSystemAdapter) WriteFileBytesAtomic(filePath string, content []byte, perm os.FileMode) error {
	if m.writeShouldFail {
		return fmt.Errorf("mock write error")
	}
	m.files[filePath] = content
	// Update stats after write
	m.stats[filePath] = &filesystem.FileStats{
		Size:    int64(len(content)),
		IsDir:   false,
		ModTime: time.Now(),
		Mode:    perm,
	}
	return nil
}

func (m *mockFileSystemAdapter) FileExists(filePath string) (bool, error) {
	if m.existsShouldFail {
		return false, fmt.Errorf("mock exists error")
	}
	_, ok := m.files[filePath]
	return ok, nil
}

func (m *mockFileSystemAdapter) GetFileStats(filePath string) (*filesystem.FileStats, error) {
	if m.statsShouldFail {
		return nil, fmt.Errorf("mock stats error")
	}
	s, ok := m.stats[filePath]
	if !ok {
		// If file exists in m.files but not m.stats, create basic stats
		content, fileOk := m.files[filePath]
		if fileOk {
			return &filesystem.FileStats{Size: int64(len(content)), IsDir: false, ModTime: time.Now()}, nil
		}
		return nil, os.ErrNotExist
	}
	return s, nil
}
func (m *mockFileSystemAdapter) IsWritable(path string) (bool, error) {
	if m.isWritableShouldFail {
		return false, fmt.Errorf("mock isWritable error")
	}
	return m.isWritableResult, nil
}
func (m *mockFileSystemAdapter) IsValidUTF8(content []byte) bool {
	if invalid, exists := m.isInvalidUTF8Content[string(content)]; exists && invalid { // New
		return false
	}
	return m.isValidUTF8Result
}
func (m *mockFileSystemAdapter) NormalizeNewlines(content []byte) []byte {
	s := strings.ReplaceAll(string(content), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return []byte(s)
}
func (m *mockFileSystemAdapter) SplitLines(content []byte) []string {
	normalized := m.NormalizeNewlines(content)
	sContent := string(normalized)
	if sContent == "" {
		return []string{}
	} // Consistent with actual adapter for empty
	lines := strings.Split(sContent, "\n")
	if len(lines) > 0 && strings.HasSuffix(sContent, "\n") {
		if sContent == "\n" {
			return []string{""}
		}
		if lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}
	return lines
}
func (m *mockFileSystemAdapter) JoinLinesWithNewlines(lines []string) []byte {
	return []byte(strings.Join(lines, "\n"))
}

func (m *mockFileSystemAdapter) ListDir(dirPath string) ([]filesystem.DirEntryInfo, error) { // MODIFIED Signature
	if m.listDirShouldFail {
		return nil, fmt.Errorf("mock ListDir error")
	}
	entries, ok := m.listDirEntries[dirPath]
	if !ok {
		return []filesystem.DirEntryInfo{}, nil
	}
	return entries, nil
}

func (m *mockFileSystemAdapter) EvalSymlinks(path string) (string, error) { // MODIFIED to use maps
	if err, ok := m.evalSymlinksErrorForPath[path]; ok {
		return "", err
	}
	if resolvedPath, ok := m.evalSymlinksPaths[path]; ok {
		return resolvedPath, nil
	}
	return path, nil // Default behavior: no symlink, path resolves to itself
}

// --- Mock LockManager ---
type mockLockManager struct {
	locksHeld         map[string]bool
	acquireShouldFail bool
	releaseShouldFail bool
}

func newMockLockManager() *mockLockManager {
	return &mockLockManager{locksHeld: make(map[string]bool)}
}
func (m *mockLockManager) AcquireLock(filename string, timeout time.Duration) error {
	if m.acquireShouldFail {
		return lock.ErrLockTimeout // Simulate a timeout
	}
	if m.locksHeld[filename] {
		return lock.ErrLockTimeout // Already locked by someone else in this mock
	}
	m.locksHeld[filename] = true
	return nil
}
func (m *mockLockManager) ReleaseLock(filename string) error {
	if m.releaseShouldFail {
		return fmt.Errorf("mock release error")
	}
	if !m.locksHeld[filename] {
		return lock.ErrLockNotFound
	}
	delete(m.locksHeld, filename)
	return nil
}
func (m *mockLockManager) GetCurrentLockCount() int { return len(m.locksHeld) }
func (m *mockLockManager) CleanupExpiredLocks()     {}

// --- Test Setup ---
var testConfig *config.Config
var tempWorkingDir string

func setup(t *testing.T) (*DefaultFileOperationService, *mockFileSystemAdapter, *mockLockManager) {
	var err error
	// Create a temporary working directory for tests
	tempWorkingDir, err = os.MkdirTemp("", "service_test_workdir_")
	if err != nil {
		t.Fatalf("Failed to create temp working dir: %v", err)
	}

	testConfig = &config.Config{
		WorkingDirectory: tempWorkingDir,
		Transport:        "http",
		Port:             8080,
		MaxFileSizeMB:    1, // 1 MB for tests
		// MaxConcurrentOps removed
		OperationTimeoutSec: 5,
	}

	mockFs := newMockFsAdapter()
	mockLm := newMockLockManager()

	service, err := NewDefaultFileOperationService(mockFs, mockLm, testConfig)
	if err != nil {
		t.Fatalf("Failed to create service: %v", err)
	}
	// Set a smaller max line count for easier testing
	service.maxLineCount = 100
	return service, mockFs, mockLm
}

func cleanup(t *testing.T) {
	if tempWorkingDir != "" {
		os.RemoveAll(tempWorkingDir)
	}
}

// --- ReadFile Tests ---
func TestReadFile_Success_FullRead(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)

	filename := "test.txt"
	content := "line1\nline2\nline3"
	fullPath := filepath.Join(tempWorkingDir, filename)
	mockFs.files[fullPath] = []byte(content)
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: int64(len(content)), IsDir: false}

	req := models.ReadFileRequest{Name: filename}
	resp, err := service.ReadFile(req)

	if err != nil {
		t.Fatalf("ReadFile failed: %v", err.Message)
	}
	if resp.Content != content {
		t.Errorf("Expected content %q, got %q", content, resp.Content)
	}
	if resp.TotalLines != 3 {
		t.Errorf("Expected TotalLines 3, got %d", resp.TotalLines)
	}
	if resp.RangeRequested.StartLine != 1 || resp.RangeRequested.EndLine != 3 {
		t.Errorf("Expected RangeRequested 1-3, got %d-%d", resp.RangeRequested.StartLine, resp.RangeRequested.EndLine)
	}
}

func TestReadFile_Success_PartialRead_StartOnly(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)
	filename := "partial.txt"
	content := "line1\nline2\nline3\nline4"
	fullPath := filepath.Join(tempWorkingDir, filename)
	mockFs.files[fullPath] = []byte(content)
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: int64(len(content)), IsDir: false}

	req := models.ReadFileRequest{Name: filename, StartLine: 3}
	resp, err := service.ReadFile(req)

	if err != nil {
		t.Fatalf("ReadFile failed: %v", err.Message)
	}
	expectedContent := "line3\nline4"
	if resp.Content != expectedContent {
		t.Errorf("Expected content %q, got %q", expectedContent, resp.Content)
	}
	if resp.TotalLines != 4 {
		t.Errorf("Expected TotalLines 4, got %d", resp.TotalLines)
	}
	if resp.RangeRequested.StartLine != 3 || resp.RangeRequested.EndLine != 4 {
		t.Errorf("Expected RangeRequested 3-4, got %d-%d", resp.RangeRequested.StartLine, resp.RangeRequested.EndLine)
	}
}

func TestReadFile_Success_PartialRead_StartAndEnd(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)
	filename := "partial_se.txt"
	content := "l1\nl2\nl3\nl4\nl5"
	fullPath := filepath.Join(tempWorkingDir, filename)
	mockFs.files[fullPath] = []byte(content)
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: int64(len(content)), IsDir: false}

	req := models.ReadFileRequest{Name: filename, StartLine: 2, EndLine: 4}
	resp, err := service.ReadFile(req)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err.Message)
	}

	expected := "l2\nl3\nl4"
	if resp.Content != expected {
		t.Errorf("Expected content %q, got %q", expected, resp.Content)
	}
	if resp.TotalLines != 5 {
		t.Errorf("Expected TotalLines 5, got %d", resp.TotalLines)
	}
	if resp.RangeRequested.StartLine != 2 || resp.RangeRequested.EndLine != 4 {
		t.Errorf("Expected RangeRequested 2-4, got %d-%d", resp.RangeRequested.StartLine, resp.RangeRequested.EndLine)
	}
}

func TestReadFile_Error_FileNotFound(t *testing.T) {
	service, _, _ := setup(t)
	defer cleanup(t)
	req := models.ReadFileRequest{Name: "nonexistent.txt"}
	_, err := service.ReadFile(req)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if err.Code != errors.CodeFileSystemError { // Mapped from NewFileNotFoundError
		t.Errorf("Expected CodeFileSystemError (%d), got %d (%s)", errors.CodeFileSystemError, err.Code, err.Message)
	}
}

func TestReadFile_Error_FileTooLarge(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)
	filename := "largefile.txt"
	fullPath := filepath.Join(tempWorkingDir, filename)
	mockFs.files[fullPath] = []byte("some content") // Content doesn't matter, stats do
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: int64(testConfig.MaxFileSizeMB*1024*1024 + 1), IsDir: false}

	req := models.ReadFileRequest{Name: filename}
	_, err := service.ReadFile(req)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if err.Code != errors.CodeFileSystemError { // MODIFIED: Check for generic file system error
		t.Errorf("Expected CodeFileSystemError (%d), got %d (%s)", errors.CodeFileSystemError, err.Code, err.Message)
	}
	// Optionally, check for the specific type in Data
	if dataMap, ok := err.Data.(map[string]interface{}); ok {
		if dataType, ok := dataMap["type"].(string); !ok || dataType != errors.CodeFileTooLargeType {
			t.Errorf("Expected error data type '%s', got '%s'", errors.CodeFileTooLargeType, dataType)
		}
	} else {
		t.Errorf("Expected error data to be a map[string]interface{} for type check")
	}
}

func TestReadFile_Error_InvalidUTF8(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)
	filename := "invalidutf8.txt"
	fullPath := filepath.Join(tempWorkingDir, filename)
	invalidContent := []byte{0xff, 0xfe, 0xfd} // Invalid UTF-8 sequence
	mockFs.files[fullPath] = invalidContent
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: int64(len(invalidContent)), IsDir: false}
	mockFs.isInvalidUTF8Content[string(invalidContent)] = true // MODIFIED: Use map

	req := models.ReadFileRequest{Name: filename}
	_, err := service.ReadFile(req)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if err.Code != errors.CodeFileSystemError { // MODIFIED: Expected CodeFileSystemError
		t.Errorf("Expected CodeFileSystemError (%d), got %d (%s)", errors.CodeFileSystemError, err.Code, err.Message)
	}

	// More robust check of error Data
	if dataMap, ok := err.Data.(map[string]interface{}); ok {
		if dataType, ok := dataMap["type"].(string); !ok || dataType != errors.CodeInvalidEncodingType {
			t.Errorf("Expected error data type '%s', got '%v'", errors.CodeInvalidEncodingType, dataMap["type"])
		}
		expectedDetails := "File content is not valid UTF-8"
		if dataDetails, ok := dataMap["details"].(string); !ok || dataDetails != expectedDetails {
			t.Errorf("Expected error data details '%s', got '%v'", expectedDetails, dataMap["details"])
		}
	} else {
		t.Errorf("Expected error Data to be a map[string]interface{}")
	}
}

func TestReadFile_Error_PathTraversal(t *testing.T) {
	service, _, _ := setup(t)
	defer cleanup(t)
	invalidNames := []string{"../file.txt", "dir/../../file.txt", "/etc/passwd"}
	for _, name := range invalidNames {
		t.Run(name, func(t *testing.T) {
			req := models.ReadFileRequest{Name: name}
			_, err := service.ReadFile(req)
			if err == nil {
				t.Fatalf("Expected error for path %s, got nil", name)
			}
			if err.Code != errors.CodeInvalidParams {
				t.Errorf("Expected CodeInvalidParams for %s, got %d (%s)", name, err.Code, err.Message)
			}
			if !strings.Contains(err.Message, "Path traversal") && !strings.Contains(err.Message, "Filename contains invalid characters") {
				t.Errorf("Expected traversal/invalid char error for %s, got %s", name, err.Message)
			}
		})
	}
}

func TestReadFile_Error_MaxLineCountExceeded(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)
	filename := "maxlines.txt"
	fullPath := filepath.Join(tempWorkingDir, filename)
	// Create content with more lines than service.maxLineCount (set to 100 in setup)
	var sb strings.Builder
	for i := 0; i < service.maxLineCount+5; i++ {
		sb.WriteString(fmt.Sprintf("line%d\n", i))
	}
	content := sb.String()
	mockFs.files[fullPath] = []byte(content)
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: int64(len(content)), IsDir: false}

	req := models.ReadFileRequest{Name: filename}
	_, err := service.ReadFile(req)
	if err == nil {
		t.Fatalf("Expected error for max line count, got nil")
	}
	if err.Code != errors.CodeInvalidParams {
		t.Errorf("Expected CodeInvalidParams, got %d (%s)", err.Code, err.Message)
	}
	if !strings.Contains(err.Message, "exceeds maximum line count") {
		t.Errorf("Expected 'exceeds maximum line count' in error, got: %s", err.Message)
	}
}

// --- EditFile Tests ---
func TestEditFile_Success_CreateFile(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)
	filename := "newfile.txt"
	req := models.EditFileRequest{
		Name:            filename,
		CreateIfMissing: true,
		Edits: []models.EditOperation{
			{Line: 1, Operation: "insert", Content: "hello world"},
		},
		Append: "new line", // Appending the second line
	}
	resp, err := service.EditFile(req)
	if err != nil {
		t.Fatalf("EditFile failed: %v", err.Message)
	}

	if !resp.Success {
		t.Error("Expected Success to be true")
	}
	if !resp.FileCreated {
		t.Error("Expected FileCreated to be true")
	}
	if resp.NewTotalLines != 2 {
		t.Errorf("Expected NewTotalLines 2, got %d", resp.NewTotalLines)
	}
	// LinesModified is abs(NewTotalLines - OriginalTotalLines) -> for new file, Original is 0. So 2.
	if resp.LinesModified != 2 {
		t.Errorf("Expected LinesModified 2, got %d", resp.LinesModified)
	}

	fullPath := filepath.Join(tempWorkingDir, filename)
	finalContent, ok := mockFs.files[fullPath]
	if !ok {
		t.Fatal("File not found in mockFs after create")
	}
	expectedContent := "hello world\nnew line"
	if string(finalContent) != expectedContent {
		t.Errorf("Expected file content %q, got %q", expectedContent, string(finalContent))
	}
}

func TestEditFile_Success_ModifyExisting(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)
	filename := "existing.txt"
	fullPath := filepath.Join(tempWorkingDir, filename)
	initialContent := "line one\nline two\nline three"
	mockFs.files[fullPath] = []byte(initialContent)
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: int64(len(initialContent)), IsDir: false}

	req := models.EditFileRequest{
		Name: filename,
		Edits: []models.EditOperation{
			{Line: 2, Operation: "replace", Content: "line two replaced"},         // Edits are 1-based
			{Line: 1, Operation: "delete"},                                        // Will be processed first due to reverse sort
			{Line: 3, Operation: "insert", Content: "inserted before line three"}, // Line 3 becomes line 2 after delete
		},
		Append: "appended line",
	}
	// Expected processing due to reverse sort by line:
	// 1. Delete line 1 ("line one") -> lines are: "line two\nline three"
	//    (Edit for line 3 now targets original "line three" which is at index 1)
	//    (Edit for line 2 now targets original "line two" which is at index 0)
	// After delete line 1:
	// Original line 2 ("line two") is now line 1.
	// Original line 3 ("line three") is now line 2.
	//
	// The line numbers in EditOperation are relative to the state of the file *before any edits in the current request*.
	// The service logic sorts edits by line number descending.
	// 1. Edit for Line 3 (insert "inserted before line three"):
	//    File: "line one\nline two\ninserted before line three\nline three" (4 lines)
	// 2. Edit for Line 2 (replace "line two" with "line two replaced"):
	//    File: "line one\nline two replaced\ninserted before line three\nline three" (4 lines)
	// 3. Edit for Line 1 (delete "line one"):
	//    File: "line two replaced\ninserted before line three\nline three" (3 lines)
	// 4. Append "appended line":
	//    File: "line two replaced\ninserted before line three\nline three\nappended line" (4 lines)

	resp, err := service.EditFile(req)
	if err != nil {
		t.Fatalf("EditFile failed: %v", err.Message)
	}

	if !resp.Success {
		t.Error("Expected Success to be true")
	}
	if resp.FileCreated {
		t.Error("Expected FileCreated to be false")
	}
	if resp.NewTotalLines != 4 {
		t.Errorf("Expected NewTotalLines 4, got %d", resp.NewTotalLines)
	}
	// original 3, new 4. LinesModified = abs(4-3) = 1.
	if resp.LinesModified != 1 {
		t.Errorf("Expected LinesModified 1, got %d", resp.LinesModified)
	}

	finalContent := mockFs.files[fullPath] // S1005: unnecessary assignment to the blank identifier
	expectedFinalContent := "line two replaced\ninserted before line three\nline three\nappended line"
	if string(finalContent) != expectedFinalContent {
		t.Errorf("Expected final content %q, got %q", expectedFinalContent, string(finalContent))
	}
}

func TestEditFile_Error_FileNotFound_NoCreate(t *testing.T) {
	service, _, _ := setup(t)
	defer cleanup(t)
	req := models.EditFileRequest{Name: "no_create.txt", CreateIfMissing: false}
	_, err := service.EditFile(req)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if err.Code != errors.CodeFileSystemError { // Mapped from NewFileNotFoundError
		t.Errorf("Expected CodeFileSystemError, got %d (%s)", err.Code, err.Message)
	}
}

func TestEditFile_Error_LockFailed(t *testing.T) {
	service, _, mockLm := setup(t)
	defer cleanup(t)
	mockLm.acquireShouldFail = true
	req := models.EditFileRequest{Name: "lockfail.txt", CreateIfMissing: true}
	_, err := service.EditFile(req)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if err.Code != errors.CodeOperationLockFailed {
		t.Errorf("Expected CodeOperationLockFailed, got %d (%s)", err.Code, err.Message)
	}
}

func TestEditFile_Error_EditLineOutOfRange(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)
	filename := "editrange.txt"
	fullPath := filepath.Join(tempWorkingDir, filename)
	mockFs.files[fullPath] = []byte("line1\nline2")
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: 11, IsDir: false}

	req := models.EditFileRequest{
		Name:  filename,
		Edits: []models.EditOperation{{Line: 5, Operation: "insert", Content: "fail"}},
	}
	_, err := service.EditFile(req)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if err.Code != errors.CodeInvalidParams {
		t.Errorf("Expected CodeInvalidParams, got %d (%s)", err.Code, err.Message)
	}
	if !strings.Contains(err.Message, "out of range") {
		t.Errorf("Expected 'out of range' in error, got: %s", err.Message)
	}
}

func TestEditFile_Error_ContentTooLargeAfterEdit(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)
	filename := "editlarge.txt"
	fullPath := filepath.Join(tempWorkingDir, filename)
	mockFs.files[fullPath] = []byte("small")
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: 5, IsDir: false}
	service.maxFileSize = 10 // Set very small max size for test

	longContent := string(make([]byte, 15)) // Content larger than maxFileSize

	req := models.EditFileRequest{
		Name: filename, CreateIfMissing: true, // Needs to be true for this test path if file doesn't exist
		Edits: []models.EditOperation{{Line: 1, Operation: "replace", Content: longContent}},
	}
	_, err := service.EditFile(req)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if err.Code != errors.CodeFileSystemError { // MODIFIED: Check for generic file system error
		t.Errorf("Expected CodeFileSystemError, got %d (%s)", errors.CodeFileSystemError, err.Message)
	}
	// Optionally, check for the specific type in Data
	if dataMap, ok := err.Data.(map[string]interface{}); ok {
		if dataType, ok := dataMap["type"].(string); !ok || dataType != errors.CodeFileTooLargeType {
			t.Errorf("Expected error data type '%s', got '%s'", errors.CodeFileTooLargeType, dataType)
		}
	} else {
		t.Errorf("Expected error data to be a map[string]interface{} for type check")
	}
}

func TestEditFile_Error_FilenameValidation(t *testing.T) {
	service, _, _ := setup(t)
	defer cleanup(t)
	req := models.EditFileRequest{Name: "inval*d.txt", CreateIfMissing: true}
	_, err := service.EditFile(req)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}
	if err.Code != errors.CodeInvalidParams {
		t.Errorf("Expected CodeInvalidParams, got %d (%s)", err.Code, err.Message)
	}
	if !strings.Contains(err.Message, "Filename contains invalid characters") {
		t.Errorf("Expected 'invalid characters' in error, got: %s", err.Message)
	}
}

func TestEditFile_Success_DeleteLastLine(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)
	filename := "delete_last.txt"
	fullPath := filepath.Join(tempWorkingDir, filename)
	initialContent := "line1\nline2"
	mockFs.files[fullPath] = []byte(initialContent)
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: int64(len(initialContent)), IsDir: false}

	req := models.EditFileRequest{
		Name: filename,
		Edits: []models.EditOperation{
			{Line: 2, Operation: "delete"},
		},
	}
	resp, err := service.EditFile(req)
	if err != nil {
		t.Fatalf("EditFile failed: %v", err.Message)
	}
	if resp.NewTotalLines != 1 {
		t.Errorf("Expected NewTotalLines 1, got %d", resp.NewTotalLines)
	}
	expectedContent := "line1"
	finalContent := mockFs.files[fullPath] // S1005: unnecessary assignment to the blank identifier
	if string(finalContent) != expectedContent {
		t.Errorf("Expected content %q, got %q", expectedContent, string(finalContent))
	}
}

func TestEditFile_Error_DeleteFromEmptyFile(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)
	filename := "empty_delete.txt"
	fullPath := filepath.Join(tempWorkingDir, filename)
	initialContent := "" // Empty file
	mockFs.files[fullPath] = []byte(initialContent)
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: 0, IsDir: false}

	req := models.EditFileRequest{
		Name: filename,
		Edits: []models.EditOperation{
			{Line: 1, Operation: "delete"},
		},
	}
	_, err := service.EditFile(req)
	if err == nil {
		t.Fatal("Expected error when deleting from empty file, got nil")
	}
	if err.Code != errors.CodeInvalidParams {
		t.Errorf("Expected CodeInvalidParams, got %d. Message: %s", err.Code, err.Message)
	}
	if !strings.Contains(err.Message, "out of range, file is empty") {
		t.Errorf("Expected 'file is empty' in error, got: %s", err.Message)
	}
}

func TestReadFile_EmptyFile(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)

	filename := "empty.txt"
	fullPath := filepath.Join(tempWorkingDir, filename)
	mockFs.files[fullPath] = []byte("")
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: 0, IsDir: false}

	req := models.ReadFileRequest{Name: filename}
	resp, err := service.ReadFile(req)

	if err != nil {
		t.Fatalf("ReadFile failed for empty file: %v", err.Message)
	}
	if resp.Content != "" {
		t.Errorf("Expected empty content for empty file, got %q", resp.Content)
	}
	if resp.TotalLines != 0 { // SplitLines on "" results in []string{} which is 0 lines
		t.Errorf("Expected TotalLines 0 for empty file, got %d", resp.TotalLines)
	}
	if resp.RangeRequested.StartLine != 1 || resp.RangeRequested.EndLine != 0 { // Default range for empty file
		t.Errorf("Expected RangeRequested 1-0 for empty file, got %d-%d", resp.RangeRequested.StartLine, resp.RangeRequested.EndLine)
	}
}

func TestReadFile_SingleNewlineFile(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)

	filename := "newline.txt"
	fullPath := filepath.Join(tempWorkingDir, filename)
	mockFs.files[fullPath] = []byte("\n") // File with a single newline
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: 1, IsDir: false}

	req := models.ReadFileRequest{Name: filename}
	resp, err := service.ReadFile(req)

	if err != nil {
		t.Fatalf("ReadFile failed for single newline file: %v", err.Message)
	}
	// SplitLines on "\n" results in {""} (one empty line)
	if resp.Content != "" { // JoinLinesWithNewlines on {""} is ""
		t.Errorf("Expected empty content for single newline file, got %q", resp.Content)
	}
	if resp.TotalLines != 1 {
		t.Errorf("Expected TotalLines 1 for single newline file, got %d", resp.TotalLines)
	}
	if resp.RangeRequested.StartLine != 1 || resp.RangeRequested.EndLine != 1 {
		t.Errorf("Expected RangeRequested 1-1 for single newline, got %d-%d", resp.RangeRequested.StartLine, resp.RangeRequested.EndLine)
	}
}

// --- ListFiles Tests ---

func TestListFiles_EmptyDirectory(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)

	// Configure ListDir to return an empty list for the root working directory
	mockFs.listDirEntries[tempWorkingDir] = []filesystem.DirEntryInfo{} // MODIFIED

	req := models.ListFilesRequest{} // MODIFIED: Path field removed
	resp, err := service.ListFiles(req)

	if err != nil {
		t.Fatalf("ListFiles failed: %v", err.Message)
	}

	if resp.TotalCount != 0 {
		t.Errorf("Expected TotalCount 0, got %d", resp.TotalCount)
	}
	if len(resp.Files) != 0 {
		t.Errorf("Expected Files to be empty, got %d items", len(resp.Files))
	}
}

func TestListFiles_WithFilesHiddenAndDirs(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)

	now := time.Now()
	anotherTxtContent := "some content"
	file1TxtContent := "more content\nnext line"

	// Use mockDirEntryInfo and toDirEntryInfo to populate listDirEntries
	mockFs.listDirEntries[tempWorkingDir] = []filesystem.DirEntryInfo{
		mockDirEntryInfo{name: "another.txt", size: int64(len(anotherTxtContent)), modTime: now, isDir: false, mode: 0o644, isHidden: false}.toDirEntryInfo(),
		mockDirEntryInfo{name: "file1.txt", size: int64(len(file1TxtContent)), modTime: now, isDir: false, mode: 0o644, isHidden: false}.toDirEntryInfo(),
		mockDirEntryInfo{name: ".hiddenfile", size: 10, modTime: now, isDir: false, mode: 0o644, isHidden: true}.toDirEntryInfo(),
		mockDirEntryInfo{name: "subdir", size: 0, modTime: now, isDir: true, mode: os.ModeDir | 0o755, isHidden: false}.toDirEntryInfo(),
	}

	// Mock file content for line counting
	pathAnother := filepath.Join(tempWorkingDir, "another.txt")
	pathFile1 := filepath.Join(tempWorkingDir, "file1.txt")
	mockFs.files[pathAnother] = []byte(anotherTxtContent)
	mockFs.files[pathFile1] = []byte(file1TxtContent)

	// Mock stats (needed by ListFiles internally for size and to skip large files)
	mockFs.stats[pathAnother] = &filesystem.FileStats{Size: int64(len(anotherTxtContent)), ModTime: now, IsDir: false, Mode: 0o644} // MODIFIED: Name field removed
	mockFs.stats[pathFile1] = &filesystem.FileStats{Size: int64(len(file1TxtContent)), ModTime: now, IsDir: false, Mode: 0o644}     // MODIFIED: Name field removed
	// No need to mock stats for .hiddenfile or subdir as they should be filtered out before stats are read by the tested logic

	req := models.ListFilesRequest{} // MODIFIED: Path field removed
	resp, err := service.ListFiles(req)

	if err != nil {
		t.Fatalf("ListFiles failed: %v", err.Message)
	}

	if resp.TotalCount != 2 {
		t.Errorf("Expected TotalCount 2, got %d", resp.TotalCount)
	}
	if len(resp.Files) != 2 {
		t.Fatalf("Expected 2 files in response, got %d", len(resp.Files))
	}

	// Check sorting and content
	if resp.Files[0].Name != "another.txt" {
		t.Errorf("Expected file[0] to be 'another.txt', got %s", resp.Files[0].Name)
	}
	if resp.Files[0].Lines != 1 { // "some content" is 1 line
		t.Errorf("Expected 'another.txt' to have 1 line, got %d", resp.Files[0].Lines)
	}

	if resp.Files[1].Name != "file1.txt" {
		t.Errorf("Expected file[1] to be 'file1.txt', got %s", resp.Files[1].Name)
	}
	if resp.Files[1].Lines != 2 { // "more content\nnext line" is 2 lines
		t.Errorf("Expected 'file1.txt' to have 2 lines, got %d", resp.Files[1].Lines)
	}
}

func TestListFiles_LineCounts(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)

	now := time.Now()
	// Use the service's actual maxFileSize for threshold, which is derived from config
	// This field is not exported, but for testing purposes, we are aware of its existence and meaning.
	// If the internal field name changes, this test would need an update.
	// A better way might be to use a known config value that translates to this, e.g. testConfig.MaxFileSizeMB * 1024 * 1024
	maxSizeForLineCountThreshold := service.maxFileSize // MODIFIED

	// Define file contents
	emptyContent := ""
	normalContent := "line1\nline2\nline3"
	tooLargeContent := "abc" // Content itself doesn't make it too large, its stat size will
	invalidUTF8ContentBytes := []byte{0xff, 0xfe, 0xfd}
	unreadableContent := "cannot read this"

	// Mock ListDir entries
	mockFs.listDirEntries[tempWorkingDir] = []filesystem.DirEntryInfo{ // MODIFIED
		mockDirEntryInfo{name: "empty.txt", size: int64(len(emptyContent)), modTime: now, isDir: false, mode: 0o644, isHidden: false}.toDirEntryInfo(),
		mockDirEntryInfo{name: "normal.txt", size: int64(len(normalContent)), modTime: now, isDir: false, mode: 0o644, isHidden: false}.toDirEntryInfo(),
		mockDirEntryInfo{name: "toolarge.txt", size: maxSizeForLineCountThreshold + 1, modTime: now, isDir: false, mode: 0o644, isHidden: false}.toDirEntryInfo(), // MODIFIED
		mockDirEntryInfo{name: "invalidutf8.txt", size: int64(len(invalidUTF8ContentBytes)), modTime: now, isDir: false, mode: 0o644, isHidden: false}.toDirEntryInfo(),
		mockDirEntryInfo{name: "unreadable_content.txt", size: int64(len(unreadableContent)), modTime: now, isDir: false, mode: 0o644, isHidden: false}.toDirEntryInfo(),
	}

	// Mock file contents in mockFs.files
	pathEmpty := filepath.Join(tempWorkingDir, "empty.txt")
	pathNormal := filepath.Join(tempWorkingDir, "normal.txt")
	pathTooLarge := filepath.Join(tempWorkingDir, "toolarge.txt") // Content not strictly needed as size check comes first
	pathInvalidUTF8 := filepath.Join(tempWorkingDir, "invalidutf8.txt")
	pathUnreadable := filepath.Join(tempWorkingDir, "unreadable_content.txt")

	mockFs.files[pathEmpty] = []byte(emptyContent)
	mockFs.files[pathNormal] = []byte(normalContent)
	mockFs.files[pathTooLarge] = []byte(tooLargeContent) // mock an actual file for GetFileStats if it tries to read it
	mockFs.files[pathInvalidUTF8] = invalidUTF8ContentBytes
	mockFs.files[pathUnreadable] = []byte(unreadableContent)

	// Mock stats (especially for toolarge.txt)
	mockFs.stats[pathEmpty] = &filesystem.FileStats{Size: int64(len(emptyContent)), ModTime: now, IsDir: false, Mode: 0o644}                  // MODIFIED: Name field removed
	mockFs.stats[pathNormal] = &filesystem.FileStats{Size: int64(len(normalContent)), ModTime: now, IsDir: false, Mode: 0o644}                // MODIFIED: Name field removed
	mockFs.stats[pathTooLarge] = &filesystem.FileStats{Size: maxSizeForLineCountThreshold + 1, ModTime: now, IsDir: false, Mode: 0o644}       // MODIFIED: Name field removed and used threshold
	mockFs.stats[pathInvalidUTF8] = &filesystem.FileStats{Size: int64(len(invalidUTF8ContentBytes)), ModTime: now, IsDir: false, Mode: 0o644} // MODIFIED: Name field removed
	mockFs.stats[pathUnreadable] = &filesystem.FileStats{Size: int64(len(unreadableContent)), ModTime: now, IsDir: false, Mode: 0o644}        // MODIFIED: Name field removed

	// Mock specific behaviors
	mockFs.readFileErrorForPath[pathUnreadable] = fmt.Errorf("mock error reading unreadable_content.txt")
	mockFs.isInvalidUTF8Content[string(invalidUTF8ContentBytes)] = true

	req := models.ListFilesRequest{} // MODIFIED: Path field removed
	resp, err := service.ListFiles(req)

	if err != nil {
		t.Fatalf("ListFiles failed: %v", err.Message)
	}

	if resp.TotalCount != 5 {
		t.Errorf("Expected TotalCount 5, got %d", resp.TotalCount)
	}
	if len(resp.Files) != 5 {
		t.Fatalf("Expected 5 files in response, got %d", len(resp.Files))
	}

	// Create a map for easy lookup and verification
	results := make(map[string]models.FileInfo)
	for _, f := range resp.Files {
		results[f.Name] = f
	}

	expectedLines := map[string]int{
		"empty.txt":              0,
		"normal.txt":             3,
		"toolarge.txt":           -1,
		"invalidutf8.txt":        -1,
		"unreadable_content.txt": -1,
	}

	for name, expectedLineCount := range expectedLines {
		fileInfo, ok := results[name]
		if !ok {
			t.Errorf("Expected file %s in results, but not found", name)
			continue
		}
		if fileInfo.Lines != expectedLineCount { // MODIFIED: Removed int64 cast
			t.Errorf("File %s: expected %d lines, got %d lines", name, expectedLineCount, fileInfo.Lines)
		}
	}
}

// Main entry point for tests to ensure cleanup runs
func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	// No global setup/cleanup needed here as individual tests handle it
	os.Exit(code)
}

// --- UTF-8 Validation in EditFile Tests ---

func TestEditFile_Error_InvalidUTF8_InEditOperation_Replace(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)

	filename := "test_utf8_edit_replace.txt"
	fullPath := filepath.Join(tempWorkingDir, filename)
	invalidUTF8Content := string([]byte{0xff, 0xfe, 0xfd})

	// Setup an existing file
	mockFs.files[fullPath] = []byte("line1\nline2")
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: 11, IsDir: false, ModTime: time.Now()}

	req := models.EditFileRequest{
		Name: filename,
		Edits: []models.EditOperation{
			{Line: 1, Operation: "replace", Content: invalidUTF8Content},
		},
	}

	_, err := service.EditFile(req)

	if err == nil {
		t.Fatal("EditFile expected to fail for invalid UTF-8 in replace operation, but succeeded")
	}

	if err.Code != errors.CodeInvalidParams {
		t.Errorf("Expected error code %d (InvalidParams), got %d. Message: %s", errors.CodeInvalidParams, err.Code, err.Message)
	}
	if !strings.Contains(err.Message, "content contains invalid UTF-8 encoding") {
		t.Errorf("Expected error message to indicate invalid UTF-8 in edit content, but got: %s", err.Message)
	}
}

func TestEditFile_Error_InvalidUTF8_InEditOperation_Insert(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)

	filename := "test_utf8_edit_insert.txt"
	fullPath := filepath.Join(tempWorkingDir, filename)
	invalidUTF8Content := string([]byte{0xff, 0xfe, 0xfd})

	// Setup an existing file
	mockFs.files[fullPath] = []byte("line1\nline2")
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: 11, IsDir: false, ModTime: time.Now()}

	req := models.EditFileRequest{
		Name: filename,
		Edits: []models.EditOperation{
			{Line: 1, Operation: "insert", Content: invalidUTF8Content},
		},
	}

	_, err := service.EditFile(req)

	if err == nil {
		t.Fatal("EditFile expected to fail for invalid UTF-8 in insert operation, but succeeded")
	}

	if err.Code != errors.CodeInvalidParams {
		t.Errorf("Expected error code %d (InvalidParams), got %d. Message: %s", errors.CodeInvalidParams, err.Code, err.Message)
	}
	if !strings.Contains(err.Message, "content contains invalid UTF-8 encoding") {
		t.Errorf("Expected error message to indicate invalid UTF-8 in edit content, but got: %s", err.Message)
	}
}

func TestEditFile_Error_InvalidUTF8_InAppendContent(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)

	filename := "test_utf8_append.txt"
	fullPath := filepath.Join(tempWorkingDir, filename)
	invalidUTF8Content := string([]byte{0xff, 0xfe, 0xfd})

	// Setup an existing file (or allow creation, doesn't matter much as validation is pre-fs)
	mockFs.files[fullPath] = []byte("line1")
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: 5, IsDir: false, ModTime: time.Now()}

	req := models.EditFileRequest{
		Name:            filename,
		CreateIfMissing: true,
		Append:          invalidUTF8Content,
		Edits: []models.EditOperation{ // Can have valid edits or be empty
			{Line: 1, Operation: "replace", Content: "valid line"},
		},
	}

	_, err := service.EditFile(req)

	if err == nil {
		t.Fatal("EditFile expected to fail for invalid UTF-8 in append operation, but succeeded")
	}

	if err.Code != errors.CodeInvalidParams {
		t.Errorf("Expected error code %d (InvalidParams), got %d. Message: %s", errors.CodeInvalidParams, err.Code, err.Message)
	}
	if !strings.Contains(err.Message, "Append content contains invalid UTF-8 encoding") {
		t.Errorf("Expected error message to indicate invalid UTF-8 in append content, but got: %s", err.Message)
	}
}

// --- Symlink and Path Validation Tests ---

func TestReadFile_Symlink_Allowed(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)

	targetFilename := "target.txt"
	symlinkFilename := "symlink.txt"
	targetContent := "This is the target content."

	absTargetFile := filepath.Join(tempWorkingDir, targetFilename)
	absSymlinkFile := filepath.Join(tempWorkingDir, symlinkFilename)

	// Setup: symlink.txt -> target.txt (both within workingDir)
	mockFs.evalSymlinksPaths[absSymlinkFile] = absTargetFile

	// Mock file system state for the symlink path, as ReadFile operates on it.
	// The mock adapter needs to make the symlink path behave like its target for read/stat.
	mockFs.files[absSymlinkFile] = []byte(targetContent)
	mockFs.stats[absSymlinkFile] = &filesystem.FileStats{
		Size:    int64(len(targetContent)),
		IsDir:   false,
		ModTime: time.Now(),
		Mode:    0644,
	}
	// Actual target file should also exist in mock for completeness if any part of the code
	// (not current ReadFile directly after resolveAndValidatePath) tries to access it by its real name.
	mockFs.files[absTargetFile] = []byte(targetContent)
	mockFs.stats[absTargetFile] = &filesystem.FileStats{
		Size:    int64(len(targetContent)),
		IsDir:   false,
		ModTime: time.Now(),
		Mode:    0644,
	}

	req := models.ReadFileRequest{Name: symlinkFilename}
	resp, err := service.ReadFile(req)

	if err != nil {
		t.Fatalf("ReadFile failed: %v", err.Message)
	}
	if resp.Content != targetContent {
		t.Errorf("Expected content %q, got %q", targetContent, resp.Content)
	}
}

func TestReadFile_Symlink_Traversal_Denied(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)

	symlinkFilename := "symlink_outside.txt"
	absSymlinkFile := filepath.Join(tempWorkingDir, symlinkFilename)
	outsidePath := "/etc/passwd" // A path outside tempWorkingDir

	// Setup: symlink_outside.txt -> /etc/passwd
	mockFs.evalSymlinksPaths[absSymlinkFile] = outsidePath
	// No need to mock content for symlink_outside.txt or /etc/passwd,
	// as the operation should fail due to path traversal before any read attempt.
	// Stats for absSymlinkFile might be needed if FileExists is called on it before EvalSymlinks check fully denies.
	// However, resolveAndValidatePath should catch this.
	// Let's assume the symlink itself exists.
	mockFs.stats[absSymlinkFile] = &filesystem.FileStats{
		Size:    50, // Arbitrary size for the symlink file itself
		IsDir:   false,
		ModTime: time.Now(),
		Mode:    0777, // Symlinks often have 0777 mode
	}

	req := models.ReadFileRequest{Name: symlinkFilename}
	_, err := service.ReadFile(req)

	if err == nil {
		t.Fatal("ReadFile expected to fail for symlink traversal, but succeeded")
	}
	if err.Code != errors.CodeInvalidParams {
		t.Errorf("Expected error code %d (InvalidParams), got %d. Message: %s", errors.CodeInvalidParams, err.Code, err.Message)
	}
	if !strings.Contains(err.Message, "Path traversal attempt detected") {
		t.Errorf("Expected error message to contain 'Path traversal attempt detected', but got: %s", err.Message)
	}
}

func TestReadFile_FilenameTooLong(t *testing.T) {
	service, _, _ := setup(t) // mockFs might not be strictly needed if validation happens before fs ops
	defer cleanup(t)

	// defaultMaxFilenameLength is a const (255) in service package, not exported from service instance.
	// We use the known value here for the test.
	maxLength := 255
	longFilename := strings.Repeat("a", maxLength+1)

	req := models.ReadFileRequest{Name: longFilename}
	_, err := service.ReadFile(req)

	if err == nil {
		t.Fatalf("ReadFile expected to fail for filename too long, but succeeded")
	}
	if err.Code != errors.CodeInvalidParams {
		t.Errorf("Expected error code %d (InvalidParams), got %d. Message: %s", errors.CodeInvalidParams, err.Code, err.Message)
	}
	expectedMsgPart := fmt.Sprintf("Filename length must be between 1 and %d characters", maxLength)
	if !strings.Contains(err.Message, expectedMsgPart) {
		t.Errorf("Expected error message to contain '%s', but got: %s", expectedMsgPart, err.Message)
	}
}

func TestEditFile_FilenameTooLong(t *testing.T) {
	service, _, _ := setup(t)
	defer cleanup(t)

	maxLength := 255
	longFilename := strings.Repeat("b", maxLength+1)

	req := models.EditFileRequest{
		Name:  longFilename,
		Edits: []models.EditOperation{{Line: 1, Operation: "insert", Content: "test"}},
	}
	_, err := service.EditFile(req)

	if err == nil {
		t.Fatalf("EditFile expected to fail for filename too long, but succeeded")
	}
	if err.Code != errors.CodeInvalidParams {
		t.Errorf("Expected error code %d (InvalidParams), got %d. Message: %s", errors.CodeInvalidParams, err.Code, err.Message)
	}
	expectedMsgPart := fmt.Sprintf("Filename length must be between 1 and %d characters", maxLength)
	if !strings.Contains(err.Message, expectedMsgPart) {
		t.Errorf("Expected error message to contain '%s', but got: %s", expectedMsgPart, err.Message)
	}
}

func TestReadFile_Symlink_Dangling(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)

	symlinkFilename := "dangling_symlink.txt"
	absSymlinkFile := filepath.Join(tempWorkingDir, symlinkFilename)

	// Setup: dangling_symlink.txt -> nonexistent_target.txt
	// Mock EvalSymlinks to return an error that os.IsNotExist will catch
	simulatedError := fmt.Errorf("mock EvalSymlinks error: target does not exist: %w", os.ErrNotExist)
	mockFs.evalSymlinksErrorForPath[absSymlinkFile] = simulatedError

	// The symlink itself exists, so it might have stats.
	mockFs.stats[absSymlinkFile] = &filesystem.FileStats{
		Size:    20, // Arbitrary size for the symlink file
		IsDir:   false,
		ModTime: time.Now(),
		Mode:    0777,
	}

	req := models.ReadFileRequest{Name: symlinkFilename}
	_, err := service.ReadFile(req)

	if err == nil {
		t.Fatal("ReadFile expected to fail for dangling symlink, but succeeded")
	}

	// resolveAndValidatePath should convert os.ErrNotExist from EvalSymlinks
	// into a errors.NewFileNotFoundError, which has CodeFileSystemError.
	if err.Code != errors.CodeFileSystemError {
		t.Errorf("Expected error code %d (CodeFileSystemError), got %d. Message: %s", errors.CodeFileSystemError, err.Code, err.Message)
	}

	// Check for "file_not_found" type in error Data
	if dataMap, ok := err.Data.(map[string]interface{}); ok {
		if errorType, exists := dataMap["type"].(string); !exists || errorType != "file_not_found" {
			t.Errorf("Expected error data type 'file_not_found', got '%v'", dataMap["type"])
		}
		// Check that the operation indicates the context of eval_symlinks
		if operation, exists := dataMap["operation"].(string); !exists || operation != "eval_symlinks_path_not_found" {
			t.Errorf("Expected error data operation 'eval_symlinks_path_not_found', got '%v'", dataMap["operation"])
		}
	} else {
		t.Errorf("Expected error Data to be a map[string]interface{} for type check")
	}
}

func TestEditFile_Symlink_Allowed(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)

	targetFilename := "target.txt"
	symlinkFilename := "symlink.txt"
	initialTargetContent := "Initial target content."
	editContent := "New content after edit."

	absTargetFile := filepath.Join(tempWorkingDir, targetFilename)
	absSymlinkFile := filepath.Join(tempWorkingDir, symlinkFilename)

	// Setup: symlink.txt -> target.txt
	mockFs.evalSymlinksPaths[absSymlinkFile] = absTargetFile

	// Mock file system state for the symlink path
	mockFs.files[absSymlinkFile] = []byte(initialTargetContent)
	mockFs.stats[absSymlinkFile] = &filesystem.FileStats{
		Size:    int64(len(initialTargetContent)),
		IsDir:   false,
		ModTime: time.Now(),
		Mode:    0644,
	}
	// Actual target also exists
	mockFs.files[absTargetFile] = []byte(initialTargetContent)
	mockFs.stats[absTargetFile] = &filesystem.FileStats{
		Size:    int64(len(initialTargetContent)),
		IsDir:   false,
		ModTime: time.Now(),
		Mode:    0644,
	}

	req := models.EditFileRequest{
		Name: symlinkFilename,
		Edits: []models.EditOperation{
			{Line: 1, Operation: "replace", Content: editContent},
		},
	}
	resp, err := service.EditFile(req)

	if err != nil {
		t.Fatalf("EditFile failed: %v", err.Message)
	}
	if !resp.Success {
		t.Error("EditFile was not successful")
	}

	// Verify that the content of absSymlinkFile (which represents the target via OS behavior) was changed.
	// Our mock WriteFileBytesAtomic writes to the given path, so absSymlinkFile's content in mockFs.files will be updated.
	if string(mockFs.files[absSymlinkFile]) != editContent {
		t.Errorf("Expected content %q for symlink path, got %q", editContent, string(mockFs.files[absSymlinkFile]))
	}
	// Additionally, if the mock adapter were more sophisticated, it would update absTargetFile.
	// For this test, we assume the service correctly passed absSymlinkFile to WriteFileBytesAtomic.
}

func TestEditFile_Symlink_Traversal_Denied(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)

	symlinkFilename := "symlink_outside.txt"
	absSymlinkFile := filepath.Join(tempWorkingDir, symlinkFilename)
	outsidePath := "/etc/passwd"

	// Setup: symlink_outside.txt -> /etc/passwd
	mockFs.evalSymlinksPaths[absSymlinkFile] = outsidePath
	mockFs.stats[absSymlinkFile] = &filesystem.FileStats{Size: 50, IsDir: false, ModTime: time.Now(), Mode: 0777}

	req := models.EditFileRequest{
		Name: symlinkFilename,
		Edits: []models.EditOperation{
			{Line: 1, Operation: "insert", Content: "hacked"},
		},
		CreateIfMissing: false, // Ensure it doesn't try to create /etc/passwd
	}
	_, err := service.EditFile(req)

	if err == nil {
		t.Fatal("EditFile expected to fail for symlink traversal, but succeeded")
	}
	if err.Code != errors.CodeInvalidParams {
		t.Errorf("Expected error code %d (InvalidParams), got %d. Message: %s", errors.CodeInvalidParams, err.Code, err.Message)
	}
	if !strings.Contains(err.Message, "Path traversal attempt detected") {
		t.Errorf("Expected error message to contain 'Path traversal attempt detected', but got: %s", err.Message)
	}
}
