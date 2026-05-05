// Package api — tools.go contains the tool dispatch registry and executor factory.
// Individual tool implementations live in tools_{domain}.go files.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// buildToolExecutor returns a function that executes a named tool with args.
func (s *Server) buildToolExecutor(parentCtx context.Context, userID string) func(context.Context, string, map[string]any) (string, error) {
	return func(ctx context.Context, name string, args map[string]any) (string, error) {
		execCtx := ctx
		if execCtx == nil {
			execCtx = parentCtx
		}
		if clientTimezoneFromContext(execCtx) == "" {
			if parentTZ := clientTimezoneFromContext(parentCtx); parentTZ != "" {
				execCtx = context.WithValue(execCtx, clientTimezoneContextKey, parentTZ)
			}
		}
		if execCtx.Value(conversationIDContextKey) == nil {
			if parentConvID, _ := parentCtx.Value(conversationIDContextKey).(string); parentConvID != "" {
				execCtx = context.WithValue(execCtx, conversationIDContextKey, parentConvID)
			}
		}
		result, err := s.executeTool(execCtx, userID, name, args)
		execErr := err
		if execErr == nil {
			execErr = toolResultError(result)
		}
		if name != "execute_shell_command" {
			kind := "TOOL"
			if !isBuiltinToolName(name) {
				kind = "MCP"
			}
			if m, ok := result.(map[string]any); ok {
				if _, has := m["mcpServer"]; has {
					kind = "MCP"
				}
			}
			s.writeToolCallToTerminal(userID, kind, name, args, result, execErr)
		}
		if execErr != nil {
			return "", execErr
		}
		out, _ := json.Marshal(result)
		return string(out), nil
	}
}

func toolResultError(result any) error {
	m, ok := result.(map[string]any)
	if !ok {
		return nil
	}
	success, ok := m["success"].(bool)
	if !ok || success {
		return nil
	}
	if errText := strings.TrimSpace(fmt.Sprint(m["error"])); errText != "" && errText != "<nil>" {
		return fmt.Errorf("%s", errText)
	}
	return fmt.Errorf("tool reported success=false")
}

func isBuiltinToolName(name string) bool {
	switch name {
	case "get_local_time", "get_location",
		"web_search", "open_url",
		"remember_memory_entity", "relate_memory_entities", "search_memory", "forget_memory_entity", "edit_memory_entity",
		"list_tasks",
		"queue_device_action",
		"list_devices", "list_device_tasks", "get_device_capabilities",
		"check_email", "read_email", "compose_email", "search_email",
		"send_notification", "send_channel_notification", "send_push_notification",
		"setup_dav", "list_dav_integrations", "test_dav_connection", "list_webdav_files",
		"list_caldav_calendars", "list_carddav_address_books",
		"create_caldav_event", "delete_caldav_event",
		"create_carddav_contact", "delete_carddav_contact",
		"list_caldav_events", "get_caldav_event", "search_caldav_events",
		"list_carddav_contacts", "get_carddav_contact", "search_carddav_contacts",
		"execute_shell_command", "manage_process",
		"ssh_execute",
		"transcribe_audio",
		"list_skills", "get_skill",
		"discover_mcp_tools", "use_mcp_tool":
		return true
	default:
		return false
	}
}

// executeTool dispatches a tool call by name.
func (s *Server) executeTool(ctx context.Context, userID, name string, args map[string]any) (any, error) {
	switch name {

	// ── Time & Location ──────────────────────────────────────────────
	case "get_local_time":
		return s.toolGetLocalTime(ctx, userID, args), nil

	case "get_location":
		return s.toolGetLocation(ctx)

	// ── Web ──────────────────────────────────────────────────────────
	case "web_search":
		return s.toolWebSearch(ctx, args)

	case "open_url":
		return s.toolOpenURL(ctx, args)

	// ── Memory Graph ─────────────────────────────────────────────────
	case "remember_memory_entity":
		return s.toolRememberEntity(ctx, userID, args)

	case "relate_memory_entities":
		return s.toolRelateEntities(ctx, userID, args)

	case "search_memory":
		return s.toolSearchMemory(ctx, userID, args)

	case "forget_memory_entity":
		return s.toolForgetEntity(ctx, userID, args)

	case "edit_memory_entity":
		return s.toolEditEntity(ctx, userID, args)

	// ── Tasks / Scheduling ───────────────────────────────────────────
	case "list_tasks":
		return s.toolListTasks(ctx, userID, args)

	// ── Heartbeat ────────────────────────────────────────────────────
	case "trigger_heartbeat":
		return s.toolTriggerHeartbeat(ctx, userID)

	case "queue_device_action":
		return s.toolQueueDeviceAction(ctx, userID, args)

	case "list_devices":
		return s.toolListDevices(ctx, userID)

	case "list_device_tasks":
		return s.toolListDeviceTasks(ctx, userID, args)

	case "get_device_capabilities":
		return s.toolGetDeviceCapabilities(ctx, userID, args)

	// ── Email ────────────────────────────────────────────────────────
	case "check_email":
		return s.toolCheckEmail(ctx, userID, args)

	case "read_email":
		return s.toolReadEmail(ctx, userID, args)

	case "compose_email":
		return s.toolComposeEmail(ctx, userID, args)

	case "search_email":
		return s.toolSearchEmail(ctx, userID, args)

	// ── Notification ─────────────────────────────────────────────────
	case "send_notification", "send_channel_notification":
		return s.toolSendNotification(ctx, userID, args)

	case "send_push_notification":
		return s.toolSendPushNotification(ctx, userID, args)

	// ── Channels ─────────────────────────────────────────────────────

	// ── DAV ──────────────────────────────────────────────────────────
	case "setup_dav":
		return s.toolSetupDAV(ctx, userID, args)

	case "list_dav_integrations":
		return s.toolListDAVIntegrations(ctx, userID, args)

	case "test_dav_connection":
		return s.toolTestDAVConnection(ctx, userID, args)

	case "list_webdav_files":
		return s.toolListWebDAVFiles(ctx, userID, args)

	case "list_caldav_calendars":
		return s.toolListCalDAVCalendars(ctx, userID, args)

	case "list_carddav_address_books":
		return s.toolListCardDAVAddressBooks(ctx, userID, args)

	case "create_caldav_event":
		return s.toolCreateCalDAVEvent(ctx, userID, args)

	case "delete_caldav_event":
		return s.toolDeleteCalDAVEvent(ctx, userID, args)

	case "create_carddav_contact":
		return s.toolCreateCardDAVContact(ctx, userID, args)

	case "delete_carddav_contact":
		return s.toolDeleteCardDAVContact(ctx, userID, args)

	case "list_caldav_events":
		return s.toolListCalDAVEvents(ctx, userID, args)

	case "get_caldav_event":
		return s.toolGetCalDAVEvent(ctx, userID, args)

	case "search_caldav_events":
		return s.toolSearchCalDAVEvents(ctx, userID, args)

	case "list_carddav_contacts":
		return s.toolListCardDAVContacts(ctx, userID, args)

	case "get_carddav_contact":
		return s.toolGetCardDAVContact(ctx, userID, args)

	case "search_carddav_contacts":
		return s.toolSearchCardDAVContacts(ctx, userID, args)

	// ── Shell ────────────────────────────────────────────────────────
	case "execute_shell_command":
		return s.toolExecuteShellCommand(ctx, userID, args)

	case "manage_process":
		return s.toolManageProcess(args)

	// ── Remote SSH ───────────────────────────────────────────────────
	case "ssh_execute":
		return s.toolRemoteExecute(ctx, userID, args)

	// ── Voice / Whisper ──────────────────────────────────────────────
	case "transcribe_audio":
		return s.toolTranscribeAudio(ctx, args)

	// ── Skills ───────────────────────────────────────────────────────
	case "list_skills":
		return s.toolListSkills()

	case "get_skill":
		return s.toolGetSkill(args)

	// ── MCP ───────────────────────────────────────────────────────────
	case "discover_mcp_tools":
		return s.toolDiscoverMCPTools(ctx, userID, args)

	case "use_mcp_tool":
		return s.toolUseMCPTool(ctx, userID, args)

	default:
		// Check if this is a built-in config MCP tool (setup_email, create_device, etc.)
		if _, isConfigTool := mcpConfigToolIndex[name]; isConfigTool {
			return s.handleConfigMCPCall(ctx, userID, name, args)
		}
		if mcpResult, ok := s.toolCallMCPTool(ctx, userID, name, args); ok {
			return mcpResult, nil
		}
		return map[string]any{"success": false, "error": fmt.Sprintf("unknown tool: %s", name)}, nil
	}
}
