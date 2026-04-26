package api

import (
	"encoding/json"
	"io"
	"net/http"
	"unicode"
)

// detectLanguage performs a best-effort script/language detection based on the
// Unicode blocks present in the text. It returns a BCP-47 language code or an
// empty string when the language cannot be determined confidently.
func detectLanguage(text string) string {
	var (
		cjk, hiragana, katakana, hangul, arabic, cyrillic, latin, devanagari int
	)
	for _, r := range text {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF: // CJK Unified Ideographs
			cjk++
		case r >= 0x3040 && r <= 0x309F: // Hiragana
			hiragana++
		case r >= 0x30A0 && r <= 0x30FF: // Katakana
			katakana++
		case r >= 0xAC00 && r <= 0xD7AF: // Hangul Syllables
			hangul++
		case r >= 0x0600 && r <= 0x06FF: // Arabic
			arabic++
		case r >= 0x0400 && r <= 0x04FF: // Cyrillic
			cyrillic++
		case r >= 0x0900 && r <= 0x097F: // Devanagari (Hindi etc.)
			devanagari++
		case unicode.Is(unicode.Latin, r) && unicode.IsLetter(r):
			latin++
		}
	}

	total := cjk + hiragana + katakana + hangul + arabic + cyrillic + latin + devanagari
	if total == 0 {
		return ""
	}

	// Japanese: hiragana/katakana present, or mix of CJK + Japanese scripts
	if hiragana > 0 || katakana > 0 {
		return "ja"
	}
	// Chinese: CJK dominant without Japanese kana
	if cjk*100/total > 30 {
		return "zh"
	}
	if hangul*100/total > 30 {
		return "ko"
	}
	if arabic*100/total > 30 {
		return "ar"
	}
	if cyrillic*100/total > 30 {
		return "ru"
	}
	if devanagari*100/total > 30 {
		return "hi"
	}
	if latin*100/total > 50 {
		return "en"
	}
	return ""
}

// voiceForText selects a Kokoro voice for the given text using the user's VoiceConfig.
// It detects the language of the text, looks it up in LanguageVoices, and falls back
// to DefaultVoice when no mapping exists.
func voiceForText(text, requestVoice string, defaultVoice string, langVoices map[string]string) string {
	// Explicit voice in request always wins.
	if requestVoice != "" {
		return requestVoice
	}
	if len(langVoices) > 0 {
		lang := detectLanguage(text)
		if lang != "" {
			if v, ok := langVoices[lang]; ok && v != "" {
				return v
			}
		}
	}
	if defaultVoice != "" {
		return defaultVoice
	}
	return "af_heart"
}

// @Summary Get voice sidecar status
// @Tags    server
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 401 {object} ErrorResponse
// @Router  /v1/voice/status [get]
// handleVoiceStatus returns the current status of the voice transcription sidecar.
// GET /v1/voice/status
// Response: { "status": "ok" | "downloading" | "down", "model": "ggml-base" }
func (s *Server) handleVoiceStatus(w http.ResponseWriter, _ *http.Request) {
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

// @Summary Get TTS sidecar status
// @Tags    server
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string]string
// @Failure 401 {object} ErrorResponse
// @Router  /v1/voice/tts/status [get]
// handleVoiceTtsStatus returns the current status of the Kokoro TTS sidecar.
// GET /v1/voice/tts/status
// Response: { "status": "ok" | "down" }
func (s *Server) handleVoiceTtsStatus(w http.ResponseWriter, _ *http.Request) {
	status := "down"
	if s.kokoro != nil && s.kokoro.IsConfigured() && s.kokoro.IsReady() {
		status = "ok"
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": status}) //nolint:errcheck
}

// @Summary Synthesize speech via the Kokoro TTS sidecar
// @Tags    server
// @Security BearerAuth
// @Accept  json
// @Produce octet-stream
// @Param   body body object true "TTS request" SchemaExample({"text":"Hello world","voice":"af_heart"})
// @Success 200 {string} string "audio/mpeg stream"
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 503 {object} ErrorResponse
// @Router  /v1/voice/tts [post]
// handleVoiceTts accepts a JSON body with text and optional voice, synthesizes
// speech via Kokoro, and streams the audio back to the client.
// POST /v1/voice/tts
// Body: { "text": "...", "voice": "af_heart" }
// Response: audio/mpeg stream
func (s *Server) handleVoiceTts(w http.ResponseWriter, r *http.Request) {
	if s.kokoro == nil || !s.kokoro.IsConfigured() {
		writeError(w, http.StatusServiceUnavailable, "TTS not configured (KOKORO_ENDPOINT is not set)")
		return
	}

	var req struct {
		Text  string `json:"text"`
		Voice string `json:"voice"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Text == "" {
		writeError(w, http.StatusBadRequest, "text is required")
		return
	}

	// Resolve the voice: request field -> language-detection map -> config default -> hardcoded fallback.
	voice := req.Voice
	if voice == "" {
		userID := userIDFromContext(r.Context())
		if userID != "" && s.configStore != nil {
			if cfg, err := s.configStore.GetUserConfig(userID); err == nil {
				voice = voiceForText(req.Text, req.Voice, cfg.Voice.DefaultVoice, cfg.Voice.LanguageVoices)
			}
		}
	}
	if voice == "" {
		voice = "af_heart"
	}

	audio, ct, err := s.kokoro.Synthesize(r.Context(), req.Text, voice)
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}
	defer audio.Close()

	w.Header().Set("Content-Type", ct)
	w.WriteHeader(http.StatusOK)
	io.Copy(w, audio) //nolint:errcheck
}
