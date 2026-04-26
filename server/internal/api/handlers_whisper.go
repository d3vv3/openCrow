package api

import (
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"unicode"

	"github.com/pemistahl/lingua-go"
)

// linguaDetector is a lazily-initialised detector covering the languages
// that Kokoro-FastAPI supports.
var (
	linguaOnce     sync.Once
	linguaDetector lingua.LanguageDetector
)

func getLinguaDetector() lingua.LanguageDetector {
	linguaOnce.Do(func() {
		linguaDetector = lingua.NewLanguageDetectorBuilder().
			FromLanguages(
				lingua.English,
				lingua.Spanish,
				lingua.French,
				lingua.German,
				lingua.Italian,
				lingua.Portuguese,
				lingua.Hindi,
				lingua.Japanese,
				lingua.Chinese,
				lingua.Korean,
				lingua.Arabic,
				lingua.Russian,
			).
			Build()
	})
	return linguaDetector
}

// linguaToTag maps lingua Language values to BCP-47 codes used in VoiceConfig.
var linguaToTag = map[lingua.Language]string{
	lingua.English:    "en",
	lingua.Spanish:    "es",
	lingua.French:     "fr",
	lingua.German:     "de",
	lingua.Italian:    "it",
	lingua.Portuguese: "pt",
	lingua.Hindi:      "hi",
	lingua.Japanese:   "ja",
	lingua.Chinese:    "zh",
	lingua.Korean:     "ko",
	lingua.Arabic:     "ar",
	lingua.Russian:    "ru",
}

// detectLanguage performs language detection on the given text.
// For non-Latin scripts the Unicode fast-path is used; for Latin-script text
// lingua is used so that Spanish/French/Italian/Portuguese/German etc. are
// distinguished from English.
func detectLanguage(text string) string {
	var (
		cjk, hiragana, katakana, hangul, arabic, cyrillic, devanagari, latin int
	)
	for _, r := range text {
		switch {
		case r >= 0x4E00 && r <= 0x9FFF:
			cjk++
		case r >= 0x3040 && r <= 0x309F:
			hiragana++
		case r >= 0x30A0 && r <= 0x30FF:
			katakana++
		case r >= 0xAC00 && r <= 0xD7AF:
			hangul++
		case r >= 0x0600 && r <= 0x06FF:
			arabic++
		case r >= 0x0400 && r <= 0x04FF:
			cyrillic++
		case r >= 0x0900 && r <= 0x097F:
			devanagari++
		case unicode.Is(unicode.Latin, r) && unicode.IsLetter(r):
			latin++
		}
	}

	total := cjk + hiragana + katakana + hangul + arabic + cyrillic + latin + devanagari
	if total == 0 {
		return ""
	}

	// Fast-path: unambiguous non-Latin scripts.
	if hiragana > 0 || katakana > 0 {
		return "ja"
	}
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

	// Latin-script text: use lingua for proper per-language detection.
	if latin*100/total > 50 {
		det := getLinguaDetector()
		if lang, ok := det.DetectLanguageOf(text); ok {
			if tag, found := linguaToTag[lang]; found {
				return tag
			}
		}
		return "en" // fallback for unrecognised Latin
	}
	return ""
}

// builtinLangVoice maps BCP-47 language codes to the best Kokoro-FastAPI voice
// for that language. Used as a fallback when the user has not configured a
// per-language override.
var builtinLangVoice = map[string]string{
	"es": "ef_dora",     // Spanish female
	"fr": "ff_siwis",    // French female
	"it": "if_sara",     // Italian female
	"pt": "pf_dora",     // Portuguese female
	"de": "af_heart",    // No native German voice; use English default
	"hi": "hf_alpha",    // Hindi female
	"ja": "jf_alpha",    // Japanese female
	"zh": "zf_xiaoxiao", // Chinese female
	"ko": "af_heart",    // No native Korean voice; use English default
	"ar": "af_heart",    // No native Arabic voice; use English default
	"ru": "af_heart",    // No native Russian voice; use English default
	"en": "",            // Let default voice handle English
}

// voiceForText selects a Kokoro voice for the given text using the user's VoiceConfig.
// Priority: explicit request voice > user lang override > builtin lang default > user default > hardcoded fallback.
func voiceForText(text, requestVoice string, defaultVoice string, langVoices map[string]string) string {
	// Explicit voice in request always wins.
	if requestVoice != "" {
		return requestVoice
	}

	lang := detectLanguage(text)

	// User-configured per-language override.
	if lang != "" {
		if v, ok := langVoices[lang]; ok && v != "" {
			return v
		}
		// Built-in language default (so Spanish/French/etc. get a native voice
		// even without manual configuration).
		if v, ok := builtinLangVoice[lang]; ok && v != "" {
			return v
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

// handleVoiceTtsVoices returns the list of available TTS voices from the Kokoro sidecar.
// GET /v1/voice/tts/voices
// Response: { "voices": ["af_heart", ...] }
func (s *Server) handleVoiceTtsVoices(w http.ResponseWriter, r *http.Request) {
	if s.kokoro == nil || !s.kokoro.IsConfigured() {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": "TTS not configured"}) //nolint:errcheck
		return
	}
	voices, err := s.kokoro.ListVoices(r.Context())
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"error": err.Error()}) //nolint:errcheck
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string][]string{"voices": voices}) //nolint:errcheck
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
