package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"ghostwriter/internal/fileops"
)

// Config holds server configuration
type Config struct {
	Dir           string
	Transport     string
	Port          int
	MaxFileSizeMB int
	MaxConcurrent int
	Timeout       int
}

var filenameRegexp = regexp.MustCompile(`^[^/\\:*?"<>|]+$`)

func validateConfig(cfg *Config) error {
	if cfg.Dir == "" {
		return errors.New("--dir is required")
	}
	info, err := os.Stat(cfg.Dir)
	if err != nil {
		return fmt.Errorf("invalid dir: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%s is not a directory", cfg.Dir)
	}
	testfile := filepath.Join(cfg.Dir, ".writable")
	if err := os.WriteFile(testfile, []byte("test"), 0644); err != nil {
		return fmt.Errorf("directory not writable: %w", err)
	}
	os.Remove(testfile)
	if cfg.Port < 1024 || cfg.Port > 65535 {
		return errors.New("port out of range")
	}
	if cfg.Transport == "http" {
		l, err := net.Listen("tcp", ":"+strconv.Itoa(cfg.Port))
		if err != nil {
			return fmt.Errorf("port unavailable: %w", err)
		}
		l.Close()
	}
	if cfg.MaxFileSizeMB < 1 || cfg.MaxFileSizeMB > 100 {
		return errors.New("max-file-size out of range")
	}
	if cfg.MaxConcurrent < 1 || cfg.MaxConcurrent > 100 {
		return errors.New("max-concurrent out of range")
	}
	if cfg.Timeout < 5 || cfg.Timeout > 300 {
		return errors.New("timeout out of range")
	}
	if cfg.Transport != "http" && cfg.Transport != "stdio" {
		return errors.New("invalid transport")
	}
	return nil
}

func main() {
	cfg := Config{}
	flag.StringVar(&cfg.Dir, "dir", "", "working directory")
	flag.StringVar(&cfg.Transport, "transport", "http", "http or stdio")
	flag.IntVar(&cfg.Port, "port", 8080, "http port")
	flag.IntVar(&cfg.MaxFileSizeMB, "max-file-size", 10, "max file size MB")
	flag.IntVar(&cfg.MaxConcurrent, "max-concurrent", 10, "max concurrent ops")
	flag.IntVar(&cfg.Timeout, "timeout", 30, "timeout seconds")
	flag.Parse()

	if err := validateConfig(&cfg); err != nil {
		log.Fatal(err)
	}

	fsvc := fileops.NewService(cfg.Dir, int64(cfg.MaxFileSizeMB)*1024*1024, cfg.MaxConcurrent, time.Duration(cfg.Timeout)*time.Second)

	if cfg.Transport == "http" {
		startHTTP(&cfg, fsvc)
	} else {
		startStdio(fsvc)
	}
}

func startHTTP(cfg *Config, svc *fileops.Service) {
	srv := &http.Server{
		Addr:    ":" + strconv.Itoa(cfg.Port),
		Handler: fileops.NewHTTPServer(svc).Router(),
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-stop
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			log.Printf("shutdown error: %v", err)
		}
	}()

	log.Printf("starting HTTP server on %s", srv.Addr)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

func startStdio(svc *fileops.Service) {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var req fileops.RPCRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			resp := fileops.RPCErrorResponseRaw(json.RawMessage("null"), fileops.NewClientError("invalid json", nil))
			json.NewEncoder(os.Stdout).Encode(resp)
			continue
		}
		resp := svc.HandleRPC(&req)
		b, _ := json.Marshal(resp)
		io.WriteString(os.Stdout, string(b)+"\n")
	}
	if err := scanner.Err(); err != nil {
		log.Println("stdin error:", err)
	}
}
