"use client";

import { useState, useEffect } from "react";
import QRCode from "react-qr-code";
import type { UserConfig, CompanionAppConfig, DeviceTaskDTO, DeviceRegistration } from "@/lib/api";
import { endpoints } from "@/lib/api";
import { Input } from "@/components/ui/Input";
import { Toggle } from "@/components/ui/Toggle";
import { Button } from "@/components/ui/Button";
import { SectionHeader } from "@/components/ui/SectionHeader";
import { SaveBar } from "./SaveBar";
import type { UpdateConfigFn } from "./types";

function QRModal({ payload, onClose }: { payload: string; onClose: () => void }) {
  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
      <div className="bg-surface-high border border-white/10 rounded-2xl p-6 w-full max-w-sm shadow-2xl flex flex-col gap-6">
        <div className="flex justify-between items-center">
          <h3 className="text-lg font-medium text-on-surface">Pair Companion App</h3>
          <button onClick={onClose} className="text-on-surface-variant hover:text-on-surface p-1">
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
              <path d="M18 6L6 18M6 6l12 12" />
            </svg>
          </button>
        </div>
        <div className="flex flex-col items-center gap-6">
          <div className="bg-white p-4 rounded-xl">
            <QRCode value={payload} size={200} />
          </div>
          <p className="text-sm text-center text-on-surface-variant">
            Scan this QR code from the Companion App to securely pair it to your openCrow server.
          </p>
          <Button variant="secondary" className="w-full" onClick={onClose}>Done</Button>
        </div>
      </div>
    </div>
  );
}

function CompanionAppCard({
  app,
  index: i,
  registration,
  updateConfig,
}: {
  app: CompanionAppConfig;
  index: number;
  registration?: DeviceRegistration;
  updateConfig: UpdateConfigFn;
}) {
  const [generating, setGenerating] = useState(false);
  const [qrPayload, setQrPayload] = useState<string | null>(null);

  const isOnline = registration
    ? Date.now() - new Date(registration.lastSeenAt).getTime() < 10 * 60 * 1000
    : false;

  const handleRepair = async () => {
    setGenerating(true);
    try {
      const res = await endpoints.createDeviceTokens(app.label || app.name);
      const payload = JSON.stringify({
        id: app.id,
        server: window.location.origin,
        accessToken: res.tokens.accessToken,
        refreshToken: res.tokens.refreshToken,
      });
      setQrPayload(payload);
    } catch (e) {
      console.error(e);
    } finally {
      setGenerating(false);
    }
  };

  return (
    <div className="rounded-lg border border-white/10 bg-surface-mid overflow-hidden p-4 space-y-4">
      <div className="grid grid-cols-2 gap-3">
        <Input
          label="Display Name"
          value={app.label ?? ""}
          onChange={(e) => updateConfig((c) => { c.integrations.companionApps[i].label = e.target.value; return c; })}
          placeholder="e.g. Pixel 8 Pro"
        />
        <Input
          label="Identifier"
          value={app.name}
          onChange={(e) => updateConfig((c) => { c.integrations.companionApps[i].name = e.target.value; return c; })}
          placeholder="e.g. pixel8"
        />
      </div>
      {registration && registration.capabilities.length > 0 && (
        <div className="flex flex-wrap gap-1.5 pt-1">
          {registration.capabilities.map((cap) => (
            <span
              key={cap.name}
              title={cap.description}
              className="text-[10px] font-mono px-2 py-0.5 rounded-full bg-primary/10 text-primary border border-primary/20"
            >
              {cap.name}
            </span>
          ))}
        </div>
      )}

      <div className="flex gap-2 pt-2 items-center flex-wrap">
        <Toggle
          label="Enabled"
          checked={app.enabled}
          onChange={(v) => updateConfig((c) => { c.integrations.companionApps[i].enabled = v; return c; })}
        />
        <span className={`text-[10px] px-1.5 py-0.5 rounded-full font-medium ${isOnline ? "bg-green-500/15 text-green-400" : "bg-white/5 text-on-surface-variant"}`}>
          {isOnline ? "online" : "offline"}
        </span>
        <span className="text-xs text-on-surface-variant font-mono px-2 py-1 bg-white/5 rounded">
          ID: {app.id || "legacy"}
        </span>
        <Button variant="ghost" size="sm" onClick={handleRepair} loading={generating}>
          Re-Pair
        </Button>
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto hover:text-error"
          onClick={() => updateConfig((c) => { c.integrations.companionApps.splice(i, 1); return c; })}
        >
          Remove
        </Button>
      </div>
      {qrPayload && <QRModal payload={qrPayload} onClose={() => setQrPayload(null)} />}
    </div>
  );
}

export function DevicesTab({
  config,
  updateConfig,
  saving,
  saveFullConfig,
  saveStatus,
}: {
  config: UserConfig;
  updateConfig: UpdateConfigFn;
  saving: boolean;
  saveFullConfig: () => void;
  saveStatus: string | null;
}) {
  const [tasks, setTasks] = useState<DeviceTaskDTO[]>([]);
  const [loading, setLoading] = useState(false);
  const [newTaskTarget, setNewTaskTarget] = useState("");
  const [newTaskInstruction, setNewTaskInstruction] = useState("");
  const [addingTask, setAddingTask] = useState(false);
  const [registrations, setRegistrations] = useState<Record<string, DeviceRegistration>>({});

  const [isAddingDevice, setIsAddingDevice] = useState(false);
  const [newDeviceName, setNewDeviceName] = useState("");
  const [newDeviceLabel, setNewDeviceLabel] = useState("");
  const [generatingQR, setGeneratingQR] = useState(false);
  const [qrPayload, setQrPayload] = useState<string | null>(null);

  const companionApps = config.integrations.companionApps || [];

  useEffect(() => {
    fetchTasks();
    fetchRegistrations();
  }, []);

  const fetchRegistrations = async () => {
    try {
      const res = await endpoints.listDeviceRegistrations();
      const map: Record<string, DeviceRegistration> = {};
      for (const r of res.registrations ?? []) map[r.deviceId] = r;
      setRegistrations(map);
    } catch {
      // non-fatal
    }
  };

  const fetchTasks = async () => {
    setLoading(true);
    try {
      const res = await endpoints.listDeviceTasks();
      setTasks(res.tasks || []);
    } catch (e) {
      console.error(e);
    } finally {
      setLoading(false);
    }
  };

  const handleAddTask = async () => {
    if (!newTaskTarget || !newTaskInstruction) return;
    setAddingTask(true);
    try {
      await endpoints.createDeviceTask({ targetDevice: newTaskTarget, instruction: newTaskInstruction });
      setNewTaskInstruction("");
      await fetchTasks();
    } catch (e) {
      console.error(e);
    } finally {
      setAddingTask(false);
    }
  };

  const handleDeleteTask = async (id: string) => {
    try {
      await endpoints.deleteDeviceTask(id);
      await fetchTasks();
    } catch (e) {
      console.error(e);
    }
  };

  const startAddDevice = () => {
    setIsAddingDevice(true);
    setNewDeviceName("");
    setNewDeviceLabel("");
    setQrPayload(null);
  };

  const generateDeviceTokens = async () => {
    const name = newDeviceName.trim();
    const label = newDeviceLabel.trim() || name;
    if (!name) return;
    setGeneratingQR(true);
    try {
      const res = await endpoints.createDeviceTokens(label);
      const deviceId = "dev_" + Math.random().toString(36).substring(2, 9);
      const payload = JSON.stringify({
        id: deviceId,
        server: window.location.origin,
        accessToken: res.tokens.accessToken,
        refreshToken: res.tokens.refreshToken,
      });
      setQrPayload(payload);
      updateConfig((c) => {
        if (!c.integrations.companionApps) c.integrations.companionApps = [];
        c.integrations.companionApps.push({ id: deviceId, name, label, enabled: true });
        return c;
      });
      saveFullConfig();
    } catch (e) {
      console.error(e);
    } finally {
      setGeneratingQR(false);
    }
  };

  const closeAddDevice = () => {
    setIsAddingDevice(false);
    setQrPayload(null);
  };

  return (
    <div className="space-y-8 relative">
      <div className="space-y-6">
        <SectionHeader
          title="Companion Apps / Devices"
          description="Register companion apps that can poll for remote tasks"
          action={
            <Button variant="secondary" size="sm" onClick={startAddDevice}>
              Add Device
            </Button>
          }
        />
        {companionApps.map((app, i) => (
          <CompanionAppCard key={app.id || i} app={app} index={i} registration={app.id ? registrations[app.id] : undefined} updateConfig={updateConfig} />
        ))}
        {!companionApps.length && (
          <p className="text-on-surface-variant text-sm">No companion apps configured.</p>
        )}
        <SaveBar onClick={saveFullConfig} loading={saving} label="Save Device Config" status={saveStatus} />
      </div>

      <div className="h-px bg-white/10" />

      <div className="space-y-6">
        <SectionHeader
          title="Pending Device Tasks"
          description="Tasks waiting to be polled by the companion apps"
          action={
            <Button variant="ghost" size="sm" onClick={fetchTasks} loading={loading}>
              Refresh
            </Button>
          }
        />

        <div className="flex gap-2 items-end bg-surface-mid p-3 rounded-lg border border-white/5">
          <div className="flex-1 space-y-2">
            <label className="block text-xs text-on-surface-variant mb-1">Target Device</label>
            {companionApps.length > 0 ? (
              <select
                value={newTaskTarget}
                onChange={(e) => setNewTaskTarget(e.target.value)}
                className="w-full rounded-md bg-surface-low border border-white/10 text-on-surface text-sm px-3 py-2 focus:outline-none focus:border-primary"
              >
                <option value="">Select device…</option>
                {companionApps.map((app) => (
                  <option key={app.id} value={app.id ?? app.name}>
                    {app.label || app.name} ({app.id || app.name})
                  </option>
                ))}
              </select>
            ) : (
              <Input
                value={newTaskTarget}
                onChange={(e) => setNewTaskTarget(e.target.value)}
                placeholder="Device ID"
              />
            )}
          </div>
          <div className="flex-[2] space-y-2">
            <Input
              label="Instruction"
              value={newTaskInstruction}
              onChange={(e) => setNewTaskInstruction(e.target.value)}
              placeholder="e.g. Set an alarm for 7 AM"
            />
          </div>
          <Button
            variant="primary"
            onClick={handleAddTask}
            loading={addingTask}
            disabled={!newTaskTarget || !newTaskInstruction}
          >
            Queue Task
          </Button>
        </div>

        <div className="space-y-2">
          {tasks.length === 0 ? (
            <p className="text-sm text-on-surface-variant p-4 text-center border border-dashed border-white/10 rounded-lg">
              No pending tasks
            </p>
          ) : (
            tasks.map((task) => {
              const device = companionApps.find((a) => (a.id ?? a.name) === task.targetDevice);
              const deviceLabel = device ? (device.label || device.name) : task.targetDevice;
              return (
                <div key={task.id} className="flex flex-col gap-2 p-3 rounded-lg bg-surface-mid border border-white/5 text-sm">
                  <div className="flex justify-between items-start">
                    <div className="flex gap-2 items-center">
                      <span className="font-mono text-cyan bg-cyan/10 px-1.5 py-0.5 rounded text-xs">
                        {deviceLabel}
                      </span>
                      <span
                        className={`text-xs px-1.5 py-0.5 rounded ${
                          task.status === "pending"
                            ? "bg-warning/10 text-warning"
                            : task.status === "processing"
                            ? "bg-violet/10 text-violet"
                            : "bg-cyan/10 text-cyan"
                        }`}
                      >
                        {task.status}
                      </span>
                    </div>
                    <button
                      onClick={() => handleDeleteTask(task.id)}
                      className="text-on-surface-variant hover:text-error transition-colors"
                    >
                      <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth="1.5">
                        <path d="M2 4h12M5 4V2h6v2M6 7v5M10 7v5M3 4l1 10h8l1-10" />
                      </svg>
                    </button>
                  </div>
                  <p className="text-on-surface">{task.instruction}</p>
                  <div className="text-[10px] text-on-surface-variant font-mono mt-1">
                    ID: {task.id} • Created: {new Date(task.createdAt).toLocaleString()}
                  </div>
                </div>
              );
            })
          )}
        </div>
      </div>

      {isAddingDevice && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 backdrop-blur-sm p-4">
          <div className="bg-surface-high border border-white/10 rounded-2xl p-6 w-full max-w-sm shadow-2xl flex flex-col gap-6">
            <div className="flex justify-between items-center">
              <h3 className="text-lg font-medium text-on-surface">Pair Companion App</h3>
              <button onClick={closeAddDevice} className="text-on-surface-variant hover:text-on-surface p-1">
                <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round">
                  <path d="M18 6L6 18M6 6l12 12" />
                </svg>
              </button>
            </div>

            {!qrPayload ? (
              <div className="space-y-4">
                <p className="text-sm text-on-surface-variant">
                  Give this device a name and label to generate a pairing QR code.
                </p>
                <Input
                  label="Display Name"
                  value={newDeviceLabel}
                  onChange={(e) => setNewDeviceLabel(e.target.value)}
                  placeholder="e.g. Pixel 8 Pro"
                  autoFocus
                />
                <Input
                  label="Identifier"
                  value={newDeviceName}
                  onChange={(e) => setNewDeviceName(e.target.value)}
                  placeholder="e.g. pixel8"
                />
                <Button
                  variant="primary"
                  className="w-full"
                  onClick={generateDeviceTokens}
                  loading={generatingQR}
                  disabled={!newDeviceName.trim()}
                >
                  Generate QR Code
                </Button>
              </div>
            ) : (
              <div className="flex flex-col items-center gap-6">
                <div className="bg-white p-4 rounded-xl">
                  <QRCode value={qrPayload} size={200} />
                </div>
                <p className="text-sm text-center text-on-surface-variant">
                  Scan this QR code from the Companion App to securely pair it to your openCrow server.
                </p>
                <Button variant="secondary" className="w-full" onClick={closeAddDevice}>
                  Done
                </Button>
              </div>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
