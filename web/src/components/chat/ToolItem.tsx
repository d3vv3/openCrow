import { useState } from "react";
import { formatTime, displayValue } from "./helpers";
import { ToolIcon } from "@/components/ui/icons";
import type { ToolCallRecord } from "@/lib/api";

export function ToolItem({ tc, isLive }: { tc: ToolCallRecord; isLive: boolean }) {
  const [isExpanded, setIsExpanded] = useState(false);
  const toggleExpand = () => setIsExpanded((v) => !v);

  const toolKind = tc.kind === "MCP" ? "MCP" : "TOOL";

  const args = tc.arguments ?? {};
  const primaryArgKeys = [
    "command",
    "query",
    "content",
    "prompt",
    "action",
    "url",
    "messageId",
    "memoryId",
    "taskId",
  ];
  const primaryKey = primaryArgKeys.find((k) => k in args) ?? Object.keys(args)[0];
  const primaryVal = primaryKey ? String(args[primaryKey] ?? "") : "";

  let stdout = "";
  let stdoutIsJson = false;
  if (tc.output != null) {
    const raw = String(tc.output);
    try {
      const parsed = JSON.parse(raw);
      const extracted = parsed.stdout ?? parsed.output ?? parsed.result;
      if (extracted !== undefined) {
        const rendered = displayValue(extracted);
        stdout = rendered.text;
        stdoutIsJson = rendered.isJson;
      } else {
        stdout = JSON.stringify(parsed, null, 2);
        stdoutIsJson = true;
      }
    } catch {
      stdout = raw;
    }
  }

  return (
    <div className="flex justify-center animate-in fade-in duration-300">
      <button
        onClick={toggleExpand}
        className="w-full max-w-full sm:max-w-[72%] cursor-pointer text-left rounded-lg border border-white/8 bg-surface-lowest hover:border-white/15 transition-colors font-mono text-xs overflow-hidden"
      >
        <div className="flex items-center gap-2 px-3 py-1.5">
          <ToolIcon className="text-on-surface-variant shrink-0 w-3.5 h-3.5" />
          <span
            className={`shrink-0 text-[10px] px-1.5 py-0.5 rounded border font-semibold ${toolKind === "MCP" ? "text-violet border-violet/40 bg-violet/10" : "text-cyan border-cyan/40 bg-cyan/10"}`}
          >
            [{toolKind}]
          </span>
          <span className="text-cyan shrink-0">{tc.toolName}</span>
          {primaryVal && (
            <span className="text-on-surface/60 truncate flex-1">{primaryVal.slice(0, 80)}</span>
          )}
          {!isLive &&
            (tc.error ? (
              <span className="shrink-0 ml-auto text-[10px] px-1.5 py-0.5 rounded-full font-semibold bg-error/15 text-error border border-error/30">
                failed
              </span>
            ) : (
              <span className="shrink-0 ml-auto text-[10px] px-1.5 py-0.5 rounded-full font-semibold bg-success/15 text-success border border-success/30">
                ok
              </span>
            ))}
          <span className="text-on-surface-variant shrink-0">{formatTime(tc.createdAt)}</span>
          {isLive && (
            <span className="text-on-surface-variant animate-pulse shrink-0 ml-auto">...</span>
          )}
          <span className="text-on-surface-variant shrink-0 ml-1">{isExpanded ? "▲" : "▼"}</span>
        </div>

        {isExpanded && (
          <div className="border-t border-white/8 px-3 py-2 space-y-2">
            {Object.keys(args).length > 0 && (
              <div className="space-y-1">
                {Object.entries(args).map(([k, v]) => {
                  const isObj = typeof v === "object" && v !== null;
                  const prettyVal = isObj ? JSON.stringify(v, null, 2) : String(v);
                  const looksLikeJson =
                    !isObj &&
                    (() => {
                      try {
                        JSON.parse(String(v));
                        return true;
                      } catch {
                        return false;
                      }
                    })();
                  const displayVal = looksLikeJson
                    ? JSON.stringify(JSON.parse(String(v)), null, 2)
                    : prettyVal;
                  const multiline = displayVal.includes("\n");
                  return (
                    <div key={k}>
                      <span className="text-warning">{k}</span>
                      <span className="text-on-surface-variant">=</span>
                      {multiline ? (
                        <pre className="text-success whitespace-pre-wrap break-all mt-0.5 pl-2 border-l border-on-surface-variant/30">
                          {displayVal}
                        </pre>
                      ) : (
                        <span className="text-success break-all ml-1">{displayVal}</span>
                      )}
                    </div>
                  );
                })}
              </div>
            )}
            {stdout && (
              <pre
                className={`whitespace-pre-wrap break-all leading-relaxed ${stdoutIsJson ? "text-on-surface/80 bg-black/20 rounded p-2" : "text-success opacity-80"}`}
              >
                {stdout}
              </pre>
            )}
            {tc.error && <pre className="text-error whitespace-pre-wrap break-all">{tc.error}</pre>}
          </div>
        )}
      </button>
    </div>
  );
}
