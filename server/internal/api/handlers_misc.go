package api

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok", Name: "openCrow-server", Env: s.env})
}

func (s *Server) handleRunServerCommand(w http.ResponseWriter, r *http.Request) {
	var req RunServerCommandRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	commandText := strings.TrimSpace(req.Command)
	if commandText == "" {
		writeError(w, http.StatusBadRequest, "command is required")
		return
	}

	timeout := s.serverShellTimeout
	if req.TimeoutSeconds > 0 {
		requested := time.Duration(req.TimeoutSeconds) * time.Second
		if requested > 60*time.Second {
			requested = 60 * time.Second
		}
		timeout = requested
	}

	shell := "/bin/sh"
	if s.configStore != nil {
		if cfg, err := s.configStore.GetUserConfig(userIDFromContext(r.Context())); err == nil {
			candidate := strings.TrimSpace(cfg.LinuxSandbox.Shell)
			if candidate != "" {
				shell = candidate
			}
		}
	}

	started := time.Now().UTC()
	execCtx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	cmd := exec.CommandContext(execCtx, shell, "-lc", commandText)
	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf

	err := cmd.Run()
	finished := time.Now().UTC()

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

	resp := RunServerCommandResponse{
		Shell:      shell,
		Command:    commandText,
		ExitCode:   exitCode,
		Stdout:     truncateOutput(stdoutBuf.String(), 32_000),
		Stderr:     truncateOutput(stderrBuf.String(), 32_000),
		DurationMS: finished.Sub(started).Milliseconds(),
		TimedOut:   timedOut,
		StartedAt:  started.Format(time.RFC3339),
		FinishedAt: finished.Format(time.RFC3339),
	}

	status := http.StatusOK
	if timedOut {
		status = http.StatusGatewayTimeout
	} else if err != nil {
		status = http.StatusBadRequest
	}

	writeJSON(w, status, resp)
}

// handleWorkerStatus returns the latest health snapshot of all background workers.
func (s *Server) handleWorkerStatus(w http.ResponseWriter, _ *http.Request) {
	workers := s.workerStatus.all()
	writeJSON(w, http.StatusOK, map[string]any{"workers": workers})
}

// handleWorkerLogs returns recent log lines for a specific worker.
// Query param: worker=task-worker|heartbeat-worker|email-worker
func (s *Server) handleWorkerLogs(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("worker")
	if name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "worker query param required"})
		return
	}
	entries := s.workerLogs.Get(name)
	if entries == nil {
		entries = []WorkerLogEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"worker": name, "entries": entries})
}
