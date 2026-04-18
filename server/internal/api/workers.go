package api

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"strings"
	"time"

	"github.com/opencrow/opencrow/server/internal/configstore"
	"github.com/opencrow/opencrow/server/internal/orchestrator"
	"github.com/opencrow/opencrow/server/internal/realtime"
	"github.com/opencrow/opencrow/server/internal/scheduler"
)

// wlog writes to both the global logger and the worker's ring buffer.
func (s *Server) wlog(worker, format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	log.Printf("%s", line)
	s.workerLogs.Append(worker, line)
}

// StartWorkers launches background goroutines for scheduled tasks, heartbeat, and email polling.
// It returns immediately; workers run until ctx is cancelled.
func (s *Server) StartWorkers(ctx context.Context) {
	// Reset any tasks stuck in RUNNING from a previous server run
	if err := s.recoverStuckTasks(ctx); err != nil {
		s.wlog("task-worker", "[task-worker] failed to recover stuck tasks: %v", err)
	}
	go s.runTaskWorker(ctx)
	go s.runHeartbeatWorker(ctx)
	go s.runEmailWorker(ctx)
}

// ------------------------------------------------------------
// Scheduled Task Worker
// ------------------------------------------------------------

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
		if _, msgErr := s.createMessage(taskCtx, userID, conv.ID, "user", prompt); msgErr != nil {
			s.wlog("task-worker", "[task-worker] failed to insert user message for task %s: %v", taskID, msgErr)
		}
		if _, msgErr := s.createMessage(taskCtx, userID, conv.ID, "assistant", resultContent); msgErr != nil {
			s.wlog("task-worker", "[task-worker] failed to insert assistant message for task %s: %v", taskID, msgErr)
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

// runOrchestratorForUser runs a prompt through the orchestrator using the user's configured providers.
func (s *Server) runOrchestratorForUser(ctx context.Context, workerName, userID, prompt string) (orchestrator.CompletionResult, error) {
	var cfg *configstore.UserConfig
	if s.configStore != nil {
		if c, err := s.configStore.GetUserConfig(userID); err == nil {
			cfg = &c
		}
	}

	systemPrompt := s.buildSystemPrompt(ctx, userID, cfg)

	// Build tool specs from enabled tools (same logic as chat handlers)
	var toolSpecs []orchestrator.ToolSpec
	if cfg != nil {
		for _, def := range cfg.Tools.Definitions {
			enabled := cfg.Tools.Enabled[def.ID]
			if !enabled {
				continue
			}
			props := map[string]any{}
			var required []string
			for _, p := range def.Parameters {
				prop := map[string]any{"type": p.Type, "description": p.Description}
				props[p.Name] = prop
				if p.Required {
					required = append(required, p.Name)
				}
			}
			toolSpecs = append(toolSpecs, orchestrator.ToolSpec{
				Name:        sanitizeToolName(def.Name),
				Description: def.Description,
				Parameters: map[string]any{
					"properties": props,
					"required":   required,
				},
			})
		}
	}

	// Build providers sorted by Priority (ascending = highest priority first)
	providers := buildProvidersFromConfig(cfg)
	providerNames := make([]string, 0, len(providers))
	for _, provider := range providers {
		providerNames = append(providerNames, provider.Name())
	}
	if len(providerNames) > 0 {
		s.wlog(workerName, "[%s] provider order for user %s: %s", workerName, userID, strings.Join(providerNames, " -> "))
	}
	svc := s.orchestrator
	if len(providers) > 0 {
		svc = orchestrator.NewService(providers, orchestrator.ToolLoopGuard{})
	}

	result, err := svc.Complete(ctx, orchestrator.CompletionRequest{
		System:       systemPrompt,
		Messages:     []orchestrator.ChatMessage{{Role: "user", Content: prompt}},
		Tools:        toolSpecs,
		ToolExecutor: s.buildToolExecutor(ctx, userID),
	})
	if err != nil {
		return result, err
	}
	return result, nil
}

func (s *Server) logWorkerOrchestratorTrace(workerName, runLabel string, result orchestrator.CompletionResult) {
	if len(result.Trace.ProviderAttempts) > 0 {
		for _, attempt := range result.Trace.ProviderAttempts {
			if attempt.Success {
				s.wlog(workerName, "[%s] %s provider %s attempt %d succeeded", workerName, runLabel, attempt.Provider, attempt.Attempt)
			} else {
				s.wlog(workerName, "[%s] %s provider %s attempt %d failed: %s", workerName, runLabel, attempt.Provider, attempt.Attempt, attempt.Error)
			}
		}
	}

	if len(result.Trace.ToolCalls) == 0 {
		s.wlog(workerName, "[%s] %s executed no tools", workerName, runLabel)
		return
	}

	for i, tc := range result.Trace.ToolCalls {
		argsJSON, _ := json.Marshal(tc.Arguments)
		if len(argsJSON) == 0 || string(argsJSON) == "null" {
			argsJSON = []byte("{}")
		}
		preview := workerLogPreview(tc.Output, 220)
		if tc.Status == "error" {
			s.wlog(workerName, "[%s] %s tool %d/%d %s %s -> error: %s", workerName, runLabel, i+1, len(result.Trace.ToolCalls), tc.Name, string(argsJSON), preview)
			continue
		}
		s.wlog(workerName, "[%s] %s tool %d/%d %s %s -> %s", workerName, runLabel, i+1, len(result.Trace.ToolCalls), tc.Name, string(argsJSON), preview)
	}
}

func workerLogPreview(text string, max int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "(no output)"
	}
	text = strings.ReplaceAll(text, "\n", " ↩ ")
	text = strings.ReplaceAll(text, "\r", "")
	if len(text) <= max {
		return text
	}
	if max <= 3 {
		return text[:max]
	}
	return text[:max-3] + "..."
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

// ------------------------------------------------------------
// Heartbeat Worker
// ------------------------------------------------------------

func (s *Server) runHeartbeatWorker(ctx context.Context) {
	scanTicker := time.NewTicker(1 * time.Second)
	diagTicker := time.NewTicker(60 * time.Second)
	defer scanTicker.Stop()
	defer diagTicker.Stop()
	s.wlog("heartbeat-worker", "[heartbeat-worker] started")
	s.logHeartbeatWorkerStatus(ctx)
	if err := s.processDueHeartbeats(ctx); err != nil {
		s.wlog("heartbeat-worker", "[heartbeat-worker] error on startup scan: %v", err)
	}
	for {
		select {
		case <-ctx.Done():
			s.wlog("heartbeat-worker", "[heartbeat-worker] stopped")
			return
		case <-scanTicker.C:
			err := s.processDueHeartbeats(ctx)
			s.workerStatus.tick("heartbeat-worker", err)
			if err != nil {
				s.wlog("heartbeat-worker", "[heartbeat-worker] error: %v", err)
			}
		case <-diagTicker.C:
			s.logHeartbeatWorkerStatus(ctx)
		}
	}
}

func (s *Server) logHeartbeatWorkerStatus(ctx context.Context) {
	const countQ = `
SELECT
	COUNT(*) FILTER (WHERE enabled = TRUE) AS enabled_count,
	COUNT(*) FILTER (WHERE enabled = TRUE AND (next_run_at IS NULL OR next_run_at <= NOW())) AS due_count
FROM user_heartbeat_configs;
`
	var enabledCount, dueCount int
	if err := s.db.QueryRow(ctx, countQ).Scan(&enabledCount, &dueCount); err == nil {
		s.wlog("heartbeat-worker", "[heartbeat-worker] heartbeat configs: %d enabled, %d due now", enabledCount, dueCount)
	} else {
		s.wlog("heartbeat-worker", "[heartbeat-worker] failed to query heartbeat status: %v", err)
	}
}

func (s *Server) processDueHeartbeats(ctx context.Context) error {
	const q = `
SELECT user_id::text, interval_seconds
FROM user_heartbeat_configs
WHERE enabled = TRUE AND (next_run_at IS NULL OR next_run_at <= NOW());
`
	rows, err := s.db.Query(ctx, q)
	if err != nil {
		return fmt.Errorf("query heartbeats: %w", err)
	}
	defer rows.Close()

	type hbRow struct {
		userID          string
		intervalSeconds int
	}
	var due []hbRow
	for rows.Next() {
		var r hbRow
		if err := rows.Scan(&r.userID, &r.intervalSeconds); err != nil {
			return fmt.Errorf("scan heartbeat row: %w", err)
		}
		due = append(due, r)
	}
	rows.Close()

	if len(due) == 0 {
		return nil
	}
	s.wlog("heartbeat-worker", "[heartbeat-worker] running heartbeat for %d user(s)", len(due))
	for _, r := range due {
		s.executeHeartbeat(ctx, r.userID, r.intervalSeconds)
	}
	return nil
}

func (s *Server) executeHeartbeat(ctx context.Context, userID string, intervalSeconds int) {
	hbCtx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	var cfg *configstore.UserConfig
	if s.configStore != nil {
		if c, err := s.configStore.GetUserConfig(userID); err == nil {
			cfg = &c
		}
	}
	resolvedTZ := preferredTimezoneName(hbCtx, cfg, "")
	prompt := buildHeartbeatExecutionPrompt(cfg, resolvedTZ)
	s.wlog("heartbeat-worker", "[heartbeat-worker] heartbeat for user %s resolved timezone: %s", userID, resolvedTZ)
	result, err := s.runOrchestratorForUser(hbCtx, "heartbeat-worker", userID, prompt)
	output := result.Output
	s.logWorkerOrchestratorTrace("heartbeat-worker", fmt.Sprintf("heartbeat user %s", userID), result)

	status := "ok"
	message := output
	if err != nil {
		status = "error"
		message = err.Error()
		s.wlog("heartbeat-worker", "[heartbeat-worker] heartbeat for user %s failed: %v", userID, err)
	} else if strings.TrimSpace(output) != "HEARTBEAT_OK" {
		status = "attention"
		s.wlog("heartbeat-worker", "[heartbeat-worker] heartbeat for user %s requires attention: %s", userID, workerLogPreview(output, 220))
	} else {
		s.wlog("heartbeat-worker", "[heartbeat-worker] heartbeat for user %s succeeded", userID)
	}

	// Log event
	if _, logErr := s.createHeartbeatEvent(hbCtx, userID, status, message); logErr != nil {
		s.wlog("heartbeat-worker", "[heartbeat-worker] failed to log heartbeat event for user %s: %v", userID, logErr)
	}

	if status != "ok" {
		s.realtimeHub.Publish(realtime.Event{
			UserID: userID,
			Type:   "notification",
			Payload: map[string]any{
				"title": "Heartbeat attention",
				"body":  workerLogPreview(message, 280),
			},
		})

		titleBase := workerLogPreview(message, 72)
		if titleBase == "(no output)" {
			titleBase = strings.ToUpper(status)
		}
		title := "Heartbeat: " + titleBase
		if len(title) > 100 {
			title = title[:100] + "..."
		}
		if conv, convErr := s.createConversation(hbCtx, userID, title); convErr == nil {
			if _, msgErr := s.createMessage(hbCtx, userID, conv.ID, "user", prompt); msgErr != nil {
				s.wlog("heartbeat-worker", "[heartbeat-worker] failed to insert heartbeat prompt for user %s: %v", userID, msgErr)
			}
			resultContent := message
			if strings.TrimSpace(resultContent) == "" {
				resultContent = "Heartbeat requires attention"
			}
			if _, msgErr := s.createMessage(hbCtx, userID, conv.ID, "assistant", resultContent); msgErr != nil {
				s.wlog("heartbeat-worker", "[heartbeat-worker] failed to insert heartbeat result for user %s: %v", userID, msgErr)
			}
			for _, tc := range result.Trace.ToolCalls {
				outputStr := tc.Output
				errStr := ""
				if tc.Status == "error" {
					errStr = tc.Output
					outputStr = ""
				}
				if saveErr := s.saveToolCall(hbCtx, userID, conv.ID, tc.Name, tc.Arguments, outputStr, errStr, 0); saveErr != nil {
					s.wlog("heartbeat-worker", "[heartbeat-worker] failed to persist tool call %s for heartbeat conversation %s: %v", tc.Name, conv.ID, saveErr)
				}
			}
		} else {
			s.wlog("heartbeat-worker", "[heartbeat-worker] failed to create heartbeat conversation for user %s: %v", userID, convErr)
		}
	}

	// Update next_run_at
	nextRun := time.Now().UTC().Add(time.Duration(intervalSeconds) * time.Second)
	const q = `
UPDATE user_heartbeat_configs
SET next_run_at = $2, updated_at = NOW()
WHERE user_id = $1::uuid;
`
	if _, upErr := s.db.Exec(hbCtx, q, userID, nextRun); upErr != nil {
		s.wlog("heartbeat-worker", "[heartbeat-worker] failed to update next_run_at for user %s: %v", userID, upErr)
	}
}

func buildHeartbeatExecutionPrompt(cfg *configstore.UserConfig, resolvedTZ string) string {
	base := "Provide a brief status update. Note the current time, any pending tasks, and confirm you are operational. Reply with exactly HEARTBEAT_OK if everything is fine and there is nothing noteworthy to report. If there is anything notable, reply with a concise explanation instead."
	if cfg != nil && strings.TrimSpace(cfg.Prompts.HeartbeatPrompt) != "" {
		base = strings.TrimSpace(cfg.Prompts.HeartbeatPrompt)
	}
	if strings.TrimSpace(resolvedTZ) == "" {
		return base
	}
	return fmt.Sprintf("Resolved timezone: %s\n\n%s", resolvedTZ, base)
}

func summarizeWorkerExecution(result orchestrator.CompletionResult, execErr error, resolvedTZ string) string {
	var parts []string
	if strings.TrimSpace(resolvedTZ) != "" {
		parts = append(parts, fmt.Sprintf("timezone: %s", strings.TrimSpace(resolvedTZ)))
	}
	if execErr != nil {
		parts = append(parts, fmt.Sprintf("status: error\nerror: %s", execErr.Error()))
	} else {
		parts = append(parts, "status: ok")
	}
	if len(result.Trace.ProviderAttempts) > 0 {
		var attempts []string
		for _, attempt := range result.Trace.ProviderAttempts {
			if attempt.Success {
				attempts = append(attempts, fmt.Sprintf("%s attempt %d ok", attempt.Provider, attempt.Attempt))
			} else {
				attempts = append(attempts, fmt.Sprintf("%s attempt %d failed: %s", attempt.Provider, attempt.Attempt, attempt.Error))
			}
		}
		parts = append(parts, "provider attempts: "+strings.Join(attempts, " | "))
	}
	if len(result.Trace.ToolCalls) > 0 {
		var toolLines []string
		for _, tc := range result.Trace.ToolCalls {
			argsJSON, _ := json.Marshal(tc.Arguments)
			if len(argsJSON) == 0 || string(argsJSON) == "null" {
				argsJSON = []byte("{}")
			}
			line := fmt.Sprintf("- %s %s", tc.Name, string(argsJSON))
			if strings.TrimSpace(tc.Output) != "" {
				line += " -> " + workerLogPreview(tc.Output, 240)
			}
			toolLines = append(toolLines, line)
		}
		parts = append(parts, "tools:\n"+strings.Join(toolLines, "\n"))
	}
	if strings.TrimSpace(result.Output) != "" {
		parts = append(parts, "assistant reply:\n"+result.Output)
	}
	return strings.Join(parts, "\n\n")
}

// ------------------------------------------------------------
// Email Inbox Poll Worker
// ------------------------------------------------------------

func (s *Server) runEmailWorker(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()
	s.wlog("email-worker", "[email-worker] started")
	for {
		select {
		case <-ctx.Done():
			s.wlog("email-worker", "[email-worker] stopped")
			return
		case <-ticker.C:
			err := s.processDueEmailInboxes(ctx)
			s.workerStatus.tick("email-worker", err)
			if err != nil {
				s.wlog("email-worker", "[email-worker] error: %v", err)
			}
		}
	}
}

func (s *Server) processDueEmailInboxes(ctx context.Context) error {
	const q = `
SELECT id::text, user_id::text, address, imap_host, imap_port, imap_username, imap_password, use_tls
FROM email_inboxes
WHERE active = TRUE AND (last_polled_at IS NULL OR last_polled_at + (poll_interval_seconds * interval '1 second') <= NOW())
ORDER BY last_polled_at ASC NULLS FIRST
LIMIT 20;
`
	rows, err := s.db.Query(ctx, q)
	if err != nil {
		return fmt.Errorf("query due email inboxes: %w", err)
	}
	defer rows.Close()

	type inboxPollRow struct {
		id           string
		userID       string
		address      string
		imapHost     string
		imapPort     int
		imapUsername string
		imapPassword string
		useTLS       bool
	}

	var due []inboxPollRow
	for rows.Next() {
		var r inboxPollRow
		if err := rows.Scan(&r.id, &r.userID, &r.address, &r.imapHost, &r.imapPort, &r.imapUsername, &r.imapPassword, &r.useTLS); err != nil {
			return fmt.Errorf("scan inbox row: %w", err)
		}
		due = append(due, r)
	}
	rows.Close()

	for _, r := range due {
		s.pollEmailInbox(ctx, r.id, r.userID, r.address, r.imapHost, r.imapPort, r.imapUsername, r.imapPassword, r.useTLS)
	}
	return nil
}

func (s *Server) pollEmailInbox(ctx context.Context, inboxID, userID, address, imapHost string, imapPort int, imapUsername, imapPassword string, useTLS bool) {
	pollCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	if imapUsername == "" || imapPassword == "" {
		detail := "skipped: IMAP credentials not configured"
		s.wlog("email-worker", "[email-worker] inbox %s (%s): %s", inboxID, address, detail)
		if _, err := s.createEmailPollEvent(pollCtx, userID, inboxID, "skipped", detail); err != nil {
			s.wlog("email-worker", "[email-worker] failed to log poll event: %v", err)
		}
		return
	}

	addr := fmt.Sprintf("%s:%d", imapHost, imapPort)
	detail, pollErr := checkIMAPConnectivity(pollCtx, addr, imapUsername, imapPassword, useTLS)

	status := "ok"
	if pollErr != nil {
		status = "error"
		detail = pollErr.Error()
		s.wlog("email-worker", "[email-worker] inbox %s (%s) poll error: %v", inboxID, address, pollErr)
	} else {
		s.wlog("email-worker", "[email-worker] inbox %s (%s): %s", inboxID, address, detail)
	}

	if _, err := s.createEmailPollEvent(pollCtx, userID, inboxID, status, detail); err != nil {
		s.wlog("email-worker", "[email-worker] failed to log poll event for inbox %s: %v", inboxID, err)
	}
}

// checkIMAPConnectivity dials the IMAP server and attempts LOGIN.
// Returns a detail string describing what was found, or an error.
func checkIMAPConnectivity(ctx context.Context, addr, username, password string, useTLS bool) (string, error) {
	dialer := &net.Dialer{}
	var conn net.Conn
	var err error

	if useTLS {
		host, _, _ := net.SplitHostPort(addr)
		tlsCfg := &tls.Config{ServerName: host}
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, tlsCfg)
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return "", fmt.Errorf("dial %s: %w", addr, err)
	}
	defer conn.Close()

	// Set deadline from context
	if deadline, ok := ctx.Deadline(); ok {
		conn.SetDeadline(deadline)
	}

	// Read server greeting
	buf := make([]byte, 512)
	n, err := conn.Read(buf)
	if err != nil {
		return "", fmt.Errorf("read greeting: %w", err)
	}
	greeting := strings.TrimSpace(string(buf[:n]))

	if !strings.HasPrefix(greeting, "* OK") {
		return "", fmt.Errorf("unexpected greeting: %s", greeting)
	}

	// Send LOGIN command
	_, err = fmt.Fprintf(conn, "a001 LOGIN %s %s\r\n", imapQuote(username), imapQuote(password))
	if err != nil {
		return "", fmt.Errorf("send LOGIN: %w", err)
	}

	// Read response
	respBuf := make([]byte, 1024)
	n, err = conn.Read(respBuf)
	if err != nil {
		return "", fmt.Errorf("read LOGIN response: %w", err)
	}
	resp := strings.TrimSpace(string(respBuf[:n]))

	if strings.Contains(resp, "a001 OK") {
		// Send LOGOUT
		fmt.Fprintf(conn, "a002 LOGOUT\r\n")
		return fmt.Sprintf("connected and authenticated to %s", addr), nil
	}
	return "", fmt.Errorf("LOGIN failed: %s", resp)
}

// imapQuote wraps a string in IMAP literal or quoted string format.
func imapQuote(s string) string {
	// Simple quoted string - escape backslash and double-quote
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
