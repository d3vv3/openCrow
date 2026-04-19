package api

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"

	"github.com/opencrow/opencrow/server/internal/configstore"
	"github.com/opencrow/opencrow/server/internal/orchestrator"
)

// sanitizeToolName ensures a tool name is valid for all LLM providers (including Gemini).
// Rules: must start with letter or underscore, only [a-zA-Z0-9_.-:] allowed, max 128 chars.
var reToolNameInvalid = regexp.MustCompile(`[^a-zA-Z0-9_.:\-]`)

func sanitizeToolName(name string) string {
	// Replace invalid characters with underscores
	s := reToolNameInvalid.ReplaceAllString(name, "_")
	// Ensure first character is letter or underscore
	if len(s) > 0 && s[0] >= '0' && s[0] <= '9' {
		s = "_" + s
	}
	if len(s) > 128 {
		s = s[:128]
	}
	if s == "" {
		s = "tool"
	}
	return s
}

func decodeJSON(r *http.Request, target any) error {
	defer r.Body.Close()
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return errors.New("multiple JSON values")
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, ErrorResponse{Error: message})
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func chooseDeviceLabel(inBody, inHeader string) string {
	device := strings.TrimSpace(inBody)
	if device != "" {
		return device
	}
	device = strings.TrimSpace(inHeader)
	if device != "" {
		return device
	}
	return "unknown-device"
}

func bearerToken(header string) string {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func isUUID(value string) bool {
	_, err := uuid.Parse(value)
	return err == nil
}

func truncateOutput(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	if max <= 3 {
		return value[:max]
	}
	return value[:max-3] + "..."
}

func refreshTokenDigest(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func hashRefreshToken(token string) (string, error) {
	digest := refreshTokenDigest(token)
	hash, err := bcrypt.GenerateFromPassword([]byte(digest), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func verifyRefreshTokenHash(storedHash, token string) error {
	digest := refreshTokenDigest(token)

	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(digest)); err == nil {
		return nil
	}

	// Backward compatibility for any pre-existing sessions that were hashed directly from raw token bytes.
	if err := bcrypt.CompareHashAndPassword([]byte(storedHash), []byte(token)); err == nil {
		return nil
	}

	return errors.New("refresh token mismatch")
}

type sessionRow struct {
	ID               string
	UserID           string
	RefreshTokenHash string
	CreatedAt        time.Time
	LastSeenAt       time.Time
	DeviceLabel      string
}

// buildSystemPrompt constructs the full system prompt by appending any stored
// memories as a context section below the configured system prompt.
func (s *Server) buildSystemPrompt(ctx context.Context, userID string, cfg *configstore.UserConfig) string {
	base := configstore.DefaultSystemPrompt
	if cfg != nil && cfg.Prompts.SystemPrompt != "" {
		base = cfg.Prompts.SystemPrompt
	}

	preferredTZ := preferredTimezoneName(ctx, cfg, "")

	var sb strings.Builder
	sb.WriteString(base)
	if preferredTZ != "" {
		sb.WriteString("\n\n## Timezone\n")
		sb.WriteString(fmt.Sprintf("Default to the user's local timezone `%s` when discussing current time or date unless the user explicitly asks for a different timezone.\n", preferredTZ))
	}

	memories, _ := s.listMemories(ctx, userID)
	if len(memories) > 0 {
		sb.WriteString("\n\n## Your Memories\n")
		sb.WriteString("The following are things you have learned about the user. Use them to personalise your responses:\n\n")

		// Group by category
		byCategory := make(map[string][]MemoryDTO)
		order := []string{}
		for _, m := range memories {
			cat := m.Category
			if _, seen := byCategory[cat]; !seen {
				order = append(order, cat)
			}
			byCategory[cat] = append(byCategory[cat], m)
		}
		for _, cat := range order {
			sb.WriteString(fmt.Sprintf("### %s\n", cat))
			for _, m := range byCategory[cat] {
				sb.WriteString(fmt.Sprintf("- [%s] %s\n", m.ID, m.Content))
			}
		}
	}

	// Append email account context
	inboxes, err2 := s.listEmailInboxes(ctx, userID)
	if err2 == nil && len(inboxes) > 0 {
		sb.WriteString("\n\n## Email Accounts\n")
		sb.WriteString("You have access to the following email inboxes via the email tools:\n\n")
		for _, inbox := range inboxes {
			status := "inactive"
			if inbox.Active {
				status = "active"
			}
			sb.WriteString(fmt.Sprintf("- %s (IMAP: %s, status: %s)\n", inbox.Address, inbox.ImapHost, status))
		}
	}

	// Append available skills (file-based)
	if s.skillStore != nil {
		if skills, err := s.skillStore.List(); err == nil && len(skills) > 0 {
			sb.WriteString("\n\n## Available Skills\n")
			sb.WriteString("The following skills are installed. Use `get_skill` with the slug to read full instructions:\n\n")
			for _, sk := range skills {
				sb.WriteString(fmt.Sprintf("- **%s** (`%s`): %s\n", sk.Name, sk.Slug, sk.Description))
			}
		}
	}

	// Append configured MCP servers (names only; do not discover tools per-request).
	if cfg != nil && len(cfg.MCP.Servers) > 0 {
		sb.WriteString("\n\n## MCP Servers\n")
		sb.WriteString("Configured MCP server connections. Use available MCP tools when needed; do not invent MCP tools.\n\n")
		for _, srv := range cfg.MCP.Servers {
			name := strings.TrimSpace(srv.Name)
			if name == "" {
				name = "MCP Server"
			}
			if !srv.Enabled {
				sb.WriteString(fmt.Sprintf("- **%s** (`%s`) -- disabled\n", name, strings.TrimSpace(srv.URL)))
				continue
			}

			url := strings.TrimSpace(srv.URL)
			if url == "" {
				sb.WriteString(fmt.Sprintf("- **%s** -- enabled\n", name))
				continue
			}
			sb.WriteString(fmt.Sprintf("- **%s** (`%s`) -- enabled\n", name, url))
		}
	}

	return sb.String()
}

// buildProvidersFromConfig returns an ordered list of orchestrator providers from
// the user's LLM config, sorted by Priority ascending (0 = highest priority).
// Only enabled providers with a valid kind are included.
func buildProvidersFromConfig(cfg *configstore.UserConfig) []orchestrator.Provider {
	if cfg == nil {
		return nil
	}
	sorted := make([]configstore.ProviderConfig, len(cfg.LLM.Providers))
	copy(sorted, cfg.LLM.Providers)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})
	var providers []orchestrator.Provider
	for _, p := range sorted {
		if !p.Enabled {
			continue
		}
		prov := orchestrator.BuildProvider(p.Name, p.Kind, p.BaseURL, p.APIKeyRef, p.Model)
		if prov != nil {
			providers = append(providers, prov)
		}
	}
	return providers
}
