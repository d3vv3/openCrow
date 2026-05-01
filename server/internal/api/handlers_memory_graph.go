// handlers_memory_graph.go -- REST API for the memory graph (entities, relations, observations).
package api

import (
	"net/http"
)

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
