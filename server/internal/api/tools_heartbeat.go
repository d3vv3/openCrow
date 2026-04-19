package api

import (
	"context"
	"fmt"

	"github.com/opencrow/opencrow/server/internal/realtime"
)

func (s *Server) toolConfigureHeartbeat(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	req := UpdateHeartbeatConfigRequest{}
	if enabled, ok := args["enabled"].(bool); ok {
		req.Enabled = &enabled
	}
	if interval, ok := args["intervalSeconds"].(float64); ok {
		i := int(interval)
		req.IntervalSeconds = &i
	}
	cfg, err := s.putHeartbeatConfig(ctx, userID, req)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to configure heartbeat: %v", err)}, nil
	}
	return map[string]any{
		"success":          true,
		"enabled":          cfg.Enabled,
		"interval_seconds": cfg.IntervalSeconds,
		"message":          "Heartbeat configured",
	}, nil
}

func (s *Server) toolTriggerHeartbeat(ctx context.Context, userID string) (map[string]any, error) {
	evt, err := s.createHeartbeatEvent(ctx, userID, "TRIGGERED", "tool-triggered heartbeat")
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to trigger heartbeat: %v", err)}, nil
	}
	s.realtimeHub.Publish(realtime.Event{
		UserID:  userID,
		Type:    "heartbeat.triggered",
		Payload: map[string]any{"eventId": evt.ID},
	})
	return map[string]any{
		"success":  true,
		"event_id": evt.ID,
		"message":  "Heartbeat triggered",
	}, nil
}
