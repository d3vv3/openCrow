package auth

import (
	"testing"
	"time"
)

func TestNewTokenPairAndParse(t *testing.T) {
	mgr := NewManager("test-issuer", "super-secret-key-32bytes!!", 15*time.Minute, 720*time.Hour)

	pair, err := mgr.NewTokenPair("user-123", "session-456")
	if err != nil {
		t.Fatalf("NewTokenPair: %v", err)
	}
	if pair.SessionID != "session-456" {
		t.Errorf("SessionID = %q, want %q", pair.SessionID, "session-456")
	}
	if pair.AccessToken == "" || pair.RefreshToken == "" {
		t.Fatal("tokens must not be empty")
	}
	if pair.AccessToken == pair.RefreshToken {
		t.Error("access and refresh tokens must differ")
	}

	// Parse access token
	claims, err := mgr.Parse(pair.AccessToken, "access")
	if err != nil {
		t.Fatalf("Parse access: %v", err)
	}
	if claims.UserID != "user-123" {
		t.Errorf("UserID = %q, want %q", claims.UserID, "user-123")
	}
	if claims.SessionID != "session-456" {
		t.Errorf("SessionID = %q, want %q", claims.SessionID, "session-456")
	}
	if claims.Type != "access" {
		t.Errorf("Type = %q, want %q", claims.Type, "access")
	}

	// Parse refresh token
	rClaims, err := mgr.Parse(pair.RefreshToken, "refresh")
	if err != nil {
		t.Fatalf("Parse refresh: %v", err)
	}
	if rClaims.UserID != "user-123" {
		t.Errorf("refresh UserID = %q", rClaims.UserID)
	}
	if rClaims.Type != "refresh" {
		t.Errorf("refresh Type = %q", rClaims.Type)
	}
}

func TestParseWrongType(t *testing.T) {
	mgr := NewManager("iss", "secret-key-for-testing!!", 15*time.Minute, 720*time.Hour)
	pair, _ := mgr.NewTokenPair("u", "s")

	// access token parsed as refresh should fail
	_, err := mgr.Parse(pair.AccessToken, "refresh")
	if err == nil {
		t.Error("expected error parsing access as refresh")
	}

	// refresh token parsed as access should fail
	_, err = mgr.Parse(pair.RefreshToken, "access")
	if err == nil {
		t.Error("expected error parsing refresh as access")
	}
}

func TestParseWrongSecret(t *testing.T) {
	mgr1 := NewManager("iss", "secret-one-32-bytes-long!!", 15*time.Minute, 720*time.Hour)
	mgr2 := NewManager("iss", "secret-two-32-bytes-long!!", 15*time.Minute, 720*time.Hour)

	pair, _ := mgr1.NewTokenPair("u", "s")
	_, err := mgr2.Parse(pair.AccessToken, "access")
	if err == nil {
		t.Error("expected error with wrong secret")
	}
}

func TestParseExpiredToken(t *testing.T) {
	mgr := NewManager("iss", "secret-key-for-testing!!", 1*time.Millisecond, 1*time.Millisecond)
	pair, _ := mgr.NewTokenPair("u", "s")
	time.Sleep(10 * time.Millisecond)

	_, err := mgr.Parse(pair.AccessToken, "access")
	if err == nil {
		t.Error("expected error for expired token")
	}
}

func TestParseGarbage(t *testing.T) {
	mgr := NewManager("iss", "secret", 15*time.Minute, 720*time.Hour)
	_, err := mgr.Parse("not.a.token", "access")
	if err == nil {
		t.Error("expected error for garbage token")
	}
}
