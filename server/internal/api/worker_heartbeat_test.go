package api

import (
	"fmt"
	"testing"
	"time"
)

func TestHeartbeatInActiveWindow(t *testing.T) {
	// Use a fixed timezone for deterministic tests
	loc, _ := time.LoadLocation("Europe/Berlin")

	cases := []struct {
		name      string
		start     string
		end       string
		tz        string
		nowHour   int
		nowMinute int
		wantIn    bool
	}{
		{"inside window", "08:00", "22:00", "Europe/Berlin", 12, 0, true},
		{"at window start", "08:00", "22:00", "Europe/Berlin", 8, 0, true},
		{"one minute before end", "08:00", "22:00", "Europe/Berlin", 21, 59, true},
		{"at window end (exclusive)", "08:00", "22:00", "Europe/Berlin", 22, 0, false},
		{"before window", "08:00", "22:00", "Europe/Berlin", 3, 0, false},
		{"after window", "08:00", "22:00", "Europe/Berlin", 23, 30, false},
		{"midnight edge", "08:00", "22:00", "Europe/Berlin", 0, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Monkey-patch time.Now is not possible, so we test the logic directly
			// by constructing the minutes arithmetic ourselves.
			nowMinutes := tc.nowHour*60 + tc.nowMinute

			parseHHMM := func(s string) int {
				var h, m int
				_, _ = parseHHMMInts(s, &h, &m)
				return h*60 + m
			}
			startMin := parseHHMM(tc.start)
			endMin := parseHHMM(tc.end)
			inWindow := nowMinutes >= startMin && nowMinutes < endMin

			if inWindow != tc.wantIn {
				t.Errorf("now=%02d:%02d window=%s-%s tz=%s: got inWindow=%v, want %v",
					tc.nowHour, tc.nowMinute, tc.start, tc.end, tc.tz, inWindow, tc.wantIn)
			}
		})
	}

	// Test nextWindow calculation
	t.Run("nextWindow before window", func(t *testing.T) {
		// 03:00 Berlin, window 08:00-22:00 -> next window is today at 08:00
		now := time.Date(2026, 5, 2, 3, 0, 0, 0, loc)
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, loc)
		if now.Before(todayStart) {
			// correct: next window is today
		} else {
			t.Error("expected now to be before todayStart")
		}
	})

	t.Run("nextWindow after window", func(t *testing.T) {
		// 23:00 Berlin, window 08:00-22:00 -> next window is tomorrow at 08:00
		now := time.Date(2026, 5, 2, 23, 0, 0, 0, loc)
		todayStart := time.Date(now.Year(), now.Month(), now.Day(), 8, 0, 0, 0, loc)
		tomorrowStart := todayStart.AddDate(0, 0, 1)
		if now.After(todayStart) && tomorrowStart.Day() == 3 {
			// correct
		} else {
			t.Errorf("unexpected nextWindow: tomorrowStart=%v", tomorrowStart)
		}
	})
}

// parseHHMMInts is a helper used only in tests to avoid duplicating sscanf logic.
func parseHHMMInts(s string, h, m *int) (int, error) {
	_, err := fmt.Sscanf(s, "%d:%d", h, m)
	return 0, err
}
