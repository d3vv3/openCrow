CREATE TABLE IF NOT EXISTS tool_calls (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    conversation_id UUID NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    tool_name       TEXT NOT NULL,
    arguments       JSONB,
    output          TEXT,
    error           TEXT,
    duration_ms     BIGINT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tool_calls_conversation_id ON tool_calls(conversation_id);
