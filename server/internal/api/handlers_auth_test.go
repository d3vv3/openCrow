package api

import (
	"context"
	"testing"
)

func TestSessionIDFromContext(t *testing.T) {
	ctx := context.WithValue(context.Background(), sessionIDContextKey, "  session-123  ")
	got := sessionIDFromContext(ctx)
	if got != "session-123" {
		t.Fatalf("sessionIDFromContext = %q, want session-123", got)
	}
}
