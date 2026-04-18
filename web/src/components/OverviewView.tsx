"use client";

import { useState, useEffect, useRef } from "react";
import { endpoints, type WorkerStat, type WorkerLogEntry, type HealthResponse } from "@/lib/api";
import { Card } from "@/components/ui/Card";
import { HeartbeatDot } from "@/components/ui/HeartbeatDot";
import { AnimatedDot } from "@/components/ui/AnimatedDot";

// ─── Worker Terminal Panel ───

function WorkerTerminal({ workerKey, label }: { workerKey: string; label: string }) {
  const [entries, setEntries] = useState<WorkerLogEntry[]>([]);
  const [stat, setStat] = useState<WorkerStat | null>(null);
  const bottomRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    let active = true;

    const fetchLogs = () => {
      endpoints
        .getWorkerLogs(workerKey)
        .then((res) => {
          if (active) setEntries(res.entries ?? []);
        })
        .catch(() => {});
    };

    const fetchStats = () => {
      endpoints
        .getWorkerStatus()
        .then((res) => {
          if (active) {
            const found = res.workers?.find((w) => w.name === workerKey) ?? null;
            setStat(found);
          }
        })
        .catch(() => {});
    };

    fetchLogs();
    fetchStats();
    const id = setInterval(() => {
      fetchLogs();
      fetchStats();
    }, 3000);

    return () => {
      active = false;
      clearInterval(id);
    };
  }, [workerKey]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [entries]);

  const dotStatus = !stat || stat.ticks === 0 ? "idle" : stat.lastError ? "warn" : "ok";

  return (
    <div className="flex flex-col rounded-lg overflow-hidden border border-[#2a2a3e]" style={{ height: "260px" }}>
      {/* Title bar */}
      <div className="shrink-0 flex items-center gap-2 bg-[#1a1a2e] px-4 py-2 border-b border-[#2a2a3e]">
        <div className="flex gap-1.5 mr-1">
          <span className="block h-3 w-3 rounded-full bg-[#ff5f57]" />
          <span className="block h-3 w-3 rounded-full bg-[#ffbd2e]" />
          <span className="block h-3 w-3 rounded-full bg-[#28c840]" />
        </div>
        <AnimatedDot status={dotStatus as "ok" | "warn" | "idle"} />
        <span className="text-xs font-mono text-[#8888aa]">{label}</span>
        {stat && stat.ticks > 0 && (
          <span className="ml-auto text-xs text-[#8888aa] font-mono">
            {stat.ticks} tick{stat.ticks !== 1 ? "s" : ""}
          </span>
        )}
      </div>
      {/* Log body */}
      <div className="flex-1 overflow-y-auto bg-[#0d0d1a] px-4 py-3 font-mono text-xs leading-relaxed">
        {entries.length === 0 ? (
          <p className="text-[#6272a4] italic">No logs yet...</p>
        ) : (
          entries.map((e, i) => {
            const isError =
              e.line.toLowerCase().includes("error") || e.line.toLowerCase().includes("failed");
            const ts = new Date(e.ts).toLocaleTimeString();
            return (
              <div key={i} className="flex gap-2 leading-5">
                <span className="text-[#6272a4] shrink-0 select-none">{ts}</span>
                <span className={isError ? "text-yellow-400" : "text-[#f8f8f2]/80"}>{e.line}</span>
              </div>
            );
          })
        )}
        <div ref={bottomRef} />
      </div>
    </div>
  );
}

// ─── Overview View ───

const WORKERS = [
  { key: "task-worker", label: "Task Worker" },
  { key: "heartbeat-worker", label: "Heartbeat Worker" },
  { key: "email-worker", label: "Email Worker" },
];

export default function OverviewView() {
  const [health, setHealth] = useState<HealthResponse | null>(null);

  useEffect(() => {
    endpoints
      .health()
      .then(setHealth)
      .catch(() => setHealth(null));
  }, []);

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="font-display text-2xl text-on-surface">Overview</h2>
          <p className="mt-1 text-sm text-on-surface-variant">
            Live status of server connection and background workers
          </p>
        </div>
        <HeartbeatDot label={health ? "CORE ONLINE" : "CORE OFFLINE"} online={!!health} />
      </div>

      {/* Backend Connection */}
      <Card title="Backend Connection">
        <div className="flex items-center gap-3">
          <AnimatedDot status={health ? "ok" : "error"} />
          {health ? (
            <span className="text-sm text-on-surface">
              Connected &mdash;{" "}
              <span className="font-mono text-cyan">{health.name}</span>
              {health.environment && (
                <span className="text-on-surface-variant ml-2">({health.environment})</span>
              )}
            </span>
          ) : (
            <span className="text-sm text-on-surface-variant">Disconnected or unreachable</span>
          )}
        </div>
      </Card>

      {/* Worker Terminals */}
      <div>
        <p className="text-xs font-mono uppercase tracking-wider text-on-surface-variant mb-3">
          Worker Logs
        </p>
        <div className="grid grid-cols-1 gap-4">
          {WORKERS.map(({ key, label }) => (
            <WorkerTerminal key={key} workerKey={key} label={label} />
          ))}
        </div>
      </div>
    </div>
  );
}
