ALTER TABLE users RENAME COLUMN email TO username;

DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'users_email_key'
    ) THEN
        ALTER TABLE users RENAME CONSTRAINT users_email_key TO users_username_key;
    END IF;
END $$;
