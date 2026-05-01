package configstore

import (
	"os"
	"path/filepath"
	"testing"
)

// TestSaveLocked_FilePermissions verifies that the config file is written with
// mode 0o600 (owner read/write only), preventing other users from reading
// secrets stored in the config.
func TestSaveLocked_FilePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Trigger a write by calling PutUserConfig
	cfg := DefaultUserConfig()
	if _, err := store.PutUserConfig("test-user-id", cfg); err != nil {
		t.Fatalf("PutUserConfig: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat config file: %v", err)
	}

	got := info.Mode().Perm()
	want := os.FileMode(0o600)
	if got != want {
		t.Errorf("file permissions = %04o, want %04o", got, want)
	}
}

// TestSaveLocked_TmpFileCleanedUp verifies that no leftover .tmp file remains
// after a successful save.
func TestSaveLocked_TmpFileCleanedUp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	if _, err := store.PutUserConfig("uid", DefaultUserConfig()); err != nil {
		t.Fatalf("PutUserConfig: %v", err)
	}

	tmpPath := path + ".tmp"
	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Errorf("tmp file %q should not exist after save, err=%v", tmpPath, err)
	}
}

// TestNormalizeConfig_PrunesObsoleteMemoryTools verifies that old flat-memory
// tool names (learn_memory, store_memory, etc.) and renamed graph tool names
// (remember_entity, relate_entities, forget_entity, edit_entity) are removed
// from the definitions list when config is loaded.
func TestNormalizeConfig_PrunesObsoleteMemoryTools(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	store, err := New(path)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// Create a config with old tool definitions injected
	cfg := DefaultUserConfig()
	oldToolNames := []string{"learn_memory", "store_memory", "read_memory", "forget_memory",
		"remember_entity", "relate_entities", "forget_entity", "edit_entity"}
	for _, name := range oldToolNames {
		cfg.Tools.Definitions = append(cfg.Tools.Definitions, ToolDefinition{
			ID:   name,
			Name: name,
		})
		cfg.Tools.Enabled[name] = true
	}

	// Put it (normalize runs on Put)
	if _, err := store.PutUserConfig("uid", cfg); err != nil {
		t.Fatalf("PutUserConfig: %v", err)
	}

	// Read it back
	got, err := store.GetUserConfig("uid")
	if err != nil {
		t.Fatalf("GetUserConfig: %v", err)
	}

	for _, name := range oldToolNames {
		for _, d := range got.Tools.Definitions {
			if d.Name == name {
				t.Errorf("obsolete tool %q was not pruned", name)
			}
		}
	}
}
