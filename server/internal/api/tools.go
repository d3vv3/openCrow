package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/opencrow/opencrow/server/internal/configstore"
	"github.com/opencrow/opencrow/server/internal/realtime"
	"golang.org/x/crypto/ssh"
)

// processManager is the singleton process manager for background shell sessions.
var processManager = NewProcessManager()

// buildToolExecutor returns a function that executes a named tool with args.
func (s *Server) buildToolExecutor(parentCtx context.Context, userID string) func(context.Context, string, map[string]any) (string, error) {
	return func(ctx context.Context, name string, args map[string]any) (string, error) {
		execCtx := ctx
		if execCtx == nil {
			execCtx = parentCtx
		}
		if clientTimezoneFromContext(execCtx) == "" {
			if parentTZ := clientTimezoneFromContext(parentCtx); parentTZ != "" {
				execCtx = context.WithValue(execCtx, clientTimezoneContextKey, parentTZ)
			}
		}
		result, err := s.executeTool(execCtx, userID, name, args)
		if name != "execute_shell_command" {
			kind := "TOOL"
			if !isBuiltinToolName(name) {
				kind = "MCP"
			}
			if m, ok := result.(map[string]any); ok {
				if _, has := m["mcpServer"]; has {
					kind = "MCP"
				}
			}
			s.writeToolCallToTerminal(userID, kind, name, args, result, err)
		}
		if err != nil {
			return "", err
		}
		out, _ := json.Marshal(result)
		return string(out), nil
	}
}

func isBuiltinToolName(name string) bool {
	switch name {
	case "get_local_time", "get_location",
		"web_search", "open_url",
		"store_memory", "forget_memory", "learn_memory", "read_memory", "reinforce_memory", "promote_learning",
		"list_tasks", "schedule_task", "cancel_task",
		"configure_heartbeat", "trigger_heartbeat",
		"setup_email", "check_email", "read_email", "reply_email", "compose_email", "search_email",
		"send_notification",
		"execute_shell_command", "manage_process",
		"ssh_execute",
		"list_skills", "get_skill", "install_skills",
		"list_mcp_servers", "add_mcp_server", "remove_mcp_server":
		return true
	default:
		return false
	}
}

// executeTool dispatches a tool call by name.
func (s *Server) executeTool(ctx context.Context, userID, name string, args map[string]any) (any, error) {
	switch name {

	// ── Time & Location ──────────────────────────────────────────────
	case "get_local_time":
		return s.toolGetLocalTime(ctx, userID, args), nil

	case "get_location":
		return s.toolGetLocation(ctx)

	// ── Web ──────────────────────────────────────────────────────────
	case "web_search":
		return s.toolWebSearch(ctx, args)

	case "open_url":
		return s.toolOpenURL(ctx, args)

	// ── Memory ───────────────────────────────────────────────────────
	case "store_memory":
		return s.toolStoreMemory(ctx, userID, args)

	case "forget_memory":
		return s.toolForgetMemory(ctx, userID, args)

	case "learn_memory":
		return s.toolLearnMemory(ctx, userID, args)

	case "read_memory":
		return s.toolReadMemory(ctx, userID)

	case "reinforce_memory":
		return s.toolReinforceMemory(ctx, userID, args)

	case "promote_learning":
		return s.toolPromoteLearning(ctx, userID, args)

	// ── Tasks / Scheduling ───────────────────────────────────────────
	case "list_tasks":
		return s.toolListTasks(ctx, userID)

	case "schedule_task":
		return s.toolScheduleTask(ctx, userID, args)

	case "cancel_task":
		return s.toolCancelTask(ctx, userID, args)

	// ── Heartbeat ────────────────────────────────────────────────────
	case "configure_heartbeat":
		return s.toolConfigureHeartbeat(ctx, userID, args)

	case "trigger_heartbeat":
		return s.toolTriggerHeartbeat(ctx, userID)

	// ── Email ────────────────────────────────────────────────────────
	case "setup_email":
		return s.toolSetupEmail(ctx, userID, args)

	case "check_email":
		return s.toolCheckEmail(ctx, userID, args)

	case "read_email":
		return s.toolReadEmail(ctx, userID, args)

	case "reply_email":
		return s.toolReplyEmail(ctx, userID, args)

	case "compose_email":
		return s.toolComposeEmail(ctx, userID, args)

	case "search_email":
		return s.toolSearchEmail(ctx, userID, args)

	// ── Notification ─────────────────────────────────────────────────
	case "send_notification":
		return s.toolSendNotification(ctx, userID, args)

	// ── Shell ────────────────────────────────────────────────────────
	case "execute_shell_command":
		return s.toolExecuteShellCommand(ctx, userID, args)

	case "manage_process":
		return s.toolManageProcess(args)

	// ── Remote SSH ───────────────────────────────────────────────────
	case "ssh_execute":
		return s.toolRemoteExecute(ctx, userID, args)

	// ── Skills ───────────────────────────────────────────────────────
	case "list_skills":
		return s.toolListSkills()

	case "get_skill":
		return s.toolGetSkill(args)

	case "install_skills":
		return s.toolInstallSkills(args)

	// ── MCP ───────────────────────────────────────────────────────────
	case "list_mcp_servers":
		return s.toolListMCPServers(ctx, userID)

	case "add_mcp_server":
		return s.toolAddMCPServer(ctx, userID, args)

	case "remove_mcp_server":
		return s.toolRemoveMCPServer(ctx, userID, args)

	default:
		if mcpResult, ok := s.toolCallMCPTool(ctx, userID, name, args); ok {
			return mcpResult, nil
		}
		return map[string]any{"success": false, "error": fmt.Sprintf("unknown tool: %s", name)}, nil
	}
}

// ── Skill tools ───────────────────────────────────────────────────────────────

func (s *Server) toolListSkills() (map[string]any, error) {
	if s.skillStore == nil {
		return map[string]any{"skills": []any{}, "count": 0}, nil
	}
	skills, err := s.skillStore.List()
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	items := make([]map[string]any, len(skills))
	for i, sk := range skills {
		items[i] = map[string]any{
			"slug":        sk.Slug,
			"name":        sk.Name,
			"description": sk.Description,
		}
	}
	return map[string]any{"skills": items, "count": len(items)}, nil
}

func (s *Server) toolGetSkill(args map[string]any) (map[string]any, error) {
	if s.skillStore == nil {
		return map[string]any{"success": false, "error": "skill store not available"}, nil
	}
	slug, _ := args["slug"].(string)
	if slug == "" {
		return map[string]any{"success": false, "error": "slug is required"}, nil
	}
	sf, err := s.skillStore.Get(slug)
	if err != nil {
		return map[string]any{"success": false, "error": "skill not found: " + slug}, nil
	}
	return map[string]any{
		"slug":        sf.Slug,
		"name":        sf.Name,
		"description": sf.Description,
		"content":     sf.Content,
	}, nil
}

func (s *Server) toolInstallSkills(args map[string]any) (map[string]any, error) {
	if s.skillStore == nil {
		return map[string]any{"success": false, "error": "skill store not available"}, nil
	}
	source, _ := args["source"].(string)
	if source == "" {
		return map[string]any{"success": false, "error": "source is required"}, nil
	}
	installed, errs := s.skillStore.InstallFromGitHub(source)
	return map[string]any{
		"installed": installed,
		"errors":    errs,
		"count":     len(installed),
	}, nil
}

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

	var parsed struct {
		Result any `json:"result"`
	}
	if err := json.Unmarshal(callJSON, &parsed); err != nil {
		return nil, fmt.Errorf("parse tools/call response: %w", err)
	}
	return parsed.Result, nil
}

// ═══════════════════════════════════════════════════════════════════════════
// Tool implementations
// ═══════════════════════════════════════════════════════════════════════════

// ── get_local_time ───────────────────────────────────────────────────────

var fixedOffsetTimezonePattern = regexp.MustCompile(`^(?i:(?:gmt|utc))\s*([+-])\s*(\d{1,2})(?::?(\d{2}))?$`)

func loadLocationIfValid(tz string) (*time.Location, bool) {
	tz = strings.TrimSpace(tz)
	if tz == "" {
		return nil, false
	}
	if loc, ok := loadFixedOffsetLocation(tz); ok {
		return loc, true
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, false
	}
	return loc, true
}

func loadFixedOffsetLocation(tz string) (*time.Location, bool) {
	m := fixedOffsetTimezonePattern.FindStringSubmatch(strings.TrimSpace(tz))
	if m == nil {
		return nil, false
	}
	hours, err := strconv.Atoi(m[2])
	if err != nil || hours > 23 {
		return nil, false
	}
	minutes := 0
	if m[3] != "" {
		minutes, err = strconv.Atoi(m[3])
		if err != nil || minutes > 59 {
			return nil, false
		}
	}
	offset := hours*3600 + minutes*60
	if strings.TrimSpace(m[1]) == "-" {
		offset = -offset
	}
	name := fmt.Sprintf("UTC%s%02d:%02d", m[1], hours, minutes)
	return time.FixedZone(name, offset), true
}

func preferredTimezoneName(ctx context.Context, cfg *configstore.UserConfig, requestedTZ string) string {
	if _, ok := loadLocationIfValid(requestedTZ); ok {
		return strings.TrimSpace(requestedTZ)
	}
	if clientTZ := clientTimezoneFromContext(ctx); clientTZ != "" {
		if _, ok := loadLocationIfValid(clientTZ); ok {
			return clientTZ
		}
	}
	configuredTZ := ""
	if cfg != nil {
		configuredTZ = strings.TrimSpace(cfg.Heartbeat.ActiveHours.TZ)
		if configuredTZ != "" && !strings.EqualFold(configuredTZ, "UTC") {
			if _, ok := loadLocationIfValid(configuredTZ); ok {
				return configuredTZ
			}
		}
	}
	if envTZ := strings.TrimSpace(os.Getenv("TZ")); envTZ != "" {
		if _, ok := loadLocationIfValid(envTZ); ok {
			return envTZ
		}
	}
	if localName := strings.TrimSpace(time.Local.String()); localName != "" && localName != "Local" {
		if _, ok := loadLocationIfValid(localName); ok {
			return localName
		}
	}
	if configuredTZ != "" {
		if _, ok := loadLocationIfValid(configuredTZ); ok {
			return configuredTZ
		}
	}
	return "UTC"
}

func preferredLocation(ctx context.Context, cfg *configstore.UserConfig, requestedTZ string) *time.Location {
	if loc, ok := loadLocationIfValid(preferredTimezoneName(ctx, cfg, requestedTZ)); ok {
		return loc
	}
	return time.UTC
}

func (s *Server) toolGetLocalTime(ctx context.Context, userID string, args map[string]any) map[string]any {
	var cfg *configstore.UserConfig
	if s != nil && s.configStore != nil && userID != "" {
		if c, err := s.configStore.GetUserConfig(userID); err == nil {
			cfg = &c
		}
	}
	requestedTZ, _ := args["timezone"].(string)
	loc := preferredLocation(ctx, cfg, requestedTZ)
	now := time.Now().In(loc)
	return map[string]any{
		"success":          true,
		"iso_datetime":     now.Format(time.RFC3339),
		"display_datetime": now.Format("Monday, January 2, 2006 at 3:04 PM"),
		"timezone":         loc.String(),
		"day_of_week":      now.Weekday().String(),
	}
}

// ── get_location ─────────────────────────────────────────────────────────

func (s *Server) toolGetLocation(ctx context.Context) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", "https://ipwho.is/", nil)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("request failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024))
	if err != nil {
		return map[string]any{"success": false, "error": "failed to read response"}, nil
	}

	var result map[string]any
	if err := json.Unmarshal(body, &result); err != nil {
		return map[string]any{"success": false, "error": "failed to parse response"}, nil
	}

	return result, nil
}

// ── web_search ───────────────────────────────────────────────────────────

func (s *Server) toolWebSearch(ctx context.Context, args map[string]any) (map[string]any, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return map[string]any{"success": false, "error": "query is required"}, nil
	}

	// Scrape DuckDuckGo Lite (no API key needed)
	url := "https://lite.duckduckgo.com/lite/?q=" + strings.ReplaceAll(query, " ", "+")
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; openCrow/1.0)")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("search failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return map[string]any{"success": false, "error": "failed to read results"}, nil
	}

	// Parse the DuckDuckGo Lite HTML for results
	results := parseDuckDuckGoLite(string(body))

	return map[string]any{
		"success": true,
		"query":   query,
		"results": results,
	}, nil
}

// parseDuckDuckGoLite extracts search results from DDG Lite HTML.
func parseDuckDuckGoLite(html string) []map[string]string {
	var results []map[string]string

	// DDG Lite uses a table-based layout. Links are in <a> tags with class "result-link"
	// or just plain <a> tags in result rows. We do a simple extraction.
	lines := strings.Split(html, "\n")
	for i, line := range lines {
		line = strings.TrimSpace(line)
		// Look for result links
		if !strings.Contains(line, "result-link") && !strings.Contains(line, "result__a") {
			continue
		}

		// Extract href
		hrefIdx := strings.Index(line, "href=\"")
		if hrefIdx == -1 {
			continue
		}
		hrefStart := hrefIdx + 6
		hrefEnd := strings.Index(line[hrefStart:], "\"")
		if hrefEnd == -1 {
			continue
		}
		href := line[hrefStart : hrefStart+hrefEnd]

		// Extract link text (title)
		title := extractTextContent(line)
		if title == "" {
			continue
		}

		// Look for snippet in nearby lines
		snippet := ""
		for j := i + 1; j < len(lines) && j < i+5; j++ {
			l := strings.TrimSpace(lines[j])
			if strings.Contains(l, "result-snippet") || strings.Contains(l, "result__snippet") {
				snippet = extractTextContent(l)
				break
			}
		}

		results = append(results, map[string]string{
			"title":   title,
			"url":     href,
			"snippet": snippet,
		})

		if len(results) >= 10 {
			break
		}
	}

	return results
}

// extractTextContent strips HTML tags from a string.
func extractTextContent(s string) string {
	var b strings.Builder
	inTag := false
	for _, r := range s {
		if r == '<' {
			inTag = true
			continue
		}
		if r == '>' {
			inTag = false
			continue
		}
		if !inTag {
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// ── open_url ─────────────────────────────────────────────────────────────

func (s *Server) toolOpenURL(ctx context.Context, args map[string]any) (map[string]any, error) {
	rawURL, _ := args["url"].(string)
	if rawURL == "" {
		return map[string]any{"success": false, "error": "url is required"}, nil
	}

	// Fetch page content and return it (server-side, we fetch instead of opening browser)
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; openCrow/1.0)")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("fetch failed: %v", err)}, nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil {
		return map[string]any{"success": false, "error": "failed to read page"}, nil
	}

	content := extractTextContent(string(body))
	if len(content) > 20000 {
		content = content[:20000] + "..."
	}

	return map[string]any{
		"success":      true,
		"url":          rawURL,
		"status_code":  resp.StatusCode,
		"content_type": resp.Header.Get("Content-Type"),
		"content":      content,
	}, nil
}

// ── Memory tools ─────────────────────────────────────────────────────────

func (s *Server) toolStoreMemory(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	content, _ := args["content"].(string)
	category, _ := args["category"].(string)
	if content == "" {
		return map[string]any{"success": false, "error": "content is required"}, nil
	}
	if category == "" {
		category = "general"
	}

	mem, err := s.createMemory(ctx, userID, category, content, 1)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to store memory: %v", err)}, nil
	}

	return map[string]any{
		"success":   true,
		"memory_id": mem.ID,
		"category":  mem.Category,
		"content":   mem.Content,
		"message":   "Memory stored successfully",
	}, nil
}

func (s *Server) toolForgetMemory(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	memoryID, _ := args["memoryId"].(string)
	if memoryID == "" {
		return map[string]any{"success": false, "error": "memoryId is required"}, nil
	}

	deleted, err := s.deleteMemory(ctx, userID, memoryID)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to forget memory: %v", err)}, nil
	}
	if !deleted {
		return map[string]any{"success": false, "error": "memory not found"}, nil
	}

	return map[string]any{"success": true, "message": "Memory forgotten"}, nil
}

func (s *Server) toolLearnMemory(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	content, _ := args["content"].(string)
	if content == "" {
		return map[string]any{"success": false, "error": "content is required"}, nil
	}

	mem, err := s.createMemory(ctx, userID, "LEARNING", content, 1)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to learn: %v", err)}, nil
	}

	return map[string]any{
		"success":   true,
		"memory_id": mem.ID,
		"category":  "LEARNING",
		"message":   "Learning stored successfully",
	}, nil
}

func (s *Server) toolReadMemory(ctx context.Context, userID string) (map[string]any, error) {
	memories, err := s.listMemories(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to read memories: %v", err)}, nil
	}
	entries := make([]map[string]any, 0, len(memories))
	for _, m := range memories {
		entries = append(entries, map[string]any{
			"id":         m.ID,
			"category":   m.Category,
			"content":    m.Content,
			"confidence": m.Confidence,
		})
	}
	return map[string]any{
		"success":  true,
		"count":    len(entries),
		"memories": entries,
	}, nil
}

func (s *Server) toolReinforceMemory(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	memoryID, _ := args["memoryId"].(string)
	if memoryID == "" {
		return map[string]any{"success": false, "error": "memoryId is required"}, nil
	}

	mem, err := s.reinforceMemory(ctx, userID, memoryID)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to reinforce: %v", err)}, nil
	}

	return map[string]any{
		"success":    true,
		"memory_id":  mem.ID,
		"confidence": mem.Confidence,
		"message":    "Memory reinforced",
	}, nil
}

func (s *Server) toolPromoteLearning(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	memoryID, _ := args["memoryId"].(string)
	if memoryID == "" {
		return map[string]any{"success": false, "error": "memoryId is required"}, nil
	}

	mem, err := s.promoteMemory(ctx, userID, memoryID)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to promote: %v", err)}, nil
	}

	return map[string]any{
		"success":   true,
		"memory_id": mem.ID,
		"category":  mem.Category,
		"message":   "Learning promoted to preferred behavior",
	}, nil
}

// ── Task tools ───────────────────────────────────────────────────────────

func (s *Server) toolListTasks(ctx context.Context, userID string) (map[string]any, error) {
	tasks, err := s.listTasks(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to list tasks: %v", err)}, nil
	}

	return map[string]any{
		"success": true,
		"count":   len(tasks),
		"tasks":   tasks,
	}, nil
}

func (s *Server) toolScheduleTask(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	prompt, _ := args["prompt"].(string)
	executeAtStr, _ := args["executeAt"].(string)
	description, _ := args["description"].(string)
	if prompt == "" || executeAtStr == "" {
		return map[string]any{"success": false, "error": "prompt and executeAt are required"}, nil
	}
	if description == "" {
		description = prompt
		if len(description) > 100 {
			description = description[:100] + "..."
		}
	}

	executeAt, err := time.Parse(time.RFC3339, executeAtStr)
	if err != nil {
		return map[string]any{"success": false, "error": "executeAt must be RFC3339 format"}, nil
	}

	var cronExpr *string
	if c, ok := args["cronExpression"].(string); ok && c != "" {
		cronExpr = &c
	}

	task, err := s.createTask(ctx, userID, description, prompt, executeAt, cronExpr)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to schedule task: %v", err)}, nil
	}

	return map[string]any{
		"success": true,
		"task_id": task.ID,
		"message": "Task scheduled successfully",
	}, nil
}

func (s *Server) toolCancelTask(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	taskID, _ := args["taskId"].(string)
	if taskID == "" {
		return map[string]any{"success": false, "error": "taskId is required"}, nil
	}

	deleted, err := s.deleteTask(ctx, userID, taskID)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to cancel task: %v", err)}, nil
	}
	if !deleted {
		return map[string]any{"success": false, "error": "task not found"}, nil
	}

	return map[string]any{"success": true, "message": "Task cancelled"}, nil
}

// ── Heartbeat tools ──────────────────────────────────────────────────────

func (s *Server) toolConfigureHeartbeat(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	req := UpdateHeartbeatConfigRequest{}
	if enabled, ok := args["enabled"].(bool); ok {
		req.Enabled = &enabled
	}
	if interval, ok := args["intervalSeconds"].(float64); ok {
		i := int(interval)
		req.IntervalSeconds = &i
	}

	cfg, err := s.putHeartbeatConfig(ctx, userID, req)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to configure heartbeat: %v", err)}, nil
	}

	return map[string]any{
		"success":          true,
		"enabled":          cfg.Enabled,
		"interval_seconds": cfg.IntervalSeconds,
		"message":          "Heartbeat configured",
	}, nil
}

func (s *Server) toolTriggerHeartbeat(ctx context.Context, userID string) (map[string]any, error) {
	evt, err := s.createHeartbeatEvent(ctx, userID, "TRIGGERED", "tool-triggered heartbeat")
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to trigger heartbeat: %v", err)}, nil
	}

	s.realtimeHub.Publish(realtime.Event{
		UserID:  userID,
		Type:    "heartbeat.triggered",
		Payload: map[string]any{"eventId": evt.ID},
	})

	return map[string]any{
		"success":  true,
		"event_id": evt.ID,
		"message":  "Heartbeat triggered",
	}, nil
}

// ── Email tools ──────────────────────────────────────────────────────────

func (s *Server) toolSetupEmail(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	address, _ := args["address"].(string)
	password, _ := args["password"].(string)
	address = strings.TrimSpace(strings.ToLower(address))
	if address == "" {
		return map[string]any{"success": false, "error": "address is required"}, nil
	}
	if password == "" {
		return map[string]any{"success": false, "error": "password (or app-specific password) is required"}, nil
	}

	// Auto-detect IMAP/SMTP settings from email domain
	imapHost, _ := args["imap_host"].(string)
	smtpHost, _ := args["smtp_host"].(string)
	imapPort := 993
	smtpPort := 587

	if imapHost == "" || smtpHost == "" {
		detected := autoDetectEmailServer(address)
		if detected != nil {
			if imapHost == "" {
				imapHost = detected.imapHost
			}
			if smtpHost == "" {
				smtpHost = detected.smtpHost
			}
			imapPort = detected.imapPort
			smtpPort = detected.smtpPort
		}
	}

	if imapHost == "" {
		return map[string]any{"success": false, "error": "Cannot auto-detect server settings. Please provide imap_host and smtp_host."}, nil
	}

	if p, ok := args["imap_port"].(float64); ok {
		imapPort = int(p)
	}
	if p, ok := args["smtp_port"].(float64); ok {
		smtpPort = int(p)
	}

	// Save to config store
	if s.configStore != nil {
		cfg, err := s.configStore.GetUserConfig(userID)
		if err != nil {
			return map[string]any{"success": false, "error": "failed to load config"}, nil
		}

		account := configstore.EmailAccountConfig{
			Label:        address,
			Address:      address,
			ImapHost:     imapHost,
			ImapPort:     imapPort,
			ImapUsername: address,
			ImapPassword: password,
			SmtpHost:     smtpHost,
			SmtpPort:     smtpPort,
			UseTLS:       true,
			Enabled:      true,
		}

		// Idempotent upsert by email address to avoid duplicates when
		// setup form already saved config and the model calls setup_email again.
		idx := -1
		for i, acc := range cfg.Integrations.EmailAccounts {
			if strings.EqualFold(strings.TrimSpace(acc.Address), address) {
				idx = i
				break
			}
		}
		if idx >= 0 {
			cfg.Integrations.EmailAccounts[idx] = account
		} else {
			cfg.Integrations.EmailAccounts = append(cfg.Integrations.EmailAccounts, account)
		}

		if _, err := s.configStore.PutUserConfig(userID, cfg); err != nil {
			return map[string]any{"success": false, "error": "failed to save config"}, nil
		}

		// Keep DB inbox rows in sync using upsert semantics as well.
		if err := s.syncEmailInboxesFromConfig(ctx, userID, cfg.Integrations.EmailAccounts); err != nil {
			return map[string]any{"success": false, "error": fmt.Sprintf("failed to sync inboxes: %v", err)}, nil
		}

		return map[string]any{
			"success":   true,
			"address":   address,
			"imap_host": imapHost,
			"smtp_host": smtpHost,
			"message":   "Email account configured (upserted). You can now use check_email, read_email, reply_email, compose_email, and search_email.",
		}, nil
	}

	// Fallback path: save directly to DB when config store is unavailable.
	inbox, err := s.createEmailInbox(ctx, userID, address, imapHost, imapPort, address, password, true, 60)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			return map[string]any{"success": false, "error": "email account already configured"}, nil
		}
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to save: %v", err)}, nil
	}

	return map[string]any{
		"success":   true,
		"inbox_id":  inbox.ID,
		"address":   address,
		"imap_host": imapHost,
		"smtp_host": smtpHost,
		"message":   "Email account configured. You can now use check_email, read_email, reply_email, compose_email, and search_email.",
	}, nil
}

type emailServerInfo struct {
	imapHost string
	imapPort int
	smtpHost string
	smtpPort int
}

func autoDetectEmailServer(email string) *emailServerInfo {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return nil
	}
	domain := strings.ToLower(parts[1])

	switch { //nolint:staticcheck // complex multi-value cases can't use tagged switch
	case domain == "gmail.com" || domain == "googlemail.com":
		return &emailServerInfo{"imap.gmail.com", 993, "smtp.gmail.com", 587}
	case domain == "outlook.com" || domain == "hotmail.com" || domain == "live.com":
		return &emailServerInfo{"outlook.office365.com", 993, "smtp.office365.com", 587}
	case domain == "yahoo.com" || domain == "ymail.com":
		return &emailServerInfo{"imap.mail.yahoo.com", 993, "smtp.mail.yahoo.com", 587}
	case domain == "icloud.com" || domain == "me.com" || domain == "mac.com":
		return &emailServerInfo{"imap.mail.me.com", 993, "smtp.mail.me.com", 587}
	case domain == "aol.com":
		return &emailServerInfo{"imap.aol.com", 993, "smtp.aol.com", 587}
	case domain == "protonmail.com" || domain == "proton.me" || domain == "pm.me":
		return &emailServerInfo{"127.0.0.1", 1143, "127.0.0.1", 1025} // ProtonMail Bridge
	default:
		// Generic: try imap.<domain> / smtp.<domain>
		return &emailServerInfo{"imap." + domain, 993, "smtp." + domain, 587}
	}
}

func (s *Server) toolCheckEmail(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	creds, err := s.getFirstEmailCredentials(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": "No active email accounts configured. Use setup_email first."}, nil
	}

	sess, err := dialIMAP(ctx, creds.ImapHost, creds.ImapPort, creds.ImapUsername, creds.ImapPassword, creds.UseTLS)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("IMAP connect failed: %v", err)}, nil
	}
	defer sess.close()

	count, err := sess.selectMailbox("INBOX")
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("SELECT INBOX failed: %v", err)}, nil
	}

	// Fetch last 10 message headers
	start := count - 9
	if start < 1 {
		start = 1
	}
	var messages []map[string]any
	if count > 0 {
		headers, herr := sess.fetchHeaders(start, count)
		if herr == nil {
			for i := len(headers) - 1; i >= 0; i-- {
				h := headers[i]
				messages = append(messages, map[string]any{
					"seq":     h.SeqNum,
					"subject": h.Subject,
					"from":    h.From,
					"date":    h.Date,
					"flags":   h.Flags,
				})
			}
		}
	}

	return map[string]any{
		"success":        true,
		"inbox":          creds.Address,
		"total_messages": count,
		"recent":         messages,
	}, nil
}

func (s *Server) toolReadEmail(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	seqStr, _ := args["messageId"].(string)
	if seqStr == "" {
		return map[string]any{"success": false, "error": "messageId (sequence number) is required"}, nil
	}
	seq := 0
	if n, err := fmt.Sscanf(seqStr, "%d", &seq); n != 1 || err != nil {
		return map[string]any{"success": false, "error": "messageId must be a numeric sequence number"}, nil
	}

	creds, err := s.getFirstEmailCredentials(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": "No active email accounts configured."}, nil
	}

	sess, err := dialIMAP(ctx, creds.ImapHost, creds.ImapPort, creds.ImapUsername, creds.ImapPassword, creds.UseTLS)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("IMAP connect failed: %v", err)}, nil
	}
	defer sess.close()

	if _, err := sess.selectMailbox("INBOX"); err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("SELECT INBOX failed: %v", err)}, nil
	}

	body, err := sess.fetchBody(seq, 8000)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("FETCH failed: %v", err)}, nil
	}

	return map[string]any{
		"success": true,
		"seq":     seq,
		"body":    body,
	}, nil
}

func (s *Server) toolSearchEmail(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return map[string]any{"success": false, "error": "query is required"}, nil
	}

	creds, err := s.getFirstEmailCredentials(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": "No active email accounts configured."}, nil
	}

	sess, err := dialIMAP(ctx, creds.ImapHost, creds.ImapPort, creds.ImapUsername, creds.ImapPassword, creds.UseTLS)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("IMAP connect failed: %v", err)}, nil
	}
	defer sess.close()

	if _, err := sess.selectMailbox("INBOX"); err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("SELECT INBOX failed: %v", err)}, nil
	}

	// Build IMAP search criteria: search subject OR body text
	criteria := fmt.Sprintf("OR SUBJECT %s TEXT %s", imapQuote(query), imapQuote(query))
	seqNums, err := sess.search(criteria)
	if err != nil {
		// Fallback to simpler subject-only search
		seqNums, err = sess.search("SUBJECT " + imapQuote(query))
		if err != nil {
			return map[string]any{"success": false, "error": fmt.Sprintf("SEARCH failed: %v", err)}, nil
		}
	}

	if len(seqNums) == 0 {
		return map[string]any{"success": true, "query": query, "count": 0, "results": []any{}}, nil
	}

	// Fetch headers for the last 10 matches
	limit := seqNums
	if len(limit) > 10 {
		limit = limit[len(limit)-10:]
	}
	headers, err := sess.fetchHeaders(limit[0], limit[len(limit)-1])
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("FETCH headers failed: %v", err)}, nil
	}

	results := make([]map[string]any, 0, len(headers))
	matchSet := make(map[int]bool, len(limit))
	for _, n := range limit {
		matchSet[n] = true
	}
	for _, h := range headers {
		if matchSet[h.SeqNum] {
			results = append(results, map[string]any{
				"seq":     h.SeqNum,
				"subject": h.Subject,
				"from":    h.From,
				"date":    h.Date,
			})
		}
	}

	return map[string]any{
		"success": true,
		"query":   query,
		"count":   len(seqNums),
		"results": results,
	}, nil
}

func (s *Server) toolReplyEmail(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
	return map[string]any{"success": false, "error": "SMTP reply is not yet implemented."}, nil
}

func (s *Server) toolComposeEmail(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	to, _ := args["to"].(string)
	subject, _ := args["subject"].(string)
	body, _ := args["body"].(string)
	if to == "" || subject == "" || body == "" {
		return map[string]any{"success": false, "error": "to, subject, and body are required"}, nil
	}

	creds, err := s.getFirstEmailCredentials(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": "No active email accounts configured. Use setup_email first."}, nil
	}

	smtpHost := creds.SmtpHost
	smtpPort := creds.SmtpPort
	// Fallback: auto-detect from domain
	if smtpHost == "" {
		if info := autoDetectEmailServer(creds.Address); info != nil {
			smtpHost = info.smtpHost
			smtpPort = info.smtpPort
		}
	}
	if smtpHost == "" {
		return map[string]any{"success": false, "error": "Cannot determine SMTP server. Please reconfigure the email account with smtp_host."}, nil
	}

	from := creds.Address
	msgBytes := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body,
	))

	addr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)
	auth := smtp.PlainAuth("", creds.ImapUsername, creds.ImapPassword, smtpHost)

	if err := smtp.SendMail(addr, auth, from, []string{to}, msgBytes); err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("SMTP send failed: %v", err)}, nil
	}

	return map[string]any{
		"success": true,
		"from":    from,
		"to":      to,
		"subject": subject,
		"message": "Email sent successfully.",
	}, nil
}

// ── Notification ─────────────────────────────────────────────────────────

func (s *Server) toolSendNotification(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	title, _ := args["title"].(string)
	body, _ := args["body"].(string)
	if title == "" || body == "" {
		return map[string]any{"success": false, "error": "title and body are required"}, nil
	}

	// Publish via realtime hub for connected clients to consume
	s.realtimeHub.Publish(realtime.Event{
		UserID: userID,
		Type:   "notification",
		Payload: map[string]any{
			"title": title,
			"body":  body,
		},
	})

	// Fanout to Telegram bots with a notificationChatId configured
	telegramSent := 0
	if s.configStore != nil {
		if cfg, err := s.configStore.GetUserConfig(userID); err == nil {
			msg := fmt.Sprintf("[notification] *%s*\n%s", title, body)
			for _, bot := range cfg.Integrations.TelegramBots {
				if !bot.Enabled || bot.BotToken == "" || bot.NotificationChatID == "" {
					continue
				}
				if err := s.TelegramSendNotification(ctx, bot.BotToken, bot.NotificationChatID, msg); err != nil {
					log.Printf("[send_notification] telegram bot %s error: %v", bot.Label, err)
				} else {
					telegramSent++
				}
			}
		}
	}

	detail := "Notification sent via realtime hub."
	if telegramSent > 0 {
		detail += fmt.Sprintf(" Also sent to %d Telegram bot(s).", telegramSent)
	}
	return map[string]any{
		"success": true,
		"message": detail,
	}, nil
}

// notifyChannels fans out a title+body notification to all enabled Telegram bots
// that have a notificationChatId configured. It is safe to call from any worker.
func (s *Server) notifyChannels(ctx context.Context, userID, title, body string) {
	if s.configStore == nil {
		return
	}
	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		return
	}
	msg := fmt.Sprintf("[notification] *%s*\n%s", title, body)
	for _, bot := range cfg.Integrations.TelegramBots {
		if !bot.Enabled || bot.BotToken == "" || bot.NotificationChatID == "" {
			continue
		}
		if err := s.TelegramSendNotification(ctx, bot.BotToken, bot.NotificationChatID, msg); err != nil {
			log.Printf("[channels] telegram bot %s notify error: %v", bot.Label, err)
		}
	}
}

// ── Shell tools ──────────────────────────────────────────────────────────

func (s *Server) toolExecuteShellCommand(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return map[string]any{"success": false, "error": "command is required"}, nil
	}

	timeout := s.serverShellTimeout
	if t, ok := args["timeout"].(float64); ok && t > 0 {
		timeout = time.Duration(t) * time.Second
	}

	workingDir, _ := args["working_dir"].(string)

	var env map[string]string
	if envMap, ok := args["env"].(map[string]any); ok {
		env = make(map[string]string, len(envMap))
		for k, v := range envMap {
			env[k] = fmt.Sprint(v)
		}
	}

	// Shell is fixed.
	shell := "/bin/bash"

	// Background mode
	if bg, ok := args["background"].(bool); ok && bg {
		result := processManager.StartBackground(ctx, shell, command, timeout, workingDir, env)
		s.writeCommandToTerminal(userID, command, "", true)
		return result, nil
	}

	result := executeShellCommand(ctx, shell, command, timeout, workingDir, env)
	// Mirror command + output into user's xterm PTY (if connected)
	stdout, _ := result["stdout"].(string)
	stderr, _ := result["stderr"].(string)
	combined := stdout
	if stderr != "" {
		combined += stderr
	}
	s.writeCommandToTerminal(userID, command, combined, false)
	return result, nil
}

func (s *Server) toolRemoteExecute(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	serverName, _ := args["serverName"].(string)
	if serverName == "" {
		return map[string]any{"success": false, "error": "serverName is required"}, nil
	}
	command, _ := args["command"].(string)
	if command == "" {
		return map[string]any{"success": false, "error": "command is required"}, nil
	}

	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		return map[string]any{"success": false, "error": "failed to load config"}, nil
	}

	var srv *configstore.SSHServerConfig
	for i := range cfg.Integrations.SSHServers {
		if strings.EqualFold(cfg.Integrations.SSHServers[i].Name, serverName) {
			srv = &cfg.Integrations.SSHServers[i]
			break
		}
	}
	if srv == nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("server %q not found", serverName)}, nil
	}
	if !srv.Enabled {
		return map[string]any{"success": false, "error": fmt.Sprintf("server %q is disabled", serverName)}, nil
	}

	timeout := 300 * time.Second
	if t, ok := args["timeout"].(float64); ok && t > 0 {
		timeout = time.Duration(t) * time.Second
	}
	workingDir, _ := args["working_dir"].(string)
	background, _ := args["background"].(bool)

	sshCfg, err := buildSSHClientConfig(srv.Username, srv.AuthMode, srv.SSHKey, srv.Password, srv.Passphrase)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}

	port := srv.Port
	if port <= 0 {
		port = 22
	}
	addr := fmt.Sprintf("%s:%d", srv.Host, port)
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return map[string]any{"success": false, "error": "ssh dial: " + err.Error()}, nil
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return map[string]any{"success": false, "error": "ssh session: " + err.Error()}, nil
	}
	defer session.Close()

	cmd := command
	if workingDir != "" {
		cmd = fmt.Sprintf("cd %s && %s", workingDir, cmd)
	}
	if background {
		cmd = fmt.Sprintf("nohup sh -c %s </dev/null >nohup.out 2>&1 & echo $!", shellescape(cmd))
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		out []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := session.CombinedOutput(cmd)
		ch <- result{out, err}
	}()

	select {
	case <-execCtx.Done():
		_ = session.Signal(ssh.SIGKILL)
		return map[string]any{"success": false, "stdout": "", "stderr": "timeout", "exitCode": -1}, nil
	case res := <-ch:
		exitCode := 0
		if res.err != nil {
			if exitErr, ok := res.err.(*ssh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
			} else {
				return map[string]any{"success": false, "error": res.err.Error()}, nil
			}
		}
		output := truncateOutput(string(res.out), 32000)
		return map[string]any{"success": true, "stdout": output, "stderr": "", "exitCode": exitCode}, nil
	}
}

func shellescape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func (s *Server) toolManageProcess(args map[string]any) (map[string]any, error) {
	action, _ := args["action"].(string)
	if action == "" {
		return map[string]any{"success": false, "error": "action is required (list, log, kill, remove)"}, nil
	}

	switch action {
	case "list":
		return processManager.List(), nil
	case "log":
		sessionID, _ := args["session_id"].(string)
		if sessionID == "" {
			return map[string]any{"success": false, "error": "session_id is required for log"}, nil
		}
		offset := 0
		limit := 200
		if o, ok := args["offset"].(float64); ok {
			offset = int(o)
		}
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}
		return processManager.Log(sessionID, offset, limit), nil
	case "kill":
		sessionID, _ := args["session_id"].(string)
		if sessionID == "" {
			return map[string]any{"success": false, "error": "session_id is required for kill"}, nil
		}
		return processManager.Kill(sessionID), nil
	case "remove":
		sessionID, _ := args["session_id"].(string)
		if sessionID == "" {
			return map[string]any{"success": false, "error": "session_id is required for remove"}, nil
		}
		return processManager.Remove(sessionID), nil
	default:
		return map[string]any{"success": false, "error": fmt.Sprintf("unknown action: %s. Use: list, log, kill, remove", action)}, nil
	}
}

// writeCommandToTerminal mirrors an AI-executed shell command + its output into
// the user's persistent PTY terminal session (so it appears in TerminalView).
// Uses ANSI escape codes to visually distinguish AI-driven commands from manual ones.
func (s *Server) writeCommandToTerminal(userID, command, output string, background bool) {
	if s.termMgr == nil {
		return
	}

	bgSuffix := ""
	if background {
		bgSuffix = " &"
	}

	// Build the display block:
	// Dim separator, bold cyan prompt prefix, command, output (if any)
	var buf strings.Builder
	buf.WriteString("\r\n\x1b[2m-- [AI] command --------------------\x1b[0m\r\n")
	buf.WriteString("\x1b[1;36m> \x1b[0m\x1b[1m")
	buf.WriteString(command)
	buf.WriteString(bgSuffix)
	buf.WriteString("\x1b[0m\r\n")
	if output != "" {
		// Prefix each line so it's indented slightly
		for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
			buf.WriteString("  ")
			buf.WriteString(line)
			buf.WriteString("\r\n")
		}
	}
	buf.WriteString("\x1b[2m───────────────────────────────────\x1b[0m\r\n")

	s.termMgr.BroadcastOutput(userID, []byte(buf.String()))
}

func (s *Server) writeToolCallToTerminal(userID, kind, name string, args map[string]any, result any, execErr error) {
	if s.termMgr == nil {
		return
	}

	if kind == "" {
		kind = "TOOL"
	}

	argsJSON, _ := json.Marshal(args)
	resultJSON, _ := json.Marshal(result)

	status := "ok"
	preview := ""
	if execErr != nil {
		status = "error"
		preview = execErr.Error()
	} else {
		preview = string(resultJSON)
	}

	var buf strings.Builder
	buf.WriteString("\r\n\x1b[2m-- [AI] tool -----------------------\x1b[0m\r\n")
	buf.WriteString("\x1b[1;35m[")
	buf.WriteString(kind)
	buf.WriteString("]\x1b[0m ")
	buf.WriteString("\x1b[1m")
	buf.WriteString(name)
	buf.WriteString("\x1b[0m")
	if len(argsJSON) > 0 && string(argsJSON) != "null" {
		buf.WriteString(" ")
		buf.WriteString(truncateOutput(string(argsJSON), 240))
	}
	buf.WriteString("\r\n")
	buf.WriteString("  status: ")
	buf.WriteString(status)
	buf.WriteString("\r\n")
	if strings.TrimSpace(preview) != "" {
		for _, line := range strings.Split(strings.TrimRight(truncateOutput(preview, 420), "\n"), "\n") {
			buf.WriteString("  ")
			buf.WriteString(line)
			buf.WriteString("\r\n")
		}
	}
	buf.WriteString("\x1b[2m───────────────────────────────────\x1b[0m\r\n")

	s.termMgr.BroadcastOutput(userID, []byte(buf.String()))
}
