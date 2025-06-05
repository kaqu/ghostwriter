package main

import (
	"context"
	"file-editor-server/internal/config"
	"file-editor-server/internal/filesystem"
	"file-editor-server/internal/lock"
	"file-editor-server/internal/service"
	"file-editor-server/internal/transport"
	// "fmt" // Removed as it's unused
	"log"
	"math"      // Added import
	"net/http" // Required for http.Server in graceful shutdown
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	// 1. Parse Config & 2. Validate Config (happens inside here)
	cfg := loadAndValidateConfig()

	// 3. Initialize Logger
	initializeLogger(cfg.Transport)

	// 4. Log Effective Configuration
	logEffectiveConfig(cfg)

	// 5. Initialize Dependencies
	fsAdapter := filesystem.NewDefaultFileSystemAdapter()
	lockManager := lock.NewLockManager(math.MaxInt32, time.Duration(cfg.OperationTimeoutSec)*time.Second) // Changed MaxConcurrentOps
	fileService, err := service.NewDefaultFileOperationService(fsAdapter, lockManager, cfg)
	if err != nil {
		log.Printf("CRITICAL: Failed to initialize file operation service: %v\n", err)
		os.Exit(1)
	}
	log.Println("Core services initialized successfully.")

	// --- Shutdown context ---
	// Create a context that can be cancelled for graceful shutdown
	// mainCtx, cancel := context.WithCancel(context.Background())
	// defer cancel() // Ensure all paths cancel the context

	// 7. Setup and wait for shutdown signal
	// This will be slightly different for HTTP vs stdio regarding server instance
	var httpServer *http.Server // Declare httpServer here to access it in shutdown

	// Channel to listen for OS signals
	shutdownChan := make(chan os.Signal, 1)
	signal.Notify(shutdownChan, os.Interrupt, syscall.SIGTERM)

	// Goroutine to start the selected transport
	serverDoneChan := make(chan error, 1) // To signal when server stops

	// --- Periodic Lock Cleanup ---
	const lockCleanupInterval = 5 * time.Minute
	lockCleanupStopChan := make(chan struct{})

	go func() {
		ticker := time.NewTicker(lockCleanupInterval)
		defer ticker.Stop()
		log.Println("Starting periodic lock cleanup routine.")
		for {
			select {
			case <-ticker.C:
				log.Println("Running expired lock cleanup...")
				lockManager.CleanupExpiredLocks() // Assuming lockManager is accessible here
			case <-lockCleanupStopChan:
				log.Println("Stopping periodic lock cleanup routine.")
				return
			}
		}
	}()

	// 6. Initialize and Start Transport
	if cfg.Transport == "http" {
		log.Printf("Initializing HTTP transport on port %d...\n", cfg.Port)
		// Note: MaxFileSizeMB is a placeholder for the second arg of NewHTTPHandler,
		// as it currently uses a hardcoded 50MB for HTTP request size.
		httpHandler := transport.NewHTTPHandler(fileService, cfg.MaxFileSizeMB)
		httpServer = httpHandler.Server // Get the server instance from the handler

		// httpHandler.StartServer will be modified to return the *http.Server instance
		// or StartServer itself will run in a goroutine and pass the server instance back.
		// For now, let's adapt StartServer or create a new method for non-blocking start.

		// Modification: StartServer will be non-blocking and return the server instance.
		// Or, more simply, StartServer is blocking but we run it in a goroutine.
		go func() {
			// Capture the actual http.Server instance if StartServer is modified to provide it,
			// or manage it within StartServer itself for shutdown.
			// For simplicity, we assume StartServer is blocking and we need to handle shutdown
			// by closing a quit channel passed to StartServer, or by http.Server.Shutdown.
			// The current httpHandler.StartServer is blocking.
			// We need a way to get the *http.Server to call Shutdown on it.
			// Let's assume NewHTTPHandler can also return the server instance, or StartServer takes a context.

			// Simpler approach: modify StartServer to take a shutdown channel or return the server instance.
			// For this iteration, we'll assume StartServer is blocking and handle shutdown outside its direct control for now,
			// focusing on the signal. A more robust implementation would involve server.Shutdown().
			// The current StartServer in http.go does not return the server.
			// We will need to modify it or assume it handles shutdown internally via context.
			// For now, let's just log the attempt. A proper server.Shutdown() is preferred.

			// To enable server.Shutdown, StartServer needs to expose the *http.Server.
			// Let's make a conceptual change here and assume StartServer can be made to cooperate.
			// This part needs http.go to be refactored for proper graceful shutdown.
			// For now, we just demonstrate the signal handling part.
			// We will call a conceptual `httpHandler.GetHTTPServer()` if it existed.
			// For now, StartServer is blocking.
			log.Printf("Starting HTTP server...")
			err := httpHandler.StartServer(cfg.Port, cfg.OperationTimeoutSec, cfg.OperationTimeoutSec)
			if err != nil && err != http.ErrServerClosed {
				log.Printf("HTTP server error: %v\n", err)
				serverDoneChan <- err
			} else {
				// If err is http.ErrServerClosed, it means graceful shutdown happened.
				// If err is nil (though StartServer is documented to always return non-nil), also normal.
				log.Println("HTTP server finished.")
				serverDoneChan <- nil
			}
		}()

	} else if cfg.Transport == "stdio" {
		log.Println("Initializing STDIN/STDOUT JSON-RPC transport...")
		go func() {
			stdioHandler := transport.NewStdioHandler(fileService)
			if err := stdioHandler.Start(os.Stdin, os.Stdout); err != nil {
				log.Printf("STDIO handler error: %v\n", err)
				serverDoneChan <- err // Stdio handler error
			} else {
				serverDoneChan <- nil // Stdio handler finished (e.g. EOF)
			}
		}()
	} else {
		log.Printf("CRITICAL: Unsupported transport type: %s\n", cfg.Transport)
		os.Exit(1) // Should be caught by config validation, but defensive
	}

	// 8. Wait for signal or server error
	select {
	case sig := <-shutdownChan:
		log.Printf("Shutdown signal received: %s. Initiating graceful shutdown...\n", sig)
		close(lockCleanupStopChan) // Signal lock cleanup goroutine to stop

		// --- Graceful Shutdown Logic ---
		shutdownTimeout := time.Duration(cfg.OperationTimeoutSec) * time.Second
		// Add a small buffer to the overall shutdown timeout for cleanup tasks
		// totalShutdownDeadline := time.Now().Add(shutdownTimeout + 2*time.Second)

		// shutdownTimeout := time.Duration(cfg.OperationTimeoutSec) * time.Second // This is the redundant declaration

		if cfg.Transport == "http" && httpServer != nil {
			log.Println("Attempting to gracefully shut down HTTP server...")
			ctx, cancelShutdown := context.WithTimeout(context.Background(), shutdownTimeout)
			defer cancelShutdown() // Ensure context is cancelled to free resources

			if err := httpServer.Shutdown(ctx); err != nil {
				log.Printf("HTTP server graceful shutdown error: %v\n", err)
			} else {
				log.Println("HTTP server gracefully stopped.")
			}
		} else if cfg.Transport == "stdio" {
			// Stdio handler typically stops when stdin is closed.
			// Closing os.Stdin here is tricky and might not be the right way.
			// Usually, the input source (e.g. pipe) closing is the signal.
			log.Println("STDIO transport: Shutdown signal received. Handler will stop on input EOF or error.")
			// If Stdin is an interactive terminal, Ctrl+D (EOF) would stop it.
			// If piped, closure of the pipe stops it.
			// We can close `os.Stdin` to force it, but it's aggressive.
			// For now, rely on external closure of stdin or process termination.
			// In a real daemon, you might send a specific "shutdown" JSON-RPC message if the protocol supports it.
		}

		// Wait for the server goroutine to finish, or timeout
		// select {
		// case <-serverDoneChan:
		//	log.Println("Server goroutine finished.")
		// case <-time.After(shutdownTimeout):
		//	log.Println("Timeout waiting for server goroutine to finish.")
		// }

	case err := <-serverDoneChan:
		if err != nil {
			log.Printf("Server/handler stopped due to error: %v\n", err)
			close(lockCleanupStopChan) // Also stop cleanup if server fails
			os.Exit(1)                 // Exit with error if server failed
		}
		log.Println("Server/handler stopped normally.")
		close(lockCleanupStopChan) // Also stop cleanup if server stops normally
	}

	log.Println("Application shutting down.")
	os.Exit(0)
}

func loadAndValidateConfig() *config.Config {
	cfg := config.ParseFlags()
	if err := cfg.Validate(); err != nil {
		// Temporarily set log for this initial error, as proper logging is set up later
		log.SetOutput(os.Stderr)
		log.SetFlags(log.LstdFlags | log.Lshortfile)
		log.Printf("CRITICAL: Configuration error: %v\n", err)
		os.Exit(1)
	}
	return cfg
}

func initializeLogger(transportType string) {
	if transportType == "stdio" {
		log.SetOutput(os.Stderr) // JSON-RPC responses go to os.Stdout
	} else {
		log.SetOutput(os.Stdout)
	}
	log.SetFlags(log.LstdFlags | log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile) // More detailed log
	log.Println("Logger initialized.")
}

func logEffectiveConfig(cfg *config.Config) {
	// In a real app, filter or mask sensitive data.
	// For this app, all config is safe to log.
	log.Println("Effective configuration:")
	log.Printf("  Working Directory: %s\n", cfg.WorkingDirectory)
	log.Printf("  Transport: %s\n", cfg.Transport)
	if cfg.Transport == "http" {
		log.Printf("  HTTP Port: %d\n", cfg.Port)
	}
	log.Printf("  Max File Size (MB): %d\n", cfg.MaxFileSizeMB)
	// log.Printf("  Max Concurrent Ops: %d\n", cfg.MaxConcurrentOps) // Removed logging for MaxConcurrentOps
	log.Printf("  Operation Timeout (sec): %d\n", cfg.OperationTimeoutSec)
}

// Note: The http.Server instance needs to be accessible for graceful shutdown.
// transport.HTTPHandler.StartServer would need to be refactored to either:
// 1. Return the *http.Server instance.
// 2. Accept a context that can be canceled to trigger shutdown internally.
// 3. Have a separate Shutdown method.
// This implementation sketch assumes such a mechanism can be added to http.go.
// For stdio, graceful shutdown is typically handled by closing stdin or an EOF signal.
// The current code provides the structure for signal handling.
// The `httpServer` variable is declared but not assigned from `StartServer` due to current `StartServer` signature.
// Proper graceful HTTP shutdown requires that refactor.

// Placeholder for the http.Server instance that would be managed by HTTPHandler
// var globalHTTPServer *http.Server // This is not ideal, better to pass context or use channels

// The current StartServer in http.go is blocking and handles its own logging.
// For graceful shutdown, it would typically run in a goroutine, and main would hold the server instance.
// The refactor of http.go's StartServer to support this is outside this specific subtask's direct changes
// but is noted for completeness of graceful shutdown.
