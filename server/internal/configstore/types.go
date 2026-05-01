package configstore

import (
	"encoding/json"
	"time"
)

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
	Schedules    ScheduleConfig     `json:"schedules"`
	Heartbeat    HeartbeatConfig    `json:"heartbeat"`
	Voice        VoiceConfig        `json:"voice"`
	UpdatedAt    string             `json:"updatedAt"`
}

// VoiceConfig holds TTS preferences for the user.
type VoiceConfig struct {
	// DefaultVoice is the Kokoro voice used when no per-language override matches.
	DefaultVoice string `json:"defaultVoice"`
	// LanguageVoices maps BCP-47 language codes (e.g. "en", "ja", "fr") to voice IDs.
	// When the detected language of the TTS text matches a key, the mapped voice is used.
	LanguageVoices map[string]string `json:"languageVoices,omitempty"`
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
	EmailAccounts            []EmailAccountConfig `json:"emailAccounts"`
	TelegramBots             []TelegramBotConfig  `json:"telegramBots"`
	SSHServers               []SSHServerConfig    `json:"sshServers"`
	DAV                      []DAVConfig          `json:"dav"`
	CompanionApps            []CompanionAppConfig `json:"companionApps"`
	DefaultNotificationBotID string               `json:"defaultNotificationBotId,omitempty"`
}

type DAVConfig struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	URL                 string `json:"url"`
	Username            string `json:"username"`
	Password            string `json:"password"`
	Enabled             bool   `json:"enabled"`
	WebDAVEnabled       bool   `json:"webdavEnabled"`
	CalDAVEnabled       bool   `json:"caldavEnabled"`
	CardDAVEnabled      bool   `json:"carddavEnabled"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds"`
}

func (c *IntegrationsConfig) UnmarshalJSON(data []byte) error {
	type integrationsAlias struct {
		EmailAccounts            []EmailAccountConfig `json:"emailAccounts"`
		TelegramBots             []TelegramBotConfig  `json:"telegramBots"`
		SSHServers               []SSHServerConfig    `json:"sshServers"`
		DAV                      json.RawMessage      `json:"dav"`
		CompanionApps            []CompanionAppConfig `json:"companionApps"`
		DefaultNotificationBotID string               `json:"defaultNotificationBotId,omitempty"`
	}

	var aux integrationsAlias
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	c.EmailAccounts = aux.EmailAccounts
	c.TelegramBots = aux.TelegramBots
	c.SSHServers = aux.SSHServers
	c.CompanionApps = aux.CompanionApps
	c.DefaultNotificationBotID = aux.DefaultNotificationBotID

	if len(aux.DAV) == 0 || string(aux.DAV) == "null" {
		c.DAV = nil
		return nil
	}

	type davWire struct {
		ID                  string `json:"id"`
		Name                string `json:"name"`
		URL                 string `json:"url"`
		Username            string `json:"username"`
		Password            string `json:"password"`
		Enabled             *bool  `json:"enabled"`
		WebDAVEnabled       *bool  `json:"webdavEnabled"`
		CalDAVEnabled       *bool  `json:"caldavEnabled"`
		CardDAVEnabled      *bool  `json:"carddavEnabled"`
		PollIntervalSeconds int    `json:"pollIntervalSeconds"`
	}
	fromWire := func(w davWire) DAVConfig {
		enabled := false
		if w.Enabled != nil {
			enabled = *w.Enabled
		}
		webdavEnabled := true
		if w.WebDAVEnabled != nil {
			webdavEnabled = *w.WebDAVEnabled
		}
		caldavEnabled := true
		if w.CalDAVEnabled != nil {
			caldavEnabled = *w.CalDAVEnabled
		}
		carddavEnabled := true
		if w.CardDAVEnabled != nil {
			carddavEnabled = *w.CardDAVEnabled
		}
		return DAVConfig{
			ID:                  w.ID,
			Name:                w.Name,
			URL:                 w.URL,
			Username:            w.Username,
			Password:            w.Password,
			Enabled:             enabled,
			WebDAVEnabled:       webdavEnabled,
			CalDAVEnabled:       caldavEnabled,
			CardDAVEnabled:      carddavEnabled,
			PollIntervalSeconds: w.PollIntervalSeconds,
		}
	}

	if aux.DAV[0] == '[' {
		var arr []davWire
		if err := json.Unmarshal(aux.DAV, &arr); err != nil {
			return err
		}
		c.DAV = make([]DAVConfig, 0, len(arr))
		for _, d := range arr {
			c.DAV = append(c.DAV, fromWire(d))
		}
		return nil
	}

	var single davWire
	if err := json.Unmarshal(aux.DAV, &single); err != nil {
		return err
	}
	c.DAV = []DAVConfig{fromWire(single)}
	return nil
}

type CompanionAppConfig struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Label        string `json:"label"`
	Enabled      bool   `json:"enabled"`
	PushEndpoint string `json:"pushEndpoint,omitempty"`
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
	// KnownHostKey is the base64-encoded public key fingerprint (e.g. "ssh-ed25519 AAAA...")
	// used to verify the remote host. When empty, the connection proceeds without
	// host-key verification (insecure; logs a warning).
	KnownHostKey string `json:"knownHostKey,omitempty"`
	Enabled      bool   `json:"enabled"`
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
	ID                  string `json:"id"`
	Label               string `json:"label"`
	Address             string `json:"address"`
	ImapHost            string `json:"imapHost"`
	ImapPort            int    `json:"imapPort"`
	ImapUsername        string `json:"imapUsername"`
	ImapPassword        string `json:"imapPassword"`
	SmtpHost            string `json:"smtpHost"`
	SmtpPort            int    `json:"smtpPort"`
	UseTLS              bool   `json:"useTls"`
	Enabled             bool   `json:"enabled"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds"`
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
