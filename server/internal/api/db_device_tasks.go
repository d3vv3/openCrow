package api

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

func (s *Server) createDeviceTask(ctx context.Context, userID, targetDevice, instruction string, toolName *string, toolArguments map[string]any) (DeviceTaskDTO, error) {
	var dto DeviceTaskDTO

	var toolArgsJSON []byte
	if toolArguments != nil {
		b, err := json.Marshal(toolArguments)
		if err != nil {
			return dto, fmt.Errorf("marshal tool arguments: %w", err)
		}
		toolArgsJSON = b
	}

	q := `
INSERT INTO device_tasks (user_id, target_device, instruction, tool_name, tool_arguments, status)
VALUES ($1, $2, $3, $4, $5, 'pending')
RETURNING id, target_device, instruction, tool_name, tool_arguments, status, result_output, created_at, updated_at, expires_at;
`
	var resultOutput *string
	var createdAt, updatedAt time.Time
	var expiresAt *time.Time
	var rawArgs []byte

	err := s.db.QueryRow(ctx, q, userID, targetDevice, instruction, toolName, toolArgsJSON).Scan(
		&dto.ID, &dto.TargetDevice, &dto.Instruction, &dto.ToolName, &rawArgs, &dto.Status,
		&resultOutput, &createdAt, &updatedAt, &expiresAt,
	)
	if err != nil {
		return dto, fmt.Errorf("create device task: %w", err)
	}

	dto.ResultOutput = resultOutput
	dto.CreatedAt = createdAt.Format(time.RFC3339)
	dto.UpdatedAt = updatedAt.Format(time.RFC3339)
	if expiresAt != nil {
		exp := expiresAt.Format(time.RFC3339)
		dto.ExpiresAt = &exp
	}
	if rawArgs != nil {
		var m map[string]any
		if err := json.Unmarshal(rawArgs, &m); err == nil {
			dto.ToolArguments = m
		}
	}

	return dto, nil
}

func (s *Server) listDeviceTasks(ctx context.Context, userID string) ([]DeviceTaskDTO, error) {
	q := `
SELECT id, target_device, instruction, tool_name, tool_arguments, status, result_output, created_at, updated_at, expires_at
FROM device_tasks
WHERE user_id = $1
ORDER BY created_at DESC;
`
	rows, err := s.db.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("query device tasks: %w", err)
	}
	defer rows.Close()

	var tasks []DeviceTaskDTO
	for rows.Next() {
		var dto DeviceTaskDTO
		var resultOutput *string
		var createdAt, updatedAt time.Time
		var expiresAt *time.Time
		var rawArgs []byte

		if err := rows.Scan(
			&dto.ID, &dto.TargetDevice, &dto.Instruction, &dto.ToolName, &rawArgs, &dto.Status,
			&resultOutput, &createdAt, &updatedAt, &expiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan device task: %w", err)
		}

		dto.ResultOutput = resultOutput
		dto.CreatedAt = createdAt.Format(time.RFC3339)
		dto.UpdatedAt = updatedAt.Format(time.RFC3339)
		if expiresAt != nil {
			exp := expiresAt.Format(time.RFC3339)
			dto.ExpiresAt = &exp
		}
		if rawArgs != nil {
			var m map[string]any
			if err := json.Unmarshal(rawArgs, &m); err == nil {
				dto.ToolArguments = m
			}
		}
		tasks = append(tasks, dto)
	}
	return tasks, nil
}

func (s *Server) deleteDeviceTask(ctx context.Context, userID, taskID string) error {
	res, err := s.db.Exec(ctx, "DELETE FROM device_tasks WHERE id = $1 AND user_id = $2;", taskID, userID)
	if err != nil {
		return fmt.Errorf("delete device task: %w", err)
	}
	if res.RowsAffected() == 0 {
		return fmt.Errorf("task not found")
	}
	return nil
}

func (s *Server) deleteDeviceTasksByTarget(ctx context.Context, userID, targetDevice string) error {
	_, err := s.db.Exec(ctx,
		"DELETE FROM device_tasks WHERE user_id = $1 AND target_device = $2;",
		userID, targetDevice,
	)
	if err != nil {
		return fmt.Errorf("delete tasks for device %s: %w", targetDevice, err)
	}
	return nil
}

func (s *Server) pollDeviceTasks(ctx context.Context, userID, targetDevice string) ([]DeviceTaskDTO, error) {
	q := `
UPDATE device_tasks
SET status = 'processing', updated_at = NOW()
WHERE id IN (
    SELECT id FROM device_tasks
    WHERE user_id = $1 AND target_device = $2 AND status = 'pending'
    ORDER BY created_at ASC
    FOR UPDATE SKIP LOCKED
)
RETURNING id, target_device, instruction, tool_name, tool_arguments, status, result_output, created_at, updated_at, expires_at;
`
	rows, err := s.db.Query(ctx, q, userID, targetDevice)
	if err != nil {
		return nil, fmt.Errorf("poll device tasks: %w", err)
	}
	defer rows.Close()

	var tasks []DeviceTaskDTO
	for rows.Next() {
		var dto DeviceTaskDTO
		var resultOutput *string
		var createdAt, updatedAt time.Time
		var expiresAt *time.Time
		var rawArgs []byte

		if err := rows.Scan(
			&dto.ID, &dto.TargetDevice, &dto.Instruction, &dto.ToolName, &rawArgs, &dto.Status,
			&resultOutput, &createdAt, &updatedAt, &expiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan polled device task: %w", err)
		}

		dto.ResultOutput = resultOutput
		dto.CreatedAt = createdAt.Format(time.RFC3339)
		dto.UpdatedAt = updatedAt.Format(time.RFC3339)
		if expiresAt != nil {
			exp := expiresAt.Format(time.RFC3339)
			dto.ExpiresAt = &exp
		}
		if rawArgs != nil {
			var m map[string]any
			if err := json.Unmarshal(rawArgs, &m); err == nil {
				dto.ToolArguments = m
			}
		}
		tasks = append(tasks, dto)
	}
	return tasks, nil
}

func (s *Server) completeDeviceTask(ctx context.Context, userID, taskID string, success bool, output string) error {
	status := "completed"
	if !success {
		status = "failed"
	}

	q := `
UPDATE device_tasks
SET status = $1, result_output = $2, updated_at = NOW()
WHERE id = $3 AND user_id = $4
RETURNING id;
`
	var id string
	err := s.db.QueryRow(ctx, q, status, output, taskID, userID).Scan(&id)
	if err != nil {
		return fmt.Errorf("complete device task: %w", err)
	}
	return nil
}
