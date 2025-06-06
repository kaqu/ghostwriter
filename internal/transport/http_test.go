package transport

import (
	"bytes"
	"encoding/json"
	"file-editor-server/internal/models"
	"net/http"
	"net/http/httptest"
	"testing"
)

type mockProcessor struct {
	processFunc func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError)
}

func (m *mockProcessor) ExecuteTool(string, interface{}) (*models.MCPToolResult, error) {
	return nil, nil
}

func (m *mockProcessor) ProcessRequest(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
	if m.processFunc != nil {
		return m.processFunc(req)
	}
	return nil, nil
}

func TestHandleMCPSuccess(t *testing.T) {
	proc := &mockProcessor{
		processFunc: func(req models.JSONRPCRequest) (*models.MCPToolResult, *models.JSONRPCError) {
			if req.Method != "tools/list" {
				t.Fatalf("unexpected method %s", req.Method)
			}
			return &models.MCPToolResult{Content: []models.MCPToolContent{{Type: "text", Text: "ok"}}}, nil
		},
	}
	h := NewHTTPHandler(proc, 1)
	rr := httptest.NewRecorder()
	body := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	h.handleMCP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	var resp models.JSONRPCResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
}

func TestHandleMCPBadJSON(t *testing.T) {
	proc := &mockProcessor{}
	h := NewHTTPHandler(proc, 1)
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString("{"))
	req.Header.Set("Content-Type", "application/json")
	h.handleMCP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}
}
