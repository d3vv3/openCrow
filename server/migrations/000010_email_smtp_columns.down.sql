ALTER TABLE email_inboxes
    DROP COLUMN IF EXISTS smtp_host,
    DROP COLUMN IF EXISTS smtp_port,
    DROP COLUMN IF EXISTS label;
