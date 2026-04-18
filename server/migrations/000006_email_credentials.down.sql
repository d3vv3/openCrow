ALTER TABLE email_inboxes
    DROP COLUMN IF EXISTS imap_username,
    DROP COLUMN IF EXISTS imap_password;
