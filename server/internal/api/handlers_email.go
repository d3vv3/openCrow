package api

import (
	"bufio"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/opencrow/opencrow/server/internal/realtime"
)

func (s *Server) handleListEmailInboxes(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	inboxes, err := s.listEmailInboxes(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load inboxes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"inboxes": inboxes})
}

func (s *Server) handleCreateEmailInbox(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var req CreateEmailInboxRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	address := strings.TrimSpace(strings.ToLower(req.Address))
	imapHost := strings.TrimSpace(req.ImapHost)
	imapPort := req.ImapPort
	if imapPort <= 0 {
		imapPort = 993
	}
	pollInterval := req.PollIntervalSeconds
	if pollInterval <= 0 {
		pollInterval = 60
	}
	useTLS := true
	if req.UseTLS != nil {
		useTLS = *req.UseTLS
	}

	if address == "" || imapHost == "" {
		writeError(w, http.StatusBadRequest, "address and imapHost required")
		return
	}

	imapUsername := strings.TrimSpace(req.ImapUsername)
	imapPassword := req.ImapPassword

	inbox, err := s.createEmailInbox(r.Context(), userID, address, imapHost, imapPort, imapUsername, imapPassword, useTLS, pollInterval)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate") {
			writeError(w, http.StatusConflict, "inbox already exists")
			return
		}
		writeError(w, http.StatusInternalServerError, "unable to create inbox")
		return
	}
	writeJSON(w, http.StatusCreated, inbox)
}

func (s *Server) handleUpdateEmailInbox(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	inboxID := r.PathValue("id")
	if !isUUID(inboxID) {
		writeError(w, http.StatusBadRequest, "invalid inbox id")
		return
	}

	var req UpdateEmailInboxRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	inbox, err := s.updateEmailInbox(r.Context(), userID, inboxID, req)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "inbox not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "unable to update inbox")
		return
	}

	writeJSON(w, http.StatusOK, inbox)
}

func (s *Server) handleDeleteEmailInbox(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	inboxID := r.PathValue("id")
	if !isUUID(inboxID) {
		writeError(w, http.StatusBadRequest, "invalid inbox id")
		return
	}

	deleted, err := s.deleteEmailInbox(r.Context(), userID, inboxID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to delete inbox")
		return
	}
	if !deleted {
		writeError(w, http.StatusNotFound, "inbox not found")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

func (s *Server) handleListEmailPollEvents(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	events, err := s.listEmailPollEvents(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load poll events")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

func (s *Server) handleTriggerEmailPoll(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	var req TriggerEmailPollRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if !isUUID(req.InboxID) {
		writeError(w, http.StatusBadRequest, "invalid inbox id")
		return
	}

	evt, err := s.createEmailPollEvent(r.Context(), userID, req.InboxID, "TRIGGERED", strings.TrimSpace(req.Detail))
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "inbox not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "unable to trigger email poll")
		return
	}

	s.realtimeHub.Publish(realtime.Event{
		UserID: userID,
		Type:   "email.poll.triggered",
		Payload: map[string]any{
			"eventId": evt.ID,
			"inboxId": evt.InboxID,
		},
	})

	writeJSON(w, http.StatusCreated, map[string]any{"event": evt})
}

func (s *Server) handleTestEmailConnection(w http.ResponseWriter, r *http.Request) {
	var req TestEmailConnectionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	host := strings.TrimSpace(req.ImapHost)
	if host == "" {
		writeError(w, http.StatusBadRequest, "imapHost required")
		return
	}
	port := req.ImapPort
	if port <= 0 {
		if req.UseTLS {
			port = 993
		} else {
			port = 143
		}
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	timeout := 8 * time.Second

	var conn net.Conn
	var err error

	if req.UseTLS {
		conn, err = tls.DialWithDialer(&net.Dialer{Timeout: timeout}, "tcp", addr, &tls.Config{
			ServerName: host,
		})
	} else {
		conn, err = net.DialTimeout("tcp", addr, timeout)
	}
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "no greeting from server"})
		return
	}
	greeting := scanner.Text()
	if !strings.HasPrefix(greeting, "* OK") {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "unexpected greeting: " + greeting})
		return
	}

	// Optionally try LOGIN if credentials provided
	if req.Username != "" && req.Password != "" {
		tag := "A001"
		cmd := fmt.Sprintf("%s LOGIN %q %q\r\n", tag, req.Username, req.Password)
		if _, werr := fmt.Fprint(conn, cmd); werr != nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "send LOGIN failed: " + werr.Error()})
			return
		}
		for scanner.Scan() {
			line := scanner.Text()
			if strings.HasPrefix(line, tag+" OK") {
				writeJSON(w, http.StatusOK, map[string]any{"ok": true})
				return
			}
			if strings.HasPrefix(line, tag+" NO") || strings.HasPrefix(line, tag+" BAD") {
				writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "auth failed: " + line})
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "no response to LOGIN"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "detail": "connected"})
}
