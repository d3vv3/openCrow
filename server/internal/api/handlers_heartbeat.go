package api

import (
	"net/http"
	"strings"

	"github.com/opencrow/opencrow/server/internal/realtime"
)

// @Summary Get heartbeat configuration
// @Tags    heartbeat
// @Security BearerAuth
// @Produce json
// @Success 200 {object} HeartbeatConfigDTO
// @Failure 401 {object} ErrorResponse
// @Router  /v1/heartbeat [get]
func (s *Server) handleGetHeartbeatConfig(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	config, err := s.getHeartbeatConfig(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load heartbeat config")
		return
	}
	writeJSON(w, http.StatusOK, config)
}

// @Summary Update heartbeat configuration
// @Tags    heartbeat
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body UpdateHeartbeatConfigRequest true "Heartbeat settings"
// @Success 200 {object} HeartbeatConfigDTO
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/heartbeat [put]
func (s *Server) handlePutHeartbeatConfig(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var req UpdateHeartbeatConfigRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	config, err := s.putHeartbeatConfig(r.Context(), userID, req)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to save heartbeat config")
		return
	}
	writeJSON(w, http.StatusOK, config)
}

// @Summary Manually trigger a heartbeat event
// @Tags    heartbeat
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body TriggerHeartbeatRequest true "Optional message"
// @Success 201 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/heartbeat/trigger [post]
func (s *Server) handleTriggerHeartbeat(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var req TriggerHeartbeatRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		message = "manual heartbeat trigger"
	}
	evt, err := s.createHeartbeatEvent(r.Context(), userID, "TRIGGERED", message)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to trigger heartbeat")
		return
	}
	delay := s.backoffPolicy.NextDelay(1)
	s.realtimeHub.Publish(realtime.Event{
		UserID: userID,
		Type:   "heartbeat.triggered",
		Payload: map[string]any{
			"eventId":      evt.ID,
			"backoffDelay": delay.String(),
		},
	})
	writeJSON(w, http.StatusCreated, map[string]any{"event": evt, "nextBackoffDelay": delay.String()})
}

// @Summary List heartbeat events for the current user
// @Tags    heartbeat
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string][]HeartbeatEventDTO
// @Failure 401 {object} ErrorResponse
// @Router  /v1/heartbeat/events [get]
func (s *Server) handleListHeartbeatEvents(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	events, err := s.listHeartbeatEvents(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load heartbeat events")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}
