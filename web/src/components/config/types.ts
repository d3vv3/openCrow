// config/types.ts — Shared types and constants for ConfigStudio tabs.

import type {
  UserConfig,
  EmailAccountConfig,
  GolangToolEntry,
  ProviderConfig,
  SkillEntry,
  ScheduleEntry,
  MCPServerConfig,
  DAVConfig,
} from "@/lib/api";

// ─── Shared Types ───

export type UpdateConfigFn = (fn: (c: UserConfig) => UserConfig) => void;
export type ProviderProbeStatus = { ok: boolean; latencyMs: number; error?: string } | null;

// ─── Tab Constants ───

export const TABS = [
  { key: "email", label: "Email" },
  { key: "servers", label: "Servers" },
  { key: "dav", label: "DAV" },
  { key: "channels", label: "Channels" },
  { key: "devices", label: "Devices" },
  { key: "tools", label: "Tools" },
  { key: "skills", label: "Skills" },
  { key: "mcp", label: "MCP" },
  { key: "providers", label: "Providers" },
  { key: "soul", label: "Soul" },
  { key: "schedules", label: "Schedules" },
  { key: "heartbeat", label: "Heartbeat" },
  { key: "voice", label: "Voice" },
] as const;

export const PROVIDER_KINDS = [
  { value: "openai", label: "OpenAI" },
  { value: "anthropic", label: "Anthropic" },
  { value: "ollama", label: "Ollama" },
  { value: "openrouter", label: "OpenRouter" },
  { value: "deepseek", label: "DeepSeek" },
  { value: "litellm", label: "LiteLLM" },
  { value: "custom", label: "Custom (OpenAI-compatible)" },
];

// ─── Empty Defaults ───

export const emptyEmailAccount: EmailAccountConfig = {
  label: "",
  address: "",
  imapHost: "",
  imapPort: 993,
  imapUsername: "",
  imapPassword: "",
  smtpHost: "",
  smtpPort: 587,
  tls: true,
  enabled: true,
  pollIntervalSeconds: 900,
};

export const emptyGolangTool: GolangToolEntry = {
  name: "",
  description: "",
  sourceCode: `package tools

import (
\t"context"
\t"encoding/json"
\t"fmt"
\t"net/http"
\t"time"
)

type ToolInput struct {
\tQuery string \`json:"query"\`
}

type ToolOutput struct {
\tResult string \`json:"result"\`
}

func Run(ctx context.Context, rawInput json.RawMessage) (json.RawMessage, error) {
\tvar input ToolInput
\tif err := json.Unmarshal(rawInput, &input); err != nil {
\t\treturn nil, fmt.Errorf("invalid input: %w", err)
\t}

\tclient := &http.Client{Timeout: 10 * time.Second}
\tresp, err := client.Get("https://api.example.com/search?q=" + input.Query)
\tif err != nil {
\t\treturn nil, fmt.Errorf("request failed: %w", err)
\t}
\tdefer resp.Body.Close()

\toutput := ToolOutput{
\t\tResult: fmt.Sprintf("searched for: %s (status %d)", input.Query, resp.StatusCode),
\t}

\treturn json.Marshal(output)
}`,
  enabled: true,
};

export const emptyProvider: ProviderConfig = {
  kind: "openai",
  name: "",
  baseUrl: "",
  apiKeyRef: "",
  model: "",
  enabled: true,
};

export const emptySkill: SkillEntry = {
  name: "",
  description: "",
  content: "",
  enabled: true,
};

export const emptySchedule: ScheduleEntry = {
  description: "",
  status: "active",
  executeAt: "",
  cronExpression: "0 * * * *",
  prompt: "",
};

export const emptyMCPServer: MCPServerConfig = {
  name: "",
  url: "",
  headers: {},
  enabled: true,
};

export const emptyDAVConfig: DAVConfig = {
  id: "",
  name: "",
  url: "",
  username: "",
  password: "",
  enabled: false,
  webdavEnabled: true,
  caldavEnabled: true,
  carddavEnabled: true,
  pollIntervalSeconds: 900,
};

// ─── Helpers ───

export function isOpenAICompatibleProviderKind(kind: string): boolean {
  const normalized = kind.trim().toLowerCase();
  return (
    normalized === "openai" ||
    normalized === "custom" ||
    normalized === "openrouter" ||
    normalized === "litellm"
  );
}
