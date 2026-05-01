// tools_memory.go -- Memory Graph tools and background extraction.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/opencrow/opencrow/server/internal/configstore"
	"github.com/opencrow/opencrow/server/internal/orchestrator"
)

// ── Memory Graph tools ─────────────────────────────────────────────────────

func (s *Server) toolRememberEntity(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	name, _ := args["name"].(string)
	entityType, _ := args["type"].(string)
	summary, _ := args["summary"].(string)
	if name == "" {
		return map[string]any{"success": false, "error": "name is required"}, nil
	}
	if entityType == "" {
		entityType = "thing"
	}
	id, err := s.upsertMemoryEntity(ctx, userID, entityType, name, summary)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to remember entity: %v", err)}, nil
	}
	// Also add an observation if summary provided
	if summary != "" {
		_, _ = s.addMemoryObservation(ctx, userID, &id, summary, conversationIDFromContext(ctx))
	}
	return map[string]any{
		"success":   true,
		"entity_id": id,
		"name":      name,
		"type":      entityType,
		"message":   fmt.Sprintf("Entity '%s' saved", name),
	}, nil
}

func (s *Server) toolRelateEntities(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	fromName, _ := args["from_name"].(string)
	toName, _ := args["to_name"].(string)
	relation, _ := args["relation"].(string)
	if fromName == "" || toName == "" || relation == "" {
		return map[string]any{"success": false, "error": "from_name, to_name, and relation are required"}, nil
	}

	// Upsert both entities (type "thing" if unknown)
	fromID, err := s.upsertMemoryEntity(ctx, userID, "thing", fromName, "")
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to upsert from_entity: %v", err)}, nil
	}
	toID, err := s.upsertMemoryEntity(ctx, userID, "thing", toName, "")
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to upsert to_entity: %v", err)}, nil
	}

	convID := conversationIDFromContext(ctx)
	relID, err := s.upsertMemoryRelation(ctx, userID, fromID, toID, relation, convID)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to relate entities: %v", err)}, nil
	}
	return map[string]any{
		"success":     true,
		"relation_id": relID,
		"message":     fmt.Sprintf("'%s' --%s--> '%s'", fromName, relation, toName),
	}, nil
}

func (s *Server) toolSearchMemory(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	query, _ := args["query"].(string)
	if query == "" {
		return map[string]any{"success": false, "error": "query is required"}, nil
	}

	entities, err := s.searchMemoryEntities(ctx, userID, query)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("search failed: %v", err)}, nil
	}
	observations, err := s.searchMemoryObservations(ctx, userID, query)
	if err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("search failed: %v", err)}, nil
	}

	entityResults := make([]map[string]any, 0, len(entities))
	for _, e := range entities {
		rels, _ := s.listRelationsForEntity(ctx, userID, e.ID)
		relList := make([]string, 0, len(rels))
		for _, r := range rels {
			if r.Confidence >= 0.3 {
				relList = append(relList, fmt.Sprintf("%s --%s--> %s (%.0f%%)", r.FromEntityName, r.Relation, r.ToEntityName, r.Confidence*100))
			}
		}
		entityResults = append(entityResults, map[string]any{
			"id":            e.ID,
			"type":          e.Type,
			"name":          e.Name,
			"summary":       e.Summary,
			"relations":     relList,
		})
	}

	obsResults := make([]map[string]any, 0, len(observations))
	for _, o := range observations {
		obsResults = append(obsResults, map[string]any{
			"entity":  o.EntityName,
			"content": o.Content,
		})
	}

	return map[string]any{
		"success":      true,
		"entities":     entityResults,
		"observations": obsResults,
	}, nil
}

func (s *Server) toolForgetEntity(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	entityID, _ := args["entity_id"].(string)
	if entityID == "" {
		return map[string]any{"success": false, "error": "entity_id is required"}, nil
	}
	if err := s.deleteMemoryEntity(ctx, userID, entityID); err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to delete entity: %v", err)}, nil
	}
	return map[string]any{"success": true, "message": "Entity and its relations deleted"}, nil
}

func (s *Server) toolEditEntity(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	entityID, _ := args["entity_id"].(string)
	if entityID == "" {
		return map[string]any{"success": false, "error": "entity_id is required"}, nil
	}
	// Fetch existing entity to use as defaults for unset fields
	existing, err := s.getMemoryEntity(ctx, userID, entityID)
	if err != nil {
		return map[string]any{"success": false, "error": "entity not found"}, nil
	}
	entityType := existing.Type
	if t, ok := args["type"].(string); ok && t != "" {
		entityType = t
	}
	name := existing.Name
	if n, ok := args["name"].(string); ok && n != "" {
		name = n
	}
	summary := existing.Summary
	if sm, ok := args["summary"].(string); ok && sm != "" {
		summary = sm
	}
	if err := s.updateMemoryEntity(ctx, userID, entityID, entityType, name, summary); err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to update entity: %v", err)}, nil
	}
	return map[string]any{"success": true, "entity_id": entityID, "name": name, "type": entityType, "summary": summary}, nil
}

// conversationIDFromContext extracts the conversation ID stored in context (if any).
func conversationIDFromContext(ctx context.Context) string {
	v, _ := ctx.Value(conversationIDContextKey).(string)
	return v
}

// buildMemoryContextForPrompt returns a compact memory block for the system prompt.
// Returns empty string if nothing relevant is found.
func (s *Server) buildMemoryContextForPrompt(ctx context.Context, userID, userMessage string) string {
	if userMessage == "" {
		return ""
	}
	// Use first ~200 chars of message as query
	query := userMessage
	if len(query) > 200 {
		query = query[:200]
	}
	block, err := s.getMemoryContext(ctx, userID, query)
	if err != nil || block == "" {
		return ""
	}
	return block
}

// extractAndStoreMemories is called in the background after each assistant response.
// It asks the LLM to extract entities and relations from the exchange and stores them.
func (s *Server) extractAndStoreMemories(userID, conversationID, userMsg, assistantMsg string, cfg *configstore.UserConfig) {
	ctx := context.Background()

	prompt := fmt.Sprintf(`You are a memory extraction assistant. Given the following conversation exchange, extract any important entities (people, places, languages, trips, foods, preferences, facts about the user) and relationships between them.

User: %s
Assistant: %s

Respond ONLY with a JSON object in this exact format (no markdown, no explanation):
{
  "entities": [
    {"name": "...", "type": "person|place|language|trip|food|preference|organization|topic|thing", "summary": "..."}
  ],
  "relations": [
    {"from_name": "...", "to_name": "...", "relation": "..."}
  ],
  "observations": [
    {"entity_name": "...", "content": "..."}
  ]
}

Only include entities and relations that are clearly stated or strongly implied. If nothing is worth remembering, return {"entities":[],"relations":[],"observations":[]}.`, userMsg, assistantMsg)

	providers := buildProvidersFromConfig(ctx, cfg)
	if len(providers) == 0 {
		return
	}
	svc := orchestrator.NewService(providers, orchestrator.ToolLoopGuard{})
	result, err := svc.Complete(ctx, orchestrator.CompletionRequest{
		System:   "You are a memory extraction assistant. Respond only with valid JSON.",
		Messages: []orchestrator.ChatMessage{{Role: "user", Content: prompt}},
	})
	if err != nil || result.Output == "" {
		return
	}
	resp := strings.TrimSpace(result.Output)
	if idx := strings.Index(resp, "{"); idx > 0 {
		resp = resp[idx:]
	}
	if idx := strings.LastIndex(resp, "}"); idx >= 0 {
		resp = resp[:idx+1]
	}

	var extracted struct {
		Entities []struct {
			Name    string `json:"name"`
			Type    string `json:"type"`
			Summary string `json:"summary"`
		} `json:"entities"`
		Relations []struct {
			FromName string `json:"from_name"`
			ToName   string `json:"to_name"`
			Relation string `json:"relation"`
		} `json:"relations"`
		Observations []struct {
			EntityName string `json:"entity_name"`
			Content    string `json:"content"`
		} `json:"observations"`
	}

	if err := json.Unmarshal([]byte(resp), &extracted); err != nil {
		return
	}

	// Store entities
	entityIDs := map[string]string{}
	for _, e := range extracted.Entities {
		if e.Name == "" {
			continue
		}
		id, err := s.upsertMemoryEntity(ctx, userID, e.Type, e.Name, e.Summary)
		if err == nil {
			entityIDs[strings.ToLower(e.Name)] = id
		}
	}

	// Store relations
	for _, r := range extracted.Relations {
		if r.FromName == "" || r.ToName == "" || r.Relation == "" {
			continue
		}
		fromID, err := s.upsertMemoryEntity(ctx, userID, "thing", r.FromName, "")
		if err != nil {
			continue
		}
		toID, err := s.upsertMemoryEntity(ctx, userID, "thing", r.ToName, "")
		if err != nil {
			continue
		}
		_, _ = s.upsertMemoryRelation(ctx, userID, fromID, toID, r.Relation, conversationID)
	}

	// Store observations
	for _, o := range extracted.Observations {
		if o.Content == "" {
			continue
		}
		var entityIDPtr *string
		if id, ok := entityIDs[strings.ToLower(o.EntityName)]; ok {
			entityIDPtr = &id
		}
		_, _ = s.addMemoryObservation(ctx, userID, entityIDPtr, o.Content, conversationID)
	}
}
