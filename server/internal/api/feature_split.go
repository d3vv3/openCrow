package api

import "net/http"

type FeatureSplitResponse struct {
	Server []string `json:"server"`
	Client []string `json:"client"`
}

func (s *Server) handleFeatureSplit(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, FeatureSplitResponse{
		Server: []string{
			"Authentication and session management",
			"Conversation and message persistence",
			"Memory store and promotion pipeline",
			"LLM provider orchestration with fallback and retries",
			"Server-side tool execution and tool-loop safeguards",
			"Task scheduling, heartbeat, and automation workers",
			"Email polling and actions",
			"MCP server management and invocation",
			"Realtime bridge for client/device tool requests",
			"Settings backup/export/import APIs",
		},
		Client: []string{
			"Web UI rendering and interaction",
			"Single-user login/session handling",
			"Conversation list and message view",
			"Realtime event consumption",
			"Channel/device-specific rendering adapters",
			"Device-local tool execution (native clients, future)",
			"No dynamic UI generation feature",
		},
	})
}
