package api

import (
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5"
)

func (s *Server) handleListConversations(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	conversations, err := s.listConversations(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load conversations")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"conversations": conversations})
}

func (s *Server) handleCreateConversation(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var req CreateConversationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	title := strings.TrimSpace(req.Title)
	conversation, err := s.createConversation(r.Context(), userID, title)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to create conversation")
		return
	}
	writeJSON(w, http.StatusCreated, conversation)
}

func (s *Server) handleGetConversation(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	conversationID := r.PathValue("id")
	if !isUUID(conversationID) {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}

	conversation, err := s.getConversation(r.Context(), userID, conversationID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "unable to load conversation")
		return
	}

	writeJSON(w, http.StatusOK, conversation)
}

func (s *Server) handleUpdateConversation(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	conversationID := r.PathValue("id")
	if !isUUID(conversationID) {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}

	var req UpdateConversationRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	title := strings.TrimSpace(req.Title)
	conversation, err := s.updateConversation(r.Context(), userID, conversationID, title)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "unable to update conversation")
		return
	}

	writeJSON(w, http.StatusOK, conversation)
}

func (s *Server) handleDeleteConversation(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	conversationID := r.PathValue("id")
	if !isUUID(conversationID) {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	if err := s.deleteConversation(r.Context(), userID, conversationID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "unable to delete conversation")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleListMessages(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	conversationID := r.PathValue("id")
	if !isUUID(conversationID) {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}

	messages, err := s.listMessages(r.Context(), userID, conversationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load messages")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"messages": messages})
}

func (s *Server) handleCreateMessage(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	conversationID := r.PathValue("id")
	if !isUUID(conversationID) {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}

	var req CreateMessageRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	role := strings.TrimSpace(strings.ToLower(req.Role))
	content := strings.TrimSpace(req.Content)
	if role == "" || content == "" {
		writeError(w, http.StatusBadRequest, "role and content required")
		return
	}
	if role != "system" && role != "user" && role != "assistant" && role != "tool" {
		writeError(w, http.StatusBadRequest, "invalid role")
		return
	}

	message, err := s.createMessage(r.Context(), userID, conversationID, role, content)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "conversation not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "unable to create message")
		return
	}

	writeJSON(w, http.StatusCreated, message)
}

func (s *Server) handleListToolCalls(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	conversationID := r.PathValue("id")
	if !isUUID(conversationID) {
		writeError(w, http.StatusBadRequest, "invalid conversation id")
		return
	}
	calls, err := s.listToolCalls(r.Context(), userID, conversationID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load tool calls")
		return
	}
	if calls == nil {
		calls = []ToolCallRecord{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"toolCalls": calls})
}
