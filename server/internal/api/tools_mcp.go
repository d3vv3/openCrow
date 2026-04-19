// tools_mcp.go — MCP server management and tool invocation.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/opencrow/opencrow/server/internal/configstore"
)

// ── MCP tools ─────────────────────────────────────────────────────────────────

func normalizeMCPHeaders(raw any) map[string]string {
	headers := map[string]string{}
	if raw == nil {
		return headers
	}
	if m, ok := raw.(map[string]any); ok {
		for k, v := range m {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			headers[key] = strings.TrimSpace(fmt.Sprint(v))
		}
		return headers
	}
	if m, ok := raw.(map[string]string); ok {
		for k, v := range m {
			key := strings.TrimSpace(k)
			if key == "" {
				continue
			}
			headers[key] = strings.TrimSpace(v)
		}
	}
	return headers
}

func (s *Server) loadMCPServersForUser(ctx context.Context, userID string) ([]configstore.MCPServerConfig, error) {
	if s.configStore == nil {
		return nil, fmt.Errorf("config store not available")
	}
	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to load config")
	}
	servers := cfg.MCP.Servers
	if dbServers, found, dbErr := s.getMCPServersSetting(ctx, userID); dbErr == nil && found {
		servers = dbServers
	}
	if servers == nil {
		servers = []configstore.MCPServerConfig{}
	}
	return servers, nil
}

func (s *Server) saveMCPServersForUser(ctx context.Context, userID string, servers []configstore.MCPServerConfig) ([]configstore.MCPServerConfig, error) {
	if s.configStore == nil {
		return nil, fmt.Errorf("config store not available")
	}
	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		return nil, fmt.Errorf("failed to load config")
	}
	cfg.MCP.Servers = servers
	saved, err := s.configStore.PutUserConfig(userID, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to save config")
	}
	if err := s.putMCPServersSetting(ctx, userID, saved.MCP.Servers); err != nil {
		return nil, fmt.Errorf("failed to save mcp settings")
	}
	return saved.MCP.Servers, nil
}

func (s *Server) toolListMCPServers(ctx context.Context, userID string) (map[string]any, error) {
	servers, err := s.loadMCPServersForUser(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	items := make([]map[string]any, 0, len(servers))
	for _, srv := range servers {
		items = append(items, map[string]any{
			"id":      srv.ID,
			"name":    srv.Name,
			"url":     srv.URL,
			"enabled": srv.Enabled,
			"headers": srv.Headers,
		})
	}
	return map[string]any{"success": true, "servers": items, "count": len(items)}, nil
}

func (s *Server) toolAddMCPServer(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	name, _ := args["name"].(string)
	url, _ := args["url"].(string)
	name = strings.TrimSpace(name)
	url = strings.TrimSpace(url)
	if name == "" || url == "" {
		return map[string]any{"success": false, "error": "name and url are required"}, nil
	}
	enabled := true
	if v, ok := args["enabled"].(bool); ok {
		enabled = v
	}
	headers := normalizeMCPHeaders(args["headers"])

	servers, err := s.loadMCPServersForUser(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}

	idx := -1
	for i, srv := range servers {
		if strings.EqualFold(strings.TrimSpace(srv.Name), name) || strings.EqualFold(strings.TrimSpace(srv.URL), url) {
			idx = i
			break
		}
	}

	if idx >= 0 {
		servers[idx].Name = name
		servers[idx].URL = url
		servers[idx].Enabled = enabled
		servers[idx].Headers = headers
	} else {
		servers = append(servers, configstore.MCPServerConfig{
			Name:    name,
			URL:     url,
			Enabled: enabled,
			Headers: headers,
		})
	}

	savedServers, err := s.saveMCPServersForUser(ctx, userID, servers)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}

	var out *configstore.MCPServerConfig
	for i := range savedServers {
		srv := &savedServers[i]
		if strings.EqualFold(strings.TrimSpace(srv.Name), name) && strings.EqualFold(strings.TrimSpace(srv.URL), url) {
			out = srv
			break
		}
	}
	if out == nil {
		return map[string]any{"success": true, "message": "mcp server saved"}, nil
	}

	return map[string]any{
		"success": true,
		"server": map[string]any{
			"id":      out.ID,
			"name":    out.Name,
			"url":     out.URL,
			"enabled": out.Enabled,
			"headers": out.Headers,
		},
		"message": "mcp server saved",
	}, nil
}

func (s *Server) toolRemoveMCPServer(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	id, _ := args["id"].(string)
	name, _ := args["name"].(string)
	url, _ := args["url"].(string)
	id = strings.TrimSpace(id)
	name = strings.TrimSpace(name)
	url = strings.TrimSpace(url)
	if id == "" && name == "" && url == "" {
		return map[string]any{"success": false, "error": "provide id, name, or url"}, nil
	}

	servers, err := s.loadMCPServersForUser(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}

	removed := 0
	kept := make([]configstore.MCPServerConfig, 0, len(servers))
	for _, srv := range servers {
		match := false
		if id != "" && strings.EqualFold(strings.TrimSpace(srv.ID), id) {
			match = true
		}
		if name != "" && strings.EqualFold(strings.TrimSpace(srv.Name), name) {
			match = true
		}
		if url != "" && strings.EqualFold(strings.TrimSpace(srv.URL), url) {
			match = true
		}
		if match {
			removed++
			continue
		}
		kept = append(kept, srv)
	}

	if removed == 0 {
		return map[string]any{"success": false, "error": "no matching mcp server found"}, nil
	}

	if _, err := s.saveMCPServersForUser(ctx, userID, kept); err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}

	return map[string]any{
		"success": true,
		"removed": removed,
		"message": "mcp server removed",
	}, nil
}

func (s *Server) toolCallMCPTool(ctx context.Context, userID, toolName string, args map[string]any) (map[string]any, bool) {
	servers, err := s.loadMCPServersForUser(ctx, userID)
	if err != nil || len(servers) == 0 {
		return nil, false
	}

	sanitizedTarget := sanitizeToolName(toolName)
	for _, srv := range servers {
		if !srv.Enabled || strings.TrimSpace(srv.URL) == "" {
			continue
		}
		discoverCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		discovered, derr := fetchMCPTools(discoverCtx, strings.TrimSpace(srv.URL), srv.Headers)
		cancel()
		if derr != nil {
			continue
		}

		actualName := ""
		for _, mt := range discovered {
			if mt.Name == toolName || sanitizeToolName(mt.Name) == sanitizedTarget {
				actualName = mt.Name
				break
			}
		}
		if actualName == "" {
			continue
		}

		callCtx, cancelCall := context.WithTimeout(ctx, 20*time.Second)
		result, cerr := callMCPToolOnServer(callCtx, strings.TrimSpace(srv.URL), srv.Headers, actualName, args)
		cancelCall()
		if cerr != nil {
			return map[string]any{"success": false, "error": cerr.Error(), "mcpServer": srv.Name, "tool": actualName}, true
		}
		return map[string]any{"success": true, "mcpServer": srv.Name, "tool": actualName, "result": result}, true
	}

	return nil, false
}

func callMCPToolOnServer(ctx context.Context, serverURL string, headers map[string]string, toolName string, args map[string]any) (any, error) {
	callRPC := func(payload map[string]any, sessionID string) ([]byte, string, error) {
		body, _ := json.Marshal(payload)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, bytes.NewReader(body))
		if err != nil {
			return nil, sessionID, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		for k, v := range headers {
			if strings.TrimSpace(k) == "" {
				continue
			}
			req.Header.Set(k, v)
		}
		if sessionID != "" {
			req.Header.Set("Mcp-Session-Id", sessionID)
		}
		resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
		if err != nil {
			return nil, sessionID, err
		}
		defer resp.Body.Close()
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
		if sid := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id")); sid != "" {
			sessionID = sid
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return nil, sessionID, fmt.Errorf("mcp http %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
		}
		return respBody, sessionID, nil
	}

	var sessionID string

	initBody, sid, err := callRPC(map[string]any{
		"jsonrpc": "2.0",
		"id":      "init-1",
		"method":  "initialize",
		"params": map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{},
			"clientInfo": map[string]any{
				"name":    "openCrow",
				"version": "0.1.0",
			},
		},
	}, sessionID)
	if err != nil {
		return nil, fmt.Errorf("initialize failed: %w", err)
	}
	sessionID = sid
	initJSON, err := mcpResponseJSONBytes(initBody)
	if err != nil {
		return nil, fmt.Errorf("parse initialize response: %w", err)
	}
	if err := mcpJSONRPCError(initJSON); err != nil {
		return nil, fmt.Errorf("initialize rpc error: %w", err)
	}

	_, _, _ = callRPC(map[string]any{"jsonrpc": "2.0", "method": "notifications/initialized", "params": map[string]any{}}, sessionID)

	callBody, _, err := callRPC(map[string]any{
		"jsonrpc": "2.0",
		"id":      "call-1",
		"method":  "tools/call",
		"params": map[string]any{
			"name":      toolName,
			"arguments": args,
		},
	}, sessionID)
	if err != nil {
		return nil, fmt.Errorf("tools/call failed: %w", err)
	}
	callJSON, err := mcpResponseJSONBytes(callBody)
	if err != nil {
		return nil, fmt.Errorf("parse tools/call response: %w", err)
	}
	if err := mcpJSONRPCError(callJSON); err != nil {
		return nil, fmt.Errorf("tools/call rpc error: %w", err)
	}

	var envelope struct {
		Result json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal(callJSON, &envelope); err != nil {
		return nil, fmt.Errorf("parse tools/call response: %w", err)
	}
	if len(envelope.Result) == 0 {
		return nil, fmt.Errorf("tools/call response missing result")
	}
	var toolResult struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
		IsError bool `json:"isError"`
	}
	if err := json.Unmarshal(envelope.Result, &toolResult); err == nil && toolResult.IsError {
		parts := make([]string, 0, len(toolResult.Content))
		for _, item := range toolResult.Content {
			if strings.TrimSpace(item.Text) == "" {
				continue
			}
			parts = append(parts, item.Text)
		}
		if len(parts) == 0 {
			return nil, fmt.Errorf("tools/call returned isError=true")
		}
		return nil, fmt.Errorf(strings.Join(parts, "\n"))
	}
	var parsed any
	if err := json.Unmarshal(envelope.Result, &parsed); err != nil {
		return nil, fmt.Errorf("parse tools/call result: %w", err)
	}
	return parsed, nil
}
