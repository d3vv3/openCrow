// db_memory_graph.go -- Memory graph persistence: entities, relations, observations.
package api

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ---- Types ----

type MemoryEntity struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Type      string    `json:"type"`
	Name      string    `json:"name"`
	Summary   string    `json:"summary"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type MemoryRelation struct {
	ID                   string    `json:"id"`
	UserID               string    `json:"user_id"`
	FromEntityID         string    `json:"from_entity_id"`
	FromEntityName       string    `json:"from_entity_name,omitempty"`
	ToEntityID           string    `json:"to_entity_id"`
	ToEntityName         string    `json:"to_entity_name,omitempty"`
	Relation             string    `json:"relation"`
	Confidence           float64   `json:"confidence"`
	ReinforcementCount   int       `json:"reinforcement_count"`
	SourceConversationID *string   `json:"source_conversation_id,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type MemoryObservation struct {
	ID             string    `json:"id"`
	UserID         string    `json:"user_id"`
	EntityID       *string   `json:"entity_id,omitempty"`
	EntityName     string    `json:"entity_name,omitempty"`
	Content        string    `json:"content"`
	ConversationID *string   `json:"conversation_id,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}

// ---- Entities ----

// upsertMemoryEntity creates or updates an entity by (user_id, lower(name)).
// Returns the entity ID.
func (s *Server) upsertMemoryEntity(ctx context.Context, userID, entityType, name, summary string) (string, error) {
	const q = `
INSERT INTO memory_entities (user_id, type, name, summary)
VALUES ($1::uuid, $2, $3, $4)
ON CONFLICT (user_id, lower(name))
DO UPDATE SET
    type    = CASE WHEN EXCLUDED.type != '' AND EXCLUDED.type != 'thing' THEN EXCLUDED.type ELSE memory_entities.type END,
    summary = CASE WHEN EXCLUDED.summary != '' THEN EXCLUDED.summary ELSE memory_entities.summary END,
    updated_at = NOW()
RETURNING id::text;
`
	var id string
	err := s.db.QueryRow(ctx, q, userID, entityType, name, summary).Scan(&id)
	return id, err
}

func (s *Server) getMemoryEntity(ctx context.Context, userID, entityID string) (*MemoryEntity, error) {
	const q = `
SELECT id::text, user_id::text, type, name, summary, created_at, updated_at
FROM memory_entities WHERE id = $1::uuid AND user_id = $2::uuid;
`
	var e MemoryEntity
	err := s.db.QueryRow(ctx, q, entityID, userID).Scan(
		&e.ID, &e.UserID, &e.Type, &e.Name, &e.Summary, &e.CreatedAt, &e.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &e, nil
}

func (s *Server) listMemoryEntities(ctx context.Context, userID string) ([]MemoryEntity, error) {
	const q = `
SELECT id::text, user_id::text, type, name, summary, created_at, updated_at
FROM memory_entities WHERE user_id = $1::uuid ORDER BY updated_at DESC;
`
	rows, err := s.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MemoryEntity
	for rows.Next() {
		var e MemoryEntity
		if err := rows.Scan(&e.ID, &e.UserID, &e.Type, &e.Name, &e.Summary, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

func (s *Server) deleteMemoryEntity(ctx context.Context, userID, entityID string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM memory_entities WHERE id = $1::uuid AND user_id = $2::uuid`,
		entityID, userID,
	)
	return err
}

// updateMemoryEntity updates the name, type, and/or summary of an existing entity.
func (s *Server) updateMemoryEntity(ctx context.Context, userID, entityID, entityType, name, summary string) error {
	_, err := s.db.Exec(ctx,
		`UPDATE memory_entities SET type = $3, name = $4, summary = $5, updated_at = NOW()
		 WHERE id = $1::uuid AND user_id = $2::uuid`,
		entityID, userID, entityType, name, summary,
	)
	return err
}

// searchMemoryEntities does full-text search across entities + observations.
// Returns matching entities and a snippet of relevant observations.
func (s *Server) searchMemoryEntities(ctx context.Context, userID, query string) ([]MemoryEntity, error) {
	const q = `
SELECT id::text, user_id::text, type, name, summary, created_at, updated_at
FROM memory_entities
WHERE user_id = $1::uuid
  AND search_vector @@ plainto_tsquery('english', $2)
ORDER BY ts_rank(search_vector, plainto_tsquery('english', $2)) DESC
LIMIT 20;
`
	rows, err := s.db.Query(ctx, q, userID, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MemoryEntity
	for rows.Next() {
		var e MemoryEntity
		if err := rows.Scan(&e.ID, &e.UserID, &e.Type, &e.Name, &e.Summary, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, err
		}
		result = append(result, e)
	}
	return result, rows.Err()
}

// searchMemoryObservations does full-text search on observations, returns entity context.
func (s *Server) searchMemoryObservations(ctx context.Context, userID, query string) ([]MemoryObservation, error) {
	const q = `
SELECT o.id::text, o.user_id::text, o.entity_id::text, COALESCE(e.name, ''), o.content, o.conversation_id::text, o.created_at
FROM memory_observations o
LEFT JOIN memory_entities e ON e.id = o.entity_id
WHERE o.user_id = $1::uuid
  AND o.search_vector @@ plainto_tsquery('english', $2)
ORDER BY ts_rank(o.search_vector, plainto_tsquery('english', $2)) DESC
LIMIT 20;
`
	rows, err := s.db.Query(ctx, q, userID, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MemoryObservation
	for rows.Next() {
		var o MemoryObservation
		var entityID, convID *string
		if err := rows.Scan(&o.ID, &o.UserID, &entityID, &o.EntityName, &o.Content, &convID, &o.CreatedAt); err != nil {
			return nil, err
		}
		o.EntityID = entityID
		o.ConversationID = convID
		result = append(result, o)
	}
	return result, rows.Err()
}

// ---- Relations ----

// upsertMemoryRelation creates or reinforces a relation between two entities.
func (s *Server) upsertMemoryRelation(ctx context.Context, userID, fromEntityID, toEntityID, relation, conversationID string) (string, error) {
	const q = `
INSERT INTO memory_relations (user_id, from_entity, to_entity, relation, source_conversation_id)
VALUES ($1::uuid, $2::uuid, $3::uuid, $4, $5::uuid)
ON CONFLICT (from_entity, to_entity, relation)
DO UPDATE SET
    confidence          = LEAST(1.0, memory_relations.confidence + 0.1),
    reinforcement_count = memory_relations.reinforcement_count + 1,
    updated_at          = NOW()
RETURNING id::text;
`
	var convPtr *string
	if conversationID != "" {
		convPtr = &conversationID
	}
	var id string
	err := s.db.QueryRow(ctx, q, userID, fromEntityID, toEntityID, relation, convPtr).Scan(&id)
	return id, err
}

func (s *Server) listMemoryRelations(ctx context.Context, userID string) ([]MemoryRelation, error) {
	const q = `
SELECT r.id::text, r.user_id::text, r.from_entity::text, fe.name, r.to_entity::text, te.name,
       r.relation, r.confidence, r.reinforcement_count, r.source_conversation_id::text,
       r.created_at, r.updated_at
FROM memory_relations r
JOIN memory_entities fe ON fe.id = r.from_entity
JOIN memory_entities te ON te.id = r.to_entity
WHERE r.user_id = $1::uuid
ORDER BY r.updated_at DESC;
`
	return s.scanRelations(ctx, q, userID)
}

func (s *Server) listRelationsForEntity(ctx context.Context, userID, entityID string) ([]MemoryRelation, error) {
	const q = `
SELECT r.id::text, r.user_id::text, r.from_entity::text, fe.name, r.to_entity::text, te.name,
       r.relation, r.confidence, r.reinforcement_count, r.source_conversation_id::text,
       r.created_at, r.updated_at
FROM memory_relations r
JOIN memory_entities fe ON fe.id = r.from_entity
JOIN memory_entities te ON te.id = r.to_entity
WHERE r.user_id = $1::uuid AND (r.from_entity = $2::uuid OR r.to_entity = $2::uuid)
ORDER BY r.confidence DESC;
`
	return s.scanRelations(ctx, q, userID, entityID)
}

func (s *Server) scanRelations(ctx context.Context, q string, args ...any) ([]MemoryRelation, error) {
	rows, err := s.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []MemoryRelation
	for rows.Next() {
		var r MemoryRelation
		var convID *string
		if err := rows.Scan(
			&r.ID, &r.UserID, &r.FromEntityID, &r.FromEntityName, &r.ToEntityID, &r.ToEntityName,
			&r.Relation, &r.Confidence, &r.ReinforcementCount, &convID,
			&r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		r.SourceConversationID = convID
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *Server) deleteMemoryRelation(ctx context.Context, userID, relationID string) error {
	_, err := s.db.Exec(ctx,
		`DELETE FROM memory_relations WHERE id = $1::uuid AND user_id = $2::uuid`,
		relationID, userID,
	)
	return err
}

// ---- Observations ----

func (s *Server) addMemoryObservation(ctx context.Context, userID string, entityID *string, content, conversationID string) (string, error) {
	const q = `
INSERT INTO memory_observations (user_id, entity_id, content, conversation_id)
VALUES ($1::uuid, $2::uuid, $3, $4::uuid)
RETURNING id::text;
`
	var convPtr *string
	if conversationID != "" {
		convPtr = &conversationID
	}
	var id string
	err := s.db.QueryRow(ctx, q, userID, entityID, content, convPtr).Scan(&id)
	return id, err
}

func (s *Server) listObservationsForEntity(ctx context.Context, userID, entityID string) ([]MemoryObservation, error) {
	const q = `
SELECT o.id::text, o.user_id::text, o.entity_id::text, COALESCE(e.name, ''), o.content, o.conversation_id::text, o.created_at
FROM memory_observations o
LEFT JOIN memory_entities e ON e.id = o.entity_id
WHERE o.user_id = $1::uuid AND o.entity_id = $2::uuid
ORDER BY o.created_at DESC LIMIT 50;
`
	rows, err := s.db.Query(ctx, q, userID, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return s.scanObservations(rows)
}

func (s *Server) scanObservations(rows interface{ Next() bool; Scan(...any) error; Err() error }) ([]MemoryObservation, error) {
	var result []MemoryObservation
	for rows.Next() {
		var o MemoryObservation
		var entityID, convID *string
		if err := rows.Scan(&o.ID, &o.UserID, &entityID, &o.EntityName, &o.Content, &convID, &o.CreatedAt); err != nil {
			return nil, err
		}
		o.EntityID = entityID
		o.ConversationID = convID
		result = append(result, o)
	}
	return result, rows.Err()
}

// ---- Graph walk ----

// getMemoryContext builds a compact memory context string for injection into the system prompt.
// It searches entities + observations for the query, then fetches 1-hop relations.
func (s *Server) getMemoryContext(ctx context.Context, userID, query string) (string, error) {
	entities, err := s.searchMemoryEntities(ctx, userID, query)
	if err != nil {
		return "", fmt.Errorf("search entities: %w", err)
	}
	observations, err := s.searchMemoryObservations(ctx, userID, query)
	if err != nil {
		return "", fmt.Errorf("search observations: %w", err)
	}

	if len(entities) == 0 && len(observations) == 0 {
		return "", nil
	}

	var sb strings.Builder
	sb.WriteString("## Relevant memories\n")

	seen := map[string]bool{}
	for _, e := range entities {
		if seen[e.ID] {
			continue
		}
		seen[e.ID] = true
		sb.WriteString(fmt.Sprintf("- [%s] **%s**: %s\n", e.Type, e.Name, e.Summary))
		// 1-hop relations
		rels, _ := s.listRelationsForEntity(ctx, userID, e.ID)
		for _, r := range rels {
			if r.Confidence < 0.3 {
				continue
			}
			sb.WriteString(fmt.Sprintf("  - %s --%s--> %s (confidence: %.0f%%)\n",
				r.FromEntityName, r.Relation, r.ToEntityName, r.Confidence*100))
		}
	}

	if len(observations) > 0 {
		sb.WriteString("\n### Observations\n")
		for i, o := range observations {
			if i >= 5 {
				break
			}
			sb.WriteString(fmt.Sprintf("- %s\n", o.Content))
		}
	}

	return sb.String(), nil
}

// getFullMemoryContext returns ALL entities and relations for full graph (used by web UI).
type MemoryGraph struct {
	Entities     []MemoryEntity     `json:"entities"`
	Relations    []MemoryRelation   `json:"relations"`
	Observations []MemoryObservation `json:"observations"`
}

func (s *Server) getFullMemoryGraph(ctx context.Context, userID string) (*MemoryGraph, error) {
	entities, err := s.listMemoryEntities(ctx, userID)
	if err != nil {
		return nil, err
	}
	relations, err := s.listMemoryRelations(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &MemoryGraph{
		Entities:  entities,
		Relations: relations,
	}, nil
}
