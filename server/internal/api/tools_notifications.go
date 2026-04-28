// tools_notifications.go — Notification dispatch and Telegram bot setup tools.
package api

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/google/uuid"
	"github.com/opencrow/opencrow/server/internal/configstore"
	"github.com/opencrow/opencrow/server/internal/realtime"
)

// ── Notification ─────────────────────────────────────────────────────────

func (s *Server) toolSendNotification(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	title, _ := args["title"].(string)
	body, _ := args["body"].(string)
	if title == "" || body == "" {
		return map[string]any{"success": false, "error": "title and body are required"}, nil
	}

	// Publish via realtime hub for connected clients to consume
	s.realtimeHub.Publish(realtime.Event{
		UserID: userID,
		Type:   "notification",
		Payload: map[string]any{
			"title": title,
			"body":  body,
		},
	})

	// Fanout to Telegram bots with a notificationChatId configured
	telegramSent := 0
	if s.configStore != nil {
		if cfg, err := s.configStore.GetUserConfig(userID); err == nil {
			msg := fmt.Sprintf("🔔 *%s*\n%s", title, body)
			bots := cfg.Integrations.TelegramBots
			defaultID := cfg.Integrations.DefaultNotificationBotID
			sent := false
			if defaultID != "" {
				// Send only to the default bot
				for _, bot := range bots {
					if bot.ID == defaultID && bot.Enabled && bot.BotToken != "" && bot.NotificationChatID != "" {
						if err := s.TelegramSendNotification(ctx, bot.BotToken, bot.NotificationChatID, msg); err != nil {
							log.Printf("[send_notification] telegram bot %s error: %v", bot.Label, err)
						} else {
							telegramSent++
						}
						sent = true
						break
					}
				}
			}
			if !sent {
				// Fan out to all bots (no default set, or default not found/usable)
				for _, bot := range bots {
					if !bot.Enabled || bot.BotToken == "" || bot.NotificationChatID == "" {
						continue
					}
					if err := s.TelegramSendNotification(ctx, bot.BotToken, bot.NotificationChatID, msg); err != nil {
						log.Printf("[send_notification] telegram bot %s error: %v", bot.Label, err)
					} else {
						telegramSent++
					}
				}
			}
		}
	}

	detail := "Notification sent via realtime hub."
	if telegramSent > 0 {
		detail += fmt.Sprintf(" Also sent to %d Telegram bot(s).", telegramSent)
	}

	// Fanout to UnifiedPush endpoints for all registered devices
	channel, _ := args["channel"].(string)
	if endpoints, err := s.getDevicePushEndpoints(ctx, userID); err == nil {
		for _, ep := range endpoints {
			if err := s.sendUnifiedPush(ctx, ep, title, body, channel, ""); err != nil {
				log.Printf("[send_notification] UP push to %s error: %v", ep, err)
			} else {
				detail += " UP push sent."
			}
		}
	}

	return map[string]any{
		"success": true,
		"message": detail,
	}, nil
}

// notifyChannels fans out a title+body notification to all enabled Telegram bots
// that have a notificationChatId configured. It is safe to call from any worker.
func (s *Server) notifyChannels(ctx context.Context, userID, title, body string) {
	if s.configStore == nil {
		return
	}
	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		return
	}
	msg := fmt.Sprintf("🔔 *%s*\n%s", title, body)
	defaultID := cfg.Integrations.DefaultNotificationBotID
	if defaultID != "" {
		for _, bot := range cfg.Integrations.TelegramBots {
			if bot.ID == defaultID && bot.Enabled && bot.BotToken != "" && bot.NotificationChatID != "" {
				if err := s.TelegramSendNotification(ctx, bot.BotToken, bot.NotificationChatID, msg); err != nil {
					log.Printf("[channels] telegram bot %s notify error: %v", bot.Label, err)
				}
				return
			}
		}
		// defaultID set but bot not found or not usable -- fall through to fan-out
	}
	for _, bot := range cfg.Integrations.TelegramBots {
		if !bot.Enabled || bot.BotToken == "" || bot.NotificationChatID == "" {
			continue
		}
		if err := s.TelegramSendNotification(ctx, bot.BotToken, bot.NotificationChatID, msg); err != nil {
			log.Printf("[channels] telegram bot %s notify error: %v", bot.Label, err)
		}
	}

	// Fanout to UnifiedPush endpoints for all registered devices
	if endpoints, err := s.getDevicePushEndpoints(ctx, userID); err == nil {
		for _, ep := range endpoints {
			if err := s.sendUnifiedPush(ctx, ep, title, body, "", ""); err != nil {
				log.Printf("[channels] UP push to %s error: %v", ep, err)
			}
		}
	}
}

// toolSendPushNotification sends a UnifiedPush notification directly to one or all companion
// devices registered for the user. Unlike send_notification it only targets UP endpoints
// and does not fanout to Telegram or the realtime hub.
func (s *Server) toolSendPushNotification(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	title, _ := args["title"].(string)
	body, _ := args["body"].(string)
	if title == "" || body == "" {
		return map[string]any{"success": false, "error": "title and body are required"}, nil
	}
	channel, _ := args["channel"].(string)
	deviceID, _ := args["device_id"].(string)
	conversationID, _ := args["conversation_id"].(string)
	if conversationID == "" {
		conversationID, _ = ctx.Value(conversationIDContextKey).(string)
	}

	var endpoints []string
	if deviceID != "" {
		ep, err := s.getDevicePushEndpoint(ctx, userID, deviceID)
		if err != nil {
			return map[string]any{"success": false, "error": "device not found"}, nil
		}
		if ep == "" {
			return map[string]any{"success": false, "error": "device has no registered push endpoint"}, nil
		}
		endpoints = []string{ep}
	} else {
		eps, err := s.getDevicePushEndpoints(ctx, userID)
		if err != nil {
			return map[string]any{"success": false, "error": "failed to load push endpoints"}, nil
		}
		if len(eps) == 0 {
			return map[string]any{"success": false, "error": "no devices have a registered push endpoint"}, nil
		}
		endpoints = eps
	}

	sent := 0
	for _, ep := range endpoints {
		if err := s.sendUnifiedPush(ctx, ep, title, body, channel, conversationID); err != nil {
			log.Printf("[send_push_notification] UP push to %s error: %v", ep, err)
		} else {
			sent++
		}
	}

	if sent == 0 {
		return map[string]any{"success": false, "error": "failed to deliver push to any endpoint"}, nil
	}
	return map[string]any{
		"success": true,
		"message": fmt.Sprintf("Push notification delivered to %d device(s).", sent),
	}, nil
}

// ── Channel setup tools ───────────────────────────────────────────────────

func (s *Server) toolSetupTelegramBot(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	botToken, _ := args["bot_token"].(string)
	botToken = strings.TrimSpace(botToken)
	if botToken == "" {
		return map[string]any{"success": false, "error": "bot_token is required"}, nil
	}

	label, _ := args["label"].(string)
	if label == "" {
		label = "Telegram Bot"
	}
	notificationChatID, _ := args["notification_chat_id"].(string)
	allowedRaw, _ := args["allowed_chat_ids"].(string)
	var allowedChatIDs []string
	for _, id := range strings.Split(allowedRaw, ",") {
		id = strings.TrimSpace(id)
		if id != "" {
			allowedChatIDs = append(allowedChatIDs, id)
		}
	}
	pollInterval := 5
	if p, ok := args["poll_interval_seconds"].(float64); ok && p > 0 {
		pollInterval = int(p)
	}

	if s.configStore == nil {
		return map[string]any{"success": false, "error": "config store unavailable"}, nil
	}

	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		return map[string]any{"success": false, "error": "failed to load config"}, nil
	}

	// Check if a bot with this token already exists -- update it
	updated := false
	for i, bot := range cfg.Integrations.TelegramBots {
		if bot.BotToken == botToken {
			cfg.Integrations.TelegramBots[i].Label = label
			cfg.Integrations.TelegramBots[i].NotificationChatID = notificationChatID
			cfg.Integrations.TelegramBots[i].AllowedChatIDs = allowedChatIDs
			cfg.Integrations.TelegramBots[i].PollIntervalSeconds = pollInterval
			cfg.Integrations.TelegramBots[i].Enabled = true
			updated = true
			break
		}
	}

	if !updated {
		newID := uuid.NewString()
		cfg.Integrations.TelegramBots = append(cfg.Integrations.TelegramBots, configstore.TelegramBotConfig{
			ID:                  newID,
			Label:               label,
			BotToken:            botToken,
			AllowedChatIDs:      allowedChatIDs,
			NotificationChatID:  notificationChatID,
			Enabled:             true,
			PollIntervalSeconds: pollInterval,
		})
	}

	if _, err := s.configStore.PutUserConfig(userID, cfg); err != nil {
		return map[string]any{"success": false, "error": "failed to save config: " + err.Error()}, nil
	}

	action := "added"
	if updated {
		action = "updated"
	}
	return map[string]any{
		"success": true,
		"message": fmt.Sprintf("Telegram bot '%s' %s successfully. The bot will start polling for messages shortly.", label, action),
	}, nil
}
