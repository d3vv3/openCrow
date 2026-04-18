package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestStubProvider(t *testing.T) {
	p := StubProvider{ProviderName: "test"}
	if p.Name() != "test" {
		t.Errorf("Name() = %q", p.Name())
	}
	msgs := []ChatMessage{{Role: "user", Content: "hello"}}
	out, toolCalls, err := p.Chat(context.Background(), "", msgs, nil)
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if len(toolCalls) != 0 {
		t.Errorf("expected no tool calls, got %d", len(toolCalls))
	}
	if out == "" {
		t.Error("expected non-empty output")
	}
}

func TestStubProviderCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := StubProvider{ProviderName: "x"}
	_, _, err := p.Chat(ctx, "", []ChatMessage{{Role: "user", Content: "hi"}}, nil)
	if err == nil {
		t.Error("expected error on cancelled ctx")
	}
}

func TestServiceComplete(t *testing.T) {
	svc := NewService([]Provider{
		StubProvider{ProviderName: "primary"},
		StubProvider{ProviderName: "fallback"},
	}, ToolLoopGuard{})

	result, err := svc.Complete(context.Background(), CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if result.Output == "" {
		t.Error("expected non-empty output")
	}
	if result.Attempts != 1 {
		t.Errorf("attempts = %d", result.Attempts)
	}
}

func TestServiceCompleteEmptyMessages(t *testing.T) {
	svc := NewService([]Provider{StubProvider{ProviderName: "p"}}, ToolLoopGuard{})
	_, err := svc.Complete(context.Background(), CompletionRequest{})
	if err == nil {
		t.Error("expected error for empty messages")
	}
}

type failProvider struct{ name string }

func (f failProvider) Name() string { return f.name }
func (f failProvider) Chat(ctx context.Context, system string, messages []ChatMessage, tools []ToolSpec) (string, []ToolCall, error) {
	return "", nil, errors.New("fail")
}

type repeatToolProvider struct{}

func (p repeatToolProvider) Name() string { return "repeat" }
func (p repeatToolProvider) Chat(ctx context.Context, system string, messages []ChatMessage, tools []ToolSpec) (string, []ToolCall, error) {
	return "", []ToolCall{{
		ID:        "tc-1",
		Name:      "send_email",
		Arguments: map[string]any{"to": "a@example.com", "subject": "Current Time"},
	}}, nil
}

func TestServiceCompleteFallback(t *testing.T) {
	svc := NewService([]Provider{
		failProvider{name: "bad"},
		StubProvider{ProviderName: "good"},
	}, ToolLoopGuard{})

	result, err := svc.Complete(context.Background(), CompletionRequest{
		Messages:   []ChatMessage{{Role: "user", Content: "test"}},
		MaxRetries: 1,
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if result.Provider != "good" {
		t.Errorf("provider = %q, want good", result.Provider)
	}
}

func TestServiceCompleteAllFail(t *testing.T) {
	svc := NewService([]Provider{failProvider{name: "a"}}, ToolLoopGuard{})
	_, err := svc.Complete(context.Background(), CompletionRequest{
		Messages:   []ChatMessage{{Role: "user", Content: "test"}},
		MaxRetries: 1,
	})
	if err == nil {
		t.Error("expected error when all providers fail")
	}
	var providerErr *ProviderFailureError
	if !errors.As(err, &providerErr) {
		t.Fatalf("expected ProviderFailureError, got %T", err)
	}
	if len(providerErr.Attempts) != 1 {
		t.Fatalf("attempt count = %d, want 1", len(providerErr.Attempts))
	}
	if providerErr.Attempts[0].Provider != "a" {
		t.Fatalf("provider = %q, want a", providerErr.Attempts[0].Provider)
	}
	if providerErr.Attempts[0].Attempt != 1 {
		t.Fatalf("attempt number = %d, want 1", providerErr.Attempts[0].Attempt)
	}
	if got := err.Error(); got == "" || !strings.Contains(got, "a attempt 1: fail") {
		t.Fatalf("unexpected error message: %q", got)
	}
}

func TestServiceCompleteTimeout(t *testing.T) {
	svc := NewService([]Provider{StubProvider{ProviderName: "slow"}}, ToolLoopGuard{})
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(1 * time.Millisecond)
	_, err := svc.Complete(ctx, CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "test"}},
	})
	if err == nil {
		t.Error("expected timeout error")
	}
}

func TestToolLoopGuardValidate(t *testing.T) {
	g := ToolLoopGuard{}.Validate()
	if g.MaxIterations != 10 {
		t.Errorf("MaxIterations = %d, want 10", g.MaxIterations)
	}
	if g.MaxRepeated != 3 {
		t.Errorf("MaxRepeated = %d, want 3", g.MaxRepeated)
	}

	g2 := ToolLoopGuard{MaxIterations: 5, MaxRepeated: 2}.Validate()
	if g2.MaxIterations != 5 || g2.MaxRepeated != 2 {
		t.Errorf("custom values not preserved: %+v", g2)
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		input string
		max   int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello..."},
		{"abcd", 3, "abc"},
		{"ab", 1, "a"},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := truncate(tt.input, tt.max)
		if got != tt.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tt.input, tt.max, got, tt.want)
		}
	}
}

func TestServiceCompleteWithProviderOrder(t *testing.T) {
	svc := NewService([]Provider{
		StubProvider{ProviderName: "alpha"},
		StubProvider{ProviderName: "beta"},
	}, ToolLoopGuard{})

	result, err := svc.Complete(context.Background(), CompletionRequest{
		Messages:      []ChatMessage{{Role: "user", Content: "test"}},
		ProviderOrder: []string{"beta"},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if result.Provider != "beta" {
		t.Errorf("provider = %q, want beta", result.Provider)
	}
}

func TestServiceDetectsRepeatedToolCalls(t *testing.T) {
	svc := NewService([]Provider{repeatToolProvider{}}, ToolLoopGuard{MaxIterations: 10, MaxRepeated: 2})
	_, err := svc.Complete(context.Background(), CompletionRequest{
		Messages: []ChatMessage{{Role: "user", Content: "send email"}},
		Tools: []ToolSpec{{Name: "send_email"}},
		ToolExecutor: func(ctx context.Context, name string, args map[string]any) (string, error) {
			return "ok", nil
		},
		MaxRetries: 1,
	})
	if err == nil {
		t.Fatal("expected repeated tool loop error")
	}
	if !strings.Contains(err.Error(), "tool loop repeated same tool call pattern") {
		t.Fatalf("unexpected error: %v", err)
	}
}
