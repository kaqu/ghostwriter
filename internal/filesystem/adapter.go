package filesystem

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

// FileStats holds basic statistics about a file.
type FileStats struct {
	Size    int64
	IsDir   bool
	ModTime time.Time
	Mode    os.FileMode
}

// FileSystemAdapter defines an interface for interacting with the file system.
// This allows for easier testing and potential future extensions (e.g., virtual file systems).
type FileSystemAdapter interface {
	ReadFileBytes(filePath string) ([]byte, error)
	WriteFileBytesAtomic(filePath string, content []byte, perm os.FileMode) error
	FileExists(filePath string) (bool, error)
	GetFileStats(filePath string) (*FileStats, error)
	IsWritable(path string) (bool, error) // For directory writability check
	IsValidUTF8(content []byte) bool
	NormalizeNewlines(content []byte) []byte        // Converts \r\n and \r to \n
	SplitLines(content []byte) []string             // Uses normalized newlines
	JoinLinesWithNewlines(lines []string) []byte    // Uses \n
}

// CheckDirectoryIsWritable performs a robust check if a directory is writable.
func CheckDirectoryIsWritable(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("path does not exist: %s: %w", path, err)
		}
		return fmt.Errorf("could not stat path %s: %w", path, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("path is not a directory: %s", path)
	}

	// #nosec G404 -- rand is okay for temp file names
	tmpFileName := fmt.Sprintf("writable_test_%d_%d.tmp", time.Now().UnixNano(), rand.Intn(100000))
	tmpFilePath := filepath.Join(path, tmpFileName)

	file, err := os.Create(tmpFilePath)
	if err != nil {
		if os.IsPermission(err) {
			return fmt.Errorf("permission denied to write in directory %s: %w", path, err)
		}
		return fmt.Errorf("error creating temporary file in %s: %w", path, err)
	}
	_ = file.Close()
	errRemove := os.Remove(tmpFilePath)
	if errRemove != nil {
		// Log or handle this warning if necessary, but main check passed
		// For example: log.Printf("Warning: failed to remove temporary test file %s: %v", tmpFilePath, errRemove)
	}
	return nil
}

// DefaultFileSystemAdapter is the standard implementation of FileSystemAdapter using the os package.
type DefaultFileSystemAdapter struct {
	// WorkingDirectory could be stored here if needed for resolving relative paths,
	// but current methods assume filePath is either absolute or resolvable by the os package directly.
}

// NewDefaultFileSystemAdapter creates a new DefaultFileSystemAdapter.
func NewDefaultFileSystemAdapter() *DefaultFileSystemAdapter {
	return &DefaultFileSystemAdapter{}
}

// ReadFileBytes reads the entire file into a byte slice.
// Returns an error if the file doesn't exist or cannot be read.
func (fs *DefaultFileSystemAdapter) ReadFileBytes(filePath string) ([]byte, error) {
	content, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found: %s: %w", filePath, err)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied reading file: %s: %w", filePath, err)
		}
		return nil, fmt.Errorf("failed to read file: %s: %w", filePath, err)
	}
	return content, nil
}

// IsValidUTF8 checks if the byte slice is valid UTF-8.
func (fs *DefaultFileSystemAdapter) IsValidUTF8(content []byte) bool {
	return utf8.Valid(content)
}

// WriteFileBytesAtomic writes content to a file atomically.
// It writes to a temporary file first with 0600 perm, then renames it to the target file,
// and finally sets the desired permissions on the target file.
func (fs *DefaultFileSystemAdapter) WriteFileBytesAtomic(filePath string, content []byte, perm os.FileMode) error {
	dir := filepath.Dir(filePath)
	// #nosec G404 -- rand is okay for temp file names, not security critical here
	tmpFileName := fmt.Sprintf("%s.tmp.%d.%d", filepath.Base(filePath), time.Now().UnixNano(), rand.Intn(100000))
	tmpFilePath := filepath.Join(dir, tmpFileName)

	// 1. Write to the temporary file with 0600 permissions.
	if err := os.WriteFile(tmpFilePath, content, 0600); err != nil {
		return fmt.Errorf("failed to write to temporary file %s with 0600 permissions: %w", tmpFilePath, err)
	}

	// 2. Atomically replace the original file with the temporary file.
	if err := os.Rename(tmpFilePath, filePath); err != nil {
		// Attempt to clean up temp file on rename failure
		if removeErr := os.Remove(tmpFilePath); removeErr != nil {
			// Log or handle compound error: rename failed AND temp removal failed
			// For now, primary error is rename error.
			// Example: log.Printf("Warning: failed to remove temp file %s after rename error: %v", tmpFilePath, removeErr)
		}
		return fmt.Errorf("failed to rename temporary file %s to %s: %w", tmpFilePath, filePath, err)
	}

	// 3. Set the final permissions on the destination file.
	if err := os.Chmod(filePath, perm); err != nil {
		// The file is in place, but permissions might not be as expected.
		return fmt.Errorf("file written to %s, but failed to set final permissions to %o: %w", filePath, perm, err)
	}

	return nil
}

// FileExists checks if a file exists.
func (fs *DefaultFileSystemAdapter) FileExists(filePath string) (bool, error) {
	_, err := os.Stat(filePath)
	if err == nil {
		return true, nil // File exists
	}
	if os.IsNotExist(err) {
		return false, nil // File does not exist
	}
	// Some other error occurred (e.g., permission denied)
	return false, fmt.Errorf("error checking if file exists %s: %w", filePath, err)
}

// GetFileStats retrieves statistics for a given file.
func (fs *DefaultFileSystemAdapter) GetFileStats(filePath string) (*FileStats, error) {
	info, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("file not found for stats: %s: %w", filePath, err)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied getting stats for file: %s: %w", filePath, err)
		}
		return nil, fmt.Errorf("failed to get file stats for %s: %w", filePath, err)
	}

	return &FileStats{
		Size:    info.Size(),
		IsDir:   info.IsDir(),
		ModTime: info.ModTime(),
		Mode:    info.Mode().Perm(), // Get file permissions
	}, nil
}

// IsWritable checks if the given path (typically a directory) is writable
// by calling CheckDirectoryIsWritable.
func (fs *DefaultFileSystemAdapter) IsWritable(path string) (bool, error) {
	err := CheckDirectoryIsWritable(path)
	if err == nil {
		return true, nil
	}
	// To match the original behavior of returning (false, nil) for permission errors specifically,
	// we might need to inspect the error. However, the new CheckDirectoryIsWritable
	// already returns a detailed error. For Validate(), any error means not writable.
	// Let's keep it simple: if CheckDirectoryIsWritable errors, IsWritable returns false and that error.
	// The problem description for config.Validate() implies it expects an error for "not writable".
	// The original IsWritable returned (false, nil) on os.IsPermission(err).
	// This is a slight behavior change. Let's stick to the new instruction:
	// "If it returns nil, IsWritable returns (true, nil). If it returns an error, IsWritable returns (false, err)."
	return false, err
}

// NormalizeNewlines converts all newline variations (\r\n and \r) to a single \n.
func (fs *DefaultFileSystemAdapter) NormalizeNewlines(content []byte) []byte {
	if len(content) == 0 { // Handles nil or empty slice
		return []byte{} // Return non-nil empty slice
	}
	// Replace \r\n with \n
	normalized := bytes.ReplaceAll(content, []byte("\r\n"), []byte("\n"))
	// Replace \r with \n (for Mac OS Classic style newlines)
	normalized = bytes.ReplaceAll(normalized, []byte("\r"), []byte("\n"))
	return normalized
}

// SplitLines splits the content by \n after normalizing newlines.
// It removes a trailing empty line if the file ends with a newline, as per spec.
func (fs *DefaultFileSystemAdapter) SplitLines(content []byte) []string {
	if len(content) == 0 {
		return []string{} // Or []string{""} if an empty file means one empty line. Spec implies empty.
	}
	normalized := fs.NormalizeNewlines(content)
	// If normalized content is just a newline, it should result in one empty line,
	// but the trailing empty line removal logic might make it empty.
	// The spec: "Remove trailing empty line if file ends with newline".

	sContent := string(normalized)
	lines := strings.Split(sContent, "\n")

	// "Remove trailing empty line if file ends with newline"
	// This means if sContent ends with "\n" and strings.Split results in a "" at the end, remove it.
	if len(lines) > 0 && strings.HasSuffix(sContent, "\n") {
		// If the last line is empty, it's due to a trailing newline.
		// However, if the file IS just "\n", lines would be ["", ""]. We want [""] after this rule.
		// If content is "a\n", lines is ["a", ""]. We want ["a"].
		if sContent == "\n" { // Special case: content is just a newline
			return []string{""} // Represents a single empty line
		}
		if lines[len(lines)-1] == "" {
			lines = lines[:len(lines)-1]
		}
	}
	return lines
}

// JoinLinesWithNewlines joins a slice of strings into a single byte slice,
// with each string separated by \n.
// If lines is ["a", "b"], output is "a\nb".
// If lines is ["a"], output is "a".
// If lines is [], output is "".
func (fs *DefaultFileSystemAdapter) JoinLinesWithNewlines(lines []string) []byte {
	if len(lines) == 0 {
		return []byte{}
	}
	// strings.Join handles the cases correctly:
	// - ["a", "b"] -> "a\nb"
	// - ["a"]      -> "a" (no separator added)
	// - []         -> ""   (handled by the check above)
	return []byte(strings.Join(lines, "\n"))
}

// Ensure DefaultFileSystemAdapter implements FileSystemAdapter
var _ FileSystemAdapter = (*DefaultFileSystemAdapter)(nil)
