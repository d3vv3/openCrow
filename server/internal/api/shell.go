package api

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	shellMaxOutputLen   = 30_000
	shellDefaultTimeout = 30 * time.Second
	shellMaxTimeout     = 120 * time.Second
	processMaxOutputLen = 30_000
	sandboxRoot         = "/sandbox"
)

// blockedPatterns are regexes that match destructive shell commands.
var blockedPatterns = []*regexp.Regexp{
	regexp.MustCompile(`rm\s+-[^\s]*r[^\s]*\s+/(?:\s|$)`),
	regexp.MustCompile(`rm\s+-[^\s]*r[^\s]*\s+/\*`),
	regexp.MustCompile(`rm\s+-[^\s]*r[^\s]*\s+~(?:\s|$)`),
	regexp.MustCompile(`rm\s+-[^\s]*r[^\s]*\s+~/\*`),
	regexp.MustCompile(`mkfs\.`),
	regexp.MustCompile(`dd\s+.*if=/dev/(zero|urandom|random)`),
	regexp.MustCompile(`>\s*/dev/[sh]d[a-z]`),
	regexp.MustCompile(`:\(\)\s*\{.*\|.*&\s*\}\s*;?\s*:`),
	regexp.MustCompile(`chmod\s+-[^\s]*R[^\s]*\s+[0-7]+\s+/(?:\s|$)`),
	regexp.MustCompile(`\bshutdown\b`),
	regexp.MustCompile(`\breboot\b`),
	regexp.MustCompile(`\bhalt\b`),
	regexp.MustCompile(`\bpoweroff\b`),
	regexp.MustCompile(`\binit\s+[06]\b`),
	regexp.MustCompile(`format\s+[A-Za-z]:`),
}

var blockedEnvVars = map[string]bool{
	"PATH":                  true,
	"LD_PRELOAD":            true,
	"LD_LIBRARY_PATH":       true,
	"DYLD_INSERT_LIBRARIES": true,
	"DYLD_LIBRARY_PATH":     true,
	"DYLD_FRAMEWORK_PATH":   true,
}

func isCommandBlocked(command string) bool {
	for _, pat := range blockedPatterns {
		if pat.MatchString(command) {
			return true
		}
	}
	return false
}

// executeShellCommand runs a command in a fresh shell and returns structured result.
func executeShellCommand(ctx context.Context, shell, command string, timeout time.Duration, workingDir string, env map[string]string) map[string]any {
	if isCommandBlocked(command) {
		return map[string]any{"success": false, "error": "Command is blocked for safety reasons"}
	}

	if timeout <= 0 {
		timeout = shellDefaultTimeout
	}
	if timeout > shellMaxTimeout {
		timeout = shellMaxTimeout
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	if workingDir != "" {
		command = fmt.Sprintf("cd %s 2>/dev/null; %s", workingDir, command)
	}
	cmd := exec.CommandContext(execCtx, "chroot", sandboxRoot, shell, "-c", command)

	// Set filtered env vars
	if len(env) > 0 {
		for k, v := range env {
			if !blockedEnvVars[strings.ToUpper(k)] {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
			}
		}
	}

	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	started := time.Now()
	err := cmd.Run()
	duration := time.Since(started)

	timedOut := errors.Is(execCtx.Err(), context.DeadlineExceeded)
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else if timedOut {
			exitCode = 124
		} else {
			exitCode = 1
		}
	}

	stdout := stdoutBuf.String()
	if len(stdout) > shellMaxOutputLen {
		stdout = stdout[:shellMaxOutputLen-3] + "..."
	}
	stderr := stderrBuf.String()
	if len(stderr) > shellMaxOutputLen {
		stderr = stderr[:shellMaxOutputLen-3] + "..."
	}

	return map[string]any{
		"success":     exitCode == 0 && !timedOut,
		"stdout":      stdout,
		"stderr":      stderr,
		"exit_code":   exitCode,
		"timed_out":   timedOut,
		"duration_ms": duration.Milliseconds(),
	}
}

// ProcessManager tracks background shell sessions.
type ProcessManager struct {
	mu       sync.RWMutex
	sessions map[string]*ProcessSession
	nextID   atomic.Int64
}

// ProcessSession represents a background shell process.
type ProcessSession struct {
	ID        string
	Command   string
	StartTime time.Time

	mu       sync.Mutex
	process  *exec.Cmd
	stdout   bytes.Buffer
	stderr   bytes.Buffer
	finished bool
	exitCode int
	timedOut bool
}

func NewProcessManager() *ProcessManager {
	return &ProcessManager{
		sessions: make(map[string]*ProcessSession),
	}
}

// StartBackground launches a command in the background and returns immediately.
func (pm *ProcessManager) StartBackground(ctx context.Context, shell, command string, timeout time.Duration, workingDir string, env map[string]string) map[string]any {
	if isCommandBlocked(command) {
		return map[string]any{"success": false, "error": "Command is blocked for safety reasons"}
	}

	if timeout <= 0 {
		timeout = shellDefaultTimeout
	}
	if timeout > shellMaxTimeout {
		timeout = shellMaxTimeout
	}

	sessionID := fmt.Sprintf("bg-%d", pm.nextID.Add(1))

	cmd := exec.Command("chroot", sandboxRoot, shell, "-c", command)
	if workingDir != "" {
		// Prepend cd into the chroot command
		cmd = exec.Command("chroot", sandboxRoot, shell, "-c", fmt.Sprintf("cd %s 2>/dev/null; %s", workingDir, command))
	}
	if len(env) > 0 {
		for k, v := range env {
			if !blockedEnvVars[strings.ToUpper(k)] {
				cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
			}
		}
	}

	session := &ProcessSession{
		ID:        sessionID,
		Command:   command,
		StartTime: time.Now().UTC(),
		process:   cmd,
	}

	cmd.Stdout = &limitedWriter{buf: &session.stdout, max: processMaxOutputLen}
	cmd.Stderr = &limitedWriter{buf: &session.stderr, max: processMaxOutputLen}

	if err := cmd.Start(); err != nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("failed to start: %v", err)}
	}

	pm.mu.Lock()
	pm.sessions[sessionID] = session
	pm.mu.Unlock()

	// Monitor in background
	go func() {
		done := make(chan error, 1)
		go func() { done <- cmd.Wait() }()

		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case err := <-done:
			session.mu.Lock()
			session.finished = true
			if err != nil {
				var exitErr *exec.ExitError
				if errors.As(err, &exitErr) {
					session.exitCode = exitErr.ExitCode()
				} else {
					session.exitCode = 1
				}
			}
			session.mu.Unlock()
		case <-timer.C:
			_ = cmd.Process.Kill()
			session.mu.Lock()
			session.finished = true
			session.timedOut = true
			session.exitCode = 124
			session.mu.Unlock()
		}
	}()

	return map[string]any{
		"success":    true,
		"session_id": sessionID,
		"status":     "running",
		"message":    "Process started in background. Use manage_process tool to check status.",
	}
}

// List returns all sessions.
func (pm *ProcessManager) List() map[string]any {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	running := make([]map[string]any, 0)
	finished := make([]map[string]any, 0)

	for _, s := range pm.sessions {
		info := s.toInfo()
		s.mu.Lock()
		done := s.finished
		s.mu.Unlock()
		if done {
			finished = append(finished, info)
		} else {
			running = append(running, info)
		}
	}

	return map[string]any{
		"running":  running,
		"finished": finished,
		"total":    len(pm.sessions),
	}
}

// Log returns output from a session.
func (pm *ProcessManager) Log(sessionID string, offset, limit int) map[string]any {
	pm.mu.RLock()
	session, ok := pm.sessions[sessionID]
	pm.mu.RUnlock()
	if !ok {
		return map[string]any{"success": false, "error": fmt.Sprintf("unknown session: %s", sessionID)}
	}

	session.mu.Lock()
	stdout := session.stdout.String()
	stderr := session.stderr.String()
	status := "running"
	if session.finished {
		status = "finished"
	}
	exitCode := session.exitCode
	timedOut := session.timedOut
	session.mu.Unlock()

	lines := strings.Split(stdout, "\n")
	end := offset + limit
	if end > len(lines) {
		end = len(lines)
	}
	if offset > len(lines) {
		offset = len(lines)
	}
	sliced := strings.Join(lines[offset:end], "\n")

	if len(stderr) > 2000 {
		stderr = stderr[len(stderr)-2000:]
	}

	return map[string]any{
		"success":            true,
		"session_id":         sessionID,
		"status":             status,
		"exit_code":          exitCode,
		"stdout":             sliced,
		"stderr":             stderr,
		"total_stdout_lines": len(lines),
		"offset":             offset,
		"timed_out":          timedOut,
	}
}

// Kill terminates a running session.
func (pm *ProcessManager) Kill(sessionID string) map[string]any {
	pm.mu.RLock()
	session, ok := pm.sessions[sessionID]
	pm.mu.RUnlock()
	if !ok {
		return map[string]any{"success": false, "error": fmt.Sprintf("unknown session: %s", sessionID)}
	}

	session.mu.Lock()
	defer session.mu.Unlock()

	if session.finished {
		return map[string]any{"success": true, "message": "process already finished", "exit_code": session.exitCode}
	}

	if session.process != nil && session.process.Process != nil {
		_ = session.process.Process.Kill()
	}
	session.finished = true
	session.exitCode = -1
	return map[string]any{"success": true, "message": "process killed"}
}

// Remove deletes a session from tracking.
func (pm *ProcessManager) Remove(sessionID string) map[string]any {
	pm.mu.Lock()
	session, ok := pm.sessions[sessionID]
	if !ok {
		pm.mu.Unlock()
		return map[string]any{"success": false, "error": fmt.Sprintf("unknown session: %s", sessionID)}
	}
	delete(pm.sessions, sessionID)
	pm.mu.Unlock()

	session.mu.Lock()
	if !session.finished && session.process != nil && session.process.Process != nil {
		_ = session.process.Process.Kill()
	}
	session.mu.Unlock()

	return map[string]any{"success": true, "message": "session removed"}
}

func (s *ProcessSession) toInfo() map[string]any {
	s.mu.Lock()
	defer s.mu.Unlock()
	status := "running"
	if s.finished {
		status = "finished"
	}
	return map[string]any{
		"session_id":       s.ID,
		"command":          s.Command,
		"status":           status,
		"exit_code":        s.exitCode,
		"duration_seconds": int(time.Since(s.StartTime).Seconds()),
		"timed_out":        s.timedOut,
		"stdout_length":    s.stdout.Len(),
	}
}

// limitedWriter writes to a buffer up to max bytes.
type limitedWriter struct {
	buf *bytes.Buffer
	max int
}

func (lw *limitedWriter) Write(p []byte) (int, error) {
	if lw.buf.Len() >= lw.max {
		return len(p), nil // discard but pretend we wrote
	}
	remaining := lw.max - lw.buf.Len()
	if len(p) > remaining {
		lw.buf.Write(p[:remaining])
		return len(p), nil
	}
	return lw.buf.Write(p)
}
