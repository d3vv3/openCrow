package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// newCORSHandler is a test helper that builds a withCORS-wrapped dummy handler.
func newCORSHandler(origins []string) http.Handler {
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return withCORS(origins, inner)
}

// --- Wildcard CORS ---

func TestCORS_Wildcard_NoOrigin(t *testing.T) {
	h := newCORSHandler([]string{"*"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("ACAO = %q, want *", got)
	}
}

func TestCORS_Wildcard_AnyOrigin(t *testing.T) {
	h := newCORSHandler([]string{"*"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://evil.example.com")
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("ACAO = %q, want *", got)
	}
}

func TestCORS_Wildcard_Preflight(t *testing.T) {
	h := newCORSHandler([]string{"*"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("OPTIONS", "/v1/auth/login", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", rec.Code)
	}
	if got := rec.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Access-Control-Allow-Methods must be set on preflight")
	}
	if got := rec.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("Access-Control-Allow-Headers must be set on preflight")
	}
}

// --- Specific origin allowlist ---

func TestCORS_AllowedOrigin_Match(t *testing.T) {
	h := newCORSHandler([]string{"https://app.example.com"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "https://app.example.com" {
		t.Errorf("ACAO = %q, want specific origin", got)
	}
	if got := rec.Header().Get("Vary"); got != "Origin" {
		t.Errorf("Vary = %q, want Origin", got)
	}
}

func TestCORS_AllowedOrigin_CaseInsensitiveMatch(t *testing.T) {
	h := newCORSHandler([]string{"https://App.Example.Com"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got == "" {
		t.Error("expected allowed origin header for case-insensitive match")
	}
}

func TestCORS_AllowedOrigin_NotAllowed(t *testing.T) {
	h := newCORSHandler([]string{"https://app.example.com"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Origin", "https://evil.attacker.com")
	h.ServeHTTP(rec, req)

	if got := rec.Header().Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("ACAO must be empty for disallowed origin, got %q", got)
	}
	// The request should still be served (CORS is informational -- server decides, browser enforces)
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", rec.Code)
	}
}

func TestCORS_AllowedOrigin_NoOriginHeader(t *testing.T) {
	// Same-origin requests (no Origin header) must always pass through
	h := newCORSHandler([]string{"https://app.example.com"})
	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/", nil)
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("no-origin request: status = %d, want 200", rec.Code)
	}
}

func TestCORS_MultipleAllowedOrigins(t *testing.T) {
	origins := []string{"https://app.example.com", "https://admin.example.com"}
	h := newCORSHandler(origins)

	for _, origin := range origins {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Origin", origin)
		h.ServeHTTP(rec, req)
		if got := rec.Header().Get("Access-Control-Allow-Origin"); got != origin {
			t.Errorf("origin %s: ACAO = %q, want %q", origin, got, origin)
		}
	}
}

// --- parseCORSOrigins ---

func TestParseCORSOrigins_Empty(t *testing.T) {
	got := parseCORSOrigins("")
	if len(got) != 1 || got[0] != "*" {
		t.Errorf("empty = %v, want [*]", got)
	}
}

func TestParseCORSOrigins_Wildcard(t *testing.T) {
	got := parseCORSOrigins("*")
	if len(got) != 1 || got[0] != "*" {
		t.Errorf("wildcard = %v, want [*]", got)
	}
}

func TestParseCORSOrigins_CommaSeparated(t *testing.T) {
	got := parseCORSOrigins("https://a.com, https://b.com , https://c.com")
	if len(got) != 3 {
		t.Fatalf("len = %d, want 3; got %v", len(got), got)
	}
	for _, o := range []string{"https://a.com", "https://b.com", "https://c.com"} {
		found := false
		for _, g := range got {
			if g == o {
				found = true
			}
		}
		if !found {
			t.Errorf("origin %q not found in result %v", o, got)
		}
	}
}

// --- WebSocket origin checking ---

func TestWSUpgrader_AllowAllOrigins(t *testing.T) {
	wu := newWSUpgrader([]string{"*"})
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Origin", "https://evil.com")
	if !wu.upgrader.CheckOrigin(req) {
		t.Error("wildcard upgrader should allow all origins")
	}
}

func TestWSUpgrader_AllowedOrigin(t *testing.T) {
	wu := newWSUpgrader([]string{"https://app.example.com"})
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Origin", "https://app.example.com")
	if !wu.upgrader.CheckOrigin(req) {
		t.Error("should allow explicitly listed origin")
	}
}

func TestWSUpgrader_DisallowedOrigin(t *testing.T) {
	wu := newWSUpgrader([]string{"https://app.example.com"})
	req := httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Origin", "https://evil.attacker.com")
	if wu.upgrader.CheckOrigin(req) {
		t.Error("should deny unlisted origin")
	}
}

func TestWSUpgrader_NoOriginHeader(t *testing.T) {
	// No Origin header (non-browser client) should be allowed
	wu := newWSUpgrader([]string{"https://app.example.com"})
	req := httptest.NewRequest("GET", "/ws", nil)
	if !wu.upgrader.CheckOrigin(req) {
		t.Error("no-Origin request should be allowed")
	}
}
