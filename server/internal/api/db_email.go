// db_email.go — Email inbox and poll event persistence.
package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/opencrow/opencrow/server/internal/configstore"
)

func (s *Server) listEmailInboxes(ctx context.Context, userID string) ([]EmailInboxDTO, error) {
	const q = `
SELECT id::text, address, imap_host, imap_port, imap_username, use_tls, active, poll_interval_seconds, last_polled_at, updated_at, created_at
FROM email_inboxes
WHERE user_id = $1::uuid
ORDER BY created_at DESC;
`
	rows, err := s.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []EmailInboxDTO
	for rows.Next() {
		var dto EmailInboxDTO
		var lastPolledAt *time.Time
		var updatedAt, createdAt time.Time
		if err := rows.Scan(&dto.ID, &dto.Address, &dto.ImapHost, &dto.ImapPort, &dto.ImapUsername, &dto.UseTLS, &dto.Active, &dto.PollIntervalSeconds, &lastPolledAt, &updatedAt, &createdAt); err != nil {
			return nil, err
		}
		if lastPolledAt != nil {
			dto.LastPolledAt = lastPolledAt.UTC().Format(time.RFC3339)
		}
		dto.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
		dto.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		result = append(result, dto)
	}
	return result, rows.Err()
}

type emailInboxCredentials struct {
	ID           string
	Address      string
	Label        string
	ImapHost     string
	ImapPort     int
	ImapUsername string
	ImapPassword string
	SmtpHost     string
	SmtpPort     int
	UseTLS       bool
}

// getFirstEmailCredentials returns credentials for the first active inbox, for use in email tools.
func (s *Server) getFirstEmailCredentials(ctx context.Context, userID string) (*emailInboxCredentials, error) {
	const q = `
SELECT id::text, address, COALESCE(label,''), imap_host, imap_port, imap_username, imap_password, COALESCE(smtp_host,''), COALESCE(smtp_port,587), use_tls
FROM email_inboxes
WHERE user_id = $1::uuid AND active = true
ORDER BY created_at DESC
LIMIT 1;
`
	var c emailInboxCredentials
	err := s.db.QueryRow(ctx, q, userID).Scan(&c.ID, &c.Address, &c.Label, &c.ImapHost, &c.ImapPort, &c.ImapUsername, &c.ImapPassword, &c.SmtpHost, &c.SmtpPort, &c.UseTLS)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// syncEmailInboxesFromConfig upserts email accounts from the configstore into the email_inboxes DB table.
// This ensures accounts saved via the Config UI are visible to the email worker and LLM tools.
func (s *Server) syncEmailInboxesFromConfig(ctx context.Context, userID string, accounts []configstore.EmailAccountConfig) error {
	const q = `
INSERT INTO email_inboxes (user_id, address, label, imap_host, imap_port, imap_username, imap_password, smtp_host, smtp_port, use_tls, active, poll_interval_seconds, updated_at)
VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, NOW())
ON CONFLICT (user_id, address) DO UPDATE SET
    label         = EXCLUDED.label,
    imap_host     = EXCLUDED.imap_host,
    imap_port     = EXCLUDED.imap_port,
    imap_username = EXCLUDED.imap_username,
    imap_password = EXCLUDED.imap_password,
    smtp_host     = EXCLUDED.smtp_host,
    smtp_port     = EXCLUDED.smtp_port,
    use_tls       = EXCLUDED.use_tls,
    active        = EXCLUDED.active,
    poll_interval_seconds = EXCLUDED.poll_interval_seconds,
    updated_at    = NOW();
`
	for _, acc := range accounts {
		imapPort := acc.ImapPort
		if imapPort <= 0 {
			imapPort = 993
		}
		smtpPort := acc.SmtpPort
		if smtpPort <= 0 {
			smtpPort = 587
		}
		pollInterval := acc.PollIntervalSeconds
		if pollInterval <= 0 {
			pollInterval = 900 // 15 minutes default
		}
		if _, err := s.db.Exec(ctx, q,
			userID, acc.Address, acc.Label, acc.ImapHost, imapPort,
			acc.ImapUsername, acc.ImapPassword,
			acc.SmtpHost, smtpPort, acc.UseTLS, acc.Enabled, pollInterval,
		); err != nil {
			return fmt.Errorf("upsert inbox %s: %w", acc.Address, err)
		}
	}
	return nil
}

func (s *Server) createEmailInbox(ctx context.Context, userID, address, imapHost string, imapPort int, imapUsername, imapPassword string, useTLS bool, pollInterval int) (EmailInboxDTO, error) {
	const q = `
INSERT INTO email_inboxes (user_id, address, imap_host, imap_port, imap_username, imap_password, use_tls, active, poll_interval_seconds, updated_at)
VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, TRUE, $8, NOW())
RETURNING id::text, address, imap_host, imap_port, imap_username, use_tls, active, poll_interval_seconds, last_polled_at, updated_at, created_at;
`

	var dto EmailInboxDTO
	var lastPolledAt *time.Time
	var updatedAt, createdAt time.Time
	err := s.db.QueryRow(ctx, q, userID, address, imapHost, imapPort, imapUsername, imapPassword, useTLS, pollInterval).Scan(
		&dto.ID,
		&dto.Address,
		&dto.ImapHost,
		&dto.ImapPort,
		&dto.ImapUsername,
		&dto.UseTLS,
		&dto.Active,
		&dto.PollIntervalSeconds,
		&lastPolledAt,
		&updatedAt,
		&createdAt,
	)
	if err != nil {
		return EmailInboxDTO{}, err
	}
	if lastPolledAt != nil {
		dto.LastPolledAt = lastPolledAt.UTC().Format(time.RFC3339)
	}
	dto.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	dto.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return dto, nil
}

func (s *Server) updateEmailInbox(ctx context.Context, userID, inboxID string, req UpdateEmailInboxRequest) (EmailInboxDTO, error) {
	const currentQ = `
SELECT id::text, address, imap_host, imap_port, imap_username, imap_password, use_tls, active, poll_interval_seconds, last_polled_at, updated_at, created_at
FROM email_inboxes
WHERE id = $1::uuid AND user_id = $2::uuid;
`

	var dto EmailInboxDTO
	var currentPassword string
	var lastPolledAt *time.Time
	var updatedAt, createdAt time.Time
	err := s.db.QueryRow(ctx, currentQ, inboxID, userID).Scan(
		&dto.ID,
		&dto.Address,
		&dto.ImapHost,
		&dto.ImapPort,
		&dto.ImapUsername,
		&currentPassword,
		&dto.UseTLS,
		&dto.Active,
		&dto.PollIntervalSeconds,
		&lastPolledAt,
		&updatedAt,
		&createdAt,
	)
	if err != nil {
		return EmailInboxDTO{}, err
	}

	imapHost := dto.ImapHost
	if req.ImapHost != nil {
		imapHost = strings.TrimSpace(*req.ImapHost)
	}
	imapPort := dto.ImapPort
	if req.ImapPort != nil {
		imapPort = *req.ImapPort
	}
	imapUsername := dto.ImapUsername
	if req.ImapUsername != nil {
		imapUsername = strings.TrimSpace(*req.ImapUsername)
	}
	imapPassword := currentPassword
	if req.ImapPassword != nil {
		imapPassword = *req.ImapPassword
	}
	useTLS := dto.UseTLS
	if req.UseTLS != nil {
		useTLS = *req.UseTLS
	}
	active := dto.Active
	if req.Active != nil {
		active = *req.Active
	}
	pollInterval := dto.PollIntervalSeconds
	if req.PollIntervalSeconds != nil {
		pollInterval = *req.PollIntervalSeconds
	}

	const updateQ = `
UPDATE email_inboxes
SET imap_host = $3,
    imap_port = $4,
    imap_username = $5,
    imap_password = $6,
    use_tls = $7,
    active = $8,
    poll_interval_seconds = $9,
    updated_at = NOW()
WHERE id = $1::uuid AND user_id = $2::uuid
RETURNING id::text, address, imap_host, imap_port, imap_username, use_tls, active, poll_interval_seconds, last_polled_at, updated_at, created_at;
`

	var newLastPolledAt *time.Time
	err = s.db.QueryRow(ctx, updateQ, inboxID, userID, imapHost, imapPort, imapUsername, imapPassword, useTLS, active, pollInterval).Scan(
		&dto.ID,
		&dto.Address,
		&dto.ImapHost,
		&dto.ImapPort,
		&dto.ImapUsername,
		&dto.UseTLS,
		&dto.Active,
		&dto.PollIntervalSeconds,
		&newLastPolledAt,
		&updatedAt,
		&createdAt,
	)
	if err != nil {
		return EmailInboxDTO{}, err
	}
	if newLastPolledAt != nil {
		dto.LastPolledAt = newLastPolledAt.UTC().Format(time.RFC3339)
	}
	dto.UpdatedAt = updatedAt.UTC().Format(time.RFC3339)
	dto.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return dto, nil
}

func (s *Server) deleteEmailInbox(ctx context.Context, userID, inboxID string) (bool, error) {
	const q = `DELETE FROM email_inboxes WHERE id = $1::uuid AND user_id = $2::uuid;`
	cmd, err := s.db.Exec(ctx, q, inboxID, userID)
	if err != nil {
		return false, err
	}
	return cmd.RowsAffected() > 0, nil
}

func (s *Server) createEmailPollEvent(ctx context.Context, userID, inboxID, status, detail string) (EmailPollEventDTO, error) {
	const ownershipQ = `SELECT 1 FROM email_inboxes WHERE id = $1::uuid AND user_id = $2::uuid;`
	var exists int
	if err := s.db.QueryRow(ctx, ownershipQ, inboxID, userID).Scan(&exists); err != nil {
		return EmailPollEventDTO{}, err
	}

	const q = `
WITH inserted AS (
  INSERT INTO email_poll_events (user_id, inbox_id, status, detail)
  VALUES ($1::uuid, $2::uuid, $3, $4)
  RETURNING id::text, inbox_id::text, status, COALESCE(detail, '') AS detail, created_at
), touched AS (
  UPDATE email_inboxes
  SET last_polled_at = NOW(), updated_at = NOW()
  WHERE id = $2::uuid AND user_id = $1::uuid
)
SELECT id, inbox_id, status, detail, created_at FROM inserted;
`

	var dto EmailPollEventDTO
	var createdAt time.Time
	if err := s.db.QueryRow(ctx, q, userID, inboxID, status, detail).Scan(&dto.ID, &dto.InboxID, &dto.Status, &dto.Detail, &createdAt); err != nil {
		return EmailPollEventDTO{}, err
	}
	dto.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return dto, nil
}

func (s *Server) listEmailPollEvents(ctx context.Context, userID string) ([]EmailPollEventDTO, error) {
	const q = `
SELECT id::text, inbox_id::text, status, COALESCE(detail, ''), created_at
FROM email_poll_events
WHERE user_id = $1::uuid
ORDER BY created_at DESC
LIMIT 200;
`
	rows, err := s.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []EmailPollEventDTO
	for rows.Next() {
		var dto EmailPollEventDTO
		var createdAt time.Time
		if err := rows.Scan(&dto.ID, &dto.InboxID, &dto.Status, &dto.Detail, &createdAt); err != nil {
			return nil, err
		}
		dto.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		result = append(result, dto)
	}
	return result, rows.Err()
}
