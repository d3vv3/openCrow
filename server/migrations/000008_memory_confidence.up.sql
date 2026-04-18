ALTER TABLE user_memories RENAME COLUMN strength TO confidence;
DROP INDEX IF EXISTS idx_user_memories_user_strength;
CREATE INDEX IF NOT EXISTS idx_user_memories_user_confidence ON user_memories(user_id, confidence DESC);
