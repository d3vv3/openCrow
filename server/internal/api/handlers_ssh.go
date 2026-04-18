package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type TestSSHConnectionRequest struct {
	Host       string `json:"host"`
	Port       int    `json:"port,omitempty"`
	Username   string `json:"username"`
	AuthMode   string `json:"authMode"` // "key" or "password"
	SSHKey     string `json:"sshKey,omitempty"`
	Password   string `json:"password,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
}

func buildSSHClientConfig(username, authMode, sshKey, password, passphrase string) (*ssh.ClientConfig, error) {
	cfg := &ssh.ClientConfig{
		User:            username,
		Timeout:         10 * time.Second,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}
	if authMode == "password" {
		cfg.Auth = []ssh.AuthMethod{ssh.Password(password)}
	} else {
		keyBytes := []byte(strings.TrimSpace(sshKey))
		var signer ssh.Signer
		var err error
		if passphrase != "" {
			signer, err = ssh.ParsePrivateKeyWithPassphrase(keyBytes, []byte(passphrase))
		} else {
			signer, err = ssh.ParsePrivateKey(keyBytes)
		}
		if err != nil {
			return nil, fmt.Errorf("invalid SSH key: %w", err)
		}
		cfg.Auth = []ssh.AuthMethod{ssh.PublicKeys(signer)}
	}
	return cfg, nil
}

func (s *Server) handleTestSSHConnection(w http.ResponseWriter, r *http.Request) {
	var req TestSSHConnectionRequest
	if err := decodeJSON(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	host := strings.TrimSpace(req.Host)
	if host == "" {
		writeError(w, http.StatusBadRequest, "host required")
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		writeError(w, http.StatusBadRequest, "username required")
		return
	}
	port := req.Port
	if port <= 0 {
		port = 22
	}

	cfg, err := buildSSHClientConfig(username, req.AuthMode, req.SSHKey, req.Password, req.Passphrase)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	addr := fmt.Sprintf("%s:%d", host, port)
	client, err := ssh.Dial("tcp", addr, cfg)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "connected but failed to open session: " + err.Error()})
		return
	}
	defer session.Close()

	if _, err := session.CombinedOutput("echo ok"); err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": false, "error": "session test failed: " + err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
