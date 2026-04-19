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
  type ProviderModelsProbeResult,
  type TelegramBotConfig,
  type SSHServerConfig,
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

// ─── Extracted config sub-components ───
import {
  TABS,
  PROVIDER_KINDS,
  emptyEmailAccount,
  emptyGolangTool,
  emptyProvider,
  emptySkill,
  emptyMemory,
  emptySchedule,
  emptyMCPServer,
  isOpenAICompatibleProviderKind,
  type UpdateConfigFn,
  type ProviderProbeStatus,
} from "@/components/config";
import { SaveBar } from "@/components/config/SaveBar";
import { MemoryRow } from "@/components/config/MemoryRow";
import { ProviderCard } from "@/components/config/ProviderCard";
import { McpServerCard } from "@/components/config/McpServerCard";
import { EmailTab } from "@/components/config/EmailTab";
import { SchedulesTab } from "@/components/config/SchedulesTab";
import { ServersTab } from "@/components/config/ServersTab";
import { TelegramTab } from "@/components/config/TelegramTab";
import { DevicesTab } from "@/components/config/DevicesTab";

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
  const renderServers = () => <ServersTab config={config} updateConfig={updateConfig} saving={saving} saveFullConfig={saveFullConfig} saveStatus={saveStatus} />;
  const renderTelegram = () => <TelegramTab config={config} updateConfig={updateConfig} saving={saving} saveFullConfig={saveFullConfig} saveStatus={saveStatus} />;
  const renderDevices = () => <DevicesTab config={config} updateConfig={updateConfig} saving={saving} saveFullConfig={saveFullConfig} saveStatus={saveStatus} />;

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
    servers: renderServers,
    channels: renderTelegram,
    devices: renderDevices,
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
      <div className="max-w-6xl mx-auto w-full rounded-xl border border-gray-300/40 dark:border-white/15 bg-surface-low/80 backdrop-blur-xl shadow-md p-1 overflow-x-auto flex gap-1 ring-1 ring-black/5 dark:ring-white/5">
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
      <div className="max-w-3xl mx-auto w-full">{panels[activeTab]()}</div>
    </div>
  );
}
