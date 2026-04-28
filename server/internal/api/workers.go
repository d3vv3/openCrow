package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/opencrow/opencrow/server/internal/configstore"
	"github.com/opencrow/opencrow/server/internal/orchestrator"
)

// wlog writes to both the global logger and the worker's ring buffer.
func (s *Server) wlog(worker, format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	log.Printf("%s", line)
	s.workerLogs.Append(worker, line)
}

// StartWorkers launches background goroutines for scheduled tasks, heartbeat, and email polling.
// It returns immediately; workers run until ctx is cancelled.
func (s *Server) StartWorkers(ctx context.Context) {
	// Reset any tasks stuck in RUNNING from a previous server run
	if err := s.recoverStuckTasks(ctx); err != nil {
		s.wlog("task-worker", "[task-worker] failed to recover stuck tasks: %v", err)
	}
	go s.runTaskWorker(ctx)
	go s.runHeartbeatWorker(ctx)
	go s.runEmailWorker(ctx)
	go s.runTelegramWorker(ctx)
	go func() {
		if err := s.whisper.EnsureReady(ctx); err != nil {
			s.wlog("whisper", "[whisper] setup failed: %v", err)
		}
	}()
}

// runOrchestratorForUser runs a prompt through the orchestrator using the user's configured providers.
func (s *Server) runOrchestratorForUser(ctx context.Context, workerName, userID, prompt string) (orchestrator.CompletionResult, error) {
	var cfg *configstore.UserConfig
	if s.configStore != nil {
		if c, err := s.configStore.GetUserConfig(userID); err == nil {
			cfg = &c
		}
	}

	systemPrompt := s.buildSystemPrompt(ctx, userID, cfg)

	// Build tool specs from enabled tools (same logic as chat handlers)
	var toolSpecs []orchestrator.ToolSpec
	if cfg != nil {
		for _, def := range cfg.Tools.Definitions {
			enabled := cfg.Tools.Enabled[def.ID]
			if !enabled {
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
				Name:        sanitizeToolName(def.Name),
				Description: def.Description,
				Parameters: map[string]any{
					"properties": props,
					"required":   required,
				},
			})
		}
	}

	// Build providers sorted by Priority (ascending = highest priority first)
	providers := buildProvidersFromConfig(ctx, cfg)
	providerNames := make([]string, 0, len(providers))
	for _, provider := range providers {
		providerNames = append(providerNames, provider.Name())
	}
	if len(providerNames) > 0 {
		s.wlog(workerName, "[%s] provider order for user %s: %s", workerName, userID, strings.Join(providerNames, " -> "))
	}
	svc := s.orchestrator
	if len(providers) > 0 {
		svc = orchestrator.NewService(providers, orchestrator.ToolLoopGuard{})
	}

	result, err := svc.Complete(ctx, orchestrator.CompletionRequest{
		System:       systemPrompt,
		Messages:     []orchestrator.ChatMessage{{Role: "user", Content: prompt}},
		Tools:        toolSpecs,
		ToolExecutor: s.buildToolExecutor(ctx, userID),
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

// runOrchestratorForUserWithHistory is like runOrchestratorForUser but loads
// the full conversation history from the DB and passes it to the LLM, giving
// the model context of the ongoing conversation.
func (s *Server) runOrchestratorForUserWithHistory(ctx context.Context, workerName, userID, convID, prompt string) (orchestrator.CompletionResult, error) {
	if convID != "" {
		ctx = context.WithValue(ctx, conversationIDContextKey, convID)
	}
	var cfg *configstore.UserConfig
	if s.configStore != nil {
		if c, err := s.configStore.GetUserConfig(userID); err == nil {
			cfg = &c
		}
	}

	systemPrompt := s.buildSystemPrompt(ctx, userID, cfg)

	var toolSpecs []orchestrator.ToolSpec
	if cfg != nil {
		for _, def := range cfg.Tools.Definitions {
			if !cfg.Tools.Enabled[def.ID] {
				continue
			}
			props := map[string]any{}
			var required []string
			for _, p := range def.Parameters {
				props[p.Name] = map[string]any{"type": p.Type, "description": p.Description}
				if p.Required {
					required = append(required, p.Name)
				}
			}
			toolSpecs = append(toolSpecs, orchestrator.ToolSpec{
				Name:        sanitizeToolName(def.Name),
				Description: def.Description,
				Parameters:  map[string]any{"properties": props, "required": required},
			})
		}
	}

	providers := buildProvidersFromConfig(ctx, cfg)
	svc := s.orchestrator
	if len(providers) > 0 {
		svc = orchestrator.NewService(providers, orchestrator.ToolLoopGuard{})
	}

	// Load conversation history from DB
	var chatMsgs []orchestrator.ChatMessage
	if convID != "" {
		if history, err := s.listMessages(ctx, userID, convID); err == nil {
			for _, m := range history {
				chatMsgs = append(chatMsgs, orchestrator.ChatMessage{Role: m.Role, Content: m.Content})
			}
		} else {
			s.wlog(workerName, "[%s] failed to load history for conv %s: %v", workerName, convID, err)
		}
	}
	chatMsgs = append(chatMsgs, orchestrator.ChatMessage{Role: "user", Content: prompt})
	chatMsgs = orchestrator.TrimMessages(chatMsgs, 40)

	result, err := svc.Complete(ctx, orchestrator.CompletionRequest{
		System:       systemPrompt,
		Messages:     chatMsgs,
		Tools:        toolSpecs,
		ToolExecutor: s.buildToolExecutor(ctx, userID),
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

func (s *Server) logWorkerOrchestratorTrace(workerName, runLabel string, result orchestrator.CompletionResult) {
	if len(result.Trace.ProviderAttempts) > 0 {
		for _, attempt := range result.Trace.ProviderAttempts {
			if attempt.Success {
				s.wlog(workerName, "[%s] %s provider %s attempt %d succeeded", workerName, runLabel, attempt.Provider, attempt.Attempt)
			} else {
				s.wlog(workerName, "[%s] %s provider %s attempt %d failed: %s", workerName, runLabel, attempt.Provider, attempt.Attempt, attempt.Error)
			}
		}
	}

	if len(result.Trace.ToolCalls) == 0 {
		s.wlog(workerName, "[%s] %s executed no tools", workerName, runLabel)
		return
	}

	for i, tc := range result.Trace.ToolCalls {
		argsJSON, _ := json.Marshal(tc.Arguments)
		if len(argsJSON) == 0 || string(argsJSON) == "null" {
			argsJSON = []byte("{}")
		}
		preview := workerLogPreview(tc.Output, 220)
		if tc.Status == "error" {
			s.wlog(workerName, "[%s] %s tool %d/%d %s %s -> error: %s", workerName, runLabel, i+1, len(result.Trace.ToolCalls), tc.Name, string(argsJSON), preview)
			continue
		}
		s.wlog(workerName, "[%s] %s tool %d/%d %s %s -> %s", workerName, runLabel, i+1, len(result.Trace.ToolCalls), tc.Name, string(argsJSON), preview)
	}
}

func workerLogPreview(text string, max int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "(no output)"
	}
	text = strings.ReplaceAll(text, "\n", " ↩ ")
	text = strings.ReplaceAll(text, "\r", "")
	if len(text) <= max {
		return text
	}
	if max <= 3 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func summarizeWorkerExecution(result orchestrator.CompletionResult, execErr error, resolvedTZ string) string {
	var parts []string
	if strings.TrimSpace(resolvedTZ) != "" {
		parts = append(parts, fmt.Sprintf("timezone: %s", strings.TrimSpace(resolvedTZ)))
	}
	if execErr != nil {
		parts = append(parts, fmt.Sprintf("status: error\nerror: %s", execErr.Error()))
	} else {
		parts = append(parts, "status: ok")
	}
	if len(result.Trace.ProviderAttempts) > 0 {
		var attempts []string
		for _, attempt := range result.Trace.ProviderAttempts {
			if attempt.Success {
				attempts = append(attempts, fmt.Sprintf("%s attempt %d ok", attempt.Provider, attempt.Attempt))
			} else {
				attempts = append(attempts, fmt.Sprintf("%s attempt %d failed: %s", attempt.Provider, attempt.Attempt, attempt.Error))
			}
		}
		parts = append(parts, "provider attempts: "+strings.Join(attempts, " | "))
	}
	if len(result.Trace.ToolCalls) > 0 {
		var toolLines []string
		for _, tc := range result.Trace.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Arguments)
			if len(argsJSON) == 0 || string(argsJSON) == "null" {
				argsJSON = []byte("{}")
			}
			line := fmt.Sprintf("- %s %s", tc.Name, string(argsJSON))
			if strings.TrimSpace(tc.Output) != "" {
				line += " -> " + workerLogPreview(tc.Output, 240)
			}
			toolLines = append(toolLines, line)
		}
		parts = append(parts, "tools:\n"+strings.Join(toolLines, "\n"))
	}
	if strings.TrimSpace(result.Output) != "" {
		parts = append(parts, "assistant reply:\n"+result.Output)
	}
	return strings.Join(parts, "\n\n")
}
