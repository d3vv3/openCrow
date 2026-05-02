// handlers_memory_graph.go -- REST API for the memory graph (entities, relations, observations).
package api

import (
	"encoding/json"
	"net/http"
)

// patchEntityRequest is the JSON body for PATCH /v1/memory/entities/{id}.
// Every field is optional; omitted fields are left unchanged.
type patchEntityRequest struct {
	Type    *string `json:"type,omitempty"`
	Name    *string `json:"name,omitempty"`
	Summary *string `json:"summary,omitempty"`
}

// handleGetMemoryGraph returns the full memory graph for the authenticated user.
//
// @Summary     Get memory graph
// @Description Returns all memory entities and relations for the user
// @Tags        memory
// @Produce     json
// @Security    BearerAuth
// @Success     200 {object} MemoryGraph
// @Failure     401 {object} ErrorResponse
// @Router      /v1/memory/graph [get]
func (s *Server) handleGetMemoryGraph(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	graph, err := s.getFullMemoryGraph(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load memory graph")
		return
	}
	writeJSON(w, http.StatusOK, graph)
}

// handleGetMemoryEntity returns a single entity with its relations and observations.
//
// @Summary     Get memory entity
// @Tags        memory
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Entity ID"
// @Success     200 {object} map[string]any
// @Failure     404 {object} ErrorResponse
// @Router      /v1/memory/entities/{id} [get]
func (s *Server) handleGetMemoryEntity(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	entityID := r.PathValue("id")

	entity, err := s.getMemoryEntity(r.Context(), userID, entityID)
	if err != nil {
		writeError(w, http.StatusNotFound, "entity not found")
		return
	}
	relations, _ := s.listRelationsForEntity(r.Context(), userID, entityID)
	observations, _ := s.listObservationsForEntity(r.Context(), userID, entityID)

	writeJSON(w, http.StatusOK, map[string]any{
		"entity":       entity,
		"relations":    relations,
		"observations": observations,
	})
}

// handleUpdateMemoryEntity updates a memory entity's type, name, and/or summary.
// Only the fields present in the JSON body are changed.
//
// @Summary     Update memory entity
// @Tags        memory
// @Accept      json
// @Produce     json
// @Security    BearerAuth
// @Param       id path string true "Entity ID"
// @Param       body body patchEntityRequest false "Fields to update"
// @Success     200 {object} MemoryEntity
// @Failure     404 {object} ErrorResponse
// @Router      /v1/memory/entities/{id} [patch]
func (s *Server) handleUpdateMemoryEntity(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	entityID := r.PathValue("id")

	var req patchEntityRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// Load existing entity so we only overwrite fields that were provided.
	existing, err := s.getMemoryEntity(r.Context(), userID, entityID)
	if err != nil {
		writeError(w, http.StatusNotFound, "entity not found")
		return
	}

	t := existing.Type
	n := existing.Name
	s2 := existing.Summary
	if req.Type != nil {
		t = *req.Type
	}
	if req.Name != nil {
		n = *req.Name
	}
	if req.Summary != nil {
		s2 = *req.Summary
	}

	if err := s.updateMemoryEntity(r.Context(), userID, entityID, t, n, s2); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update entity")
		return
	}

	// Re-read to return the updated entity
	updated, err := s.getMemoryEntity(r.Context(), userID, entityID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read updated entity")
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

// handleDeleteMemoryEntity deletes an entity and all its relations.
//
// @Summary     Delete memory entity
// @Tags        memory
// @Security    BearerAuth
// @Param       id path string true "Entity ID"
// @Success     204
// @Failure     404 {object} ErrorResponse
// @Router      /v1/memory/entities/{id} [delete]
func (s *Server) handleDeleteMemoryEntity(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	entityID := r.PathValue("id")

	if err := s.deleteMemoryEntity(r.Context(), userID, entityID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete entity")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleDeleteMemoryRelation deletes a single relation.
//
// @Summary     Delete memory relation
// @Tags        memory
// @Security    BearerAuth
// @Param       id path string true "Relation ID"
// @Success     204
// @Failure     404 {object} ErrorResponse
// @Router      /v1/memory/relations/{id} [delete]
func (s *Server) handleDeleteMemoryRelation(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	relationID := r.PathValue("id")

	if err := s.deleteMemoryRelation(r.Context(), userID, relationID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete relation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
