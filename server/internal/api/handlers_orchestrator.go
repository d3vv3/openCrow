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
	ctx = context.WithValue(ctx, conversationIDContextKey, req.ConversationID)

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
	toolSpecs := s.buildEnabledToolSpecs(ctx, userID, cfg)

	// Load conversation history
	history, err := s.listMessages(ctx, userID, req.ConversationID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "conversation not found or inaccessible")
		return
	}

	// Save the user message
	if _, err := s.createMessage(ctx, userID, req.ConversationID, "user", message, req.Attachments); err != nil {
		writeError(w, http.StatusInternalServerError, "unable to save user message")
		return
	}

	// Build chat messages from history + new user message
	chatMsgs := make([]orchestrator.ChatMessage, 0, len(history)+1)
	for _, msg := range history {
		chatMsgs = append(chatMsgs, orchestrator.ChatMessage{
			Role:        msg.Role,
			Content:     msg.Content,
			Attachments: dtoAttachmentsToOrch(msg.Attachments),
		})
	}
	userMsg := orchestrator.ChatMessage{Role: "user", Content: message, Attachments: reqAttachmentsToOrch(req.Attachments)}
	chatMsgs = append(chatMsgs, userMsg)

	// Build providers from user config (sorted by Priority)
	providers := buildProvidersFromConfig(ctx, cfg)
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

	// Persist tool calls before the assistant message so their timestamps sort earlier
	for _, call := range result.Trace.ToolCalls {
		var errStr string
		if call.Status == "error" {
			errStr = call.Output
		}
		var outStr string
		if call.Status != "error" {
			outStr = call.Output
		}
		if err := s.saveToolCall(ctx, userID, req.ConversationID, call.Name, call.Arguments, outStr, errStr, 0, "builtin"); err != nil {
			log.Printf("warn: unable to save tool call %s: %v", call.Name, err)
		}
	}

	// Save the assistant response
	if _, err := s.createMessage(ctx, userID, req.ConversationID, "assistant", result.Output, nil); err != nil {
		log.Printf("warn: unable to save assistant message: %v", err)
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
	ctx = context.WithValue(ctx, conversationIDContextKey, req.ConversationID)

	var cfg *configstore.UserConfig
	if s.configStore != nil {
		if c, err := s.configStore.GetUserConfig(userID); err == nil {
			cfg = &c
		}
	}

	systemPrompt := s.buildSystemPrompt(ctx, userID, cfg)

	toolSpecs := s.buildEnabledToolSpecs(ctx, userID, cfg)

	// scheduleableDeviceTools is the set of device tool base-names that make sense
	// to queue as async device_tasks when no device is actively connected.
	scheduleableDeviceTools := map[string]bool{
		"set_alarm":              true,
		"create_calendar_event":  true,
		"delete_calendar_event":  true,
		"read_calendar":          true,
		"create_contact":         true,
		"send_sms":               true,
		"read_contacts":          true,
		"read_call_log":          true,
		"read_sms":               true,
		"get_battery":            true,
		"get_location":           true,
		"get_wifi_info":          true,
		"get_device_info":        true,
		"list_alarms":            true,
		"delete_alarm":           true,
		"list_apps":              true,
	}

	// liveOnlyDeviceTools are exposed to the LLM when no device is connected, but
	// they cannot be scheduled -- the LLM must tell the user to use the mobile app.
	liveOnlyDeviceTools := map[string]bool{
		"send_notification": true,
		"toggle_flashlight": true,
		"set_volume":        true,
		"set_brightness":    true,
		"set_ringer_mode":   true,
		"media_control":     true,
		"open_app":          true,
		"web_open":          true,
	}

	// Inject local tool specs registered by the requesting device
	localToolNames := map[string]bool{}        // bare name -> true (live execution via SSE)
	scheduleToolTargets := map[string]string{} // bare name -> target device ID (async scheduling)
	liveOnlyToolNames := map[string]bool{}     // bare name -> true (requires live device, no scheduling)

	if req.DeviceID != "" {
		// Device is actively connected - expose all its capabilities for live execution.
		if devCaps, err := s.getDeviceCapabilities(ctx, userID, req.DeviceID); err == nil {
			for _, cap := range devCaps {
				baseName := strings.TrimPrefix(cap.Name, "on_device_")
				toolSpecs = append(toolSpecs, orchestrator.ToolSpec{
					Name:        baseName,
					Description: cap.Description,
					Parameters:  cap.Parameters,
				})
				localToolNames[baseName] = true
			}
		}
	} else {
		// No active device - expose scheduleable tools (queued as device_tasks) and
		// live-only tools (shown to the LLM so it can explain they need an active connection).
		if devRegs, err := s.listDeviceRegistrations(ctx, userID); err == nil {
			for deviceID, reg := range devRegs {
				for _, cap := range reg.Capabilities {
					baseName := strings.TrimPrefix(cap.Name, "on_device_")
					if scheduleableDeviceTools[baseName] {
						if _, already := scheduleToolTargets[baseName]; already {
							continue // only register first device per tool
						}
						toolSpecs = append(toolSpecs, orchestrator.ToolSpec{
							Name:        baseName,
							Description: cap.Description,
							Parameters:  cap.Parameters,
						})
						scheduleToolTargets[baseName] = deviceID
					} else if liveOnlyDeviceTools[baseName] {
						if liveOnlyToolNames[baseName] {
							continue // only register once
						}
						desc := cap.Description
						if desc != "" {
							desc += " (only available when the companion app is open and actively connected)"
						} else {
							desc = "Only available when the companion app is open and actively connected"
						}
						toolSpecs = append(toolSpecs, orchestrator.ToolSpec{
							Name:        baseName,
							Description: desc,
							Parameters:  cap.Parameters,
						})
						liveOnlyToolNames[baseName] = true
					}
				}
			}
		}
	}

	history, err := s.listMessages(ctx, userID, req.ConversationID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "conversation not found")
		return
	}

	chatMsgs := make([]orchestrator.ChatMessage, 0, len(history)+1)
	for _, msg := range history {
		chatMsgs = append(chatMsgs, orchestrator.ChatMessage{Role: msg.Role, Content: msg.Content, Attachments: dtoAttachmentsToOrch(msg.Attachments)})
	}
	// Append current user message (already saved by client via POST /messages)
	chatMsgs = append(chatMsgs, orchestrator.ChatMessage{Role: "user", Content: message, Attachments: reqAttachmentsToOrch(req.Attachments)})
	chatMsgs = orchestrator.TrimMessages(chatMsgs, 20)

	// Pick first enabled streaming provider.
	// When tools are configured, prefer the full tool-loop path (non-streaming)
	// so that tool execution round-trips work correctly.
	// Providers are sorted by Priority (ascending = highest priority first).
	sortedProviders := buildProvidersFromConfig(ctx, cfg)
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
		name   string
		args   map[string]any
		out    string
		err    string
		source string // "builtin", "mcp", or "device"
	}
	var mu sync.Mutex
	var capturedCalls []capturedCall

	recordCall := func(name string, args map[string]any, out string, execErr error, source string) {
		c := capturedCall{name: name, args: args, out: out, source: source}
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
			// Accumulate text from every iteration (pre-tool text + final answer)
			fullOutput += text
			// No tool calls: final answer
			if len(toolCalls) == 0 {
				break
			}
			// Tool calls: notify client, execute, and continue loop
			for _, tc := range toolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				kind := "MCP"
				if isBuiltinToolName(tc.Name) {
					kind = "TOOL"
				} else if localToolNames[tc.Name] || scheduleToolTargets[tc.Name] != "" {
					kind = "DEVICE"
				} else if liveOnlyToolNames[tc.Name] {
					kind = "DEVICE"
				}
				sendEvent("tool_call", map[string]string{
					"name":      tc.Name,
					"arguments": string(argsJSON),
					"kind":      kind,
				})
			}
			// Include any pre-tool-call text in the assistant message so the LLM retains context
			assistantMsg := orchestrator.ChatMessage{Role: "assistant", Content: text, ToolCalls: toolCalls}
			loopMsgs = append(loopMsgs, assistantMsg)
			for _, tc := range toolCalls {
				var result string
				var execErr error
				callSource := "builtin"
				if isLocal := localToolNames[tc.Name]; isLocal && req.DeviceID != "" {
					callSource = "device"
					// Forward to device via SSE, block until result
					callId := fmt.Sprintf("lc-%s-%d", tc.Name, time.Now().UnixNano())
					resultCh := make(chan localCallResult, 1)
					s.pendingLocalCalls.Store(callId, resultCh)
					defer s.pendingLocalCalls.Delete(callId)
					argsJSON, _ := json.Marshal(tc.Arguments)
					sendEvent("tool_execute_local", map[string]string{
						"callId":    callId,
						"name":      tc.Name,
						"arguments": string(argsJSON),
					})
					select {
					case res := <-resultCh:
						result = res.Output
						if res.IsError {
							execErr = fmt.Errorf("%s", res.Output)
							result = ""
						}
					case <-time.After(30 * time.Second):
						execErr = fmt.Errorf("local tool %s timed out after 30s", tc.Name)
					}
				} else if targetDeviceID, isScheduled := scheduleToolTargets[tc.Name]; isScheduled {
					callSource = "device"
					// Queue as an async device_task, then try to wake the device via UP push.
					toolNameStr := tc.Name
					task, taskErr := s.createDeviceTask(ctx, userID, targetDeviceID, tc.Name, &toolNameStr, tc.Arguments)
					if taskErr != nil {
						execErr = fmt.Errorf("failed to schedule device task: %w", taskErr)
					} else {
						// Attempt to wake the device via UnifiedPush and wait up to 5s for the result.
						pushErr := s.sendDeviceTaskPush(ctx, userID, targetDeviceID, task.ID)
						if pushErr != nil {
							log.Printf("[orchestrator] UP push for task %s failed: %v (will rely on heartbeat)", task.ID, pushErr)
						}
						completed, waitErr := s.waitForDeviceTask(ctx, userID, task.ID, 5*time.Second)
						if waitErr == nil && completed.ResultOutput != nil {
							result = *completed.ResultOutput
							if completed.Status == "failed" {
								execErr = fmt.Errorf("%s", result)
								result = ""
							}
						} else {
							// Fallback: device will pick it up on next heartbeat poll.
							result = fmt.Sprintf("Scheduled on your companion device (task ID: %s). The action will execute the next time your device checks in.", task.ID)
						}
					}
				} else if liveOnlyToolNames[tc.Name] {
					callSource = "device"
					// This tool requires an active device connection -- tell the LLM so it can inform the user.
					result = fmt.Sprintf("Cannot execute '%s': this action requires your companion device to be actively connected. Please open the openCrow app on your phone and try again from there.", tc.Name)
				} else {
					if !isBuiltinToolName(tc.Name) {
						callSource = "mcp"
					}
					result, execErr = toolExecutor(ctx, tc.Name, tc.Arguments)
					if execErr != nil {
						result = execErr.Error()
					}
				}
				recordCall(tc.Name, tc.Arguments, result, execErr, callSource)
				toolResultPayload := map[string]string{
					"name":   tc.Name,
					"result": truncateOutput(result, 200),
				}
				if execErr != nil {
					toolResultPayload["isError"] = "true"
				}
				sendEvent("tool_result", toolResultPayload)
				loopResultContent := result
				if execErr != nil {
					loopResultContent = execErr.Error()
				}
				loopMsgs = append(loopMsgs, orchestrator.ChatMessage{
					Role: "tool", Content: loopResultContent, ToolCallID: tc.ID,
				})
			}
		}
		// Persist tool calls before the assistant message so their timestamps sort earlier.
		for _, c := range capturedCalls {
			if err := s.saveToolCall(ctx, userID, req.ConversationID, c.name, c.args, c.out, c.err, 0, c.source); err != nil {
				log.Printf("warn: unable to save tool call %s: %v", c.name, err)
			}
		}
		capturedCalls = nil // avoid double-write in deferred block below
		savedMsg, saveErr := s.createMessage(ctx, userID, req.ConversationID, "assistant", fullOutput, nil)
		if saveErr != nil {
			log.Printf("warn: unable to save assistant message: %v", saveErr)
		}
		donePayload := map[string]any{"output": fullOutput}
		if saveErr == nil {
			donePayload["messageId"] = savedMsg.ID
		}
		sendEvent("done", donePayload)
		return
	} else if len(fallbackProviders) > 0 {
		// Non-streaming fallback -- wrap executor to emit tool_call SSE events
		baseExecutor := s.buildToolExecutor(ctx, userID)
		wrappedExecutor := func(ectx context.Context, name string, args map[string]any) (string, error) {
			argsJSON, _ := json.Marshal(args)
			src := "builtin"
			kind := "TOOL"
			if !isBuiltinToolName(name) {
				src = "mcp"
				kind = "MCP"
			}
			sendEvent("tool_call", map[string]string{"name": name, "arguments": string(argsJSON), "kind": kind})
			res, err := baseExecutor(ectx, name, args)
			if err != nil {
				sendEvent("tool_result", map[string]string{"name": name, "result": err.Error(), "isError": "true"})
			} else {
				sendEvent("tool_result", map[string]string{"name": name, "result": truncateOutput(res, 200)})
			}
			recordCall(name, args, res, err, src)
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
		// Persist tool calls before the assistant message so their timestamps sort earlier.
		for _, c := range capturedCalls {
			if err := s.saveToolCall(ctx, userID, req.ConversationID, c.name, c.args, c.out, c.err, 0, c.source); err != nil {
				log.Printf("warn: unable to save tool call %s: %v", c.name, err)
			}
		}
		savedMsg, saveErr := s.createMessage(ctx, userID, req.ConversationID, "assistant", fullOutput, nil)
		if saveErr != nil {
			log.Printf("warn: unable to save assistant message: %v", saveErr)
		}
		donePayload := map[string]any{"output": fullOutput}
		if !result.Usage.IsZero() {
			donePayload["usage"] = map[string]int{
				"promptTokens":     result.Usage.PromptTokens,
				"completionTokens": result.Usage.CompletionTokens,
				"totalTokens":      result.Usage.TotalTokens,
			}
		}
		if saveErr == nil {
			donePayload["messageId"] = savedMsg.ID
		}
		sendEvent("done", donePayload)
	} else {
		sendEvent("error", map[string]string{"error": "no providers configured"})
	}
}

// dtoAttachmentsToOrch converts MessageAttachmentDTO slice to orchestrator.MessageAttachment slice.
func dtoAttachmentsToOrch(dtos []MessageAttachmentDTO) []orchestrator.MessageAttachment {
	if len(dtos) == 0 {
		return nil
	}
	out := make([]orchestrator.MessageAttachment, len(dtos))
	for i, a := range dtos {
		out[i] = orchestrator.MessageAttachment{FileName: a.FileName, MimeType: a.MimeType, DataURL: a.DataURL}
	}
	return out
}

// reqAttachmentsToOrch converts CreateMessageAttachmentRequest slice to orchestrator.MessageAttachment slice.
func reqAttachmentsToOrch(reqs []CreateMessageAttachmentRequest) []orchestrator.MessageAttachment {
	if len(reqs) == 0 {
		return nil
	}
	out := make([]orchestrator.MessageAttachment, len(reqs))
	for i, a := range reqs {
		out[i] = orchestrator.MessageAttachment{FileName: a.FileName, MimeType: a.MimeType, DataURL: a.DataURL}
	}
	return out
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
	var lastUserMsgCreatedAt time.Time
	for _, msg := range history {
		if msg.ID == msgID {
			found = true
			if parsed, parseErr := time.Parse(time.RFC3339, msg.CreatedAt); parseErr == nil {
				regenerateCreatedAt = parsed
			}
			break
		}
		if msg.Role == "user" {
			if parsed, parseErr := time.Parse(time.RFC3339, msg.CreatedAt); parseErr == nil {
				lastUserMsgCreatedAt = parsed
			}
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

	toolSpecs := s.buildEnabledToolSpecs(ctx, userID, cfg)

	// Pick streaming provider (sorted by Priority, ascending = highest priority first)
	sortedProviders2 := buildProvidersFromConfig(ctx, cfg)
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
		name   string
		args   map[string]any
		out    string
		err    string
		source string
	}
	var mu sync.Mutex
	var capturedCalls []capturedCall

	recordCall := func(name string, args map[string]any, out string, execErr error, source string) {
		c := capturedCall{name: name, args: args, out: out, source: source}
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
				src := "builtin"
				if !isBuiltinToolName(tc.Name) {
					src = "mcp"
				}
				recordCall(tc.Name, tc.Arguments, result, execErr, src)
				toolResultPayload := map[string]string{"name": tc.Name, "result": truncateOutput(result, 200)}
				if execErr != nil {
					toolResultPayload["isError"] = "true"
				}
				sendEvent("tool_result", toolResultPayload)
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
					sendEvent("tool_result", map[string]string{"name": name, "result": err.Error(), "isError": "true"})
				} else {
					sendEvent("tool_result", map[string]string{"name": name, "result": truncateOutput(res, 200)})
				}
				rSrc := "builtin"
				if !isBuiltinToolName(name) {
					rSrc = "mcp"
				}
				recordCall(name, args, res, err, rSrc)
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

	// Persist before sending done: once the client receives done it closes the SSE
	// connection which cancels r.Context(). All DB writes must complete first.
	deleteCutoff := lastUserMsgCreatedAt
	if deleteCutoff.IsZero() {
		deleteCutoff = regenerateCreatedAt
	}
	if err := s.deleteToolCallsSince(ctx, userID, convID, deleteCutoff); err != nil {
		log.Printf("warn: unable to delete stale tool calls for conversation %s: %v", convID, err)
	}
	for _, c := range capturedCalls {
		if err := s.saveToolCall(ctx, userID, convID, c.name, c.args, c.out, c.err, 0, c.source); err != nil {
			log.Printf("warn: unable to save regenerated tool call %s: %v", c.name, err)
		}
	}
	// Update content last so created_at (reset to NOW()) stays after the tool calls
	if err := s.updateMessageContent(ctx, userID, convID, msgID, fullOutput); err != nil {
		log.Printf("warn: unable to update regenerated message %s: %v", msgID, err)
	}

	sendEvent("done", map[string]string{"output": fullOutput, "messageId": msgID})
}

func (s *Server) buildEnabledToolSpecs(ctx context.Context, userID string, cfg *configstore.UserConfig) []orchestrator.ToolSpec {
	if cfg == nil {
		return nil
	}

	toolSpecs := make([]orchestrator.ToolSpec, 0)
	seen := map[string]struct{}{}

	// Always expose send_push_notification when at least one device has a UP endpoint,
	// regardless of whether the user has manually configured it in cfg.Tools.
	if eps, err := s.getDevicePushEndpoints(ctx, userID); err == nil && len(eps) > 0 {
		toolSpecs = append(toolSpecs, orchestrator.ToolSpec{
			Name:        "send_push_notification",
			Description: "Send a push notification directly to the user's companion device(s). Use this to proactively notify the user on their phone. Include conversation_id when the notification relates to a specific chat so tapping it opens that conversation in the app.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":           map[string]any{"type": "string", "description": "Notification title"},
					"body":            map[string]any{"type": "string", "description": "Notification body text"},
					"channel":         map[string]any{"type": "string", "description": "Optional notification channel, e.g. 'default' or 'alert'"},
					"device_id":       map[string]any{"type": "string", "description": "Optional companion device id to target a specific device"},
					"conversation_id": map[string]any{"type": "string", "description": "Optional conversation id to open when the user taps the notification"},
				},
				"required": []string{"title", "body"},
			},
		})
		seen["send_push_notification"] = struct{}{}
	}

	for _, def := range cfg.Tools.Definitions {
		enabled := cfg.Tools.Enabled[def.ID]
		if !enabled {
			continue
		}
		name := sanitizeToolName(def.Name)
		if name == "" {
			continue
		}
		// send_push_notification is now always auto-injected above; skip manual config.
		// send_notification is a legacy/deprecated name replaced by send_channel_notification; skip it.
		if name == "send_push_notification" || name == "send_notification" {
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

	return toolSpecs
}
