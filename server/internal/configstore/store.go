package configstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type Store struct {
	path string
	mu   sync.Mutex
}

func New(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("config path is required")
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("mkdir config dir: %w", err)
	}

	return &Store{path: path}, nil
}

func (s *Store) GetUserConfig(userID string) (UserConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	root, err := s.loadLocked()
	if err != nil {
		return UserConfig{}, err
	}

	cfg, ok := root.Users[userID]
	if !ok {
		cfg = DefaultUserConfig()
		root.Users[userID] = cfg
		if err := s.saveLocked(root); err != nil {
			return UserConfig{}, err
		}
	}

	return normalize(cfg), nil
}

func (s *Store) ListUserIDs() ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	root, err := s.loadLocked()
	if err != nil {
		return nil, err
	}

	ids := make([]string, 0, len(root.Users))
	for id := range root.Users {
		ids = append(ids, id)
	}
	return ids, nil
}

func (s *Store) PutUserConfig(userID string, cfg UserConfig) (UserConfig, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	root, err := s.loadLocked()
	if err != nil {
		return UserConfig{}, err
	}

	normalized := normalize(cfg)
	normalized.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	root.Users[userID] = normalized

	if err := s.saveLocked(root); err != nil {
		return UserConfig{}, err
	}

	return normalized, nil
}

func (s *Store) loadLocked() (rootConfig, error) {
	root := rootConfig{
		Version:   1,
		Users:     map[string]UserConfig{},
		UpdatedAt: time.Now().UTC(),
	}

	bytes, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return root, nil
		}
		return rootConfig{}, fmt.Errorf("read config file: %w", err)
	}

	if len(bytes) == 0 {
		return root, nil
	}

	if err := json.Unmarshal(bytes, &root); err != nil {
		return rootConfig{}, fmt.Errorf("decode config file: %w", err)
	}
	if root.Users == nil {
		root.Users = map[string]UserConfig{}
	}
	if root.Version == 0 {
		root.Version = 1
	}

	return root, nil
}

func (s *Store) saveLocked(root rootConfig) error {
	root.UpdatedAt = time.Now().UTC()

	bytes, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return fmt.Errorf("encode config file: %w", err)
	}

	tmpPath := s.path + ".tmp"
	if err := os.WriteFile(tmpPath, bytes, 0o600); err != nil {
		return fmt.Errorf("write temp config file: %w", err)
	}

	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("replace config file: %w", err)
	}

	return nil
}

func normalize(cfg UserConfig) UserConfig {
	def := DefaultUserConfig()

	if cfg.Tools.Definitions == nil {
		cfg.Tools.Definitions = []ToolDefinition{}
	}
	if len(cfg.Tools.Definitions) == 0 && len(cfg.Tools.Toggles) > 0 {
		for name, enabled := range cfg.Tools.Toggles {
			id := uuid.NewString()
			cfg.Tools.Definitions = append(cfg.Tools.Definitions, ToolDefinition{
				ID:          id,
				Name:        name,
				Description: "",
				Source:      "builtin",
				Parameters:  []ToolParameter{},
			})
			if cfg.Tools.Enabled == nil {
				cfg.Tools.Enabled = map[string]bool{}
			}
			cfg.Tools.Enabled[id] = enabled
		}
	}
	if cfg.Tools.Enabled == nil {
		cfg.Tools.Enabled = map[string]bool{}
	}
	for i := range cfg.Tools.Definitions {
		if cfg.Tools.Definitions[i].ID == "" {
			cfg.Tools.Definitions[i].ID = uuid.NewString()
		}
		if cfg.Tools.Definitions[i].Source == "" {
			cfg.Tools.Definitions[i].Source = "custom"
		}
		if cfg.Tools.Definitions[i].Parameters == nil {
			cfg.Tools.Definitions[i].Parameters = []ToolParameter{}
		}
		if _, ok := cfg.Tools.Enabled[cfg.Tools.Definitions[i].ID]; !ok {
			cfg.Tools.Enabled[cfg.Tools.Definitions[i].ID] = true
		}
	}

	for _, defaultTool := range def.Tools.Definitions {
		exists := false
		for i, existing := range cfg.Tools.Definitions {
			if strings.EqualFold(existing.Name, defaultTool.Name) {
				exists = true
				// Migrate name/description from defaultTools (handles space->snake_case rename)
				cfg.Tools.Definitions[i].Name = defaultTool.Name
				cfg.Tools.Definitions[i].Description = defaultTool.Description
				cfg.Tools.Definitions[i].Parameters = defaultTool.Parameters
				break
			}
			// Also check old space-based names against new snake_case defaults
			oldName := strings.ReplaceAll(defaultTool.Name, "_", " ")
			if strings.EqualFold(existing.Name, oldName) {
				exists = true
				cfg.Tools.Definitions[i].Name = defaultTool.Name
				cfg.Tools.Definitions[i].Description = defaultTool.Description
				cfg.Tools.Definitions[i].Parameters = defaultTool.Parameters
				break
			}
			// Migrate renamed tools
			renames := map[string]string{"ssh_execute": "remote_execute", "send_notification": "send_channel_notification"}
			if old, ok := renames[defaultTool.Name]; ok && strings.EqualFold(existing.Name, old) {
				exists = true
				cfg.Tools.Definitions[i].Name = defaultTool.Name
				cfg.Tools.Definitions[i].Description = defaultTool.Description
				cfg.Tools.Definitions[i].Parameters = defaultTool.Parameters
				break
			}
		}
		if !exists {
			id := uuid.NewString()
			defaultTool.ID = id
			cfg.Tools.Definitions = append(cfg.Tools.Definitions, defaultTool)
			cfg.Tools.Enabled[id] = true
		}
	}
	cfg.Tools.Toggles = nil

	if cfg.Tools.GolangTools == nil {
		cfg.Tools.GolangTools = []GolangToolEntry{}
	}
	for i := range cfg.Tools.GolangTools {
		if cfg.Tools.GolangTools[i].ID == "" {
			cfg.Tools.GolangTools[i].ID = uuid.NewString()
		}
	}

	if cfg.Integrations.EmailAccounts == nil {
		cfg.Integrations.EmailAccounts = []EmailAccountConfig{}
	}
	for i := range cfg.Integrations.EmailAccounts {
		if cfg.Integrations.EmailAccounts[i].ID == "" {
			cfg.Integrations.EmailAccounts[i].ID = uuid.NewString()
		}
		if cfg.Integrations.EmailAccounts[i].ImapPort <= 0 {
			cfg.Integrations.EmailAccounts[i].ImapPort = 993
		}
		if cfg.Integrations.EmailAccounts[i].SmtpPort <= 0 {
			cfg.Integrations.EmailAccounts[i].SmtpPort = 587
		}
	}
	if cfg.Integrations.TelegramBots == nil {
		cfg.Integrations.TelegramBots = []TelegramBotConfig{}
	}
	if cfg.Integrations.SSHServers == nil {
		cfg.Integrations.SSHServers = []SSHServerConfig{}
	}
	if cfg.Integrations.DAV == nil {
		cfg.Integrations.DAV = []DAVConfig{}
	}
	for i := range cfg.Integrations.DAV {
		if cfg.Integrations.DAV[i].ID == "" {
			cfg.Integrations.DAV[i].ID = uuid.NewString()
		}
		if strings.TrimSpace(cfg.Integrations.DAV[i].Name) == "" {
			cfg.Integrations.DAV[i].Name = fmt.Sprintf("DAV %d", i+1)
		}
		if cfg.Integrations.DAV[i].PollIntervalSeconds <= 0 {
			cfg.Integrations.DAV[i].PollIntervalSeconds = 900
		}
	}
	for i := range cfg.Integrations.SSHServers {
		if cfg.Integrations.SSHServers[i].ID == "" {
			cfg.Integrations.SSHServers[i].ID = uuid.NewString()
		}
		if cfg.Integrations.SSHServers[i].Port <= 0 {
			cfg.Integrations.SSHServers[i].Port = 22
		}
		if cfg.Integrations.SSHServers[i].AuthMode == "" {
			cfg.Integrations.SSHServers[i].AuthMode = "key"
		}
	}
	for i := range cfg.Integrations.TelegramBots {
		if cfg.Integrations.TelegramBots[i].ID == "" {
			cfg.Integrations.TelegramBots[i].ID = uuid.NewString()
		}
		if cfg.Integrations.TelegramBots[i].AllowedChatIDs == nil {
			cfg.Integrations.TelegramBots[i].AllowedChatIDs = []string{}
		}
		if cfg.Integrations.TelegramBots[i].PollIntervalSeconds <= 0 {
			cfg.Integrations.TelegramBots[i].PollIntervalSeconds = 5
		}
	}

	if cfg.LLM.Providers == nil {
		cfg.LLM.Providers = []ProviderConfig{}
	}
	for i := range cfg.LLM.Providers {
		if cfg.LLM.Providers[i].ID == "" {
			cfg.LLM.Providers[i].ID = uuid.NewString()
		}
	}

	if cfg.MCP.Servers == nil {
		cfg.MCP.Servers = []MCPServerConfig{}
	}
	for i := range cfg.MCP.Servers {
		if cfg.MCP.Servers[i].ID == "" {
			cfg.MCP.Servers[i].ID = uuid.NewString()
		}
		if cfg.MCP.Servers[i].Headers == nil {
			cfg.MCP.Servers[i].Headers = map[string]string{}
		}
	}

	if cfg.Skills.Entries == nil {
		cfg.Skills.Entries = []SkillEntry{}
	}
	for i := range cfg.Skills.Entries {
		if cfg.Skills.Entries[i].ID == "" {
			cfg.Skills.Entries[i].ID = uuid.NewString()
		}
	}

	if cfg.Memory.Entries == nil {
		cfg.Memory.Entries = []MemoryEntry{}
	}
	for i := range cfg.Memory.Entries {
		if cfg.Memory.Entries[i].ID == "" {
			cfg.Memory.Entries[i].ID = uuid.NewString()
		}
		if cfg.Memory.Entries[i].Strength <= 0 {
			cfg.Memory.Entries[i].Strength = 1
		}
	}

	if cfg.Schedules.Entries == nil {
		cfg.Schedules.Entries = []ScheduleEntry{}
	}
	for i := range cfg.Schedules.Entries {
		if cfg.Schedules.Entries[i].ID == "" {
			cfg.Schedules.Entries[i].ID = uuid.NewString()
		}
		if cfg.Schedules.Entries[i].Status == "" {
			cfg.Schedules.Entries[i].Status = "PENDING"
		}
	}

	if cfg.Heartbeat.IntervalSeconds <= 0 {
		cfg.Heartbeat.IntervalSeconds = def.Heartbeat.IntervalSeconds
	}
	if cfg.Heartbeat.ActiveHours.Start == "" {
		cfg.Heartbeat.ActiveHours.Start = def.Heartbeat.ActiveHours.Start
	}
	if cfg.Heartbeat.ActiveHours.End == "" {
		cfg.Heartbeat.ActiveHours.End = def.Heartbeat.ActiveHours.End
	}
	if cfg.Heartbeat.ActiveHours.TZ == "" {
		cfg.Heartbeat.ActiveHours.TZ = def.Heartbeat.ActiveHours.TZ
	}

	// Linux sandbox is now always on.
	cfg.LinuxSandbox.Enabled = true

	if cfg.Prompts.SystemPrompt == "" {
		cfg.Prompts.SystemPrompt = def.Prompts.SystemPrompt
	}
	legacyHeartbeatPrompt := strings.TrimSpace(LegacyHeartbeatPrompt)
	currentHeartbeatPrompt := strings.TrimSpace(cfg.Prompts.HeartbeatPrompt)
	isLegacyDefaultPrompt := currentHeartbeatPrompt == legacyHeartbeatPrompt ||
		(strings.Contains(currentHeartbeatPrompt, "This is an automatic self-check") &&
			strings.Contains(currentHeartbeatPrompt, "respond with exactly: HEARTBEAT_OK"))
	if currentHeartbeatPrompt == "" || isLegacyDefaultPrompt {
		cfg.Prompts.HeartbeatPrompt = def.Prompts.HeartbeatPrompt
		// Migrate legacy heartbeat defaults for users that still have the old heartbeat prompt untouched.
		if cfg.Heartbeat.IntervalSeconds == 300 {
			cfg.Heartbeat.IntervalSeconds = def.Heartbeat.IntervalSeconds
		}
		if !cfg.Heartbeat.Enabled {
			cfg.Heartbeat.Enabled = def.Heartbeat.Enabled
		}
	}

	if cfg.UpdatedAt == "" {
		cfg.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	return cfg
}
