package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBearerToken(t *testing.T) {
	tests := []struct {
		header string
		want   string
	}{
		{"Bearer abc123", "abc123"},
		{"bearer abc123", "abc123"},
		{"BEARER abc123", "abc123"},
		{"Bearer  abc123 ", "abc123"},
		{"", ""},
		{"Basic abc123", ""},
		{"Bearerabc", ""},
		{"Bearer", ""},
	}
	for _, tt := range tests {
		got := bearerToken(tt.header)
		if got != tt.want {
			t.Errorf("bearerToken(%q) = %q, want %q", tt.header, got, tt.want)
		}
	}
}

func TestChooseDeviceLabel(t *testing.T) {
	if got := chooseDeviceLabel("phone", "tablet"); got != "phone" {
		t.Errorf("got %q", got)
	}
	if got := chooseDeviceLabel("", "tablet"); got != "tablet" {
		t.Errorf("got %q", got)
	}
	if got := chooseDeviceLabel("", ""); got != "unknown-device" {
		t.Errorf("got %q", got)
	}
	if got := chooseDeviceLabel("  ", "  "); got != "unknown-device" {
		t.Errorf("got %q", got)
	}
}

func TestIsUUID(t *testing.T) {
	if !isUUID("550e8400-e29b-41d4-a716-446655440000") {
		t.Error("expected valid UUID")
	}
	if isUUID("not-a-uuid") {
		t.Error("expected invalid")
	}
	if isUUID("") {
		t.Error("expected invalid for empty")
	}
}

func TestTruncateOutput(t *testing.T) {
	if got := truncateOutput("hello", 10); got != "hello" {
		t.Errorf("got %q", got)
	}
	if got := truncateOutput("hello world", 8); got != "hello..." {
		t.Errorf("got %q", got)
	}
	if got := truncateOutput("abc", 0); got != "abc" {
		t.Errorf("got %q for max=0", got)
	}
	if got := truncateOutput("abcd", 3); got != "abc" {
		t.Errorf("got %q for max=3", got)
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})

	if w.Code != 200 {
		t.Errorf("code = %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("content-type = %q", ct)
	}
	if !strings.Contains(w.Body.String(), `"status":"ok"`) {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "bad input")

	if w.Code != 400 {
		t.Errorf("code = %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "bad input") {
		t.Errorf("body = %q", w.Body.String())
	}
}

func TestDecodeJSON(t *testing.T) {
	body := strings.NewReader(`{"username":"admin","password":"secret"}`)
	req := httptest.NewRequest("POST", "/", body)
	var target LoginRequest
	err := decodeJSON(req, &target)
	if err != nil {
		t.Fatalf("decodeJSON: %v", err)
	}
	if target.Username != "admin" {
		t.Errorf("Username = %q", target.Username)
	}
}

func TestDecodeJSON_Invalid(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader("not json"))
	var target LoginRequest
	if err := decodeJSON(req, &target); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestDecodeJSON_MultipleValues(t *testing.T) {
	req := httptest.NewRequest("POST", "/", strings.NewReader(`{}{}`))
	var target LoginRequest
	if err := decodeJSON(req, &target); err == nil {
		t.Error("expected error for multiple JSON values")
	}
}

func TestHashAndVerifyRefreshToken(t *testing.T) {
	token := "my-refresh-token-value"
	hash, err := hashRefreshToken(token)
	if err != nil {
		t.Fatalf("hashRefreshToken: %v", err)
	}
	if hash == "" {
		t.Error("hash is empty")
	}

	if err := verifyRefreshTokenHash(hash, token); err != nil {
		t.Errorf("verifyRefreshTokenHash: %v", err)
	}

	if err := verifyRefreshTokenHash(hash, "wrong-token"); err == nil {
		t.Error("expected error for wrong token")
	}
}

func TestRefreshTokenDigest(t *testing.T) {
	d := refreshTokenDigest("test")
	if len(d) != 64 { // sha256 hex
		t.Errorf("digest len = %d", len(d))
	}
	// deterministic
	if refreshTokenDigest("test") != d {
		t.Error("not deterministic")
	}
}
