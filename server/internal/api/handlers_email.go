package api

import (
	"bufio"
	"crypto/tls"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/opencrow/opencrow/server/internal/realtime"
)

// @Summary List email inboxes for the current user
// @Tags    email
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string][]EmailInboxDTO
// @Failure 401 {object} ErrorResponse
// @Router  /v1/email/inboxes [get]
func (s *Server) handleListEmailInboxes(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	inboxes, err := s.listEmailInboxes(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load inboxes")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"inboxes": inboxes})
}

// @Summary Create a new email inbox
// @Tags    email
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body CreateEmailInboxRequest true "Inbox configuration"
// @Success 201 {object} EmailInboxDTO
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/email/inboxes [post]
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

// @Summary Update an email inbox by ID
// @Tags    email
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   id   path string                 true "Inbox ID"
// @Param   body body UpdateEmailInboxRequest true "Fields to update"
// @Success 200 {object} EmailInboxDTO
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router  /v1/email/inboxes/{id} [patch]
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

// @Summary Delete an email inbox by ID
// @Tags    email
// @Security BearerAuth
// @Produce json
// @Param   id path string true "Inbox ID"
// @Success 200 {object} map[string]bool
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router  /v1/email/inboxes/{id} [delete]
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

// @Summary List email poll events for the current user
// @Tags    email
// @Security BearerAuth
// @Produce json
// @Success 200 {object} map[string][]EmailPollEventDTO
// @Failure 401 {object} ErrorResponse
// @Router  /v1/email/poll-events [get]
func (s *Server) handleListEmailPollEvents(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())
	events, err := s.listEmailPollEvents(r.Context(), userID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "unable to load poll events")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}

// @Summary Trigger an email poll for an inbox
// @Tags    email
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body TriggerEmailPollRequest true "Inbox ID and optional detail"
// @Success 201 {object} map[string]EmailPollEventDTO
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Failure 404 {object} ErrorResponse
// @Router  /v1/email/poll [post]
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

// @Summary Test an IMAP email connection
// @Tags    email
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body TestEmailConnectionRequest true "IMAP connection details"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/email/test [post]
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

// @Summary Auto-detect IMAP/SMTP settings for an email address
// @Tags    email
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body EmailAutoconfigRequest true "Email address to look up"
// @Success 200 {object} EmailAutoconfigResult
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/email/autoconfig [post]
func (s *Server) handleEmailAutoconfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	email := strings.TrimSpace(req.Email)
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || parts[1] == "" {
		writeError(w, http.StatusBadRequest, "invalid email address")
		return
	}
	domain := strings.ToLower(parts[1])

	type result struct {
		ImapHost     string `json:"imapHost,omitempty"`
		ImapPort     int    `json:"imapPort,omitempty"`
		ImapUsername string `json:"imapUsername,omitempty"`
		SmtpHost     string `json:"smtpHost,omitempty"`
		SmtpPort     int    `json:"smtpPort,omitempty"`
		UseTLS       bool   `json:"useTls"`
		Source       string `json:"source,omitempty"`
	}

	if res, src := fetchThunderbirdAutoconfig(domain, email); res != nil {
		writeJSON(w, http.StatusOK, result{
			ImapHost: res.imapHost, ImapPort: res.imapPort,
			ImapUsername: res.imapUsername,
			SmtpHost: res.smtpHost, SmtpPort: res.smtpPort,
			UseTLS: res.useTLS, Source: src,
		})
		return
	}

	writeJSON(w, http.StatusOK, result{Source: "none"})
}

type autoconfigResult struct {
	imapHost     string
	imapPort     int
	imapUsername string
	smtpHost     string
	smtpPort     int
	useTLS       bool
}

func fetchThunderbirdAutoconfig(domain, email string) (*autoconfigResult, string) {
	type attempt struct {
		url    string
		source string
	}
	urls := []attempt{
		{"https://autoconfig." + domain + "/mail/config-v1.1.xml?emailaddress=" + email, "autoconfig"},
		{"http://autoconfig." + domain + "/mail/config-v1.1.xml?emailaddress=" + email, "autoconfig-http"},
		{"https://" + domain + "/.well-known/autoconfig/mail/config-v1.1.xml?emailaddress=" + email, "wellknown"},
		{"http://" + domain + "/.well-known/autoconfig/mail/config-v1.1.xml?emailaddress=" + email, "wellknown-http"},
		{"https://autoconfig.thunderbird.net/v1.1/" + domain, "ispdb"},
	}

	// Resolve CNAME on autoconfig.{domain} — if it points to another domain,
	// also try the ISPDB for that provider's domain.
	if cname, err := net.LookupCNAME("autoconfig." + domain); err == nil {
		cname = strings.TrimSuffix(strings.ToLower(cname), ".")
		if cname != "autoconfig."+domain && cname != "" {
			// Extract parent domain from CNAME target (e.g. autoconfig.fastmail.com -> fastmail.com)
			if parts := strings.SplitN(cname, ".", 2); len(parts) == 2 && parts[1] != "" {
				cnameDomain := parts[1]
				if cnameDomain != domain {
					urls = append(urls, attempt{
						"https://autoconfig.thunderbird.net/v1.1/" + cnameDomain, "ispdb-cname",
					})
				}
			}
		}
	}

	client := &http.Client{Timeout: 5 * time.Second}
	for _, u := range urls {
		resp, err := client.Get(u.url)
		if err != nil {
			continue
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
		resp.Body.Close()
		if err != nil || resp.StatusCode != 200 {
			continue
		}
		if res := parseThunderbirdXML(body, email); res != nil {
			return res, u.source
		}
	}
	return nil, ""
}

func parseThunderbirdXML(data []byte, email string) *autoconfigResult {
	type xmlServer struct {
		Type       string `xml:"type,attr"`
		Hostname   string `xml:"hostname"`
		Port       int    `xml:"port"`
		SocketType string `xml:"socketType"`
		Username   string `xml:"username"`
	}
	type xmlConfig struct {
		XMLName  xml.Name `xml:"clientConfig"`
		Provider struct {
			Incoming []xmlServer `xml:"incomingServer"`
			Outgoing []xmlServer `xml:"outgoingServer"`
		} `xml:"emailProvider"`
	}

	var cfg xmlConfig
	if err := xml.Unmarshal(data, &cfg); err != nil {
		return nil
	}

	res := &autoconfigResult{useTLS: true}
	parts := strings.SplitN(email, "@", 2)
	localPart := parts[0]
	domain := ""
	if len(parts) == 2 {
		domain = parts[1]
	}

	replacePlaceholders := func(s string) string {
		s = strings.ReplaceAll(s, "%EMAILADDRESS%", email)
		s = strings.ReplaceAll(s, "%EMAILLOCALPART%", localPart)
		s = strings.ReplaceAll(s, "%EMAILDOMAIN%", domain)
		return s
	}

	for _, srv := range cfg.Provider.Incoming {
		if strings.EqualFold(srv.Type, "imap") {
			res.imapHost = replacePlaceholders(srv.Hostname)
			res.imapPort = srv.Port
			res.imapUsername = replacePlaceholders(srv.Username)
			st := strings.ToUpper(srv.SocketType)
			res.useTLS = st == "SSL" || st == "STARTTLS"
			break
		}
	}
	for _, srv := range cfg.Provider.Outgoing {
		if strings.EqualFold(srv.Type, "smtp") {
			res.smtpHost = replacePlaceholders(srv.Hostname)
			res.smtpPort = srv.Port
			break
		}
	}

	if res.imapHost == "" {
		return nil
	}
	if res.imapPort == 0 {
		if res.useTLS {
			res.imapPort = 993
		} else {
			res.imapPort = 143
		}
	}
	if res.smtpPort == 0 {
		res.smtpPort = 587
	}
	return res
}
