"use client";

import { useState, useEffect, useRef } from "react";
import {
  endpoints,
  type WorkerStat,
  type WorkerLogEntry,
  type HealthResponse,
  type TelegramBotConfig,
} from "@/lib/api";
import { Spinner } from "@/components/ui/Spinner";
import { HeartbeatDot } from "@/components/ui/HeartbeatDot";
import { AnimatedDot } from "@/components/ui/AnimatedDot";

// Always-dark terminal colors -- intentional exception to theme tokens
// (terminals are a specialized UI component that must stay dark regardless of system theme)
const TERMINAL_BG = "#0d0d1a";
const TERMINAL_HEADER_BG = "#1a1a2e";
const TERMINAL_BORDER = "#2a2a3e";
const TERMINAL_TEXT_DIM = "rgba(158,168,195,0.55)";

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
          if (active) setStat(res.workers?.find((w) => w.name === workerKey) ?? null);
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
    <div
      className="flex flex-col rounded-sm overflow-hidden"
      style={{ height: "260px", border: `1px solid ${TERMINAL_BORDER}` }}
    >
      {/* Title bar */}
      <div
        className="shrink-0 flex items-center gap-2 px-4 py-2"
        style={{ background: TERMINAL_HEADER_BG, borderBottom: `1px solid ${TERMINAL_BORDER}` }}
      >
        <div className="flex gap-1.5 mr-1">
          <span className="block h-3 w-3 rounded-full bg-error" />
          <span className="block h-3 w-3 rounded-full bg-warning" />
          <span className="block h-3 w-3 rounded-full bg-success" />
        </div>
        <AnimatedDot status={dotStatus as "ok" | "warn" | "idle"} />
        <span className="text-xs font-mono" style={{ color: TERMINAL_TEXT_DIM }}>
          {label}
        </span>
        {stat && stat.ticks > 0 && (
          <span className="ml-auto text-xs font-mono" style={{ color: TERMINAL_TEXT_DIM }}>
            {stat.ticks} tick{stat.ticks !== 1 ? "s" : ""}
          </span>
        )}
      </div>
      {/* Log body */}
      <div
        className="flex-1 overflow-y-auto px-4 py-3 font-mono text-xs leading-relaxed"
        style={{ background: TERMINAL_BG }}
      >
        {entries.length === 0 ? (
          <p className="italic" style={{ color: TERMINAL_TEXT_DIM }}>
            No logs yet...
          </p>
        ) : (
          entries.map((e, i) => {
            const isError =
              e.line.toLowerCase().includes("error") || e.line.toLowerCase().includes("failed");
            const ts = new Date(e.ts).toLocaleTimeString();
            return (
              <div key={i} className="flex gap-2 leading-5">
                <span className="shrink-0 select-none" style={{ color: TERMINAL_TEXT_DIM }}>
                  {ts}
                </span>
                <span style={{ color: isError ? "#f59e0b" : "rgba(236,242,255,0.75)" }}>
                  {e.line}
                </span>
              </div>
            );
          })
        )}
        <div ref={bottomRef} />
      </div>
    </div>
  );
}

// ─── Glassy status card — reused for all three indicators ───

function StatusCard({
  title,
  status,
  children,
}: {
  title: string;
  status: "ok" | "error" | "idle";
  children: React.ReactNode;
}) {
  const borderColor =
    status === "ok"
      ? "border-success/30"
      : status === "error"
        ? "border-error/30"
        : "border-white/10";
  const gradientFrom =
    status === "ok" ? "from-success/10" : status === "error" ? "from-error/10" : "from-white/5";
  return (
    <div
      className={`relative rounded-2xl border ${borderColor} bg-surface-low/70 backdrop-blur-xl overflow-hidden shadow-[0_8px_32px_rgba(0,0,0,0.25)]`}
    >
      {/* Gradient fill */}
      <div
        className={`absolute inset-0 bg-gradient-to-br ${gradientFrom} via-transparent to-transparent pointer-events-none`}
      />
      <div className="relative p-5">
        <p className="text-xs font-mono uppercase tracking-wider text-on-surface-variant mb-3">
          {title}
        </p>
        {children}
      </div>
    </div>
  );
}

// ─── Whisper Status Card ───

function WhisperStatusCard() {
  const [status, setStatus] = useState<"ok" | "downloading" | "down" | "loading">("loading");
  const [model, setModel] = useState<string>("");

  useEffect(() => {
    endpoints
      .getVoiceStatus()
      .then((res) => {
        setStatus(res.status);
        setModel(res.model);
      })
      .catch(() => setStatus("down"));
  }, []);

  const cardStatus: "ok" | "error" | "idle" =
    status === "ok" ? "ok" : status === "down" ? "error" : "idle";

  const dotStatus =
    status === "loading" || status === "downloading"
      ? "pending"
      : status === "ok"
        ? "ok"
        : status === "down"
          ? "error"
          : "idle";

  return (
    <StatusCard title="Whisper (Voice)" status={cardStatus}>
      <div className="flex items-center gap-3">
        {status === "loading" ? (
          <Spinner size="sm" />
        ) : (
          <AnimatedDot status={dotStatus as "ok" | "error" | "pending" | "idle"} />
        )}
        <span className="text-sm text-on-surface">
          {status === "loading" && <span className="text-on-surface-variant">Checking...</span>}
          {status === "ok" && (
            <>
              <span className="font-mono text-cyan">{model}</span>
              <span className="text-on-surface-variant ml-2">ready</span>
            </>
          )}
          {status === "downloading" && (
            <>
              <span className="font-mono text-warning">{model}</span>
              <span className="text-on-surface-variant ml-2">downloading model...</span>
            </>
          )}
          {status === "down" && (
            <span className="text-on-surface-variant">Whisper sidecar unavailable</span>
          )}
        </span>
      </div>
    </StatusCard>
  );
}

// ─── Default Channel Status ───

function DefaultChannelCard() {
  const [bot, setBot] = useState<TelegramBotConfig | null | undefined>(undefined);
  const [testResult, setTestResult] = useState<{
    ok: boolean;
    latencyMs?: number;
    detail?: string;
    error?: string;
  } | null>(null);
  const [testing, setTesting] = useState(false);

  useEffect(() => {
    endpoints
      .getConfig()
      .then((cfg) => {
        const firstEnabled = cfg.integrations.telegramBots.find((b) => b.enabled && b.botToken);
        setBot(firstEnabled ?? null);
        if (firstEnabled) {
          setTesting(true);
          endpoints
            .testTelegramBot({ botToken: firstEnabled.botToken })
            .then((res) => setTestResult(res))
            .catch(() => setTestResult({ ok: false, error: "Request failed" }))
            .finally(() => setTesting(false));
        }
      })
      .catch(() => setBot(null));
  }, []);

  const cardStatus: "ok" | "error" | "idle" =
    testing || bot === undefined
      ? "idle"
      : !bot
        ? "idle"
        : testResult
          ? testResult.ok
            ? "ok"
            : "error"
          : "idle";

  const dotStatus = testing ? "pending" : cardStatus;

  return (
    <StatusCard title="Default Channel" status={cardStatus}>
      <div className="flex items-center gap-3">
        <AnimatedDot status={dotStatus} />
        {bot === undefined && <span className="text-sm text-on-surface-variant">Loading...</span>}
        {bot === null && (
          <span className="text-sm text-on-surface-variant">No Telegram channel configured</span>
        )}
        {bot && testing && (
          <span className="text-sm text-on-surface-variant font-mono">
            Checking {bot.label || "bot"}...
          </span>
        )}
        {bot && !testing && testResult && (
          <span className="text-sm text-on-surface">
            {testResult.ok ? (
              <>
                <span className="font-mono text-cyan">{bot.label || "Telegram"}</span>
                {testResult.detail && (
                  <span className="text-on-surface-variant ml-2">{testResult.detail}</span>
                )}
                {testResult.latencyMs != null && (
                  <span className="text-on-surface-variant ml-2 font-mono text-xs">
                    {testResult.latencyMs}ms
                  </span>
                )}
              </>
            ) : (
              <span className="text-error">{testResult.error ?? "Connection failed"}</span>
            )}
          </span>
        )}
        {bot && !testing && !testResult && (
          <span className="text-sm text-on-surface-variant">Telegram -- not tested</span>
        )}
      </div>
    </StatusCard>
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

  const backendStatus: "ok" | "error" | "idle" = health ? "ok" : health === null ? "error" : "idle";

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between">
        <div>
          <h2 className="font-display text-3xl font-semibold text-on-surface">Overview</h2>
          <p className="mt-1 text-sm text-on-surface-variant">
            Live status of server connection and background workers
          </p>
        </div>
        <HeartbeatDot label={health ? "CORE ONLINE" : "CORE OFFLINE"} online={!!health} />
      </div>

      {/* Status grid */}
      <div className="grid grid-cols-1 sm:grid-cols-3 gap-4">
        <StatusCard title="Backend Connection" status={backendStatus}>
          <div className="flex items-center gap-3">
            <AnimatedDot status={backendStatus === "idle" ? "idle" : backendStatus} />
            {health ? (
              <span className="text-sm text-on-surface">
                Connected &mdash; <span className="font-mono text-cyan">{health.name}</span>
                {health.environment && (
                  <span className="text-on-surface-variant ml-2">({health.environment})</span>
                )}
              </span>
            ) : (
              <span className="text-sm text-on-surface-variant">Disconnected or unreachable</span>
            )}
          </div>
        </StatusCard>

        <DefaultChannelCard />
        <WhisperStatusCard />
      </div>

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
