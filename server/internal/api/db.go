package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/opencrow/opencrow/server/internal/auth"
	"github.com/opencrow/opencrow/server/internal/configstore"
)

func (s *Server) ensureAdminUser(ctx context.Context) (UserDTO, error) {
	if s.adminUsername == "" || s.adminPasswordBcrypt == "" {
		return UserDTO{}, errors.New("single-user admin mode requires ADMIN_USERNAME and ADMIN_PASSWORD_BCRYPT")
	}

	const q = `
INSERT INTO users (username, password_hash)
VALUES ($1, $2)
ON CONFLICT (username)
DO UPDATE SET password_hash = EXCLUDED.password_hash
RETURNING id::text, username;
`
	var user UserDTO
	err := s.db.QueryRow(ctx, q, s.adminUsername, s.adminPasswordBcrypt).Scan(&user.ID, &user.Username)
	return user, err
}

func (s *Server) findUserByID(ctx context.Context, id string) (UserDTO, string, error) {
	const q = `SELECT id::text, username, password_hash FROM users WHERE id = $1::uuid;`
	var user UserDTO
	var passwordHash string
	err := s.db.QueryRow(ctx, q, id).Scan(&user.ID, &user.Username, &passwordHash)
	return user, passwordHash, err
}

func (s *Server) createSessionAndTokens(ctx context.Context, userID, deviceLabel string) (auth.TokenPair, error) {
	const q = `
INSERT INTO device_sessions (user_id, device_label, refresh_token_hash)
VALUES ($1::uuid, $2, '')
RETURNING id::text;
`

	var sessionID string
	if err := s.db.QueryRow(ctx, q, userID, deviceLabel).Scan(&sessionID); err != nil {
		return auth.TokenPair{}, fmt.Errorf("insert device session: %w", err)
	}

	tokens, err := s.authMgr.NewTokenPair(userID, sessionID)
	if err != nil {
		return auth.TokenPair{}, fmt.Errorf("mint token pair: %w", err)
	}

	hash, err := hashRefreshToken(tokens.RefreshToken)
	if err != nil {
		return auth.TokenPair{}, fmt.Errorf("hash refresh token: %w", err)
	}

	if err := s.updateSessionRefreshToken(ctx, sessionID, string(hash)); err != nil {
		return auth.TokenPair{}, fmt.Errorf("update session refresh token: %w", err)
	}

	return tokens, nil
}

func (s *Server) findSession(ctx context.Context, sessionID, userID string) (sessionRow, error) {
	const q = `
SELECT id::text, user_id::text, refresh_token_hash, created_at, last_seen_at, COALESCE(device_label, '')
FROM device_sessions
WHERE id = $1::uuid AND user_id = $2::uuid;
`
	var row sessionRow
	err := s.db.QueryRow(ctx, q, sessionID, userID).Scan(
		&row.ID,
		&row.UserID,
		&row.RefreshTokenHash,
		&row.CreatedAt,
		&row.LastSeenAt,
		&row.DeviceLabel,
	)
	return row, err
}

func (s *Server) updateSessionRefreshToken(ctx context.Context, sessionID, refreshTokenHash string) error {
	const q = `
UPDATE device_sessions
SET refresh_token_hash = $2, last_seen_at = NOW()
WHERE id = $1::uuid;
`
	_, err := s.db.Exec(ctx, q, sessionID, refreshTokenHash)
	return err
}

func (s *Server) listUserSessions(ctx context.Context, userID string) ([]SessionDTO, error) {
	const q = `
SELECT id::text, COALESCE(device_label, ''), created_at, last_seen_at
FROM device_sessions
WHERE user_id = $1::uuid
ORDER BY last_seen_at DESC;
`

	rows, err := s.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SessionDTO
	for rows.Next() {
		var session SessionDTO
		var createdAt time.Time
		var lastSeenAt time.Time
		if err := rows.Scan(&session.ID, &session.DeviceLabel, &createdAt, &lastSeenAt); err != nil {
			return nil, err
		}
		session.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		session.LastSeenAt = lastSeenAt.UTC().Format(time.RFC3339)
		result = append(result, session)
	}
	return result, rows.Err()
}

func (s *Server) deleteUserSession(ctx context.Context, userID, sessionID string) (bool, error) {
	const q = `DELETE FROM device_sessions WHERE id = $1::uuid AND user_id = $2::uuid;`
	cmd, err := s.db.Exec(ctx, q, sessionID, userID)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

func (s *Server) touchSession(ctx context.Context, userID, sessionID string) error {
	const q = `
UPDATE device_sessions
SET last_seen_at = NOW()
WHERE id = $1::uuid AND user_id = $2::uuid
RETURNING id;
`
	var id string
	return s.db.QueryRow(ctx, q, sessionID, userID).Scan(&id)
}

func (s *Server) listConversations(ctx context.Context, userID string) ([]ConversationDTO, error) {
	const q = `
SELECT id::text, COALESCE(title, ''), created_at, updated_at
FROM conversations
WHERE user_id = $1::uuid
ORDER BY updated_at DESC;
`
	rows, err := s.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []ConversationDTO
	for rows.Next() {
		var item ConversationDTO
		var createdAt time.Time
		var updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.Title, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
		applyConversationAutomationMeta(&item)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Server) createConversation(ctx context.Context, userID, title string) (ConversationDTO, error) {
	const q = `
INSERT INTO conversations (user_id, title)
VALUES ($1::uuid, $2)
RETURNING id::text, COALESCE(title, ''), created_at, updated_at;
`
	var item ConversationDTO
	var createdAt, updatedAt time.Time
	err := s.db.QueryRow(ctx, q, userID, title).Scan(&item.ID, &item.Title, &createdAt, &updatedAt)
	if err != nil {
		return ConversationDTO{}, err
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	applyConversationAutomationMeta(&item)
	return item, nil
}

func (s *Server) getConversation(ctx context.Context, userID, conversationID string) (ConversationDTO, error) {
	const q = `
SELECT id::text, COALESCE(title, ''), created_at, updated_at
FROM conversations
WHERE id = $1::uuid AND user_id = $2::uuid;
`
	var item ConversationDTO
	var createdAt time.Time
	var updatedAt time.Time
	err := s.db.QueryRow(ctx, q, conversationID, userID).Scan(&item.ID, &item.Title, &createdAt, &updatedAt)
	if err != nil {
		return ConversationDTO{}, err
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	applyConversationAutomationMeta(&item)
	return item, nil
}

func (s *Server) updateConversation(ctx context.Context, userID, conversationID, title string) (ConversationDTO, error) {
	const q = `
UPDATE conversations
SET title = $3, updated_at = NOW()
WHERE id = $1::uuid AND user_id = $2::uuid
RETURNING id::text, COALESCE(title, ''), created_at, updated_at;
`
	var item ConversationDTO
	var createdAt time.Time
	var updatedAt time.Time
	err := s.db.QueryRow(ctx, q, conversationID, userID, title).Scan(&item.ID, &item.Title, &createdAt, &updatedAt)
	if err != nil {
		return ConversationDTO{}, err
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	applyConversationAutomationMeta(&item)
	return item, nil
}

func applyConversationAutomationMeta(item *ConversationDTO) {
	if item == nil {
		return
	}
	title := strings.TrimSpace(strings.ToLower(item.Title))
	switch {
	case strings.HasPrefix(title, "scheduled task:"):
		item.IsAutomatic = true
		item.AutomationKind = "scheduled_task"
	case strings.HasPrefix(title, "heartbeat:"):
		item.IsAutomatic = true
		item.AutomationKind = "heartbeat"
	}
}

func (s *Server) deleteConversation(ctx context.Context, userID, conversationID string) error {
	const q = `
DELETE FROM conversations WHERE id = $1::uuid AND user_id = $2::uuid;
`
	tag, err := s.db.Exec(ctx, q, conversationID, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Server) listMessages(ctx context.Context, userID, conversationID string) ([]MessageDTO, error) {
	const q = `
SELECT m.id::text, m.conversation_id::text, m.role, m.content, m.created_at
FROM messages m
JOIN conversations c ON c.id = m.conversation_id
WHERE m.conversation_id = $1::uuid AND c.user_id = $2::uuid
ORDER BY m.created_at ASC;
`
	rows, err := s.db.Query(ctx, q, conversationID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []MessageDTO
	for rows.Next() {
		var item MessageDTO
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.ConversationID, &item.Role, &item.Content, &createdAt); err != nil {
			return nil, err
		}
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Server) createMessage(ctx context.Context, userID, conversationID, role, content string) (MessageDTO, error) {
	const ownershipQ = `
SELECT 1 FROM conversations WHERE id = $1::uuid AND user_id = $2::uuid;
`
	var exists int
	if err := s.db.QueryRow(ctx, ownershipQ, conversationID, userID).Scan(&exists); err != nil {
		return MessageDTO{}, err
	}

	const insertQ = `
INSERT INTO messages (conversation_id, role, content)
VALUES ($1::uuid, $2, $3)
RETURNING id::text, conversation_id::text, role, content, created_at;
`

	var item MessageDTO
	var createdAt time.Time
	err := s.db.QueryRow(ctx, insertQ, conversationID, role, content).Scan(&item.ID, &item.ConversationID, &item.Role, &item.Content, &createdAt)
	if err != nil {
		return MessageDTO{}, err
	}

	const updateConversationQ = `UPDATE conversations SET updated_at = NOW() WHERE id = $1::uuid AND user_id = $2::uuid;`
	if _, err := s.db.Exec(ctx, updateConversationQ, conversationID, userID); err != nil {
		return MessageDTO{}, err
	}

	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return item, nil
}

func (s *Server) updateMessageContent(ctx context.Context, userID, conversationID, messageID, content string) error {
	const updateMessageQ = `
	UPDATE messages
	SET content = $4
	WHERE id = $1::uuid
	  AND conversation_id = $2::uuid
	  AND conversation_id IN (SELECT id FROM conversations WHERE id = $2::uuid AND user_id = $3::uuid);
	`
	tag, err := s.db.Exec(ctx, updateMessageQ, messageID, conversationID, userID, content)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}

	const updateConversationQ = `UPDATE conversations SET updated_at = NOW() WHERE id = $1::uuid AND user_id = $2::uuid;`
	_, err = s.db.Exec(ctx, updateConversationQ, conversationID, userID)
	return err
}

func (s *Server) deleteToolCallsSince(ctx context.Context, userID, conversationID string, since time.Time) error {
	const q = `
	DELETE FROM tool_calls
	WHERE conversation_id = $1::uuid
	  AND created_at >= $3
	  AND conversation_id IN (SELECT id FROM conversations WHERE id = $1::uuid AND user_id = $2::uuid);
	`
	_, err := s.db.Exec(ctx, q, conversationID, userID, since)
	return err
}

// saveToolCall persists a tool call record for a conversation owned by userID.
func (s *Server) saveToolCall(ctx context.Context, userID, conversationID, toolName string, arguments map[string]any, output, errStr string, durationMS int64) error {
	// Verify ownership
	var exists int
	if err := s.db.QueryRow(ctx,
		`SELECT 1 FROM conversations WHERE id = $1::uuid AND user_id = $2::uuid`,
		conversationID, userID,
	).Scan(&exists); err != nil {
		return err
	}
	argsJSON, _ := json.Marshal(arguments)
	var errPtr *string
	if errStr != "" {
		errPtr = &errStr
	}
	_, err := s.db.Exec(ctx,
		`INSERT INTO tool_calls (conversation_id, tool_name, arguments, output, error, duration_ms)
		 VALUES ($1::uuid, $2, $3, $4, $5, $6)`,
		conversationID, toolName, argsJSON, output, errPtr, durationMS,
	)
	return err
}

// listToolCalls returns all tool calls for a conversation owned by userID.
func (s *Server) listToolCalls(ctx context.Context, userID, conversationID string) ([]ToolCallRecord, error) {
	const q = `
SELECT tc.id::text, tc.tool_name, tc.arguments, tc.output, tc.error, tc.duration_ms, tc.created_at
FROM tool_calls tc
JOIN conversations c ON c.id = tc.conversation_id
WHERE tc.conversation_id = $1::uuid AND c.user_id = $2::uuid
ORDER BY tc.created_at ASC;
`
	rows, err := s.db.Query(ctx, q, conversationID, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ToolCallRecord
	for rows.Next() {
		var item ToolCallRecord
		var argsJSON []byte
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.ToolName, &argsJSON, &item.Output, &item.Error, &item.DurationMS, &createdAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(argsJSON, &item.Arguments)
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Server) listTasks(ctx context.Context, userID string) ([]TaskDTO, error) {
	const q = `
SELECT id::text, description, prompt, execute_at, cron_expression, status, last_result, consecutive_failures, created_at, updated_at
FROM scheduled_tasks
WHERE user_id = $1::uuid
ORDER BY execute_at ASC;
`
	rows, err := s.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []TaskDTO
	for rows.Next() {
		item, err := scanTask(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

// recoverStuckTasks resets any tasks left in RUNNING state (from a previous crashed/restarted server)
// back to PENDING so they will be retried by the task worker.
func (s *Server) recoverStuckTasks(ctx context.Context) error {
	const q = `
UPDATE scheduled_tasks
SET status = 'PENDING', updated_at = NOW()
WHERE status = 'RUNNING';
`
	cmd, err := s.db.Exec(ctx, q)
	if err != nil {
		return fmt.Errorf("recover stuck tasks: %w", err)
	}
	if cmd.RowsAffected() > 0 {
		log.Printf("[task-worker] recovered %d stuck RUNNING task(s) to PENDING on startup", cmd.RowsAffected())
	}
	return nil
}

func (s *Server) createTask(ctx context.Context, userID, description, prompt string, executeAt time.Time, cronExpression *string) (TaskDTO, error) {
	const q = `
INSERT INTO scheduled_tasks (user_id, description, prompt, execute_at, cron_expression, status)
VALUES ($1::uuid, $2, $3, $4, $5, 'PENDING')
RETURNING id::text, description, prompt, execute_at, cron_expression, status, last_result, consecutive_failures, created_at, updated_at;
`
	row := s.db.QueryRow(ctx, q, userID, description, prompt, executeAt.UTC(), cronExpression)
	return scanTaskRow(row)
}

func (s *Server) deleteTask(ctx context.Context, userID, taskID string) (bool, error) {
	const q = `DELETE FROM scheduled_tasks WHERE id = $1::uuid AND user_id = $2::uuid;`
	cmd, err := s.db.Exec(ctx, q, taskID, userID)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

func (s *Server) getTask(ctx context.Context, userID, taskID string) (TaskDTO, error) {
	const q = `
SELECT id::text, description, prompt, execute_at, cron_expression, status, last_result, consecutive_failures, created_at, updated_at
FROM scheduled_tasks
WHERE id = $1::uuid AND user_id = $2::uuid;
`
	return scanTaskRow(s.db.QueryRow(ctx, q, taskID, userID))
}

func (s *Server) updateTask(ctx context.Context, userID, taskID string, req UpdateTaskRequest) (TaskDTO, error) {
	current, err := s.getTask(ctx, userID, taskID)
	if err != nil {
		return TaskDTO{}, err
	}

	description := current.Description
	if req.Description != nil {
		description = strings.TrimSpace(*req.Description)
	}

	prompt := current.Prompt
	if req.Prompt != nil {
		prompt = strings.TrimSpace(*req.Prompt)
	}

	executeAt := current.ExecuteAt
	if req.ExecuteAt != nil {
		parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(*req.ExecuteAt))
		if err != nil {
			return TaskDTO{}, errors.New("executeAt must be RFC3339")
		}
		executeAt = parsed.UTC().Format(time.RFC3339)
	}

	status := current.Status
	if req.Status != nil {
		status = strings.TrimSpace(strings.ToUpper(*req.Status))
	}

	var cronExpression *string
	if req.CronExpression != nil {
		trimmed := strings.TrimSpace(*req.CronExpression)
		if trimmed != "" {
			cronExpression = &trimmed
		}
	} else {
		cronExpression = current.CronExpression
	}

	execAtTime, err := time.Parse(time.RFC3339, executeAt)
	if err != nil {
		return TaskDTO{}, err
	}

	const q = `
UPDATE scheduled_tasks
SET description = $3,
    prompt = $4,
    execute_at = $5,
    cron_expression = $6,
    status = $7,
    updated_at = NOW()
WHERE id = $1::uuid AND user_id = $2::uuid
RETURNING id::text, description, prompt, execute_at, cron_expression, status, last_result, consecutive_failures, created_at, updated_at;
`
	return scanTaskRow(s.db.QueryRow(ctx, q, taskID, userID, description, prompt, execAtTime.UTC(), cronExpression, status))
}

func (s *Server) listMemories(ctx context.Context, userID string) ([]MemoryDTO, error) {
	const q = `
SELECT id::text, category, content, confidence, created_at, updated_at
FROM user_memories
WHERE user_id = $1::uuid
ORDER BY confidence DESC, updated_at DESC;
`
	rows, err := s.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []MemoryDTO
	for rows.Next() {
		var item MemoryDTO
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&item.ID, &item.Category, &item.Content, &item.Confidence, &createdAt, &updatedAt); err != nil {
			return nil, err
		}
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
		result = append(result, item)
	}
	return result, rows.Err()
}

func (s *Server) createMemory(ctx context.Context, userID, category, content string, confidence int) (MemoryDTO, error) {
	const q = `
INSERT INTO user_memories (user_id, category, content, confidence)
VALUES ($1::uuid, $2, $3, $4)
RETURNING id::text, category, content, confidence, created_at, updated_at;
`
	var item MemoryDTO
	var createdAt, updatedAt time.Time
	err := s.db.QueryRow(ctx, q, userID, category, content, confidence).Scan(
		&item.ID,
		&item.Category,
		&item.Content,
		&item.Confidence,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return MemoryDTO{}, err
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return item, nil
}

func (s *Server) deleteMemory(ctx context.Context, userID, memoryID string) (bool, error) {
	const q = `DELETE FROM user_memories WHERE id = $1::uuid AND user_id = $2::uuid;`
	cmd, err := s.db.Exec(ctx, q, memoryID, userID)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

func (s *Server) reinforceMemory(ctx context.Context, userID, memoryID string) (MemoryDTO, error) {
	const q = `
UPDATE user_memories
SET confidence = confidence + 1, updated_at = NOW()
WHERE id = $1::uuid AND user_id = $2::uuid
RETURNING id::text, category, content, confidence, created_at, updated_at;
`
	var item MemoryDTO
	var createdAt, updatedAt time.Time
	err := s.db.QueryRow(ctx, q, memoryID, userID).Scan(
		&item.ID, &item.Category, &item.Content, &item.Confidence, &createdAt, &updatedAt,
	)
	if err != nil {
		return MemoryDTO{}, err
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return item, nil
}

func (s *Server) promoteMemory(ctx context.Context, userID, memoryID string) (MemoryDTO, error) {
	const q = `
UPDATE user_memories
SET category = 'PROMOTED', updated_at = NOW()
WHERE id = $1::uuid AND user_id = $2::uuid
RETURNING id::text, category, content, confidence, created_at, updated_at;
`
	var item MemoryDTO
	var createdAt, updatedAt time.Time
	err := s.db.QueryRow(ctx, q, memoryID, userID).Scan(
		&item.ID, &item.Category, &item.Content, &item.Confidence, &createdAt, &updatedAt,
	)
	if err != nil {
		return MemoryDTO{}, err
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return item, nil
}

func (s *Server) getSettings(ctx context.Context, userID string) (UserSettingsDTO, error) {
	const q = `
SELECT COALESCE(data, '{}'::jsonb), updated_at
FROM user_settings
WHERE user_id = $1::uuid;
`

	var raw []byte
	var updatedAt time.Time
	err := s.db.QueryRow(ctx, q, userID).Scan(&raw, &updatedAt)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return UserSettingsDTO{}, err
		}
		return UserSettingsDTO{UserID: userID, Settings: map[string]any{}, UpdatedAt: ""}, nil
	}

	settings := map[string]any{}
	if err := json.Unmarshal(raw, &settings); err != nil {
		return UserSettingsDTO{}, err
	}

	return UserSettingsDTO{
		UserID:    userID,
		Settings:  settings,
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *Server) putSettings(ctx context.Context, userID string, settings map[string]any) (UserSettingsDTO, error) {
	raw, err := json.Marshal(settings)
	if err != nil {
		return UserSettingsDTO{}, err
	}

	const q = `
INSERT INTO user_settings (user_id, data, updated_at)
VALUES ($1::uuid, $2::jsonb, NOW())
ON CONFLICT (user_id)
DO UPDATE SET data = EXCLUDED.data, updated_at = NOW()
RETURNING updated_at;
`

	var updatedAt time.Time
	if err := s.db.QueryRow(ctx, q, userID, string(raw)).Scan(&updatedAt); err != nil {
		return UserSettingsDTO{}, err
	}

	return UserSettingsDTO{
		UserID:    userID,
		Settings:  settings,
		UpdatedAt: updatedAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *Server) getMCPServersSetting(ctx context.Context, userID string) ([]configstore.MCPServerConfig, bool, error) {
	settingsDTO, err := s.getSettings(ctx, userID)
	if err != nil {
		return nil, false, err
	}
	if settingsDTO.Settings == nil {
		return []configstore.MCPServerConfig{}, false, nil
	}
	rawMCP, ok := settingsDTO.Settings["mcp"]
	if !ok {
		return []configstore.MCPServerConfig{}, false, nil
	}
	b, err := json.Marshal(rawMCP)
	if err != nil {
		return nil, true, err
	}
	var payload struct {
		Servers []configstore.MCPServerConfig `json:"servers"`
	}
	if err := json.Unmarshal(b, &payload); err != nil {
		return nil, true, err
	}
	if payload.Servers == nil {
		payload.Servers = []configstore.MCPServerConfig{}
	}
	return payload.Servers, true, nil
}

func (s *Server) putMCPServersSetting(ctx context.Context, userID string, servers []configstore.MCPServerConfig) error {
	settingsDTO, err := s.getSettings(ctx, userID)
	if err != nil {
		return err
	}
	settings := settingsDTO.Settings
	if settings == nil {
		settings = map[string]any{}
	}
	if servers == nil {
		servers = []configstore.MCPServerConfig{}
	}
	settings["mcp"] = map[string]any{
		"servers": servers,
	}
	_, err = s.putSettings(ctx, userID, settings)
	return err
}

func (s *Server) getHeartbeatConfig(ctx context.Context, userID string) (HeartbeatConfigDTO, error) {
	const q = `
SELECT enabled, interval_seconds, next_run_at, updated_at
FROM user_heartbeat_configs
WHERE user_id = $1::uuid;
`

	var enabled bool
	var intervalSeconds int
	var nextRunAt *time.Time
	var updatedAt time.Time
	err := s.db.QueryRow(ctx, q, userID).Scan(&enabled, &intervalSeconds, &nextRunAt, &updatedAt)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			return HeartbeatConfigDTO{}, err
		}
		return HeartbeatConfigDTO{
			UserID:          userID,
			Enabled:         false,
			IntervalSeconds: 300,
		}, nil
	}

	dto := HeartbeatConfigDTO{
		UserID:          userID,
		Enabled:         enabled,
		IntervalSeconds: intervalSeconds,
		UpdatedAt:       updatedAt.UTC().Format(time.RFC3339),
	}
	if nextRunAt != nil {
		dto.NextRunAt = nextRunAt.UTC().Format(time.RFC3339)
	}
	return dto, nil
}

func (s *Server) putHeartbeatConfig(ctx context.Context, userID string, req UpdateHeartbeatConfigRequest) (HeartbeatConfigDTO, error) {
	current, err := s.getHeartbeatConfig(ctx, userID)
	if err != nil {
		return HeartbeatConfigDTO{}, err
	}

	enabled := current.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	interval := current.IntervalSeconds
	if req.IntervalSeconds != nil {
		interval = *req.IntervalSeconds
	}
	if interval <= 0 {
		interval = 300
	}

	nextRun := time.Now().UTC().Add(time.Duration(interval) * time.Second)
	const q = `
INSERT INTO user_heartbeat_configs (user_id, enabled, interval_seconds, next_run_at, updated_at)
VALUES ($1::uuid, $2, $3, $4, NOW())
ON CONFLICT (user_id)
DO UPDATE SET enabled = EXCLUDED.enabled,
              interval_seconds = EXCLUDED.interval_seconds,
              next_run_at = CASE
                -- Reset timer when: interval changed, or heartbeat just became enabled
                WHEN user_heartbeat_configs.interval_seconds != EXCLUDED.interval_seconds
                  OR (user_heartbeat_configs.enabled = FALSE AND EXCLUDED.enabled = TRUE)
                THEN EXCLUDED.next_run_at
                -- Otherwise keep the existing scheduled time (don't push it into the future)
                ELSE user_heartbeat_configs.next_run_at
              END,
              updated_at = NOW()
RETURNING updated_at;
`

	var updatedAt time.Time
	if err := s.db.QueryRow(ctx, q, userID, enabled, interval, nextRun).Scan(&updatedAt); err != nil {
		return HeartbeatConfigDTO{}, err
	}

	return HeartbeatConfigDTO{
		UserID:          userID,
		Enabled:         enabled,
		IntervalSeconds: interval,
		NextRunAt:       nextRun.UTC().Format(time.RFC3339),
		UpdatedAt:       updatedAt.UTC().Format(time.RFC3339),
	}, nil
}

func (s *Server) createHeartbeatEvent(ctx context.Context, userID, status, message string) (HeartbeatEventDTO, error) {
	const q = `
INSERT INTO heartbeat_events (user_id, status, message)
VALUES ($1::uuid, $2, $3)
RETURNING id::text, status, COALESCE(message, ''), created_at;
`
	var dto HeartbeatEventDTO
	var createdAt time.Time
	if err := s.db.QueryRow(ctx, q, userID, status, message).Scan(&dto.ID, &dto.Status, &dto.Message, &createdAt); err != nil {
		return HeartbeatEventDTO{}, err
	}
	dto.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return dto, nil
}

func (s *Server) listHeartbeatEvents(ctx context.Context, userID string) ([]HeartbeatEventDTO, error) {
	const q = `
SELECT id::text, status, COALESCE(message, ''), created_at
FROM heartbeat_events
WHERE user_id = $1::uuid
ORDER BY created_at DESC
LIMIT 100;
`
	rows, err := s.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []HeartbeatEventDTO
	for rows.Next() {
		var dto HeartbeatEventDTO
		var createdAt time.Time
		if err := rows.Scan(&dto.ID, &dto.Status, &dto.Message, &createdAt); err != nil {
			return nil, err
		}
		dto.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		result = append(result, dto)
	}
	return result, rows.Err()
}

func (s *Server) listEmailInboxes(ctx context.Context, userID string) ([]EmailInboxDTO, error) {
	const q = `
SELECT id::text, address, imap_host, imap_port, imap_username, use_tls, active, poll_interval_seconds, last_polled_at, updated_at, created_at
FROM email_inboxes
WHERE user_id = $1::uuid
ORDER BY created_at DESC;
`
	rows, err := s.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []EmailInboxDTO
	for rows.Next() {
		var dto EmailInboxDTO
		var lastPolledAt *time.Time
		var updatedAt, createdAt time.Time
		if err := rows.Scan(&dto.ID, &dto.Address, &dto.ImapHost, &dto.ImapPort, &dto.ImapUsername, &dto.UseTLS, &dto.Active, &dto.PollIntervalSeconds, &lastPolledAt, &updatedAt, &createdAt); err != nil {
			return nil, err
		}
		if lastPolledAt != nil {
			dto.LastPolledAt = lastPolledAt.UTC().Format(time.RFC3339)
		}
		dto.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
		dto.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		result = append(result, dto)
	}
	return result, rows.Err()
}

type emailInboxCredentials struct {
	ID           string
	Address      string
	Label        string
	ImapHost     string
	ImapPort     int
	ImapUsername string
	ImapPassword string
	SmtpHost     string
	SmtpPort     int
	UseTLS       bool
}

// getFirstEmailCredentials returns credentials for the first active inbox, for use in email tools.
func (s *Server) getFirstEmailCredentials(ctx context.Context, userID string) (*emailInboxCredentials, error) {
	const q = `
SELECT id::text, address, COALESCE(label,''), imap_host, imap_port, imap_username, imap_password, COALESCE(smtp_host,''), COALESCE(smtp_port,587), use_tls
FROM email_inboxes
WHERE user_id = $1::uuid AND active = true
ORDER BY created_at DESC
LIMIT 1;
`
	var c emailInboxCredentials
	err := s.db.QueryRow(ctx, q, userID).Scan(&c.ID, &c.Address, &c.Label, &c.ImapHost, &c.ImapPort, &c.ImapUsername, &c.ImapPassword, &c.SmtpHost, &c.SmtpPort, &c.UseTLS)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// syncEmailInboxesFromConfig upserts email accounts from the configstore into the email_inboxes DB table.
// This ensures accounts saved via the Config UI are visible to the email worker and LLM tools.
func (s *Server) syncEmailInboxesFromConfig(ctx context.Context, userID string, accounts []configstore.EmailAccountConfig) error {
	const q = `
INSERT INTO email_inboxes (user_id, address, label, imap_host, imap_port, imap_username, imap_password, smtp_host, smtp_port, use_tls, active, poll_interval_seconds, updated_at)
VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, 60, NOW())
ON CONFLICT (user_id, address) DO UPDATE SET
    label         = EXCLUDED.label,
    imap_host     = EXCLUDED.imap_host,
    imap_port     = EXCLUDED.imap_port,
    imap_username = EXCLUDED.imap_username,
    imap_password = EXCLUDED.imap_password,
    smtp_host     = EXCLUDED.smtp_host,
    smtp_port     = EXCLUDED.smtp_port,
    use_tls       = EXCLUDED.use_tls,
    active        = EXCLUDED.active,
    updated_at    = NOW();
`
	for _, acc := range accounts {
		imapPort := acc.ImapPort
		if imapPort <= 0 {
			imapPort = 993
		}
		smtpPort := acc.SmtpPort
		if smtpPort <= 0 {
			smtpPort = 587
		}
		if _, err := s.db.Exec(ctx, q,
			userID, acc.Address, acc.Label, acc.ImapHost, imapPort,
			acc.ImapUsername, acc.ImapPassword,
			acc.SmtpHost, smtpPort, acc.UseTLS, acc.Enabled,
		); err != nil {
			return fmt.Errorf("upsert inbox %s: %w", acc.Address, err)
		}
	}
	return nil
}

func (s *Server) createEmailInbox(ctx context.Context, userID, address, imapHost string, imapPort int, imapUsername, imapPassword string, useTLS bool, pollInterval int) (EmailInboxDTO, error) {
	const q = `
INSERT INTO email_inboxes (user_id, address, imap_host, imap_port, imap_username, imap_password, use_tls, active, poll_interval_seconds, updated_at)
VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, TRUE, $8, NOW())
RETURNING id::text, address, imap_host, imap_port, imap_username, use_tls, active, poll_interval_seconds, last_polled_at, updated_at, created_at;
`

	var dto EmailInboxDTO
	var lastPolledAt *time.Time
	var updatedAt, createdAt time.Time
	err := s.db.QueryRow(ctx, q, userID, address, imapHost, imapPort, imapUsername, imapPassword, useTLS, pollInterval).Scan(
		&dto.ID,
		&dto.Address,
		&dto.ImapHost,
		&dto.ImapPort,
		&dto.ImapUsername,
		&dto.UseTLS,
		&dto.Active,
		&dto.PollIntervalSeconds,
		&lastPolledAt,
		&updatedAt,
		&createdAt,
	)
	if err != nil {
		return EmailInboxDTO{}, err
	}
	if lastPolledAt != nil {
		dto.LastPolledAt = lastPolledAt.UTC().Format(time.RFC3339)
	}
	dto.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	dto.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return dto, nil
}

func (s *Server) updateEmailInbox(ctx context.Context, userID, inboxID string, req UpdateEmailInboxRequest) (EmailInboxDTO, error) {
	const currentQ = `
SELECT id::text, address, imap_host, imap_port, imap_username, imap_password, use_tls, active, poll_interval_seconds, last_polled_at, updated_at, created_at
FROM email_inboxes
WHERE id = $1::uuid AND user_id = $2::uuid;
`

	var dto EmailInboxDTO
	var currentPassword string
	var lastPolledAt *time.Time
	var updatedAt, createdAt time.Time
	err := s.db.QueryRow(ctx, currentQ, inboxID, userID).Scan(
		&dto.ID,
		&dto.Address,
		&dto.ImapHost,
		&dto.ImapPort,
		&dto.ImapUsername,
		&currentPassword,
		&dto.UseTLS,
		&dto.Active,
		&dto.PollIntervalSeconds,
		&lastPolledAt,
		&updatedAt,
		&createdAt,
	)
	if err != nil {
		return EmailInboxDTO{}, err
	}

	imapHost := dto.ImapHost
	if req.ImapHost != nil {
		imapHost = strings.TrimSpace(*req.ImapHost)
	}
	imapPort := dto.ImapPort
	if req.ImapPort != nil {
		imapPort = *req.ImapPort
	}
	imapUsername := dto.ImapUsername
	if req.ImapUsername != nil {
		imapUsername = strings.TrimSpace(*req.ImapUsername)
	}
	imapPassword := currentPassword
	if req.ImapPassword != nil {
		imapPassword = *req.ImapPassword
	}
	useTLS := dto.UseTLS
	if req.UseTLS != nil {
		useTLS = *req.UseTLS
	}
	active := dto.Active
	if req.Active != nil {
		active = *req.Active
	}
	pollInterval := dto.PollIntervalSeconds
	if req.PollIntervalSeconds != nil {
		pollInterval = *req.PollIntervalSeconds
	}

	const updateQ = `
UPDATE email_inboxes
SET imap_host = $3,
    imap_port = $4,
    imap_username = $5,
    imap_password = $6,
    use_tls = $7,
    active = $8,
    poll_interval_seconds = $9,
    updated_at = NOW()
WHERE id = $1::uuid AND user_id = $2::uuid
RETURNING id::text, address, imap_host, imap_port, imap_username, use_tls, active, poll_interval_seconds, last_polled_at, updated_at, created_at;
`

	var newLastPolledAt *time.Time
	err = s.db.QueryRow(ctx, updateQ, inboxID, userID, imapHost, imapPort, imapUsername, imapPassword, useTLS, active, pollInterval).Scan(
		&dto.ID,
		&dto.Address,
		&dto.ImapHost,
		&dto.ImapPort,
		&dto.ImapUsername,
		&dto.UseTLS,
		&dto.Active,
		&dto.PollIntervalSeconds,
		&newLastPolledAt,
		&updatedAt,
		&createdAt,
	)
	if err != nil {
		return EmailInboxDTO{}, err
	}
	if newLastPolledAt != nil {
		dto.LastPolledAt = newLastPolledAt.UTC().Format(time.RFC3339)
	}
	dto.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	dto.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return dto, nil
}

func (s *Server) deleteEmailInbox(ctx context.Context, userID, inboxID string) (bool, error) {
	const q = `DELETE FROM email_inboxes WHERE id = $1::uuid AND user_id = $2::uuid;`
	cmd, err := s.db.Exec(ctx, q, inboxID, userID)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

func (s *Server) createEmailPollEvent(ctx context.Context, userID, inboxID, status, detail string) (EmailPollEventDTO, error) {
	const ownershipQ = `SELECT 1 FROM email_inboxes WHERE id = $1::uuid AND user_id = $2::uuid;`
	var exists int
	if err := s.db.QueryRow(ctx, ownershipQ, inboxID, userID).Scan(&exists); err != nil {
		return EmailPollEventDTO{}, err
	}

	const q = `
WITH inserted AS (
  INSERT INTO email_poll_events (user_id, inbox_id, status, detail)
  VALUES ($1::uuid, $2::uuid, $3, $4)
  RETURNING id::text, inbox_id::text, status, COALESCE(detail, '') AS detail, created_at
), touched AS (
  UPDATE email_inboxes
  SET last_polled_at = NOW(), updated_at = NOW()
  WHERE id = $2::uuid AND user_id = $1::uuid
)
SELECT id, inbox_id, status, detail, created_at FROM inserted;
`

	var dto EmailPollEventDTO
	var createdAt time.Time
	if err := s.db.QueryRow(ctx, q, userID, inboxID, status, detail).Scan(&dto.ID, &dto.InboxID, &dto.Status, &dto.Detail, &createdAt); err != nil {
		return EmailPollEventDTO{}, err
	}
	dto.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return dto, nil
}

func (s *Server) listEmailPollEvents(ctx context.Context, userID string) ([]EmailPollEventDTO, error) {
	const q = `
SELECT id::text, inbox_id::text, status, COALESCE(detail, ''), created_at
FROM email_poll_events
WHERE user_id = $1::uuid
ORDER BY created_at DESC
LIMIT 200;
`
	rows, err := s.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []EmailPollEventDTO
	for rows.Next() {
		var dto EmailPollEventDTO
		var createdAt time.Time
		if err := rows.Scan(&dto.ID, &dto.InboxID, &dto.Status, &dto.Detail, &createdAt); err != nil {
			return nil, err
		}
		dto.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		result = append(result, dto)
	}
	return result, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanTask(row scanner) (TaskDTO, error) {
	var item TaskDTO
	var executeAt, createdAt, updatedAt time.Time
	err := row.Scan(
		&item.ID,
		&item.Description,
		&item.Prompt,
		&executeAt,
		&item.CronExpression,
		&item.Status,
		&item.LastResult,
		&item.ConsecutiveFailures,
		&createdAt,
		&updatedAt,
	)
	if err != nil {
		return TaskDTO{}, err
	}
	item.ExecuteAt = executeAt.UTC().Format(time.RFC3339)
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	item.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	return item, nil
}

func scanTaskRow(row pgx.Row) (TaskDTO, error) {
	return scanTask(row)
}
