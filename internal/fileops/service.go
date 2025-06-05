package fileops

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const maxRequestSize = 50 * 1024 * 1024

var filenameRegex = regexp.MustCompile(`^[^/\\:*?"<>|]+$`)

// resolvePath returns an absolute path within the service directory.
// It validates against path traversal and symbolic links escaping the root
// directory. When the target file does not yet exist, the existing portion of
// the path is resolved to detect links outside the root.
func (s *Service) resolvePath(name string) (string, error) {
	if err := s.validateName(name); err != nil {
		return "", err
	}
	joined := filepath.Join(s.dir, name)
	// Resolve symlinks on the existing parent directory only
	dir := filepath.Dir(joined)
	absDir, err := filepath.EvalSymlinks(dir)
	if err != nil {
		// Treat nonexistent parent as just the configured dir
		if !os.IsNotExist(err) {
			return "", err
		}
		absDir = s.dir
	}
	absDir, err = filepath.Abs(absDir)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(absDir, s.dir) {
		return "", errors.New("invalid filename")
	}
	// If the target exists, ensure it does not resolve outside the root
	if fi, err := os.Stat(joined); err == nil {
		absFile, err2 := filepath.EvalSymlinks(joined)
		if err2 == nil && !strings.HasPrefix(absFile, s.dir) {
			return "", errors.New("invalid filename")
		}
		_ = fi
	}
	return joined, nil
}

// Service manages file operations within a directory
type Service struct {
	dir     string
	maxSize int64
	locks   *LockManager
}

func NewService(dir string, maxSize int64, maxConcurrent int, timeout time.Duration) *Service {
	return &Service{dir: dir, maxSize: maxSize, locks: NewLockManager(maxConcurrent, timeout)}
}

func (s *Service) fullPath(name string) (string, error) { return s.resolvePath(name) }

// ----- Read File -----

// ReadRequest defines parameters for reading files
type ReadRequest struct {
	Name      string `json:"name"`
	StartLine *int   `json:"start_line,omitempty"`
	EndLine   *int   `json:"end_line,omitempty"`
}

type ReadResponse struct {
	Content        string `json:"content"`
	TotalLines     int    `json:"total_lines"`
	RangeRequested *Range `json:"range_requested,omitempty"`
}

type Range struct {
	StartLine int `json:"start_line"`
	EndLine   int `json:"end_line"`
}

func (s *Service) validateName(name string) error {
	if !filenameRegex.MatchString(name) {
		return errors.New("invalid filename")
	}
	if strings.Contains(name, string(os.PathSeparator)) {
		return errors.New("invalid filename")
	}
	if len(name) > 255 {
		return errors.New("invalid filename")
	}
	if name == "." || name == ".." {
		return errors.New("invalid filename")
	}
	return nil
}

func (s *Service) ReadFile(req *ReadRequest) (*ReadResponse, *OperationError) {
	if err := s.validateName(req.Name); err != nil {
		return nil, NewClientError(err.Error(), nil)
	}
	path, err := s.fullPath(req.Name)
	if err != nil {
		return nil, NewClientError(err.Error(), nil)
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, NewSystemError(fmt.Sprintf("File '%s' not found", req.Name), nil)
		}
		return nil, NewSystemError("stat error", nil)
	}
	if info.Size() > s.maxSize {
		return nil, NewSystemError("file too large", nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, NewSystemError("read error", nil)
	}
	if !utf8.Valid(data) {
		return nil, NewSystemError("invalid utf8", nil)
	}
	lines := splitLines(string(data))
	total := len(lines)
	if total > 100000 {
		return nil, NewSystemError("file exceeds line limit", nil)
	}
	start := 1
	end := total
	if req.StartLine != nil {
		if *req.StartLine < 1 || *req.StartLine > total {
			return nil, NewClientError("start line exceeds file length", nil)
		}
		start = *req.StartLine
	}
	if req.EndLine != nil {
		if *req.EndLine < 1 {
			return nil, NewClientError("end line must be >=1", nil)
		}
		if *req.EndLine < start {
			return nil, NewClientError("start greater than end", nil)
		}
		if *req.EndLine < end {
			end = *req.EndLine
		}
	}
	sel := lines[start-1 : end]
	resp := &ReadResponse{Content: strings.Join(sel, "\n"), TotalLines: total}
	if req.StartLine != nil || req.EndLine != nil {
		r := Range{}
		if req.StartLine != nil {
			r.StartLine = *req.StartLine
		} else {
			r.StartLine = 1
		}
		if req.EndLine != nil {
			r.EndLine = *req.EndLine
		} else {
			r.EndLine = end
		}
		resp.RangeRequested = &r
	}
	return resp, nil
}

func splitLines(s string) []string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	lines := strings.Split(s, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// ----- Edit File -----
type Edit struct {
	Line      int    `json:"line"`
	Content   string `json:"content,omitempty"`
	Operation string `json:"operation"`
}

type EditRequest struct {
	Name            string `json:"name"`
	Edits           []Edit `json:"edits,omitempty"`
	Append          string `json:"append,omitempty"`
	CreateIfMissing bool   `json:"create_if_missing,omitempty"`
}

type EditResponse struct {
	Success       bool `json:"success"`
	LinesModified int  `json:"lines_modified"`
	FileCreated   bool `json:"file_created"`
	NewTotalLines int  `json:"new_total_lines"`
}

func (s *Service) EditFile(req *EditRequest) (*EditResponse, *OperationError) {
	if err := s.validateName(req.Name); err != nil {
		return nil, NewClientError(err.Error(), nil)
	}
	for _, e := range req.Edits {
		if e.Line < 1 {
			return nil, NewClientError("line numbers must be positive", nil)
		}
		if e.Operation != "replace" && e.Operation != "insert" && e.Operation != "delete" {
			return nil, NewClientError("invalid edit operation", nil)
		}
		if e.Operation == "delete" && e.Content != "" {
			return nil, NewClientError("delete operation cannot specify content", nil)
		}
	}
	path, err := s.fullPath(req.Name)
	if err != nil {
		return nil, NewClientError(err.Error(), nil)
	}
	lh, err := s.locks.Acquire(req.Name)
	if err != nil {
		return nil, NewSystemError("file locked", nil)
	}
	defer s.locks.Release(lh)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if !req.CreateIfMissing {
				return nil, NewSystemError("file not found", nil)
			}
			data = []byte{}
		} else {
			return nil, NewSystemError("read error", nil)
		}
	}
	if len(data) > 0 && !utf8.Valid(data) {
		return nil, NewSystemError("invalid utf8", nil)
	}
	lines := splitLines(string(data))
	orig := len(lines)
	if orig > 100000 {
		return nil, NewSystemError("file exceeds line limit", nil)
	}
	sort.Slice(req.Edits, func(i, j int) bool { return req.Edits[i].Line > req.Edits[j].Line })
	for _, e := range req.Edits {
		idx := e.Line - 1
		switch e.Operation {
		case "replace":
			if idx < 0 || idx >= len(lines) {
				return nil, NewClientError(fmt.Sprintf("line %d out of range", e.Line), nil)
			}
			lines[idx] = e.Content
		case "insert":
			if idx < 0 || idx > len(lines) {
				return nil, NewClientError(fmt.Sprintf("line %d out of range", e.Line), nil)
			}
			lines = append(lines[:idx], append([]string{e.Content}, lines[idx:]...)...)
		case "delete":
			if idx < 0 || idx >= len(lines) {
				return nil, NewClientError(fmt.Sprintf("line %d out of range", e.Line), nil)
			}
			lines = append(lines[:idx], lines[idx+1:]...)
		}
	}
	if req.Append != "" {
		lines = append(lines, splitLines(req.Append)...)
	}
	if len(lines) > 100000 {
		return nil, NewSystemError("file exceeds line limit", nil)
	}
	finalContent := strings.Join(lines, "\n")
	if int64(len(finalContent)) > s.maxSize {
		return nil, NewSystemError("file too large", nil)
	}
	tmp := path + ".tmp"
	if err := ioutil.WriteFile(tmp, []byte(finalContent), 0600); err != nil {
		return nil, NewSystemError("write temp failed", nil)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return nil, NewSystemError("rename failed", nil)
	}
	resp := &EditResponse{Success: true, LinesModified: abs(len(lines) - orig), FileCreated: orig == 0 && len(lines) > 0, NewTotalLines: len(lines)}
	return resp, nil
}

func abs(i int) int {
	if i < 0 {
		return -i
	}
	return i
}

// ----- JSON-RPC -----
type RPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  interface{}     `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func (s *Service) HandleRPC(req *RPCRequest) *rpcResponse {
	id := req.ID
	if req.JSONRPC != "2.0" {
		return RPCErrorResponseRaw(id, NewClientError("invalid jsonrpc version", nil))
	}
	switch req.Method {
	case "read_file":
		var r ReadRequest
		if err := json.Unmarshal(req.Params, &r); err != nil {
			return RPCErrorResponseRaw(id, NewClientError("invalid params", nil))
		}
		resp, op := s.ReadFile(&r)
		if op != nil {
			return RPCErrorResponseRaw(id, op)
		}
		return &rpcResponse{JSONRPC: "2.0", ID: id, Result: resp}
	case "edit_file":
		var r EditRequest
		if err := json.Unmarshal(req.Params, &r); err != nil {
			return RPCErrorResponseRaw(id, NewClientError("invalid params", nil))
		}
		resp, op := s.EditFile(&r)
		if op != nil {
			return RPCErrorResponseRaw(id, op)
		}
		return &rpcResponse{JSONRPC: "2.0", ID: id, Result: resp}
	default:
		return RPCErrorResponseRaw(id, NewClientError("unknown method", nil))
	}
}

func RPCErrorResponseRaw(id json.RawMessage, op *OperationError) *rpcResponse {
	return &rpcResponse{JSONRPC: "2.0", ID: id, Error: &rpcError{Code: op.Code, Message: op.Message, Data: op.Data}}
}

// ----- HTTP Server -----
type HTTPServer struct{ svc *Service }

func NewHTTPServer(svc *Service) *HTTPServer { return &HTTPServer{svc: svc} }

func (h *HTTPServer) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/read_file", h.handleRead)
	mux.HandleFunc("/edit_file", h.handleEdit)
	return mux
}

type httpError struct {
	Error *rpcError `json:"error"`
}

func (h *HTTPServer) writeError(w http.ResponseWriter, op *OperationError) {
	w.Header().Set("Content-Type", "application/json")
	status := 400
	if op.Code == -32001 {
		msg := strings.ToLower(op.Message)
		if strings.Contains(msg, "not found") {
			status = http.StatusNotFound
		} else if strings.Contains(msg, "permission") {
			status = http.StatusForbidden
		} else if strings.Contains(msg, "too large") {
			status = http.StatusRequestEntityTooLarge
		} else if strings.Contains(msg, "file locked") {
			status = http.StatusConflict
		} else {
			status = http.StatusInternalServerError
		}
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(httpError{Error: &rpcError{Code: op.Code, Message: op.Message, Data: op.Data}})
}

func (h *HTTPServer) handleRead(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		h.writeError(w, NewClientError("Content-Type must be application/json", nil))
		return
	}
	var req ReadRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestSize)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, NewClientError("invalid json", nil))
		return
	}
	resp, op := h.svc.ReadFile(&req)
	if op != nil {
		h.writeError(w, op)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

func (h *HTTPServer) handleEdit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		h.writeError(w, NewClientError("Content-Type must be application/json", nil))
		return
	}
	var req EditRequest
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestSize)
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		h.writeError(w, NewClientError("invalid json", nil))
		return
	}
	resp, op := h.svc.EditFile(&req)
	if op != nil {
		h.writeError(w, op)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ----- Error Types -----
type OperationError struct {
	Code      int         `json:"code"`
	Message   string      `json:"message"`
	Category  string      `json:"category"`
	Data      interface{} `json:"data,omitempty"`
	Timestamp time.Time   `json:"timestamp"`
}

func NewClientError(msg string, data interface{}) *OperationError {
	return &OperationError{Code: -32602, Message: msg, Category: "client", Data: data, Timestamp: time.Now().UTC()}
}
func NewSystemError(msg string, data interface{}) *OperationError {
	return &OperationError{Code: -32001, Message: msg, Category: "system", Data: data, Timestamp: time.Now().UTC()}
}
