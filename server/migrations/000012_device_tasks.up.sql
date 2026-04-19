CREATE TABLE IF NOT EXISTS device_tasks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    target_device VARCHAR(255) NOT NULL,
    instruction TEXT NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'pending',
    result_output TEXT,
    created_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP WITH TIME ZONE NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMP WITH TIME ZONE
);

CREATE INDEX idx_device_tasks_user_target_status ON device_tasks(user_id, target_device, status);
