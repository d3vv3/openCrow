// push.go — UnifiedPush HTTP notification dispatch.
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
