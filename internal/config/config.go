package config

import (
	"file-editor-server/internal/filesystem"
	"flag"
	"fmt"
	"net"
	"os"
)

// Config holds all configurable values for the server.
type Config struct {
	WorkingDirectory    string
	Transport           string
	Port                int
	MaxFileSizeMB       int
	OperationTimeoutSec int
}

// ParseFlags parses the command-line flags and populates the Config struct.
func ParseFlags() *Config {
	cfg := &Config{}

	flag.StringVar(&cfg.WorkingDirectory, "dir", "", "Path to the working directory (required)")
	flag.StringVar(&cfg.Transport, "transport", "http", "Transport protocol (http or stdio)")
	flag.IntVar(&cfg.Port, "port", 8080, "Port for HTTP transport")
	flag.IntVar(&cfg.MaxFileSizeMB, "max-size", 10, "Maximum file size in MB")
	flag.IntVar(&cfg.OperationTimeoutSec, "timeout", 10, "Operation timeout in seconds")

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
	// Use the new robust writability check
	if err := filesystem.CheckDirectoryIsWritable(c.WorkingDirectory); err != nil {
		return fmt.Errorf("working directory is not writable: %s: %w", c.WorkingDirectory, err)
	}

	if c.Transport != "http" && c.Transport != "stdio" {
		return fmt.Errorf("transport must be 'http' or 'stdio'")
	}

	if c.Transport == "http" {
		if c.Port < 1024 || c.Port > 65535 {
			return fmt.Errorf("port must be between 1024 and 65535")
		}
		// Check if port is available
		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", c.Port))
		if err != nil {
			return fmt.Errorf("HTTP port %d is not available: %w", c.Port, err)
		}
		// Attempt to close the listener and ignore the error, as the port was available.
		_ = listener.Close()
	}

	if c.MaxFileSizeMB < 1 || c.MaxFileSizeMB > 100 {
		return fmt.Errorf("max file size must be between 1 and 100 MB")
	}

	if c.OperationTimeoutSec < 1 || c.OperationTimeoutSec > 30 {
		return fmt.Errorf("operation timeout must be between 1 and 30 seconds")
	}

	return nil
}
