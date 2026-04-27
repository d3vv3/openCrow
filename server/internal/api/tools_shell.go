// tools_shell.go — Shell command execution, remote SSH, and process management tools.
package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/opencrow/opencrow/server/internal/configstore"
	"golang.org/x/crypto/ssh"
)

// processManager is the singleton process manager for background shell sessions.
var processManager = NewProcessManager()

func (s *Server) toolExecuteShellCommand(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	command, _ := args["command"].(string)
	if command == "" {
		return map[string]any{"success": false, "error": "command is required"}, nil
	}

	timeout := s.serverShellTimeout
	if t, ok := args["timeout"].(float64); ok && t > 0 {
		timeout = time.Duration(t) * time.Second
	}

	workingDir, _ := args["working_dir"].(string)

	var env map[string]string
	if envMap, ok := args["env"].(map[string]any); ok {
		env = make(map[string]string, len(envMap))
		for k, v := range envMap {
			env[k] = fmt.Sprint(v)
		}
	}

	// Shell is fixed.
	shell := resolveShell()

	// Background mode
	if bg, ok := args["background"].(bool); ok && bg {
		result := processManager.StartBackground(ctx, shell, command, timeout, workingDir, env)
		s.writeCommandToTerminal(userID, command, "", true)
		return result, nil
	}

	result := executeShellCommand(ctx, shell, command, timeout, workingDir, env)
	// Mirror command + output into user's xterm PTY (if connected)
	stdout, _ := result["stdout"].(string)
	stderr, _ := result["stderr"].(string)
	combined := stdout
	if stderr != "" {
		combined += stderr
	}
	s.writeCommandToTerminal(userID, command, combined, false)
	return result, nil
}

func (s *Server) toolRemoteExecute(ctx context.Context, userID string, args map[string]any) (map[string]any, error) {
	serverName, _ := args["serverName"].(string)
	if serverName == "" {
		return map[string]any{"success": false, "error": "serverName is required"}, nil
	}
	command, _ := args["command"].(string)
	if command == "" {
		return map[string]any{"success": false, "error": "command is required"}, nil
	}

	cfg, err := s.configStore.GetUserConfig(userID)
	if err != nil {
		return map[string]any{"success": false, "error": "failed to load config"}, nil
	}

	var srv *configstore.SSHServerConfig
	for i := range cfg.Integrations.SSHServers {
		if strings.EqualFold(cfg.Integrations.SSHServers[i].Name, serverName) {
			srv = &cfg.Integrations.SSHServers[i]
			break
		}
	}
	if srv == nil {
		return map[string]any{"success": false, "error": fmt.Sprintf("server %q not found", serverName)}, nil
	}
	if !srv.Enabled {
		return map[string]any{"success": false, "error": fmt.Sprintf("server %q is disabled", serverName)}, nil
	}

	timeout := 300 * time.Second
	if t, ok := args["timeout"].(float64); ok && t > 0 {
		timeout = time.Duration(t) * time.Second
	}
	workingDir, _ := args["working_dir"].(string)
	background, _ := args["background"].(bool)

	sshCfg, err := buildSSHClientConfig(srv.Username, srv.AuthMode, srv.SSHKey, srv.Password, srv.Passphrase, srv.KnownHostKey)
	if err != nil {
		return map[string]any{"success": false, "error": err.Error()}, nil
	}

	port := srv.Port
	if port <= 0 {
		port = 22
	}
	addr := fmt.Sprintf("%s:%d", srv.Host, port)
	client, err := ssh.Dial("tcp", addr, sshCfg)
	if err != nil {
		return map[string]any{"success": false, "error": "ssh dial: " + err.Error()}, nil
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		return map[string]any{"success": false, "error": "ssh session: " + err.Error()}, nil
	}
	defer session.Close()

	cmd := command
	if workingDir != "" {
		cmd = fmt.Sprintf("cd %s && %s", workingDir, cmd)
	}
	if background {
		cmd = fmt.Sprintf("nohup sh -c %s </dev/null >nohup.out 2>&1 & echo $!", shellescape(cmd))
	}

	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	type result struct {
		out []byte
		err error
	}
	ch := make(chan result, 1)
	go func() {
		out, err := session.CombinedOutput(cmd)
		ch <- result{out, err}
	}()

	select {
	case <-execCtx.Done():
		_ = session.Signal(ssh.SIGKILL)
		return map[string]any{"success": false, "stdout": "", "stderr": "timeout", "exitCode": -1}, nil
	case res := <-ch:
		exitCode := 0
		if res.err != nil {
			if exitErr, ok := res.err.(*ssh.ExitError); ok {
				exitCode = exitErr.ExitStatus()
			} else {
				return map[string]any{"success": false, "error": res.err.Error()}, nil
			}
		}
		output := truncateOutput(string(res.out), 32000)
		return map[string]any{"success": true, "stdout": output, "stderr": "", "exitCode": exitCode}, nil
	}
}

func shellescape(s string) string {
	return "'" + strings.ReplaceAll(s, "'", "'\"'\"'") + "'"
}

func (s *Server) toolManageProcess(args map[string]any) (map[string]any, error) {
	action, _ := args["action"].(string)
	if action == "" {
		return map[string]any{"success": false, "error": "action is required (list, log, kill, remove)"}, nil
	}

	switch action {
	case "list":
		return processManager.List(), nil
	case "log":
		sessionID, _ := args["session_id"].(string)
		if sessionID == "" {
			return map[string]any{"success": false, "error": "session_id is required for log"}, nil
		}
		offset := 0
		limit := 200
		if o, ok := args["offset"].(float64); ok {
			offset = int(o)
		}
		if l, ok := args["limit"].(float64); ok {
			limit = int(l)
		}
		return processManager.Log(sessionID, offset, limit), nil
	case "kill":
		sessionID, _ := args["session_id"].(string)
		if sessionID == "" {
			return map[string]any{"success": false, "error": "session_id is required for kill"}, nil
		}
		return processManager.Kill(sessionID), nil
	case "remove":
		sessionID, _ := args["session_id"].(string)
		if sessionID == "" {
			return map[string]any{"success": false, "error": "session_id is required for remove"}, nil
		}
		return processManager.Remove(sessionID), nil
	default:
		return map[string]any{"success": false, "error": fmt.Sprintf("unknown action: %s. Use: list, log, kill, remove", action)}, nil
	}
}

// writeCommandToTerminal mirrors an AI-executed shell command + its output into
// the user's persistent PTY terminal session (so it appears in TerminalView).
// Uses ANSI escape codes to visually distinguish AI-driven commands from manual ones.
func (s *Server) writeCommandToTerminal(userID, command, output string, background bool) {
	if s.termMgr == nil {
		return
	}

	bgSuffix := ""
	if background {
		bgSuffix = " &"
	}

	// Build the display block:
	// Dim separator, bold cyan prompt prefix, command, output (if any)
	var buf strings.Builder
	buf.WriteString("\r\n\x1b[2m-- [AI] command --------------------\x1b[0m\r\n")
	buf.WriteString("\x1b[1;36m> \x1b[0m\x1b[1m")
	buf.WriteString(command)
	buf.WriteString(bgSuffix)
	buf.WriteString("\x1b[0m\r\n")
	if output != "" {
		// Prefix each line so it's indented slightly
		for _, line := range strings.Split(strings.TrimRight(output, "\n"), "\n") {
			buf.WriteString("  ")
			buf.WriteString(line)
			buf.WriteString("\r\n")
		}
	}
	buf.WriteString("\x1b[2m───────────────────────────────────\x1b[0m\r\n")

	s.termMgr.BroadcastOutput(userID, []byte(buf.String()))
}

func (s *Server) writeToolCallToTerminal(userID, kind, name string, args map[string]any, result any, execErr error) {
	if s.termMgr == nil {
		return
	}

	if kind == "" {
		kind = "TOOL"
	}

	argsJSON, _ := json.Marshal(args)
	resultJSON, _ := json.Marshal(result)

	status := "ok"
	preview := ""
	if execErr != nil {
		status = "error"
		preview = execErr.Error()
	} else {
		preview = string(resultJSON)
	}

	var buf strings.Builder
	buf.WriteString("\r\n\x1b[2m-- [AI] tool -----------------------\x1b[0m\r\n")
	buf.WriteString("\x1b[1;35m[")
	buf.WriteString(kind)
	buf.WriteString("]\x1b[0m ")
	buf.WriteString("\x1b[1m")
	buf.WriteString(name)
	buf.WriteString("\x1b[0m")
	if len(argsJSON) > 0 && string(argsJSON) != "null" {
		buf.WriteString(" ")
		buf.WriteString(truncateOutput(string(argsJSON), 240))
	}
	buf.WriteString("\r\n")
	buf.WriteString("  status: ")
	buf.WriteString(status)
	buf.WriteString("\r\n")
	if strings.TrimSpace(preview) != "" {
		for _, line := range strings.Split(strings.TrimRight(truncateOutput(preview, 420), "\n"), "\n") {
			buf.WriteString("  ")
			buf.WriteString(line)
			buf.WriteString("\r\n")
		}
	}
	buf.WriteString("\x1b[2m───────────────────────────────────\x1b[0m\r\n")

	s.termMgr.BroadcastOutput(userID, []byte(buf.String()))
}
