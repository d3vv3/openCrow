package configstore

import (
	"time"

	"github.com/google/uuid"
)

var defaultTools = []ToolDefinition{
	{
		ID:          "web_search",
		Name:        "web_search",
		Description: "Search the web and return summarized results.",
		Source:      "builtin",
		Parameters: []ToolParameter{
			{Name: "query", Type: "string", Description: "Search query", Required: true},
		},
	},
	{
		ID:          "get_local_time",
		Name:        "get_local_time",
		Description: "Get the current local time for a given IANA timezone (e.g. America/New_York). Returns ISO8601 timestamp.",
		Source:      "builtin",
		Parameters: []ToolParameter{
			{Name: "timezone", Type: "string", Description: "IANA timezone string, e.g. UTC or America/New_York", Required: false},
		},
	},
	{
		ID:          "get_location",
		Name:        "get_location",
		Description: "Resolve rough location from IP/device context.",
		Source:      "builtin",
		Parameters:  []ToolParameter{},
	},
	{
		ID:          "open_url",
		Name:        "open_url",
		Description: "Open a URL and fetch page content.",
		Source:      "builtin",
		Parameters: []ToolParameter{
			{Name: "url", Type: "string", Description: "Absolute URL", Required: true},
		},
	},
	{
		ID:          "store_memory",
		Name:        "store_memory",
		Description: "Create a memory entry.",
		Source:      "builtin",
		Parameters: []ToolParameter{
			{Name: "content", Type: "string", Description: "Memory text", Required: true},
			{Name: "category", Type: "string", Description: "Memory category", Required: false},
		},
	},
	{
		ID:          "forget_memory",
		Name:        "forget_memory",
		Description: "Remove a memory entry.",
		Source:      "builtin",
		Parameters:  []ToolParameter{{Name: "memoryId", Type: "string", Description: "Memory ID", Required: true}},
	},
	{
		ID:          "learn_memory",
		Name:        "learn_memory",
		Description: "Learn and store structured memory.",
		Source:      "builtin",
		Parameters:  []ToolParameter{{Name: "content", Type: "string", Description: "Memory text", Required: true}},
	},
	{
		ID:          "read_memory",
		Name:        "read_memory",
		Description: "Read all stored memories. Returns a list of memory entries with their category, content, strength, and ID.",
		Source:      "builtin",
		Parameters:  []ToolParameter{},
	},
	{
		ID:          "reinforce_memory",
		Name:        "reinforce_memory",
		Description: "Increase memory strength.",
		Source:      "builtin",
		Parameters:  []ToolParameter{{Name: "memoryId", Type: "string", Description: "Memory ID", Required: true}},
	},
	{
		ID:          "schedule_task",
		Name:        "schedule_task",
		Description: "Create a one-time or cron task.",
		Source:      "builtin",
		Parameters:  []ToolParameter{{Name: "prompt", Type: "string", Description: "Task prompt", Required: true}, {Name: "executeAt", Type: "string", Description: "RFC3339 datetime", Required: true}, {Name: "cronExpression", Type: "string", Description: "Cron expression", Required: false}},
	},
	{
		ID:          "cancel_task",
		Name:        "cancel_task",
		Description: "Cancel/remove scheduled task.",
		Source:      "builtin",
		Parameters:  []ToolParameter{{Name: "taskId", Type: "string", Description: "Task ID", Required: true}},
	},
	{
		ID:          "list_tasks",
		Name:        "list_tasks",
		Description: "List scheduled tasks.",
		Source:      "builtin",
		Parameters:  []ToolParameter{},
	},
	{
		ID:          "promote_learning",
		Name:        "promote_learning",
		Description: "Promote strong memory to preferred behavior.",
		Source:      "builtin",
		Parameters:  []ToolParameter{{Name: "memoryId", Type: "string", Description: "Memory ID", Required: true}},
	},
	{ID: "setup_email", Name: "setup_email", Description: "Configure an email account integration. Auto-detects server settings for Gmail, Outlook, Yahoo, iCloud, etc.", Source: "builtin", Parameters: []ToolParameter{{Name: "address", Type: "string", Description: "Email address", Required: true}, {Name: "password", Type: "string", Description: "Password or app-specific password", Required: true}, {Name: "imap_host", Type: "string", Description: "IMAP server hostname (auto-detected if omitted)", Required: false}, {Name: "imap_port", Type: "integer", Description: "IMAP port (default 993)", Required: false}, {Name: "smtp_host", Type: "string", Description: "SMTP server hostname (auto-detected if omitted)", Required: false}, {Name: "smtp_port", Type: "integer", Description: "SMTP port (default 587)", Required: false}}},
	{ID: "check_email", Name: "check_email", Description: "Fetch inbox summary.", Source: "builtin", Parameters: []ToolParameter{}},
	{ID: "read_email", Name: "read_email", Description: "Read one email by ID.", Source: "builtin", Parameters: []ToolParameter{{Name: "messageId", Type: "string", Description: "Message ID", Required: true}}},
	{ID: "reply_email", Name: "reply_email", Description: "Reply to an email.", Source: "builtin", Parameters: []ToolParameter{{Name: "messageId", Type: "string", Description: "Message ID", Required: true}, {Name: "body", Type: "string", Description: "Reply body", Required: true}}},
	{ID: "compose_email", Name: "compose_email", Description: "Send an email via SMTP using the configured email account.", Source: "builtin", Parameters: []ToolParameter{{Name: "to", Type: "string", Description: "Recipient email address", Required: true}, {Name: "subject", Type: "string", Description: "Email subject", Required: true}, {Name: "body", Type: "string", Description: "Plain-text email body", Required: true}}},
	{ID: "search_email", Name: "search_email", Description: "Search inbox messages.", Source: "builtin", Parameters: []ToolParameter{{Name: "query", Type: "string", Description: "Search query", Required: true}}},
	{ID: "send_notification", Name: "send_notification", Description: "Send notification to user via connected channels (realtime, Telegram, WhatsApp).", Source: "builtin", Parameters: []ToolParameter{{Name: "title", Type: "string", Description: "Notification title", Required: true}, {Name: "body", Type: "string", Description: "Notification body", Required: true}}},
	{ID: "execute_shell_command", Name: "execute_shell_command", Description: "Execute a shell command on the server with full system permissions (can install packages, modify files, run as root, etc). Returns stdout, stderr, and exit code. Each command runs in a fresh shell. Set background=true for long-lived processes. Use timeout parameter for commands that may take a while (e.g. installs).", Source: "builtin", Parameters: []ToolParameter{{Name: "command", Type: "string", Description: "The shell command to execute (has full system permissions)", Required: true}, {Name: "timeout", Type: "integer", Description: "Timeout in seconds (default 300)", Required: false}, {Name: "working_dir", Type: "string", Description: "Working directory for the command", Required: false}, {Name: "background", Type: "boolean", Description: "Run in background and return session_id. Use manage_process to check status.", Required: false}}},
	{ID: "manage_process", Name: "manage_process", Description: "Manage background shell processes. Actions: list, log (session_id, offset, limit), kill (session_id), remove (session_id).", Source: "builtin", Parameters: []ToolParameter{{Name: "action", Type: "string", Description: "Action: list, log, kill, or remove", Required: true}, {Name: "session_id", Type: "string", Description: "Session ID (required for log, kill, remove)", Required: false}, {Name: "offset", Type: "integer", Description: "Line offset for log output (default 0)", Required: false}, {Name: "limit", Type: "integer", Description: "Max lines for log (default 200)", Required: false}}},
	{ID: "list_skills", Name: "list_skills", Description: "List all installed agent skills (name, slug, description). Use get_skill to read the full instructions of a specific skill.", Source: "builtin", Parameters: []ToolParameter{}},
	{ID: "get_skill", Name: "get_skill", Description: "Get the full SKILL.md content of an installed skill by its slug.", Source: "builtin", Parameters: []ToolParameter{{Name: "slug", Type: "string", Description: "Skill slug (from list_skills)", Required: true}}},
	{ID: "install_skills", Name: "install_skills", Description: "Install agent skills from a GitHub repository. Downloads all SKILL.md files and saves them to the skills directory. Source format: 'owner/repo' or a full GitHub URL (e.g. 'vercel-labs/agent-skills').", Source: "builtin", Parameters: []ToolParameter{{Name: "source", Type: "string", Description: "GitHub source, e.g. 'vercel-labs/agent-skills'", Required: true}}},
	{ID: "list_mcp_servers", Name: "list_mcp_servers", Description: "List configured MCP servers.", Source: "builtin", Parameters: []ToolParameter{}},
	{ID: "add_mcp_server", Name: "add_mcp_server", Description: "Add (or update by name/URL) an MCP server configuration.", Source: "builtin", Parameters: []ToolParameter{{Name: "name", Type: "string", Description: "Display name for the MCP server", Required: true}, {Name: "url", Type: "string", Description: "Base URL for the MCP server", Required: true}, {Name: "enabled", Type: "boolean", Description: "Whether the MCP server is enabled (default true)", Required: false}, {Name: "headers", Type: "object", Description: "Optional HTTP headers map", Required: false}}},
	{ID: "remove_mcp_server", Name: "remove_mcp_server", Description: "Remove an MCP server configuration by id, name, or url.", Source: "builtin", Parameters: []ToolParameter{{Name: "id", Type: "string", Description: "MCP server id", Required: false}, {Name: "name", Type: "string", Description: "MCP server name", Required: false}, {Name: "url", Type: "string", Description: "MCP server URL", Required: false}}},
	{ID: "ssh_execute", Name: "ssh_execute", Description: "Execute a shell command on a remote server over SSH. Returns stdout, stderr and exit code. Each command runs in a fresh shell. Set background=true for long-lived processes. Use timeout parameter for commands that may take a while (e.g. installs).", Source: "builtin", Parameters: []ToolParameter{{Name: "serverName", Type: "string", Description: "Name of the configured SSH server", Required: true}, {Name: "command", Type: "string", Description: "Shell command to execute", Required: true}, {Name: "timeout", Type: "integer", Description: "Timeout in seconds (default 300)", Required: false}, {Name: "working_dir", Type: "string", Description: "Working directory for the command", Required: false}, {Name: "background", Type: "boolean", Description: "Run in background (nohup). Returns immediately.", Required: false}}},
}

const DefaultSystemPrompt = `You're not a chatbot. You're a personal assistant who grows with your user.

## How to Be

**Be genuinely helpful.** Skip the "Great question!" and "I'd be happy to help!" -- just help. Actions speak louder than filler words.

**Have opinions.** You're allowed to disagree, prefer things, or find stuff interesting. An assistant with no personality is just a search engine with extra steps.

**Be resourceful.** Try to figure it out from context and your memories before asking. Come back with answers, not questions. Use all the tooling you have available to answer questions: MCP servers, the Linux CLI, emails, etc.

**Be concise.** Short and clear by default. Go deeper when the topic calls for it.

## Boundaries

- Respect privacy. Don't repeat sensitive information unnecessarily.
- When in doubt about an action, ask first.
- Be honest when you don't know something.`

const LegacyHeartbeatPrompt = `[HEARTBEAT] This is an automatic self-check. Review your memories and pending tasks. If everything looks good and nothing needs attention, respond with exactly: HEARTBEAT_OK
If something needs attention (stale memories, due tasks, user follow-ups), address it.`

const DefaultHeartbeatPrompt = `[HEARTBEAT] This is an automatic self-check.

Review your memories, pending tasks, and recent context.
Collect important news relevant to the user (their interests, location, ongoing topics, and priorities), then decide if anything needs attention.

If everything looks good and there is nothing noteworthy, respond with exactly:
HEARTBEAT_OK

If anything needs attention, respond concisely with what changed and what action is recommended.`

func DefaultUserConfig() UserConfig {
	toolDefinitions := make([]ToolDefinition, 0, len(defaultTools))
	toolEnabled := make(map[string]bool, len(defaultTools))
	for _, tool := range defaultTools {
		id := uuid.NewString()
		toolDefinitions = append(toolDefinitions, ToolDefinition{
			ID:          id,
			Name:        tool.Name,
			Description: tool.Description,
			Source:      tool.Source,
			Parameters:  tool.Parameters,
		})
		toolEnabled[id] = true
	}

	return UserConfig{
		Integrations: IntegrationsConfig{EmailAccounts: []EmailAccountConfig{}, TelegramBots: []TelegramBotConfig{}, SSHServers: []SSHServerConfig{}},
		Tools: ToolsConfig{
			Definitions: toolDefinitions,
			Enabled:     toolEnabled,
			GolangTools: []GolangToolEntry{},
		},
		MCP: MCPConfig{Servers: []MCPServerConfig{}},
		LinuxSandbox: LinuxSandboxConfig{
			Enabled: true,
		},
		LLM: LLMConfig{
			Providers: []ProviderConfig{},
		},
		Skills: SkillsConfig{Entries: []SkillEntry{}},
		Prompts: PromptsConfig{
			SystemPrompt:    DefaultSystemPrompt,
			HeartbeatPrompt: DefaultHeartbeatPrompt,
		},
		Memory:    MemoryConfig{Entries: []MemoryEntry{}},
		Schedules: ScheduleConfig{Entries: []ScheduleEntry{}},
		Heartbeat: HeartbeatConfig{
			Enabled:         true,
			IntervalSeconds: 900,
			ActiveHours: HeartbeatSchedule{
				Start: "08:00",
				End:   "22:00",
				TZ:    "UTC",
			},
			Model: "",
		},
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}
