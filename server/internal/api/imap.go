package api

// Minimal hand-rolled IMAP client used by email tools.
// Supports: LOGIN, SELECT, FETCH headers, SEARCH, UID FETCH body, LOGOUT.

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"
)

type imapSession struct {
	conn   net.Conn
	r      *bufio.Reader
	w      *bufio.Writer
	tagSeq int
}

// dialIMAP connects and authenticates, returning a ready session.
func dialIMAP(ctx context.Context, host string, port int, username, password string, useTLS bool) (*imapSession, error) {
	addr := fmt.Sprintf("%s:%d", host, port)
	deadline := time.Now().Add(12 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}

	var conn net.Conn
	var err error
	dialer := &net.Dialer{Deadline: deadline}
	if useTLS {
		conn, err = tls.DialWithDialer(dialer, "tcp", addr, &tls.Config{ServerName: host})
	} else {
		conn, err = dialer.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return nil, fmt.Errorf("dial: %w", err)
	}
	conn.SetDeadline(deadline)

	s := &imapSession{
		conn: conn,
		r:    bufio.NewReader(conn),
		w:    bufio.NewWriter(conn),
	}

	// Read greeting
	line, err := s.r.ReadString('\n')
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("greeting: %w", err)
	}
	if !strings.HasPrefix(line, "* OK") {
		conn.Close()
		return nil, fmt.Errorf("unexpected greeting: %s", strings.TrimSpace(line))
	}

	// LOGIN
	tag := s.nextTag()
	if err := s.send("%s LOGIN %s %s", tag, imapQuote(username), imapQuote(password)); err != nil {
		conn.Close()
		return nil, fmt.Errorf("send LOGIN: %w", err)
	}
	if _, err := s.expectOK(tag); err != nil {
		conn.Close()
		return nil, fmt.Errorf("LOGIN failed: %w", err)
	}

	return s, nil
}

func (s *imapSession) close() {
	tag := s.nextTag()
	_ = s.send("%s LOGOUT", tag)
	s.conn.Close()
}

func (s *imapSession) nextTag() string {
	s.tagSeq++
	return fmt.Sprintf("t%03d", s.tagSeq)
}

func (s *imapSession) send(format string, args ...any) error {
	line := fmt.Sprintf(format, args...) + "\r\n"
	if _, err := s.w.WriteString(line); err != nil {
		return err
	}
	return s.w.Flush()
}

// readUntilTag reads lines until the tagged response line (tXXX OK/NO/BAD).
// Returns all untagged lines and the final tagged line.
func (s *imapSession) readUntilTag(tag string) ([]string, string, error) {
	var lines []string
	for {
		line, err := s.r.ReadString('\n')
		if err != nil {
			return lines, "", fmt.Errorf("read: %w", err)
		}
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, tag+" ") {
			return lines, line, nil
		}
		lines = append(lines, line)
	}
}

func (s *imapSession) expectOK(tag string) ([]string, error) {
	lines, tagged, err := s.readUntilTag(tag)
	if err != nil {
		return nil, err
	}
	status := strings.TrimPrefix(tagged, tag+" ")
	if !strings.HasPrefix(status, "OK") {
		return lines, fmt.Errorf("server returned: %s", status)
	}
	return lines, nil
}

// selectMailbox selects the mailbox, returns the EXISTS count.
func (s *imapSession) selectMailbox(mailbox string) (int, error) {
	tag := s.nextTag()
	if err := s.send("%s SELECT %s", tag, mailbox); err != nil {
		return 0, err
	}
	lines, err := s.expectOK(tag)
	if err != nil {
		return 0, err
	}
	for _, l := range lines {
		// "* 12 EXISTS"
		if strings.HasSuffix(l, " EXISTS") {
			parts := strings.Fields(l)
			if len(parts) >= 2 {
				n, _ := strconv.Atoi(parts[1])
				return n, nil
			}
		}
	}
	return 0, nil
}

// MessageHeader holds parsed envelope data for a message.
type MessageHeader struct {
	SeqNum  int
	Subject string
	From    string
	Date    string
	Flags   string
}

// fetchHeaders fetches envelope+flags for a range "start:end" (1-based seq nums).
func (s *imapSession) fetchHeaders(start, end int) ([]MessageHeader, error) {
	tag := s.nextTag()
	if err := s.send("%s FETCH %d:%d (FLAGS ENVELOPE)", tag, start, end); err != nil {
		return nil, err
	}
	lines, err := s.expectOK(tag)
	if err != nil {
		return nil, err
	}
	return parseEnvelopes(lines), nil
}

// search runs IMAP SEARCH and returns sequence numbers.
func (s *imapSession) search(criteria string) ([]int, error) {
	tag := s.nextTag()
	if err := s.send("%s SEARCH %s", tag, criteria); err != nil {
		return nil, err
	}
	lines, err := s.expectOK(tag)
	if err != nil {
		return nil, err
	}
	var nums []int
	for _, l := range lines {
		if strings.HasPrefix(l, "* SEARCH") {
			parts := strings.Fields(l)
			for _, p := range parts[2:] {
				if n, e := strconv.Atoi(p); e == nil {
					nums = append(nums, n)
				}
			}
		}
	}
	return nums, nil
}

// fetchBody fetches the RFC822 body for a sequence number.
// Returns the raw message text (truncated to maxBytes if > 0).
func (s *imapSession) fetchBody(seqNum int, maxBytes int) (string, error) {
	tag := s.nextTag()
	if err := s.send("%s FETCH %d (BODY[HEADER.FIELDS (FROM TO SUBJECT DATE)] BODY[TEXT])", tag, seqNum); err != nil {
		return "", err
	}
	lines, err := s.expectOK(tag)
	if err != nil {
		return "", err
	}
	var sb strings.Builder
	for _, l := range lines {
		sb.WriteString(l)
		sb.WriteByte('\n')
	}
	result := sb.String()
	if maxBytes > 0 && len(result) > maxBytes {
		result = result[:maxBytes] + "\n...[truncated]"
	}
	return result, nil
}

// ── Envelope parser ──────────────────────────────────────────────────────

// parseEnvelopes does a best-effort parse of FETCH ENVELOPE responses.
// IMAP envelopes are complex nested structures; we extract Subject/From/Date
// with simple string scanning rather than a full parser.
func parseEnvelopes(lines []string) []MessageHeader {
	var headers []MessageHeader
	// Join lines to handle multi-line responses
	full := strings.Join(lines, "\n")

	// Each message block starts with "* N FETCH ("
	parts := strings.Split(full, "\n* ")
	for _, part := range parts {
		if !strings.Contains(part, "FETCH (") {
			continue
		}
		h := MessageHeader{}

		// Sequence number
		fields := strings.Fields(part)
		if len(fields) >= 1 {
			n := strings.TrimPrefix(fields[0], "*")
			n = strings.TrimSpace(n)
			h.SeqNum, _ = strconv.Atoi(n)
		}

		// FLAGS
		if i := strings.Index(part, "FLAGS ("); i >= 0 {
			end := strings.Index(part[i:], ")")
			if end >= 0 {
				h.Flags = part[i+7 : i+end]
			}
		}

		// ENVELOPE (date subject from ...)
		// Format: ENVELOPE ("date" "subject" (("name" NIL "user" "host")) ...)
		if i := strings.Index(part, "ENVELOPE ("); i >= 0 {
			env := part[i+10:]
			// date is first quoted string
			h.Date = extractQuoted(env, 0)
			// subject is second
			h.Subject = extractQuoted(env, 1)
			// from address - find first "user" "host" pair after third (
			h.From = extractAddress(env)
		}

		if h.SeqNum > 0 {
			headers = append(headers, h)
		}
	}
	return headers
}

// extractQuoted returns the nth quoted string in s (0-indexed).
func extractQuoted(s string, n int) string {
	count := 0
	i := 0
	for i < len(s) {
		if s[i] == '"' {
			start := i + 1
			i++
			for i < len(s) && s[i] != '"' {
				if s[i] == '\\' {
					i++
				}
				i++
			}
			if count == n {
				if i <= len(s) {
					return s[start:i]
				}
			}
			count++
		}
		i++
	}
	return ""
}

// extractAddress tries to pull a human-readable From address from the ENVELOPE from field.
func extractAddress(env string) string {
	// From field is 3rd item: (("personal" NIL "mailbox" "host"))
	// Find "mailbox" and "host" quoted strings after the first set of nested parens
	fromStart := strings.Index(env, "((")
	if fromStart < 0 {
		return ""
	}
	chunk := env[fromStart:]
	// chunk: (("personal" NIL "mailbox" "host") ...)
	personal := extractQuoted(chunk, 0)
	mailbox := extractQuoted(chunk, 2)
	host := extractQuoted(chunk, 3)
	addr := mailbox + "@" + host
	if personal != "" && personal != "NIL" {
		addr = personal + " <" + addr + ">"
	}
	return addr
}
