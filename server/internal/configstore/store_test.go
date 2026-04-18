package configstore

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew_EmptyPath(t *testing.T) {
	_, err := New("")
	if err == nil {
		t.Error("expected error for empty path")
	}
}

func TestStoreGetPut(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Get creates default
	cfg, err := store.GetUserConfig("user1")
	if err != nil {
		t.Fatalf("GetUserConfig: %v", err)
	}
	if cfg.Prompts.SystemPrompt == "" {
		t.Error("expected default system prompt")
	}
	if len(cfg.Tools.Definitions) == 0 {
		t.Error("expected default tool definitions")
	}

	// File should exist now
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file not created: %v", err)
	}

	// Put custom config
	cfg.Prompts.SystemPrompt = "custom prompt"
	saved, err := store.PutUserConfig("user1", cfg)
	if err != nil {
		t.Fatalf("PutUserConfig: %v", err)
	}
	if saved.Prompts.SystemPrompt != "custom prompt" {
		t.Errorf("SystemPrompt = %q", saved.Prompts.SystemPrompt)
	}

	// Re-get should return saved
	cfg2, err := store.GetUserConfig("user1")
	if err != nil {
		t.Fatalf("GetUserConfig after put: %v", err)
	}
	if cfg2.Prompts.SystemPrompt != "custom prompt" {
		t.Errorf("SystemPrompt after re-read = %q", cfg2.Prompts.SystemPrompt)
	}
}

func TestDefaultUserConfig(t *testing.T) {
	cfg := DefaultUserConfig()
	if len(cfg.Tools.Definitions) == 0 {
		t.Error("expected builtin tools")
	}
	if len(cfg.Tools.Enabled) == 0 {
		t.Error("expected enabled map")
	}
	// All tools should be enabled
	for _, def := range cfg.Tools.Definitions {
		if !cfg.Tools.Enabled[def.ID] {
			t.Errorf("tool %q not enabled", def.Name)
		}
	}
	if cfg.LinuxSandbox.Shell != "/bin/bash" {
		t.Errorf("Shell = %q", cfg.LinuxSandbox.Shell)
	}
	if cfg.Heartbeat.IntervalSeconds != 300 {
		t.Errorf("IntervalSeconds = %d", cfg.Heartbeat.IntervalSeconds)
	}
}

func TestNormalize_FillsDefaults(t *testing.T) {
	cfg := UserConfig{} // empty
	n := normalize(cfg)

	if n.LinuxSandbox.Shell == "" {
		t.Error("shell should be filled")
	}
	if n.Prompts.SystemPrompt == "" {
		t.Error("system prompt should be filled")
	}
	if n.Heartbeat.IntervalSeconds <= 0 {
		t.Error("heartbeat interval should be filled")
	}
	if len(n.Tools.Definitions) == 0 {
		t.Error("should have default tools after normalize")
	}
}

func TestNormalize_AssignsIDs(t *testing.T) {
	cfg := UserConfig{
		Memory:    MemoryConfig{Entries: []MemoryEntry{{Content: "test"}}},
		Schedules: ScheduleConfig{Entries: []ScheduleEntry{{Description: "t"}}},
	}
	n := normalize(cfg)
	if n.Memory.Entries[0].ID == "" {
		t.Error("memory entry should get ID")
	}
	if n.Memory.Entries[0].Strength != 1 {
		t.Errorf("strength = %d, want 1", n.Memory.Entries[0].Strength)
	}
	if n.Schedules.Entries[0].ID == "" {
		t.Error("schedule entry should get ID")
	}
	if n.Schedules.Entries[0].Status != "PENDING" {
		t.Errorf("status = %q", n.Schedules.Entries[0].Status)
	}
}

func TestDefaultTools_NoCalendarOrAlarm(t *testing.T) {
	cfg := DefaultUserConfig()
	for _, def := range cfg.Tools.Definitions {
		if def.Name == "create_calendar_event" || def.Name == "set_alarm" {
			t.Errorf("tool %q should have been removed", def.Name)
		}
	}
}

func TestDefaultTools_SetupEmailHasPassword(t *testing.T) {
	cfg := DefaultUserConfig()
	for _, def := range cfg.Tools.Definitions {
		if def.Name == "setup_email" {
			hasPassword := false
			for _, p := range def.Parameters {
				if p.Name == "password" && p.Required {
					hasPassword = true
				}
			}
			if !hasPassword {
				t.Error("setup_email should have required 'password' parameter")
			}
			return
		}
	}
	t.Error("setup_email tool not found")
}

func TestDefaultTools_ShellToolsExist(t *testing.T) {
	cfg := DefaultUserConfig()
	found := map[string]bool{}
	for _, def := range cfg.Tools.Definitions {
		found[def.Name] = true
	}
	if !found["execute_shell_command"] {
		t.Error("missing execute_shell_command tool")
	}
	if !found["manage_process"] {
		t.Error("missing manage_process tool")
	}
}

func TestDefaultTools_SendNotificationExists(t *testing.T) {
	cfg := DefaultUserConfig()
	for _, def := range cfg.Tools.Definitions {
		if def.Name == "send_notification" {
			return
		}
	}
	t.Error("send_notification tool not found")
}

func TestMultipleUsers(t *testing.T) {
	dir := t.TempDir()
	store, _ := New(filepath.Join(dir, "config.json"))

	store.PutUserConfig("u1", UserConfig{Prompts: PromptsConfig{SystemPrompt: "one"}})
	store.PutUserConfig("u2", UserConfig{Prompts: PromptsConfig{SystemPrompt: "two"}})

	c1, _ := store.GetUserConfig("u1")
	c2, _ := store.GetUserConfig("u2")
	if c1.Prompts.SystemPrompt != "one" {
		t.Errorf("u1 prompt = %q", c1.Prompts.SystemPrompt)
	}
	if c2.Prompts.SystemPrompt != "two" {
		t.Errorf("u2 prompt = %q", c2.Prompts.SystemPrompt)
	}
}
