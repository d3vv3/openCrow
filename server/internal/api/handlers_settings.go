package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/opencrow/opencrow/server/internal/configstore"
	"github.com/opencrow/opencrow/server/internal/orchestrator"
)

func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	settings, err := s.getSettings(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handlePutSettings(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var req UpdateSettingsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Settings == nil {
		writeError(w, http.StatusBadRequest, "settings object required")
		return
	}

	settings, err := s.putSettings(r.Context(), userID, req.Settings)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to save settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

func (s *Server) handleGetUserConfig(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())

	if s.configStore == nil {
		writeError(w, http.StatusServiceUnavailable, "config store not configured")
		return
	}

	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load config")
		return
	}

	if servers, found, mcpErr := s.getMCPServersSetting(r.Context(), userID); mcpErr != nil {
		log.Printf("[config] failed to load MCP settings for user %s: %v", userID, mcpErr)
	} else if found {
		cfg.MCP.Servers = servers
	}

	writeJSON(w, http.StatusOK, cfg)
}

func (s *Server) handlePutUserConfig(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())

	if s.configStore == nil {
		writeError(w, http.StatusServiceUnavailable, "config store not configured")
		return
	}

	var cfg configstore.UserConfig
	// Use a lenient decoder here: the config schema evolves and clients may
	// send extra/renamed fields (e.g. both "tls" and "useTls" during transitions).
	if err := json.NewDecoder(r.Body).Decode(&cfg); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	saved, err := s.configStore.PutUserConfig(userID, cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to save config")
		return
	}

	// Sync email accounts to DB so the email worker and LLM tools can see them
	if len(saved.Integrations.EmailAccounts) > 0 {
		if syncErr := s.syncEmailInboxesFromConfig(r.Context(), userID, saved.Integrations.EmailAccounts); syncErr != nil {
			log.Printf("[config] failed to sync email inboxes for user %s: %v", userID, syncErr)
		}
	}

	if _, hbErr := s.putHeartbeatConfig(r.Context(), userID, UpdateHeartbeatConfigRequest{
		Enabled:         &saved.Heartbeat.Enabled,
		IntervalSeconds: &saved.Heartbeat.IntervalSeconds,
	}); hbErr != nil {
		log.Printf("[config] failed to sync heartbeat config for user %s: %v", userID, hbErr)
	}

	if mcpErr := s.putMCPServersSetting(r.Context(), userID, saved.MCP.Servers); mcpErr != nil {
		log.Printf("[config] failed to sync MCP settings for user %s: %v", userID, mcpErr)
	}

	writeJSON(w, http.StatusOK, saved)
}

type ProviderTestRequest struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKeyRef"`
	Model   string `json:"model"`
}

type ProviderTestResult struct {
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
	Model     string `json:"model,omitempty"`
}

func (s *Server) handleTestProvider(w http.ResponseWriter, r *http.Request) {
	var req ProviderTestRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Kind == "" {
		writeError(w, http.StatusBadRequest, "kind is required")
		return
	}

	prov := orchestrator.BuildProvider(req.Name, req.Kind, req.BaseURL, req.APIKey, req.Model)
	if prov == nil {
		writeJSON(w, http.StatusOK, ProviderTestResult{OK: false, Error: "unknown provider kind: " + req.Kind})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	start := time.Now()
	_, _, err := prov.Chat(ctx, "", []orchestrator.ChatMessage{{Role: "user", Content: "Respond with exactly: OK"}}, nil)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		writeJSON(w, http.StatusOK, ProviderTestResult{OK: false, LatencyMs: latency, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ProviderTestResult{OK: true, LatencyMs: latency, Model: req.Model})
}

type ProviderStatusEntry struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Model     string `json:"model"`
	Enabled   bool   `json:"enabled"`
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latencyMs"`
	Error     string `json:"error,omitempty"`
}

func (s *Server) handleProvidersStatus(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())

	if s.configStore == nil {
		writeError(w, http.StatusServiceUnavailable, "config store not configured")
		return
	}

	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load config")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	results := make([]ProviderStatusEntry, 0, len(cfg.LLM.Providers))
	for _, p := range cfg.LLM.Providers {
		entry := ProviderStatusEntry{
			Name:    p.Name,
			Kind:    p.Kind,
			Model:   p.Model,
			Enabled: p.Enabled,
		}
		if p.Enabled {
			prov := orchestrator.BuildProvider(p.Name, p.Kind, p.BaseURL, p.APIKeyRef, p.Model)
			if prov == nil {
				entry.Error = "unknown provider kind"
			} else {
				start := time.Now()
				_, _, probeErr := prov.Chat(ctx, "", []orchestrator.ChatMessage{{Role: "user", Content: "Respond with exactly: OK"}}, nil)
				entry.LatencyMs = time.Since(start).Milliseconds()
				if probeErr != nil {
					entry.Error = probeErr.Error()
				} else {
					entry.OK = true
				}
			}
		}
		results = append(results, entry)
	}

	writeJSON(w, http.StatusOK, map[string]any{"providers": results})
}

func (s *Server) handleTestMCPServer(w http.ResponseWriter, r *http.Request) {
	var req MCPServerTestRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	url := strings.TrimSpace(req.URL)
	if url == "" {
		writeError(w, http.StatusBadRequest, "url is required")
		return
	}

	headers := map[string]string{}
	for k, v := range req.Headers {
		key := strings.TrimSpace(k)
		if key == "" {
			continue
		}
		headers[key] = strings.TrimSpace(v)
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	tools, err := fetchMCPTools(ctx, url, headers)
	latency := time.Since(start).Milliseconds()
	if err != nil {
		writeJSON(w, http.StatusOK, MCPServerTestResult{OK: false, LatencyMs: latency, Error: err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, MCPServerTestResult{OK: true, LatencyMs: latency, Tools: tools})
}

func fetchMCPTools(ctx context.Context, serverURL string, headers map[string]string) ([]MCPToolSummary, error) {
	headerNames := make([]string, 0, len(headers))
	for k := range headers {
		if strings.TrimSpace(k) == "" {
			continue
		}
		headerNames = append(headerNames, k)
	}
	sort.Strings(headerNames)

	callRPC := func(payload any, sessionID string) ([]byte, string, error) {
		b, err := json.Marshal(payload)
		if err != nil {
			return nil, sessionID, err
		}

		var rpcMethod string
		if m, ok := payload.(map[string]any); ok {
			if mv, ok := m["method"].(string); ok {
				rpcMethod = mv
			}
		}
		if rpcMethod == "" {
			rpcMethod = "unknown"
		}
		log.Printf("[mcp-test] -> %s method=%s session=%q headerKeys=%v body=%s", serverURL, rpcMethod, sessionID, headerNames, truncateOutput(string(b), 1200))

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL, strings.NewReader(string(b)))
		if err != nil {
			return nil, sessionID, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json, text/event-stream")
		if sessionID != "" {
			req.Header.Set("Mcp-Session-Id", sessionID)
		}
		for k, v := range headers {
			req.Header.Set(k, v)
		}

		client := &http.Client{Timeout: 20 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, sessionID, err
		}
		defer resp.Body.Close()

		if sid := strings.TrimSpace(resp.Header.Get("Mcp-Session-Id")); sid != "" {
			sessionID = sid
		}

		body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
		if err != nil {
			return nil, sessionID, err
		}
		log.Printf("[mcp-test] <- %s method=%s status=%d session=%q response=%s", serverURL, rpcMethod, resp.StatusCode, sessionID, truncateOutput(strings.TrimSpace(string(body)), 1200))
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			wwwAuth := strings.TrimSpace(resp.Header.Get("Www-Authenticate"))
			reqID := strings.TrimSpace(resp.Header.Get("X-Request-Id"))
			extra := ""
			if wwwAuth != "" {
				extra += fmt.Sprintf("; www-authenticate: %s", wwwAuth)
			}
			if reqID != "" {
				extra += fmt.Sprintf("; request-id: %s", reqID)
			}
			if len(headerNames) > 0 {
				return nil, sessionID, fmt.Errorf("mcp http %d: %s (url: %s; sent header keys: %s%s)", resp.StatusCode, strings.TrimSpace(string(body)), serverURL, strings.Join(headerNames, ", "), extra)
			}
			return nil, sessionID, fmt.Errorf("mcp http %d: %s (url: %s%s)", resp.StatusCode, strings.TrimSpace(string(body)), serverURL, extra)
		}
		return body, sessionID, nil
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

	_, _, _ = callRPC(map[string]any{
		"jsonrpc": "2.0",
		"method":  "notifications/initialized",
		"params":  map[string]any{},
	}, sessionID)

	listBody, _, err := callRPC(map[string]any{
		"jsonrpc": "2.0",
		"id":      "tools-1",
		"method":  "tools/list",
		"params":  map[string]any{},
	}, sessionID)
	if err != nil {
		return nil, fmt.Errorf("tools/list failed: %w", err)
	}
	listJSON, err := mcpResponseJSONBytes(listBody)
	if err != nil {
		return nil, fmt.Errorf("parse tools/list response: %w", err)
	}
	if err := mcpJSONRPCError(listJSON); err != nil {
		return nil, fmt.Errorf("tools/list rpc error: %w", err)
	}

	var parsed struct {
		Result struct {
			Tools []struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				InputSchema map[string]any `json:"inputSchema"`
			} `json:"tools"`
		} `json:"result"`
	}
	if err := json.Unmarshal(listJSON, &parsed); err != nil {
		return nil, fmt.Errorf("parse tools/list response: %w", err)
	}

	out := make([]MCPToolSummary, 0, len(parsed.Result.Tools))
	for _, t := range parsed.Result.Tools {
		out = append(out, MCPToolSummary{Name: t.Name, Description: t.Description, InputSchema: t.InputSchema})
	}
	return out, nil
}

func mcpResponseJSONBytes(body []byte) ([]byte, error) {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return nil, fmt.Errorf("empty response body")
	}
	if json.Valid([]byte(trimmed)) {
		return []byte(trimmed), nil
	}

	// Some MCP servers respond over SSE framing, e.g.:
	// event: message
	// data: { ...json-rpc... }
	lines := strings.Split(trimmed, "\n")
	dataLines := make([]string, 0)
	for _, line := range lines {
		l := strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(l), "data:") {
			continue
		}
		payload := strings.TrimSpace(l[len("data:"):])
		if payload == "" || payload == "[DONE]" {
			continue
		}
		dataLines = append(dataLines, payload)
	}
	if len(dataLines) == 0 {
		return nil, fmt.Errorf("no JSON payload in response: %s", truncateOutput(trimmed, 220))
	}

	joined := strings.Join(dataLines, "\n")
	if json.Valid([]byte(joined)) {
		return []byte(joined), nil
	}
	for _, payload := range dataLines {
		if json.Valid([]byte(payload)) {
			return []byte(payload), nil
		}
	}

	return nil, fmt.Errorf("invalid JSON payload in response: %s", truncateOutput(joined, 220))
}

func mcpJSONRPCError(body []byte) error {
	var payload struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	if payload.Error == nil {
		return nil
	}
	return fmt.Errorf("code %d: %s", payload.Error.Code, payload.Error.Message)
}

func (s *Server) handleGetToolsConfig(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())

	if s.configStore == nil {
		writeError(w, http.StatusServiceUnavailable, "config store not configured")
		return
	}

	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load tools")
		return
	}

	writeJSON(w, http.StatusOK, cfg.Tools)
}

func (s *Server) handlePutToolsConfig(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())

	if s.configStore == nil {
		writeError(w, http.StatusServiceUnavailable, "config store not configured")
		return
	}

	var tools configstore.ToolsConfig
	if err := decodeJSON(r, &tools); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load user config")
		return
	}

	cfg.Tools = tools
	saved, err := s.configStore.PutUserConfig(userID, cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to save tools")
		return
	}

	writeJSON(w, http.StatusOK, saved.Tools)
}

func (s *Server) handleGetSkillsConfig(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())

	if s.configStore == nil {
		writeError(w, http.StatusServiceUnavailable, "config store not configured")
		return
	}

	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load skills")
		return
	}

	writeJSON(w, http.StatusOK, cfg.Skills)
}

func (s *Server) handlePutSkillsConfig(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())

	if s.configStore == nil {
		writeError(w, http.StatusServiceUnavailable, "config store not configured")
		return
	}

	var skills configstore.SkillsConfig
	if err := decodeJSON(r, &skills); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load user config")
		return
	}

	cfg.Skills = skills
	saved, err := s.configStore.PutUserConfig(userID, cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to save skills")
		return
	}

	writeJSON(w, http.StatusOK, saved.Skills)
}
