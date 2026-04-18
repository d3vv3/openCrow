ALTER TABLE user_memories RENAME COLUMN confidence TO strength;
DROP INDEX IF EXISTS idx_user_memories_user_confidence;
CREATE INDEX IF NOT EXISTS idx_user_memories_user_strength ON user_memories(user_id, strength DESC);
