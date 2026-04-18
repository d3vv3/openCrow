package app

import (
	"os"
	"testing"
)

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_GET_ENV_KEY", "val")
	defer os.Unsetenv("TEST_GET_ENV_KEY")

	if got := getEnv("TEST_GET_ENV_KEY", "default"); got != "val" {
		t.Errorf("got %q", got)
	}
	if got := getEnv("TEST_MISSING_KEY_XYZ", "default"); got != "default" {
		t.Errorf("got %q", got)
	}
}

func TestParseDurationWithDefault(t *testing.T) {
	d, err := parseDurationWithDefault("MISSING_DUR_KEY", "5m")
	if err != nil {
		t.Fatal(err)
	}
	if d.Minutes() != 5 {
		t.Errorf("duration = %v", d)
	}

	os.Setenv("TEST_DUR", "30s")
	defer os.Unsetenv("TEST_DUR")
	d, err = parseDurationWithDefault("TEST_DUR", "5m")
	if err != nil {
		t.Fatal(err)
	}
	if d.Seconds() != 30 {
		t.Errorf("duration = %v", d)
	}
}

func TestParseBoolWithDefault(t *testing.T) {
	tests := []struct {
		value string
		want  bool
	}{
		{"", false},
		{"1", true},
		{"true", true},
		{"TRUE", true},
		{"yes", true},
		{"on", true},
		{"0", false},
		{"false", false},
		{"no", false},
	}
	for _, tt := range tests {
		os.Setenv("TEST_BOOL", tt.value)
		got := parseBoolWithDefault("TEST_BOOL", false)
		if got != tt.want {
			t.Errorf("parseBool(%q) = %v, want %v", tt.value, got, tt.want)
		}
	}
	os.Unsetenv("TEST_BOOL")

	// missing key uses fallback
	if parseBoolWithDefault("MISSING_BOOL_KEY", true) != true {
		t.Error("expected true fallback")
	}
}

func TestAddr(t *testing.T) {
	c := Config{APIHost: "0.0.0.0", APIPort: "9090"}
	if c.Addr() != "0.0.0.0:9090" {
		t.Errorf("Addr() = %q", c.Addr())
	}
}
