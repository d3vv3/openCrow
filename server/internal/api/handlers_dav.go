package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/opencrow/opencrow/server/internal/configstore"
)

type TestDAVConnectionRequest struct {
	URL                 string `json:"url"`
	Username            string `json:"username,omitempty"`
	Password            string `json:"password,omitempty"`
	Enabled             *bool  `json:"enabled,omitempty"`
	WebDAVEnabled       *bool  `json:"webdavEnabled,omitempty"`
	CalDAVEnabled       *bool  `json:"caldavEnabled,omitempty"`
	CardDAVEnabled      *bool  `json:"carddavEnabled,omitempty"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds,omitempty"`
}

// @Summary Test a DAV connection
// @Tags    dav
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body TestDAVConnectionRequest true "DAV connection parameters"
// @Success 200 {object} DAVTestResult
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/dav/test [post]
func (s *Server) handleTestDAVConnection(w http.ResponseWriter, r *http.Request) {
	var req TestDAVConnectionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	url := strings.TrimSpace(req.URL)
	if url == "" {
		writeError(w, http.StatusBadRequest, "url required")
		return
	}

	cfg := configstore.DAVConfig{
		URL:                 url,
		Username:            strings.TrimSpace(req.Username),
		Password:            req.Password,
		Enabled:             true,
		WebDAVEnabled:       true,
		CalDAVEnabled:       true,
		CardDAVEnabled:      true,
		PollIntervalSeconds: req.PollIntervalSeconds,
	}
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	if req.WebDAVEnabled != nil {
		cfg.WebDAVEnabled = *req.WebDAVEnabled
	}
	if req.CalDAVEnabled != nil {
		cfg.CalDAVEnabled = *req.CalDAVEnabled
	}
	if req.CardDAVEnabled != nil {
		cfg.CardDAVEnabled = *req.CardDAVEnabled
	}
	if cfg.PollIntervalSeconds <= 0 {
		cfg.PollIntervalSeconds = 900
	}
	// Allow testing draft configs before the integration is enabled/saved.
	cfg.Enabled = true

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	writeJSON(w, http.StatusOK, s.testDAVConnection(ctx, cfg))
}
