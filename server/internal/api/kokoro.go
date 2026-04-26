package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

// KokoroManager calls a Kokoro TTS sidecar over HTTP to synthesize speech.
// The sidecar exposes an OpenAI-compatible API:
//
//	POST /v1/audio/speech  - synthesize text to audio
//	GET  /health           - readiness probe
type KokoroManager struct {
	endpoint string // e.g. "http://kokoro:8880"
}

// NewKokoroManager creates a KokoroManager.
// If endpoint is empty, all operations return a clear error.
func NewKokoroManager(endpoint string) *KokoroManager {
	return &KokoroManager{
		endpoint: strings.TrimRight(endpoint, "/"),
	}
}

// IsConfigured reports whether a Kokoro endpoint is set.
func (k *KokoroManager) IsConfigured() bool {
	return k.endpoint != ""
}

// IsReady pings the Kokoro health endpoint and returns true when the sidecar
// responds with HTTP 200.
func (k *KokoroManager) IsReady() bool {
	if k.endpoint == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, k.endpoint+"/health", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// kokoroSpeechRequest mirrors the OpenAI TTS request body.
type kokoroSpeechRequest struct {
	Model          string `json:"model"`
	Input          string `json:"input"`
	Voice          string `json:"voice"`
	ResponseFormat string `json:"response_format"`
	Stream         bool   `json:"stream"`
}

// Synthesize sends text to the Kokoro sidecar and returns a streaming reader
// of the mp3 audio, plus the content-type reported by the sidecar.
// The caller is responsible for closing the returned ReadCloser.
func (k *KokoroManager) Synthesize(ctx context.Context, text, voice string) (io.ReadCloser, string, error) {
	if k.endpoint == "" {
		return nil, "", fmt.Errorf("kokoro not configured (KOKORO_ENDPOINT is empty)")
	}
	if voice == "" {
		voice = "af_heart"
	}

	body, err := json.Marshal(kokoroSpeechRequest{
		Model:          "tts-1",
		Input:          text,
		Voice:          voice,
		ResponseFormat: "mp3",
		Stream:         true,
	})
	if err != nil {
		return nil, "", fmt.Errorf("kokoro: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, k.endpoint+"/v1/audio/speech", bytes.NewReader(body))
	if err != nil {
		return nil, "", fmt.Errorf("kokoro: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("kokoro: request failed: %w", err)
	}

	if resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, "", fmt.Errorf("kokoro: status %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	ct := resp.Header.Get("Content-Type")
	if ct == "" {
		ct = "audio/mpeg"
	}
	log.Printf("[kokoro] synthesizing %d chars with voice=%s", len(text), voice)
	return resp.Body, ct, nil
}
