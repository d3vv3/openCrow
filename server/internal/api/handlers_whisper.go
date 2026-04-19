package api

import (
	"encoding/json"
	"io"
	"net/http"
)

// @Summary Get whisper sidecar status
// @Tags    server
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 401 {object} ErrorResponse
// @Router  /v1/whisper/status [get]
// handleWhisperStatus returns the current status of the whisper sidecar.
// GET /v1/whisper/status
// Response: { "status": "ok" | "downloading" | "down", "model": "ggml-base" }
func (s *Server) handleWhisperStatus(w http.ResponseWriter, _ *http.Request) {
	status := "down"
	if s.whisper != nil {
		if s.whisper.endpoint == "" {
			status = "down"
		} else if s.whisper.IsReady() {
			status = "ok"
		} else {
			// Endpoint is set but not yet ready -- still downloading/initializing
			status = "downloading"
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{ //nolint:errcheck
		"status": status,
		"model":  s.whisper.modelName,
	})
}

// @Summary Transcribe an audio file via the whisper sidecar
// @Tags    server
// @Security BearerAuth
// @Accept  multipart/form-data
// @Produce json
// @Param   audio formData file true "Audio file to transcribe"
// @Success 200 {object} map[string]string
// @Failure 400 {string} string "Bad request"
// @Failure 401 {object} ErrorResponse
// @Router  /v1/voice/transcribe [post]
// handleVoiceTranscribe accepts a multipart audio upload, forwards it to the
// whisper sidecar, and returns the transcript as JSON.
// POST /v1/voice/transcribe
// Form fields: audio (file)
// Response: { "transcript": "..." }
func (s *Server) handleVoiceTranscribe(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseMultipartForm(32 << 20); err != nil {
		http.Error(w, "invalid multipart form", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("audio")
	if err != nil {
		http.Error(w, "missing audio field", http.StatusBadRequest)
		return
	}
	defer file.Close()

	data, err := io.ReadAll(file)
	if err != nil {
		http.Error(w, "failed to read audio", http.StatusInternalServerError)
		return
	}

	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "audio/webm"
	}

	transcript, err := s.whisper.Transcribe(r.Context(), data, mimeType)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"transcript": transcript}) //nolint:errcheck
}
