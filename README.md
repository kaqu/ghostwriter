# File Editing Server

This repository contains a small file editing server written in Go. The server exposes a single `/mcp` HTTP endpoint that accepts JSON-RPC requests for file operations. A JSON-RPC STDIO mode is also available.

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

You can then send a JSON-RPC request to the `/mcp` endpoint:

```bash
curl -X POST -H 'Content-Type: application/json' \
    -d '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' \
    http://localhost:8080/mcp
```

The server logs initialization information and will shut down gracefully on `SIGTERM` or Ctrl+C.

## Testing

Run the unit tests to verify the server's behavior:

```bash
go test ./...
```

