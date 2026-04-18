"use client";

import { useState, useEffect, useCallback } from "react";
import React from "react";
import cronstrue from "cronstrue";
import type { ReactElement } from "react";
import {
  endpoints,
  type UserConfig,
  type EmailAccountConfig,
  type ToolDefinition,
  type ToolParameter,
  type GolangToolEntry,
  type ProviderConfig,
  type SkillEntry,
  type SkillFile,
  type MemoryEntry,
  type TaskDTO,
  type ScheduleEntry,
  type MCPServerConfig,
  type MCPToolSummary,
  type MCPServerTestResult,
} from "@/lib/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { TextArea } from "@/components/ui/TextArea";
import { Toggle } from "@/components/ui/Toggle";
import { Select } from "@/components/ui/Select";
import { Card } from "@/components/ui/Card";
import { AnimatedDot } from "@/components/ui/AnimatedDot";
import { Badge } from "@/components/ui/Badge";
import { Chip } from "@/components/ui/Chip";
import { Spinner } from "@/components/ui/Spinner";
import { SectionHeader } from "@/components/ui/SectionHeader";
import { IconButton } from "@/components/ui/IconButton";

// ─── Constants ───

const TABS = [
  { key: "email", label: "Email" },
  { key: "tools", label: "Tools" },
  { key: "skills", label: "Skills" },
  { key: "mcp", label: "MCP" },
  { key: "providers", label: "Providers" },
  { key: "soul", label: "Soul" },
  { key: "memory", label: "Memory" },
  { key: "schedules", label: "Schedules" },
  { key: "heartbeat", label: "Heartbeat" },
] as const;

const PROVIDER_KINDS = [
  { value: "openai", label: "OpenAI" },
  { value: "anthropic", label: "Anthropic" },
  { value: "ollama", label: "Ollama" },
  { value: "openrouter", label: "OpenRouter" },
  { value: "custom", label: "Custom (OpenAI-compatible)" },
];

const GOLANG_TOOL_EXAMPLE = `package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ToolInput defines the parameters your tool receives.
type ToolInput struct {
	Query string \`json:"query"\`
}

// ToolOutput defines what your tool returns.
type ToolOutput struct {
	Result string \`json:"result"\`
}

// Run is the entry point called by the openCrow runtime.
// It receives a JSON-encoded input and returns JSON-encoded output.
func Run(ctx context.Context, rawInput json.RawMessage) (json.RawMessage, error) {
	var input ToolInput
	if err := json.Unmarshal(rawInput, &input); err != nil {
		return nil, fmt.Errorf("invalid input: %w", err)
	}

	// Example: make an HTTP request with timeout
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get("https://api.example.com/search?q=" + input.Query)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	output := ToolOutput{
		Result: fmt.Sprintf("searched for: %s (status %d)", input.Query, resp.StatusCode),
	}

	return json.Marshal(output)
}`;

// ─── Defaults ───

const emptyEmailAccount: EmailAccountConfig = {
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
};

const emptyGolangTool: GolangToolEntry = {
  name: "",
  description: "",
  sourceCode: GOLANG_TOOL_EXAMPLE,
  enabled: true,
};

const emptyProvider: ProviderConfig = {
  kind: "openai",
  name: "",
  baseUrl: "",
  apiKeyRef: "",
  model: "",
  enabled: true,
};

const emptySkill: SkillEntry = {
  name: "",
  description: "",
  content: "",
  enabled: true,
};

const emptyMemory: MemoryEntry = {
  category: "",
  content: "",
  confidence: 50,
};

const emptySchedule: ScheduleEntry = {
  description: "",
  status: "active",
  executeAt: "",
  cronExpression: "0 * * * *",
  prompt: "",
};

const emptyMCPServer: MCPServerConfig = {
  name: "",
  url: "",
  headers: {},
  enabled: true,
};

function McpServerCard({
  server,
  index: i,
  updateConfig,
}: {
  server: MCPServerConfig;
  index: number;
  updateConfig: UpdateConfigFn;
}) {
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<MCPServerTestResult | null>(null);
  const [tools, setTools] = useState<MCPToolSummary[]>([]);

  const handleTest = async () => {
    if (!server.url.trim()) {
      setTestResult({ ok: false, latencyMs: 0, error: "Server URL is required" });
      return;
    }
    setTesting(true);
    setTestResult(null);
    try {
      const result = await endpoints.testMCPServer({
        name: server.name,
        url: server.url,
        headers: server.headers ?? {},
      });
      setTestResult(result);
      setTools(result.tools ?? []);
    } catch (err) {
      setTestResult({ ok: false, latencyMs: 0, error: err instanceof Error ? err.message : "Test failed" });
      setTools([]);
    } finally {
      setTesting(false);
    }
  };

  const headerRows = Object.entries(server.headers ?? {});

  return (
    <Card key={server.id ?? i} title={server.name || `MCP Server ${i + 1}`}>
      <div className="space-y-4">
        <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
          <Input
            label="Name"
            value={server.name}
            onChange={(e) => updateConfig((c) => {
              c.mcp.servers[i].name = e.target.value;
              return c;
            })}
            placeholder="My MCP"
          />
          <Input
            label="Server URL"
            value={server.url}
            onChange={(e) => updateConfig((c) => {
              c.mcp.servers[i].url = e.target.value;
              return c;
            })}
            placeholder="https://example.com/mcp"
          />
        </div>

        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <p className="text-xs uppercase tracking-wide text-on-surface-variant">Headers</p>
            <Button
              variant="ghost"
              size="sm"
              onClick={() => updateConfig((c) => {
                const current = c.mcp.servers[i].headers ?? {};
                c.mcp.servers[i].headers = { ...current, "": "" };
                return c;
              })}
            >
              Add Header
            </Button>
          </div>

          {headerRows.length === 0 && (
            <p className="text-xs text-on-surface-variant">No custom headers configured.</p>
          )}

          {headerRows.map(([key, value], hi) => (
            <div key={`header-${i}-${hi}`} className="grid grid-cols-1 sm:grid-cols-[1fr_1fr_auto] gap-2">
              <Input
                label="Key"
                value={key}
                onChange={(e) => updateConfig((c) => {
                  const rows = Object.entries(c.mcp.servers[i].headers ?? {});
                  const next: Record<string, string> = {};
                  rows.forEach(([k, v], idx) => {
                    if (idx === hi) {
                      next[e.target.value] = v;
                    } else {
                      next[k] = v;
                    }
                  });
                  c.mcp.servers[i].headers = next;
                  return c;
                })}
                placeholder="Authorization"
              />
              <Input
                label="Value"
                value={value}
                onChange={(e) => updateConfig((c) => {
                  const rows = Object.entries(c.mcp.servers[i].headers ?? {});
                  const next: Record<string, string> = {};
                  rows.forEach(([k, v], idx) => {
                    if (idx === hi) {
                      next[k] = e.target.value;
                    } else {
                      next[k] = v;
                    }
                  });
                  c.mcp.servers[i].headers = next;
                  return c;
                })}
                placeholder="Bearer ..."
              />
              <div className="flex items-end">
                <Button
                  variant="ghost"
                  size="sm"
                  className="hover:text-error"
                  onClick={() => updateConfig((c) => {
                    const rows = Object.entries(c.mcp.servers[i].headers ?? {});
                    const next: Record<string, string> = {};
                    rows.forEach(([k, v], idx) => {
                      if (idx !== hi) next[k] = v;
                    });
                    c.mcp.servers[i].headers = next;
                    return c;
                  })}
                >
                  Remove
                </Button>
              </div>
            </div>
          ))}
        </div>

        <div className="flex items-center gap-4 flex-wrap">
          <Toggle
            label="Enabled"
            checked={server.enabled}
            onChange={(v) => updateConfig((c) => {
              c.mcp.servers[i].enabled = v;
              return c;
            })}
          />
          <Button
            variant="secondary"
            size="sm"
            loading={testing}
            onClick={handleTest}
            disabled={!server.url.trim()}
          >
            Test connection
          </Button>
          <Button
            variant="ghost"
            size="sm"
            className="ml-auto hover:text-error"
            onClick={() => updateConfig((c) => {
              c.mcp.servers.splice(i, 1);
              return c;
            })}
          >
            Remove Server
          </Button>
        </div>

        {testResult && (
          <div className={`rounded-lg px-3 py-2 text-sm border ${testResult.ok ? "bg-green-400/10 text-green-400 border-green-400/20" : "bg-red-400/10 text-red-400 border-red-400/20"}`}>
            <div className="flex items-center gap-2">
              <span>{testResult.ok ? "Connected" : "Failed"}</span>
              <span className="text-xs opacity-80">{testResult.latencyMs}ms</span>
            </div>
            {testResult.error && <p className="text-xs mt-1 opacity-90 break-all">{testResult.error}</p>}
          </div>
        )}

        <div className="space-y-2">
          <p className="text-xs uppercase tracking-wide text-on-surface-variant">Exposed Tools</p>
          {tools.length === 0 ? (
            <p className="text-xs text-on-surface-variant">Run test to fetch MCP tools.</p>
          ) : (
            <div className="flex flex-wrap gap-2">
              {tools.map((tool) => (
                <Chip key={tool.name} className="bg-violet/12 text-violet">
                  {tool.name}
                </Chip>
              ))}
            </div>
          )}
        </div>
      </div>
    </Card>
  );
}

// ─── Save Bar ───

function SaveBar({
  onClick,
  loading,
  label,
  status,
}: {
  onClick: () => void;
  loading: boolean;
  label: string;
  status?: string | null;
}) {
  return (
    <div className="flex items-center gap-3">
      <Button onClick={onClick} loading={loading}>
        {label}
      </Button>
      {status && <span className="text-cyan text-sm">{status}</span>}
    </div>
  );
}

// ─── ProviderCard ───

type UpdateConfigFn = (fn: (c: UserConfig) => UserConfig) => void;

type ProviderProbeStatus = { ok: boolean; latencyMs: number; error?: string } | null;

function ProviderCard({
  prov,
  index: i,
  updateConfig,
  probeStatus,
}: {
  prov: ProviderConfig;
  index: number;
  updateConfig: UpdateConfigFn;
  probeStatus?: ProviderProbeStatus;
}) {
  const configured = !!(prov.name && prov.model && (prov.apiKeyRef || prov.baseUrl));
  const [expanded, setExpanded] = useState(!configured);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; latencyMs: number; error?: string } | null>(null);

  const handleTest = async (e: React.MouseEvent) => {
    e.stopPropagation();
    setTesting(true);
    setTestResult(null);
    try {
      const res = await endpoints.testProvider({
        kind: prov.kind,
        name: prov.name,
        baseUrl: prov.baseUrl,
        apiKeyRef: prov.apiKeyRef,
        model: prov.model,
      });
      setTestResult(res);
    } catch (err) {
      setTestResult({ ok: false, latencyMs: 0, error: err instanceof Error ? err.message : "Test failed" });
    } finally {
      setTesting(false);
    }
  };

  // Manual test result takes priority over background probe
  const displayStatus = testResult ?? probeStatus;

  const statusDotStatus = !prov.enabled
    ? "idle" as const
    : (displayStatus === null || displayStatus === undefined)
      ? "pending" as const
      : displayStatus.ok
        ? "ok" as const
        : "error" as const;

  return (
    <div className="rounded-xl border border-outline-ghost bg-surface-low overflow-hidden">
      <button
        className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-surface-mid/40 transition-colors"
        onClick={() => setExpanded((e) => !e)}
      >
        <AnimatedDot status={statusDotStatus} />
        <span className="font-body font-medium text-on-surface flex-1">
          {prov.name || `Provider ${i + 1}`}
        </span>
        {configured && (
          <span className="text-xs text-on-surface-variant font-mono shrink-0">
            {prov.kind} . {prov.model}
          </span>
        )}
        {displayStatus && (
          <span className={`text-xs font-mono shrink-0 ${displayStatus.ok ? "text-green-400" : "text-red-400"}`}>
            {displayStatus.ok ? `${displayStatus.latencyMs}ms` : "offline"}
          </span>
        )}
        <svg
          className={`h-4 w-4 shrink-0 text-on-surface-variant transition-transform duration-150 ${expanded ? "rotate-180" : ""}`}
          viewBox="0 0 16 16" fill="none"
        >
          <path d="M4 6l4 4 4-4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="round" strokeLinejoin="round" />
        </svg>
      </button>

      {expanded && (
        <div className="px-4 pb-4 space-y-4 border-t border-outline-ghost pt-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Select label="Kind" options={PROVIDER_KINDS} value={prov.kind} onChange={(e) => updateConfig((c) => { c.llm.providers[i].kind = e.target.value; return c; })} />
            <Input label="Name" value={prov.name} onChange={(e) => updateConfig((c) => { c.llm.providers[i].name = e.target.value; return c; })} />
            <Input label="Model" value={prov.model} onChange={(e) => updateConfig((c) => { c.llm.providers[i].model = e.target.value; return c; })} />
            <Input label="Base URL" value={prov.baseUrl} onChange={(e) => updateConfig((c) => { c.llm.providers[i].baseUrl = e.target.value; return c; })} />
            <Input label="API Key" type="password" value={prov.apiKeyRef} onChange={(e) => updateConfig((c) => { c.llm.providers[i].apiKeyRef = e.target.value; return c; })} />
          </div>

          {testResult && (
            <div className={`flex items-center gap-2 rounded-lg px-3 py-2 text-sm border ${testResult.ok ? "bg-green-400/10 text-green-400 border-green-400/20" : "bg-red-400/10 text-red-400 border-red-400/20"}`}>
              <span>{testResult.ok ? "Connected" : "Failed"}</span>
              {testResult.ok && <span className="text-xs opacity-70">{testResult.latencyMs}ms</span>}
              {testResult.error && <span className="text-xs opacity-80 ml-1 truncate">{testResult.error}</span>}
            </div>
          )}

          <div className="flex items-center gap-3 flex-wrap">
            <Toggle label="Enabled" checked={prov.enabled} onChange={(v) => updateConfig((c) => { c.llm.providers[i].enabled = v; return c; })} />
            <div className="flex items-center gap-1.5">
              <label className="text-xs text-on-surface-variant">Priority</label>
              <input
                type="number"
                min={0}
                className="w-16 rounded border border-white/10 bg-white/5 px-2 py-1 text-xs text-on-surface focus:outline-none focus:border-violet/40"
                value={prov.priority ?? 0}
                onChange={(e) => updateConfig((c) => { c.llm.providers[i].priority = parseInt(e.target.value, 10) || 0; return c; })}
              />
              <span className="text-xs text-on-surface-variant">(lower = higher priority)</span>
            </div>
            <Button
              variant="secondary"
              size="sm"
              loading={testing}
              onClick={handleTest}
              disabled={!prov.kind}
            >
              Test connection
            </Button>
            <Button variant="ghost" size="sm" className="ml-auto hover:text-error" onClick={() => updateConfig((c) => { c.llm.providers.splice(i, 1); return c; })}>
              Remove
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

// ─── Component ───

function MemoryRow({
  mem,
  index: i,
  onUpdate,
  onDelete,
}: {
  mem: MemoryEntry;
  index: number;
  onUpdate: (updated: MemoryEntry) => void;
  onDelete: () => void;
}) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="rounded-lg border border-white/10 bg-surface-mid overflow-hidden">
      {/* Compact row */}
      <button
        className="w-full flex items-center gap-3 px-4 py-2.5 text-left hover:bg-white/5 transition-colors"
        onClick={() => setExpanded((v) => !v)}
      >
        <span className="text-xs font-mono text-cyan/70 bg-cyan/10 px-1.5 py-0.5 rounded shrink-0">{mem.category || "--"}</span>
        <span className="text-sm text-on-surface flex-1 truncate">{mem.content}</span>
        <span className="text-xs text-on-surface-variant shrink-0">{mem.confidence ?? 50}%</span>
        <svg className={`w-3.5 h-3.5 text-on-surface-variant transition-transform flex-shrink-0 ${expanded ? "rotate-180" : ""}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {/* Expanded editor */}
      {expanded && (
        <div className="px-4 pb-4 space-y-3 border-t border-white/10 pt-3">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="Category"
              value={mem.category}
              onChange={(e) => onUpdate({ ...mem, category: e.target.value })}
            />
            <Input
              label="Confidence (%)"
              type="number"
              min={0}
              max={100}
              step={5}
              value={mem.confidence ?? 50}
              onChange={(e) => onUpdate({ ...mem, confidence: Math.min(100, Math.max(0, parseInt(e.target.value) || 50)) })}
            />
          </div>
          <TextArea
            label="Content"
            value={mem.content}
            onChange={(e) => onUpdate({ ...mem, content: e.target.value })}
            rows={3}
          />
          <Button variant="ghost" size="sm" className="hover:text-error" onClick={onDelete}>
            Remove
          </Button>
        </div>
      )}
    </div>
  );
}

function EmailAccountCard({
  acct,
  index: i,
  updateConfig,
  setError,
}: {
  acct: EmailAccountConfig;
  index: number;
  updateConfig: UpdateConfigFn;
  setError: (e: string | null) => void;
}) {
  const configured = !!(acct.imapHost && acct.imapUsername);
  const [expanded, setExpanded] = useState(!configured);
  const [testing, setTesting] = useState(false);
  const [testResult, setTestResult] = useState<{ ok: boolean; error?: string; detail?: string } | null>(null);

  const handleTest = async (e: React.MouseEvent) => {
    e.stopPropagation();
    setTesting(true);
    setTestResult(null);
    try {
      const res = await endpoints.testEmailConnection({
        imapHost: acct.imapHost,
        imapPort: acct.imapPort,
        username: acct.imapUsername ?? "",
        password: acct.imapPassword ?? "",
        useTls: acct.tls,
      });
      setTestResult(res);
    } catch {
      setTestResult({ ok: false, error: "Request failed" });
    } finally {
      setTesting(false);
    }
  };

  return (
    <div className="rounded-lg border border-white/10 bg-surface-mid overflow-hidden">
      {/* Header row */}
      <button
        className="w-full flex items-center gap-3 px-4 py-3 text-left hover:bg-white/5 transition-colors"
        onClick={() => setExpanded((v) => !v)}
      >
        <AnimatedDot status={acct.enabled ? "ok" : "idle"} />
        <span className="font-medium text-sm flex-1 truncate">{acct.label || `Account ${i + 1}`}</span>
        {acct.address && <span className="text-xs text-on-surface-variant truncate hidden sm:block">{acct.address}</span>}
        {testResult && (
          <span className={`flex items-center gap-1.5 text-xs font-mono px-2 py-0.5 rounded ${testResult.ok ? "text-cyan bg-cyan/10" : "text-error bg-error/10"}`}>
            <AnimatedDot status={testResult.ok ? "ok" : "error"} />
            {testResult.ok ? "OK" : "FAIL"}
          </span>
        )}
        <svg className={`w-4 h-4 text-on-surface-variant transition-transform flex-shrink-0 ${expanded ? "rotate-180" : ""}`} fill="none" viewBox="0 0 24 24" stroke="currentColor">
          <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M19 9l-7 7-7-7" />
        </svg>
      </button>

      {/* Expanded content */}
      {expanded && (
        <div className="px-4 pb-4 space-y-4 border-t border-white/10 pt-4">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Input label="Label" value={acct.label} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].label = e.target.value; return c; })} />
            <Input label="Address" value={acct.address} onChange={(e) => updateConfig((c) => {
              const newAddr = e.target.value;
              const prev = c.integrations.emailAccounts[i];
              // Auto-fill IMAP username if it's blank or still mirrors the old address
              if (!prev.imapUsername || prev.imapUsername === prev.address) {
                prev.imapUsername = newAddr;
              }
              prev.address = newAddr;
              return c;
            })} />
            <Input label="IMAP Host" value={acct.imapHost} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].imapHost = e.target.value; return c; })} />
            <Input label="IMAP Port" type="number" value={acct.imapPort} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].imapPort = parseInt(e.target.value) || 0; return c; })} />
            <Input label="IMAP Username" value={acct.imapUsername ?? ""} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].imapUsername = e.target.value; return c; })} />
            <Input label="IMAP Password" type="password" value={acct.imapPassword ?? ""} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].imapPassword = e.target.value; return c; })} />
            <Input label="SMTP Host" value={acct.smtpHost} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].smtpHost = e.target.value; return c; })} />
            <Input label="SMTP Port" type="number" value={acct.smtpPort} onChange={(e) => updateConfig((c) => { c.integrations.emailAccounts[i].smtpPort = parseInt(e.target.value) || 0; return c; })} />
          </div>
          <div className="flex items-center gap-6">
            <Toggle label="TLS" checked={acct.tls} onChange={(v) => updateConfig((c) => { c.integrations.emailAccounts[i].tls = v; return c; })} />
            <Toggle label="Enabled" checked={acct.enabled} onChange={(v) => updateConfig((c) => { c.integrations.emailAccounts[i].enabled = v; return c; })} />
          </div>
          {testResult && !testResult.ok && (
            <p className="text-xs text-error font-mono">{testResult.error}</p>
          )}
          {testResult?.ok && testResult.detail && (
            <p className="text-xs text-cyan font-mono">{testResult.detail}</p>
          )}
          <div className="flex gap-2 pt-2">
            <Button variant="secondary" size="sm" loading={testing} onClick={handleTest}>
              Test connection
            </Button>
            <Button variant="ghost" size="sm" className="ml-auto hover:text-error" onClick={() => updateConfig((c) => { c.integrations.emailAccounts.splice(i, 1); return c; })}>
              Remove
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}

function parseCron(expr: string): string {
  try {
    return cronstrue.toString(expr, { throwExceptionOnParseError: true });
  } catch {
    return "";
  }
}

function SchedulesTab({
  tasks,
  tasksLoading,
  refreshTasks,
  setTasks,
  setError,
}: {
  tasks: TaskDTO[];
  tasksLoading: boolean;
  refreshTasks: () => void;
  setTasks: React.Dispatch<React.SetStateAction<TaskDTO[]>>;
  setError: (e: string | null) => void;
}) {
  const [newTask, setNewTask] = useState({ description: "", prompt: "", executeAt: "", cronExpression: "" });
  const [creating, setCreating] = useState(false);
  const [expandedTaskId, setExpandedTaskId] = useState<string | null>(null);

  const handleCreate = async () => {
    if (!newTask.prompt || !newTask.executeAt) return;
    setCreating(true);
    try {
      await endpoints.createTask({
        description: newTask.description || newTask.prompt.slice(0, 80),
        prompt: newTask.prompt,
        executeAt: newTask.executeAt,
        cronExpression: newTask.cronExpression || null,
      });
      setNewTask({ description: "", prompt: "", executeAt: "", cronExpression: "" });
      refreshTasks();
    } catch {
      setError("Failed to create task");
    } finally {
      setCreating(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await endpoints.deleteTask(id);
      setTasks((prev) => prev.filter((t) => t.id !== id));
    } catch {
      setError("Failed to delete task");
    }
  };

  const formatDate = (iso: string) => {
    try {
      return new Date(iso).toLocaleString();
    } catch {
      return iso;
    }
  };

  const cronHint = parseCron(newTask.cronExpression);

  return (
    <div className="space-y-4">
      <SectionHeader
        title="Schedules"
        description="Scheduled tasks and cron jobs -- managed by the assistant or created manually"
        action={
          <Button variant="secondary" size="sm" onClick={refreshTasks} loading={tasksLoading}>
            Refresh
          </Button>
        }
      />

      {/* Create form */}
      <Card title="New Schedule">
        <div className="space-y-3">
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
            <Input
              label="Description (optional)"
              value={newTask.description}
              onChange={(e) => setNewTask((p) => ({ ...p, description: e.target.value }))}
            />
            <Input
              label="Execute At (one-shot)"
              value={newTask.executeAt}
              onChange={(e) => setNewTask((p) => ({ ...p, executeAt: e.target.value }))}
              placeholder="2026-04-20T09:00:00Z"
            />
            <div>
              <Input
                label="Cron Expression (optional)"
                value={newTask.cronExpression}
                onChange={(e) => setNewTask((p) => ({ ...p, cronExpression: e.target.value }))}
                placeholder="0 9 * * 1-5"
              />
              {cronHint && (
                <p className="mt-1 text-xs text-cyan/80">{cronHint}</p>
              )}
            </div>
          </div>
          <TextArea
            label="Prompt"
            value={newTask.prompt}
            onChange={(e) => setNewTask((p) => ({ ...p, prompt: e.target.value }))}
            rows={2}
          />
          <Button
            variant="primary"
            size="sm"
            onClick={handleCreate}
            loading={creating}
            disabled={!newTask.prompt || !newTask.executeAt}
          >
            Create Schedule
          </Button>
        </div>
      </Card>

      {/* Existing tasks */}
      <div className="space-y-2">
        {tasks.map((task) => {
          const cronDesc = task.cronExpression ? parseCron(task.cronExpression) : null;
          const statusColor =
            task.status === "PENDING" ? "text-cyan" : task.status === "FAILED" ? "text-error" : "text-on-surface-variant";
          return (
            <div
              key={task.id}
              className="rounded-lg border border-white/10 bg-white/5 backdrop-blur-sm px-4 py-3 flex flex-col gap-3"
            >
              <div className="flex items-start gap-3">
                <button
                  type="button"
                  onClick={() => setExpandedTaskId((prev) => prev === task.id ? null : task.id)}
                  className="flex-1 min-w-0 text-left space-y-1"
                >
                  <div className="flex items-center gap-2 flex-wrap">
                    <span className="text-sm font-medium text-on-surface truncate">
                      {task.description || task.prompt.slice(0, 60)}
                    </span>
                    <span className={`text-xs font-mono ${statusColor}`}>{task.status}</span>
                    {task.consecutiveFailures > 0 && (
                      <span className="text-xs text-error">{task.consecutiveFailures} failure{task.consecutiveFailures !== 1 ? "s" : ""}</span>
                    )}
                    <span className="ml-auto text-on-surface-variant text-xs">{expandedTaskId === task.id ? "▲" : "▼"}</span>
                  </div>
                  <div className="flex flex-wrap items-center gap-x-3 gap-y-0.5 text-xs text-on-surface-variant">
                    <span>{formatDate(task.executeAt)}</span>
                    {task.cronExpression && (
                      <span>
                        <code className="font-mono text-cyan">{task.cronExpression}</code>
                        {cronDesc && <span className="ml-1 text-on-surface-variant/70">-- {cronDesc}</span>}
                      </span>
                    )}
                  </div>
                  <p className="text-xs text-on-surface-variant/60 font-mono truncate">{task.prompt}</p>
                  {task.lastResult && (
                    <p className="text-xs text-on-surface-variant/70 truncate">↳ {task.lastResult.split("\n")[0]}</p>
                  )}
                </button>
                <button
                  onClick={() => handleDelete(task.id)}
                  className="text-xs text-on-surface-variant/50 hover:text-error transition-colors shrink-0 self-start"
                  title="Delete task"
                >
                  ✕
                </button>
              </div>
              {expandedTaskId === task.id && (
                <div className="rounded-md border border-outline-ghost bg-surface-low px-3 py-3 space-y-3">
                  <div>
                    <p className="text-[11px] uppercase tracking-wider text-on-surface-variant font-mono mb-1">Prompt</p>
                    <pre className="whitespace-pre-wrap break-words text-xs text-on-surface font-mono">{task.prompt}</pre>
                  </div>
                  <div>
                    <p className="text-[11px] uppercase tracking-wider text-on-surface-variant font-mono mb-1">Last execution result</p>
                    <pre className="whitespace-pre-wrap break-words text-xs text-on-surface font-mono">{task.lastResult || "No execution result yet."}</pre>
                  </div>
                </div>
              )}
            </div>
          );
        })}
      </div>
      {tasks.length === 0 && !tasksLoading && (
        <p className="text-on-surface-variant text-sm">No scheduled tasks. The assistant can create them, or use the form above.</p>
      )}
    </div>
  );
}


function EmailTab({
  config,
  updateConfig,
  saving,
  saveFullConfig,
  saveStatus,
  setError,
}: {
  config: UserConfig;
  updateConfig: UpdateConfigFn;
  saving: boolean;
  saveFullConfig: () => void;
  saveStatus: string | null;
  setError: (e: string | null) => void;
}) {
  return (
    <div className="space-y-6">
      <SectionHeader
        title="Email Accounts"
        description="Manage connected email accounts for IMAP polling"
        action={
          <Button variant="secondary" size="sm" onClick={() => updateConfig((c) => {
            c.integrations.emailAccounts.push({
              label: "", address: "", imapHost: "", imapPort: 993,
              imapUsername: "", imapPassword: "",
              smtpHost: "", smtpPort: 587, tls: true, enabled: true,
            });
            return c;
          })}>
            Add Account
          </Button>
        }
      />
      {config.integrations.emailAccounts.map((acct, i) => (
        <EmailAccountCard key={i} acct={acct} index={i} updateConfig={updateConfig} setError={setError} />
      ))}
      {config.integrations.emailAccounts.length === 0 && (
        <p className="text-on-surface-variant text-sm">No email accounts configured.</p>
      )}
      <SaveBar onClick={saveFullConfig} loading={saving} label="Save Email Config" status={saveStatus} />
    </div>
  );
}


export default function ConfigStudio({ requestedTab }: { requestedTab?: string }) {
  const [config, setConfig] = useState<UserConfig | null>(null);
  const [activeTab, setActiveTab] = useState(requestedTab ?? "email");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saveStatus, setSaveStatus] = useState<string | null>(null);
  const [providerStatuses, setProviderStatuses] = useState<Record<string, ProviderProbeStatus>>({});
  const [memories, setMemories] = useState<MemoryEntry[]>([]);
  const [memoriesLoading, setMemoriesLoading] = useState(false);
  const [tasks, setTasks] = useState<TaskDTO[]>([]);
  const [tasksLoading, setTasksLoading] = useState(false);
  const [skillFiles, setSkillFiles] = useState<SkillFile[]>([]);
  const [installSource, setInstallSource] = useState("");
  const [installing, setInstalling] = useState(false);
  const [editingSkill, setEditingSkill] = useState<SkillFile | null>(null);

  // Respond to parent changing the requested tab (e.g. sidebar button clicked)
  useEffect(() => {
    if (requestedTab) setActiveTab(requestedTab);
  }, [requestedTab]);

  // ─── Load on mount ───

  useEffect(() => {
    (async () => {
      try {
        const cfg = await endpoints.getConfig();
        setConfig(cfg);

        // Background probe all enabled providers
        endpoints.getProvidersStatus().then((res) => {
          const map: Record<string, ProviderProbeStatus> = {};
          for (const p of res.providers) {
            map[p.name] = { ok: p.ok, latencyMs: p.latencyMs, error: p.error };
          }
          setProviderStatuses(map);
        }).catch(() => {});

        // Load memories from DB
        endpoints.listMemories().then((res) => {
          setMemories(res.memories ?? []);
        }).catch(() => {});

        // Load tasks from DB
        endpoints.listTasks().then((res) => {
          setTasks(res.tasks ?? []);
        }).catch(() => {});

        // Load skill files
        endpoints.listSkillFiles().then(setSkillFiles).catch(() => {});
      } catch (e) {
        setError(e instanceof Error ? e.message : "Failed to load config");
      }
    })();
  }, []);

  // ─── Immutable config updater ───

  const updateConfig = useCallback(
    (updater: (draft: UserConfig) => UserConfig) => {
      setConfig((prev) => (prev ? updater(structuredClone(prev)) : prev));
    },
    []
  );

  // ─── Save helpers ───

  const flashSave = (msg: string) => {
    setSaveStatus(msg);
    setTimeout(() => setSaveStatus(null), 3000);
  };

  const saveFullConfig = async () => {
    if (!config) return;
    setSaving(true);
    setError(null);
    try {
      await endpoints.putConfig(config);
      flashSave("Configuration saved");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  const saveTools = async () => {
    if (!config) return;
    setSaving(true);
    setError(null);
    try {
      await endpoints.putTools(config.tools);
      flashSave("Tools saved");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  const saveSkills = async () => {
    if (!config) return;
    setSaving(true);
    setError(null);
    try {
      await endpoints.putSkills(config.skills);
      flashSave("Skills saved");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Save failed");
    } finally {
      setSaving(false);
    }
  };

  // ─── Loading state ───

  const refreshTasks = useCallback(async () => {
    setTasksLoading(true);
    try {
      const res = await endpoints.listTasks();
      setTasks(res.tasks ?? []);
    } catch {
      setError("Failed to load tasks");
    } finally {
      setTasksLoading(false);
    }
  }, []);

  if (!config) {
    return (
      <div className="flex items-center justify-center h-[60vh]">
        {error ? (
          <div className="text-center space-y-3">
            <p className="text-error font-display text-lg">Failed to load configuration</p>
            <p className="text-on-surface-variant text-sm font-mono">{error}</p>
          </div>
        ) : (
          <Spinner size="lg" />
        )}
      </div>
    );
  }

  // ─── Panels ───

  const renderTools = () => (
    <div className="space-y-6">

      {/* Built-in Tool Definitions (read-only) */}
      <SectionHeader title="Built-in Tools" description="Server-provided tools (read-only)" />
      {config.tools.definitions.length === 0 ? (
        <p className="text-on-surface-variant text-sm">No built-in tools registered.</p>
      ) : (
        <div className="space-y-2">
          {config.tools.definitions.map((tool, i) => (
            <div key={i} className="border border-outline-ghost rounded-md p-3 bg-surface-low">
              <div className="flex items-center justify-between mb-2">
                <div className="flex items-center gap-3">
                  <code className="text-sm font-mono text-cyan">{tool.name}</code>
                  <Badge variant="info">{tool.source || "builtin"}</Badge>
                </div>
                <Toggle
                  label="Enabled"
                  checked={config.tools.enabledTools[tool.name] ?? false}
                  onChange={(v) => updateConfig((c) => { c.tools.enabledTools[c.tools.definitions[i].name] = v; return c; })}
                />
              </div>
              <p className="text-xs text-on-surface-variant mb-2">{tool.description}</p>
              {tool.parameters.length > 0 && (
                <div className="flex flex-wrap gap-1">
                  {tool.parameters.map((param, pi) => (
                    <span key={pi} className="inline-flex items-center gap-1 rounded bg-surface-mid px-2 py-0.5 text-xs font-mono text-on-surface-variant">
                      {param.name}
                      <span className="text-on-surface-variant/60">:{param.type}</span>
                      {param.required && <span className="text-error">*</span>}
                    </span>
                  ))}
                </div>
              )}
            </div>
          ))}
        </div>
      )}

      {/* Golang Tools */}
      <SectionHeader
        title="Golang Tools"
        description="Custom server-side Go tool implementations"
        action={
          <Button variant="secondary" size="sm" onClick={() => updateConfig((c) => { c.tools.golangTools.push({ ...emptyGolangTool }); return c; })}>
            Add Golang Tool
          </Button>
        }
      />
      {config.tools.golangTools.map((gt, i) => (
        <Card key={i} title={gt.name || `Golang Tool ${i + 1}`}>
          <div className="space-y-3">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
              <Input label="Name" value={gt.name} onChange={(e) => updateConfig((c) => { c.tools.golangTools[i].name = e.target.value; return c; })} />
              <Input label="Description" value={gt.description} onChange={(e) => updateConfig((c) => { c.tools.golangTools[i].description = e.target.value; return c; })} />
            </div>
            <TextArea
              label="Source Code"
              value={gt.sourceCode}
              onChange={(e) => updateConfig((c) => { c.tools.golangTools[i].sourceCode = e.target.value; return c; })}
              rows={12}
              className="font-mono text-xs"
            />
            <div className="flex items-center gap-4">
              <Toggle label="Enabled" checked={gt.enabled} onChange={(v) => updateConfig((c) => { c.tools.golangTools[i].enabled = v; return c; })} />
              <Button variant="ghost" size="sm" className="ml-auto hover:text-error" onClick={() => updateConfig((c) => { c.tools.golangTools.splice(i, 1); return c; })}>
                Remove
              </Button>
            </div>
          </div>
        </Card>
      ))}
      {config.tools.golangTools.length === 0 && (
        <p className="text-on-surface-variant text-sm">No Golang tools. Click &quot;Add Golang Tool&quot; to create one with example code.</p>
      )}

      <SaveBar onClick={saveTools} loading={saving} label="Save Tools" status={saveStatus} />
    </div>
  );

  const renderSkills = () => (
    <div className="space-y-8">
      {/* ── File-based Skills ── */}
      <div className="space-y-4">
        <SectionHeader
          title="Installed Skills"
          description="SKILL.md files installed from GitHub or created manually"
          action={
            <Button variant="secondary" size="sm" onClick={() => setEditingSkill({ slug: "", name: "", description: "", content: "" })}>
              New Skill
            </Button>
          }
        />

        {/* Install from GitHub */}
        <Card title="Install from GitHub">
          <div className="space-y-3">
            <Input
              label="GitHub source (owner/repo)"
              value={installSource}
              onChange={(e) => setInstallSource(e.target.value)}
              placeholder="e.g. myorg/my-skills"
            />
            <Button
              variant="secondary"
              size="sm"
              loading={installing}
              onClick={async () => {
                if (!installSource.trim()) return;
                setInstalling(true);
                try {
                  const result = await endpoints.installSkills(installSource.trim());
                  const updated = await endpoints.listSkillFiles();
                  setSkillFiles(updated);
                  setInstallSource("");
                  setSaveStatus(`Installed ${result.count} skill(s)`);
                  setTimeout(() => setSaveStatus(null), 3000);
                } catch (e) {
                  setError(e instanceof Error ? e.message : "Install failed");
                } finally {
                  setInstalling(false);
                }
              }}
            >
              Install Skills
            </Button>
          </div>
        </Card>

        {skillFiles.length === 0 && (
          <p className="text-on-surface-variant text-sm">No skill files installed.</p>
        )}
        {skillFiles.map((sf) => (
          <Card key={sf.slug} title={sf.name || sf.slug}>
            <div className="space-y-2">
              <p className="text-sm text-on-surface-variant">{sf.description || "No description"}</p>
              <p className="text-xs text-on-surface-variant opacity-60 font-mono">{sf.path}</p>
              <div className="flex gap-2 mt-2">
                <Button variant="secondary" size="sm" onClick={() => endpoints.getSkillFile(sf.slug).then(setEditingSkill).catch(() => {})}>Edit</Button>
                <Button variant="ghost" size="sm" className="hover:text-error ml-auto" onClick={async () => {
                  await endpoints.deleteSkillFile(sf.slug);
                  setSkillFiles((prev) => prev.filter((s) => s.slug !== sf.slug));
                }}>Delete</Button>
              </div>
            </div>
          </Card>
        ))}

        {/* Edit / Create modal */}
        {editingSkill !== null && (
          <Card title={editingSkill.slug ? `Editing: ${editingSkill.slug}` : "New Skill"}>
            <div className="space-y-3">
              {!editingSkill.slug && (
                <Input label="Slug" value={editingSkill.slug} onChange={(e) => setEditingSkill({ ...editingSkill, slug: e.target.value })} placeholder="my-skill" />
              )}
              <TextArea
                label="Content (SKILL.md)"
                value={editingSkill.content ?? ""}
                onChange={(e) => setEditingSkill({ ...editingSkill, content: e.target.value })}
                rows={12}
              />
              <div className="flex gap-2">
                <Button variant="primary" size="sm" onClick={async () => {
                  if (!editingSkill.slug) return;
                  if (skillFiles.find((s) => s.slug === editingSkill.slug)) {
                    await endpoints.updateSkillFile(editingSkill.slug, editingSkill.content ?? "");
                  } else {
                    await endpoints.createSkillFile({ name: editingSkill.slug, content: editingSkill.content ?? "" });
                  }
                  const updated = await endpoints.listSkillFiles();
                  setSkillFiles(updated);
                  setEditingSkill(null);
                }}>Save</Button>
                <Button variant="ghost" size="sm" onClick={() => setEditingSkill(null)}>Cancel</Button>
              </div>
            </div>
          </Card>
        )}
      </div>

      {/* ── Config-based Custom Skills ── */}
      <div className="space-y-4">
        <SectionHeader
          title="Custom Skills"
          description="Inline skill definitions saved in your config"
          action={
            <Button variant="secondary" size="sm" onClick={() => updateConfig((c) => { c.skills.entries.push({ ...emptySkill }); return c; })}>
              Add Skill
            </Button>
          }
        />
        {config.skills.entries.map((skill, i) => (
          <Card key={i} title={skill.name || `Skill ${i + 1}`}>
            <div className="space-y-4">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
                <Input label="Name" value={skill.name} onChange={(e) => updateConfig((c) => { c.skills.entries[i].name = e.target.value; return c; })} />
                <Input label="Description" value={skill.description} onChange={(e) => updateConfig((c) => { c.skills.entries[i].description = e.target.value; return c; })} />
              </div>
              <TextArea label="Content" value={skill.content} onChange={(e) => updateConfig((c) => { c.skills.entries[i].content = e.target.value; return c; })} rows={6} />
              <div className="flex items-center gap-4">
                <Toggle label="Enabled" checked={skill.enabled} onChange={(v) => updateConfig((c) => { c.skills.entries[i].enabled = v; return c; })} />
                <Button variant="ghost" size="sm" className="ml-auto hover:text-error" onClick={() => updateConfig((c) => { c.skills.entries.splice(i, 1); return c; })}>
                  Remove
                </Button>
              </div>
            </div>
          </Card>
        ))}
        {config.skills.entries.length === 0 && (
          <p className="text-on-surface-variant text-sm">No custom skills configured.</p>
        )}
        <SaveBar onClick={saveSkills} loading={saving} label="Save Skills" status={saveStatus} />
      </div>
    </div>
  );

  const renderProviders = () => (
    <div className="space-y-6">
      <SectionHeader
        title="LLM Providers"
        description="Configure AI model providers and fallback order"
        action={
          <Button variant="secondary" size="sm" onClick={() => updateConfig((c) => { c.llm.providers.push({ ...emptyProvider }); return c; })}>
            Add Provider
          </Button>
        }
      />
      {config.llm.providers.map((prov, i) => (
        <ProviderCard key={i} prov={prov} index={i} updateConfig={updateConfig} probeStatus={providerStatuses[prov.name] ?? null} />
      ))}
      {config.llm.providers.length === 0 && (
        <p className="text-on-surface-variant text-sm">No providers configured.</p>
      )}
      <SaveBar onClick={saveFullConfig} loading={saving} label="Save Providers" status={saveStatus} />
    </div>
  );

  const renderMCP = () => (
    <div className="space-y-6">
      <SectionHeader
        title="MCP Servers"
        description="Configure external MCP servers (config/UI only for now)"
        action={
          <Button
            variant="secondary"
            size="sm"
            onClick={() => updateConfig((c) => {
              c.mcp.servers.push({ ...emptyMCPServer });
              return c;
            })}
          >
            Add MCP Server
          </Button>
        }
      />

      {config.mcp.servers.map((server, i) => (
        <McpServerCard
          key={server.id ?? i}
          server={server}
          index={i}
          updateConfig={updateConfig}
        />
      ))}

      {config.mcp.servers.length === 0 && (
        <p className="text-on-surface-variant text-sm">No MCP servers configured.</p>
      )}

      <SaveBar onClick={saveFullConfig} loading={saving} label="Save MCP Config" status={saveStatus} />
    </div>
  );

  const renderSoul = () => (
    <div className="space-y-6">
      <SectionHeader title="Soul" description="System prompt and assistant behavior" />
      <Card title="System Prompt">
        <TextArea value={config.prompts.systemPrompt} onChange={(e) => updateConfig((c) => { c.prompts.systemPrompt = e.target.value; return c; })} rows={12} className="font-mono" />
      </Card>
      <SaveBar onClick={saveFullConfig} loading={saving} label="Save Soul Config" status={saveStatus} />
    </div>
  );

  const renderMemory = () => {
    const handleAddMemory = async () => {
      const entry = { ...emptyMemory };
      try {
        const created = await endpoints.createMemory(entry);
        setMemories((prev) => [...prev, created]);
      } catch {
        setError("Failed to create memory entry");
      }
    };

    const handleDeleteMemory = async (id: string | undefined, i: number) => {
      if (id) {
        try {
          await endpoints.deleteMemory(id);
        } catch {
          setError("Failed to delete memory");
          return;
        }
      }
      setMemories((prev) => prev.filter((_, idx) => idx !== i));
    };

    const refreshMemories = async () => {
      setMemoriesLoading(true);
      try {
        const res = await endpoints.listMemories();
        setMemories(res.memories ?? []);
      } catch {
        setError("Failed to load memories");
      } finally {
        setMemoriesLoading(false);
      }
    };

    return (
      <div className="space-y-3">
        <SectionHeader
          title="Memory"
          description="Persistent memory entries learned by the assistant"
          action={
            <div className="flex gap-2">
              <Button variant="ghost" size="sm" onClick={refreshMemories} loading={memoriesLoading}>
                Refresh
              </Button>
              <Button variant="secondary" size="sm" onClick={handleAddMemory}>
                Add Memory
              </Button>
            </div>
          }
        />
        {memories.map((mem, i) => (
          <MemoryRow
            key={mem.id ?? i}
            mem={mem}
            index={i}
            onUpdate={(updated) => setMemories((prev) => { const next = [...prev]; next[i] = updated; return next; })}
            onDelete={() => handleDeleteMemory(mem.id, i)}
          />
        ))}
        {memories.length === 0 && !memoriesLoading && (
          <p className="text-on-surface-variant text-sm">No memory entries yet. The assistant learns memories automatically via conversation.</p>
        )}
      </div>
    );
  };


  const renderSchedules = () => (
    <SchedulesTab
      tasks={tasks}
      tasksLoading={tasksLoading}
      refreshTasks={refreshTasks}
      setTasks={setTasks}
      setError={setError}
    />
  );

  const renderEmail = () => <EmailTab config={config} updateConfig={updateConfig} saving={saving} saveFullConfig={saveFullConfig} saveStatus={saveStatus} setError={setError} />;

  const renderHeartbeat = () => {
    const providerOptions = [
      { value: "", label: "-- same as chat --" },
      ...config.llm.providers
        .filter((p) => p.enabled && p.name)
        .map((p) => ({ value: p.model || p.name, label: `${p.name} . ${p.model || "default"}` })),
    ];

    return (
    <div className="space-y-6">
      <SectionHeader title="Heartbeat" description="Autonomous heartbeat configuration" />
      <Card>
        <div className="space-y-4">
          <Toggle label="Enabled" checked={config.heartbeat.enabled} onChange={(v) => updateConfig((c) => { c.heartbeat.enabled = v; return c; })} />
          <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
            <Input label="Interval (seconds)" type="number" value={config.heartbeat.intervalSeconds} onChange={(e) => updateConfig((c) => { c.heartbeat.intervalSeconds = parseInt(e.target.value) || 0; return c; })} />
            <Select
              label="Model"
              options={providerOptions}
              value={config.heartbeat.model}
              onChange={(e) => updateConfig((c) => { c.heartbeat.model = e.target.value; return c; })}
            />
            <Input label="Active Hours Start" type="time" value={config.heartbeat.activeHoursStart} onChange={(e) => updateConfig((c) => { c.heartbeat.activeHoursStart = e.target.value; return c; })} />
            <Input label="Active Hours End" type="time" value={config.heartbeat.activeHoursEnd} onChange={(e) => updateConfig((c) => { c.heartbeat.activeHoursEnd = e.target.value; return c; })} />
            <Input label="Timezone" value={config.heartbeat.timezone} onChange={(e) => updateConfig((c) => { c.heartbeat.timezone = e.target.value; return c; })} />
          </div>
        </div>
      </Card>
      <Card title="Heartbeat Prompt">
        <TextArea value={config.prompts.heartbeatPrompt} onChange={(e) => updateConfig((c) => { c.prompts.heartbeatPrompt = e.target.value; return c; })} rows={8} className="font-mono" />
      </Card>
      <SaveBar onClick={saveFullConfig} loading={saving} label="Save Heartbeat" status={saveStatus} />
    </div>
    );
  };


  const panels: Record<string, () => ReactElement> = {
    email: renderEmail,
    tools: renderTools,
    skills: renderSkills,
    mcp: renderMCP,
    providers: renderProviders,
    soul: renderSoul,
    memory: renderMemory,
    schedules: renderSchedules,
    heartbeat: renderHeartbeat,
  };

  // ─── Render ───

  return (
    <div className="space-y-6">
      {/* Error banner */}
      {error && (
        <div className="bg-error/10 text-error text-sm px-4 py-3 rounded-sm flex items-center justify-between">
          <span>{error}</span>
          <button onClick={() => setError(null)} className="text-error hover:text-on-surface ml-4">
            &times;
          </button>
        </div>
      )}

      {/* Tab bar - glassy */}
      <div className="rounded-xl border border-gray-300/40 dark:border-white/15 bg-surface-low/80 backdrop-blur-xl shadow-md p-1 overflow-x-auto flex gap-1 ring-1 ring-black/5 dark:ring-white/5">
        {TABS.map((tab) => (
          <button
            key={tab.key}
            onClick={() => setActiveTab(tab.key)}
            className={`px-4 py-2 text-sm font-body rounded-lg whitespace-nowrap transition-all duration-150 ${
              activeTab === tab.key
                ? "bg-violet text-white shadow-md shadow-violet/30"
                : "text-on-surface-variant hover:bg-white/10 hover:text-on-surface"
            }`}
          >
            {tab.label}
          </button>
        ))}
      </div>

      {/* Active panel */}
      <div>{panels[activeTab]()}</div>
    </div>
  );
}
