package orchestrator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

var ErrNoProviderAvailable = errors.New("no provider available")

type ProviderFailureError struct {
	Attempts []ProviderAttempt
	LastErr  error
}

func (e *ProviderFailureError) Error() string {
	if len(e.Attempts) == 0 {
		if e.LastErr != nil {
			return fmt.Sprintf("all providers failed: %v", e.LastErr)
		}
		return "all providers failed"
	}
	parts := make([]string, 0, len(e.Attempts))
	for _, attempt := range e.Attempts {
		if attempt.Success {
			continue
		}
		part := fmt.Sprintf("%s attempt %d", attempt.Provider, attempt.Attempt)
		if attempt.Error != "" {
			part += ": " + attempt.Error
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 {
		if e.LastErr != nil {
			return fmt.Sprintf("all providers failed: %v", e.LastErr)
		}
		return "all providers failed"
	}
	return "all providers failed: " + strings.Join(parts, " | ")
}

func (e *ProviderFailureError) Unwrap() error {
	return e.LastErr
}

// ToolCall represents a function call by the model, with result filled in after execution.
type ToolCall struct {
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments"`
	Status    string         `json:"status"`
	Output    string         `json:"output,omitempty"`
}

type RuntimeAction struct {
	Kind      string
	Command   string
	Status    string
	Output    string
	StartedAt time.Time
}

type ProviderAttempt struct {
	Provider string
	Attempt  int
	Success  bool
	Error    string
}

type CompletionTrace struct {
	ProviderAttempts []ProviderAttempt
	ToolCalls        []ToolCall
	RuntimeActions   []RuntimeAction
}

type CompletionRequest struct {
	System        string
	Messages      []ChatMessage
	Tools         []ToolSpec
	ToolExecutor  func(ctx context.Context, name string, args map[string]any) (string, error)
	ProviderOrder []string
	MaxRetries    int
}

type CompletionResult struct {
	Provider string
	Output   string
	Attempts int
	Trace    CompletionTrace
}

type ToolLoopGuard struct {
	MaxIterations int
	MaxRepeated   int
}

func (g ToolLoopGuard) Validate() ToolLoopGuard {
	if g.MaxIterations <= 0 {
		g.MaxIterations = 10
	}
	if g.MaxRepeated <= 0 {
		g.MaxRepeated = 3
	}
	return g
}

type Service struct {
	providers      map[string]Provider
	providerOrder  []string // insertion order, used as default fallback order
	guard          ToolLoopGuard
}

func NewService(providerList []Provider, guard ToolLoopGuard) *Service {
	providers := make(map[string]Provider, len(providerList))
	order := make([]string, 0, len(providerList))
	for _, p := range providerList {
		key := strings.ToLower(p.Name())
		providers[key] = p
		order = append(order, key)
	}
	return &Service{
		providers:     providers,
		providerOrder: order,
		guard:         guard.Validate(),
	}
}

func (s *Service) Complete(ctx context.Context, req CompletionRequest) (CompletionResult, error) {
	if len(req.Messages) == 0 {
		return CompletionResult{}, fmt.Errorf("messages are required")
	}

	// Trim context: keep system message + last 20 conversation messages
	trimmedMessages := trimMessages(req.Messages, 20)

	providerNames := req.ProviderOrder
	if len(providerNames) == 0 {
		providerNames = s.defaultProviderOrder()
	}

	maxRetries := req.MaxRetries
	if maxRetries <= 0 {
		maxRetries = 2
	}

	trace := CompletionTrace{
		ProviderAttempts: []ProviderAttempt{},
		ToolCalls:        []ToolCall{},
		RuntimeActions:   []RuntimeAction{},
	}

	var lastErr error
	var attempts int

	for _, rawName := range providerNames {
		name := strings.ToLower(strings.TrimSpace(rawName))
		provider, ok := s.providers[name]
		if !ok {
			continue
		}

		for i := 0; i < maxRetries; i++ {
			attempts++
			attemptNo := i + 1

			// Tool loop with this provider
			output, toolCalls, err := s.runWithTools(ctx, provider, req.System, trimmedMessages, req.Tools, req.ToolExecutor, &trace)
			if err == nil {
				trace.ProviderAttempts = append(trace.ProviderAttempts, ProviderAttempt{
					Provider: provider.Name(),
					Attempt:  attemptNo,
					Success:  true,
				})
				_ = toolCalls
				return CompletionResult{Provider: provider.Name(), Output: output, Attempts: attempts, Trace: trace}, nil
			}
			trace.ProviderAttempts = append(trace.ProviderAttempts, ProviderAttempt{
				Provider: provider.Name(),
				Attempt:  attemptNo,
				Success:  false,
				Error:    err.Error(),
			})
			lastErr = err
			if ctx.Err() != nil {
				return CompletionResult{}, ctx.Err()
			}
		}
	}

	if lastErr != nil {
		return CompletionResult{Attempts: attempts, Trace: trace}, &ProviderFailureError{
			Attempts: trace.ProviderAttempts,
			LastErr:  lastErr,
		}
	}
	return CompletionResult{Attempts: attempts, Trace: trace}, ErrNoProviderAvailable
}

// runWithTools runs the chat loop with tool execution.
func (s *Service) runWithTools(
	ctx context.Context,
	provider Provider,
	system string,
	baseMessages []ChatMessage,
	tools []ToolSpec,
	executor func(ctx context.Context, name string, args map[string]any) (string, error),
	trace *CompletionTrace,
) (string, []ToolCall, error) {
	guard := s.guard
	msgs := append([]ChatMessage{}, baseMessages...)
	repeatedToolCallCount := map[string]int{}
	lastToolResultsBySignature := map[string][]ToolCall{}

	for iteration := 0; iteration < guard.MaxIterations; iteration++ {
		text, toolCalls, err := provider.Chat(ctx, system, msgs, tools)
		if err != nil {
			return "", nil, err
		}

		// No tool calls: we have a final answer
		if len(toolCalls) == 0 {
			return text, nil, nil
		}

		if err := validateToolCalls(toolCalls, tools, trace); err != nil {
			return "", nil, err
		}

		signature := toolCallSignature(toolCalls)
		repeatedToolCallCount[signature]++
		if repeatedToolCallCount[signature] > guard.MaxRepeated {
			if synthesized, ok := synthesizeRepeatedToolResult(lastToolResultsBySignature[signature]); ok {
				return synthesized, lastToolResultsBySignature[signature], nil
			}
			return "", nil, fmt.Errorf("tool loop repeated same tool call pattern more than %d times", guard.MaxRepeated)
		}

		// No executor: return the text as-is (or empty)
		if executor == nil {
			return text, toolCalls, nil
		}

		// Build the assistant message with tool_calls
		assistantMsg := ChatMessage{
			Role:      "assistant",
			ToolCalls: toolCalls,
		}
		msgs = append(msgs, assistantMsg)

		// Execute each tool call
		executedCalls := make([]ToolCall, 0, len(toolCalls))
		for _, tc := range toolCalls {
			tc.Status = "running"
			result, execErr := executor(ctx, tc.Name, tc.Arguments)
			if execErr != nil {
				tc.Status = "error"
				tc.Output = execErr.Error()
			} else {
				tc.Status = "ok"
				tc.Output = result
			}
			trace.ToolCalls = append(trace.ToolCalls, tc)
			executedCalls = append(executedCalls, tc)

			// Add tool result message
			msgs = append(msgs, ChatMessage{
				Role:       "tool",
				Content:    tc.Output,
				ToolCallID: tc.ID,
			})
		}
		lastToolResultsBySignature[signature] = executedCalls
	}

	return "", nil, fmt.Errorf("tool loop exceeded max iterations (%d)", guard.MaxIterations)
}

func validateToolCalls(toolCalls []ToolCall, tools []ToolSpec, trace *CompletionTrace) error {
	requiredByTool := make(map[string][]string, len(tools))
	for _, tool := range tools {
		requiredByTool[tool.Name] = requiredToolParams(tool)
	}

	for _, tc := range toolCalls {
		required, ok := requiredByTool[tc.Name]
		if !ok {
			invalid := tc
			invalid.Status = "error"
			invalid.Output = fmt.Sprintf("model requested unknown tool %s", tc.Name)
			trace.ToolCalls = append(trace.ToolCalls, invalid)
			return fmt.Errorf(invalid.Output)
		}
		missing := missingRequiredArgs(tc.Arguments, required)
		if len(missing) == 0 {
			continue
		}
		argsJSON, _ := json.Marshal(tc.Arguments)
		if len(argsJSON) == 0 || string(argsJSON) == "null" {
			argsJSON = []byte("{}")
		}
		invalid := tc
		invalid.Status = "error"
		invalid.Output = fmt.Sprintf("model requested tool %s without required args: %s (args=%s)", tc.Name, strings.Join(missing, ", "), string(argsJSON))
		trace.ToolCalls = append(trace.ToolCalls, invalid)
		return fmt.Errorf(invalid.Output)
	}
	return nil
}

func requiredToolParams(tool ToolSpec) []string {
	if req, ok := tool.Parameters["required"].([]string); ok {
		return req
	}
	if reqAny, ok := tool.Parameters["required"].([]any); ok {
		out := make([]string, 0, len(reqAny))
		for _, item := range reqAny {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, s)
			}
		}
		return out
	}
	return nil
}

func missingRequiredArgs(args map[string]any, required []string) []string {
	missing := make([]string, 0)
	for _, key := range required {
		val, ok := args[key]
		if !ok || val == nil {
			missing = append(missing, key)
			continue
		}
		if s, ok := val.(string); ok && strings.TrimSpace(s) == "" {
			missing = append(missing, key)
		}
	}
	return missing
}

func toolCallSignature(toolCalls []ToolCall) string {
	parts := make([]string, 0, len(toolCalls))
	for _, tc := range toolCalls {
		argsJSON, _ := json.Marshal(tc.Arguments)
		if len(argsJSON) == 0 || string(argsJSON) == "null" {
			argsJSON = []byte("{}")
		}
		parts = append(parts, tc.Name+":"+string(argsJSON))
	}
	return strings.Join(parts, "|")
}

func synthesizeRepeatedToolResult(toolCalls []ToolCall) (string, bool) {
	if len(toolCalls) == 0 {
		return "", false
	}
	outputs := make([]string, 0, len(toolCalls))
	for _, tc := range toolCalls {
		if tc.Status == "error" || strings.TrimSpace(tc.Output) == "" {
			return "", false
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(tc.Output), &payload); err == nil {
			if success, ok := payload["success"].(bool); ok && !success {
				return "", false
			}
		}
		outputs = append(outputs, fmt.Sprintf("%s => %s", tc.Name, tc.Output))
	}
	return "Completed using tool results: " + strings.Join(outputs, " | "), true
}

// TrimMessages returns at most maxMessages from the tail of messages.
func TrimMessages(msgs []ChatMessage, maxMessages int) []ChatMessage {
	return trimMessages(msgs, maxMessages)
}

// trimMessages returns at most maxMessages from the tail of messages.
func trimMessages(msgs []ChatMessage, maxMessages int) []ChatMessage {
	if len(msgs) <= maxMessages {
		return msgs
	}
	return msgs[len(msgs)-maxMessages:]
}

func (s *Service) defaultProviderOrder() []string {
	return s.providerOrder
}

type StubProvider struct {
	ProviderName string
}

func (p StubProvider) Name() string { return p.ProviderName }

func (p StubProvider) Chat(ctx context.Context, system string, messages []ChatMessage, tools []ToolSpec) (string, []ToolCall, error) {
	select {
	case <-ctx.Done():
		return "", nil, ctx.Err()
	case <-time.After(10 * time.Millisecond):
	}
	last := ""
	if len(messages) > 0 {
		last = messages[len(messages)-1].Content
	}
	return "stub: " + truncate(last, 80), nil, nil
}

func truncate(input string, max int) string {
	if len(input) <= max {
		return input
	}
	if max <= 3 {
		return input[:max]
	}
	return input[:max-3] + "..."
}
