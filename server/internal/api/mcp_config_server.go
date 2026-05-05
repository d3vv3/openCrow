// mcp_config_server.go -- built-in MCP server that exposes all configuration/setup
// tools over the standard MCP JSON-RPC protocol.
//
// The server is mounted at POST /v1/mcp/config and is always enabled.
// It handles the MCP initialize / notifications/initialized / tools/list / tools/call flow.
// The LLM sees this as a single MCP server entry; individual config tools are never
// injected into the flat tool list.
package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// configMCPToolDefs is the canonical list of configuration tools exposed via the
// built-in MCP server.  Keep in sync with the dispatch table in handleConfigMCPCall.
var configMCPToolDefs = []map[string]any{
	{
		"name":        "setup_email",
		"description": "Configure an email account integration. Auto-detects server settings for Gmail, Outlook, Yahoo, iCloud, etc.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"address":              map[string]any{"type": "string", "description": "Email address"},
				"password":             map[string]any{"type": "string", "description": "Password or app-specific password"},
				"imap_host":            map[string]any{"type": "string", "description": "IMAP server hostname (auto-detected if omitted)"},
				"imap_port":            map[string]any{"type": "integer", "description": "IMAP port (default 993)"},
				"smtp_host":            map[string]any{"type": "string", "description": "SMTP server hostname (auto-detected if omitted)"},
				"smtp_port":            map[string]any{"type": "integer", "description": "SMTP port (default 587)"},
				"poll_interval_seconds": map[string]any{"type": "integer", "description": "How often to poll the inbox in seconds (default 900)"},
			},
			"required": []string{"address", "password"},
		},
	},
	{
		"name":        "remove_email",
		"description": "Remove a configured email account by address.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"address": map[string]any{"type": "string", "description": "Email address to remove"}},
			"required":   []string{"address"},
		},
	},
	{
		"name":        "setup_telegram_bot",
		"description": "Add or update a Telegram bot integration. Stores the bot in config so it can receive messages and send notifications. Get a bot token from @BotFather on Telegram.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"bot_token":             map[string]any{"type": "string", "description": "Telegram bot token from @BotFather"},
				"label":                 map[string]any{"type": "string", "description": "Friendly label for the bot"},
				"notification_chat_id":  map[string]any{"type": "string", "description": "Chat ID to send proactive notifications to (optional)"},
				"allowed_chat_ids":      map[string]any{"type": "string", "description": "Comma-separated list of chat IDs allowed to message this bot (leave empty to allow all)"},
				"poll_interval_seconds": map[string]any{"type": "integer", "description": "How often to poll for messages (default 5)"},
			},
			"required": []string{"bot_token"},
		},
	},
	{
		"name":        "setup_dav",
		"description": "Configure or update a WebDAV/CalDAV/CardDAV integration.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"dav_id":                map[string]any{"type": "string", "description": "Existing DAV integration ID to update (omit to create new)"},
				"name":                  map[string]any{"type": "string", "description": "Friendly integration name"},
				"url":                   map[string]any{"type": "string", "description": "Base DAV URL"},
				"username":              map[string]any{"type": "string", "description": "DAV username"},
				"password":              map[string]any{"type": "string", "description": "DAV password"},
				"enabled":               map[string]any{"type": "boolean", "description": "Enable the integration (default true)"},
				"webdav_enabled":        map[string]any{"type": "boolean", "description": "Enable generic WebDAV support"},
				"caldav_enabled":        map[string]any{"type": "boolean", "description": "Enable CalDAV support"},
				"carddav_enabled":       map[string]any{"type": "boolean", "description": "Enable CardDAV support"},
				"poll_interval_seconds": map[string]any{"type": "integer", "description": "Polling interval in seconds (default 900)"},
			},
			"required": []string{"url"},
		},
	},
	{
		"name":        "inspect_dav",
		"description": "List all configured DAV integrations and optionally test one connection.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"dav_id": map[string]any{"type": "string", "description": "DAV integration ID to test (omit to only list integrations)"}},
		},
	},
	{
		"name":        "add_mcp_server",
		"description": "Add (or update by name/URL) an MCP server configuration.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":    map[string]any{"type": "string", "description": "Display name for the MCP server"},
				"url":     map[string]any{"type": "string", "description": "Base URL for the MCP server"},
				"enabled": map[string]any{"type": "boolean", "description": "Whether the MCP server is enabled (default true)"},
				"headers": map[string]any{"type": "object", "description": "Optional HTTP headers map"},
			},
			"required": []string{"name", "url"},
		},
	},
	{
		"name":        "remove_mcp_server",
		"description": "Remove an MCP server configuration by id, name, or url.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":   map[string]any{"type": "string", "description": "MCP server id"},
				"name": map[string]any{"type": "string", "description": "MCP server name"},
				"url":  map[string]any{"type": "string", "description": "MCP server URL"},
			},
		},
	},
	{
		"name":        "list_mcp_servers",
		"description": "List configured MCP servers.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
	{
		"name":        "create_device",
		"description": "Create a new companion app device. Returns the device ID. The user can then go to the Devices tab to generate a pairing QR code.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"name":  map[string]any{"type": "string", "description": "Short identifier for the device (e.g. 'pixel8', 'my_phone')"},
				"label": map[string]any{"type": "string", "description": "Human-readable display name (e.g. 'Pixel 8 Pro', 'My Phone')"},
			},
			"required": []string{"name", "label"},
		},
	},
	{
		"name":        "delete_device",
		"description": "Delete a companion app device by id or name.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":   map[string]any{"type": "string", "description": "Device ID"},
				"name": map[string]any{"type": "string", "description": "Device name identifier"},
			},
		},
	},
	{
		"name":        "edit_device",
		"description": "Edit a companion app device's name, label, or enabled state.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"id":      map[string]any{"type": "string", "description": "Device ID"},
				"name":    map[string]any{"type": "string", "description": "New name identifier"},
				"label":   map[string]any{"type": "string", "description": "New display label"},
				"enabled": map[string]any{"type": "boolean", "description": "Enable or disable the device"},
			},
			"required": []string{"id"},
		},
	},
	{
		"name":        "edit_device_task",
		"description": "Edit a queued device task. Can update the instruction, tool_name, or tool_arguments. Set reset_status=true to re-queue a failed or completed task.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"task_id":        map[string]any{"type": "string", "description": "ID of the task to edit (from list_device_tasks)"},
				"instruction":    map[string]any{"type": "string", "description": "New instruction text (optional)"},
				"tool_name":      map[string]any{"type": "string", "description": "New tool name override (optional)"},
				"tool_arguments": map[string]any{"type": "object", "description": "New tool arguments map (optional)"},
				"reset_status":   map[string]any{"type": "boolean", "description": "Set to true to reset a failed/completed task back to pending"},
			},
			"required": []string{"task_id"},
		},
	},
	{
		"name":        "create_skill",
		"description": "Create or overwrite a skill file in the skills folder.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"slug":        map[string]any{"type": "string", "description": "Unique identifier / filename slug (e.g. 'my-skill', lowercase, hyphens only)"},
				"description": map[string]any{"type": "string", "description": "Short one-line description of what the skill does"},
				"content":     map[string]any{"type": "string", "description": "Full SKILL.md content in Markdown"},
			},
			"required": []string{"slug", "description", "content"},
		},
	},
	{
		"name":        "delete_skill",
		"description": "Delete an installed skill file from the skills folder by its slug.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"slug": map[string]any{"type": "string", "description": "Skill slug to delete"}},
			"required":   []string{"slug"},
		},
	},
	{
		"name":        "install_skills",
		"description": "Install agent skills from a GitHub repository. Source format: 'owner/repo' or a full GitHub URL.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"source": map[string]any{"type": "string", "description": "GitHub source, e.g. 'vercel-labs/agent-skills'"}},
			"required":   []string{"source"},
		},
	},
	{
		"name":        "schedule_task",
		"description": "Schedule a one-time or recurring automated task. The task runs a prompt through the AI at the specified time.",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"prompt":          map[string]any{"type": "string", "description": "The prompt the AI will run at the scheduled time"},
				"execute_at":      map[string]any{"type": "string", "description": "RFC3339 datetime for first execution"},
				"cron_expression": map[string]any{"type": "string", "description": "Cron expression for recurring tasks"},
				"description":     map[string]any{"type": "string", "description": "Human-readable label shown in the task list"},
			},
			"required": []string{"prompt", "execute_at"},
		},
	},
	{
		"name":        "cancel_task",
		"description": "Cancel and remove a scheduled task by its ID. Use list_tasks to find the task_id first.",
		"inputSchema": map[string]any{
			"type":       "object",
			"properties": map[string]any{"task_id": map[string]any{"type": "string", "description": "Task ID to cancel (from list_tasks)"}},
			"required":   []string{"task_id"},
		},
	},
	{
		"name":        "configure_heartbeat",
		"description": "Configure the heartbeat settings (interval, active hours, enabled state, model).",
		"inputSchema": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"enabled":          map[string]any{"type": "boolean", "description": "Enable or disable the heartbeat"},
				"interval_seconds": map[string]any{"type": "integer", "description": "Heartbeat interval in seconds"},
				"start_hour":       map[string]any{"type": "string", "description": "Active hours start time, e.g. '08:00'"},
				"end_hour":         map[string]any{"type": "string", "description": "Active hours end time, e.g. '22:00'"},
				"timezone":         map[string]any{"type": "string", "description": "IANA timezone for active hours"},
				"model":            map[string]any{"type": "string", "description": "Model to use for heartbeat (leave empty for default)"},
				"prompt":           map[string]any{"type": "string", "description": "Custom heartbeat prompt (leave empty for default)"},
			},
		},
	},
	{
		"name":        "trigger_heartbeat",
		"description": "Trigger an immediate heartbeat check right now.",
		"inputSchema": map[string]any{"type": "object", "properties": map[string]any{}},
	},
}

// mcpConfigToolIndex is a fast name->index lookup built once at init.
var mcpConfigToolIndex = func() map[string]int {
	m := make(map[string]int, len(configMCPToolDefs))
	for i, t := range configMCPToolDefs {
		m[t["name"].(string)] = i
	}
	return m
}()

// handleConfigMCPServer is the HTTP handler for the built-in configuration MCP server.
// It speaks the MCP JSON-RPC 2.0 protocol (HTTP+JSON transport).
func (s *Server) handleConfigMCPServer(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())

	var req struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      any             `json:"id"`
		Method  string          `json:"method"`
		Params  json.RawMessage `json:"params"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeMCPError(w, nil, -32700, "parse error")
		return
	}

	w.Header().Set("Content-Type", "application/json")

	switch req.Method {
	case "initialize":
		writeJSON(w, http.StatusOK, map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]any{
				"protocolVersion": "2024-11-05",
				"capabilities":    map[string]any{"tools": map[string]any{}},
				"serverInfo":      map[string]any{"name": "openCrow Config", "version": "1.0.0"},
			},
		})

	case "notifications/initialized":
		// No response needed for notifications.
		w.WriteHeader(http.StatusNoContent)

	case "tools/list":
		writeJSON(w, http.StatusOK, map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result":  map[string]any{"tools": configMCPToolDefs},
		})

	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			writeMCPError(w, req.ID, -32602, "invalid params")
			return
		}
		result, callErr := s.handleConfigMCPCall(r.Context(), userID, params.Name, params.Arguments)
		if callErr != nil {
			writeJSON(w, http.StatusOK, map[string]any{
				"jsonrpc": "2.0",
				"id":      req.ID,
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": callErr.Error()}},
					"isError": true,
				},
			})
			return
		}
		out, _ := json.Marshal(result)
		writeJSON(w, http.StatusOK, map[string]any{
			"jsonrpc": "2.0",
			"id":      req.ID,
			"result": map[string]any{
				"content": []map[string]any{{"type": "text", "text": string(out)}},
				"isError": false,
			},
		})

	default:
		writeMCPError(w, req.ID, -32601, "method not found: "+req.Method)
	}
}

// handleConfigMCPCall dispatches a config MCP tool call to the real implementation.
func (s *Server) handleConfigMCPCall(ctx context.Context, userID, name string, args map[string]any) (any, error) {
	switch strings.TrimSpace(name) {
	case "setup_email":
		return s.toolSetupEmail(ctx, userID, args)
	case "remove_email":
		return s.toolRemoveEmail(ctx, userID, args)
	case "setup_telegram_bot":
		return s.toolSetupTelegramBot(ctx, userID, args)
	case "setup_dav":
		return s.toolSetupDAV(ctx, userID, args)
	case "inspect_dav":
		return s.toolInspectDAV(ctx, userID, args)
	case "add_mcp_server":
		return s.toolAddMCPServer(ctx, userID, args)
	case "remove_mcp_server":
		return s.toolRemoveMCPServer(ctx, userID, args)
	case "list_mcp_servers":
		return s.toolListMCPServers(ctx, userID)
	case "create_device":
		return s.toolCreateDevice(ctx, userID, args)
	case "delete_device":
		return s.toolDeleteDevice(ctx, userID, args)
	case "edit_device":
		return s.toolEditDevice(ctx, userID, args)
	case "edit_device_task":
		return s.toolEditDeviceTask(ctx, userID, args)
	case "create_skill":
		return s.toolCreateSkill(args)
	case "delete_skill":
		return s.toolDeleteSkill(args)
	case "install_skills":
		return s.toolInstallSkills(args)
	case "schedule_task":
		return s.toolScheduleTask(ctx, userID, args)
	case "cancel_task":
		return s.toolCancelTask(ctx, userID, args)
	case "configure_heartbeat":
		return s.toolConfigureHeartbeat(ctx, userID, args)
	case "trigger_heartbeat":
		return s.toolTriggerHeartbeat(ctx, userID)
	default:
		return nil, &mcpUnknownToolError{name: name}
	}
}

type mcpUnknownToolError struct{ name string }

func (e *mcpUnknownToolError) Error() string { return "unknown config tool: " + e.name }

func writeMCPError(w http.ResponseWriter, id any, code int, msg string) {
	writeJSON(w, http.StatusOK, map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"error":   map[string]any{"code": code, "message": msg},
	})
}
