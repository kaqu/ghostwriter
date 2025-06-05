package config

import (
	"flag"
	"fmt"
	"os"
)

// Config holds all configurable values for the server.
type Config struct {
	WorkingDirectory    string
	Transport           string
	Port                int
	MaxFileSizeMB       int
	MaxConcurrentOps    int
	OperationTimeoutSec int
}

// ParseFlags parses the command-line flags and populates the Config struct.
func ParseFlags() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.WorkingDirectory, "dir", "", "Path to the working directory (required)")
	flag.StringVar(&cfg.Transport, "transport", "http", "Transport protocol (http or stdio)")
	flag.IntVar(&cfg.Port, "port", 8080, "Port for HTTP transport")
	flag.IntVar(&cfg.MaxFileSizeMB, "max-file-size", 10, "Maximum file size in MB")
	flag.IntVar(&cfg.MaxConcurrentOps, "max-concurrent", 10, "Maximum concurrent operations")
	flag.IntVar(&cfg.OperationTimeoutSec, "timeout", 30, "Operation timeout in seconds")

	flag.Parse()
	return cfg
}

// Validate checks if the configuration values are valid.
func (c *Config) Validate() error {
	if c.WorkingDirectory == "" {
		return fmt.Errorf("working directory is required")
	}

	// Check if WorkingDirectory exists and is writable
	info, err := os.Stat(c.WorkingDirectory)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("working directory does not exist: %s", c.WorkingDirectory)
		}
		return fmt.Errorf("error accessing working directory: %v", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("working directory is not a directory: %s", c.WorkingDirectory)
	}
	// Minimal check for writability (Unix-like systems).
	// A more robust check might involve trying to create a temporary file.
	if info.Mode().Perm()&(1<<(uint(7))) == 0 { // Check for write permission for others
		// Fallback to check user write permission if group/other don't have it.
		// This is a simplified check. True writability can be complex.
		if os.Geteuid() == 0 { // root user
			// root can often write even if permissions say otherwise
		} else if info.Mode().Perm()&(1<<(uint(8))) == 0 { // Check for write permission for user
			return fmt.Errorf("working directory is not writable: %s", c.WorkingDirectory)
		}
	}


	if c.Transport != "http" && c.Transport != "stdio" {
		return fmt.Errorf("transport must be 'http' or 'stdio'")
	}

	if c.Port < 1024 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1024 and 65535")
	}

	if c.MaxFileSizeMB < 1 || c.MaxFileSizeMB > 100 {
		return fmt.Errorf("max file size must be between 1 and 100 MB")
	}

	if c.MaxConcurrentOps < 1 || c.MaxConcurrentOps > 100 {
		return fmt.Errorf("max concurrent operations must be between 1 and 100")
	}

	if c.OperationTimeoutSec < 5 || c.OperationTimeoutSec > 300 {
		return fmt.Errorf("operation timeout must be between 5 and 300 seconds")
	}

	return nil
}
