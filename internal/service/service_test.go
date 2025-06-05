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
	files              map[string][]byte
	stats              map[string]*filesystem.FileStats
	existsShouldFail   bool
	readShouldFail     bool
	writeShouldFail    bool
	statsShouldFail    bool
	isWritableResult   bool
	isWritableShouldFail bool
	isValidUTF8Result  bool
	// Add more controls as needed
}

func newMockFsAdapter() *mockFileSystemAdapter {
	return &mockFileSystemAdapter{
		files:             make(map[string][]byte),
		stats:             make(map[string]*filesystem.FileStats),
		isValidUTF8Result: true, // Default to valid UTF-8
		isWritableResult:  true,
	}
}

func (m *mockFileSystemAdapter) ReadFileBytes(filePath string) ([]byte, error) {
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
	if m.isWritableShouldFail {return false, fmt.Errorf("mock isWritable error")}
	return m.isWritableResult, nil
}
func (m *mockFileSystemAdapter) IsValidUTF8(content []byte) bool { return m.isValidUTF8Result }
func (m *mockFileSystemAdapter) NormalizeNewlines(content []byte) []byte {
	s := strings.ReplaceAll(string(content), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return []byte(s)
}
func (m *mockFileSystemAdapter) SplitLines(content []byte) []string {
	normalized := m.NormalizeNewlines(content)
	sContent := string(normalized)
	if sContent == "" { return []string{} } // Consistent with actual adapter for empty
	lines := strings.Split(sContent, "\n")
	if len(lines) > 0 && strings.HasSuffix(sContent, "\n") {
		if sContent == "\n" { return []string{""} }
		if lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}
	return lines
}
func (m *mockFileSystemAdapter) JoinLinesWithNewlines(lines []string) []byte {
	return []byte(strings.Join(lines, "\n"))
}

// --- Mock LockManager ---
type mockLockManager struct {
	locksHeld        map[string]bool
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
func (m *mockLockManager) CleanupExpiredLocks()   {}


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
		WorkingDirectory:    tempWorkingDir,
		Transport:           "http",
		Port:                8080,
		MaxFileSizeMB:       1, // 1 MB for tests
		MaxConcurrentOps:    10,
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
	if err != nil { t.Fatalf("ReadFile failed: %v", err.Message) }

	expected := "l2\nl3\nl4"
	if resp.Content != expected { t.Errorf("Expected content %q, got %q", expected, resp.Content) }
	if resp.TotalLines != 5 { t.Errorf("Expected TotalLines 5, got %d", resp.TotalLines) }
	if resp.RangeRequested.StartLine != 2 || resp.RangeRequested.EndLine != 4 {
		t.Errorf("Expected RangeRequested 2-4, got %d-%d", resp.RangeRequested.StartLine, resp.RangeRequested.EndLine)
	}
}

func TestReadFile_Error_FileNotFound(t *testing.T) {
	service, _, _ := setup(t)
	defer cleanup(t)
	req := models.ReadFileRequest{Name: "nonexistent.txt"}
	_, err := service.ReadFile(req)
	if err == nil { t.Fatal("Expected error, got nil") }
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
	if err == nil { t.Fatal("Expected error, got nil") }
	if err.Code != errors.CodeFileTooLarge {
		t.Errorf("Expected CodeFileTooLarge (%d), got %d (%s)", errors.CodeFileTooLarge, err.Code, err.Message)
	}
}

func TestReadFile_Error_InvalidUTF8(t *testing.T) {
	service, mockFs, _ := setup(t)
	defer cleanup(t)
	filename := "invalidutf8.txt"
	fullPath := filepath.Join(tempWorkingDir, filename)
	mockFs.files[fullPath] = []byte{0xff, 0xfe, 0xfd} // Invalid UTF-8 sequence
	mockFs.stats[fullPath] = &filesystem.FileStats{Size: 3, IsDir: false}
	mockFs.isValidUTF8Result = false // Tell mock fsAdapter this is invalid

	req := models.ReadFileRequest{Name: filename}
	_, err := service.ReadFile(req)
	if err == nil { t.Fatal("Expected error, got nil") }
	if err.Code != errors.CodeInvalidParams {
		t.Errorf("Expected CodeInvalidParams (%d), got %d (%s)", errors.CodeInvalidParams, err.Code, err.Message)
	}
	if !strings.Contains(err.Message, "invalid UTF-8") {
		t.Errorf("Expected error message to contain 'invalid UTF-8', got '%s'", err.Message)
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
			if err == nil { t.Fatalf("Expected error for path %s, got nil", name) }
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
	if err == nil { t.Fatalf("Expected error for max line count, got nil") }
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
	if err != nil { t.Fatalf("EditFile failed: %v", err.Message) }

	if !resp.Success { t.Error("Expected Success to be true") }
	if !resp.FileCreated { t.Error("Expected FileCreated to be true") }
	if resp.NewTotalLines != 2 { t.Errorf("Expected NewTotalLines 2, got %d", resp.NewTotalLines) }
	// LinesModified is abs(NewTotalLines - OriginalTotalLines) -> for new file, Original is 0. So 2.
	if resp.LinesModified != 2 {t.Errorf("Expected LinesModified 2, got %d", resp.LinesModified) }

	fullPath := filepath.Join(tempWorkingDir, filename)
	finalContent, ok := mockFs.files[fullPath]
	if !ok { t.Fatal("File not found in mockFs after create") }
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
			{Line: 2, Operation: "replace", Content: "line two replaced"}, // Edits are 1-based
			{Line: 1, Operation: "delete"},      // Will be processed first due to reverse sort
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
	if err != nil { t.Fatalf("EditFile failed: %v", err.Message) }

	if !resp.Success { t.Error("Expected Success to be true") }
	if resp.FileCreated { t.Error("Expected FileCreated to be false") }
	if resp.NewTotalLines != 4 { t.Errorf("Expected NewTotalLines 4, got %d", resp.NewTotalLines) }
	// original 3, new 4. LinesModified = abs(4-3) = 1.
	if resp.LinesModified != 1 { t.Errorf("Expected LinesModified 1, got %d", resp.LinesModified) }

	finalContent, _ := mockFs.files[fullPath]
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
	if err == nil { t.Fatal("Expected error, got nil") }
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
	if err == nil { t.Fatal("Expected error, got nil") }
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
		Name: filename,
		Edits: []models.EditOperation{{Line: 5, Operation: "insert", Content: "fail"}},
	}
	_, err := service.EditFile(req)
	if err == nil { t.Fatal("Expected error, got nil") }
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
	if err == nil { t.Fatal("Expected error, got nil") }
	if err.Code != errors.CodeFileTooLarge {
		t.Errorf("Expected CodeFileTooLarge, got %d (%s)", err.Code, err.Message)
	}
}

func TestEditFile_Error_FilenameValidation(t *testing.T) {
	service, _, _ := setup(t)
	defer cleanup(t)
	req := models.EditFileRequest{Name: "inval*d.txt", CreateIfMissing: true}
	_, err := service.EditFile(req)
	if err == nil { t.Fatal("Expected error, got nil") }
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
    finalContent, _ := mockFs.files[fullPath]
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
	if !strings.Contains(err.Message, "out of range, file is empty"){
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

// Main entry point for tests to ensure cleanup runs
func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	// No global setup/cleanup needed here as individual tests handle it
	os.Exit(code)
}
