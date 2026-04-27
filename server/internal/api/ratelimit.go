package api

import (
	"net"
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// ipRateLimiter enforces per-IP request rate limits using a token-bucket algorithm.
// Stale entries are pruned automatically every 5 minutes.
type ipRateLimiter struct {
	mu       sync.Mutex
	limiters map[string]*rlEntry
	r        rate.Limit // tokens per second
	b        int        // burst size
}

type rlEntry struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// newIPRateLimiter creates a rate limiter allowing r events per second with burst b.
func newIPRateLimiter(r rate.Limit, b int) *ipRateLimiter {
	rl := &ipRateLimiter{
		limiters: make(map[string]*rlEntry),
		r:        r,
		b:        b,
	}
	go rl.cleanup()
	return rl
}

func (rl *ipRateLimiter) getLimiter(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	e, ok := rl.limiters[ip]
	if !ok {
		e = &rlEntry{limiter: rate.NewLimiter(rl.r, rl.b)}
		rl.limiters[ip] = e
	}
	e.lastSeen = time.Now()
	return e.limiter
}

func (rl *ipRateLimiter) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for range ticker.C {
		rl.mu.Lock()
		for ip, e := range rl.limiters {
			if time.Since(e.lastSeen) > 10*time.Minute {
				delete(rl.limiters, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// middleware wraps a handler and rejects requests that exceed the rate limit.
func (rl *ipRateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := realIP(r)
		if !rl.getLimiter(ip).Allow() {
			writeError(w, http.StatusTooManyRequests, "too many requests")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// realIP extracts the client IP, respecting X-Forwarded-For when the direct
// peer is a private/loopback address (i.e. a trusted reverse proxy).
func realIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// Only trust XFF when the direct connection comes from a private address
		// (meaning a local reverse proxy). Otherwise use RemoteAddr directly to
		// prevent IP spoofing.
		if host, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
			if ip := net.ParseIP(host); ip != nil && (ip.IsLoopback() || ip.IsPrivate()) {
				// Take the first (leftmost / original client) IP from XFF.
				first := xff
				if idx := indexOf(xff, ','); idx >= 0 {
					first = xff[:idx]
				}
				if parsed := net.ParseIP(trimSpace(first)); parsed != nil {
					return parsed.String()
				}
			}
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func indexOf(s string, b byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == b {
			return i
		}
	}
	return -1
}

func trimSpace(s string) string {
	start, end := 0, len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
