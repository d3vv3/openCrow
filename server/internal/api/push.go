// push.go -- UnifiedPush HTTP notification dispatch.
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// unifiedPushPayload is the JSON body sent to a UnifiedPush distributor endpoint.
type unifiedPushPayload struct {
	Title          string `json:"title"`
	Body           string `json:"body"`
	Channel        string `json:"channel,omitempty"`
	ConversationID string `json:"conversation_id,omitempty"`
}

// deviceTaskPushPayload is a minimal push payload that wakes the companion app
// to fetch and execute a pending device task. No sensitive data is included.
type deviceTaskPushPayload struct {
	Type   string `json:"type"`
	TaskID string `json:"task_id"`
}

// sendUnifiedPush performs a plain HTTP POST to the given UnifiedPush endpoint.
// The distributor handles last-mile delivery to the device; no VAPID/encryption required.
func (s *Server) sendUnifiedPush(ctx context.Context, endpoint, title, body, channel, conversationID string) error {
	if endpoint == "" {
		return nil
	}

	payload := unifiedPushPayload{Title: title, Body: body, Channel: channel, ConversationID: conversationID}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal push payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("push request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("push endpoint returned %d", resp.StatusCode)
	}
	return nil
}

// sendDeviceTaskPush sends a minimal wake-up push to the device that owns the given task.
// The payload only contains the task ID; the device fetches the full task over HTTPS.
func (s *Server) sendDeviceTaskPush(ctx context.Context, userID, deviceID, taskID string) error {
	endpoint, err := s.getDevicePushEndpoint(ctx, userID, deviceID)
	if err != nil || endpoint == "" {
		return nil // no endpoint registered -- silent no-op, heartbeat poll will pick it up
	}

	payload := deviceTaskPushPayload{Type: "device_task", TaskID: taskID}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal device task push: %w", err)
	}

	pushCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(pushCtx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("create device task push request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("device task push request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("device task push endpoint returned %d", resp.StatusCode)
	}
	return nil
}

// waitForDeviceTask polls the DB until the task transitions out of pending/processing,
// or until the deadline is reached. Returns the completed task or an error on timeout.
func (s *Server) waitForDeviceTask(ctx context.Context, userID, taskID string, timeout time.Duration) (DeviceTaskDTO, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		task, err := s.getDeviceTask(ctx, userID, taskID)
		if err != nil {
			return DeviceTaskDTO{}, err
		}
		if task.Status == "completed" || task.Status == "failed" {
			return task, nil
		}
		select {
		case <-ctx.Done():
			return DeviceTaskDTO{}, ctx.Err()
		case <-time.After(250 * time.Millisecond):
		}
	}
	return DeviceTaskDTO{}, fmt.Errorf("timeout waiting for device task %s", taskID)
}
