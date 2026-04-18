"use client";

interface HeartbeatDotProps {
  label?: string;
  online?: boolean;
}

export function HeartbeatDot({ label, online = true }: HeartbeatDotProps) {
  return (
    <div className="inline-flex items-center gap-2">
      <div className="relative h-4 w-4 flex items-center justify-center">
        <span className={`absolute h-2 w-2 rounded-full ${online ? "bg-cyan" : "bg-error"}`} />
        {online ? (
          <>
            <span className="absolute h-2 w-2 rounded-full border border-cyan animate-[heartbeat-ring_2s_ease-out_infinite]" />
            <span className="absolute h-2 w-2 rounded-full border border-cyan animate-[heartbeat-ring_2s_ease-out_infinite_0.4s]" />
          </>
        ) : (
          <span className="absolute h-3.5 w-3.5 rounded-full border border-error/50" />
        )}
        <style>{`
          @keyframes heartbeat-ring {
            0% { transform: scale(1); opacity: 0.6; }
            100% { transform: scale(2.5); opacity: 0; }
          }
        `}</style>
      </div>
      {label && <span className={`text-sm ${online ? "text-on-surface-variant" : "text-error"}`}>{label}</span>}
    </div>
  );
}
