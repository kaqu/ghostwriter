# File Editing Server

This repository contains a small file editing server written in Go. The server exposes HTTP endpoints for reading, editing and listing files in a specified working directory. A JSON-RPC STDIO mode is also available.

## Building

```bash
go build ./cmd/file-editor
```

This will produce a `file-editor` binary in the current directory.

## Running

The server requires a working directory for file operations. Use the `-dir` flag to specify it. Choose `http` or `stdio` transport with `-transport` (defaults to `http`).

Example running the HTTP server on port 8080:

```bash
./file-editor -dir /path/to/workdir -transport http -port 8080
```

You can then check the health endpoint:

```bash
curl http://localhost:8080/health
```

The server logs initialization information and will shut down gracefully on `SIGTERM` or Ctrl+C.

