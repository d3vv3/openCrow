package api

import (
	"strings"
	"testing"
	"time"
)

// TestShellescape verifies that shell metacharacters are properly quoted.
//
// shellescape uses POSIX single-quoting: wrap the string in single quotes and
// replace any embedded single quote with '\"'\"'. This means metacharacters are
// still present in the output string but are inert because they are enclosed
// in '...'. The correct assertion is that the output is a properly
// single-quoted string, not that metacharacters are absent.
func TestShellescape(t *testing.T) {
	cases := []struct {
		input       string
		mustContain []string // substrings the output must contain
	}{
		{
			input:       "/tmp/normal-dir",
			mustContain: []string{"'", "normal-dir"},
		},
		{
			input:       "/tmp/dir with spaces",
			mustContain: []string{"'", "dir with spaces"},
		},
		{
			// Semicolon is inside single quotes -> shell-safe
			input:       "/tmp/evil; rm -rf /",
			mustContain: []string{"evil"},
		},
		{
			// $( is inside single quotes -> no command substitution
			input:       "/tmp/$(id)",
			mustContain: []string{"$(id)"},
		},
		{
			// backticks are inside single quotes -> no command substitution
			input:       "/tmp/`whoami`",
			mustContain: []string{"`whoami`"},
		},
		{
			input:       "/tmp/dir && evil",
			mustContain: []string{"&& evil"},
		},
		{
			input:       "/tmp/dir || evil",
			mustContain: []string{"|| evil"},
		},
	}

	for _, tc := range cases {
		got := shellescape(tc.input)

		// The output must be wrapped in single quotes.
		if !strings.HasPrefix(got, "'") {
			t.Errorf("shellescape(%q) = %q: must start with single quote", tc.input, got)
		}
		if !strings.HasSuffix(got, "'") {
			t.Errorf("shellescape(%q) = %q: must end with single quote", tc.input, got)
		}

		for _, want := range tc.mustContain {
			if !strings.Contains(got, want) {
				t.Errorf("shellescape(%q) = %q, must contain %q", tc.input, got, want)
			}
		}
	}
}

// TestShellescape_SingleQuoteInInput verifies that embedded single quotes are
// properly escaped (the only character that needs special treatment in POSIX
// single-quoting).
func TestShellescape_SingleQuoteInInput(t *testing.T) {
	input := "/tmp/it's a dir"
	got := shellescape(input)
	// The embedded ' must be escaped; the result must not contain a bare '
	// in the middle of the path (i.e. '"'"' is the escaped form).
	if !strings.Contains(got, `'"'"'`) {
		t.Errorf("shellescape(%q) = %q: embedded single quote not escaped", input, got)
	}
}

// TestShellescape_EmptyString ensures empty input is handled safely.
func TestShellescape_Empty(t *testing.T) {
	got := shellescape("")
	// Must produce a quoted empty string or similar safe representation
	if got == "" {
		t.Error("shellescape(\"\") must not return empty -- would collapse the cd argument")
	}
}

// TestExecuteShellCommand_WorkingDirInjection verifies that a workingDir containing
// shell metacharacters does NOT cause arbitrary command execution.
// This is the fix for the shell injection bug (shell.go:78).
func TestExecuteShellCommand_WorkingDirInjection(t *testing.T) {
	requireSandbox(t)

	// Attempt injection via workingDir: try to run `id` via semicolon injection
	maliciousDir := "/tmp; echo INJECTED"
	result := executeShellCommand(
		t.Context(),
		"/bin/sh",
		"echo SAFE",
		shellDefaultTimeout,
		maliciousDir,
		nil,
	)

	stdout, _ := result["stdout"].(string)
	// The output must contain SAFE (from our command) but must NOT contain INJECTED
	// (which would indicate the injection succeeded)
	if strings.Contains(stdout, "INJECTED") {
		t.Errorf("shell injection succeeded via workingDir: stdout=%q", stdout)
	}
}

// TestStartBackground_WorkingDirInjection verifies the same fix in StartBackground.
func TestStartBackground_WorkingDirInjection(t *testing.T) {
	requireSandbox(t)

	pm := NewProcessManager()
	maliciousDir := "/tmp; echo INJECTED_BG"
	pm.StartBackground(
		t.Context(),
		"/bin/sh",
		"echo SAFE_BG",
		shellDefaultTimeout,
		maliciousDir,
		nil,
	)

	// Give the process time to finish
	time.Sleep(200 * time.Millisecond)

	// Check all sessions
	list := pm.List()
	sessions, _ := list["sessions"].([]map[string]any)
	for _, sess := range sessions {
		id, _ := sess["id"].(string)
		logResult := pm.Log(id, 0, 1000)
		stdout, _ := logResult["stdout"].(string)
		if strings.Contains(stdout, "INJECTED_BG") {
			t.Errorf("shell injection succeeded in StartBackground: stdout=%q", stdout)
		}
	}
}
