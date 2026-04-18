package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// httpClient is a shared client with reasonable timeouts.
var httpClient = &http.Client{Timeout: 90 * time.Second}

// ── Shared chat types ─────────────────────────────────────────────────────────

// ChatMessage is a single message in a conversation.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
}

// ToolSpec describes a callable tool for function-calling APIs.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]any // JSON Schema object
}

// ── Provider interface ────────────────────────────────────────────────────────

// Provider can complete a chat conversation, optionally using tools.
type Provider interface {
	Name() string
	// Chat sends messages and optional tools. Returns (text, toolCalls, error).
	// toolCalls is non-empty when the model wants to call a function.
	Chat(ctx context.Context, system string, messages []ChatMessage, tools []ToolSpec) (string, []ToolCall, error)
}

// ── OpenAI / OpenAI-compatible ────────────────────────────────────────────────

type OpenAIProvider struct {
	name    string
	baseURL string
	apiKey  string
	model   string
}

func parseToolCallArguments(raw []byte) (map[string]any, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty tool arguments")
	}

	var args map[string]any
	if err := json.Unmarshal(raw, &args); err == nil {
		if args == nil {
			args = map[string]any{}
		}
		return args, nil
	}

	var encoded string
	if err := json.Unmarshal(raw, &encoded); err == nil {
		encoded = strings.TrimSpace(encoded)
		if encoded == "" {
			return nil, fmt.Errorf("empty string tool arguments")
		}
		if err := json.Unmarshal([]byte(encoded), &args); err != nil {
			return nil, fmt.Errorf("invalid string-encoded tool arguments: %w", err)
		}
		if args == nil {
			args = map[string]any{}
		}
		return args, nil
	}

	return nil, fmt.Errorf("tool arguments were not a JSON object")
}

func NewOpenAIProvider(name, baseURL, apiKey, model string) *OpenAIProvider {
	if baseURL == "" {
		baseURL = "https://api.openai.com"
	}
	if model == "" {
		model = "gpt-4o-mini"
	}
	return &OpenAIProvider{name: name, baseURL: strings.TrimRight(baseURL, "/"), apiKey: apiKey, model: model}
}

func (p *OpenAIProvider) Name() string { return p.name }

var markdownImagePattern = regexp.MustCompile(`!\[(.*?)\]\(((?:data:image/[^)]+)|(?:https?://[^)]+))\)`)

func buildOpenAIContent(role, content string) any {
	if role != "user" {
		return content
	}
	matches := markdownImagePattern.FindAllStringSubmatchIndex(content, -1)
	if len(matches) == 0 {
		return content
	}
	parts := make([]map[string]any, 0, len(matches)*2+1)
	last := 0
	for _, m := range matches {
		if len(m) < 6 {
			continue
		}
		fullStart, fullEnd := m[0], m[1]
		altStart, altEnd := m[2], m[3]
		urlStart, urlEnd := m[4], m[5]
		if fullStart > last {
			text := content[last:fullStart]
			if text != "" {
				parts = append(parts, map[string]any{"type": "text", "text": text})
			}
		}
		url := content[urlStart:urlEnd]
		part := map[string]any{
			"type": "image_url",
			"image_url": map[string]any{
				"url": url,
			},
		}
		if altStart >= 0 && altEnd >= altStart {
			alt := content[altStart:altEnd]
			if alt != "" {
				part["image_url"].(map[string]any)["detail"] = "auto"
			}
		}
		parts = append(parts, part)
		last = fullEnd
	}
	if last < len(content) {
		text := content[last:]
		if text != "" {
			parts = append(parts, map[string]any{"type": "text", "text": text})
		}
	}
	if len(parts) == 0 {
		return content
	}
	return parts
}

func (p *OpenAIProvider) Chat(ctx context.Context, system string, messages []ChatMessage, tools []ToolSpec) (string, []ToolCall, error) {
	// Build messages array
	type oaiMsg struct {
		Role       string          `json:"role"`
		Content    any             `json:"content,omitempty"`
		ToolCallID string          `json:"tool_call_id,omitempty"`
		ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	}

	msgs := make([]oaiMsg, 0, len(messages)+1)
	if system != "" {
		msgs = append(msgs, oaiMsg{Role: "system", Content: system})
	}
	for _, m := range messages {
		msg := oaiMsg{Role: m.Role, Content: buildOpenAIContent(m.Role, m.Content), ToolCallID: m.ToolCallID}
		if len(m.ToolCalls) > 0 {
			// Serialize in OpenAI wire format: [{id, type:"function", function:{name, arguments}}]
			type oaiTC struct {
				ID       string `json:"id,omitempty"`
				Type     string `json:"type"`
				Function struct {
					Name      string `json:"name"`
					Arguments string `json:"arguments"`
				} `json:"function"`
			}
			oaiTCs := make([]oaiTC, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				if string(argsJSON) == "null" {
					argsJSON = []byte("{}")
				}
				oaiTCs = append(oaiTCs, oaiTC{
					ID:   tc.ID,
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: tc.Name, Arguments: string(argsJSON)},
				})
			}
			raw, _ := json.Marshal(oaiTCs)
			msg.ToolCalls = raw
		}
		msgs = append(msgs, msg)
	}

	// Build tools
	type oaiParam struct {
		Type       string         `json:"type"`
		Properties map[string]any `json:"properties"`
		Required   []string       `json:"required,omitempty"`
		AdditionalProperties bool `json:"additionalProperties"`
	}
	type oaiFn struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Parameters  oaiParam `json:"parameters"`
	}
	type oaiTool struct {
		Type     string `json:"type"`
		Function oaiFn  `json:"function"`
	}

	var oaiTools []oaiTool
	for _, t := range tools {
		props, _ := t.Parameters["properties"].(map[string]any)
		if props == nil {
			props = map[string]any{}
		}
		var required []string
		if req, ok := t.Parameters["required"].([]string); ok {
			required = req
		}
			oaiTools = append(oaiTools, oaiTool{
				Type: "function",
				Function: oaiFn{
					Name:        t.Name,
					Description: t.Description,
					Parameters: oaiParam{
						Type:       "object",
						Properties: props,
						Required:   required,
						AdditionalProperties: false,
					},
				},
			})
		}

	reqBody := map[string]any{
		"model":    p.model,
		"messages": msgs,
	}
	if len(oaiTools) > 0 {
		reqBody["tools"] = oaiTools
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, fmt.Errorf("read response: %w", err)
	}
	if resp.StatusCode >= 400 {
		return "", nil, fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(raw), 400))
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", nil, fmt.Errorf("openai: parse response: %w", err)
	}
	if result.Error != nil {
		return "", nil, fmt.Errorf("openai api error: %s", result.Error.Message)
	}
	if len(result.Choices) == 0 {
		return "", nil, fmt.Errorf("openai: empty response")
	}

	choice := result.Choices[0].Message
	if len(choice.ToolCalls) > 0 {
		var calls []ToolCall
		for _, tc := range choice.ToolCalls {
			args, err := parseToolCallArguments(tc.Function.Arguments)
			if err != nil {
				return "", nil, fmt.Errorf("openai: parse tool call %s arguments for %s: %v (raw: %s)", tc.ID, tc.Function.Name, err, truncate(string(tc.Function.Arguments), 200))
			}
			calls = append(calls, ToolCall{
				ID:        tc.ID,
				Name:      tc.Function.Name,
				Arguments: args,
				Status:    "pending",
			})
		}
		return "", calls, nil
	}

	if choice.Content == "" {
		return "", nil, fmt.Errorf("openai: empty content")
	}
	return choice.Content, nil, nil
}

// ── Anthropic ────────────────────────────────────────────────────────────────

type AnthropicProvider struct {
	name   string
	apiKey string
	model  string
}

func NewAnthropicProvider(name, apiKey, model string) *AnthropicProvider {
	if model == "" {
		model = "claude-3-5-haiku-20241022"
	}
	return &AnthropicProvider{name: name, apiKey: apiKey, model: model}
}

func (p *AnthropicProvider) Name() string { return p.name }

func (p *AnthropicProvider) Chat(ctx context.Context, system string, messages []ChatMessage, tools []ToolSpec) (string, []ToolCall, error) {
	type contentBlock struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	type message struct {
		Role    string         `json:"role"`
		Content []contentBlock `json:"content"`
	}

	var msgs []message
	for _, m := range messages {
		if m.Role == "system" {
			continue // handled separately
		}
		role := m.Role
		if role != "user" && role != "assistant" {
			role = "user"
		}
		msgs = append(msgs, message{Role: role, Content: []contentBlock{{Type: "text", Text: m.Content}}})
	}

	reqBody := map[string]any{
		"model":      p.model,
		"max_tokens": 4096,
		"messages":   msgs,
	}
	if system != "" {
		reqBody["system"] = system
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.anthropic.com/v1/messages", bytes.NewReader(b))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", p.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode >= 400 {
		return "", nil, fmt.Errorf("anthropic http %d: %s", resp.StatusCode, truncate(string(raw), 400))
	}

	var result struct {
		Content []contentBlock `json:"content"`
		Error   *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", nil, fmt.Errorf("anthropic: parse response: %w", err)
	}
	if result.Error != nil {
		return "", nil, fmt.Errorf("anthropic api error: %s", result.Error.Message)
	}
	for _, block := range result.Content {
		if block.Type == "text" && block.Text != "" {
			return block.Text, nil, nil
		}
	}
	return "", nil, fmt.Errorf("anthropic: empty response")
}

// ── Ollama ────────────────────────────────────────────────────────────────────

type OllamaProvider struct {
	name    string
	baseURL string
	model   string
}

func NewOllamaProvider(name, baseURL, model string) *OllamaProvider {
	if baseURL == "" {
		baseURL = "http://localhost:11434"
	}
	if model == "" {
		model = "llama3.2"
	}
	return &OllamaProvider{name: name, baseURL: strings.TrimRight(baseURL, "/"), model: model}
}

func (p *OllamaProvider) Name() string { return p.name }

func (p *OllamaProvider) Chat(ctx context.Context, system string, messages []ChatMessage, tools []ToolSpec) (string, []ToolCall, error) {
	type oMsg struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	var msgs []oMsg
	if system != "" {
		msgs = append(msgs, oMsg{Role: "system", Content: system})
	}
	for _, m := range messages {
		role := m.Role
		if role != "user" && role != "assistant" && role != "system" {
			role = "user"
		}
		msgs = append(msgs, oMsg{Role: role, Content: m.Content})
	}

	reqBody := map[string]any{
		"model":    p.model,
		"messages": msgs,
		"stream":   false,
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/api/chat", bytes.NewReader(b))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("ollama http request: %w", err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}
	if resp.StatusCode >= 400 {
		return "", nil, fmt.Errorf("ollama http %d: %s", resp.StatusCode, truncate(string(raw), 400))
	}

	var result struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
		Error string `json:"error"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", nil, fmt.Errorf("ollama: parse response: %w", err)
	}
	if result.Error != "" {
		return "", nil, fmt.Errorf("ollama error: %s", result.Error)
	}
	if result.Message.Content == "" {
		return "", nil, fmt.Errorf("ollama: empty response")
	}
	return result.Message.Content, nil, nil
}

// ── Factory ───────────────────────────────────────────────────────────────────

// BuildProvider creates a Provider from configstore fields.
func BuildProvider(name, kind, baseURL, apiKey, model string) Provider {
	switch strings.ToLower(kind) {
	case "openai", "custom":
		return NewOpenAIProvider(name, baseURL, apiKey, model)
	case "anthropic":
		return NewAnthropicProvider(name, apiKey, model)
	case "ollama":
		return NewOllamaProvider(name, baseURL, model)
	case "openrouter":
		if baseURL == "" {
			baseURL = "https://openrouter.ai/api"
		}
		return NewOpenAIProvider(name, baseURL, apiKey, model)
	case "litellm":
		// LiteLLM exposes an OpenAI-compatible API; baseURL points to the proxy
		if baseURL == "" {
			baseURL = "http://localhost:4000"
		}
		return NewOpenAIProvider(name, baseURL, apiKey, model)
	}
	return nil
}

// ── Streaming ─────────────────────────────────────────────────────────────────

// StreamingProvider can stream completion tokens via a callback.
// It returns the full text output, any tool calls requested, and an error.
type StreamingProvider interface {
	Provider
	ChatStream(ctx context.Context, system string, messages []ChatMessage, tools []ToolSpec, onToken func(token string)) (string, []ToolCall, error)
}

// OpenAI streaming implementation
func (p *OpenAIProvider) ChatStream(ctx context.Context, system string, messages []ChatMessage, tools []ToolSpec, onToken func(token string)) (string, []ToolCall, error) {
	// Build messages in the same OpenAI wire format as Chat()
	type oaiTC struct {
		ID       string `json:"id,omitempty"`
		Type     string `json:"type"`
		Function struct {
			Name      string `json:"name"`
			Arguments string `json:"arguments"`
		} `json:"function"`
	}
	type oaiMsg struct {
		Role       string          `json:"role"`
		Content    any             `json:"content,omitempty"`
		ToolCallID string          `json:"tool_call_id,omitempty"`
		ToolCalls  json.RawMessage `json:"tool_calls,omitempty"`
	}
	msgs := make([]oaiMsg, 0, len(messages)+1)
	if system != "" {
		msgs = append(msgs, oaiMsg{Role: "system", Content: system})
	}
	for _, m := range messages {
		msg := oaiMsg{Role: m.Role, Content: buildOpenAIContent(m.Role, m.Content), ToolCallID: m.ToolCallID}
		if len(m.ToolCalls) > 0 {
			oaiTCs := make([]oaiTC, 0, len(m.ToolCalls))
			for _, tc := range m.ToolCalls {
				argsJSON, _ := json.Marshal(tc.Arguments)
				if string(argsJSON) == "null" {
					argsJSON = []byte("{}")
				}
				oaiTCs = append(oaiTCs, oaiTC{
					ID:   tc.ID,
					Type: "function",
					Function: struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					}{Name: tc.Name, Arguments: string(argsJSON)},
				})
			}
			raw, _ := json.Marshal(oaiTCs)
			msg.ToolCalls = raw
		}
		msgs = append(msgs, msg)
	}

	reqBody := map[string]any{
		"model":    p.model,
		"messages": msgs,
		"stream":   true,
	}
	if len(tools) > 0 {
		oaiTools := make([]map[string]any, 0, len(tools))
		for _, t := range tools {
			props, _ := t.Parameters["properties"].(map[string]any)
			if props == nil {
				props = map[string]any{}
			}
			var required []string
			if req, ok := t.Parameters["required"].([]string); ok {
				required = req
			}
			oaiTools = append(oaiTools, map[string]any{
				"type": "function",
				"function": map[string]any{
					"name":        t.Name,
					"description": t.Description,
					"parameters": map[string]any{
						"type":       "object",
						"properties": props,
						"required":   required,
						"additionalProperties": false,
					},
				},
			})
		}
		reqBody["tools"] = oaiTools
	}

	b, err := json.Marshal(reqBody)
	if err != nil {
		return "", nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(b))
	if err != nil {
		return "", nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	if p.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.apiKey)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", nil, fmt.Errorf("stream request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return "", nil, fmt.Errorf("http %d: %s", resp.StatusCode, truncate(string(raw), 400))
	}

	// Accumulate tool call arguments by index
	type tcAccum struct {
		id       string
		name     string
		argsJSON strings.Builder
	}
	tcMap := map[int]*tcAccum{}

	var full strings.Builder
	buf := make([]byte, 4096)
	remainder := ""
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			chunk := remainder + string(buf[:n])
			remainder = ""
			lines := strings.Split(chunk, "\n")
			for i, line := range lines {
				line = strings.TrimSpace(line)
				if i == len(lines)-1 && line != "" {
					remainder = line
					continue
				}
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				data := strings.TrimPrefix(line, "data: ")
				if data == "[DONE]" {
					break
				}
				var event struct {
					Choices []struct {
						Delta struct {
							Content   string `json:"content"`
							ToolCalls []struct {
								Index    int    `json:"index"`
								ID       string `json:"id"`
								Type     string `json:"type"`
								Function struct {
									Name      string `json:"name"`
									Arguments string `json:"arguments"`
								} `json:"function"`
							} `json:"tool_calls"`
						} `json:"delta"`
						FinishReason string `json:"finish_reason"`
					} `json:"choices"`
				}
				if err2 := json.Unmarshal([]byte(data), &event); err2 != nil {
					continue
				}
				if len(event.Choices) == 0 {
					continue
				}
				choice := event.Choices[0]
				// Accumulate content tokens
				if choice.Delta.Content != "" {
					full.WriteString(choice.Delta.Content)
					onToken(choice.Delta.Content)
				}
				// Accumulate tool call deltas
				for _, tc := range choice.Delta.ToolCalls {
					acc, ok := tcMap[tc.Index]
					if !ok {
						acc = &tcAccum{}
						tcMap[tc.Index] = acc
					}
					if tc.ID != "" {
						acc.id = tc.ID
					}
					if tc.Function.Name != "" {
						acc.name = tc.Function.Name
					}
					if tc.Function.Arguments != "" {
						acc.argsJSON.WriteString(tc.Function.Arguments)
					}
				}
			}
		}
		if readErr != nil {
			break
		}
	}

	// If tool calls were accumulated, return them
	if len(tcMap) > 0 {
		calls := make([]ToolCall, 0, len(tcMap))
		for i := 0; i < len(tcMap); i++ {
			acc, ok := tcMap[i]
			if !ok {
				continue
			}
			args, err := parseToolCallArguments([]byte(acc.argsJSON.String()))
			if err != nil {
				return "", nil, fmt.Errorf("openai stream: parse tool call %s arguments for %s: %v (raw: %s)", acc.id, acc.name, err, truncate(acc.argsJSON.String(), 200))
			}
			calls = append(calls, ToolCall{
				ID:        acc.id,
				Name:      acc.name,
				Arguments: args,
				Status:    "pending",
			})
		}
		return "", calls, nil
	}

	result := full.String()
	if result == "" {
		return "", nil, fmt.Errorf("openai stream: empty response")
	}
	return result, nil, nil
}
