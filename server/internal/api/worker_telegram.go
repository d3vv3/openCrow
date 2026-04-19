// worker_telegram.go — Background worker for Telegram bot polling and message handling.
package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/opencrow/opencrow/server/internal/configstore"
)

func (s *Server) runTelegramWorker(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	s.wlog("telegram-worker", "[telegram-worker] started")
	for {
		select {
		case <-ctx.Done():
			s.wlog("telegram-worker", "[telegram-worker] stopped")
			return
		case <-ticker.C:
			err := s.processTelegramBots(ctx)
			s.workerStatus.tick("telegram-worker", err)
			if err != nil {
				s.wlog("telegram-worker", "[telegram-worker] error: %v", err)
			}
		}
	}
}

func (s *Server) processTelegramBots(ctx context.Context) error {
	if s.configStore == nil {
		return nil
	}
	userIDs, err := s.configStore.ListUserIDs()
	if err != nil {
		return fmt.Errorf("list user IDs: %w", err)
	}
	for _, userID := range userIDs {
		cfg, err := s.configStore.GetUserConfig(userID)
		if err != nil {
			continue
		}
		changed := false
		for i, bot := range cfg.Integrations.TelegramBots {
			if !bot.Enabled || bot.BotToken == "" {
				continue
			}
			if _, alreadyDone := s.tgRegistered.LoadOrStore(bot.BotToken, struct{}{}); !alreadyDone {
				if err := s.setTelegramCommands(ctx, bot.BotToken); err != nil {
					s.wlog("telegram-worker", "[telegram-worker] failed to register commands for bot %s: %v", bot.Label, err)
				} else {
					s.wlog("telegram-worker", "[telegram-worker] registered commands for bot %s", bot.Label)
				}
			}
			newLastID, err := s.pollTelegramBot(ctx, userID, bot)
			if err != nil {
				s.wlog("telegram-worker", "[telegram-worker] user %s bot %s poll error: %v", userID, bot.Label, err)
				continue
			}
			if newLastID > cfg.Integrations.TelegramBots[i].LastUpdateID {
				cfg.Integrations.TelegramBots[i].LastUpdateID = newLastID
				changed = true
			}
		}
		if changed {
			if _, err := s.configStore.PutUserConfig(userID, cfg); err != nil {
				s.wlog("telegram-worker", "[telegram-worker] failed to save config for user %s: %v", userID, err)
			}
		}
	}
	return nil
}

type tgGetUpdatesResp struct {
	OK     bool       `json:"ok"`
	Result []tgUpdate `json:"result"`
}

type tgUpdate struct {
	UpdateID int64     `json:"update_id"`
	Message  tgMessage `json:"message"`
}

type tgMessage struct {
	MessageID int64         `json:"message_id"`
	Text      string        `json:"text"`
	Caption   string        `json:"caption"`
	Photo     []tgPhotoSize `json:"photo"`
	Voice     *tgVoice      `json:"voice"`
	Chat      tgChat        `json:"chat"`
	From      tgUser        `json:"from"`
}

type tgPhotoSize struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int    `json:"file_size"`
}

type tgVoice struct {
	FileID   string `json:"file_id"`
	Duration int    `json:"duration"`
	MimeType string `json:"mime_type"`
	FileSize int    `json:"file_size"`
}

type tgChat struct {
	ID int64 `json:"id"`
}

type tgUser struct {
	Username  string `json:"username"`
	FirstName string `json:"first_name"`
}

func (s *Server) pollTelegramBot(ctx context.Context, userID string, bot configstore.TelegramBotConfig) (int64, error) {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/getUpdates?offset=%d&timeout=0&limit=100", bot.BotToken, bot.LastUpdateID+1)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return bot.LastUpdateID, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return bot.LastUpdateID, err
	}
	defer resp.Body.Close()

	var result tgGetUpdatesResp
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return bot.LastUpdateID, fmt.Errorf("decode getUpdates: %w", err)
	}
	if !result.OK {
		return bot.LastUpdateID, fmt.Errorf("telegram getUpdates not ok")
	}

	// Build allowed chat set
	allowedSet := map[string]bool{}
	for _, cid := range bot.AllowedChatIDs {
		allowedSet[strings.TrimSpace(cid)] = true
	}

	lastID := bot.LastUpdateID
	for _, upd := range result.Result {
		if upd.UpdateID > lastID {
			lastID = upd.UpdateID
		}

		msg := upd.Message
		hasText := msg.Text != ""
		hasPhoto := len(msg.Photo) > 0
		hasVoice := msg.Voice != nil

		// Skip updates with no actionable content
		if !hasText && !hasPhoto && !hasVoice {
			continue
		}

		chatIDStr := fmt.Sprintf("%d", msg.Chat.ID)
		if len(allowedSet) > 0 && !allowedSet[chatIDStr] {
			s.wlog("telegram-worker", "[telegram-worker] ignoring message from disallowed chat %s", chatIDStr)
			continue
		}

		// Handle /start command
		if strings.TrimSpace(msg.Text) == "/start" {
			if sendErr := s.sendTelegramMessage(ctx, bot.BotToken, msg.Chat.ID, "👋 Hi! I'm your personal AI assistant. Just send me a message to get started.\n\nUse /new to clear the conversation history."); sendErr != nil {
				s.wlog("telegram-worker", "[telegram-worker] failed to send /start reply to chat %s: %v", chatIDStr, sendErr)
			}
			continue
		}

		// Handle /new command -- start a fresh conversation
		if strings.TrimSpace(msg.Text) == "/new" {
			s.wlog("telegram-worker", "[telegram-worker] /new command from chat %s -- starting fresh conversation", chatIDStr)
			if err := s.deleteTelegramConversations(ctx, userID, chatIDStr); err != nil {
				s.wlog("telegram-worker", "[telegram-worker] failed to delete old conversation for chat %s: %v", chatIDStr, err)
			}
			if _, err := s.newTelegramConversation(ctx, userID, chatIDStr); err != nil {
				s.wlog("telegram-worker", "[telegram-worker] failed to create new conversation: %v", err)
			}
			if sendErr := s.sendTelegramMessage(ctx, bot.BotToken, msg.Chat.ID, "👋 New conversation started. What's on your mind?"); sendErr != nil {
				s.wlog("telegram-worker", "[telegram-worker] failed to send /new confirmation to chat %s: %v", chatIDStr, sendErr)
			}
			continue
		}

		// ── Build the prompt ─────────────────────────────────────────────────

		var promptParts []string

		// Voice message: transcribe with local Whisper
		if hasVoice {
			_ = s.sendChatAction(ctx, bot.BotToken, msg.Chat.ID, "typing")
			audioData, _, dlErr := downloadTelegramFile(ctx, bot.BotToken, msg.Voice.FileID)
			if dlErr != nil {
				s.wlog("telegram-worker", "[telegram-worker] failed to download voice file: %v", dlErr)
			} else {
				mimeType := msg.Voice.MimeType
				if mimeType == "" {
					mimeType = "audio/ogg"
				}
				transcript, tErr := s.whisper.Transcribe(ctx, audioData, mimeType)
				if tErr != nil {
					s.wlog("telegram-worker", "[telegram-worker] whisper transcription failed: %v", tErr)
					_ = s.sendTelegramMessage(ctx, bot.BotToken, msg.Chat.ID, "⚠️ Could not transcribe voice message.")
					continue
				}
				s.wlog("telegram-worker", "[telegram-worker] voice transcription: %s", workerLogPreview(transcript, 120))
				promptParts = append(promptParts, "🎤 "+transcript)
			}
		}

		// Photo message: download and embed as data URL
		if hasPhoto {
			// Pick the largest photo (last in the array)
			largest := msg.Photo[len(msg.Photo)-1]
			imgData, filePath, dlErr := downloadTelegramFile(ctx, bot.BotToken, largest.FileID)
			if dlErr != nil {
				s.wlog("telegram-worker", "[telegram-worker] failed to download photo: %v", dlErr)
			} else {
				ext := strings.ToLower(filepath.Ext(filePath))
				mimeType := "image/jpeg"
				switch ext {
				case ".png":
					mimeType = "image/png"
				case ".gif":
					mimeType = "image/gif"
				case ".webp":
					mimeType = "image/webp"
				}
				dataURL := "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(imgData)
				caption := msg.Caption
				if caption == "" {
					caption = "photo"
				}
				promptParts = append(promptParts, fmt.Sprintf("![%s](%s)", caption, dataURL))
			}
		}

		// Text (or caption for photo-only messages)
		textContent := msg.Text
		if textContent == "" {
			textContent = msg.Caption
		}
		if textContent != "" {
			promptParts = append(promptParts, textContent)
		}

		rawPrompt := strings.Join(promptParts, "\n\n")
		prompt := rawPrompt

		s.wlog("telegram-worker", "[telegram-worker] user %s bot %s message from chat %s: %s", userID, bot.Label, chatIDStr, workerLogPreview(prompt, 120))

		// ── Load / create conversation for this chat ──────────────────────────
		convID, convErr := s.getOrCreateTelegramConversation(ctx, userID, chatIDStr)
		if convErr != nil {
			s.wlog("telegram-worker", "[telegram-worker] conversation lookup failed: %v", convErr)
		}

		// ── Typing indicator ─────────────────────────────────────────────────
		typingDone := make(chan struct{})
		go func() {
			_ = s.sendChatAction(ctx, bot.BotToken, msg.Chat.ID, "typing")
			ticker := time.NewTicker(4 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-typingDone:
					return
				case <-ticker.C:
					_ = s.sendChatAction(ctx, bot.BotToken, msg.Chat.ID, "typing")
				}
			}
		}()

		orchResult, err := s.runOrchestratorForUserWithHistory(ctx, "telegram-worker", userID, convID, prompt)
		close(typingDone)

		reply := orchResult.Output
		if err != nil {
			reply = fmt.Sprintf("Error: %v", err)
		}
		if reply == "" {
			reply = "(no reply)"
		}

		if sendErr := s.sendTelegramFormattedReply(ctx, bot.BotToken, msg.Chat.ID, reply); sendErr != nil {
			s.wlog("telegram-worker", "[telegram-worker] failed to send reply to chat %s: %v", chatIDStr, sendErr)
		}

		// Persist the turn to the DB so history is available next time
		if convID != "" {
			if _, dbErr := s.createMessage(ctx, userID, convID, "user", rawPrompt, nil); dbErr != nil {
				s.wlog("telegram-worker", "[telegram-worker] failed to persist user message: %v", dbErr)
			}
			if _, dbErr := s.createMessage(ctx, userID, convID, "assistant", reply, nil); dbErr != nil {
				s.wlog("telegram-worker", "[telegram-worker] failed to persist assistant message: %v", dbErr)
			}
			for _, tc := range orchResult.Trace.ToolCalls {
				outputStr := tc.Output
				errStr := ""
				if tc.Status == "error" {
					errStr = tc.Output
					outputStr = ""
				}
				if saveErr := s.saveToolCall(ctx, userID, convID, tc.Name, tc.Arguments, outputStr, errStr, 0); saveErr != nil {
					s.wlog("telegram-worker", "[telegram-worker] failed to persist tool call %s: %v", tc.Name, saveErr)
				}
			}
		}
	}

	return lastID, nil
}

func (s *Server) sendTelegramMessage(ctx context.Context, botToken string, chatID int64, text string) error {
	return s.sendTelegramRaw(ctx, botToken, chatID, text, "")
}

// sendTelegramFormattedReply converts the LLM's markdown reply to Telegram HTML
// and sends it with parse_mode=HTML. On failure it retries as plain text.
func (s *Server) sendTelegramFormattedReply(ctx context.Context, botToken string, chatID int64, markdownText string) error {
	htmlText := markdownToTelegramHTML(markdownText)
	err := s.sendTelegramRaw(ctx, botToken, chatID, htmlText, "HTML")
	if err != nil {
		// Fallback: strip formatting and send as plain text
		return s.sendTelegramRaw(ctx, botToken, chatID, markdownText, "")
	}
	return nil
}

func (s *Server) sendTelegramRaw(ctx context.Context, botToken string, chatID int64, text, parseMode string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", botToken)
	payload := map[string]any{
		"chat_id": chatID,
		"text":    text,
	}
	if parseMode != "" {
		payload["parse_mode"] = parseMode
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("sendMessage HTTP %d", resp.StatusCode)
	}
	return nil
}

// setTelegramCommands registers the bot's command list with Telegram so they
// appear in the menu button. Called once per bot token.
func (s *Server) setTelegramCommands(ctx context.Context, botToken string) error {
	type tgBotCommand struct {
		Command     string `json:"command"`
		Description string `json:"description"`
	}
	payload := map[string]any{
		"commands": []tgBotCommand{
			{Command: "start", Description: "Start chatting with your AI assistant"},
			{Command: "new", Description: "Clear history and start a fresh conversation"},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("https://api.telegram.org/bot%s/setMyCommands", botToken)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("setMyCommands HTTP %d", resp.StatusCode)
	}
	return nil
}

// downloadTelegramFile fetches a file from the Telegram Bot API by file_id.
// Returns the file bytes and the file_path (which contains the extension).
func downloadTelegramFile(ctx context.Context, botToken, fileID string) ([]byte, string, error) {
	getFileURL := fmt.Sprintf("https://api.telegram.org/bot%s/getFile?file_id=%s", botToken, fileID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, getFileURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	var result struct {
		OK     bool `json:"ok"`
		Result struct {
			FilePath string `json:"file_path"`
		} `json:"result"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, "", fmt.Errorf("getFile decode: %w", err)
	}
	if !result.OK || result.Result.FilePath == "" {
		return nil, "", fmt.Errorf("telegram getFile failed")
	}

	dlURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", botToken, result.Result.FilePath)
	req2, err := http.NewRequestWithContext(ctx, http.MethodGet, dlURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return nil, "", err
	}
	defer resp2.Body.Close()

	data, err := io.ReadAll(resp2.Body)
	if err != nil {
		return nil, "", err
	}
	return data, result.Result.FilePath, nil
}

// sendChatAction sends a chat action (e.g. "typing") to a Telegram chat.
func (s *Server) sendChatAction(ctx context.Context, botToken string, chatID int64, action string) error {
	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendChatAction", botToken)
	payload := map[string]any{
		"chat_id": chatID,
		"action":  action,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

// TelegramSendNotification sends a text message to a bot's notification chat.
func (s *Server) TelegramSendNotification(ctx context.Context, botToken, notificationChatID, text string) error {
	if notificationChatID == "" {
		return nil
	}
	var chatID int64
	if _, err := fmt.Sscanf(notificationChatID, "%d", &chatID); err != nil {
		return fmt.Errorf("invalid notificationChatId %q: %w", notificationChatID, err)
	}
	return s.sendTelegramMessage(ctx, botToken, chatID, text)
}
