package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Server) handleListMemories(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	memories, err := s.listMemories(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load memory")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"memories": memories})
}

func (s *Server) handleCreateMemory(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var req CreateMemoryRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	category := strings.TrimSpace(req.Category)
	content := strings.TrimSpace(req.Content)
	confidence := req.Confidence
	if confidence <= 0 {
		confidence = 50
	}
	if confidence > 100 {
		confidence = 100
	}
	if category == "" || content == "" {
		writeError(w, http.StatusBadRequest, "category and content required")
		return
	}

	memory, err := s.createMemory(r.Context(), userID, category, content, confidence)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to create memory")
		return
	}
	writeJSON(w, http.StatusCreated, memory)
}

func (s *Server) handleDeleteMemory(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	memID := r.PathValue("id")
	if !isUUID(memID) {
		writeError(w, http.StatusBadRequest, "invalid memory id")
		return
	}

	deleted, err := s.deleteMemory(r.Context(), userID, memID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "memory not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "unable to delete memory")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "memory not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}
