// lib/api-types.ts — TypeScript interfaces for the openCrow API.

export interface HealthResponse {
  status: string;
  name: string;
  environment: string;
}

export interface AuthResponse {
  user: { id: string; username: string };
  tokens: {
    accessToken: string;
    refreshToken: string;
  };
}

export interface ConversationDTO {
  id: string;
  title: string;
  createdAt: string;
  updatedAt: string;
  isAutomatic?: boolean;
  automationKind?: string;
  channel?: string;
}

export interface MessageDTO {
  id: string;
  conversationId: string;
  role: string;
  content: string;
  attachments?: MessageAttachmentDTO[];
  createdAt: string;
}

export interface MessageAttachmentDTO {
  id: string;
  fileName: string;
  mimeType: string;
  sizeBytes: number;
  dataUrl: string;
  createdAt: string;
}

export interface CreateMessageAttachmentRequest {
  fileName: string;
  mimeType: string;
  sizeBytes?: number;
  dataUrl: string;
}

export interface ToolCallRecord {
  id: string;
  toolName: string;
  kind?: "TOOL" | "MCP" | "DEVICE";
  arguments: Record<string, unknown>;
  output?: string;
  error?: string;
  durationMs?: number;
  createdAt: string;
}

export interface CompletionTraceResponse {
  providerAttempts?: Array<{
    provider: string;
    attempt: number;
    success: boolean;
    error?: string;
  }>;
  toolCalls?: Array<{
    name: string;
    arguments?: Record<string, unknown>;
    status: string;
    output?: string;
  }>;
  runtimeActions?: Array<{
    kind: string;
    command?: string;
    status: string;
    output?: string;
    startedAt?: string;
  }>;
}

export interface CompleteResponse {
  provider: string;
  output: string;
  attempts: number;
  trace?: CompletionTraceResponse;
}

export interface RunCommandResponse {
  exitCode: number;
  stdout: string;
  stderr: string;
  durationMs: number;
  shell: string;
  timedOut: boolean;
}

export interface WorkerStat {
  name: string;
  lastTick: string; // ISO timestamp or empty
  lastError: string;
  lastSuccess: string; // ISO timestamp or empty
  ticks: number;
}

export interface WorkerLogEntry {
  ts: string; // ISO timestamp
  line: string;
}

export interface TokenUsage {
  promptTokens: number;
  completionTokens: number;
  totalTokens: number;
}

export interface TelegramBotConfig {
  id?: string;
  label: string;
  botToken: string;
  allowedChatIds: string[];
  notificationChatId: string;
  enabled: boolean;
  pollIntervalSeconds: number;
  lastUpdateId?: number;
}

export interface DeviceCapability {
  name: string;
  description?: string;
}

export interface DeviceRegistration {
  deviceId: string;
  capabilities: DeviceCapability[];
  lastSeenAt: string;
  pushEndpoint?: string;
}

export interface DeviceSession {
  id: string;
  deviceLabel: string;
  createdAt: string;
  lastSeenAt: string;
}

export interface CompanionAppConfig {
  id?: string;
  name: string;
  label?: string;
  enabled: boolean;
  pushEndpoint?: string;
}

export interface EmailAccountConfig {
  id?: string;
  label: string;
  address: string;
  imapHost: string;
  imapPort: number;
  imapUsername: string;
  imapPassword: string;
  smtpHost: string;
  smtpPort: number;
  tls: boolean;
  enabled: boolean;
  pollIntervalSeconds?: number;
}

export interface ToolParameter {
  name: string;
  type: string;
  description: string;
  required: boolean;
}

export interface ToolDefinition {
  id?: string;
  name: string;
  description: string;
  source: string;
  parameters: ToolParameter[];
}

export interface GolangToolEntry {
  id?: string;
  name: string;
  description: string;
  sourceCode: string;
  enabled: boolean;
}

export interface ProviderConfig {
  id?: string;
  kind: string;
  name: string;
  baseUrl: string;
  apiKeyRef: string;
  model: string;
  enabled: boolean;
  priority?: number; // Lower = higher priority (0 = first)
}

export interface SkillEntry {
  id?: string;
  name: string;
  description: string;
  content: string;
  enabled: boolean;
}

export interface SkillFile {
  slug: string;
  name: string;
  description: string;
  content?: string;
  path?: string;
}

export interface InstallSkillsResult {
  installed: string[];
  errors?: string[];
  count: number;
}

export interface MemoryEntity {
  id: string;
  user_id: string;
  type: string;
  name: string;
  summary?: string;
  created_at: string;
  updated_at: string;
}

export interface MemoryRelation {
  id: string;
  user_id: string;
  from_entity_id: string;
  from_entity_name?: string;
  to_entity_id: string;
  to_entity_name?: string;
  relation: string;
  confidence: number;
  reinforcement_count: number;
  source_conversation_id?: string;
  created_at: string;
  updated_at: string;
}

export interface MemoryGraph {
  entities: MemoryEntity[];
  relations: MemoryRelation[];
}

export interface TaskDTO {
  id: string;
  description: string;
  prompt: string;
  executeAt: string;
  cronExpression?: string | null;
  status: string;
  lastResult?: string | null;
  consecutiveFailures: number;
  createdAt: string;
  updatedAt: string;
}

export interface DeviceTaskDTO {
  id: string;
  targetDevice: string;
  instruction: string;
  status: string;
  resultOutput?: string | null;
  createdAt: string;
  updatedAt: string;
  expiresAt?: string | null;
}

export interface CreateDeviceTaskRequest {
  targetDevice: string;
  instruction: string;
}

export interface CompleteDeviceTaskRequest {
  resultOutput: string;
}

export interface ScheduleEntry {
  id?: string;
  description: string;
  status: string;
  executeAt: string;
  cronExpression: string;
  prompt: string;
}

export interface HeartbeatConfig {
  enabled: boolean;
  intervalSeconds: number;
  model: string;
  activeHoursStart: string;
  activeHoursEnd: string;
  timezone: string;
}

export interface VoiceConfig {
  defaultVoice: string;
  languageVoices: Record<string, string>;
}

export interface MCPServerConfig {
  id?: string;
  name: string;
  url: string;
  headers?: Record<string, string>;
  enabled: boolean;
}

export interface SSHServerConfig {
  id?: string;
  name: string;
  host: string;
  port?: number;
  username: string;
  authMode: "key" | "password";
  sshKey?: string;
  password?: string;
  passphrase?: string;
  enabled: boolean;
}

export interface DAVConfig {
  id?: string;
  name: string;
  url: string;
  username: string;
  password: string;
  enabled: boolean;
  webdavEnabled: boolean;
  caldavEnabled: boolean;
  carddavEnabled: boolean;
  pollIntervalSeconds: number;
}

export interface DAVCollection {
  path: string;
  displayName?: string;
  description?: string;
  ctag?: string;
}

export interface DAVFileListItem {
  path: string;
  displayName?: string;
  contentType?: string;
  isCollection: boolean;
  size?: number;
  updatedAt?: string;
}

export interface DAVDiscoveryInfo {
  enabled: boolean;
  homeSet?: string;
  collections?: DAVCollection[];
  entries?: DAVFileListItem[];
  error?: string;
}

export interface DAVTestResult {
  ok: boolean;
  latencyMs: number;
  error?: string;
  principal?: string;
  webdav?: DAVDiscoveryInfo;
  caldav?: DAVDiscoveryInfo;
  carddav?: DAVDiscoveryInfo;
  capabilities?: string[];
}

export interface HeartbeatEventDTO {
  id: string;
  status: string;
  message?: string;
  createdAt: string;
}

export interface RealtimeEvent {
  userId?: string;
  type: string;
  payload?: Record<string, unknown>;
}

export interface MCPToolSummary {
  name: string;
  description?: string;
}

export interface MCPServerTestResult {
  ok: boolean;
  latencyMs: number;
  error?: string;
  tools?: MCPToolSummary[];
}

export interface ProviderModelsProbeResult {
  ok: boolean;
  models?: string[];
  error?: string;
}

export interface UserConfig {
  integrations: {
    emailAccounts: EmailAccountConfig[];
    telegramBots: TelegramBotConfig[];
    sshServers: SSHServerConfig[];
    dav: DAVConfig[];
    companionApps: CompanionAppConfig[];
    defaultNotificationBotId: string;
  };
  tools: {
    definitions: ToolDefinition[];
    golangTools: GolangToolEntry[];
    enabledTools: Record<string, boolean>;
  };
  mcp: { servers: MCPServerConfig[] };
  linuxSandbox: { enabled: boolean };
  llm: { providers: ProviderConfig[] };
  skills: { entries: SkillEntry[] };
  prompts: { systemPrompt: string; heartbeatPrompt: string };
  schedules: { entries: ScheduleEntry[] };
  heartbeat: HeartbeatConfig;
  voice: VoiceConfig;
}

// ─── Internal server shape (for normalization) ───

export interface ServerUserConfig {
  integrations?: {
    emailAccounts?: Array<EmailAccountConfig & { useTls?: boolean }>;
    telegramBots?: TelegramBotConfig[];
    sshServers?: SSHServerConfig[];
    dav?: Partial<DAVConfig> | Array<Partial<DAVConfig>>;
    companionApps?: CompanionAppConfig[];
    defaultNotificationBotId?: string;
  };
  tools?: {
    definitions?: ToolDefinition[];
    golangTools?: GolangToolEntry[];
    enabled?: Record<string, boolean>;
    enabledTools?: Record<string, boolean>;
  };
  linuxSandbox?: { enabled?: boolean };
  mcp?: { servers?: MCPServerConfig[] };
  llm?: { providers?: ProviderConfig[] };
  skills?: { entries?: SkillEntry[] };
  prompts?: { systemPrompt?: string; heartbeatPrompt?: string };
  schedules?: {
    entries?: Array<Omit<ScheduleEntry, "cronExpression"> & { cronExpression?: string | null }>;
  };
  heartbeat?: {
    enabled?: boolean;
    intervalSeconds?: number;
    model?: string;
    activeHours?: { start?: string; end?: string; tz?: string };
    activeHoursStart?: string;
    activeHoursEnd?: string;
    timezone?: string;
  };
  voice?: {
    defaultVoice?: string;
    languageVoices?: Record<string, string>;
  };
}

// ─── Internal response wrappers ───

export interface ConversationsResponse {
  conversations?: ConversationDTO[];
}

export interface MessagesResponse {
  messages?: MessageDTO[];
}
