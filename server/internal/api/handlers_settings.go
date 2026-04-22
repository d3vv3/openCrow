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

// @Summary Get user settings
// @Tags    config
// @Security BearerAuth
// @Produce json
// @Success 200 {object} UserSettingsDTO
// @Failure 401 {object} ErrorResponse
// @Router  /v1/settings [get]
func (s *Server) handleGetSettings(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	settings, err := s.getSettings(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load settings")
		return
	}
	writeJSON(w, http.StatusOK, settings)
}

// @Summary Update user settings
// @Tags    config
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body UpdateSettingsRequest true "Settings map"
// @Success 200 {object} UserSettingsDTO
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/settings [put]
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

// @Summary Get full user config (providers, tools, integrations, etc.)
// @Tags    config
// @Security BearerAuth
// @Produce json
// @Success 200 {object} configstore.UserConfig
// @Failure 401 {object} ErrorResponse
// @Router  /v1/config [get]
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

// @Summary Replace full user config
// @Tags    config
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body configstore.UserConfig true "Full user config"
// @Success 200 {object} configstore.UserConfig
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/config [put]
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

	// Snapshot current companion app IDs before saving so we can detect removals.
	prevAppIDs := map[string]struct{}{}
	if prev, err := s.configStore.GetUserConfig(userID); err == nil {
		for _, app := range prev.Integrations.CompanionApps {
			if app.ID != "" {
				prevAppIDs[app.ID] = struct{}{}
			}
		}
	}

	saved, err := s.configStore.PutUserConfig(userID, cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to save config")
		return
	}

	// Cascade-delete registrations and pending tasks for removed companion apps.
	newAppIDs := map[string]struct{}{}
	for _, app := range saved.Integrations.CompanionApps {
		if app.ID != "" {
			newAppIDs[app.ID] = struct{}{}
		}
	}
	for id := range prevAppIDs {
		if _, stillPresent := newAppIDs[id]; stillPresent {
			continue
		}
		if err := s.deleteDeviceRegistration(r.Context(), userID, id); err != nil {
			log.Printf("[config] failed to delete registration for removed device %s: %v", id, err)
		}
		if err := s.deleteDeviceTasksByTarget(r.Context(), userID, id); err != nil {
			log.Printf("[config] failed to delete tasks for removed device %s: %v", id, err)
		}
	}

	// Sync email accounts to DB (reconcile: upserts present accounts, deletes removed ones)
	if syncErr := s.syncEmailInboxesFromConfig(r.Context(), userID, saved.Integrations.EmailAccounts); syncErr != nil {
		log.Printf("[config] failed to sync email inboxes for user %s: %v", userID, syncErr)
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

type ProviderModelsRequest struct {
	Kind    string `json:"kind"`
	BaseURL string `json:"baseUrl"`
	APIKey  string `json:"apiKeyRef"`
}

type ProviderModelsResponse struct {
	OK     bool     `json:"ok"`
	Models []string `json:"models,omitempty"`
	Error  string   `json:"error,omitempty"`
}

// @Summary Test an LLM provider configuration
// @Tags    config
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body ProviderTestRequest true "Provider connection details"
// @Success 200 {object} ProviderTestResult
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/providers/test [post]
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

	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()

	prov, provErr := orchestrator.BuildProvider(ctx, req.Name, req.Kind, req.BaseURL, req.APIKey, req.Model)
	if provErr != nil {
		writeJSON(w, http.StatusOK, ProviderTestResult{OK: false, Error: provErr.Error()})
		return
	}
	if prov == nil {
		writeJSON(w, http.StatusOK, ProviderTestResult{OK: false, Error: "unknown provider kind: " + req.Kind})
		return
	}

	start := time.Now()
	_, _, _, err := prov.Chat(ctx, "", []orchestrator.ChatMessage{{Role: "user", Content: "Respond with exactly: OK"}}, nil)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		writeJSON(w, http.StatusOK, ProviderTestResult{OK: false, LatencyMs: latency, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ProviderTestResult{OK: true, LatencyMs: latency, Model: req.Model})
}

func isOpenAICompatibleKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "openai", "custom", "openrouter", "litellm":
		return true
	default:
		return false
	}
}

func resolveOpenAICompatibleBaseURL(kind, baseURL string) string {
	k := strings.ToLower(strings.TrimSpace(kind))
	b := strings.TrimSpace(baseURL)
	if b != "" {
		return b
	}
	switch k {
	case "openrouter":
		return "https://openrouter.ai/api"
	case "litellm":
		return "http://localhost:4000"
	default:
		return "https://api.openai.com"
	}
}

func openAIModelsEndpoint(baseURL string) string {
	b := strings.TrimRight(strings.TrimSpace(baseURL), "/")
	lb := strings.ToLower(b)
	if strings.HasSuffix(lb, "/models") {
		return b
	}
	if strings.HasSuffix(lb, "/v1") {
		return b + "/models"
	}
	return b + "/v1/models"
}

func fetchOpenAICompatibleModels(ctx context.Context, kind, baseURL, apiKey string) ([]string, error) {
	resolvedBase := resolveOpenAICompatibleBaseURL(kind, baseURL)
	url := openAIModelsEndpoint(resolvedBase)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/json")
	if strings.TrimSpace(apiKey) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(apiKey))
	}

	resp, err := (&http.Client{Timeout: 20 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("models probe http %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var payload struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
		Models []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil, fmt.Errorf("invalid models response: %w", err)
	}

	seen := map[string]struct{}{}
	models := make([]string, 0, len(payload.Data)+len(payload.Models))
	appendModel := func(id, name string) {
		m := strings.TrimSpace(id)
		if m == "" {
			m = strings.TrimSpace(name)
		}
		if m == "" {
			return
		}
		if _, ok := seen[m]; ok {
			return
		}
		seen[m] = struct{}{}
		models = append(models, m)
	}
	for _, m := range payload.Data {
		appendModel(m.ID, m.Name)
	}
	for _, m := range payload.Models {
		appendModel(m.ID, m.Name)
	}

	sort.Strings(models)
	return models, nil
}

// @Summary Probe available models from a provider
// @Tags    config
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body ProviderModelsRequest true "Provider kind and credentials"
// @Success 200 {object} ProviderModelsResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/providers/models [post]
func (s *Server) handleProbeProviderModels(w http.ResponseWriter, r *http.Request) {
	var req ProviderModelsRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !isOpenAICompatibleKind(req.Kind) {
		writeJSON(w, http.StatusOK, ProviderModelsResponse{OK: false, Error: "model probing only supported for OpenAI-compatible providers"})
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	models, err := fetchOpenAICompatibleModels(ctx, req.Kind, req.BaseURL, req.APIKey)
	if err != nil {
		writeJSON(w, http.StatusOK, ProviderModelsResponse{OK: false, Error: err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, ProviderModelsResponse{OK: true, Models: models})
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

// @Summary Get connectivity status for all configured providers
// @Tags    config
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string][]ProviderStatusEntry
// @Failure 401 {object} ErrorResponse
// @Router  /v1/providers/status [get]
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
			prov, provErr := orchestrator.BuildProvider(ctx, p.Name, p.Kind, p.BaseURL, p.APIKeyRef, p.Model)
			if provErr != nil {
				entry.Error = provErr.Error()
			} else if prov == nil {
				entry.Error = "unknown provider kind"
			} else {
				start := time.Now()
				_, _, _, probeErr := prov.Chat(ctx, "", []orchestrator.ChatMessage{{Role: "user", Content: "Respond with exactly: OK"}}, nil)
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

// @Summary Test an MCP server connection and list its tools
// @Tags    config
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body MCPServerTestRequest true "MCP server URL and headers"
// @Success 200 {object} MCPServerTestResult
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/mcp/test [post]
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
				Name        string         `json:"name"`
				Description string         `json:"description"`
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

// @Summary Get tool configuration for the current user
// @Tags    config
// @Security BearerAuth
// @Produce json
// @Success 200 {object} configstore.ToolsConfig
// @Failure 401 {object} ErrorResponse
// @Router  /v1/tools [get]
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

// @Summary Update tool configuration for the current user
// @Tags    config
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body configstore.ToolsConfig true "Tools config"
// @Success 200 {object} configstore.ToolsConfig
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/tools [put]
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

// @Summary Get skills configuration for the current user
// @Tags    config
// @Security BearerAuth
// @Produce json
// @Success 200 {object} configstore.SkillsConfig
// @Failure 401 {object} ErrorResponse
// @Router  /v1/skills [get]
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

// @Summary Update skills configuration for the current user
// @Tags    config
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body configstore.SkillsConfig true "Skills config"
// @Success 200 {object} configstore.SkillsConfig
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/skills [put]
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

// @Summary Test a Telegram bot token
// @Tags    config
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body TestTelegramBotRequest true "Bot token and optional notification chat ID"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/telegram/test [post]
// handleTestTelegramBot verifies a bot token by calling getMe and optionally sends a test message.
func (s *Server) handleTestTelegramBot(w http.ResponseWriter, r *http.Request) {
	var req struct {
		BotToken           string `json:"botToken"`
		NotificationChatID string `json:"notificationChatId"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.BotToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "botToken is required"})
		return
	}

	type tgUser struct {
		ID        int64  `json:"id"`
		IsBot     bool   `json:"is_bot"`
		FirstName string `json:"first_name"`
		Username  string `json:"username"`
	}
	type getMeResp struct {
		OK     bool   `json:"ok"`
		Result tgUser `json:"result"`
	}

	start := time.Now()
	apiURL := fmt.Sprintf("https://api.telegram.org/bot%s/getMe", req.BotToken)
	httpReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, apiURL, nil)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer resp.Body.Close()
	latencyMs := time.Since(start).Milliseconds()

	var getMeResult getMeResp
	if err := json.NewDecoder(resp.Body).Decode(&getMeResult); err != nil || !getMeResult.OK {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "latencyMs": latencyMs, "error": "invalid bot token or Telegram API error"})
		return
	}

	bot := getMeResult.Result
	detail := fmt.Sprintf("@%s (id: %d)", bot.Username, bot.ID)

	// Note: no test message is sent to avoid polluting the chat.
	// Token validity is confirmed via getMe above.

	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"latencyMs": latencyMs,
		"detail":    detail,
	})
}
