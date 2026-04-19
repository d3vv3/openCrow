// tools_email.go — Email setup, check, read, search, and compose tool implementations.
package api

import (
	"context"
	"fmt"
	"net/smtp"
	"strings"

	"github.com/opencrow/opencrow/server/internal/configstore"
)

// ── Email tools ──────────────────────────────────────────────────────────

func (s *Server) toolSetupEmail(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	address, _ := args["address"].(string)
	password, _ := args["password"].(string)
	address = strings.TrimSpace(strings.ToLower(address))
	if address == "" {
		return map[string]any{"success": false, "error": "address is required"}, nil
	}
	if password == "" {
		return map[string]any{"success": false, "error": "password (or app-specific password) is required"}, nil
	}

	// Auto-detect IMAP/SMTP settings from email domain
	imapHost, _ := args["imap_host"].(string)
	smtpHost, _ := args["smtp_host"].(string)
	imapPort := 993
	smtpPort := 587

	if imapHost == "" || smtpHost == "" {
		detected := autoDetectEmailServer(address)
		if detected != nil {
			if imapHost == "" {
				imapHost = detected.imapHost
			}
			if smtpHost == "" {
				smtpHost = detected.smtpHost
			}
			imapPort = detected.imapPort
			smtpPort = detected.smtpPort
		}
	}

	if imapHost == "" {
		return map[string]any{"success": false, "error": "Cannot auto-detect server settings. Please provide imap_host and smtp_host."}, nil
	}

	if p, ok := args["imap_port"].(float64); ok {
		imapPort = int(p)
	}
	if p, ok := args["smtp_port"].(float64); ok {
		smtpPort = int(p)
	}
	pollInterval := 900
	if p, ok := args["poll_interval_seconds"].(float64); ok {
		pollInterval = int(p)
	}

	// Save to config store
	if s.configStore != nil {
		cfg, err := s.configStore.GetUserConfig(userID)
		if err != nil {
			return map[string]any{"success": false, "error": "failed to load config"}, nil
		}

		account := configstore.EmailAccountConfig{
			Label:        address,
			Address:      address,
			ImapHost:     imapHost,
			ImapPort:     imapPort,
			ImapUsername: address,
			ImapPassword: password,
			SmtpHost:     smtpHost,
			SmtpPort:     smtpPort,
			UseTLS:       true,
			Enabled:      true,
			PollIntervalSeconds: pollInterval,
		}

		// Idempotent upsert by email address to avoid duplicates when
		// setup form already saved config and the model calls setup_email again.
		idx := -1
		for i, acc := range cfg.Integrations.EmailAccounts {
			if strings.EqualFold(strings.TrimSpace(acc.Address), address) {
				idx = i
				break
			}
		}
		if idx >= 0 {
			cfg.Integrations.EmailAccounts[idx] = account
		} else {
			cfg.Integrations.EmailAccounts = append(cfg.Integrations.EmailAccounts, account)
		}

		if _, err := s.configStore.PutUserConfig(userID, cfg); err != nil {
			return map[string]any{"success": false, "error": "failed to save config"}, nil
		}

		// Keep DB inbox rows in sync using upsert semantics as well.
		if err := s.syncEmailInboxesFromConfig(ctx, userID, cfg.Integrations.EmailAccounts); err != nil {
			return map[string]any{"success": false, "error": fmt.Sprintf("failed to sync inboxes: %v", err)}, nil
		}

		return map[string]any{
			"success":   true,
			"address":   address,
			"imap_host": imapHost,
			"smtp_host": smtpHost,
			"message":   "Email account configured (upserted). You can now use check_email, read_email, reply_email, compose_email, and search_email.",
		}, nil
	}

	// Fallback path: save directly to DB when config store is unavailable.
	inbox, err := s.createEmailInbox(ctx, userID, address, imapHost, imapPort, address, password, true, pollInterval)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			return map[string]any{"success": false, "error": "email account already configured"}, nil
		}
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to save: %v", err)}, nil
	}

	return map[string]any{
		"success":   true,
		"inbox_id":  inbox.ID,
		"address":   address,
		"imap_host": imapHost,
		"smtp_host": smtpHost,
		"message":   "Email account configured. You can now use check_email, read_email, reply_email, compose_email, and search_email.",
	}, nil
}

type emailServerInfo struct {
	imapHost string
	imapPort int
	smtpHost string
	smtpPort int
}

func autoDetectEmailServer(email string) *emailServerInfo {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 {
		return nil
	}
	domain := strings.ToLower(parts[1])

	switch { //nolint:staticcheck // complex multi-value cases can't use tagged switch
	case domain == "gmail.com" || domain == "googlemail.com":
		return &emailServerInfo{"imap.gmail.com", 993, "smtp.gmail.com", 587}
	case domain == "outlook.com" || domain == "hotmail.com" || domain == "live.com":
		return &emailServerInfo{"outlook.office365.com", 993, "smtp.office365.com", 587}
	case domain == "yahoo.com" || domain == "ymail.com":
		return &emailServerInfo{"imap.mail.yahoo.com", 993, "smtp.mail.yahoo.com", 587}
	case domain == "icloud.com" || domain == "me.com" || domain == "mac.com":
		return &emailServerInfo{"imap.mail.me.com", 993, "smtp.mail.me.com", 587}
	case domain == "aol.com":
		return &emailServerInfo{"imap.aol.com", 993, "smtp.aol.com", 587}
	case domain == "protonmail.com" || domain == "proton.me" || domain == "pm.me":
		return &emailServerInfo{"127.0.0.1", 1143, "127.0.0.1", 1025} // ProtonMail Bridge
	default:
		// Generic: try imap.<domain> / smtp.<domain>
		return &emailServerInfo{"imap." + domain, 993, "smtp." + domain, 587}
	}
}

func (s *Server) toolCheckEmail(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	creds, err := s.getFirstEmailCredentials(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": "No active email accounts configured. Use setup_email first."}, nil
	}

	sess, err := dialIMAP(ctx, creds.ImapHost, creds.ImapPort, creds.ImapUsername, creds.ImapPassword, creds.UseTLS)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("IMAP connect failed: %v", err)}, nil
	}
	defer sess.close()

	count, err := sess.selectMailbox("INBOX")
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("SELECT INBOX failed: %v", err)}, nil
	}

	// Fetch last 10 message headers
	start := count - 9
	if start < 1 {
		start = 1
	}
	var messages []map[string]any
	if count > 0 {
		headers, herr := sess.fetchHeaders(start, count)
		if herr == nil {
			for i := len(headers) - 1; i >= 0; i-- {
				h := headers[i]
				messages = append(messages, map[string]any{
					"seq":     h.SeqNum,
					"subject": h.Subject,
					"from":    h.From,
					"date":    h.Date,
					"flags":   h.Flags,
				})
			}
		}
	}

	return map[string]any{
		"success":        true,
		"inbox":          creds.Address,
		"total_messages": count,
		"recent":         messages,
	}, nil
}

func (s *Server) toolReadEmail(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	seqStr, _ := args["messageId"].(string)
	if seqStr == "" {
		return map[string]any{"success": false, "error": "messageId (sequence number) is required"}, nil
	}
	seq := 0
	if n, err := fmt.Sscanf(seqStr, "%d", &seq); n != 1 || err != nil {
		return map[string]any{"success": false, "error": "messageId must be a numeric sequence number"}, nil
	}

	creds, err := s.getFirstEmailCredentials(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": "No active email accounts configured."}, nil
	}

	sess, err := dialIMAP(ctx, creds.ImapHost, creds.ImapPort, creds.ImapUsername, creds.ImapPassword, creds.UseTLS)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("IMAP connect failed: %v", err)}, nil
	}
	defer sess.close()

	if _, err := sess.selectMailbox("INBOX"); err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("SELECT INBOX failed: %v", err)}, nil
	}

	body, err := sess.fetchBody(seq, 8000)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("FETCH failed: %v", err)}, nil
	}

	return map[string]any{
		"success": true,
		"seq":     seq,
		"body":    body,
	}, nil
}

func (s *Server) toolSearchEmail(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return map[string]any{"success": false, "error": "query is required"}, nil
	}

	creds, err := s.getFirstEmailCredentials(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": "No active email accounts configured."}, nil
	}

	sess, err := dialIMAP(ctx, creds.ImapHost, creds.ImapPort, creds.ImapUsername, creds.ImapPassword, creds.UseTLS)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("IMAP connect failed: %v", err)}, nil
	}
	defer sess.close()

	if _, err := sess.selectMailbox("INBOX"); err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("SELECT INBOX failed: %v", err)}, nil
	}

	// Build IMAP search criteria: search subject OR body text
	criteria := fmt.Sprintf("OR SUBJECT %s TEXT %s", imapQuote(query), imapQuote(query))
	seqNums, err := sess.search(criteria)
	if err != nil {
		// Fallback to simpler subject-only search
		seqNums, err = sess.search("SUBJECT " + imapQuote(query))
		if err != nil {
			return map[string]any{"success": false, "error": fmt.Sprintf("SEARCH failed: %v", err)}, nil
		}
	}

	if len(seqNums) == 0 {
		return map[string]any{"success": true, "query": query, "count": 0, "results": []any{}}, nil
	}

	// Fetch headers for the last 10 matches
	limit := seqNums
	if len(limit) > 10 {
		limit = limit[len(limit)-10:]
	}
	headers, err := sess.fetchHeaders(limit[0], limit[len(limit)-1])
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("FETCH headers failed: %v", err)}, nil
	}

	results := make([]map[string]any, 0, len(headers))
	matchSet := make(map[int]bool, len(limit))
	for _, n := range limit {
		matchSet[n] = true
	}
	for _, h := range headers {
		if matchSet[h.SeqNum] {
			results = append(results, map[string]any{
				"seq":     h.SeqNum,
				"subject": h.Subject,
				"from":    h.From,
				"date":    h.Date,
			})
		}
	}

	return map[string]any{
		"success": true,
		"query":   query,
		"count":   len(seqNums),
		"results": results,
	}, nil
}

func (s *Server) toolReplyEmail(_ context.Context, _ string, _ map[string]any) (map[string]any, error) {
	return map[string]any{"success": false, "error": "SMTP reply is not yet implemented."}, nil
}

func (s *Server) toolComposeEmail(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	to, _ := args["to"].(string)
	subject, _ := args["subject"].(string)
	body, _ := args["body"].(string)
	if to == "" || subject == "" || body == "" {
		return map[string]any{"success": false, "error": "to, subject, and body are required"}, nil
	}

	creds, err := s.getFirstEmailCredentials(ctx, userID)
	if err != nil {
		return map[string]any{"success": false, "error": "No active email accounts configured. Use setup_email first."}, nil
	}

	smtpHost := creds.SmtpHost
	smtpPort := creds.SmtpPort
	// Fallback: auto-detect from domain
	if smtpHost == "" {
		if info := autoDetectEmailServer(creds.Address); info != nil {
			smtpHost = info.smtpHost
			smtpPort = info.smtpPort
		}
	}
	if smtpHost == "" {
		return map[string]any{"success": false, "error": "Cannot determine SMTP server. Please reconfigure the email account with smtp_host."}, nil
	}

	from := creds.Address
	msgBytes := []byte(fmt.Sprintf(
		"From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/plain; charset=UTF-8\r\n\r\n%s",
		from, to, subject, body,
	))

	addr := fmt.Sprintf("%s:%d", smtpHost, smtpPort)
	auth := smtp.PlainAuth("", creds.ImapUsername, creds.ImapPassword, smtpHost)

	if err := smtp.SendMail(addr, auth, from, []string{to}, msgBytes); err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("SMTP send failed: %v", err)}, nil
	}

	return map[string]any{
		"success": true,
		"from":    from,
		"to":      to,
		"subject": subject,
		"message": "Email sent successfully.",
	}, nil
}
