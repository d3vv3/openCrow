package scheduler

import (
	"testing"
	"time"
)

func TestBackoffPolicy_NextDelay(t *testing.T) {
	p := BackoffPolicy{BaseDelay: 2 * time.Second, MaxDelay: 2 * time.Minute}

	tests := []struct {
		attempt int
		want    time.Duration
	}{
		{0, 2 * time.Second}, // clamped to 1
		{1, 2 * time.Second},
		{2, 4 * time.Second},
		{3, 8 * time.Second},
		{4, 16 * time.Second},
		{5, 32 * time.Second},
		{6, 64 * time.Second},
		{7, 2 * time.Minute}, // capped
		{100, 2 * time.Minute},
	}

	for _, tt := range tests {
		got := p.NextDelay(tt.attempt)
		if got != tt.want {
			t.Errorf("NextDelay(%d) = %v, want %v", tt.attempt, got, tt.want)
		}
	}
}

func TestBackoffPolicy_Defaults(t *testing.T) {
	p := BackoffPolicy{} // zero values
	got := p.NextDelay(1)
	if got != 2*time.Second {
		t.Errorf("default NextDelay(1) = %v, want 2s", got)
	}
	got = p.NextDelay(100)
	if got != 2*time.Minute {
		t.Errorf("default NextDelay(100) = %v, want 2m", got)
	}
}
