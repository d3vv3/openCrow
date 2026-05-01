-- Memory graph: flexible nodes and edges for user memory

CREATE TABLE IF NOT EXISTS memory_entities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    type TEXT NOT NULL DEFAULT 'thing',   -- person, place, language, trip, food, etc.
    name TEXT NOT NULL,
    summary TEXT NOT NULL DEFAULT '',
    search_vector TSVECTOR,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_memory_entities_user_id ON memory_entities(user_id);
CREATE INDEX IF NOT EXISTS idx_memory_entities_search ON memory_entities USING GIN(search_vector);
CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_entities_user_name ON memory_entities(user_id, lower(name));

CREATE TABLE IF NOT EXISTS memory_relations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    from_entity UUID NOT NULL REFERENCES memory_entities(id) ON DELETE CASCADE,
    to_entity UUID NOT NULL REFERENCES memory_entities(id) ON DELETE CASCADE,
    relation TEXT NOT NULL,              -- "speaks", "visited in 2023", "is allergic to"
    confidence FLOAT NOT NULL DEFAULT 0.6,
    reinforcement_count INT NOT NULL DEFAULT 1,
    source_conversation_id UUID REFERENCES conversations(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_memory_relations_user_id ON memory_relations(user_id);
CREATE INDEX IF NOT EXISTS idx_memory_relations_from ON memory_relations(from_entity);
CREATE INDEX IF NOT EXISTS idx_memory_relations_to ON memory_relations(to_entity);
CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_relations_unique ON memory_relations(from_entity, to_entity, relation);

CREATE TABLE IF NOT EXISTS memory_observations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entity_id UUID REFERENCES memory_entities(id) ON DELETE SET NULL,
    content TEXT NOT NULL,
    search_vector TSVECTOR,
    conversation_id UUID REFERENCES conversations(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_memory_observations_user_id ON memory_observations(user_id);
CREATE INDEX IF NOT EXISTS idx_memory_observations_entity ON memory_observations(entity_id);
CREATE INDEX IF NOT EXISTS idx_memory_observations_search ON memory_observations USING GIN(search_vector);

-- Auto-update search vectors
CREATE OR REPLACE FUNCTION memory_entities_search_update() RETURNS trigger AS $$
BEGIN
    NEW.search_vector := to_tsvector('english', coalesce(NEW.name, '') || ' ' || coalesce(NEW.summary, '') || ' ' || coalesce(NEW.type, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER memory_entities_search_trigger
    BEFORE INSERT OR UPDATE ON memory_entities
    FOR EACH ROW EXECUTE FUNCTION memory_entities_search_update();

CREATE OR REPLACE FUNCTION memory_observations_search_update() RETURNS trigger AS $$
BEGIN
    NEW.search_vector := to_tsvector('english', coalesce(NEW.content, ''));
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER memory_observations_search_trigger
    BEFORE INSERT OR UPDATE ON memory_observations
    FOR EACH ROW EXECUTE FUNCTION memory_observations_search_update();
