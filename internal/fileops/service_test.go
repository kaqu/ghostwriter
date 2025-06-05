package fileops

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func setupTempDir(t *testing.T) string {
	dir := t.TempDir()
	return dir
}

func newServiceForTest(dir string) *Service {
	return NewService(dir, 1024*1024, 5, time.Second)
}

func writeFile(t *testing.T, dir, name, content string) {
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
}

func readFileContent(t *testing.T, dir, name string) string {
	b, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		t.Fatalf("read file: %v", err)
	}
	return string(b)
}

func TestReadFileFull(t *testing.T) {
	dir := setupTempDir(t)
	writeFile(t, dir, "a.txt", "line1\nline2\nline3")
	svc := newServiceForTest(dir)
	resp, err := svc.ReadFile(&ReadRequest{Name: "a.txt"})
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if resp.TotalLines != 3 || resp.Content != "line1\nline2\nline3" {
		t.Fatalf("unexpected response: %+v", resp)
	}
}

func TestEditFileOperations(t *testing.T) {
	dir := setupTempDir(t)
	writeFile(t, dir, "b.txt", "l1\nl2\nl3")
	svc := newServiceForTest(dir)
	edits := []Edit{
		{Line: 2, Content: "new2", Operation: "replace"},
		{Line: 3, Content: "x", Operation: "insert"},
	}
	resp, err := svc.EditFile(&EditRequest{Name: "b.txt", Edits: edits})
	if err != nil || !resp.Success {
		t.Fatalf("edit error: %v", err)
	}
	expected := "l1\nnew2\nx\nl3"
	out := readFileContent(t, dir, "b.txt")
	if out != expected {
		t.Fatalf("expected %q got %q", expected, out)
	}
}

func TestReadFileRange(t *testing.T) {
	dir := setupTempDir(t)
	writeFile(t, dir, "c.txt", "l1\nl2\nl3\nl4")
	svc := newServiceForTest(dir)
	start := 2
	end := 3
	resp, err := svc.ReadFile(&ReadRequest{Name: "c.txt", StartLine: &start, EndLine: &end})
	if err != nil {
		t.Fatalf("read error: %v", err)
	}
	if resp.Content != "l2\nl3" || resp.RangeRequested == nil {
		t.Fatalf("unexpected range response: %+v", resp)
	}
}

func TestEditFileCreate(t *testing.T) {
	dir := setupTempDir(t)
	svc := newServiceForTest(dir)
	resp, err := svc.EditFile(&EditRequest{Name: "new.txt", Append: "a", CreateIfMissing: true})
	if err != nil || !resp.Success || !resp.FileCreated {
		t.Fatalf("create error: %v", err)
	}
	out := readFileContent(t, dir, "new.txt")
	if out != "a" {
		t.Fatalf("unexpected content: %s", out)
	}
}

func TestLineLimitExceeded(t *testing.T) {
	dir := setupTempDir(t)
	svc := newServiceForTest(dir)
	// create file with 100001 lines
	lines := make([]string, 100001)
	for i := 0; i < len(lines); i++ {
		lines[i] = "x"
	}
	content := strings.Join(lines, "\n")
	writeFile(t, dir, "big.txt", content)
	if _, err := svc.ReadFile(&ReadRequest{Name: "big.txt"}); err == nil {
		t.Fatalf("expected error for line limit")
	}
}

func TestHTTPHandlersValidation(t *testing.T) {
	dir := setupTempDir(t)
	svc := newServiceForTest(dir)
	server := NewHTTPServer(svc)
	ts := httptest.NewServer(server.Router())
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/read_file", "text/plain", strings.NewReader("{}"))
	if err != nil {
		t.Fatalf("http post: %v", err)
	}
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected 400 got %d", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/read_file", nil)
	r2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("http get: %v", err)
	}
	if r2.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405 got %d", r2.StatusCode)
	}
}

func TestInvalidFilename(t *testing.T) {
	dir := setupTempDir(t)
	svc := newServiceForTest(dir)
	if _, err := svc.ReadFile(&ReadRequest{Name: "../bad"}); err == nil {
		t.Fatalf("expected error for path traversal")
	}
}

func TestHTTPConflictStatus(t *testing.T) {
	dir := setupTempDir(t)
	writeFile(t, dir, "conflict.txt", "a")
	svc := newServiceForTest(dir)
	lh, err := svc.locks.Acquire("conflict.txt")
	if err != nil {
		t.Fatalf("acquire lock: %v", err)
	}
	defer svc.locks.Release(lh)

	server := NewHTTPServer(svc)
	ts := httptest.NewServer(server.Router())
	defer ts.Close()

	payload := `{"name":"conflict.txt","append":"b"}`
	resp, err := http.Post(ts.URL+"/edit_file", "application/json", strings.NewReader(payload))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		t.Fatalf("expected 409 got %d", resp.StatusCode)
	}
}
