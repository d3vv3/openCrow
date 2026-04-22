// ─── openCrow API Client ───
// Centralized HTTP client with token management

// Re-export all types so existing `import { ... } from "@/lib/api"` imports keep working.
export type {
  HealthResponse,
  AuthResponse,
  ConversationDTO,
  MessageDTO,
  CreateMessageAttachmentRequest,
  ToolCallRecord,
  CompletionTraceResponse,
  CompleteResponse,
  RunCommandResponse,
  WorkerStat,
  WorkerLogEntry,
  TokenUsage,
  TelegramBotConfig,
  EmailAccountConfig,
  ToolParameter,
  ToolDefinition,
  GolangToolEntry,
  ProviderConfig,
  SkillEntry,
  SkillFile,
  InstallSkillsResult,
  MemoryEntry,
  TaskDTO,
  ScheduleEntry,
  HeartbeatConfig,
  MCPServerConfig,
  SSHServerConfig,
  DAVConfig,
  DAVTestResult,
  HeartbeatEventDTO,
  RealtimeEvent,
  MCPToolSummary,
  MCPServerTestResult,
  ProviderModelsProbeResult,
  UserConfig,
  ServerUserConfig,
  ConversationsResponse,
  MessagesResponse,
  DeviceTaskDTO,
  CreateDeviceTaskRequest,
  CompleteDeviceTaskRequest,
  CompanionAppConfig,
  DeviceCapability,
  DeviceRegistration,
} from "./api-types";

import type {
  HealthResponse,
  AuthResponse,
  ConversationDTO,
  MessageDTO,
  ToolCallRecord,
  CompleteResponse,
  RunCommandResponse,
  WorkerStat,
  WorkerLogEntry,
  TokenUsage,
  SkillFile,
  InstallSkillsResult,
  MemoryEntry,
  TaskDTO,
  HeartbeatConfig,
  HeartbeatEventDTO,
  RealtimeEvent,
  MCPServerTestResult,
  DAVConfig,
  DAVTestResult,
  ProviderModelsProbeResult,
  UserConfig,
  ServerUserConfig,
  ConversationsResponse,
  MessagesResponse,
  DeviceTaskDTO,
  CreateDeviceTaskRequest,
  CompleteDeviceTaskRequest,
  DeviceCapability,
  DeviceRegistration,
  CreateMessageAttachmentRequest,
} from "./api-types";

let _apiBase = "http://localhost:8080";
let _openCrowVersion = "dev";

type AppRuntimeConfig = {
  apiBaseUrl?: string;
  openCrowVersion?: string;
};

export async function initApiBase(): Promise<void> {
  try {
    const res = await fetch("/api/config");
    const data = (await res.json()) as AppRuntimeConfig;
    if (data.apiBaseUrl) _apiBase = data.apiBaseUrl;
    if (data.openCrowVersion) _openCrowVersion = data.openCrowVersion;
  } catch {
    // keep default
  }
}

export function getApiBase(): string {
  return _apiBase;
}

export function getOpenCrowVersion(): string {
  return _openCrowVersion;
}

function getCookie(name: string): string | null {
  if (typeof document === "undefined") return null;
  const match = document.cookie.match(new RegExp(`(^| )${name}=([^;]+)`));
  return match ? decodeURIComponent(match[2]) : null;
}

function setCookie(name: string, value: string, days = 30) {
  const expires = new Date(Date.now() + days * 864e5).toUTCString();
  document.cookie = `${name}=${encodeURIComponent(value)}; expires=${expires}; path=/; SameSite=Lax`;
}

function deleteCookie(name: string) {
  document.cookie = `${name}=; expires=Thu, 01 Jan 1970 00:00:00 GMT; path=/`;
}

export function getAccessToken(): string | null {
  return getCookie("opencrow_access_token");
}

export function getRefreshToken(): string | null {
  return getCookie("opencrow_refresh_token");
}

export function setTokens(access: string, refresh: string) {
  setCookie("opencrow_access_token", access, 1);
  setCookie("opencrow_refresh_token", refresh, 30);
}

export function clearTokens() {
  deleteCookie("opencrow_access_token");
  deleteCookie("opencrow_refresh_token");
}

export function isAuthenticated(): boolean {
  return !!getAccessToken();
}

// Global callback invoked when all auth attempts fail (e.g. expired refresh token).
// Set this from your root component to redirect to the login page.
let _onAuthFailure: (() => void) | null = null;
export function setAuthFailureHandler(fn: () => void) {
  _onAuthFailure = fn;
}
function notifyAuthFailure() {
  clearTokens();
  if (_onAuthFailure) _onAuthFailure();
}

// Singleton refresh promise to prevent concurrent refresh races
let _refreshPromise: Promise<boolean> | null = null;

async function refreshAccessToken(): Promise<boolean> {
  if (_refreshPromise) return _refreshPromise;
  _refreshPromise = _doRefresh().finally(() => {
    _refreshPromise = null;
  });
  return _refreshPromise;
}

async function _doRefresh(): Promise<boolean> {
  const refresh = getRefreshToken();
  if (!refresh) return false;
  try {
    const res = await fetch(`${getApiBase()}/v1/auth/refresh`, {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ refreshToken: refresh }),
    });
    if (!res.ok) return false;
    const data = await res.json();
    const tokenPayload = data?.tokens ?? data;
    if (!tokenPayload?.accessToken || !tokenPayload?.refreshToken) return false;
    setTokens(tokenPayload.accessToken, tokenPayload.refreshToken);
    return true;
  } catch {
    return false;
  }
}

function getClientTimezone(): string {
  if (typeof Intl === "undefined") return "";
  try {
    return Intl.DateTimeFormat().resolvedOptions().timeZone || "";
  } catch {
    return "";
  }
}

export async function api<T = unknown>(path: string, options: RequestInit = {}): Promise<T> {
  // Don't force Content-Type for FormData (browser sets it with boundary automatically)
  const isFormData = options.body instanceof FormData;
  const headers: Record<string, string> = {
    ...(isFormData ? {} : { "Content-Type": "application/json" }),
    ...(options.headers as Record<string, string>),
  };

  const clientTimezone = getClientTimezone();
  if (clientTimezone && !headers["X-Client-Timezone"]) {
    headers["X-Client-Timezone"] = clientTimezone;
  }

  const token = getAccessToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;

  let res = await fetch(`${getApiBase()}${path}`, { ...options, headers });

  // Auto-refresh on 401
  if (res.status === 401 && token) {
    const refreshed = await refreshAccessToken();
    if (refreshed) {
      headers["Authorization"] = `Bearer ${getAccessToken()}`;
      res = await fetch(`${getApiBase()}${path}`, { ...options, headers });
    } else {
      notifyAuthFailure();
      throw new ApiError(401, "Session expired");
    }
  }

  if (!res.ok) {
    if (res.status === 401) notifyAuthFailure();
    const body = await res.text();
    throw new ApiError(res.status, body);
  }

  const text = await res.text();
  return text ? JSON.parse(text) : ({} as T);
}

export class ApiError extends Error {
  constructor(
    public status: number,
    public body: string,
  ) {
    super(`API ${status}: ${body}`);
  }
}

// ─── Config normalization (server ↔ client shape) ───

function normalizeUserConfig(raw: ServerUserConfig): UserConfig {
  const definitions = Array.isArray(raw?.tools?.definitions) ? raw.tools!.definitions : [];
  const enabledRaw = raw?.tools?.enabledTools ?? raw?.tools?.enabled ?? {};
  const enabledTools: Record<string, boolean> = {};

  for (const tool of definitions) {
    const name = tool?.name?.trim();
    if (!name) continue;
    const byName = enabledRaw[name];
    const byID = tool.id ? enabledRaw[tool.id] : undefined;
    enabledTools[name] =
      typeof byName === "boolean" ? byName : typeof byID === "boolean" ? byID : true;
  }

  const heartbeat = raw?.heartbeat ?? {};

  const rawDAV = raw?.integrations?.dav;
  const davList = Array.isArray(rawDAV)
    ? rawDAV
    : rawDAV && typeof rawDAV === "object"
      ? [rawDAV]
      : [];

  return {
    integrations: {
      emailAccounts: (raw?.integrations?.emailAccounts ?? []).map((acct) => ({
        ...acct,
        tls: acct.tls ?? acct.useTls ?? true,
      })),
      telegramBots: Array.isArray(raw?.integrations?.telegramBots)
        ? raw.integrations!.telegramBots
        : [],
      sshServers: Array.isArray(raw?.integrations?.sshServers) ? raw.integrations!.sshServers : [],
      dav: davList.map((dav, index) => ({
        id: dav.id,
        name: dav.name?.trim() || `DAV ${index + 1}`,
        url: dav.url ?? "",
        username: dav.username ?? "",
        password: dav.password ?? "",
        enabled: dav.enabled ?? false,
        webdavEnabled: dav.webdavEnabled ?? true,
        caldavEnabled: dav.caldavEnabled ?? true,
        carddavEnabled: dav.carddavEnabled ?? true,
        pollIntervalSeconds: dav.pollIntervalSeconds ?? 900,
      })),
      companionApps: Array.isArray(raw?.integrations?.companionApps)
        ? raw.integrations!.companionApps
        : [],
      defaultNotificationBotId: raw?.integrations?.defaultNotificationBotId ?? "",
    },
    tools: {
      definitions: definitions.map((tool) => ({
        ...tool,
        parameters: Array.isArray(tool.parameters) ? tool.parameters : [],
      })),
      golangTools: Array.isArray(raw?.tools?.golangTools) ? raw.tools!.golangTools : [],
      enabledTools,
    },
    linuxSandbox: {
      enabled: !!raw?.linuxSandbox?.enabled,
    },
    mcp: {
      servers: Array.isArray(raw?.mcp?.servers) ? raw.mcp!.servers : [],
    },
    llm: {
      providers: Array.isArray(raw?.llm?.providers) ? raw.llm!.providers : [],
    },
    skills: {
      entries: Array.isArray(raw?.skills?.entries) ? raw.skills!.entries : [],
    },
    prompts: {
      systemPrompt: raw?.prompts?.systemPrompt ?? "",
      heartbeatPrompt: raw?.prompts?.heartbeatPrompt ?? "",
    },
    memory: {
      entries: Array.isArray(raw?.memory?.entries) ? raw.memory!.entries : [],
    },
    schedules: {
      entries: (raw?.schedules?.entries ?? []).map((entry) => ({
        ...entry,
        cronExpression: entry?.cronExpression ?? "",
      })),
    },
    heartbeat: {
      enabled: !!heartbeat.enabled,
      intervalSeconds: heartbeat.intervalSeconds ?? 300,
      model: heartbeat.model ?? "",
      activeHoursStart: heartbeat.activeHoursStart ?? heartbeat.activeHours?.start ?? "08:00",
      activeHoursEnd: heartbeat.activeHoursEnd ?? heartbeat.activeHours?.end ?? "22:00",
      timezone: heartbeat.timezone ?? heartbeat.activeHours?.tz ?? "UTC",
    },
  };
}

function toServerUserConfig(config: UserConfig): ServerUserConfig {
  const enabled: Record<string, boolean> = {};
  for (const tool of config.tools.definitions ?? []) {
    const key = tool.id || tool.name;
    if (!key) continue;
    enabled[key] = config.tools.enabledTools?.[tool.name] ?? true;
  }

  return {
    ...config,
    integrations: {
      emailAccounts: (config.integrations.emailAccounts ?? []).map((acct) => ({
        ...acct,
        useTls: acct.tls,
      })),
      telegramBots: config.integrations.telegramBots ?? [],
      sshServers: config.integrations.sshServers ?? [],
      dav: (config.integrations.dav ?? []).map((dav) => ({
        id: dav.id || undefined,
        name: dav.name,
        url: dav.url,
        username: dav.username,
        password: dav.password,
        enabled: dav.enabled,
        webdavEnabled: dav.webdavEnabled,
        caldavEnabled: dav.caldavEnabled,
        carddavEnabled: dav.carddavEnabled,
        pollIntervalSeconds: dav.pollIntervalSeconds,
      })),
      companionApps: config.integrations.companionApps ?? [],
      defaultNotificationBotId: config.integrations.defaultNotificationBotId ?? "",
    },
    tools: {
      definitions: config.tools.definitions,
      golangTools: config.tools.golangTools,
      enabled,
    },
    schedules: {
      entries: (config.schedules.entries ?? []).map((entry) => ({
        ...entry,
        cronExpression: entry.cronExpression?.trim() ? entry.cronExpression : null,
      })),
    },
    heartbeat: {
      enabled: config.heartbeat.enabled,
      intervalSeconds: config.heartbeat.intervalSeconds,
      model: config.heartbeat.model,
      activeHours: {
        start: config.heartbeat.activeHoursStart,
        end: config.heartbeat.activeHoursEnd,
        tz: config.heartbeat.timezone,
      },
    },
  };
}

// ─── API Endpoints ───

export const endpoints = {
  health: () => api<HealthResponse>("/healthz"),
  login: (username: string, password: string, device: string) =>
    api<AuthResponse>("/v1/auth/login", {
      method: "POST",
      body: JSON.stringify({ username, password, device }),
    }),
  logout: () =>
    api<{ loggedOut: boolean }>("/v1/auth/logout", {
      method: "POST",
    }),

  // Config
  getConfig: async () => {
    const data = await api<ServerUserConfig>("/v1/config");
    return normalizeUserConfig(data);
  },
  putConfig: (config: UserConfig) =>
    api("/v1/config", { method: "PUT", body: JSON.stringify(toServerUserConfig(config)) }),

  testProvider: (provider: {
    kind: string;
    name: string;
    baseUrl: string;
    apiKeyRef: string;
    model: string;
  }) =>
    api<{ ok: boolean; latencyMs: number; error?: string; model?: string }>("/v1/providers/test", {
      method: "POST",
      body: JSON.stringify(provider),
    }),
  probeProviderModels: (provider: { kind: string; baseUrl: string; apiKeyRef: string }) =>
    api<ProviderModelsProbeResult>("/v1/providers/models", {
      method: "POST",
      body: JSON.stringify(provider),
    }),
  getProvidersStatus: () =>
    api<{
      providers: Array<{
        name: string;
        kind: string;
        model: string;
        enabled: boolean;
        ok: boolean;
        latencyMs: number;
        error?: string;
      }>;
    }>("/v1/providers/status"),
  getTools: async () => {
    const tools = await api<ServerUserConfig["tools"]>("/v1/tools");
    return normalizeUserConfig({ tools }).tools;
  },
  putTools: (tools: UserConfig["tools"]) =>
    api("/v1/tools", {
      method: "PUT",
      body: JSON.stringify({
        definitions: tools.definitions,
        golangTools: tools.golangTools,
        enabled: Object.fromEntries(
          (tools.definitions ?? [])
            .filter((tool) => !!(tool.id || tool.name))
            .map((tool) => [tool.id || tool.name, tools.enabledTools?.[tool.name] ?? true]),
        ),
      }),
    }),
  getSkills: () => api<UserConfig["skills"]>("/v1/skills"),
  putSkills: (skills: UserConfig["skills"]) =>
    api("/v1/skills", { method: "PUT", body: JSON.stringify(skills) }),
  testMCPServer: (server: { name?: string; url: string; headers?: Record<string, string> }) =>
    api<MCPServerTestResult>("/v1/mcp/test", {
      method: "POST",
      body: JSON.stringify(server),
    }),

  testTelegramBot: (params: { botToken: string; notificationChatId?: string }) =>
    api<{ ok: boolean; latencyMs?: number; error?: string; detail?: string }>("/v1/telegram/test", {
      method: "POST",
      body: JSON.stringify(params),
    }),

  // File-based skills
  listSkillFiles: () => api<SkillFile[]>("/v1/skill-files"),
  getSkillFile: (slug: string) => api<SkillFile>(`/v1/skill-files/${slug}`),
  createSkillFile: (data: { name: string; description?: string; content?: string }) =>
    api<SkillFile>("/v1/skill-files", { method: "POST", body: JSON.stringify(data) }),
  updateSkillFile: (slug: string, content: string) =>
    api<SkillFile>(`/v1/skill-files/${slug}`, { method: "PUT", body: JSON.stringify({ content }) }),
  deleteSkillFile: (slug: string) => api(`/v1/skill-files/${slug}`, { method: "DELETE" }),
  installSkills: (source: string) =>
    api<InstallSkillsResult>("/v1/skill-files/install", {
      method: "POST",
      body: JSON.stringify({ source }),
    }),

  // Conversations
  listConversations: async () => {
    const data = await api<ConversationDTO[] | ConversationsResponse>("/v1/conversations");
    if (Array.isArray(data)) return data;
    return Array.isArray(data?.conversations) ? data.conversations : [];
  },
  createConversation: (title: string) =>
    api<ConversationDTO>("/v1/conversations", {
      method: "POST",
      body: JSON.stringify({ title }),
    }),
  deleteConversation: (id: string) => api(`/v1/conversations/${id}`, { method: "DELETE" }),
  getMessages: async (convId: string) => {
    const data = await api<MessageDTO[] | MessagesResponse>(`/v1/conversations/${convId}/messages`);
    if (Array.isArray(data)) return data;
    return Array.isArray(data?.messages) ? data.messages : [];
  },
  getToolCalls: async (convId: string): Promise<ToolCallRecord[]> => {
    const data = await api<{ toolCalls: ToolCallRecord[] }>(
      `/v1/conversations/${convId}/tool-calls`,
    );
    return data?.toolCalls ?? [];
  },
  createMessage: (
    convId: string,
    role: string,
    content: string,
    attachments?: CreateMessageAttachmentRequest[],
  ) =>
    api<MessageDTO>(`/v1/conversations/${convId}/messages`, {
      method: "POST",
      body: JSON.stringify({ role, content, attachments: attachments ?? [] }),
    }),

  // Orchestrator
  complete: (conversationId: string, message: string, providerOrder?: string[]) =>
    api<CompleteResponse>("/v1/orchestrator/complete", {
      method: "POST",
      body: JSON.stringify({ conversationId, message, providerOrder }),
    }),

  // Streaming completion: calls onToken for each delta, returns full output
  streamComplete: async (
    conversationId: string,
    message: string,
    onToken: (token: string) => void,
    providerOrder?: string[],
    onToolCall?: (name: string, args: string, kind?: "TOOL" | "MCP" | "DEVICE") => void,
    onToolResult?: (name: string, result: string, isError?: boolean) => void,
    onUsage?: (usage: TokenUsage) => void,
  ): Promise<string> => {
    const token = getAccessToken();
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
    };
    const clientTimezone = getClientTimezone();
    if (clientTimezone) headers["X-Client-Timezone"] = clientTimezone;
    if (token) headers["Authorization"] = `Bearer ${token}`;

    const res = await fetch(`${getApiBase()}/v1/orchestrator/stream`, {
      method: "POST",
      headers,
      body: JSON.stringify({ conversationId, message, providerOrder }),
    });

    if (!res.ok || !res.body) {
      const text = await res.text();
      throw new ApiError(res.status, text);
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let fullOutput = "";
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() ?? "";
      let eventType = "";
      for (const line of lines) {
        if (line.startsWith("event: ")) {
          eventType = line.slice(7).trim();
        } else if (line.startsWith("data: ")) {
          const data = JSON.parse(line.slice(6));
          if (eventType === "delta" && data.token) {
            fullOutput += data.token;
            onToken(data.token);
          } else if (eventType === "done") {
            // Prefer accumulated tokens; fall back to server value only if we received nothing
            if (!fullOutput && data.output) fullOutput = data.output;
            if (data.usage && onUsage) {
              onUsage({
                promptTokens: data.usage.promptTokens ?? 0,
                completionTokens: data.usage.completionTokens ?? 0,
                totalTokens: data.usage.totalTokens ?? 0,
              });
            }
          } else if (eventType === "tool_call" && onToolCall) {
            const kind =
              data.kind === "MCP"
                ? "MCP"
                : data.kind === "DEVICE"
                  ? "DEVICE"
                  : data.kind === "TOOL"
                    ? "TOOL"
                    : undefined;
            onToolCall(data.name, data.arguments ?? "{}", kind);
          } else if (eventType === "tool_result" && onToolResult) {
            onToolResult(data.name, data.result ?? "", Boolean(data.error));
          } else if (eventType === "error") {
            throw new ApiError(502, data.error ?? "stream error");
          }
          eventType = "";
        }
      }
    }
    return fullOutput;
  },

  // Heartbeat
  getHeartbeat: () => api<HeartbeatConfig>("/v1/heartbeat"),
  putHeartbeat: (config: HeartbeatConfig) =>
    api("/v1/heartbeat", { method: "PUT", body: JSON.stringify(config) }),
  listHeartbeatEvents: () => api<{ events: HeartbeatEventDTO[] }>("/v1/heartbeat/events"),
  getRealtimeLastEvent: () => api<{ event: RealtimeEvent | null }>("/v1/realtime/last"),

  // Worker status
  getWorkerStatus: () => api<{ workers: WorkerStat[] }>("/v1/status/workers"),

  getWorkerLogs: (worker: string) =>
    api<{ worker: string; entries: WorkerLogEntry[] }>(
      `/v1/workers/logs?worker=${encodeURIComponent(worker)}`,
    ),

  // Regenerate a message (streams SSE like streamComplete)
  regenerateMessage: async (
    convId: string,
    msgId: string,
    onToken: (token: string) => void,
  ): Promise<string> => {
    const token = getAccessToken();
    const headers: Record<string, string> = {
      "Content-Type": "application/json",
      Accept: "text/event-stream",
    };
    const clientTimezone = getClientTimezone();
    if (clientTimezone) headers["X-Client-Timezone"] = clientTimezone;
    if (token) headers["Authorization"] = `Bearer ${token}`;

    const res = await fetch(
      `${getApiBase()}/v1/conversations/${convId}/messages/${msgId}/regenerate`,
      {
        method: "POST",
        headers,
      },
    );

    if (!res.ok || !res.body) {
      const text = await res.text();
      throw new ApiError(res.status, text);
    }

    const reader = res.body.getReader();
    const decoder = new TextDecoder();
    let fullOutput = "";
    let buffer = "";

    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      buffer += decoder.decode(value, { stream: true });
      const lines = buffer.split("\n");
      buffer = lines.pop() ?? "";
      let eventType = "";
      for (const line of lines) {
        if (line.startsWith("event: ")) {
          eventType = line.slice(7).trim();
        } else if (line.startsWith("data: ")) {
          const data = JSON.parse(line.slice(6));
          if (eventType === "delta" && data.token) {
            fullOutput += data.token;
            onToken(data.token);
          } else if (eventType === "done") {
            fullOutput = data.output ?? fullOutput;
          } else if (eventType === "error") {
            throw new ApiError(502, data.error ?? "stream error");
          }
          eventType = "";
        }
      }
    }
    return fullOutput;
  },

  // Server command
  runCommand: (command: string, timeout: number) =>
    api<RunCommandResponse>("/v1/server/command", {
      method: "POST",
      body: JSON.stringify({ command, timeoutSeconds: timeout }),
    }),

  // Memory
  listMemories: () => api<{ memories: MemoryEntry[] }>("/v1/memory"),
  createMemory: (entry: Omit<MemoryEntry, "id">) =>
    api<MemoryEntry>("/v1/memory", { method: "POST", body: JSON.stringify(entry) }),
  deleteMemory: (id: string) => api(`/v1/memory/${id}`, { method: "DELETE" }),

  // Email test
  testEmailConnection: (params: {
    imapHost: string;
    imapPort: number;
    username: string;
    password: string;
    useTls: boolean;
  }) =>
    api<{ ok: boolean; error?: string; detail?: string }>("/v1/email/test", {
      method: "POST",
      body: JSON.stringify(params),
    }),

  // Email autoconfig
  emailAutoconfig: (email: string) =>
    api<{
      imapHost?: string;
      imapPort?: number;
      imapUsername?: string;
      smtpHost?: string;
      smtpPort?: number;
      useTls?: boolean;
      source?: string;
    }>("/v1/email/autoconfig", {
      method: "POST",
      body: JSON.stringify({ email }),
    }),

  // SSH test
  testSSHConnection: (params: {
    host: string;
    port: number;
    username: string;
    authMode: string;
    sshKey?: string;
    password?: string;
    passphrase?: string;
  }) =>
    api<{ ok: boolean; error?: string }>("/v1/ssh/test", {
      method: "POST",
      body: JSON.stringify(params),
    }),

  // DAV test
  testDAVConnection: (params: DAVConfig) =>
    api<DAVTestResult>("/v1/dav/test", {
      method: "POST",
      body: JSON.stringify({
        url: params.url,
        username: params.username,
        password: params.password,
        enabled: params.enabled,
        webdavEnabled: params.webdavEnabled,
        caldavEnabled: params.caldavEnabled,
        carddavEnabled: params.carddavEnabled,
        pollIntervalSeconds: params.pollIntervalSeconds,
      }),
    }),

  // Voice
  getVoiceStatus: () =>
    api<{ status: "ok" | "downloading" | "down"; model: string }>("/v1/voice/status"),
  transcribeAudio: (audioBlob: Blob) => {
    const form = new FormData();
    form.append("audio", audioBlob, "recording.webm");
    return api<{ transcript: string }>("/v1/voice/transcribe", { method: "POST", body: form });
  },

  // Tasks
  listTasks: () => api<{ tasks: TaskDTO[] }>("/v1/tasks"),
  createTask: (task: {
    description: string;
    prompt: string;
    executeAt: string;
    cronExpression?: string | null;
  }) => api<TaskDTO>("/v1/tasks", { method: "POST", body: JSON.stringify(task) }),
  deleteTask: (id: string) => api(`/v1/tasks/${id}`, { method: "DELETE" }),
  updateTask: (
    id: string,
    patch: {
      description?: string;
      prompt?: string;
      executeAt?: string;
      cronExpression?: string | null;
      status?: string;
    },
  ) => api<TaskDTO>(`/v1/tasks/${id}`, { method: "PATCH", body: JSON.stringify(patch) }),

  // Device Tasks
  listDeviceTasks: () => api<{ tasks: DeviceTaskDTO[] }>("/v1/devices/tasks"),
  createDeviceTask: (req: CreateDeviceTaskRequest) =>
    api<DeviceTaskDTO>("/v1/devices/tasks", { method: "POST", body: JSON.stringify(req) }),
  deleteDeviceTask: (id: string) => api(`/v1/devices/tasks/${id}`, { method: "DELETE" }),
  completeDeviceTask: (id: string, req: CompleteDeviceTaskRequest) =>
    api(`/v1/devices/tasks/${id}/complete`, { method: "POST", body: JSON.stringify(req) }),

  // Device Registration
  registerDevice: (deviceId: string, capabilities: DeviceCapability[]) =>
    api<DeviceRegistration>(`/v1/devices/${deviceId}/register`, {
      method: "POST",
      body: JSON.stringify({ capabilities }),
    }),
  listDeviceRegistrations: () =>
    api<{ registrations: DeviceRegistration[] }>("/v1/devices/registrations"),

  // Device Pairing
  deleteDevice: (deviceId: string) => api(`/v1/devices/${deviceId}`, { method: "DELETE" }),
  createDeviceTokens: (deviceLabel: string) =>
    api<{ tokens: { accessToken: string; refreshToken: string } }>("/v1/auth/device-tokens", {
      method: "POST",
      body: JSON.stringify({ deviceLabel }),
    }),
};
