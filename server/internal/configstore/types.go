package configstore

import "time"

type rootConfig struct {
	Version   int                   `json:"version"`
	Users     map[string]UserConfig `json:"users"`
	UpdatedAt time.Time             `json:"updatedAt"`
}

type UserConfig struct {
	Integrations IntegrationsConfig `json:"integrations"`
	Tools        ToolsConfig        `json:"tools"`
	MCP          MCPConfig          `json:"mcp"`
	LinuxSandbox LinuxSandboxConfig `json:"linuxSandbox"`
	LLM          LLMConfig          `json:"llm"`
	Skills       SkillsConfig       `json:"skills"`
	Prompts      PromptsConfig      `json:"prompts"`
	Memory       MemoryConfig       `json:"memory"`
	Schedules    ScheduleConfig     `json:"schedules"`
	Heartbeat    HeartbeatConfig    `json:"heartbeat"`
	UpdatedAt    string             `json:"updatedAt"`
}

type SkillsConfig struct {
	Entries []SkillEntry `json:"entries"`
}

type SkillEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Content     string `json:"content"`
	Enabled     bool   `json:"enabled"`
}

type IntegrationsConfig struct {
	EmailAccounts []EmailAccountConfig `json:"emailAccounts"`
	TelegramBots  []TelegramBotConfig  `json:"telegramBots"`
	SSHServers    []SSHServerConfig    `json:"sshServers"`
}

type SSHServerConfig struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	AuthMode   string `json:"authMode"` // "key" or "password"
	SSHKey     string `json:"sshKey,omitempty"`
	Password   string `json:"password,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
	Enabled    bool   `json:"enabled"`
}

type TelegramBotConfig struct {
	ID                  string   `json:"id"`
	Label               string   `json:"label"`
	BotToken            string   `json:"botToken"`
	AllowedChatIDs      []string `json:"allowedChatIds"`
	NotificationChatID  string   `json:"notificationChatId"`
	Enabled             bool     `json:"enabled"`
	PollIntervalSeconds int      `json:"pollIntervalSeconds"`
	LastUpdateID        int64    `json:"lastUpdateId"`
}

type MCPConfig struct {
	Servers []MCPServerConfig `json:"servers"`
}

type MCPServerConfig struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Enabled bool              `json:"enabled"`
}

type EmailAccountConfig struct {
	ID           string `json:"id"`
	Label        string `json:"label"`
	Address      string `json:"address"`
	ImapHost     string `json:"imapHost"`
	ImapPort     int    `json:"imapPort"`
	ImapUsername string `json:"imapUsername"`
	ImapPassword string `json:"imapPassword"`
	SmtpHost     string `json:"smtpHost"`
	SmtpPort     int    `json:"smtpPort"`
	UseTLS       bool   `json:"useTls"`
	Enabled      bool   `json:"enabled"`
}

type ToolsConfig struct {
	Definitions []ToolDefinition  `json:"definitions"`
	Enabled     map[string]bool   `json:"enabled"`
	GolangTools []GolangToolEntry `json:"golangTools"`
	Toggles     map[string]bool   `json:"toggles,omitempty"`
}

type ToolDefinition struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Source      string          `json:"source"`
	Parameters  []ToolParameter `json:"parameters"`
}

type ToolParameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

type GolangToolEntry struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	SourceCode  string `json:"sourceCode"`
	Enabled     bool   `json:"enabled"`
}

type LinuxSandboxConfig struct {
	Enabled bool `json:"enabled"`
}

type LLMConfig struct {
	Providers []ProviderConfig `json:"providers"`
}

type ProviderConfig struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	BaseURL   string `json:"baseUrl,omitempty"`
	APIKeyRef string `json:"apiKeyRef,omitempty"`
	Model     string `json:"model,omitempty"`
	Enabled   bool   `json:"enabled"`
	Priority  int    `json:"priority"` // Lower = higher priority (0 = first). Used for fallback order.
}

type PromptsConfig struct {
	SystemPrompt    string `json:"systemPrompt"`
	HeartbeatPrompt string `json:"heartbeatPrompt"`
}

type MemoryConfig struct {
	Entries []MemoryEntry `json:"entries"`
}

type MemoryEntry struct {
	ID       string `json:"id"`
	Category string `json:"category"`
	Content  string `json:"content"`
	Strength int    `json:"strength"`
}

type ScheduleConfig struct {
	Entries []ScheduleEntry `json:"entries"`
}

type ScheduleEntry struct {
	ID             string  `json:"id"`
	Description    string  `json:"description"`
	Prompt         string  `json:"prompt"`
	ExecuteAt      string  `json:"executeAt"`
	CronExpression *string `json:"cronExpression,omitempty"`
	Status         string  `json:"status"`
}

type HeartbeatConfig struct {
	Enabled         bool              `json:"enabled"`
	IntervalSeconds int               `json:"intervalSeconds"`
	ActiveHours     HeartbeatSchedule `json:"activeHours"`
	Model           string            `json:"model"`
}

type HeartbeatSchedule struct {
	Start string `json:"start"`
	End   string `json:"end"`
	TZ    string `json:"tz"`
}
