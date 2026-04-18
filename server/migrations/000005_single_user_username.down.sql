DO $$
BEGIN
    IF EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'users_username_key'
    ) THEN
        ALTER TABLE users RENAME CONSTRAINT users_username_key TO users_email_key;
    END IF;
END $$;

ALTER TABLE users RENAME COLUMN username TO email;
