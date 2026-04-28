ALTER TABLE device_registrations
    DROP COLUMN IF EXISTS push_endpoint,
    DROP COLUMN IF EXISTS push_auth,
    DROP COLUMN IF EXISTS push_public_key;
