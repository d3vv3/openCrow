ALTER TABLE device_tasks
    ADD COLUMN IF NOT EXISTS tool_name      VARCHAR(255),
    ADD COLUMN IF NOT EXISTS tool_arguments JSONB;
