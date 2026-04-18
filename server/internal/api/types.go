package api

type ErrorResponse struct {
	Error string `json:"error"`
}

type HealthResponse struct {
	Status string `json:"status"`
	Name   string `json:"name"`
	Env    string `json:"env"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Device   string `json:"device,omitempty"`
}

type RefreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

type AuthResponse struct {
	User   UserDTO `json:"user"`
	Tokens any     `json:"tokens"`
}

type UserDTO struct {
	ID       string `json:"id"`
	Username string `json:"username"`
}

type SessionDTO struct {
	ID          string `json:"id"`
	DeviceLabel string `json:"deviceLabel"`
	CreatedAt   string `json:"createdAt"`
	LastSeenAt  string `json:"lastSeenAt"`
}

type ConversationDTO struct {
	ID             string `json:"id"`
	Title          string `json:"title"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
	IsAutomatic    bool   `json:"isAutomatic,omitempty"`
	AutomationKind string `json:"automationKind,omitempty"`
}

type CreateConversationRequest struct {
	Title string `json:"title"`
}

type UpdateConversationRequest struct {
	Title string `json:"title"`
}

type MessageDTO struct {
	ID             string `json:"id"`
	ConversationID string `json:"conversationId"`
	Role           string `json:"role"`
	Content        string `json:"content"`
	CreatedAt      string `json:"createdAt"`
}

type CreateMessageRequest struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ToolCallRecord struct {
	ID         string         `json:"id"`
	ToolName   string         `json:"toolName"`
	Arguments  map[string]any `json:"arguments"`
	Output     *string        `json:"output"`
	Error      *string        `json:"error,omitempty"`
	DurationMS *int64         `json:"durationMs,omitempty"`
	CreatedAt  string         `json:"createdAt"`
}

type TaskDTO struct {
	ID                  string  `json:"id"`
	Description         string  `json:"description"`
	Prompt              string  `json:"prompt"`
	ExecuteAt           string  `json:"executeAt"`
	CronExpression      *string `json:"cronExpression,omitempty"`
	Status              string  `json:"status"`
	LastResult          *string `json:"lastResult,omitempty"`
	ConsecutiveFailures int     `json:"consecutiveFailures"`
	CreatedAt           string  `json:"createdAt"`
	UpdatedAt           string  `json:"updatedAt"`
}

type CreateTaskRequest struct {
	Description    string  `json:"description"`
	Prompt         string  `json:"prompt"`
	ExecuteAt      string  `json:"executeAt"`
	CronExpression *string `json:"cronExpression,omitempty"`
}

type UpdateTaskRequest struct {
	Description    *string `json:"description,omitempty"`
	Prompt         *string `json:"prompt,omitempty"`
	ExecuteAt      *string `json:"executeAt,omitempty"`
	CronExpression *string `json:"cronExpression,omitempty"`
	Status         *string `json:"status,omitempty"`
}

type MemoryDTO struct {
	ID         string `json:"id"`
	Category   string `json:"category"`
	Content    string `json:"content"`
	Confidence int    `json:"confidence"`
	CreatedAt  string `json:"createdAt"`
	UpdatedAt  string `json:"updatedAt"`
}

type CreateMemoryRequest struct {
	Category   string `json:"category"`
	Content    string `json:"content"`
	Confidence int    `json:"confidence,omitempty"`
}

type UserSettingsDTO struct {
	UserID    string         `json:"userId"`
	Settings  map[string]any `json:"settings"`
	UpdatedAt string         `json:"updatedAt"`
}

type UpdateSettingsRequest struct {
	Settings map[string]any `json:"settings"`
}

type CompleteRequest struct {
	ConversationID string   `json:"conversationId"`
	Message        string   `json:"message"`
	ProviderOrder  []string `json:"providerOrder,omitempty"`
	MaxRetries     int      `json:"maxRetries,omitempty"`
}

type CompleteResponse struct {
	Provider string                  `json:"provider"`
	Output   string                  `json:"output"`
	Attempts int                     `json:"attempts"`
	Trace    CompletionTraceResponse `json:"trace"`
}

type CompletionTraceResponse struct {
	ProviderAttempts []ProviderAttemptDTO `json:"providerAttempts"`
	ToolCalls        []ToolCallDTO        `json:"toolCalls"`
	RuntimeActions   []RuntimeActionDTO   `json:"runtimeActions"`
}

type ProviderAttemptDTO struct {
	Provider string `json:"provider"`
	Attempt  int    `json:"attempt"`
	Success  bool   `json:"success"`
	Error    string `json:"error,omitempty"`
}

type ToolCallDTO struct {
	Name      string         `json:"name"`
	Arguments map[string]any `json:"arguments,omitempty"`
	Status    string         `json:"status"`
	Output    string         `json:"output,omitempty"`
}

type RuntimeActionDTO struct {
	Kind      string `json:"kind"`
	Command   string `json:"command,omitempty"`
	Status    string `json:"status"`
	Output    string `json:"output,omitempty"`
	StartedAt string `json:"startedAt,omitempty"`
}

type RunServerCommandRequest struct {
	Command        string `json:"command"`
	TimeoutSeconds int    `json:"timeoutSeconds,omitempty"`
}

type RunServerCommandResponse struct {
	Shell      string `json:"shell"`
	Command    string `json:"command"`
	ExitCode   int    `json:"exitCode"`
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	DurationMS int64  `json:"durationMs"`
	TimedOut   bool   `json:"timedOut"`
	StartedAt  string `json:"startedAt"`
	FinishedAt string `json:"finishedAt"`
}

type HeartbeatConfigDTO struct {
	UserID          string `json:"userId"`
	Enabled         bool   `json:"enabled"`
	IntervalSeconds int    `json:"intervalSeconds"`
	NextRunAt       string `json:"nextRunAt,omitempty"`
	UpdatedAt       string `json:"updatedAt"`
}

type UpdateHeartbeatConfigRequest struct {
	Enabled         *bool `json:"enabled,omitempty"`
	IntervalSeconds *int  `json:"intervalSeconds,omitempty"`
}

type HeartbeatEventDTO struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Message   string `json:"message,omitempty"`
	CreatedAt string `json:"createdAt"`
}

type TriggerHeartbeatRequest struct {
	Message string `json:"message,omitempty"`
}

type EmailInboxDTO struct {
	ID                  string `json:"id"`
	Address             string `json:"address"`
	ImapHost            string `json:"imapHost"`
	ImapPort            int    `json:"imapPort"`
	ImapUsername        string `json:"imapUsername"`
	UseTLS              bool   `json:"useTls"`
	Active              bool   `json:"active"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds"`
	LastPolledAt        string `json:"lastPolledAt,omitempty"`
	UpdatedAt           string `json:"updatedAt"`
	CreatedAt           string `json:"createdAt"`
}

type CreateEmailInboxRequest struct {
	Address             string `json:"address"`
	ImapHost            string `json:"imapHost"`
	ImapPort            int    `json:"imapPort,omitempty"`
	ImapUsername        string `json:"imapUsername,omitempty"`
	ImapPassword        string `json:"imapPassword,omitempty"`
	UseTLS              *bool  `json:"useTls,omitempty"`
	PollIntervalSeconds int    `json:"pollIntervalSeconds,omitempty"`
}

type UpdateEmailInboxRequest struct {
	ImapHost            *string `json:"imapHost,omitempty"`
	ImapPort            *int    `json:"imapPort,omitempty"`
	ImapUsername        *string `json:"imapUsername,omitempty"`
	ImapPassword        *string `json:"imapPassword,omitempty"`
	UseTLS              *bool   `json:"useTls,omitempty"`
	Active              *bool   `json:"active,omitempty"`
	PollIntervalSeconds *int    `json:"pollIntervalSeconds,omitempty"`
}

type EmailPollEventDTO struct {
	ID        string `json:"id"`
	InboxID   string `json:"inboxId"`
	Status    string `json:"status"`
	Detail    string `json:"detail,omitempty"`
	CreatedAt string `json:"createdAt"`
}

type TriggerEmailPollRequest struct {
	InboxID string `json:"inboxId"`
	Detail  string `json:"detail,omitempty"`
}

type TestEmailConnectionRequest struct {
	ImapHost string `json:"imapHost"`
	ImapPort int    `json:"imapPort,omitempty"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	UseTLS   bool   `json:"useTls"`
}

type MCPToolSummary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	InputSchema map[string]any `json:"inputSchema,omitempty"`
}

type MCPServerTestRequest struct {
	Name    string            `json:"name"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
}

type MCPServerTestResult struct {
	OK        bool             `json:"ok"`
	LatencyMs int64            `json:"latencyMs"`
	Error     string           `json:"error,omitempty"`
	Tools     []MCPToolSummary `json:"tools,omitempty"`
}
