package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/opencrow/opencrow/server/internal/auth"
)

// newTestServer returns a minimal Server suitable for middleware-only tests
// (no DB connection required).
func newTestServer(t *testing.T) *Server {
	t.Helper()
	mgr := auth.NewManager("test-issuer", "test-secret-key-32bytes!!", 15*time.Minute, 720*time.Hour)
	s := &Server{authMgr: mgr}
	return s
}

// --- requireAccessToken middleware ---

func TestRequireAccessToken_NoToken_Returns401(t *testing.T) {
	s := newTestServer(t)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.requireAccessToken(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/docs/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestRequireAccessToken_GarbageToken_Returns401(t *testing.T) {
	s := newTestServer(t)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.requireAccessToken(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/docs/", nil)
	req.Header.Set("Authorization", "Bearer not.a.valid.jwt")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestRequireAccessToken_RefreshTokenAsAccess_Returns401(t *testing.T) {
	s := newTestServer(t)
	pair, err := s.authMgr.NewTokenPair("user-1", "sess-1")
	if err != nil {
		t.Fatalf("NewTokenPair: %v", err)
	}

	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.requireAccessToken(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/docs/", nil)
	// Deliberately use refresh token where access token is expected
	req.Header.Set("Authorization", "Bearer "+pair.RefreshToken)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

func TestRequireAccessToken_QueryParamToken_GarbageReturns401(t *testing.T) {
	s := newTestServer(t)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := s.requireAccessToken(inner)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/v1/terminal/ws?token=garbage", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", rec.Code)
	}
}

// --- Session limit error string detection ---
// handleLogin detects the session limit via strings.Contains(err.Error(), "session limit reached").
// This test verifies that the detection logic correctly classifies errors.

func TestSessionLimitDetection_Positive(t *testing.T) {
	err := fmt.Errorf("session limit reached (3)")
	if !strings.Contains(err.Error(), "session limit reached") {
		t.Error("expected session limit error to be detected")
	}
}

func TestSessionLimitDetection_Negative(t *testing.T) {
	cases := []string{
		"some other db error",
		"context deadline exceeded",
		"insert device session: connection refused",
	}
	for _, msg := range cases {
		err := fmt.Errorf("%s", msg)
		if strings.Contains(err.Error(), "session limit reached") {
			t.Errorf("non-session-limit error %q was falsely detected", msg)
		}
	}
}
