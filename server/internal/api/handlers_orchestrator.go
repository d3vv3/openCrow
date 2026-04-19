package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/opencrow/opencrow/server/internal/configstore"
	"github.com/opencrow/opencrow/server/internal/orchestrator"
	"github.com/opencrow/opencrow/server/internal/realtime"
)

// @Summary Run a full orchestrator completion (blocking)
// @Tags    orchestrator
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body CompleteRequest true "Message and conversation ID"
// @Success 200 {object} CompleteResponse
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 502 {object} ErrorResponse
// @Router  /v1/orchestrator/complete [post]
func (s *Server) handleOrchestratorComplete(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var req CompleteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	message := strings.TrimSpace(req.Message)
	if message == "" {
		writeError(w, http.StatusBadRequest, "message is required")
		return
	}
	if !isUUID(req.ConversationID) {
		writeError(w, http.StatusBadRequest, "conversationId is required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	// Load user config once
	var cfg *configstore.UserConfig
	if s.configStore != nil {
		if c, err := s.configStore.GetUserConfig(userID); err == nil {
			cfg = &c
		}
	}

	// Build system prompt (includes injected memories)
	systemPrompt := s.buildSystemPrompt(ctx, userID, cfg)

	// Build tool specs from enabled tools (native + MCP)
	toolSpecs := s.buildEnabledToolSpecs(ctx, cfg)

	// Load conversation history
	history, err := s.listMessages(ctx, userID, req.ConversationID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "conversation not found or inaccessible")
		return
	}

	// Save the user message
	if _, err := s.createMessage(ctx, userID, req.ConversationID, "user", message); err != nil {
		writeError(w, http.StatusInternalServerError, "unable to save user message")
		return
	}

	// Build chat messages from history + new user message
	chatMsgs := make([]orchestrator.ChatMessage, 0, len(history)+1)
	for _, msg := range history {
		chatMsgs = append(chatMsgs, orchestrator.ChatMessage{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	chatMsgs = append(chatMsgs, orchestrator.ChatMessage{Role: "user", Content: message})

	// Build providers from user config (sorted by Priority)
	providers := buildProvidersFromConfig(cfg)
	svc := s.orchestrator
	if len(providers) > 0 {
		svc = orchestrator.NewService(providers, orchestrator.ToolLoopGuard{})
	}

	// Tool executor: runs built-in tools server-side
	toolExecutor := s.buildToolExecutor(ctx, userID)

	result, err := svc.Complete(ctx, orchestrator.CompletionRequest{
		System:        systemPrompt,
		Messages:      chatMsgs,
		Tools:         toolSpecs,
		ToolExecutor:  toolExecutor,
		ProviderOrder: req.ProviderOrder,
		MaxRetries:    req.MaxRetries,
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, "orchestrator failed: "+err.Error())
		return
	}

	// Save the assistant response
	if _, err := s.createMessage(ctx, userID, req.ConversationID, "assistant", result.Output); err != nil {
		log.Printf("warn: unable to save assistant message: %v", err)
	}

	// Persist tool calls
	for _, call := range result.Trace.ToolCalls {
		var errStr string
		if call.Status == "error" {
			errStr = call.Output
		}
		var outStr string
		if call.Status != "error" {
			outStr = call.Output
		}
		if err := s.saveToolCall(ctx, userID, req.ConversationID, call.Name, call.Arguments, outStr, errStr, 0); err != nil {
			log.Printf("warn: unable to save tool call %s: %v", call.Name, err)
		}
	}

	s.realtimeHub.Publish(realtime.Event{
		UserID: userID,
		Type:   "orchestrator.complete",
		Payload: map[string]any{
			"provider":       result.Provider,
			"attempts":       result.Attempts,
			"toolCalls":      len(result.Trace.ToolCalls),
			"runtimeActions": len(result.Trace.RuntimeActions),
		},
	})

	trace := CompletionTraceResponse{
		ProviderAttempts: make([]ProviderAttemptDTO, 0, len(result.Trace.ProviderAttempts)),
		ToolCalls:        make([]ToolCallDTO, 0, len(result.Trace.ToolCalls)),
		RuntimeActions:   make([]RuntimeActionDTO, 0, len(result.Trace.RuntimeActions)),
	}
	for _, attempt := range result.Trace.ProviderAttempts {
		trace.ProviderAttempts = append(trace.ProviderAttempts, ProviderAttemptDTO{
			Provider: attempt.Provider,
			Attempt:  attempt.Attempt,
			Success:  attempt.Success,
			Error:    attempt.Error,
		})
	}
	for _, call := range result.Trace.ToolCalls {
		trace.ToolCalls = append(trace.ToolCalls, ToolCallDTO{
			Name:      call.Name,
			Arguments: call.Arguments,
			Status:    call.Status,
			Output:    call.Output,
		})
	}
	for _, action := range result.Trace.RuntimeActions {
		trace.RuntimeActions = append(trace.RuntimeActions, RuntimeActionDTO{
			Kind:      action.Kind,
			Command:   action.Command,
			Status:    action.Status,
			Output:    action.Output,
			StartedAt: action.StartedAt.UTC().Format(time.RFC3339),
		})
	}

	resp := CompleteResponse{
		Provider: result.Provider,
		Output:   result.Output,
		Attempts: result.Attempts,
		Trace:    trace,
	}
	if !result.Usage.IsZero() {
		resp.Usage = &TokenUsageDTO{
			PromptTokens:     result.Usage.PromptTokens,
			CompletionTokens: result.Usage.CompletionTokens,
			TotalTokens:      result.Usage.TotalTokens,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// buildToolExecutor is defined in tools.go

// @Summary Stream an orchestrator completion via SSE
// @Tags    orchestrator
// @Security BearerAuth
// @Accept  json
// @Produce text/event-stream
// @Param   body body CompleteRequest true "Message and conversation ID"
// @Success 200 {string} string "SSE stream of delta/tool_call/tool_result/done/error events"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/orchestrator/stream [post]
func (s *Server) handleOrchestratorStream(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var req CompleteRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	message := strings.TrimSpace(req.Message)
	if message == "" || !isUUID(req.ConversationID) {
		writeError(w, http.StatusBadRequest, "message and conversationId required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	var cfg *configstore.UserConfig
	if s.configStore != nil {
		if c, err := s.configStore.GetUserConfig(userID); err == nil {
			cfg = &c
		}
	}

	systemPrompt := s.buildSystemPrompt(ctx, userID, cfg)

	toolSpecs := s.buildEnabledToolSpecs(ctx, cfg)

	history, err := s.listMessages(ctx, userID, req.ConversationID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "conversation not found")
		return
	}

	chatMsgs := make([]orchestrator.ChatMessage, 0, len(history)+1)
	for _, msg := range history {
		chatMsgs = append(chatMsgs, orchestrator.ChatMessage{Role: msg.Role, Content: msg.Content})
	}
	// Append current user message (already saved by client via POST /messages)
	chatMsgs = append(chatMsgs, orchestrator.ChatMessage{Role: "user", Content: message})
	chatMsgs = orchestrator.TrimMessages(chatMsgs, 20)

	// Pick first enabled streaming provider.
	// When tools are configured, prefer the full tool-loop path (non-streaming)
	// so that tool execution round-trips work correctly.
	// Providers are sorted by Priority (ascending = highest priority first).
	sortedProviders := buildProvidersFromConfig(cfg)
	var streamProv orchestrator.StreamingProvider
	var fallbackProviders []orchestrator.Provider
	for _, prov := range sortedProviders {
		if sp, ok := prov.(orchestrator.StreamingProvider); ok && streamProv == nil {
			// Respect explicit provider order from request if set
			if len(req.ProviderOrder) == 0 || req.ProviderOrder[0] == prov.Name() {
				streamProv = sp
			}
		}
		fallbackProviders = append(fallbackProviders, prov)
	}

	// SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, canFlush := w.(http.Flusher)

	sendEvent := func(event string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		if canFlush {
			flusher.Flush()
		}
	}

	var fullOutput string

	// Collect tool calls for persistence
	type capturedCall struct {
		name string
		args map[string]any
		out  string
		err  string
	}
	var mu sync.Mutex
	var capturedCalls []capturedCall

	recordCall := func(name string, args map[string]any, out string, execErr error) {
		c := capturedCall{name: name, args: args, out: out}
		if execErr != nil {
			c.err = execErr.Error()
			c.out = ""
		}
		mu.Lock()
		capturedCalls = append(capturedCalls, c)
		mu.Unlock()
	}

	if streamProv != nil {
		// Streaming tool loop: stream each LLM turn, execute any tool calls, repeat.
		toolExecutor := s.buildToolExecutor(ctx, userID)
		loopMsgs := chatMsgs
		guard := orchestrator.ToolLoopGuard{MaxIterations: 10}
		guard = guard.Validate()

		for iteration := 0; iteration < guard.MaxIterations; iteration++ {
			text, toolCalls, streamErr := streamProv.ChatStream(ctx, systemPrompt, loopMsgs, toolSpecs, func(token string) {
				sendEvent("delta", map[string]string{"token": token})
			})
			if streamErr != nil {
				sendEvent("error", map[string]string{"error": streamErr.Error()})
				return
			}
			// No tool calls: final answer
			if len(toolCalls) == 0 {
				fullOutput = text
				break
			}
			// Tool calls: notify client, execute, and continue loop
			for _, tc := range toolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				kind := "MCP"
				if isBuiltinToolName(tc.Name) {
					kind = "TOOL"
				}
				sendEvent("tool_call", map[string]string{
					"name":      tc.Name,
					"arguments": string(argsJSON),
					"kind":      kind,
				})
			}
			assistantMsg := orchestrator.ChatMessage{Role: "assistant", ToolCalls: toolCalls}
			loopMsgs = append(loopMsgs, assistantMsg)
			for _, tc := range toolCalls {
				result, execErr := toolExecutor(ctx, tc.Name, tc.Arguments)
				if execErr != nil {
					result = execErr.Error()
				}
				recordCall(tc.Name, tc.Arguments, result, execErr)
				sendEvent("tool_result", map[string]string{
					"name":   tc.Name,
					"result": truncateOutput(result, 200),
				})
				loopMsgs = append(loopMsgs, orchestrator.ChatMessage{
					Role: "tool", Content: result, ToolCallID: tc.ID,
				})
			}
		}
		sendEvent("done", map[string]string{"output": fullOutput})
	} else if len(fallbackProviders) > 0 {
		// Non-streaming fallback -- wrap executor to emit tool_call SSE events
		baseExecutor := s.buildToolExecutor(ctx, userID)
		wrappedExecutor := func(ectx context.Context, name string, args map[string]any) (string, error) {
			argsJSON, _ := json.Marshal(args)
			kind := "MCP"
			if isBuiltinToolName(name) {
				kind = "TOOL"
			}
			sendEvent("tool_call", map[string]string{"name": name, "arguments": string(argsJSON), "kind": kind})
			res, err := baseExecutor(ectx, name, args)
			if err != nil {
				sendEvent("tool_result", map[string]string{"name": name, "result": err.Error()})
			} else {
				sendEvent("tool_result", map[string]string{"name": name, "result": truncateOutput(res, 200)})
			}
			recordCall(name, args, res, err)
			return res, err
		}
		svc := orchestrator.NewService(fallbackProviders, orchestrator.ToolLoopGuard{})
		result, svcErr := svc.Complete(ctx, orchestrator.CompletionRequest{
			System: systemPrompt, Messages: chatMsgs, Tools: toolSpecs,
			ToolExecutor:  wrappedExecutor,
			ProviderOrder: req.ProviderOrder,
		})
		if svcErr != nil {
			sendEvent("error", map[string]string{"error": svcErr.Error()})
			return
		}
		fullOutput = result.Output
		donePayload := map[string]any{"output": fullOutput}
		if !result.Usage.IsZero() {
			donePayload["usage"] = map[string]int{
				"promptTokens":     result.Usage.PromptTokens,
				"completionTokens": result.Usage.CompletionTokens,
				"totalTokens":      result.Usage.TotalTokens,
			}
		}
		sendEvent("done", donePayload)
	} else {
		sendEvent("error", map[string]string{"error": "no providers configured"})
		return
	}

	if _, err := s.createMessage(ctx, userID, req.ConversationID, "assistant", fullOutput); err != nil {
		log.Printf("warn: unable to save assistant message: %v", err)
	}

	// Persist captured tool calls (background, non-fatal)
	for _, c := range capturedCalls {
		if err := s.saveToolCall(ctx, userID, req.ConversationID, c.name, c.args, c.out, c.err, 0); err != nil {
			log.Printf("warn: unable to save tool call %s: %v", c.name, err)
		}
	}
}

// @Summary Get the last realtime event for the current user
// @Tags    orchestrator
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]interface{}
// @Failure 401 {object} ErrorResponse
// @Router  /v1/realtime/last [get]
func (s *Server) handleRealtimeLastEvent(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	evt, ok := s.realtimeHub.LastEvent(userID)
	if !ok {
		writeJSON(w, http.StatusOK, map[string]any{"event": nil})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"event": evt})
}

// @Summary Regenerate an assistant message via SSE stream
// @Tags    conversations
// @Security BearerAuth
// @Produce text/event-stream
// @Param   id    path string true "Conversation ID"
// @Param   msgId path string true "Message ID"
// @Success 200 {string} string "SSE stream of delta/tool_call/tool_result/done/error events"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router  /v1/conversations/{id}/messages/{msgId}/regenerate [post]
// handleRegenerateMessage re-runs the LLM for a given assistant message in a conversation.
// It replaces the message content in-place and streams back via SSE.
func (s *Server) handleRegenerateMessage(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	convID := r.PathValue("id")
	msgID := r.PathValue("msgId")
	if !isUUID(convID) || !isUUID(msgID) {
		writeError(w, http.StatusBadRequest, "invalid ids")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 120*time.Second)
	defer cancel()

	// Load history up to but not including the message being regenerated
	history, err := s.listMessages(ctx, userID, convID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "conversation not found")
		return
	}

	// Build chatMsgs up to (but not including) msgId; find last user message before it.
	var chatMsgs []orchestrator.ChatMessage
	found := false
	var regenerateCreatedAt time.Time
	for _, msg := range history {
		if msg.ID == msgID {
			found = true
			if parsed, parseErr := time.Parse(time.RFC3339, msg.CreatedAt); parseErr == nil {
				regenerateCreatedAt = parsed
			}
			break
		}
		chatMsgs = append(chatMsgs, orchestrator.ChatMessage{Role: msg.Role, Content: msg.Content})
	}
	if !found {
		writeError(w, http.StatusNotFound, "message not found")
		return
	}
	chatMsgs = orchestrator.TrimMessages(chatMsgs, 20)
	if regenerateCreatedAt.IsZero() {
		writeError(w, http.StatusInternalServerError, "unable to determine regenerate point")
		return
	}

	var cfg *configstore.UserConfig
	if s.configStore != nil {
		if c, err2 := s.configStore.GetUserConfig(userID); err2 == nil {
			cfg = &c
		}
	}

	systemPrompt := s.buildSystemPrompt(ctx, userID, cfg)

	toolSpecs := s.buildEnabledToolSpecs(ctx, cfg)

	// Pick streaming provider (sorted by Priority, ascending = highest priority first)
	sortedProviders2 := buildProvidersFromConfig(cfg)
	var streamProv orchestrator.StreamingProvider
	var fallbackProviders []orchestrator.Provider
	for _, prov := range sortedProviders2 {
		if sp, ok := prov.(orchestrator.StreamingProvider); ok && streamProv == nil {
			streamProv = sp
		}
		fallbackProviders = append(fallbackProviders, prov)
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, canFlush := w.(http.Flusher)

	sendEvent := func(event string, data any) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, b)
		if canFlush {
			flusher.Flush()
		}
	}

	var fullOutput string

	type capturedCall struct {
		name string
		args map[string]any
		out  string
		err  string
	}
	var mu sync.Mutex
	var capturedCalls []capturedCall

	recordCall := func(name string, args map[string]any, out string, execErr error) {
		c := capturedCall{name: name, args: args, out: out}
		if execErr != nil {
			c.err = execErr.Error()
			c.out = ""
		}
		mu.Lock()
		capturedCalls = append(capturedCalls, c)
		mu.Unlock()
	}

	if streamProv != nil {
		toolExecutor := s.buildToolExecutor(ctx, userID)
		loopMsgs := chatMsgs
		guard := orchestrator.ToolLoopGuard{MaxIterations: 10}
		guard = guard.Validate()

		for iteration := 0; iteration < guard.MaxIterations; iteration++ {
			text, toolCalls, streamErr := streamProv.ChatStream(ctx, systemPrompt, loopMsgs, toolSpecs, func(token string) {
				sendEvent("delta", map[string]string{"token": token})
			})
			if streamErr != nil {
				sendEvent("error", map[string]string{"error": streamErr.Error()})
				return
			}
			if len(toolCalls) == 0 {
				fullOutput = text
				break
			}
			for _, tc := range toolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				kind := "MCP"
				if isBuiltinToolName(tc.Name) {
					kind = "TOOL"
				}
				sendEvent("tool_call", map[string]string{"name": tc.Name, "arguments": string(argsJSON), "kind": kind})
			}
			loopMsgs = append(loopMsgs, orchestrator.ChatMessage{Role: "assistant", ToolCalls: toolCalls})
			for _, tc := range toolCalls {
				result, execErr := toolExecutor(ctx, tc.Name, tc.Arguments)
				if execErr != nil {
					result = execErr.Error()
				}
				recordCall(tc.Name, tc.Arguments, result, execErr)
				sendEvent("tool_result", map[string]string{"name": tc.Name, "result": truncateOutput(result, 200)})
				loopMsgs = append(loopMsgs, orchestrator.ChatMessage{Role: "tool", Content: result, ToolCallID: tc.ID})
			}
		}
	} else if len(fallbackProviders) > 0 {
		svc := orchestrator.NewService(fallbackProviders, orchestrator.ToolLoopGuard{})
		result, svcErr := svc.Complete(ctx, orchestrator.CompletionRequest{
			System: systemPrompt, Messages: chatMsgs, Tools: toolSpecs,
			ToolExecutor: func(ectx context.Context, name string, args map[string]any) (string, error) {
				argsJSON, _ := json.Marshal(args)
				kind := "MCP"
				if isBuiltinToolName(name) {
					kind = "TOOL"
				}
				sendEvent("tool_call", map[string]string{"name": name, "arguments": string(argsJSON), "kind": kind})
				res, err := s.buildToolExecutor(ctx, userID)(ectx, name, args)
				if err != nil {
					sendEvent("tool_result", map[string]string{"name": name, "result": err.Error()})
				} else {
					sendEvent("tool_result", map[string]string{"name": name, "result": truncateOutput(res, 200)})
				}
				recordCall(name, args, res, err)
				return res, err
			},
		})
		if svcErr != nil {
			sendEvent("error", map[string]string{"error": svcErr.Error()})
			return
		}
		fullOutput = result.Output
	} else {
		sendEvent("error", map[string]string{"error": "no providers configured"})
		return
	}

	sendEvent("done", map[string]string{"output": fullOutput})

	if err := s.updateMessageContent(ctx, userID, convID, msgID, fullOutput); err != nil {
		log.Printf("warn: unable to update regenerated message %s: %v", msgID, err)
	}
	if err := s.deleteToolCallsSince(ctx, userID, convID, regenerateCreatedAt); err != nil {
		log.Printf("warn: unable to delete stale tool calls for conversation %s: %v", convID, err)
	}
	for _, c := range capturedCalls {
		if err := s.saveToolCall(ctx, userID, convID, c.name, c.args, c.out, c.err, 0); err != nil {
			log.Printf("warn: unable to save regenerated tool call %s: %v", c.name, err)
		}
	}
}

func (s *Server) buildEnabledToolSpecs(ctx context.Context, cfg *configstore.UserConfig) []orchestrator.ToolSpec {
	if cfg == nil {
		return nil
	}

	toolSpecs := make([]orchestrator.ToolSpec, 0)
	seen := map[string]struct{}{}

	for _, def := range cfg.Tools.Definitions {
		enabled := cfg.Tools.Enabled[def.ID]
		if !enabled {
			continue
		}
		name := sanitizeToolName(def.Name)
		if name == "" {
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		props := map[string]any{}
		var required []string
		for _, p := range def.Parameters {
			prop := map[string]any{"type": p.Type, "description": p.Description}
			props[p.Name] = prop
			if p.Required {
				required = append(required, p.Name)
			}
		}
		toolSpecs = append(toolSpecs, orchestrator.ToolSpec{
			Name:        name,
			Description: def.Description,
			Parameters: map[string]any{
				"properties": props,
				"required":   required,
			},
		})
		seen[name] = struct{}{}
	}

	for _, srv := range cfg.MCP.Servers {
		if !srv.Enabled || strings.TrimSpace(srv.URL) == "" {
			continue
		}
		mcpCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		discovered, derr := fetchMCPTools(mcpCtx, strings.TrimSpace(srv.URL), srv.Headers)
		cancel()
		if derr != nil {
			log.Printf("warn: unable to discover mcp tools for %s: %v", strings.TrimSpace(srv.URL), derr)
			continue
		}
		for _, mt := range discovered {
			name := sanitizeToolName(mt.Name)
			if name == "" {
				continue
			}
			if _, exists := seen[name]; exists {
				continue
			}
			desc := strings.TrimSpace(mt.Description)
			if desc == "" {
				desc = "MCP tool"
			}
			params := map[string]any{}
			for k, v := range mt.InputSchema {
				params[k] = v
			}
			if _, ok := params["type"]; !ok {
				params["type"] = "object"
			}
			if _, ok := params["properties"]; !ok {
				params["properties"] = map[string]any{}
			}
			toolSpecs = append(toolSpecs, orchestrator.ToolSpec{
				Name:        name,
				Description: fmt.Sprintf("[MCP:%s] %s", strings.TrimSpace(srv.Name), desc),
				Parameters:  params,
			})
			seen[name] = struct{}{}
		}
	}

	return toolSpecs
}
