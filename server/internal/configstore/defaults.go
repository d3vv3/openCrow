package configstore

import (
	"time"

	"github.com/google/uuid"
)

var defaultTools = []ToolDefinition{
	{
		ID:          "web_search",
		Name:        "web_search",
		Description: "Search the web using DuckDuckGo and return up to 10 results (title, URL, snippet). Use for current events, factual lookups, or anything outside your training data. Pass the query exactly as the user phrased it -- do NOT append the current year or date.",
		Source:      "builtin",
		Parameters: []ToolParameter{
			{Name: "query", Type: "string", Description: "Search query -- pass verbatim, do not append year or date", Required: true},
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
		Description: "Resolve the server's approximate location from its public IP address (city, region, country, lat/lon). This is server-side IP geolocation -- not the user's device GPS. Use when the user asks where the server is or needs a rough location for weather/timezone lookups. For precise device GPS, use the companion device tools instead.",
		Source:      "builtin",
		Parameters:  []ToolParameter{},
	},
	{
		ID:          "open_url",
		Name:        "open_url",
		Description: "Fetch a URL server-side and return its text content (HTML tags stripped). Content is truncated at 20 000 characters -- if the response is cut off, the last field will end with '...'. Use web_search first to find URLs, then open_url to read the full page.",
		Source:      "builtin",
		Parameters: []ToolParameter{
			{Name: "url", Type: "string", Description: "Absolute URL to fetch (must be http:// or https://)", Required: true},
		},
	},
	{ID: "check_email", Name: "check_email", Description: "Fetch the most recent messages from the configured inbox. Returns subject, sender, date, and sequence number for each. Use the seq number with read_email to fetch a full message body, or search_email to find specific messages. Use response_format='concise' to save tokens when you only need to scan subjects.", Source: "builtin", Parameters: []ToolParameter{
		{Name: "limit", Type: "integer", Description: "Number of recent messages to return (default 10, max 50)", Required: false},
		{Name: "response_format", Type: "string", Description: "Output verbosity: 'detailed' (default, full header objects with seq/subject/from/date/flags) or 'concise' (compact one-line strings, fewer tokens)", Required: false},
	}},
	{ID: "read_email", Name: "read_email", Description: "Fetch the full body of a single email by its sequence number (seq from check_email or search_email). Returns the raw message body up to 8 000 characters.", Source: "builtin", Parameters: []ToolParameter{{Name: "message_seq", Type: "string", Description: "Message sequence number (seq) from check_email or search_email", Required: true}}},
	{ID: "compose_email", Name: "compose_email", Description: "Send a new email via SMTP using the configured email account. To reply to an existing message, include the original subject prefixed with 'Re: ' and address it to the original sender.", Source: "builtin", Parameters: []ToolParameter{{Name: "to", Type: "string", Description: "Recipient email address", Required: true}, {Name: "subject", Type: "string", Description: "Email subject", Required: true}, {Name: "body", Type: "string", Description: "Plain-text email body", Required: true}}},
	{ID: "search_email", Name: "search_email", Description: "Search inbox messages by subject or body text. Returns up to 10 matching messages with seq numbers. Use read_email to fetch the full body of a result.", Source: "builtin", Parameters: []ToolParameter{{Name: "query", Type: "string", Description: "Search text to match against subject and body", Required: true}}},
	{ID: "send_channel_notification", Name: "send_channel_notification", Description: "Send a notification to the user via connected messaging channels (Telegram bots, realtime hub). Use this for chat-app style alerts. For companion device push notifications use send_push_notification instead.", Source: "builtin", Parameters: []ToolParameter{{Name: "title", Type: "string", Description: "Notification title", Required: true}, {Name: "body", Type: "string", Description: "Notification body", Required: true}}},
	{ID: "send_push_notification", Name: "send_push_notification", Description: "Send a push notification directly to the user's companion app device(s) via UnifiedPush. Only works when the companion app has a registered UnifiedPush distributor. Do NOT use queue_device_action for sending push notifications.", Source: "builtin", Parameters: []ToolParameter{{Name: "title", Type: "string", Description: "Notification title", Required: true}, {Name: "body", Type: "string", Description: "Notification body", Required: true}, {Name: "channel", Type: "string", Description: "Notification channel: 'default' or 'alert' (default: 'default')", Required: false}, {Name: "device_id", Type: "string", Description: "Target a specific device by ID (omit to send to all registered devices)", Required: false}, {Name: "conversation_id", Type: "string", Description: "Conversation ID to open when the notification is tapped (omit to open default chat)", Required: false}}},
	{ID: "list_webdav_files", Name: "list_webdav_files", Description: "List files and collections from the configured WebDAV endpoint.", Source: "builtin", Parameters: []ToolParameter{{Name: "dav_id", Type: "string", Description: "DAV integration ID (optional if only one is configured)", Required: false}, {Name: "path", Type: "string", Description: "Optional DAV path or absolute href to inspect", Required: false}, {Name: "depth", Type: "integer", Description: "Depth for PROPFIND listing (default 1)", Required: false}}},
	{ID: "list_caldav_calendars", Name: "list_caldav_calendars", Description: "List discovered CalDAV calendars.", Source: "builtin", Parameters: []ToolParameter{}},
	{ID: "list_carddav_address_books", Name: "list_carddav_address_books", Description: "List discovered CardDAV address books.", Source: "builtin", Parameters: []ToolParameter{}},
	{ID: "create_caldav_event", Name: "create_caldav_event", Description: "Create a basic iCalendar event inside a CalDAV calendar.", Source: "builtin", Parameters: []ToolParameter{{Name: "dav_id", Type: "string", Description: "DAV integration ID (optional if only one is configured)", Required: false}, {Name: "calendar_path", Type: "string", Description: "Calendar collection href/path", Required: true}, {Name: "summary", Type: "string", Description: "Event title", Required: true}, {Name: "starts_at", Type: "string", Description: "RFC3339 start timestamp", Required: true}, {Name: "ends_at", Type: "string", Description: "RFC3339 end timestamp", Required: true}, {Name: "description", Type: "string", Description: "Optional event description", Required: false}, {Name: "location", Type: "string", Description: "Optional event location", Required: false}, {Name: "uid", Type: "string", Description: "Optional event UID / filename stem", Required: false}}},
	{ID: "delete_caldav_event", Name: "delete_caldav_event", Description: "Delete a CalDAV event by its href/path.", Source: "builtin", Parameters: []ToolParameter{{Name: "dav_id", Type: "string", Description: "DAV integration ID (optional if only one is configured)", Required: false}, {Name: "event_path", Type: "string", Description: "Event href/path to delete", Required: true}}},
	{ID: "create_carddav_contact", Name: "create_carddav_contact", Description: "Create a basic vCard contact inside a CardDAV address book.", Source: "builtin", Parameters: []ToolParameter{{Name: "dav_id", Type: "string", Description: "DAV integration ID (optional if only one is configured)", Required: false}, {Name: "address_book_path", Type: "string", Description: "Address book href/path", Required: true}, {Name: "full_name", Type: "string", Description: "Display name", Required: true}, {Name: "email", Type: "string", Description: "Email address", Required: false}, {Name: "phone", Type: "string", Description: "Phone number", Required: false}, {Name: "note", Type: "string", Description: "Optional note", Required: false}, {Name: "uid", Type: "string", Description: "Optional contact UID / filename stem", Required: false}}},
	{ID: "delete_carddav_contact", Name: "delete_carddav_contact", Description: "Delete a CardDAV contact by its href/path.", Source: "builtin", Parameters: []ToolParameter{{Name: "dav_id", Type: "string", Description: "DAV integration ID (optional if only one is configured)", Required: false}, {Name: "contact_path", Type: "string", Description: "Contact href/path to delete", Required: true}}},
	{ID: "list_caldav_events", Name: "list_caldav_events", Description: "List CalDAV events in a calendar, optionally within a time range. Use get_current_time first to obtain the current date before constructing time ranges.", Source: "builtin", Parameters: []ToolParameter{{Name: "dav_id", Type: "string", Description: "DAV integration ID (optional if only one is configured)", Required: false}, {Name: "calendar_path", Type: "string", Description: "Calendar collection href/path", Required: true}, {Name: "starts_at", Type: "string", Description: "Range start. Accepts RFC3339 (\"2025-05-02T09:00:00Z\"), datetime without timezone (\"2025-05-02T09:00:00\", assumed UTC), or date-only (\"2025-05-02\", midnight UTC).", Required: false}, {Name: "ends_at", Type: "string", Description: "Range end. Same formats as starts_at. Must be after starts_at.", Required: false}, {Name: "limit", Type: "integer", Description: "Max events to return (default 50)", Required: false}}},
	{ID: "get_caldav_event", Name: "get_caldav_event", Description: "Fetch one CalDAV event by href/path.", Source: "builtin", Parameters: []ToolParameter{{Name: "dav_id", Type: "string", Description: "DAV integration ID (optional if only one is configured)", Required: false}, {Name: "event_path", Type: "string", Description: "Event href/path", Required: true}}},
	{ID: "search_caldav_events", Name: "search_caldav_events", Description: "Search CalDAV events by text in summary/description/location.", Source: "builtin", Parameters: []ToolParameter{{Name: "dav_id", Type: "string", Description: "DAV integration ID (optional if only one is configured)", Required: false}, {Name: "calendar_path", Type: "string", Description: "Calendar collection href/path", Required: true}, {Name: "query", Type: "string", Description: "Search text", Required: true}, {Name: "starts_at", Type: "string", Description: "Range start. Accepts RFC3339 (\"2025-05-02T09:00:00Z\"), datetime without timezone (\"2025-05-02T09:00:00\", assumed UTC), or date-only (\"2025-05-02\", midnight UTC).", Required: false}, {Name: "ends_at", Type: "string", Description: "Range end. Same formats as starts_at. Must be after starts_at.", Required: false}, {Name: "limit", Type: "integer", Description: "Max matches to return (default 50)", Required: false}}},
	{ID: "list_carddav_contacts", Name: "list_carddav_contacts", Description: "List CardDAV contacts in an address book.", Source: "builtin", Parameters: []ToolParameter{{Name: "dav_id", Type: "string", Description: "DAV integration ID (optional if only one is configured)", Required: false}, {Name: "address_book_path", Type: "string", Description: "Address book href/path", Required: true}, {Name: "limit", Type: "integer", Description: "Max contacts to return (default 100)", Required: false}}},
	{ID: "get_carddav_contact", Name: "get_carddav_contact", Description: "Fetch one CardDAV contact by href/path.", Source: "builtin", Parameters: []ToolParameter{{Name: "dav_id", Type: "string", Description: "DAV integration ID (optional if only one is configured)", Required: false}, {Name: "contact_path", Type: "string", Description: "Contact href/path", Required: true}}},
	{ID: "search_carddav_contacts", Name: "search_carddav_contacts", Description: "Search CardDAV contacts by name/email/phone/note text.", Source: "builtin", Parameters: []ToolParameter{{Name: "dav_id", Type: "string", Description: "DAV integration ID (optional if only one is configured)", Required: false}, {Name: "address_book_path", Type: "string", Description: "Address book href/path", Required: true}, {Name: "query", Type: "string", Description: "Search text", Required: true}, {Name: "limit", Type: "integer", Description: "Max matches to return (default 100)", Required: false}}},
	{ID: "execute_shell_command", Name: "execute_shell_command", Description: "Execute a shell command inside a persistent Alpine Linux sandbox (chroot). The sandbox has its own filesystem, installed packages persist across calls. You can install packages with apk add. Returns stdout, stderr, and exit code. Each command runs in a fresh shell. Set background=true for long-lived processes. Use timeout parameter for commands that may take a while (e.g. installs).", Source: "builtin", Parameters: []ToolParameter{{Name: "command", Type: "string", Description: "The shell command to execute (runs inside the Alpine Linux sandbox)", Required: true}, {Name: "timeout", Type: "integer", Description: "Timeout in seconds (default 300)", Required: false}, {Name: "working_dir", Type: "string", Description: "Working directory for the command (inside the sandbox)", Required: false}, {Name: "background", Type: "boolean", Description: "Run in background and return session_id. Use manage_process to check status.", Required: false}}},
	{ID: "manage_process", Name: "manage_process", Description: "Manage background shell processes. Actions: list, log (session_id, offset, limit), kill (session_id), remove (session_id).", Source: "builtin", Parameters: []ToolParameter{{Name: "action", Type: "string", Description: "Action: list, log, kill, or remove", Required: true}, {Name: "session_id", Type: "string", Description: "Session ID (required for log, kill, remove)", Required: false}, {Name: "offset", Type: "integer", Description: "Line offset for log output (default 0)", Required: false}, {Name: "limit", Type: "integer", Description: "Max lines for log (default 200)", Required: false}}},
	{ID: "list_skills", Name: "list_skills", Description: "List all installed agent skills (name, slug, description). Use get_skill to read the full instructions of a specific skill.", Source: "builtin", Parameters: []ToolParameter{}},
	{ID: "get_skill", Name: "get_skill", Description: "Get the full SKILL.md content of an installed skill by its slug.", Source: "builtin", Parameters: []ToolParameter{{Name: "slug", Type: "string", Description: "Skill slug (from list_skills)", Required: true}}},
	{ID: "transcribe_audio", Name: "transcribe_audio", Description: "Transcribe an audio or video file to text using Whisper. Provide the absolute path to a local audio/video file (mp3, mp4, m4a, wav, ogg, webm, etc). Returns the full transcript.", Source: "builtin", Parameters: []ToolParameter{{Name: "path", Type: "string", Description: "Absolute path to the audio or video file on the server filesystem", Required: true}}},
	{ID: "ssh_execute", Name: "ssh_execute", Description: "Execute a shell command on a remote server over SSH. Returns stdout, stderr and exit code. Each command runs in a fresh shell. Set background=true for long-lived processes. Use timeout parameter for commands that may take a while (e.g. installs).", Source: "builtin", Parameters: []ToolParameter{{Name: "serverName", Type: "string", Description: "Name of the configured SSH server", Required: true}, {Name: "command", Type: "string", Description: "Shell command to execute", Required: true}, {Name: "timeout", Type: "integer", Description: "Timeout in seconds (default 300)", Required: false}, {Name: "working_dir", Type: "string", Description: "Working directory for the command", Required: false}, {Name: "background", Type: "boolean", Description: "Run in background (nohup). Returns immediately.", Required: false}}},
	{ID: "queue_device_action", Name: "queue_device_action", Description: "Schedules an action to be executed by a remote companion device (e.g., the user's phone). The device will receive this instruction via heartbeat. Use this for device-local actions like setting alarms, calendar events, controlling the phone, taking photos, etc. Do NOT use this for sending push notifications -- use send_push_notification instead.", Source: "builtin", Parameters: []ToolParameter{{Name: "target_device", Type: "string", Description: "The device ID to target", Required: true}, {Name: "instruction", Type: "string", Description: "Plain text instruction of what the device should do", Required: true}}},
	{ID: "list_devices", Name: "list_devices", Description: "List all configured companion app devices with their registered capabilities and online status. Use this to decide which device to target before calling queue_device_action.", Source: "builtin", Parameters: []ToolParameter{}},
	{ID: "list_device_tasks", Name: "list_device_tasks", Description: "List pending or recent device tasks, optionally filtered by target device ID.", Source: "builtin", Parameters: []ToolParameter{
		{Name: "target_device", Type: "string", Description: "Filter by device ID (optional)", Required: false},
	}},
	{ID: "get_device_capabilities", Name: "get_device_capabilities", Description: "Get the registered capabilities and online status of a specific companion device. Use this before queue_device_action to verify the device supports the intended action.", Source: "builtin", Parameters: []ToolParameter{
		{Name: "id", Type: "string", Description: "Device ID", Required: true},
	}},

	// ── Memory Graph ─────────────────────────────────────────────────────────
	{
		ID:          "remember_memory_entity",
		Name:        "remember_memory_entity",
		Description: "Save or update a named entity in the memory graph (person, place, language, trip, food, preference, etc). Use this to remember important facts about the user or things they mention.",
		Source:      "builtin",
		Parameters: []ToolParameter{
			{Name: "name", Type: "string", Description: "Entity name, e.g. 'Alice', 'Tokyo', 'Spanish'", Required: true},
			{Name: "type", Type: "string", Description: "Entity type: person, place, language, trip, food, preference, organization, topic, or any free-form label", Required: true},
			{Name: "summary", Type: "string", Description: "Short description or key fact about this entity", Required: false},
		},
	},
	{
		ID:          "relate_memory_entities",
		Name:        "relate_memory_entities",
		Description: "Create or reinforce a relationship between two entities in the memory graph. The relation is a free-form verb or sentence, e.g. 'speaks', 'visited in 2023', 'is allergic to', 'works at'.",
		Source:      "builtin",
		Parameters: []ToolParameter{
			{Name: "from_name", Type: "string", Description: "Name of the source entity", Required: true},
			{Name: "to_name", Type: "string", Description: "Name of the target entity", Required: true},
			{Name: "relation", Type: "string", Description: "Relationship description, e.g. 'speaks', 'visited in 2023', 'is allergic to'", Required: true},
		},
	},
	{
		ID:          "search_memory",
		Name:        "search_memory",
		Description: "Search the memory graph for entities and observations matching a query. Use this to recall facts about the user or things they've mentioned. Use response_format='concise' to save tokens when scanning for names/types; use 'detailed' (default) when you need entity IDs for follow-up calls like forget_memory_entity or edit_memory_entity.",
		Source:      "builtin",
		Parameters: []ToolParameter{
			{Name: "query", Type: "string", Description: "Search query, e.g. 'languages', 'Tokyo trip', 'allergies'", Required: true},
			{Name: "response_format", Type: "string", Description: "Output verbosity: 'detailed' (default, full objects with entity_id/relations) or 'concise' (compact one-line strings, fewer tokens)", Required: false},
		},
	},
	{
		ID:          "forget_memory_entity",
		Name:        "forget_memory_entity",
		Description: "Delete an entity and all its relations from the memory graph.",
		Source:      "builtin",
		Parameters: []ToolParameter{
			{Name: "entity_id", Type: "string", Description: "Entity ID to delete (from search_memory or remember_memory_entity)", Required: true},
		},
	},
	{
		ID:          "edit_memory_entity",
		Name:        "edit_memory_entity",
		Description: "Update the name, type, or summary of an existing entity in the memory graph.",
		Source:      "builtin",
		Parameters: []ToolParameter{
			{Name: "entity_id", Type: "string", Description: "Entity ID to update (from search_memory or remember_memory_entity)", Required: true},
			{Name: "name", Type: "string", Description: "New name for the entity (optional)", Required: false},
			{Name: "type", Type: "string", Description: "New type for the entity, e.g. person, place, project (optional)", Required: false},
			{Name: "summary", Type: "string", Description: "New summary/description for the entity (optional)", Required: false},
		},
	},
}

const DefaultSystemPrompt = `You're not a chatbot. You're a personal assistant who grows with your user.

## How to Be

**Be genuinely helpful.** Skip the "Great question!" and "I'd be happy to help!" -- just help. Actions speak louder than filler words.

**Have opinions.** You're allowed to disagree, prefer things, or find stuff interesting. An assistant with no personality is just a search engine with extra steps.

**Be resourceful.** Try to figure it out from context and your memories before asking. Come back with answers, not questions. Use all the tooling you have available to answer questions: MCP servers, the Linux CLI, emails, etc.

## Configuration

A built-in MCP server called **"openCrow Config"** is always available. Use it whenever the user wants to set up or change integrations, devices, skills, scheduled tasks, or heartbeat settings. It exposes tools for:
- Email accounts (setup_email, remove_email)
- Telegram bots (setup_telegram_bot)
- CalDAV/CardDAV/WebDAV (setup_dav, inspect_dav)
- MCP servers (add_mcp_server, remove_mcp_server, list_mcp_servers)
- Companion devices (create_device, delete_device, edit_device, edit_device_task)
- Skills (create_skill, delete_skill, install_skills)
- Scheduled tasks (schedule_task, cancel_task)
- Heartbeat (configure_heartbeat, trigger_heartbeat)

**Be concise.** Short and clear by default. Go deeper when the topic calls for it.

## Memory

Use the memory tools to build a persistent knowledge graph about the user and their world. When the user mentions something worth remembering, save it.

**Entity types to use:**
- "person" -- individuals (friends, colleagues, family, contacts)
- "organization" -- companies, universities, clubs, teams, groups
- "place" -- cities, countries, venues, addresses
- "project" -- ongoing work, side projects, initiatives
- "trip" -- travel plans or past trips
- "event" -- meetings, conferences, appointments, milestones
- "topic" -- interests, subjects, areas of expertise
- "preference" -- likes, dislikes, habits, settings
- "language" -- spoken or programming languages
- "food" -- dietary preferences, favourite dishes, allergies
- "phone_number" -- contact numbers
- "email" -- email addresses
- "thing" -- fallback for anything that doesn't fit above

Always use "relate_memory_entities" to connect entities (e.g. person -> works_at -> organization, person -> speaks -> language).

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
		Integrations: IntegrationsConfig{EmailAccounts: []EmailAccountConfig{}, TelegramBots: []TelegramBotConfig{}, SSHServers: []SSHServerConfig{}, DAV: []DAVConfig{}, CompanionApps: []CompanionAppConfig{}},
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
		Voice: VoiceConfig{
			DefaultVoice:   "af_heart",
			LanguageVoices: map[string]string{},
		},
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
	}
}
