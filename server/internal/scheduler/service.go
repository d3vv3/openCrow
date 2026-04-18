package scheduler

import "time"

type BackoffPolicy struct {
	BaseDelay time.Duration
	MaxDelay  time.Duration
}

func (p BackoffPolicy) NextDelay(attempt int) time.Duration {
	if attempt <= 0 {
		attempt = 1
	}
	base := p.BaseDelay
	if base <= 0 {
		base = 2 * time.Second
	}
	max := p.MaxDelay
	if max <= 0 {
		max = 2 * time.Minute
	}

	delay := base
	for i := 1; i < attempt; i++ {
		delay *= 2
		if delay >= max {
			return max
		}
	}

	if delay > max {
		return max
	}
	return delay
}
