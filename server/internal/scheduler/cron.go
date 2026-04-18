package scheduler

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// CronNext returns the next time after `after` that matches the 5-field cron expression.
// Supports: minute hour dom month dow (standard cron, no seconds).
// Special values: * (any), */n (every n), a-b (range), a,b (list).
func CronNext(expr string, after time.Time) (time.Time, error) {
	fields := strings.Fields(expr)
	if len(fields) != 5 {
		return time.Time{}, fmt.Errorf("cron expression must have 5 fields, got %d", len(fields))
	}

	// Start searching from 1 minute after `after`.
	t := after.Truncate(time.Minute).Add(time.Minute)

	// Limit search to 4 years of minutes to avoid infinite loop.
	limit := t.Add(4 * 365 * 24 * time.Hour)

	for t.Before(limit) {
		if matches(fields[3], int(t.Month()), 1, 12) &&
			matches(fields[2], t.Day(), 1, 31) &&
			matches(fields[4], int(t.Weekday()), 0, 6) &&
			matches(fields[1], t.Hour(), 0, 23) &&
			matches(fields[0], t.Minute(), 0, 59) {
			return t, nil
		}
		t = t.Add(time.Minute)
	}
	return time.Time{}, fmt.Errorf("no next time found for cron %q within 4 years", expr)
}

// matches checks if value v satisfies a single cron field spec.
func matches(spec string, v, min, max int) bool {
	if spec == "*" {
		return true
	}
	// Handle comma-separated list
	for _, part := range strings.Split(spec, ",") {
		if matchesPart(part, v, min, max) {
			return true
		}
	}
	return false
}

func matchesPart(part string, v, min, max int) bool {
	// */n
	if strings.HasPrefix(part, "*/") {
		n, err := strconv.Atoi(part[2:])
		if err != nil || n <= 0 {
			return false
		}
		return v%n == 0
	}
	// a-b
	if strings.Contains(part, "-") {
		ab := strings.SplitN(part, "-", 2)
		a, err1 := strconv.Atoi(ab[0])
		b, err2 := strconv.Atoi(ab[1])
		if err1 != nil || err2 != nil {
			return false
		}
		return v >= a && v <= b
	}
	// exact
	n, err := strconv.Atoi(part)
	if err != nil {
		return false
	}
	return v == n
}
