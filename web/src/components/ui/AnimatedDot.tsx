"use client";

type Status = "ok" | "error" | "warn" | "idle" | "pending";

const colors: Record<Status, string> = {
  ok:      "bg-green-400",
  error:   "bg-red-400",
  warn:    "bg-yellow-400",
  idle:    "bg-on-surface-variant/30",
  pending: "bg-cyan",
};

interface AnimatedDotProps {
  status?: Status;
  pulse?: boolean;
  className?: string;
}

export function AnimatedDot({ status = "ok", pulse = true, className = "" }: AnimatedDotProps) {
  const color = colors[status];
  return (
    <span className={`relative inline-flex h-2 w-2 shrink-0 ${className}`}>
      {pulse && status !== "idle" && (
        <span className={`animate-ping absolute inline-flex h-full w-full rounded-full ${color} opacity-60`} />
      )}
      <span className={`relative inline-flex h-2 w-2 rounded-full ${color}`} />
    </span>
  );
}
