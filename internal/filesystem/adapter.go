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
// It writes to a temporary file first, then renames it to the target file.
// The perm parameter is for the new file if created, typically 0600 or 0644.
func (fs *DefaultFileSystemAdapter) WriteFileBytesAtomic(filePath string, content []byte, perm os.FileMode) error {
	// Create a temporary file in the same directory.
	dir := filepath.Dir(filePath)
	// Generate a random suffix for the temp file to avoid collisions.
	// Using current time and a random number.
	// #nosec G404 -- rand is okay for temp file names
	tmpFileName := fmt.Sprintf("%s.tmp.%d.%d", filepath.Base(filePath), time.Now().UnixNano(), rand.Intn(100000))
	tmpFilePath := filepath.Join(dir, tmpFileName)

	// Write to the temporary file.
	// The spec mentions 0600 for temp files, but WriteFile uses 0666 before umask.
	// We will explicitly chmod after writing if perm is different or more restrictive.
	// For atomic operations, the final perm is what matters.
	if err := os.WriteFile(tmpFilePath, content, perm); err != nil {
		return fmt.Errorf("failed to write to temporary file %s: %w", tmpFilePath, err)
	}

	// Explicitly set permissions if needed, though os.WriteFile with perm should handle it.
	// This is more of a safeguard.
	if err := os.Chmod(tmpFilePath, perm); err != nil {
		_ = os.Remove(tmpFilePath) // Attempt to clean up
		return fmt.Errorf("failed to set permissions on temporary file %s: %w", tmpFilePath, err)
	}

	// Atomically replace the original file with the temporary file.
	if err := os.Rename(tmpFilePath, filePath); err != nil {
		_ = os.Remove(tmpFilePath) // Attempt to clean up
		return fmt.Errorf("failed to rename temporary file %s to %s: %w", tmpFilePath, filePath, err)
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

// IsWritable checks if the given path (typically a directory) is writable.
// It does this by trying to create and then delete a temporary file in that directory.
func (fs *DefaultFileSystemAdapter) IsWritable(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, fmt.Errorf("path does not exist: %s: %w", path, err)
		}
		return false, fmt.Errorf("could not stat path %s: %w", path, err)
	}
	if !info.IsDir() {
		return false, fmt.Errorf("path is not a directory: %s", path)
	}

	// Attempt to create a temporary file in the directory
	// #nosec G404 -- rand is okay for temp file names
	tmpFileName := fmt.Sprintf("writable_test_%d_%d.tmp", time.Now().UnixNano(), rand.Intn(100000))
	tmpFilePath := filepath.Join(path, tmpFileName)

	file, err := os.Create(tmpFilePath)
	if err != nil {
		if os.IsPermission(err) {
			return false, nil // Not writable due to permissions
		}
		return false, fmt.Errorf("error creating temporary file in %s: %w", path, err)
	}

	// Clean up: close and remove the temporary file
	_ = file.Close()
	errRemove := os.Remove(tmpFilePath)
	if errRemove != nil {
		// Log this, as it's not ideal but the writability check passed.
		// fmt.Fprintf(os.Stderr, "Warning: failed to remove temporary test file %s: %v\n", tmpFilePath, errRemove)
	}

	return true, nil // Successfully created and can infer writability
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
