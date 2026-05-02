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
	Channel        string `json:"channel,omitempty"`
}

type CreateConversationRequest struct {
	Title string `json:"title"`
}

type UpdateConversationRequest struct {
	Title string `json:"title"`
}

type MessageDTO struct {
	ID             string                 `json:"id"`
	ConversationID string                 `json:"conversationId"`
	Role           string                 `json:"role"`
	Content        string                 `json:"content"`
	Attachments    []MessageAttachmentDTO `json:"attachments,omitempty"`
	CreatedAt      string                 `json:"createdAt"`
}

type MessageAttachmentDTO struct {
	ID        string `json:"id"`
	FileName  string `json:"fileName"`
	MimeType  string `json:"mimeType"`
	SizeBytes int64  `json:"sizeBytes"`
	DataURL   string `json:"dataUrl"`
	CreatedAt string `json:"createdAt"`
}

type CreateMessageAttachmentRequest struct {
	ID        string `json:"id,omitempty"`
	FileName  string `json:"fileName"`
	MimeType  string `json:"mimeType"`
	SizeBytes int64  `json:"sizeBytes,omitempty"`
	DataURL   string `json:"dataUrl"`
	CreatedAt string `json:"createdAt,omitempty"`
}

type CreateMessageRequest struct {
	Role        string                           `json:"role"`
	Content     string                           `json:"content"`
	Attachments []CreateMessageAttachmentRequest `json:"attachments,omitempty"`
}

type ToolCallRecord struct {
	ID         string         `json:"id"`
	ToolName   string         `json:"toolName"`
	Kind       string         `json:"kind,omitempty"`
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

type UserSettingsDTO struct {
	UserID    string         `json:"userId"`
	Settings  map[string]any `json:"settings"`
	UpdatedAt string         `json:"updatedAt"`
}

type UpdateSettingsRequest struct {
	Settings map[string]any `json:"settings"`
}

type CompleteRequest struct {
	ConversationID string                           `json:"conversationId"`
	Message        string                           `json:"message"`
	Attachments    []CreateMessageAttachmentRequest `json:"attachments,omitempty"`
	ProviderOrder  []string                         `json:"providerOrder,omitempty"`
	MaxRetries     int                              `json:"maxRetries,omitempty"`
	DeviceID       string                           `json:"deviceId,omitempty"` // requesting device, used to inject local tool specs
}

type CompleteResponse struct {
	Provider string                  `json:"provider"`
	Output   string                  `json:"output"`
	Attempts int                     `json:"attempts"`
	Trace    CompletionTraceResponse `json:"trace"`
	Usage    *TokenUsageDTO          `json:"usage,omitempty"`
}

type TokenUsageDTO struct {
	PromptTokens     int `json:"promptTokens"`
	CompletionTokens int `json:"completionTokens"`
	TotalTokens      int `json:"totalTokens"`
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
	UserID           string `json:"userId"`
	Enabled          bool   `json:"enabled"`
	IntervalSeconds  int    `json:"intervalSeconds"`
	NextRunAt        string `json:"nextRunAt,omitempty"`
	UpdatedAt        string `json:"updatedAt"`
	HeartbeatPrompt  string `json:"heartbeatPrompt,omitempty"`
	ActiveHoursStart string `json:"activeHoursStart,omitempty"`
	ActiveHoursEnd   string `json:"activeHoursEnd,omitempty"`
	Timezone         string `json:"timezone,omitempty"`
}

type UpdateHeartbeatConfigRequest struct {
	Enabled         *bool   `json:"enabled,omitempty"`
	IntervalSeconds *int    `json:"intervalSeconds,omitempty"`
	HeartbeatPrompt *string `json:"heartbeatPrompt,omitempty"`
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
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
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

type DeviceTaskDTO struct {
	ID            string         `json:"id"`
	TargetDevice  string         `json:"targetDevice"`
	Instruction   string         `json:"instruction"`
	ToolName      *string        `json:"toolName,omitempty"`
	ToolArguments map[string]any `json:"toolArguments,omitempty"`
	Status        string         `json:"status"`
	ResultOutput  *string        `json:"resultOutput,omitempty"`
	CreatedAt     string         `json:"createdAt"`
	UpdatedAt     string         `json:"updatedAt"`
	ExpiresAt     *string        `json:"expiresAt,omitempty"`
}

type CreateDeviceTaskRequest struct {
	TargetDevice  string         `json:"targetDevice"`
	Instruction   string         `json:"instruction"`
	ToolName      *string        `json:"toolName,omitempty"`
	ToolArguments map[string]any `json:"toolArguments,omitempty"`
}

type CompleteDeviceTaskRequest struct {
	Success bool   `json:"success"`
	Output  string `json:"output"`
}

type UpdateDeviceTaskRequest struct {
	Instruction   *string        `json:"instruction,omitempty"`
	ToolName      *string        `json:"toolName,omitempty"`
	ToolArguments map[string]any `json:"toolArguments,omitempty"`
	// Reset a failed/completed task back to pending so it runs again
	ResetStatus bool `json:"resetStatus,omitempty"`
}

type DeviceCapability struct {
	Name        string         `json:"name"`
	Description string         `json:"description,omitempty"`
	Parameters  map[string]any `json:"parameters,omitempty"` // JSON Schema object for LLM function-calling
}

// ToolResultRequest is the body for POST /v1/tool-results/{callId}.
type ToolResultRequest struct {
	Output  string `json:"output"`
	IsError bool   `json:"isError,omitempty"`
}

// RecordToolCallRequest is the body for POST /v1/conversations/{id}/tool-calls.
// Used by devices to persist on-device tool executions (e.g. heartbeat tasks) into the conversation.
type RecordToolCallRequest struct {
	Name       string         `json:"name"`
	Arguments  map[string]any `json:"arguments"`
	Output     string         `json:"output"`
	Error      string         `json:"error,omitempty"`
	DurationMS int64          `json:"durationMs,omitempty"`
	Source     string         `json:"source,omitempty"` // "device", "builtin", etc. defaults to "device"
}

type RegisterDeviceRequest struct {
	Capabilities []DeviceCapability `json:"capabilities"`
	PushEndpoint string             `json:"pushEndpoint,omitempty"`
	PushAuth     string             `json:"pushAuth,omitempty"`
}

type DeviceRegistrationDTO struct {
	DeviceID     string             `json:"deviceId"`
	Capabilities []DeviceCapability `json:"capabilities"`
	LastSeenAt   string             `json:"lastSeenAt"`
	PushEndpoint string             `json:"pushEndpoint,omitempty"`
}

// CreateDeviceTokensRequest is the request body for POST /v1/auth/device-tokens.
type CreateDeviceTokensRequest struct {
	DeviceLabel string `json:"deviceLabel"`
}

// TestTelegramBotRequest is the request body for POST /v1/telegram/test.
type TestTelegramBotRequest struct {
	BotToken           string `json:"botToken"`
	NotificationChatID string `json:"notificationChatId,omitempty"`
}

// EmailAutoconfigRequest is the request body for POST /v1/email/autoconfig.
type EmailAutoconfigRequest struct {
	Email string `json:"email"`
}

// EmailAutoconfigResult is the response body for POST /v1/email/autoconfig.
type EmailAutoconfigResult struct {
	ImapHost     string `json:"imapHost,omitempty"`
	ImapPort     int    `json:"imapPort,omitempty"`
	ImapUsername string `json:"imapUsername,omitempty"`
	SmtpHost     string `json:"smtpHost,omitempty"`
	SmtpPort     int    `json:"smtpPort,omitempty"`
	UseTLS       bool   `json:"useTls"`
	Source       string `json:"source,omitempty"`
}
