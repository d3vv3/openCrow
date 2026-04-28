ALTER TABLE device_registrations
    ADD COLUMN IF NOT EXISTS push_endpoint   TEXT,
    ADD COLUMN IF NOT EXISTS push_auth       TEXT,
    ADD COLUMN IF NOT EXISTS push_public_key TEXT;
