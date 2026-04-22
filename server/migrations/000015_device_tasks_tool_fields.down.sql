ALTER TABLE device_tasks
    DROP COLUMN IF EXISTS tool_name,
    DROP COLUMN IF EXISTS tool_arguments;
