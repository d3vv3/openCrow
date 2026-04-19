// worker_heartbeat.go — Background worker for periodic heartbeat checks.
package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/opencrow/opencrow/server/internal/configstore"
	"github.com/opencrow/opencrow/server/internal/realtime"
)

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

		// Fan out to configured Telegram channels
		s.notifyChannels(hbCtx, userID, "Heartbeat attention", workerLogPreview(message, 280))

		titleBase := workerLogPreview(message, 72)
		if titleBase == "(no output)" {
			titleBase = strings.ToUpper(status)
		}
		title := "Heartbeat: " + titleBase
		if len(title) > 100 {
			title = title[:100] + "..."
		}
		if conv, convErr := s.createConversation(hbCtx, userID, title); convErr == nil {
			if _, msgErr := s.createMessage(hbCtx, userID, conv.ID, "user", prompt, nil); msgErr != nil {
				s.wlog("heartbeat-worker", "[heartbeat-worker] failed to insert heartbeat prompt for user %s: %v", userID, msgErr)
			}
			resultContent := message
			if strings.TrimSpace(resultContent) == "" {
				resultContent = "Heartbeat requires attention"
			}
			if _, msgErr := s.createMessage(hbCtx, userID, conv.ID, "assistant", resultContent, nil); msgErr != nil {
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
	base := configstore.DefaultHeartbeatPrompt
	if cfg != nil && strings.TrimSpace(cfg.Prompts.HeartbeatPrompt) != "" {
		base = strings.TrimSpace(cfg.Prompts.HeartbeatPrompt)
	}
	if strings.TrimSpace(resolvedTZ) == "" {
		return base
	}
	return fmt.Sprintf("Resolved timezone: %s\n\n%s", resolvedTZ, base)
}
