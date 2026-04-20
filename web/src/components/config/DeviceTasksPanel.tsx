"use client";

import { useState, useEffect } from "react";
import type { CompanionAppConfig, DeviceTaskDTO } from "@/lib/api";
import { endpoints } from "@/lib/api";
import { Input } from "@/components/ui/Input";
import { Button } from "@/components/ui/Button";
import { TrashIcon } from "@/components/ui/icons";

export function DeviceTasksPanel({ companionApps }: { companionApps: CompanionAppConfig[] }) {
  const [tasks, setTasks] = useState<DeviceTaskDTO[]>([]);
  const [loading, setLoading] = useState(false);
  const [target, setTarget] = useState("");
  const [instruction, setInstruction] = useState("");
  const [adding, setAdding] = useState(false);

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

  useEffect(() => {
    void fetchTasks();
  }, []);

  const handleAdd = async () => {
    if (!target || !instruction) return;
    setAdding(true);
    try {
      await endpoints.createDeviceTask({ targetDevice: target, instruction });
      setInstruction("");
      await fetchTasks();
    } catch (e) {
      console.error(e);
    } finally {
      setAdding(false);
    }
  };

  const handleDelete = async (id: string) => {
    try {
      await endpoints.deleteDeviceTask(id);
      await fetchTasks();
    } catch (e) {
      console.error(e);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex gap-2 items-end bg-surface-mid p-3 rounded-lg border border-white/5">
        <div className="flex-1 space-y-2">
          <label className="block text-xs text-on-surface-variant mb-1">Target Device</label>
          {companionApps.length > 0 ? (
            <select
              value={target}
              onChange={(e) => setTarget(e.target.value)}
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
              value={target}
              onChange={(e) => setTarget(e.target.value)}
              placeholder="Device ID"
            />
          )}
        </div>
        <div className="flex-[2] space-y-2">
          <Input
            label="Instruction"
            value={instruction}
            onChange={(e) => setInstruction(e.target.value)}
            placeholder="e.g. Set an alarm for 7 AM"
          />
        </div>
        <Button
          variant="primary"
          onClick={handleAdd}
          loading={adding}
          disabled={!target || !instruction}
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
            const deviceLabel = device ? device.label || device.name : task.targetDevice;
            return (
              <div
                key={task.id}
                className="flex flex-col gap-2 p-3 rounded-lg bg-surface-mid border border-white/5 text-sm"
              >
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
                    onClick={() => handleDelete(task.id)}
                    className="cursor-pointer text-on-surface-variant hover:text-error transition-colors"
                  >
                    <TrashIcon />
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

      <div className="flex justify-end">
        <Button variant="ghost" size="sm" onClick={fetchTasks} loading={loading}>
          Refresh
        </Button>
      </div>
    </div>
  );
}
