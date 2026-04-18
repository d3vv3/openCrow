package api

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/creack/pty"
	"github.com/gorilla/websocket"
)

var wsUpgrader = websocket.Upgrader{
	CheckOrigin:     func(r *http.Request) bool { return true },
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
}

// resolveShell returns the best available interactive shell on this system.
// Prefers bash; falls back to sh if bash is not installed (e.g. Alpine).
func resolveShell() string {
	for _, sh := range []string{"/bin/bash", "/usr/bin/bash", "/bin/sh"} {
		if _, err := os.Stat(sh); err == nil {
			return sh
		}
	}
	return "/bin/sh"
}

// terminalSession holds an active PTY session for one user.
type terminalSession struct {
	ptmx   *os.File
	cmd    *exec.Cmd
	mu     sync.Mutex
	closed bool
}

func (ts *terminalSession) close() {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if ts.closed {
		return
	}
	ts.closed = true
	_ = ts.ptmx.Close()
	if ts.cmd.Process != nil {
		_ = ts.cmd.Process.Kill()
	}
}

// TerminalSessionManager keeps one persistent PTY per user (keyed by userID)
// and tracks active WebSocket connections so output can be broadcast directly.
type TerminalSessionManager struct {
	mu       sync.Mutex
	sessions map[string]*terminalSession
	// wsConns tracks active WebSocket connections per user for direct broadcast.
	wsConns map[string]map[*websocket.Conn]*sync.Mutex
	wsMu    sync.Mutex
}

func NewTerminalSessionManager() *TerminalSessionManager {
	return &TerminalSessionManager{
		sessions: make(map[string]*terminalSession),
		wsConns:  make(map[string]map[*websocket.Conn]*sync.Mutex),
	}
}

// registerConn registers a WebSocket connection for a user.
func (m *TerminalSessionManager) registerConn(userID string, conn *websocket.Conn) {
	m.wsMu.Lock()
	defer m.wsMu.Unlock()
	if m.wsConns[userID] == nil {
		m.wsConns[userID] = make(map[*websocket.Conn]*sync.Mutex)
	}
	m.wsConns[userID][conn] = &sync.Mutex{}
}

// unregisterConn removes a WebSocket connection for a user.
func (m *TerminalSessionManager) unregisterConn(userID string, conn *websocket.Conn) {
	m.wsMu.Lock()
	defer m.wsMu.Unlock()
	delete(m.wsConns[userID], conn)
	if len(m.wsConns[userID]) == 0 {
		delete(m.wsConns, userID)
		// No active viewers left: close and drop PTY session so next open gets
		// a fresh interactive shell with prompt.
		m.mu.Lock()
		if ts, ok := m.sessions[userID]; ok {
			delete(m.sessions, userID)
			go ts.close()
		}
		m.mu.Unlock()
	}
}

// BroadcastOutput sends terminal display data directly to all WebSocket connections
// for a user, bypassing the PTY stdin entirely.
// This is used by the AI tool executor to show command banners without the shell
// misinterpreting them as input.
func (m *TerminalSessionManager) BroadcastOutput(userID string, data []byte) {
	m.wsMu.Lock()
	type connState struct {
		conn *websocket.Conn
		mu   *sync.Mutex
	}
	conns := make([]connState, 0, len(m.wsConns[userID]))
	for c, mu := range m.wsConns[userID] {
		conns = append(conns, connState{conn: c, mu: mu})
	}
	m.wsMu.Unlock()

	for _, c := range conns {
		c.mu.Lock()
		_ = c.conn.WriteMessage(websocket.BinaryMessage, data)
		c.mu.Unlock()
	}
}

// getOrCreate returns an existing live PTY session or starts a fresh one.
func (m *TerminalSessionManager) getOrCreate(userID, shell string) (*terminalSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// If session exists and shell is still alive, reuse it.
	if ts, ok := m.sessions[userID]; ok {
		ts.mu.Lock()
		dead := ts.closed
		ts.mu.Unlock()
		if !dead {
			return ts, nil
		}
		delete(m.sessions, userID)
	}

	if shell == "" {
		shell = resolveShell()
	}

	cmd := exec.Command("chroot", sandboxRoot, shell)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptmx, err := pty.Start(cmd)
	if err != nil {
		return nil, err
	}

	ts := &terminalSession{ptmx: ptmx, cmd: cmd}
	m.sessions[userID] = ts

	// Single PTY reader per user session: broadcast output to all active WS clients.
	// This avoids multiple concurrent readers stealing PTY output after reconnects.
	go func() {
		buf := make([]byte, 4096)
		for {
			n, rerr := ts.ptmx.Read(buf)
			if n > 0 {
				out := make([]byte, n)
				copy(out, buf[:n])
				m.BroadcastOutput(userID, out)
			}
			if rerr != nil {
				return
			}
		}
	}()

	// Reap dead sessions when the shell exits
	go func() {
		_ = cmd.Wait()
		ts.mu.Lock()
		ts.closed = true
		ts.mu.Unlock()
		m.mu.Lock()
		if m.sessions[userID] == ts {
			delete(m.sessions, userID)
		}
		m.mu.Unlock()
	}()

	return ts, nil
}

// ptyResizeMsg is the JSON sent by the client to resize the terminal.
type ptyResizeMsg struct {
	Type string `json:"type"` // "resize"
	Cols uint16 `json:"cols"`
	Rows uint16 `json:"rows"`
}

// handleTerminalWS upgrades to WebSocket and bridges stdin/stdout with a PTY.
func (s *Server) handleTerminalWS(w http.ResponseWriter, r *http.Request) {
	userID := userIDFromContext(r.Context())

	// Shell is fixed.
	shell := resolveShell()

	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("terminal ws upgrade error: %v", err)
		return
	}
	defer conn.Close()

	// Track this connection so BroadcastOutput can reach it.
	s.termMgr.registerConn(userID, conn)
	defer s.termMgr.unregisterConn(userID, conn)

	ts, err := s.termMgr.getOrCreate(userID, shell)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("failed to start terminal: "+err.Error()))
		return
	}

	conn.SetReadDeadline(time.Time{}) // no deadline

	// WebSocket -> PTY (receive keystrokes / resize from browser)
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			return
		}
		if msgType == websocket.TextMessage {
			// Try to parse as a resize message
			var msg ptyResizeMsg
			if json.Unmarshal(data, &msg) == nil && msg.Type == "resize" && msg.Cols > 0 && msg.Rows > 0 {
				setTermSize(ts.ptmx, msg.Cols, msg.Rows)
				continue
			}
			// Otherwise treat as text input
			_, _ = ts.ptmx.Write(data)
		} else if msgType == websocket.BinaryMessage {
			_, _ = io.Copy(ts.ptmx, newBytesReader(data))
		}
	}
}

func setTermSize(f *os.File, cols, rows uint16) {
	ws := struct {
		rows, cols, xpixel, ypixel uint16
	}{rows, cols, 0, 0}
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(&ws)))
}

// minimal io.Reader over a []byte
type bytesReader struct{ data []byte; pos int }
func newBytesReader(b []byte) *bytesReader { return &bytesReader{data: b} }
func (r *bytesReader) Read(p []byte) (int, error) {
	if r.pos >= len(r.data) { return 0, io.EOF }
	n := copy(p, r.data[r.pos:])
	r.pos += n
	return n, nil
}
