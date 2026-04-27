package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/time/rate"
)

func TestRateLimiter_AllowsUnderLimit(t *testing.T) {
	// 10 req/s, burst 5 -- should allow first 5 immediately
	rl := newIPRateLimiter(rate.Limit(10), 5)
	handler := rl.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/v1/auth/login", nil)
		req.RemoteAddr = "10.0.0.1:1234"
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i+1, rec.Code)
		}
	}
}

func TestRateLimiter_BlocksOverLimit(t *testing.T) {
	// Very restrictive: 1 token total, no refill during test
	rl := newIPRateLimiter(rate.Limit(0.001), 1)
	handler := rl.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("POST", "/", nil)
	req1.RemoteAddr = "10.0.0.2:1234"
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first request should pass, got %d", rec1.Code)
	}

	// Second request must be rate-limited
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/", nil)
	req2.RemoteAddr = "10.0.0.2:1234"
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second request should be rate-limited (429), got %d", rec2.Code)
	}
}

func TestRateLimiter_SeparateLimitsPerIP(t *testing.T) {
	// Burst of 1 per IP
	rl := newIPRateLimiter(rate.Limit(0.001), 1)
	handler := rl.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, ip := range []string{"1.2.3.4", "1.2.3.5", "1.2.3.6"} {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/", nil)
		req.RemoteAddr = ip + ":5000"
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("first request from %s: expected 200, got %d", ip, rec.Code)
		}
	}
}

func TestRateLimiter_XForwardedFor(t *testing.T) {
	rl := newIPRateLimiter(rate.Limit(0.001), 1)
	handler := rl.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request from forwarded IP should pass
	rec1 := httptest.NewRecorder()
	req1 := httptest.NewRequest("POST", "/", nil)
	req1.Header.Set("X-Forwarded-For", "203.0.113.1")
	req1.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(rec1, req1)
	if rec1.Code != http.StatusOK {
		t.Fatalf("first forwarded request: expected 200, got %d", rec1.Code)
	}

	// Second from same forwarded IP should be blocked
	rec2 := httptest.NewRecorder()
	req2 := httptest.NewRequest("POST", "/", nil)
	req2.Header.Set("X-Forwarded-For", "203.0.113.1")
	req2.RemoteAddr = "127.0.0.1:1234"
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusTooManyRequests {
		t.Fatalf("second forwarded request: expected 429, got %d", rec2.Code)
	}
}

func TestRateLimiter_RecoverAfterWait(t *testing.T) {
	// 10 req/s, burst 1 -- should recover after 100ms
	rl := newIPRateLimiter(rate.Limit(10), 1)
	handler := rl.middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	send := func() int {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("POST", "/", nil)
		req.RemoteAddr = "9.9.9.9:1234"
		handler.ServeHTTP(rec, req)
		return rec.Code
	}

	if code := send(); code != http.StatusOK {
		t.Fatalf("first: want 200, got %d", code)
	}
	if code := send(); code != http.StatusTooManyRequests {
		t.Fatalf("second immediate: want 429, got %d", code)
	}

	time.Sleep(150 * time.Millisecond) // wait for token to refill

	if code := send(); code != http.StatusOK {
		t.Fatalf("after wait: want 200, got %d", code)
	}
}
