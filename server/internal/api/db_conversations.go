// db_conversations.go — Conversation, message, and tool call persistence.
package api

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

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
	case strings.HasPrefix(title, "[telegram] "):
		item.Channel = "telegram"
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
	messageOrder := make([]string, 0)
	for rows.Next() {
		var item MessageDTO
		var createdAt time.Time
		if err := rows.Scan(&item.ID, &item.ConversationID, &item.Role, &item.Content, &createdAt); err != nil {
			return nil, err
		}
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		result = append(result, item)
		messageOrder = append(messageOrder, item.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(messageOrder) == 0 {
		return result, nil
	}

	const attachmentsQ = `
SELECT a.id::text, a.message_id::text, a.file_name, a.mime_type, a.size_bytes, a.data_url, a.created_at
FROM message_attachments a
JOIN messages m ON m.id = a.message_id
JOIN conversations c ON c.id = m.conversation_id
WHERE m.conversation_id = $1::uuid AND c.user_id = $2::uuid
ORDER BY a.created_at ASC;
`
	attachmentRows, err := s.db.Query(ctx, attachmentsQ, conversationID, userID)
	if err != nil {
		return nil, err
	}
	defer attachmentRows.Close()

	attachmentsByMessageID := make(map[string][]MessageAttachmentDTO)
	for attachmentRows.Next() {
		var messageID string
		var attachment MessageAttachmentDTO
		var createdAt time.Time
		if err := attachmentRows.Scan(
			&attachment.ID,
			&messageID,
			&attachment.FileName,
			&attachment.MimeType,
			&attachment.SizeBytes,
			&attachment.DataURL,
			&createdAt,
		); err != nil {
			return nil, err
		}
		attachment.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		attachmentsByMessageID[messageID] = append(attachmentsByMessageID[messageID], attachment)
	}
	if err := attachmentRows.Err(); err != nil {
		return nil, err
	}

	for i := range result {
		result[i].Attachments = attachmentsByMessageID[result[i].ID]
	}

	return result, nil
}

func (s *Server) createMessage(ctx context.Context, userID, conversationID, role, content string, attachments []CreateMessageAttachmentRequest) (MessageDTO, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return MessageDTO{}, err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	const ownershipQ = `
SELECT 1 FROM conversations WHERE id = $1::uuid AND user_id = $2::uuid;
`
	var exists int
	if err := tx.QueryRow(ctx, ownershipQ, conversationID, userID).Scan(&exists); err != nil {
		return MessageDTO{}, err
	}

	const insertQ = `
INSERT INTO messages (conversation_id, role, content)
VALUES ($1::uuid, $2, $3)
RETURNING id::text, conversation_id::text, role, content, created_at;
`

	var item MessageDTO
	var createdAt time.Time
	err = tx.QueryRow(ctx, insertQ, conversationID, role, content).Scan(&item.ID, &item.ConversationID, &item.Role, &item.Content, &createdAt)
	if err != nil {
		return MessageDTO{}, err
	}
	item.CreatedAt = createdAt.UTC().Format(time.RFC3339)

	for _, attachment := range attachments {
		const insertAttachmentQ = `
INSERT INTO message_attachments (message_id, file_name, mime_type, size_bytes, data_url)
VALUES ($1::uuid, $2, $3, $4, $5)
RETURNING id::text, file_name, mime_type, size_bytes, data_url, created_at;
`
		var added MessageAttachmentDTO
		var attachmentCreatedAt time.Time
		if err := tx.QueryRow(
			ctx,
			insertAttachmentQ,
			item.ID,
			attachment.FileName,
			attachment.MimeType,
			attachment.SizeBytes,
			attachment.DataURL,
		).Scan(
			&added.ID,
			&added.FileName,
			&added.MimeType,
			&added.SizeBytes,
			&added.DataURL,
			&attachmentCreatedAt,
		); err != nil {
			return MessageDTO{}, err
		}
		added.CreatedAt = attachmentCreatedAt.UTC().Format(time.RFC3339)
		item.Attachments = append(item.Attachments, added)
	}

	const updateConversationQ = `UPDATE conversations SET updated_at = NOW() WHERE id = $1::uuid AND user_id = $2::uuid;`
	if _, err := tx.Exec(ctx, updateConversationQ, conversationID, userID); err != nil {
		return MessageDTO{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return MessageDTO{}, err
	}

	return item, nil
}

func (s *Server) updateMessageContent(ctx context.Context, userID, conversationID, messageID, content string) error {
	const updateMessageQ = `
	UPDATE messages
	SET content = $4, created_at = NOW()
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
		if isBuiltinToolName(item.ToolName) {
			item.Kind = "TOOL"
		} else {
			item.Kind = "MCP"
		}
		item.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
		result = append(result, item)
	}
	return result, rows.Err()
}
