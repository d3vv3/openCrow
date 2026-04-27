package api

import (
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

type TestSSHConnectionRequest struct {
	Host         string `json:"host"`
	Port         int    `json:"port,omitempty"`
	Username     string `json:"username"`
	AuthMode     string `json:"authMode"` // "key" or "password"
	SSHKey       string `json:"sshKey,omitempty"`
	Password     string `json:"password,omitempty"`
	Passphrase   string `json:"passphrase,omitempty"`
	KnownHostKey string `json:"knownHostKey,omitempty"`
}

// buildSSHClientConfig constructs an ssh.ClientConfig.
// If knownHostKey is non-empty it must be a base64-encoded public key blob
// (the second field in an authorized_keys / known_hosts line, e.g. "AAAA...").
// When knownHostKey is empty the connection proceeds with InsecureIgnoreHostKey
// and a warning is logged -- the caller should surface this to the user.
func buildSSHClientConfig(username, authMode, sshKey, password, passphrase, knownHostKey string) (*ssh.ClientConfig, error) {
	var hostKeyCallback ssh.HostKeyCallback
	if knownHostKey != "" {
		decoded, err := base64.StdEncoding.DecodeString(knownHostKey)
		if err != nil {
			return nil, fmt.Errorf("invalid knownHostKey (expected base64): %w", err)
		}
		pubKey, err := ssh.ParsePublicKey(decoded)
		if err != nil {
			return nil, fmt.Errorf("invalid knownHostKey (not a valid SSH public key): %w", err)
		}
		hostKeyCallback = ssh.FixedHostKey(pubKey)
	} else {
		log.Printf("WARNING: SSH connection for user %q has no knownHostKey set -- host identity will not be verified (MITM risk)", username)
		//nolint:gosec // Intentional backward-compat: warn but proceed when no key is configured.
		hostKeyCallback = ssh.InsecureIgnoreHostKey()
	}

	cfg := &ssh.ClientConfig{
		User:            username,
		Timeout:         10 * time.Second,
		HostKeyCallback: hostKeyCallback,
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

// @Summary Test an SSH connection
// @Tags    ssh
// @Security BearerAuth
// @Accept  json
// @Produce json
// @Param   body body TestSSHConnectionRequest true "SSH connection parameters"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} ErrorResponse
// @Failure 401 {object} ErrorResponse
// @Router  /v1/ssh/test [post]
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

	cfg, err := buildSSHClientConfig(username, req.AuthMode, req.SSHKey, req.Password, req.Passphrase, req.KnownHostKey)
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
