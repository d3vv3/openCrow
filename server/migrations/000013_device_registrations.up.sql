CREATE TABLE IF NOT EXISTS device_registrations (
    user_id    UUID NOT NULL,
    device_id  TEXT NOT NULL,
    capabilities JSONB NOT NULL DEFAULT '[]',
    last_seen_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, device_id)
);
