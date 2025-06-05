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
	NormalizeNewlines(content []byte) []byte     // Converts \r\n and \r to \n
	SplitLines(content []byte) []string          // Uses normalized newlines
	JoinLinesWithNewlines(lines []string) []byte // Uses \n
	EvalSymlinks(path string) (string, error)    // New method for symlink evaluation
	ListDir(path string) ([]DirEntryInfo, error) // New method for listing directory contents
}

// DirEntryInfo holds information about a directory entry.
type DirEntryInfo struct {
	Name     string
	IsDir    bool
	IsHidden bool // Helper based on name
	Mode     os.FileMode
	ModTime  time.Time
	Size     int64
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
	// Attempt to remove the temporary file. The error is ignored as the primary purpose
	// of this function is the writability check, which passed if we reached here.
	// This addresses staticcheck SA9003 for an empty branch.
	_ = os.Remove(tmpFilePath)
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
func (fs *DefaultFileSystemAdapter) WriteFileBytesAtomic(filePath string, content []byte, finalPerm os.FileMode) error {
	dir := filepath.Dir(filePath)

	// 1. Create a temporary file. os.CreateTemp creates files with 0600 permissions on Unix-like systems.
	// The pattern argument includes a "*" which will be replaced by a random string.
	tempFile, err := os.CreateTemp(dir, filepath.Base(filePath)+".tmp.*")
	if err != nil {
		return fmt.Errorf("failed to create temporary file in %s: %w", dir, err)
	}
	// Defer removal of the temporary file in case of errors before rename.
	// If rename succeeds, this Remove will fail harmlessly (as file no longer exists at tempFile.Name()).
	defer os.Remove(tempFile.Name())

	// Although os.CreateTemp typically uses 0600, an explicit Chmod can be used
	// for guarantee if desired, but it's often redundant on POSIX systems.
	// For this exercise, we'll trust os.CreateTemp's default 0600 permission for the temp file.
	// If an explicit chmod was required here, it would be:
	// if errChmod := os.Chmod(tempFile.Name(), 0600); errChmod != nil {
	// 	 tempFile.Close() // Close before returning due to chmod error
	// 	 return fmt.Errorf("failed to chmod temporary file %s to 0600: %w", tempFile.Name(), errChmod)
	// }

	// 2. Write content to the temporary file.
	if _, errWrite := tempFile.Write(content); errWrite != nil {
		tempFile.Close() // Attempt to close before returning
		return fmt.Errorf("failed to write to temporary file %s: %w", tempFile.Name(), errWrite)
	}

	// 3. Close the temporary file.
	if errClose := tempFile.Close(); errClose != nil {
		return fmt.Errorf("failed to close temporary file %s: %w", tempFile.Name(), errClose)
	}

	// 4. Atomically replace the original file with the temporary file.
	if errRename := os.Rename(tempFile.Name(), filePath); errRename != nil {
		// If rename fails, tempFile should have already been scheduled for removal by defer.
		return fmt.Errorf("failed to rename temporary file %s to %s: %w", tempFile.Name(), filePath, errRename)
	}

	// 5. Set the final permissions on the destination file.
	// This is important because os.Rename might preserve permissions of an existing filePath,
	// or use default permissions if filePath is newly created by the rename (which can be influenced by tempFile's perms or umask).
	if errChmodFinal := os.Chmod(filePath, finalPerm); errChmodFinal != nil {
		return fmt.Errorf("file written to %s, but failed to set final permissions to %o: %w", filePath, finalPerm, errChmodFinal)
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

// EvalSymlinks evaluates symbolic links for the given path.
func (fs *DefaultFileSystemAdapter) EvalSymlinks(path string) (string, error) {
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		// The error from EvalSymlinks can be os.ErrNotExist, os.ErrPermission, etc.
		// It might also be a more generic error if a path component is not a directory, etc.
		// Returning it directly is usually fine, the service layer can interpret it.
		return "", fmt.Errorf("failed to evaluate symlinks for %s: %w", path, err)
	}
	return resolvedPath, nil
}

// ListDir lists the contents of a directory.
// It returns a slice of DirEntryInfo, excluding the "." and ".." entries.
func (fs *DefaultFileSystemAdapter) ListDir(path string) ([]DirEntryInfo, error) {
	entries, err := os.ReadDir(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("directory not found: %s: %w", path, err)
		}
		if os.IsPermission(err) {
			return nil, fmt.Errorf("permission denied reading directory: %s: %w", path, err)
		}
		return nil, fmt.Errorf("failed to read directory %s: %w", path, err)
	}

	var dirEntries []DirEntryInfo
	for _, entry := range entries {
		// os.ReadDir does not return "." or ".."
		info, err := entry.Info() // Get os.FileInfo for more details
		if err != nil {
			// This can happen if the file is removed/changed between ReadDir and Info()
			// or due to permission issues on the specific file.
			// Log this and continue, or return an error. For now, log and skip.
			// log.Printf("Warning: could not get info for entry %s in %s: %v", entry.Name(), path, err)
			// Alternatively, to be safer, we could return the error.
			// Let's return an error to be safe, as partial listings can be misleading.
			return nil, fmt.Errorf("failed to get info for entry %s in %s: %w", entry.Name(), path, err)
		}

		dirEntries = append(dirEntries, DirEntryInfo{
			Name:     info.Name(),
			IsDir:    info.IsDir(),
			IsHidden: strings.HasPrefix(info.Name(), "."),
			Mode:     info.Mode().Perm(), // Only permission bits
			ModTime:  info.ModTime(),
			Size:     info.Size(),
		})
	}
	return dirEntries, nil
}
