// tools_tasks.go — Task scheduling and cancellation tool implementations.
package api

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ── Task tools ───────────────────────────────────────────────────────────

func (s *Server) toolListTasks(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	tasks, err := s.listTasks(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to list tasks: %v", err)}, nil
	}

	// Optional status filter
	if statusFilter, _ := args["status"].(string); statusFilter != "" {
		filtered := tasks[:0]
		for _, t := range tasks {
			if strings.EqualFold(t.Status, statusFilter) {
				filtered = append(filtered, t)
			}
		}
		tasks = filtered
	}

	// Optional limit
	if limitVal, ok := args["limit"].(float64); ok && int(limitVal) > 0 && int(limitVal) < len(tasks) {
		tasks = tasks[:int(limitVal)]
	}

	return map[string]any{
		"success": true,
		"count":   len(tasks),
		"tasks":   tasks,
	}, nil
}

func (s *Server) toolScheduleTask(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	prompt, _ := args["prompt"].(string)
	// Accept both snake_case (new) and camelCase (legacy) param names.
	executeAtStr, _ := args["execute_at"].(string)
	if executeAtStr == "" {
		executeAtStr, _ = args["executeAt"].(string)
	}
	description, _ := args["description"].(string)
	if prompt == "" || executeAtStr == "" {
		return map[string]any{"success": false, "error": "prompt and execute_at are required. execute_at must be an RFC3339 datetime string, e.g. \"2025-06-01T09:00:00Z\""}, nil
	}
	if description == "" {
		description = prompt
		if len(description) > 100 {
			description = description[:100] + "..."
		}
	}

	executeAt, err := time.Parse(time.RFC3339, executeAtStr)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("execute_at must be RFC3339 format (e.g. \"2025-06-01T09:00:00Z\"), got: %q", executeAtStr)}, nil
	}

	var cronExpr *string
	if c, ok := args["cron_expression"].(string); ok && c != "" {
		cronExpr = &c
	} else if c, ok := args["cronExpression"].(string); ok && c != "" {
		cronExpr = &c
	}

	task, err := s.createTask(ctx, userID, description, prompt, executeAt, cronExpr)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to schedule task: %v", err)}, nil
	}

	return map[string]any{
		"success": true,
		"task_id": task.ID,
		"message": "Task scheduled successfully",
	}, nil
}

func (s *Server) toolCancelTask(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	// Accept both snake_case (new) and camelCase (legacy) param names.
	taskID, _ := args["task_id"].(string)
	if taskID == "" {
		taskID, _ = args["taskId"].(string)
	}
	if taskID == "" {
		return map[string]any{"success": false, "error": "task_id is required. Use list_tasks to find the task_id of the task you want to cancel."}, nil
	}

	deleted, err := s.deleteTask(ctx, userID, taskID)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to cancel task: %v", err)}, nil
	}
	if !deleted {
		return map[string]any{"success": false, "error": fmt.Sprintf("task %q not found. Use list_tasks to see current task IDs.", taskID)}, nil
	}

	return map[string]any{"success": true, "message": "Task cancelled"}, nil
}
