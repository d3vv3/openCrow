// worker_tasks.go — Background worker for executing scheduled tasks.
package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/opencrow/opencrow/server/internal/configstore"
	"github.com/opencrow/opencrow/server/internal/scheduler"
)

func (s *Server) runTaskWorker(ctx context.Context) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	s.wlog("task-worker", "[task-worker] started")
	// Process immediately on startup to catch any overdue tasks
	if err := s.processDueTasks(ctx); err != nil {
		s.wlog("task-worker", "[task-worker] error on startup scan: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			s.wlog("task-worker", "[task-worker] stopped")
			return
		case <-ticker.C:
			err := s.processDueTasks(ctx)
			s.workerStatus.tick("task-worker", err)
			if err != nil {
				s.wlog("task-worker", "[task-worker] error: %v", err)
			}
		}
	}
}

func (s *Server) processDueTasks(ctx context.Context) error {
	// First: log overall task counts for diagnostics
	const countQ = `
SELECT
    COUNT(*) FILTER (WHERE status = 'PENDING') AS pending,
    COUNT(*) FILTER (WHERE status = 'RUNNING') AS running,
    COUNT(*) FILTER (WHERE status = 'FAILED' AND cron_expression IS NOT NULL) AS failed_cron,
    COUNT(*) FILTER (WHERE (status = 'PENDING' OR (status = 'FAILED' AND cron_expression IS NOT NULL)) AND execute_at <= NOW()) AS due_now
FROM scheduled_tasks;
`
	var pending, running, failedCron, dueNow int
	if err := s.db.QueryRow(ctx, countQ).Scan(&pending, &running, &failedCron, &dueNow); err == nil {
		s.wlog("task-worker", "[task-worker] scheduled tasks: %d pending, %d failed-cron (%d due now), %d running", pending, failedCron, dueNow, running)
	}

	const claimQ = `
WITH to_claim AS (
    SELECT id FROM scheduled_tasks
    WHERE (status = 'PENDING' OR (status = 'FAILED' AND cron_expression IS NOT NULL))
      AND execute_at <= NOW()
    ORDER BY execute_at ASC
    LIMIT 10
    FOR UPDATE SKIP LOCKED
)
UPDATE scheduled_tasks t
SET status = 'RUNNING', consecutive_failures = 0, updated_at = NOW()
FROM to_claim
WHERE t.id = to_claim.id
RETURNING t.id::text, t.user_id::text, t.description, t.prompt, t.cron_expression, t.consecutive_failures;
`
	rows, err := s.db.Query(ctx, claimQ)
	if err != nil {
		return fmt.Errorf("claim tasks: %w", err)
	}
	defer rows.Close()

	type taskRow struct {
		id                  string
		userID              string
		description         string
		prompt              string
		cronExpression      *string
		consecutiveFailures int
	}

	var tasks []taskRow
	for rows.Next() {
		var t taskRow
		if err := rows.Scan(&t.id, &t.userID, &t.description, &t.prompt, &t.cronExpression, &t.consecutiveFailures); err != nil {
			return fmt.Errorf("scan task row: %w", err)
		}
		tasks = append(tasks, t)
	}
	rows.Close()

	if len(tasks) > 0 {
		s.wlog("task-worker", "[task-worker] claimed %d due task(s)", len(tasks))
	}
	for _, t := range tasks {
		s.executeScheduledTask(ctx, t.id, t.userID, t.description, t.prompt, t.cronExpression, t.consecutiveFailures)
	}
	return nil
}

func (s *Server) executeScheduledTask(ctx context.Context, taskID, userID, description, prompt string, cronExpression *string, consecutiveFailures int) {
	taskCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	var cfg *configstore.UserConfig
	if s.configStore != nil {
		if c, err := s.configStore.GetUserConfig(userID); err == nil {
			cfg = &c
		}
	}
	resolvedTZ := preferredTimezoneName(taskCtx, cfg, "")

	executionPrompt := buildScheduledTaskExecutionPromptWithTimezone(taskID, description, prompt, cronExpression, resolvedTZ)

	s.wlog("task-worker", "[task-worker] executing task %s for user %s: %s", taskID, userID, description)
	s.wlog("task-worker", "[task-worker] task %s resolved timezone: %s", taskID, resolvedTZ)
	s.wlog("task-worker", "[task-worker] task %s prompt: %s", taskID, workerLogPreview(prompt, 220))

	result, execErr := s.runOrchestratorForUser(taskCtx, "task-worker", userID, executionPrompt)
	output := result.Output
	s.logWorkerOrchestratorTrace("task-worker", fmt.Sprintf("task %s", taskID), result)
	lastResult := summarizeWorkerExecution(result, execErr, resolvedTZ)

	// Notify via channels on failure
	if execErr != nil {
		notifyTitle := "Task failed: " + description
		if strings.TrimSpace(description) == "" {
			notifyTitle = "Scheduled task failed"
		}
		s.notifyChannels(taskCtx, userID, notifyTitle, execErr.Error())
	}

	// Save execution as a conversation so it's visible in the chat UI
	titleBase := description
	if strings.TrimSpace(titleBase) == "" {
		titleBase = prompt
	}
	title := "Scheduled task: " + titleBase
	if len(title) > 80 {
		title = title[:80] + "..."
	}
	if conv, convErr := s.createConversation(taskCtx, userID, title); convErr == nil {
		resultContent := output
		if execErr != nil {
			resultContent = "Task failed: " + execErr.Error()
		}
		if _, msgErr := s.createMessage(taskCtx, userID, conv.ID, "user", prompt, nil); msgErr != nil {
			s.wlog("task-worker", "[task-worker] failed to insert user message for task %s: %v", taskID, msgErr)
		}
		for _, tc := range result.Trace.ToolCalls {
			outputStr := tc.Output
			errStr := ""
			if tc.Status == "error" {
				errStr = tc.Output
				outputStr = ""
			}
			if saveErr := s.saveToolCall(taskCtx, userID, conv.ID, tc.Name, tc.Arguments, outputStr, errStr, 0); saveErr != nil {
				s.wlog("task-worker", "[task-worker] failed to persist tool call %s for task %s conversation %s: %v", tc.Name, taskID, conv.ID, saveErr)
			}
		}
		if _, msgErr := s.createMessage(taskCtx, userID, conv.ID, "assistant", resultContent, nil); msgErr != nil {
			s.wlog("task-worker", "[task-worker] failed to insert assistant message for task %s: %v", taskID, msgErr)
		}
	} else {
		s.wlog("task-worker", "[task-worker] failed to create conversation for task %s: %v", taskID, convErr)
	}

	const maxFailures = 5
	if execErr != nil {
		consecutiveFailures++
		isCron := cronExpression != nil && *cronExpression != ""

		if consecutiveFailures >= maxFailures {
			if isCron {
				// Cron tasks: never permanently fail -- reschedule at next cron time
				// and reset failure counter so transient errors (rate limits, etc.) don't kill recurring tasks
				next, cronErr := scheduler.CronNext(*cronExpression, time.Now())
				if cronErr != nil {
					next = time.Now().UTC().Add(10 * time.Minute)
				}
				s.wlog("task-worker", "[task-worker] cron task %s hit %d failures, rescheduling at %s: %v", taskID, consecutiveFailures, next.Format(time.RFC3339), execErr)
				const q = `
UPDATE scheduled_tasks
SET status = 'PENDING', last_result = $3, consecutive_failures = 0, execute_at = $2, updated_at = NOW()
WHERE id = $1::uuid;
`
				if _, err := s.db.Exec(taskCtx, q, taskID, next.UTC(), lastResult); err != nil {
					s.wlog("task-worker", "[task-worker] failed to reschedule cron task %s after failures: %v", taskID, err)
				}
			} else {
				// One-time tasks: mark permanently failed
				s.wlog("task-worker", "[task-worker] task %s failed permanently after %d failures: %v", taskID, consecutiveFailures, execErr)
				const q = `
UPDATE scheduled_tasks
SET status = 'FAILED', last_result = $2, consecutive_failures = $3, updated_at = NOW()
WHERE id = $1::uuid;
`
				if _, err := s.db.Exec(taskCtx, q, taskID, lastResult, consecutiveFailures); err != nil {
					s.wlog("task-worker", "[task-worker] failed to mark task %s failed: %v", taskID, err)
				}
			}
		} else {
			s.wlog("task-worker", "[task-worker] task %s failed (attempt %d): %v", taskID, consecutiveFailures, execErr)

			// Advance execute_at with backoff for retries
			delay := s.backoffPolicy.NextDelay(consecutiveFailures)
			nextAt := time.Now().UTC().Add(delay)

			const q = `
UPDATE scheduled_tasks
SET status = 'PENDING', last_result = $2, consecutive_failures = $3, execute_at = $4, updated_at = NOW()
WHERE id = $1::uuid;
`
			if _, err := s.db.Exec(taskCtx, q, taskID, lastResult, consecutiveFailures, nextAt); err != nil {
				s.wlog("task-worker", "[task-worker] failed to update task %s after error: %v", taskID, err)
			}
		}
		return
	}

	// Success: compute next execute_at (for cron) or mark DONE
	newStatus := "DONE"
	var nextAt time.Time

	if cronExpression != nil && *cronExpression != "" {
		next, cronErr := scheduler.CronNext(*cronExpression, time.Now())
		if cronErr == nil {
			nextAt = next
			newStatus = "PENDING"
		} else {
			s.wlog("task-worker", "[task-worker] task %s cron parse error: %v", taskID, cronErr)
			newStatus = "DONE"
		}
	}

	if newStatus == "DONE" {
		const q = `
UPDATE scheduled_tasks
SET status = 'DONE', last_result = $2, consecutive_failures = 0, updated_at = NOW()
WHERE id = $1::uuid;
`
		if _, err := s.db.Exec(taskCtx, q, taskID, lastResult); err != nil {
			s.wlog("task-worker", "[task-worker] failed to mark task %s done: %v", taskID, err)
		}
	} else {
		const q = `
UPDATE scheduled_tasks
SET status = 'PENDING', last_result = $2, consecutive_failures = 0, execute_at = $3, updated_at = NOW()
WHERE id = $1::uuid;
`
		if _, err := s.db.Exec(taskCtx, q, taskID, lastResult, nextAt.UTC()); err != nil {
			s.wlog("task-worker", "[task-worker] failed to reschedule task %s: %v", taskID, err)
		}
		s.wlog("task-worker", "[task-worker] task %s rescheduled at %s", taskID, nextAt.Format(time.RFC3339))
	}
}

func buildScheduledTaskExecutionPrompt(taskID, description, prompt string, cronExpression *string) string {
	return buildScheduledTaskExecutionPromptWithTimezone(taskID, description, prompt, cronExpression, "")
}

func buildScheduledTaskExecutionPromptWithTimezone(taskID, description, prompt string, cronExpression *string, resolvedTZ string) string {
	var b strings.Builder
	b.WriteString("You are executing a scheduled task. Complete the task described below exactly once.\n\n")
	b.WriteString("Task ID: ")
	b.WriteString(taskID)
	b.WriteString("\n")
	if strings.TrimSpace(description) != "" {
		b.WriteString("Task description: ")
		b.WriteString(description)
		b.WriteString("\n")
	}
	if cronExpression != nil && strings.TrimSpace(*cronExpression) != "" {
		b.WriteString("Schedule: ")
		b.WriteString(strings.TrimSpace(*cronExpression))
		b.WriteString("\n")
	}
	if strings.TrimSpace(resolvedTZ) != "" {
		b.WriteString("Resolved timezone: ")
		b.WriteString(strings.TrimSpace(resolvedTZ))
		b.WriteString("\n")
	}
	b.WriteString("\nInstructions:\n")
	b.WriteString(prompt)
	b.WriteString("\n\nIf the task requires calling tools, include all required tool arguments explicitly rather than assuming omitted fields.")
	return b.String()
}
