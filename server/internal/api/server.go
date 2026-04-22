package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	httpSwagger "github.com/swaggo/http-swagger/v2"

	_ "github.com/opencrow/opencrow/server/docs"
	"github.com/opencrow/opencrow/server/internal/auth"
	"github.com/opencrow/opencrow/server/internal/configstore"
	"github.com/opencrow/opencrow/server/internal/orchestrator"
	"github.com/opencrow/opencrow/server/internal/realtime"
	"github.com/opencrow/opencrow/server/internal/scheduler"
)

type contextKey string

const userIDContextKey contextKey = "userID"
const sessionIDContextKey contextKey = "sessionID"
const clientTimezoneContextKey contextKey = "clientTimezone"

type Server struct {
	env                 string
	db                  *pgxpool.Pool
	authMgr             *auth.Manager
	mux                 *http.ServeMux
	orchestrator        *orchestrator.Service
	realtimeHub         *realtime.Hub
	backoffPolicy       scheduler.BackoffPolicy
	configStore         *configstore.Store
	adminUsername       string
	adminPasswordBcrypt string
	serverShellTimeout  time.Duration
	workerStatus        WorkerStatusStore
	workerLogs          WorkerLogStore
	termMgr             *TerminalSessionManager
	skillStore          *SkillStore
	whisper             *WhisperManager
	tgRegistered        sync.Map // set of bot tokens that have had commands registered
	pendingLocalCalls   sync.Map // callId -> chan localCallResult
}

// WorkerStatusStore tracks runtime health of background workers.
type WorkerStatusStore struct {
	mu      sync.RWMutex
	workers map[string]*WorkerStat
}

// WorkerStat holds the latest status of a single background worker.
type WorkerStat struct {
	Name        string    `json:"name"`
	LastTick    time.Time `json:"lastTick"`
	LastError   string    `json:"lastError,omitempty"`
	LastSuccess time.Time `json:"lastSuccess"`
	Ticks       int64     `json:"ticks"`
}

func (ws *WorkerStatusStore) tick(name string, err error) {
	ws.mu.Lock()
	defer ws.mu.Unlock()
	if ws.workers == nil {
		ws.workers = make(map[string]*WorkerStat)
	}
	stat, ok := ws.workers[name]
	if !ok {
		stat = &WorkerStat{Name: name}
		ws.workers[name] = stat
	}
	stat.LastTick = time.Now().UTC()
	stat.Ticks++
	if err != nil {
		stat.LastError = err.Error()
	} else {
		stat.LastError = ""
		stat.LastSuccess = stat.LastTick
	}
}

func (ws *WorkerStatusStore) all() []WorkerStat {
	ws.mu.RLock()
	defer ws.mu.RUnlock()
	out := make([]WorkerStat, 0, len(ws.workers))
	for _, s := range ws.workers {
		out = append(out, *s)
	}
	return out
}

// WorkerLogEntry is a single log line from a background worker.
type WorkerLogEntry struct {
	TS   time.Time `json:"ts"`
	Line string    `json:"line"`
}

// WorkerLogStore holds a fixed-size ring buffer of log lines per worker.
type WorkerLogStore struct {
	mu      sync.RWMutex
	buffers map[string][]WorkerLogEntry
}

const workerLogCap = 200

// Append adds a log line for the named worker.
func (wl *WorkerLogStore) Append(worker, line string) {
	wl.mu.Lock()
	defer wl.mu.Unlock()
	if wl.buffers == nil {
		wl.buffers = make(map[string][]WorkerLogEntry)
	}
	buf := wl.buffers[worker]
	buf = append(buf, WorkerLogEntry{TS: time.Now().UTC(), Line: line})
	if len(buf) > workerLogCap {
		buf = buf[len(buf)-workerLogCap:]
	}
	wl.buffers[worker] = buf
}

// Get returns a copy of log entries for the named worker.
func (wl *WorkerLogStore) Get(worker string) []WorkerLogEntry {
	wl.mu.RLock()
	defer wl.mu.RUnlock()
	src := wl.buffers[worker]
	if len(src) == 0 {
		return nil
	}
	out := make([]WorkerLogEntry, len(src))
	copy(out, src)
	return out
}

type Options struct {
	AdminUsername       string
	AdminPasswordBcrypt string
	ServerShellTimeout  time.Duration
	StateDir            string
	WhisperModel        string
	WhisperEndpoint     string
}

func NewServer(env string, db *pgxpool.Pool, authMgr *auth.Manager, cfgStore *configstore.Store, opts Options) *Server {
	if opts.ServerShellTimeout <= 0 {
		opts.ServerShellTimeout = 300 * time.Second
	}

	s := &Server{
		env:     env,
		db:      db,
		authMgr: authMgr,
		mux:     http.NewServeMux(),
		orchestrator: orchestrator.NewService([]orchestrator.Provider{
			orchestrator.StubProvider{ProviderName: "primary"},
			orchestrator.StubProvider{ProviderName: "fallback"},
		}, orchestrator.ToolLoopGuard{}),
		realtimeHub: realtime.NewHub(),
		backoffPolicy: scheduler.BackoffPolicy{
			BaseDelay: 2 * time.Second,
			MaxDelay:  2 * time.Minute,
		},
		configStore:         cfgStore,
		adminUsername:       strings.ToLower(strings.TrimSpace(opts.AdminUsername)),
		adminPasswordBcrypt: strings.TrimSpace(opts.AdminPasswordBcrypt),
		serverShellTimeout:  opts.ServerShellTimeout,
		termMgr:             NewTerminalSessionManager(),
		skillStore:          NewSkillStore(opts.StateDir),
		whisper:             NewWhisperManager(opts.WhisperEndpoint, opts.WhisperModel),
	}

	s.routes()
	return s
}

func (s *Server) Handler() http.Handler {
	return s.mux
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /healthz", s.handleHealth)

	// Swagger UI
	s.mux.HandleFunc("GET /docs/", httpSwagger.Handler(
		httpSwagger.URL("/docs/doc.json"),
	))
	// Alias for openapi.json
	s.mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/docs/doc.json", http.StatusMovedPermanently)
	})

	s.registerAuthRoutes()
	s.registerSessionRoutes()
	s.registerConversationRoutes()
	s.registerTaskRoutes()
	s.registerMemoryRoutes()
	s.registerSettingsRoutes()
	s.registerOrchestratorRoutes()
	s.registerConfigRoutes()
	s.registerProviderRoutes()
	s.registerToolRoutes()
	s.registerSkillRoutes()
	s.registerMCPRoutes()
	s.registerDAVRoutes()
	s.registerTelegramRoutes()

	// File-based skills (SKILL.md on disk)
	s.registerSkillFileRoutes()
	s.mux.Handle("POST /v1/server/command", s.requireAccessToken(http.HandlerFunc(s.handleRunServerCommand)))
	s.mux.Handle("GET /v1/terminal/ws", s.requireAccessToken(http.HandlerFunc(s.handleTerminalWS)))

	s.mux.Handle("GET /v1/heartbeat", s.requireAccessToken(http.HandlerFunc(s.handleGetHeartbeatConfig)))
	s.mux.Handle("PUT /v1/heartbeat", s.requireAccessToken(http.HandlerFunc(s.handlePutHeartbeatConfig)))
	s.mux.Handle("POST /v1/heartbeat/trigger", s.requireAccessToken(http.HandlerFunc(s.handleTriggerHeartbeat)))
	s.mux.Handle("GET /v1/heartbeat/events", s.requireAccessToken(http.HandlerFunc(s.handleListHeartbeatEvents)))

	s.mux.Handle("GET /v1/devices/registrations", s.requireAccessToken(http.HandlerFunc(s.handleListDeviceRegistrations)))
	s.mux.Handle("POST /v1/devices/{id}/register", s.requireAccessToken(http.HandlerFunc(s.handleRegisterDevice)))
	s.mux.Handle("DELETE /v1/devices/{id}", s.requireAccessToken(http.HandlerFunc(s.handleDeleteDevice)))
	s.mux.Handle("POST /v1/tool-results/{callId}", s.requireAccessToken(http.HandlerFunc(s.handleLocalToolResult)))
	s.mux.Handle("GET /v1/devices/tasks", s.requireAccessToken(http.HandlerFunc(s.handleListDeviceTasks)))
	s.mux.Handle("POST /v1/devices/tasks", s.requireAccessToken(http.HandlerFunc(s.handleCreateDeviceTask)))
	s.mux.Handle("DELETE /v1/devices/tasks/{id}", s.requireAccessToken(http.HandlerFunc(s.handleDeleteDeviceTask)))
	s.mux.Handle("POST /v1/devices/tasks/{id}/complete", s.requireAccessToken(http.HandlerFunc(s.handleCompleteDeviceTask)))

	s.mux.Handle("GET /v1/email/inboxes", s.requireAccessToken(http.HandlerFunc(s.handleListEmailInboxes)))
	s.mux.Handle("POST /v1/email/inboxes", s.requireAccessToken(http.HandlerFunc(s.handleCreateEmailInbox)))
	s.mux.Handle("PATCH /v1/email/inboxes/{id}", s.requireAccessToken(http.HandlerFunc(s.handleUpdateEmailInbox)))
	s.mux.Handle("DELETE /v1/email/inboxes/{id}", s.requireAccessToken(http.HandlerFunc(s.handleDeleteEmailInbox)))
	s.mux.Handle("GET /v1/email/poll-events", s.requireAccessToken(http.HandlerFunc(s.handleListEmailPollEvents)))
	s.mux.Handle("POST /v1/email/poll", s.requireAccessToken(http.HandlerFunc(s.handleTriggerEmailPoll)))
	s.mux.Handle("POST /v1/email/test", s.requireAccessToken(http.HandlerFunc(s.handleTestEmailConnection)))
	s.mux.Handle("POST /v1/email/autoconfig", s.requireAccessToken(http.HandlerFunc(s.handleEmailAutoconfig)))
	s.mux.Handle("POST /v1/ssh/test", s.requireAccessToken(http.HandlerFunc(s.handleTestSSHConnection)))

	s.mux.Handle("GET /v1/status/workers", s.requireAccessToken(http.HandlerFunc(s.handleWorkerStatus)))
	s.mux.Handle("GET /v1/workers/logs", s.requireAccessToken(http.HandlerFunc(s.handleWorkerLogs)))
	s.registerVoiceRoutes()

	s.mux.HandleFunc("GET /v1/feature-split", s.handleFeatureSplit)
}

func (s *Server) registerAuthRoutes() {
	s.mux.HandleFunc("POST /v1/auth/login", s.handleLogin)
	s.mux.HandleFunc("POST /v1/auth/refresh", s.handleRefresh)
	s.mux.Handle("POST /v1/auth/logout", s.requireAccessToken(http.HandlerFunc(s.handleLogout)))
	s.mux.Handle("POST /v1/auth/device-tokens", s.requireAccessToken(http.HandlerFunc(s.handleCreateDeviceTokens)))
}

func (s *Server) registerSessionRoutes() {
	s.mux.Handle("GET /v1/sessions", s.requireAccessToken(http.HandlerFunc(s.handleListSessions)))
	s.mux.Handle("DELETE /v1/sessions/{id}", s.requireAccessToken(http.HandlerFunc(s.handleDeleteSession)))
	s.mux.Handle("GET /v1/devices", s.requireAccessToken(http.HandlerFunc(s.handleListDevices)))
}

func (s *Server) registerConversationRoutes() {
	s.mux.Handle("GET /v1/conversations", s.requireAccessToken(http.HandlerFunc(s.handleListConversations)))
	s.mux.Handle("POST /v1/conversations", s.requireAccessToken(http.HandlerFunc(s.handleCreateConversation)))
	s.mux.Handle("GET /v1/conversations/{id}", s.requireAccessToken(http.HandlerFunc(s.handleGetConversation)))
	s.mux.Handle("PATCH /v1/conversations/{id}", s.requireAccessToken(http.HandlerFunc(s.handleUpdateConversation)))
	s.mux.Handle("DELETE /v1/conversations/{id}", s.requireAccessToken(http.HandlerFunc(s.handleDeleteConversation)))
	s.mux.Handle("GET /v1/conversations/{id}/messages", s.requireAccessToken(http.HandlerFunc(s.handleListMessages)))
	s.mux.Handle("POST /v1/conversations/{id}/messages", s.requireAccessToken(http.HandlerFunc(s.handleCreateMessage)))
	s.mux.Handle("GET /v1/conversations/{id}/tool-calls", s.requireAccessToken(http.HandlerFunc(s.handleListToolCalls)))
	s.mux.Handle("POST /v1/conversations/{id}/messages/{msgId}/regenerate", s.requireAccessToken(http.HandlerFunc(s.handleRegenerateMessage)))
}

func (s *Server) registerTaskRoutes() {
	s.mux.Handle("GET /v1/tasks", s.requireAccessToken(http.HandlerFunc(s.handleListTasks)))
	s.mux.Handle("POST /v1/tasks", s.requireAccessToken(http.HandlerFunc(s.handleCreateTask)))
	s.mux.Handle("GET /v1/tasks/{id}", s.requireAccessToken(http.HandlerFunc(s.handleGetTask)))
	s.mux.Handle("PATCH /v1/tasks/{id}", s.requireAccessToken(http.HandlerFunc(s.handleUpdateTask)))
	s.mux.Handle("DELETE /v1/tasks/{id}", s.requireAccessToken(http.HandlerFunc(s.handleDeleteTask)))
	s.mux.Handle("GET /v1/schedules", s.requireAccessToken(http.HandlerFunc(s.handleListTasks)))
	s.mux.Handle("POST /v1/schedules", s.requireAccessToken(http.HandlerFunc(s.handleCreateTask)))
	s.mux.Handle("GET /v1/schedules/{id}", s.requireAccessToken(http.HandlerFunc(s.handleGetTask)))
	s.mux.Handle("PATCH /v1/schedules/{id}", s.requireAccessToken(http.HandlerFunc(s.handleUpdateTask)))
	s.mux.Handle("DELETE /v1/schedules/{id}", s.requireAccessToken(http.HandlerFunc(s.handleDeleteTask)))
}

func (s *Server) registerMemoryRoutes() {
	s.mux.Handle("GET /v1/memory", s.requireAccessToken(http.HandlerFunc(s.handleListMemories)))
	s.mux.Handle("POST /v1/memory", s.requireAccessToken(http.HandlerFunc(s.handleCreateMemory)))
	s.mux.Handle("DELETE /v1/memory/{id}", s.requireAccessToken(http.HandlerFunc(s.handleDeleteMemory)))
}

func (s *Server) registerSettingsRoutes() {
	s.mux.Handle("GET /v1/settings", s.requireAccessToken(http.HandlerFunc(s.handleGetSettings)))
	s.mux.Handle("PUT /v1/settings", s.requireAccessToken(http.HandlerFunc(s.handlePutSettings)))
}

func (s *Server) registerOrchestratorRoutes() {
	s.mux.Handle("POST /v1/orchestrator/complete", s.requireAccessToken(http.HandlerFunc(s.handleOrchestratorComplete)))
	s.mux.Handle("POST /v1/orchestrator/stream", s.requireAccessToken(http.HandlerFunc(s.handleOrchestratorStream)))
	s.mux.Handle("GET /v1/realtime/last", s.requireAccessToken(http.HandlerFunc(s.handleRealtimeLastEvent)))
}

func (s *Server) registerConfigRoutes() {
	s.mux.Handle("GET /v1/config", s.requireAccessToken(http.HandlerFunc(s.handleGetUserConfig)))
	s.mux.Handle("PUT /v1/config", s.requireAccessToken(http.HandlerFunc(s.handlePutUserConfig)))
}

func (s *Server) registerProviderRoutes() {
	s.mux.Handle("GET /v1/providers/status", s.requireAccessToken(http.HandlerFunc(s.handleProvidersStatus)))
	s.mux.Handle("POST /v1/providers/test", s.requireAccessToken(http.HandlerFunc(s.handleTestProvider)))
	s.mux.Handle("POST /v1/providers/models", s.requireAccessToken(http.HandlerFunc(s.handleProbeProviderModels)))
}

func (s *Server) registerToolRoutes() {
	s.mux.Handle("GET /v1/tools", s.requireAccessToken(http.HandlerFunc(s.handleGetToolsConfig)))
	s.mux.Handle("PUT /v1/tools", s.requireAccessToken(http.HandlerFunc(s.handlePutToolsConfig)))
}

func (s *Server) registerSkillRoutes() {
	s.mux.Handle("GET /v1/skills", s.requireAccessToken(http.HandlerFunc(s.handleGetSkillsConfig)))
	s.mux.Handle("PUT /v1/skills", s.requireAccessToken(http.HandlerFunc(s.handlePutSkillsConfig)))
}

func (s *Server) registerMCPRoutes() {
	s.mux.Handle("POST /v1/mcp/test", s.requireAccessToken(http.HandlerFunc(s.handleTestMCPServer)))
}

func (s *Server) registerDAVRoutes() {
	s.mux.Handle("POST /v1/dav/test", s.requireAccessToken(http.HandlerFunc(s.handleTestDAVConnection)))
}

func (s *Server) registerTelegramRoutes() {
	s.mux.Handle("POST /v1/telegram/test", s.requireAccessToken(http.HandlerFunc(s.handleTestTelegramBot)))
}

func (s *Server) registerSkillFileRoutes() {
	s.mux.Handle("GET /v1/skill-files", s.requireAccessToken(http.HandlerFunc(s.handleListSkillFiles)))
	s.mux.Handle("POST /v1/skill-files", s.requireAccessToken(http.HandlerFunc(s.handleCreateSkillFile)))
	s.mux.Handle("POST /v1/skill-files/install", s.requireAccessToken(http.HandlerFunc(s.handleInstallSkills)))
	s.mux.Handle("GET /v1/skill-files/{slug}", s.requireAccessToken(http.HandlerFunc(s.handleGetSkillFile)))
	s.mux.Handle("PUT /v1/skill-files/{slug}", s.requireAccessToken(http.HandlerFunc(s.handleUpdateSkillFile)))
	s.mux.Handle("DELETE /v1/skill-files/{slug}", s.requireAccessToken(http.HandlerFunc(s.handleDeleteSkillFile)))
}

func (s *Server) registerVoiceRoutes() {
	s.mux.Handle("GET /v1/voice/status", s.requireAccessToken(http.HandlerFunc(s.handleVoiceStatus)))
	s.mux.Handle("POST /v1/voice/transcribe", s.requireAccessToken(http.HandlerFunc(s.handleVoiceTranscribe)))
}
