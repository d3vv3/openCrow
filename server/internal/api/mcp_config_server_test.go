package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ── configMCPToolDefs integrity ──────────────────────────────────────────────

func TestConfigMCPToolDefs_AllHaveNameAndSchema(t *testing.T) {
	for i, def := range configMCPToolDefs {
		name, _ := def["name"].(string)
		if name == "" {
			t.Errorf("configMCPToolDefs[%d] missing name", i)
		}
		desc, _ := def["description"].(string)
		if desc == "" {
			t.Errorf("configMCPToolDefs[%d] (%q) missing description", i, name)
		}
		schema, _ := def["inputSchema"].(map[string]any)
		if schema == nil {
			t.Errorf("configMCPToolDefs[%d] (%q) missing inputSchema", i, name)
		}
	}
}

func TestConfigMCPToolIndex_CoversAllDefs(t *testing.T) {
	for _, def := range configMCPToolDefs {
		name := def["name"].(string)
		if _, ok := mcpConfigToolIndex[name]; !ok {
			t.Errorf("mcpConfigToolIndex missing entry for %q", name)
		}
	}
	if len(mcpConfigToolIndex) != len(configMCPToolDefs) {
		t.Errorf("index size %d != defs size %d", len(mcpConfigToolIndex), len(configMCPToolDefs))
	}
}

func TestConfigMCPToolDefs_NoDuplicateNames(t *testing.T) {
	seen := map[string]bool{}
	for _, def := range configMCPToolDefs {
		name := def["name"].(string)
		if seen[name] {
			t.Errorf("duplicate tool name %q in configMCPToolDefs", name)
		}
		seen[name] = true
	}
}

// ── handleConfigMCPServer HTTP handler ──────────────────────────────────────

func newConfigMCPRequest(t *testing.T, body map[string]any) *http.Request {
	t.Helper()
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/v1/mcp/config", bytes.NewReader(b))
	r.Header.Set("Content-Type", "application/json")
	// Inject a fake userID into context (same key the real middleware uses)
	r = r.WithContext(context.WithValue(r.Context(), userIDContextKey, "test-user"))
	return r
}

func TestHandleConfigMCPServer_Initialize(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.handleConfigMCPServer(w, newConfigMCPRequest(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      "1",
		"method":  "initialize",
		"params":  map[string]any{},
	}))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	result, _ := resp["result"].(map[string]any)
	if result == nil {
		t.Fatal("missing result")
	}
	if result["protocolVersion"] != "2024-11-05" {
		t.Errorf("protocolVersion = %v", result["protocolVersion"])
	}
	info, _ := result["serverInfo"].(map[string]any)
	if info["name"] != "openCrow Config" {
		t.Errorf("serverInfo.name = %v", info["name"])
	}
}

func TestHandleConfigMCPServer_NotificationsInitialized(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.handleConfigMCPServer(w, newConfigMCPRequest(t, map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	}))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", w.Code)
	}
}

func TestHandleConfigMCPServer_ToolsList(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.handleConfigMCPServer(w, newConfigMCPRequest(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      "2",
		"method":  "tools/list",
		"params":  map[string]any{},
	}))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	result, _ := resp["result"].(map[string]any)
	tools, _ := result["tools"].([]any)
	if len(tools) != len(configMCPToolDefs) {
		t.Errorf("tools count = %d, want %d", len(tools), len(configMCPToolDefs))
	}
}

func TestHandleConfigMCPServer_ToolsCall_UnknownTool(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.handleConfigMCPServer(w, newConfigMCPRequest(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      "3",
		"method":  "tools/call",
		"params":  map[string]any{"name": "nonexistent_tool", "arguments": map[string]any{}},
	}))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	result, _ := resp["result"].(map[string]any)
	if result["isError"] != true {
		t.Errorf("expected isError=true for unknown tool, got: %v", result)
	}
}

func TestHandleConfigMCPServer_UnknownMethod(t *testing.T) {
	s := &Server{}
	w := httptest.NewRecorder()
	s.handleConfigMCPServer(w, newConfigMCPRequest(t, map[string]any{
		"jsonrpc": "2.0",
		"id":      "4",
		"method":  "bogus/method",
	}))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] == nil {
		t.Error("expected JSON-RPC error for unknown method")
	}
}

func TestHandleConfigMCPServer_BadJSON(t *testing.T) {
	s := &Server{}
	r := httptest.NewRequest(http.MethodPost, "/v1/mcp/config", bytes.NewReader([]byte("not json")))
	r = r.WithContext(context.WithValue(r.Context(), userIDContextKey, "test-user"))
	w := httptest.NewRecorder()
	s.handleConfigMCPServer(w, r)
	// Should return a JSON-RPC parse error, not a 500
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with JSON-RPC error", w.Code)
	}
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil {
		t.Error("expected JSON-RPC error object")
	}
}

// ── handleConfigMCPCall dispatch ─────────────────────────────────────────────

func TestHandleConfigMCPCall_AllToolsDispatch(t *testing.T) {
	// Verify every tool in configMCPToolDefs dispatches without "unknown config tool" error.
	// Tools that require a live DB (heartbeat) are skipped -- we just confirm the dispatch
	// table is complete for the rest.
	dbRequiredTools := map[string]bool{
		"configure_heartbeat": true,
		"trigger_heartbeat":   true,
	}
	s := &Server{}
	ctx := context.Background()
	for _, def := range configMCPToolDefs {
		name := def["name"].(string)
		if dbRequiredTools[name] {
			continue
		}
		_, err := s.handleConfigMCPCall(ctx, "u1", name, map[string]any{})
		if err != nil {
			// Acceptable: domain errors (missing args, no configStore, etc.)
			// Unacceptable: "unknown config tool: ..."
			if ute, ok := err.(*mcpUnknownToolError); ok {
				t.Errorf("tool %q not dispatched: %v", name, ute)
			}
		}
	}
}

func TestHandleConfigMCPCall_UnknownTool(t *testing.T) {
	s := &Server{}
	_, err := s.handleConfigMCPCall(context.Background(), "u1", "does_not_exist", nil)
	if err == nil {
		t.Fatal("expected error for unknown tool")
	}
	if _, ok := err.(*mcpUnknownToolError); !ok {
		t.Errorf("expected mcpUnknownToolError, got %T: %v", err, err)
	}
}

// ── isBuiltinToolName -- config tools must NOT be builtin ────────────────────

func TestIsBuiltinToolName_ConfigToolsAreNotBuiltin(t *testing.T) {
	// These config MCP tools have NO builtin case in executeTool's switch --
	// they must NOT appear in isBuiltinToolName so they log as [MCP].
	// Note: setup_dav IS also a builtin (DAV integration), so it is excluded here.
	pureConfigTools := []string{
		"setup_email", "remove_email", "setup_telegram_bot",
		"inspect_dav",
		"add_mcp_server", "remove_mcp_server", "list_mcp_servers",
		"create_device", "delete_device", "edit_device", "edit_device_task",
		"create_skill", "delete_skill", "install_skills",
		"schedule_task", "cancel_task",
		"configure_heartbeat", "trigger_heartbeat",
	}
	for _, name := range pureConfigTools {
		if isBuiltinToolName(name) {
			t.Errorf("%q should NOT be a builtin tool (it's a config MCP tool)", name)
		}
	}
}

// ── executeTool routes config tools through handleConfigMCPCall ──────────────

func TestExecuteTool_ConfigToolsRouteToConfigMCP(t *testing.T) {
	s := &Server{}
	ctx := context.Background()
	// setup_email with missing args should return a domain error (not "unknown tool")
	result, err := s.executeTool(ctx, "u1", "setup_email", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	m, _ := result.(map[string]any)
	if m == nil {
		t.Fatal("expected map result")
	}
	// Should be a domain failure (missing address), not "unknown tool"
	if errMsg, _ := m["error"].(string); errMsg == "unknown tool: setup_email" {
		t.Error("setup_email was not dispatched to config MCP handler")
	}
}

func TestExecuteTool_ConfigToolNotLabelledUnknown(t *testing.T) {
	// Tools that hit the DB with nil pool will panic -- skip them here.
	dbRequiredTools := map[string]bool{
		"configure_heartbeat": true,
		"trigger_heartbeat":   true,
	}
	s := &Server{}
	// All config MCP tools should dispatch without "unknown tool" in the error
	for _, def := range configMCPToolDefs {
		name := def["name"].(string)
		if dbRequiredTools[name] {
			continue
		}
		result, err := s.executeTool(context.Background(), "u1", name, map[string]any{})
		if err != nil {
			continue // domain errors are fine
		}
		m, _ := result.(map[string]any)
		if errMsg, _ := m["error"].(string); errMsg == "unknown tool: "+name {
			t.Errorf("tool %q returned 'unknown tool' -- not dispatched", name)
		}
	}
}

// ── writeMCPError ────────────────────────────────────────────────────────────

func TestWriteMCPError(t *testing.T) {
	w := httptest.NewRecorder()
	writeMCPError(w, "req-1", -32601, "method not found")
	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v", resp["jsonrpc"])
	}
	if resp["id"] != "req-1" {
		t.Errorf("id = %v", resp["id"])
	}
	errObj, _ := resp["error"].(map[string]any)
	if errObj == nil {
		t.Fatal("missing error object")
	}
	if errObj["message"] != "method not found" {
		t.Errorf("message = %v", errObj["message"])
	}
}
