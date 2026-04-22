"use client";

import { useState, useEffect, useCallback } from "react";
import type { ReactElement } from "react";
import {
  endpoints,
  type UserConfig,
  type SkillFile,
  type MemoryEntry,
  type TaskDTO,
} from "@/lib/api";
import { Spinner } from "@/components/ui/Spinner";
import {
  TABS,
  type ProviderProbeStatus,
  EmailTab,
  SchedulesTab,
  ServersTab,
  DAVTab,
  TelegramTab,
  DevicesTab,
  ToolsTab,
  SkillsTab,
  ProvidersTab,
  McpTab,
  SoulTab,
  MemoryTab,
  HeartbeatTab,
} from "@/components/config";

export default function ConfigStudio({ requestedTab }: { requestedTab?: string }) {
  const [config, setConfig] = useState<UserConfig | null>(null);
  const [activeTab, setActiveTab] = useState(requestedTab ?? "email");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [saveStatus, setSaveStatus] = useState<string | null>(null);
  const [providerStatuses, setProviderStatuses] = useState<Record<string, ProviderProbeStatus>>({});
  const [initialMemories, setInitialMemories] = useState<MemoryEntry[]>([]);
  const [initialSkillFiles, setInitialSkillFiles] = useState<SkillFile[]>([]);
  const [tasks, setTasks] = useState<TaskDTO[]>([]);
  const [tasksLoading, setTasksLoading] = useState(false);

  useEffect(() => {
    if (requestedTab) setActiveTab(requestedTab);
  }, [requestedTab]);

  useEffect(() => {
    (async () => {
      try {
        const cfg = await endpoints.getConfig();
        setConfig(cfg);

        endpoints
          .getProvidersStatus()
          .then((res) => {
            const map: Record<string, ProviderProbeStatus> = {};
            for (const p of res.providers) {
              map[p.name] = { ok: p.ok, latencyMs: p.latencyMs, error: p.error };
            }
            setProviderStatuses(map);
          })
          .catch(() => {});

        endpoints
          .listMemories()
          .then((res) => setInitialMemories(res.memories ?? []))
          .catch(() => {});

        endpoints
          .listTasks()
          .then((res) => setTasks(res.tasks ?? []))
          .catch(() => {});

        endpoints
          .listSkillFiles()
          .then(setInitialSkillFiles)
          .catch(() => {});
      } catch (e) {
        setError(e instanceof Error ? e.message : "Failed to load config");
      }
    })();
  }, []);

  const updateConfig = useCallback((updater: (draft: UserConfig) => UserConfig) => {
    setConfig((prev) => (prev ? updater(structuredClone(prev)) : prev));
  }, []);

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

  // Applies an updater, saves the result immediately, and updates state --
  // avoids the stale-closure problem when save must follow a config mutation.
  const saveWithUpdate = useCallback(async (updater: (draft: UserConfig) => UserConfig) => {
    setConfig((prev) => {
      if (!prev) return prev;
      const next = updater(structuredClone(prev));
      // Fire-and-forget inside the setter -- we just need `next` synchronously
      setSaving(true);
      setError(null);
      endpoints
        .putConfig(next)
        .then(() => {
          setSaveStatus("Configuration saved");
          setTimeout(() => setSaveStatus(null), 3000);
        })
        .catch((e: unknown) => {
          setError(e instanceof Error ? e.message : "Save failed");
        })
        .finally(() => setSaving(false));
      return next;
    });
  }, []);

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

  const commonSave = { saving, onSave: saveFullConfig, saveStatus };

  const panels: Record<string, () => ReactElement> = {
    email: () => (
      <EmailTab
        config={config}
        updateConfig={updateConfig}
        saving={saving}
        saveFullConfig={saveFullConfig}
        saveStatus={saveStatus}
        setError={setError}
      />
    ),
    servers: () => (
      <ServersTab
        config={config}
        updateConfig={updateConfig}
        saving={saving}
        saveFullConfig={saveFullConfig}
        saveStatus={saveStatus}
      />
    ),
    dav: () => (
      <DAVTab
        config={config}
        updateConfig={updateConfig}
        saving={saving}
        saveFullConfig={saveFullConfig}
        saveStatus={saveStatus}
        isActive={activeTab === "dav"}
      />
    ),
    channels: () => (
      <TelegramTab
        config={config}
        updateConfig={updateConfig}
        saving={saving}
        saveFullConfig={saveFullConfig}
        saveStatus={saveStatus}
      />
    ),
    devices: () => (
      <DevicesTab
        config={config}
        updateConfig={updateConfig}
        saving={saving}
        saveFullConfig={saveFullConfig}
        saveWithUpdate={saveWithUpdate}
        saveStatus={saveStatus}
      />
    ),
    tools: () => (
      <ToolsTab
        config={config}
        updateConfig={updateConfig}
        saving={saving}
        onSave={saveTools}
        saveStatus={saveStatus}
      />
    ),
    skills: () => (
      <SkillsTab
        initialSkillFiles={initialSkillFiles}
        onSuccess={flashSave}
        onError={(msg) => setError(msg)}
      />
    ),
    providers: () => (
      <ProvidersTab
        config={config}
        updateConfig={updateConfig}
        providerStatuses={providerStatuses}
        {...commonSave}
      />
    ),
    mcp: () => <McpTab config={config} updateConfig={updateConfig} {...commonSave} />,
    soul: () => <SoulTab config={config} updateConfig={updateConfig} {...commonSave} />,
    memory: () => <MemoryTab initialMemories={initialMemories} onError={(msg) => setError(msg)} />,
    schedules: () => (
      <SchedulesTab
        tasks={tasks}
        tasksLoading={tasksLoading}
        refreshTasks={refreshTasks}
        setTasks={setTasks}
        setError={setError}
      />
    ),
    heartbeat: () => <HeartbeatTab config={config} updateConfig={updateConfig} {...commonSave} />,
  };

  return (
    <div className="space-y-6">
      {error && (
        <div className="bg-error/10 text-error text-sm px-4 py-3 rounded-sm flex items-center justify-between">
          <span>{error}</span>
          <button onClick={() => setError(null)} className="text-error hover:text-on-surface ml-4">
            &times;
          </button>
        </div>
      )}

      <div className="flex justify-center">
        <div className="inline-flex max-w-full rounded-xl border border-gray-300/40 dark:border-white/15 bg-surface-low/80 backdrop-blur-xl shadow-md p-1 gap-1 ring-1 ring-black/5 dark:ring-white/5 overflow-x-auto">
          {TABS.map((tab) => (
            <button
              key={tab.key}
              onClick={() => setActiveTab(tab.key)}
              className={`px-4 py-2 text-base font-body rounded-lg whitespace-nowrap transition-all duration-150 hover:cursor-pointer ${
                activeTab === tab.key
                  ? "bg-violet text-white shadow-md shadow-violet/30"
                  : "text-on-surface-variant hover:bg-white/10 hover:text-on-surface"
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>
      </div>

      <div
        key={activeTab}
        className="max-w-4xl mx-auto w-full animate-in fade-in slide-in-from-bottom-2 duration-200"
      >
        {panels[activeTab]?.()}
      </div>
    </div>
  );
}
